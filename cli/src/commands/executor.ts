import type { CommanderGlobalOptions } from './arguments.js'
import type { CommandRegistry } from './registry.js'
import type {
  CommandExecutionGlobals,
  CommandInvocation,
  CommandResult,
  RegisteredCommand,
  RuntimePorts,
} from './types.js'
import process from 'node:process'
import { Command, CommanderError, Option } from 'commander'
import { CLI_VERSION } from '../version.js'
import { resolveGlobalOptions, splitGlobalTokens } from './arguments.js'
import { CliCommandError, toCliCommandError } from './errors.js'
import {
  categoryHelpText,
  commandHelpText,
  localizedCategoryDescription,
  localizedCommandSummary,
  localizeHelp,
  rootHelpText,
} from './human-help.js'
import { ROOT_COMMAND_SHORTCUTS } from './shortcuts.js'

export interface CliProgramOptions {
  readonly registry: CommandRegistry
  readonly ports: RuntimePorts
  readonly name?: string
  readonly description?: string
}

export interface CliRunResult {
  readonly exitCode: number
  readonly error?: unknown
}

const DEFAULT_GLOBALS: CommandExecutionGlobals = Object.freeze({
  output: 'table',
  color: true,
  interactive: true,
  yes: false,
  quiet: false,
  agent: false,
  timeoutMs: 30_000,
  debug: false,
  insecureSkipTlsVerify: false,
})

const PROGRAM_PORTS = new WeakMap<Command, RuntimePorts>()

export function createCliProgram(options: CliProgramOptions): Command {
  const program = localizeHelp(new Command()
    .name(options.name ?? 'luna')
    .description(options.description ?? 'Luna DevOps command-line client')
    .version(
      options.ports.version ?? CLI_VERSION,
      '-V, --version',
      translate(options.ports, 'help.options.version', 'Show the CLI version'),
    )
    .showHelpAfterError()
    .showSuggestionAfterError()
    .allowExcessArguments(false)
    .allowUnknownOption(false)
    .addHelpCommand(false)
    .helpOption(
      '-h, --help',
      translate(options.ports, 'help.options.help', 'Show command help'),
    ), options.ports)
  PROGRAM_PORTS.set(program, options.ports)

  addGlobalOptions(program, options.ports)
  program.addHelpText('after', () => rootHelpText(options.registry, options.ports))
  registerRootShortcuts(program, options.registry, options.ports)
  for (const category of options.registry.categories()) {
    const categoryCommand = localizeHelp(program
      .command(category)
      .description(localizedCategoryDescription(category, options.ports))
      .addHelpCommand(false), options.ports)
      .helpOption(
        '-h, --help',
        translate(options.ports, 'help.options.help', 'Show command help'),
      )
      .addHelpText('after', () => categoryHelpText(category, options.ports))
    for (const categoryAlias of options.registry.categoryAliases(category)) {
      categoryCommand.alias(categoryAlias)
    }

    const commands = options.registry.list({ category, includeHidden: true })
    for (const registered of commands) {
      const tool = localizeHelp(categoryCommand
        .command(registered.metadata.tool, { hidden: registered.metadata.hidden })
        .description(localizedCommandSummary(registered.metadata, options.ports))
        .argument(
          '[arguments...]',
          translate(
            options.ports,
            'help.businessArguments',
            'Business parameters in key=value form',
          ),
        )
        .addHelpCommand(false)
        .allowUnknownOption(false)
        .action(async (
          tokens: string[] | undefined,
          _localOptions: unknown,
          command: Command,
        ) => {
          const invokedPath = invokedCommandPath(command)
          await executeRegistered(
            registered,
            tokens ?? [],
            explicitCommanderOptions(command),
            options.ports,
            invokedPath,
          )
        }), options.ports)
        .helpOption(
          '-h, --help',
          translate(options.ports, 'help.options.help', 'Show command help'),
        )
        .addHelpText(
          'after',
          () => commandHelpText(registered.metadata, options.ports),
        )
      for (const alias of registered.metadata.aliases) {
        const canonicalOwner = options.registry.get(
          `${registered.metadata.category}.${alias}`,
        )
        if (!canonicalOwner || canonicalOwner === registered)
          tool.alias(alias)
      }
    }
  }
  return program
}

function registerRootShortcuts(
  program: Command,
  registry: CommandRegistry,
  ports: RuntimePorts,
): void {
  for (const shortcut of ROOT_COMMAND_SHORTCUTS) {
    const registered = registry.get(shortcut.target)
    if (!registered)
      continue

    localizeHelp(program
      .command(shortcut.name)
      .description(translate(
        ports,
        shortcut.descriptionKey,
        shortcut.description,
      ))
      .argument(
        '[arguments...]',
        translate(
          ports,
          'help.businessArguments',
          'Business parameters in key=value form',
        ),
      )
      .addHelpCommand(false)
      .allowUnknownOption(false)
      .action(async (
        tokens: string[] | undefined,
        _localOptions: unknown,
        command: Command,
      ) => {
        await executeRegistered(
          registered,
          tokens ?? [],
          explicitCommanderOptions(command),
          ports,
          shortcut.name,
        )
      }), ports)
      .helpOption(
        '-h, --help',
        translate(ports, 'help.options.help', 'Show command help'),
      )
      .addHelpText(
        'after',
        () => [
          '',
          `${translate(ports, 'help.canonicalCommand', 'Canonical command')}: ${shortcut.target}`,
          '',
          commandHelpText(registered.metadata, ports),
        ].join('\n'),
      )
  }
}

export async function runCli(
  program: Command,
  argv: readonly string[] = process.argv,
  fallbackOutput?: RuntimePorts['output'],
): Promise<CliRunResult> {
  const fallbackGlobals = inferFallbackGlobals(argv)
  const rootOnly = isRootOnlyInvocation(program, argv)
  const restoreCommanderOutput = configureCommanderOutput(
    program,
    isMachineOutput(fallbackGlobals),
  )
  for (const command of commandTree(program))
    command.exitOverride()
  try {
    if (rootOnly) {
      if (isMachineOutput(fallbackGlobals)) {
        throw new CliCommandError(
          'command_required',
          translate(
            PROGRAM_PORTS.get(program),
            'errors.command_required',
            'Choose a command. Run luna --help to see available commands.',
          ),
          {
            status: 400,
            exitCode: 2,
          },
        )
      }
      program.outputHelp()
      return { exitCode: 0 }
    }
    await program.parseAsync([...argv], { from: 'node' })
    return { exitCode: 0 }
  }
  catch (error) {
    if (error instanceof CommanderError && isExpectedCommanderExit(error)) {
      return { exitCode: 0 }
    }
    const normalized = commanderFailure(error, program, argv)
    await fallbackOutput?.writeError(normalized, fallbackGlobals)
    return { exitCode: normalized.exitCode, error: normalized }
  }
  finally {
    restoreCommanderOutput()
  }
}

function isRootOnlyInvocation(program: Command, argv: readonly string[]): boolean {
  const tokens = argv.slice(2)
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index]
    if (!token)
      continue
    if (token === '--')
      return index === tokens.length - 1
    if (!token.startsWith('-'))
      return false

    const flag = token.includes('=') ? token.slice(0, token.indexOf('=')) : token
    const option = program.options.find(candidate =>
      candidate.short === flag || candidate.long === flag)
    if (!option)
      return false
    if (option.long === '--help' || option.long === '--version')
      return false
    if (!token.includes('=') && (option.required || option.optional)) {
      const value = tokens[index + 1]
      if (!value || value.startsWith('-'))
        return false
      index += 1
    }
  }
  return true
}

async function executeRegistered(
  registered: RegisteredCommand,
  tokens: readonly string[],
  flagOptions: CommanderGlobalOptions,
  ports: RuntimePorts,
  invokedPath: string,
): Promise<void> {
  const parsed = splitGlobalTokens(tokens)
  const config = await ports.config.read()
  const globals = resolveGlobalOptions(parsed.canonicalGlobals, flagOptions, {
    env: ports.env ?? process.env,
    configured: {
      output: config.output,
      project: config.project,
      language: config.language,
    },
    isTTY: ports.isTTY ?? Boolean(process.stdout.isTTY),
    streaming: registered.metadata.streaming ?? false,
  })

  const inputMetadata = metadataWithResolvedProjectRequirement(registered, globals)
  const parsedParams = await ports.input.parse(parsed.businessTokens, inputMetadata)
  const params = resolveProjectParameters(parsedParams, registered, globals)
  enforceExecutionScope(
    registered,
    invokedPath,
    globals,
    parsed.explicitGlobalKeys,
    parsedParams,
    params,
  )
  await enforceRiskPolicy(registered, invokedPath, globals, ports)
  const invocation: CommandInvocation = {
    metadata: registered.metadata,
    params,
    globals,
    explicitGlobalKeys: parsed.explicitGlobalKeys,
    canonicalGlobalValues: parsed.canonicalGlobals,
  }
  const result = normalizeResult(
    await registered.handler(invocation, ports),
    registered.metadata.schemaVersion,
  )
  await ports.output.writeSuccess(registered.metadata, result, globals)
}

function metadataWithResolvedProjectRequirement(
  registered: RegisteredCommand,
  globals: CommandExecutionGlobals,
): RegisteredCommand['metadata'] {
  if (!globals.project)
    return registered.metadata
  const parameters = registered.metadata.parameters.map(parameter =>
    parameter.required && isProjectParameter(parameter.name)
      ? { ...parameter, required: false }
      : parameter,
  )
  return { ...registered.metadata, parameters }
}

function resolveProjectParameters(
  parsedParams: Readonly<Record<string, unknown>>,
  registered: RegisteredCommand,
  globals: CommandExecutionGlobals,
): Readonly<Record<string, unknown>> {
  const requiredProjectParameters = registered.metadata.parameters
    .filter(parameter => parameter.required && isProjectParameter(parameter.name))
  if (requiredProjectParameters.length === 0)
    return parsedParams

  const params = { ...parsedParams }
  const structured = isRecord(params.params) ? { ...params.params } : undefined
  for (const parameter of requiredProjectParameters) {
    const explicitValue = params[parameter.name] ?? structured?.[parameter.name]
    const value = explicitValue ?? globals.project
    if (value === undefined || value === null || value === '') {
      throw new CliCommandError(
        'invalid_arguments',
        'Input validation failed.',
        {
          status: 400,
          exitCode: 2,
          details: {
            command: registered.metadata.canonicalPath,
            fields: [{ key: parameter.name, code: 'required' }],
          },
        },
      )
    }
    params[parameter.name] = value
    if (structured)
      delete structured[parameter.name]
  }
  if (structured)
    params.params = structured
  return params
}

function isProjectParameter(name: string): boolean {
  return name === 'project' || name === 'projectId' || name === 'projectID'
}

function enforceExecutionScope(
  registered: RegisteredCommand,
  requestedPath: string,
  globals: CommandExecutionGlobals,
  explicitGlobalKeys: ReadonlySet<string>,
  explicitParams: Readonly<Record<string, unknown>>,
  params: Readonly<Record<string, unknown>>,
): void {
  if (globals.agent && requestedPath !== registered.metadata.canonicalPath) {
    throw new CliCommandError(
      'agent_alias_forbidden',
      `Agent mode requires the canonical command "${registered.metadata.canonicalPath}".`,
      {
        status: 400,
        exitCode: 2,
        details: {
          command: registered.metadata.canonicalPath,
          invokedAs: requestedPath,
        },
      },
    )
  }
  if (globals.agent && !registered.metadata.agentAllowed) {
    throw new CliCommandError(
      'agent_command_forbidden',
      `Command "${requestedPath}" is not available in agent mode.`,
      { status: 403, details: { command: requestedPath } },
    )
  }
  if (
    registered.metadata.projectContext === 'required'
    && !hasProjectSelection(globals, params)
  ) {
    throw new CliCommandError(
      'project_required',
      `Command "${requestedPath}" requires a project.`,
      { status: 400, exitCode: 2, details: { command: requestedPath } },
    )
  }
  if (
    registered.metadata.projectContext === 'none'
    && explicitGlobalKeys.has('project')
  ) {
    throw new CliCommandError(
      'project_not_supported',
      `Command "${requestedPath}" does not accept a project selection.`,
      { status: 400, exitCode: 2, details: { command: requestedPath } },
    )
  }
  if (
    globals.agent
    && registered.metadata.projectContext === 'required'
    && registered.metadata.risk !== 'low'
    && !hasExplicitProjectSelection(explicitGlobalKeys, explicitParams)
  ) {
    throw new CliCommandError(
      'explicit_project_required',
      'Agent mode requires an explicit project=<id> for project-scoped mutations.',
      { status: 400, exitCode: 2, details: { command: requestedPath } },
    )
  }
  if (globals.agent && globals.interactive) {
    throw new CliCommandError(
      'agent_interactive_forbidden',
      'Agent mode cannot enable interactive input.',
      { status: 400, exitCode: 2 },
    )
  }
}

function hasProjectSelection(
  globals: CommandExecutionGlobals,
  params: Readonly<Record<string, unknown>>,
): boolean {
  return Boolean(globals.project) || projectValue(params) !== undefined
}

function hasExplicitProjectSelection(
  explicitGlobalKeys: ReadonlySet<string>,
  params: Readonly<Record<string, unknown>>,
): boolean {
  return explicitGlobalKeys.has('project') || projectValue(params) !== undefined
}

function projectValue(params: Readonly<Record<string, unknown>>): unknown {
  const structured = isRecord(params.params) ? params.params : undefined
  for (const name of ['project', 'projectId', 'projectID']) {
    const value = params[name] ?? structured?.[name]
    if (value !== undefined && value !== null && value !== '')
      return value
  }
  return undefined
}

async function enforceRiskPolicy(
  registered: RegisteredCommand,
  requestedPath: string,
  globals: CommandExecutionGlobals,
  ports: RuntimePorts,
): Promise<void> {
  const risk = registered.metadata.risk
  if (risk === 'low' || globals.dryRun)
    return

  if (registered.metadata.source === 'local' && risk === 'medium')
    return
  if (globals.yes)
    return

  if (!globals.interactive || !ports.input.confirm) {
    throw new CliCommandError(
      'confirmation_required',
      `Command "${requestedPath}" requires confirmation. Re-run interactively or pass --yes.`,
      {
        status: 412,
        exitCode: 6,
        details: {
          command: registered.metadata.canonicalPath,
          risk,
        },
      },
    )
  }

  const prompt = ports.translate?.(
    'confirm.execute',
    `Run ${registered.metadata.canonicalPath}?`,
    globals.lang,
  ) ?? `Run ${registered.metadata.canonicalPath}?`
  if (!await ports.input.confirm(prompt)) {
    throw new CliCommandError(
      'operation_cancelled',
      'Operation cancelled.',
      {
        status: 409,
        exitCode: 6,
        details: { command: registered.metadata.canonicalPath },
      },
    )
  }
}

function normalizeResult(value: unknown, schemaVersion?: string): CommandResult {
  if (isRecord(value) && 'data' in value
    && ('schemaVersion' in value || 'meta' in value)) {
    return value as unknown as CommandResult
  }
  return { data: value, schemaVersion }
}

function addGlobalOptions(program: Command, ports: RuntimePorts): void {
  program
    .option('--server <url>', translate(ports, 'help.options.server', 'Override the Luna server origin'))
    .option('--project <id>', translate(ports, 'help.options.project', 'Select a project for this command'))
    .addOption(new Option('-o, --output <format>', translate(ports, 'help.options.output', 'Output format'))
      .choices(['table', 'json', 'raw-json', 'yaml', 'jsonl', 'name']))
    .option('--lang <locale>', translate(ports, 'help.options.lang', 'Output and help language'))
    .option('--no-color', translate(ports, 'help.options.noColor', 'Disable terminal colors'))
    .option('--no-interactive', translate(ports, 'help.options.noInteractive', 'Disable prompts'))
    .option('-y, --yes', translate(ports, 'help.options.yes', 'Approve supported confirmation prompts'))
    .option('--quiet', translate(ports, 'help.options.quiet', 'Suppress informational diagnostics'))
    .option('--agent', translate(ports, 'help.options.agent', 'Enable strict machine-readable agent mode'))
    .addOption(new Option('--dry-run <mode>', translate(ports, 'help.options.dryRun', 'Preview without applying'))
      .choices(['client', 'server']))
    .option('--timeout <duration>', translate(ports, 'help.options.timeout', 'Request timeout'))
    .option('--debug', translate(ports, 'help.options.debug', 'Enable debug diagnostics'))
    .option('--request-id <id>', translate(ports, 'help.options.requestId', 'Use a request correlation ID'))
    .option('--idempotency-key <key>', translate(ports, 'help.options.idempotencyKey', 'Use an idempotency key'))
    .option('--insecure-skip-tls-verify', translate(ports, 'help.options.insecureTls', 'Disable TLS verification when supported'))
}

function translate(
  ports: RuntimePorts | undefined,
  key: string,
  fallback: string,
): string {
  return ports?.translate?.(key, fallback) ?? fallback
}

function invokedCommandPath(command: Command): string {
  const canonicalCategory = command.parent?.name() ?? ''
  const canonicalTool = command.name()
  const rootOperands = command.parent?.parent?.args ?? []
  const invokedCategory = typeof rootOperands[0] === 'string'
    ? rootOperands[0]
    : canonicalCategory
  const invokedTool = typeof rootOperands[1] === 'string'
    ? rootOperands[1]
    : canonicalTool
  return `${invokedCategory}.${invokedTool}`
}

function explicitCommanderOptions(command: Command): CommanderGlobalOptions {
  const values = command.optsWithGlobals<CommanderGlobalOptions>()
  return Object.fromEntries(
    Object.entries(values).filter(([key]) =>
      command.getOptionValueSourceWithGlobals(key) !== 'default'),
  ) as CommanderGlobalOptions
}

function commanderFailure(
  error: unknown,
  program: Command,
  argv: readonly string[],
): CliCommandError {
  if (!(error instanceof CommanderError))
    return toCliCommandError(error)
  const ports = PROGRAM_PORTS.get(program)
  const helpCommand = commanderHelpCommand(program, argv)
  const rawMessage = cleanCommanderMessage(error.message)
  const unknown = error.code === 'commander.unknownCommand'
  const message = unknown
    ? localizedUnknownCommandMessage(rawMessage, helpCommand, ports)
    : localizedInvalidArgumentsMessage(helpCommand, ports)
  return new CliCommandError(
    unknown ? 'unknown_command' : 'invalid_arguments',
    message,
    {
      status: 400,
      exitCode: 2,
      details: { commanderCode: error.code },
      cause: error,
    },
  )
}

function inferFallbackGlobals(argv: readonly string[]): Partial<CommandExecutionGlobals> {
  const agent = argv.includes('--agent') || argv.includes('agent=true')
  const outputToken = argv.find(token => token.startsWith('output='))
  const outputFlagIndex = argv.findIndex(token => token === '--output' || token === '-o')
  const output = outputToken?.slice('output='.length)
    ?? (outputFlagIndex >= 0 ? argv[outputFlagIndex + 1] : undefined)
  return {
    ...DEFAULT_GLOBALS,
    agent,
    output: isOutput(output) ? output : agent ? 'json' : 'table',
  }
}

function configureCommanderOutput(program: Command, machine: boolean): () => void {
  const snapshots = commandTree(program).map(command => ({
    command,
    output: command.configureOutput(),
  }))
  for (const snapshot of snapshots) {
    snapshot.command.configureOutput({
      writeOut: machine ? () => undefined : snapshot.output.writeOut,
      writeErr: () => undefined,
      outputError: () => undefined,
    })
  }
  return () => {
    for (const snapshot of snapshots)
      snapshot.command.configureOutput(snapshot.output)
  }
}

function commandTree(root: Command): Command[] {
  return [root, ...root.commands.flatMap(commandTree)]
}

function isMachineOutput(
  globals: Partial<CommandExecutionGlobals>,
): boolean {
  return Boolean(globals.agent || globals.output !== 'table')
}

function isExpectedCommanderExit(error: CommanderError): boolean {
  return error.code === 'commander.helpDisplayed' || error.code === 'commander.version'
}

function cleanCommanderMessage(value: string): string {
  return value.replace(/^error:\s*/i, '').trim() || 'Invalid command arguments.'
}

function localizedUnknownCommandMessage(
  rawMessage: string,
  helpCommand: string,
  ports: RuntimePorts | undefined,
): string {
  const command = /unknown command ['"]([^'"]+)['"]/i.exec(rawMessage)?.[1]
  const suggestion = /did you mean ([^)]+)\?/i.exec(rawMessage)?.[1]?.trim()
  const lines = [
    command
      ? `${translate(ports, 'help.errors.unknownCommand', 'Unknown command')}: ${command}`
      : translate(ports, 'help.errors.unknownCommand', 'Unknown command.'),
  ]
  if (suggestion) {
    lines.push(
      `${translate(ports, 'help.errors.didYouMean', 'Did you mean')}: ${suggestion}`,
    )
  }
  lines.push(
    `${translate(ports, 'help.errors.nextStep', 'View available commands')}: ${helpCommand}`,
  )
  return lines.join('\n')
}

function localizedInvalidArgumentsMessage(
  helpCommand: string,
  ports: RuntimePorts | undefined,
): string {
  return [
    translate(ports, 'help.errors.invalidArguments', 'Invalid command arguments.'),
    `${translate(ports, 'help.errors.nextStep', 'View command parameters')}: ${helpCommand}`,
  ].join('\n')
}

function commanderHelpCommand(program: Command, argv: readonly string[]): string {
  const rootName = program.name()
  const operands = argv.slice(2).filter(token =>
    token.length > 0
    && !token.startsWith('-')
    && !token.includes('='),
  )
  const category = operands[0]
  if (!category || !program.commands.some(command =>
    command.name() === category || command.aliases().includes(category),
  )) {
    return `${rootName} --help`
  }
  return `${rootName} ${category} --help`
}

function isOutput(value: unknown): value is CommandExecutionGlobals['output'] {
  return value === 'table'
    || value === 'json'
    || value === 'raw-json'
    || value === 'yaml'
    || value === 'jsonl'
    || value === 'name'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
