import type {
  CommandExecutionGlobals,
  CommandParameter,
  InputPort,
  NormalizedCommandMetadata,
  OutputFormat,
} from './types.js'
import { Buffer } from 'node:buffer'
import { readFile } from 'node:fs/promises'
import { extname } from 'node:path'
import process from 'node:process'
import { confirm } from '@inquirer/prompts'
import { parse as parseYaml } from 'yaml'
import { parseCommandInput } from '../input/index.js'
import { CliCommandError } from './errors.js'

const KEY_PATTERN = /^[A-Z][A-Z0-9]*$/i
const INLINE_LIMIT_BYTES = 4 * 1024
const OUTPUT_FORMATS = new Set<OutputFormat>([
  'table',
  'json',
  'raw-json',
  'yaml',
  'jsonl',
  'name',
])
const GLOBAL_KEYS = new Set([
  'server',
  'project',
  'output',
  'lang',
  'color',
  'interactive',
  'yes',
  'quiet',
  'agent',
  'dryRun',
  'timeout',
  'debug',
  'requestId',
  'idempotencyKey',
  'insecureSkipTlsVerify',
])

export interface ParsedCommandTokens {
  readonly businessTokens: readonly string[]
  readonly canonicalGlobals: Readonly<Record<string, string>>
  readonly explicitGlobalKeys: ReadonlySet<string>
}

export interface CommanderGlobalOptions {
  server?: string
  project?: string
  output?: string
  lang?: string
  color?: boolean
  interactive?: boolean
  yes?: boolean
  quiet?: boolean
  agent?: boolean
  dryRun?: string
  timeout?: string
  debug?: boolean
  requestId?: string
  idempotencyKey?: string
  insecureSkipTlsVerify?: boolean
}

export class DefaultInputPort implements InputPort {
  async parse(
    tokens: readonly string[],
    metadata: NormalizedCommandMetadata,
  ): Promise<Readonly<Record<string, unknown>>> {
    if (metadata.inputSchema?.additionalProperties !== true) {
      const parsed = await parseCommandInput(tokens, {
        command: metadata.canonicalPath,
        fields: metadata.parameters.map(parameter => ({
          name: parameter.name,
          required: parameter.required,
          repeated: parameter.repeated,
          sensitive: parameter.sensitive,
          valueSources: parameter.valueSources,
          schema: parameter.schema,
        })),
        paramsSchema: metadata.inputSchema,
      })
      return parsed.values
    }
    return parseBusinessArguments(tokens, metadata)
  }

  confirm(message: string): Promise<boolean> {
    return confirm({ message, default: false })
  }
}

export function splitGlobalTokens(tokens: readonly string[]): ParsedCommandTokens {
  const canonicalGlobals: Record<string, string> = {}
  const businessTokens: string[] = []
  const explicitGlobalKeys = new Set<string>()

  for (const token of tokens) {
    const { key, value } = splitKeyValue(token)
    if (!GLOBAL_KEYS.has(key)) {
      businessTokens.push(token)
      continue
    }
    if (key in canonicalGlobals) {
      throw invalidArguments(`Global argument "${key}" may only be provided once.`, { key })
    }
    canonicalGlobals[key] = value
    explicitGlobalKeys.add(key)
  }

  return { businessTokens, canonicalGlobals, explicitGlobalKeys }
}

export async function parseBusinessArguments(
  tokens: readonly string[],
  metadata: NormalizedCommandMetadata,
): Promise<Readonly<Record<string, unknown>>> {
  const definitions = new Map(metadata.parameters.map(parameter => [parameter.name, parameter]))
  const values: Record<string, unknown> = {}
  let stdinUsed = false

  for (const token of tokens) {
    const { key, value } = splitKeyValue(token)
    const definition = definitions.get(key)
    if (!definition && metadata.inputSchema?.additionalProperties !== true) {
      throw invalidArguments(`Unknown argument "${key}" for ${metadata.canonicalPath}.`, {
        key,
        command: metadata.canonicalPath,
      })
    }

    const parsed = await parseValueSource(value, definition)
    if (parsed.stdin) {
      if (stdinUsed) {
        throw invalidArguments('Only one argument may read from stdin.', {
          code: 'stdin_already_used',
        })
      }
      stdinUsed = true
    }
    appendValue(values, key, parsed.value, Boolean(definition?.repeated))
  }

  for (const parameter of metadata.parameters) {
    if (parameter.required && !(parameter.name in values)) {
      throw invalidArguments(`Missing required argument "${parameter.name}".`, {
        key: parameter.name,
        code: 'required',
      })
    }
  }
  if ('params' in values && Object.keys(values).length > 1) {
    throw invalidArguments('params cannot be combined with individual business arguments.', {
      code: 'params_conflict',
    })
  }
  return values
}

export function resolveGlobalOptions(
  canonical: Readonly<Record<string, string>>,
  flags: CommanderGlobalOptions,
  options: {
    env: Readonly<Record<string, string | undefined>>
    configured?: Readonly<{
      output?: string
      project?: { id?: string } | null
      language?: string
    }>
    isTTY: boolean
    streaming: boolean
  },
): CommandExecutionGlobals {
  assertNoConflicts(canonical, flags)
  const env = options.env
  const agent = booleanOption(
    first(canonical.agent, flagBoolean(flags.agent), env.LUNA_AGENT),
    false,
    'agent',
  )
  const outputCandidate = first(
    canonical.output,
    flags.output,
    env.LUNA_OUTPUT,
    options.configured?.output,
    options.isTTY ? 'table' : 'json',
  )
  const output = agent ? (options.streaming ? 'jsonl' : 'json') : outputCandidate
  if (!OUTPUT_FORMATS.has(output as OutputFormat)) {
    throw invalidArguments(`Unsupported output format "${output}".`, { output })
  }
  if (agent && output === 'raw-json') {
    throw invalidArguments('Agent mode does not allow raw-json output.', {
      code: 'agent_raw_json_forbidden',
    })
  }

  return {
    server: first(canonical.server, flags.server, env.LUNA_SERVER),
    project: first(
      canonical.project,
      flags.project,
      env.LUNA_PROJECT,
      options.configured?.project?.id,
    ),
    output: output as OutputFormat,
    lang: first(canonical.lang, flags.lang, env.LUNA_LANG, options.configured?.language),
    color: agent
      ? false
      : booleanOption(first(canonical.color, flagBoolean(flags.color), env.LUNA_COLOR), true, 'color'),
    interactive: agent
      ? false
      : booleanOption(
          first(canonical.interactive, flagBoolean(flags.interactive), env.LUNA_INTERACTIVE),
          options.isTTY,
          'interactive',
        ),
    yes: booleanOption(first(canonical.yes, flagBoolean(flags.yes), env.LUNA_YES), false, 'yes'),
    quiet: agent
      ? true
      : booleanOption(first(canonical.quiet, flagBoolean(flags.quiet), env.LUNA_QUIET), false, 'quiet'),
    agent,
    dryRun: dryRunOption(first(canonical.dryRun, flags.dryRun, env.LUNA_DRY_RUN)),
    timeoutMs: durationMilliseconds(
      first(canonical.timeout, flags.timeout, env.LUNA_TIMEOUT, '30s'),
    ),
    debug: booleanOption(
      first(canonical.debug, flagBoolean(flags.debug), env.LUNA_DEBUG),
      false,
      'debug',
    ),
    requestId: first(canonical.requestId, flags.requestId, env.LUNA_REQUEST_ID),
    idempotencyKey: first(
      canonical.idempotencyKey,
      flags.idempotencyKey,
      env.LUNA_IDEMPOTENCY_KEY,
    ),
    insecureSkipTlsVerify: booleanOption(
      first(
        canonical.insecureSkipTlsVerify,
        flagBoolean(flags.insecureSkipTlsVerify),
        env.LUNA_INSECURE_SKIP_TLS_VERIFY,
      ),
      false,
      'insecureSkipTlsVerify',
    ),
  }
}

function splitKeyValue(token: string): { key: string, value: string } {
  const separator = token.indexOf('=')
  if (separator <= 0) {
    throw invalidArguments(`Argument "${token}" must use key=value syntax.`, { token })
  }
  const key = token.slice(0, separator)
  if (!KEY_PATTERN.test(key)) {
    throw invalidArguments(`Invalid argument key "${key}".`, { key })
  }
  return { key, value: token.slice(separator + 1) }
}

async function parseValueSource(
  raw: string,
  definition?: CommandParameter,
): Promise<{ value: unknown, stdin: boolean }> {
  if (raw.startsWith('@@')) {
    return { value: convertInline(raw.slice(1), definition), stdin: false }
  }
  if (raw === '@-') {
    if (definition?.valueSources && !definition.valueSources.includes('stdin')) {
      throw invalidArguments(`Argument "${definition.name}" does not accept stdin.`, {
        key: definition.name,
      })
    }
    const content = await readStdin()
    return { value: parseFileContent(content, undefined, definition), stdin: true }
  }
  if (raw.startsWith('@')) {
    if (definition?.valueSources && !definition.valueSources.includes('file')) {
      throw invalidArguments(`Argument "${definition.name}" does not accept file input.`, {
        key: definition.name,
      })
    }
    const path = raw.slice(1)
    if (!path)
      throw invalidArguments('File input path cannot be empty.')
    const content = await readFile(path, 'utf8')
    return { value: parseFileContent(content, extname(path), definition), stdin: false }
  }
  if (definition?.sensitive || (definition?.valueSources && !definition.valueSources.includes('inline'))) {
    throw invalidArguments(`Argument "${definition?.name ?? 'value'}" cannot be provided inline.`, {
      key: definition?.name,
      code: 'sensitive_inline_forbidden',
    })
  }
  if (Buffer.byteLength(raw, 'utf8') > INLINE_LIMIT_BYTES || raw.includes('\n')) {
    throw invalidArguments('Inline value is too large; use @file or @-.', {
      code: 'inline_value_too_large',
      limitBytes: INLINE_LIMIT_BYTES,
    })
  }
  return { value: convertInline(raw, definition), stdin: false }
}

function convertInline(raw: string, definition?: CommandParameter): unknown {
  const schema = definition?.schema ?? {}
  const type = schema.type
  if (raw === 'null') {
    if (schema.nullable === true || (Array.isArray(type) && type.includes('null')))
      return null
    return raw
  }
  if (type === 'boolean') {
    if (raw === 'true')
      return true
    if (raw === 'false')
      return false
    throw invalidArguments(`Argument "${definition?.name}" must be true or false.`)
  }
  if (type === 'integer' || type === 'number') {
    const value = Number(raw)
    if (!Number.isFinite(value) || (type === 'integer' && !Number.isInteger(value))) {
      throw invalidArguments(`Argument "${definition?.name}" must be a valid ${type}.`)
    }
    return value
  }
  if (type === 'object' || type === 'array') {
    try {
      return JSON.parse(raw)
    }
    catch (cause) {
      throw new CliCommandError('invalid_arguments', `Argument "${definition?.name}" must be JSON.`, {
        status: 400,
        exitCode: 2,
        cause,
      })
    }
  }
  return raw
}

function parseFileContent(
  content: string,
  extension: string | undefined,
  definition?: CommandParameter,
): unknown {
  const type = definition?.schema?.type
  if (extension === '.yaml' || extension === '.yml')
    return parseYaml(content)
  if (extension === '.json' || type === 'object' || type === 'array') {
    try {
      return JSON.parse(content)
    }
    catch (cause) {
      throw new CliCommandError('invalid_arguments', 'Input is not valid JSON.', {
        status: 400,
        exitCode: 2,
        cause,
      })
    }
  }
  return content
}

function appendValue(
  target: Record<string, unknown>,
  key: string,
  value: unknown,
  repeated: boolean,
): void {
  if (!(key in target)) {
    target[key] = repeated ? [value] : value
    return
  }
  if (!repeated) {
    throw invalidArguments(`Argument "${key}" may only be provided once.`, { key })
  }
  (target[key] as unknown[]).push(value)
}

async function readStdin(): Promise<string> {
  const chunks: Buffer[] = []
  for await (const chunk of process.stdin) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk))
  }
  return Buffer.concat(chunks).toString('utf8')
}

function assertNoConflicts(
  canonical: Readonly<Record<string, string>>,
  flags: CommanderGlobalOptions,
): void {
  const flagValues: Readonly<Record<string, string | undefined>> = {
    server: flags.server,
    project: flags.project,
    output: flags.output,
    lang: flags.lang,
    color: flagBoolean(flags.color),
    interactive: flagBoolean(flags.interactive),
    yes: flagBoolean(flags.yes),
    quiet: flagBoolean(flags.quiet),
    agent: flagBoolean(flags.agent),
    dryRun: flags.dryRun,
    timeout: flags.timeout,
    debug: flagBoolean(flags.debug),
    requestId: flags.requestId,
    idempotencyKey: flags.idempotencyKey,
    insecureSkipTlsVerify: flagBoolean(flags.insecureSkipTlsVerify),
  }
  for (const [key, canonicalValue] of Object.entries(canonical)) {
    const flagValue = flagValues[key]
    if (flagValue !== undefined && canonicalValue !== flagValue) {
      throw invalidArguments(`Conflicting values were provided for "${key}".`, {
        key,
        canonicalValue,
        flagValue,
      })
    }
  }
}

function first(...values: (string | undefined)[]): string | undefined {
  return values.find(value => value !== undefined && value !== '')
}

function flagBoolean(value: boolean | undefined): string | undefined {
  return value === undefined ? undefined : String(value)
}

function booleanOption(value: string | undefined, fallback: boolean, key: string): boolean {
  if (value === undefined)
    return fallback
  if (value === 'true' || value === '1')
    return true
  if (value === 'false' || value === '0')
    return false
  throw invalidArguments(`Global option "${key}" must be true or false.`, { key, value })
}

function dryRunOption(value: string | undefined): 'client' | 'server' | undefined {
  if (value === undefined)
    return undefined
  if (value === 'client' || value === 'server')
    return value
  throw invalidArguments('dryRun must be client or server.', { value })
}

export function durationMilliseconds(value: string | undefined): number {
  if (!value)
    return 30_000
  const match = /^(\d+)(ms|[smh])$/.exec(value)
  if (!match)
    throw invalidArguments(`Invalid duration "${value}".`, { value })
  const amount = Number(match[1])
  const multiplier = { ms: 1, s: 1_000, m: 60_000, h: 3_600_000 }[match[2]!]!
  const result = amount * multiplier
  if (!Number.isSafeInteger(result) || result <= 0) {
    throw invalidArguments(`Invalid duration "${value}".`, { value })
  }
  return result
}

function invalidArguments(
  message: string,
  details: Readonly<Record<string, unknown>> = {},
): CliCommandError {
  return new CliCommandError('invalid_arguments', message, {
    status: 400,
    exitCode: 2,
    details,
  })
}
