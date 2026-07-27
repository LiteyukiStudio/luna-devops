import type { Command } from 'commander'
import type { CommandRegistry } from './registry.js'
import type {
  CommandParameter,
  NormalizedCommandMetadata,
  RuntimePorts,
} from './types.js'

const HELP_TITLES: Readonly<Record<string, readonly [string, string]>> = {
  'Usage:': ['help.headings.usage', 'Usage:'],
  'Arguments:': ['help.headings.arguments', 'Arguments:'],
  'Options:': ['help.headings.options', 'Options:'],
  'Global Options:': ['help.headings.globalOptions', 'Global Options:'],
  'Commands:': ['help.headings.commands', 'Commands:'],
}

export function localizeHelp(command: Command, ports: RuntimePorts): Command {
  return command.configureHelp({
    showGlobalOptions: command.parent !== null,
    styleTitle(title) {
      const translation = HELP_TITLES[title]
      return translation ? text(ports, translation[0], translation[1]) : title
    },
  })
}

export function localizedCategoryDescription(
  category: string,
  ports: RuntimePorts,
): string {
  return text(
    ports,
    `categories.${category}`,
    format(
      text(ports, 'help.categoryDescription', '{{category}} commands'),
      { category },
    ),
  )
}

export function localizedCommandSummary(
  metadata: NormalizedCommandMetadata,
  ports: RuntimePorts,
): string {
  return text(
    ports,
    metadata.summaryKey ?? `commands.${metadata.category}.${metadata.tool}.summary`,
    metadata.summary ?? metadata.canonicalPath,
  )
}

export function rootHelpText(
  registry: CommandRegistry,
  ports: RuntimePorts,
): string {
  return [
    '',
    `${text(ports, 'help.quickStart.title', 'Quick start:')}`,
    `  ${text(ports, 'help.quickStart.login', '1. Sign in with OAuth Device Code:')}`,
    '     luna login',
    `  ${text(ports, 'help.quickStart.customServer', '2. Sign in to another server when needed:')}`,
    '     luna login server=https://luna.example.com',
    `  ${text(ports, 'help.quickStart.discover', '3. Discover commands:')}`,
    '     luna help catalog query=project limit=10',
    '     luna <category> <command> --help',
    '',
    `${text(ports, 'help.input.title', 'Input syntax:')}`,
    `  ${text(ports, 'help.input.keyValue', 'Business parameters use key=value.')}`,
    `  ${text(ports, 'help.input.files', 'Use key=@file.json or key=@- for files, JSON, and multiline input.')}`,
    `  ${text(ports, 'help.input.repeat', 'Repeat a key for array parameters: scope=read scope=write.')}`,
    '',
    format(
      text(
        ports,
        'help.catalogSummary',
        '{{commands}} commands in {{categories}} categories. Use output=json for machine-readable results.',
      ),
      {
        commands: String(registry.list().length),
        categories: String(registry.categories().length),
      },
    ),
    `  ${text(ports, 'help.localeHint', 'Set LUNA_LANG=zh-CN or pass --lang zh-CN to change the help language.')}`,
  ].join('\n')
}

export function categoryHelpText(category: string, ports: RuntimePorts): string {
  return [
    '',
    `${text(ports, 'help.categoryUsage.title', 'Category usage:')}`,
    `  luna ${category} <command> [key=value ...]`,
    `  luna ${category} <command> --help`,
    '',
    `${text(ports, 'help.categoryUsage.catalog', 'Machine-readable discovery:')}`,
    `  luna help catalog category=${category} output=json interactive=false`,
  ].join('\n')
}

export function commandHelpText(
  metadata: NormalizedCommandMetadata,
  ports: RuntimePorts,
): string {
  const parameterLines = metadata.parameters.length > 0
    ? metadata.parameters.map(parameter => parameterLine(parameter, ports))
    : [`  ${text(ports, 'help.parameters.none', 'No business parameters.')}`]
  const examples = commandExamples(metadata, ports)
  const details = [
    `${text(ports, 'help.details.command', 'Command')}: ${metadata.canonicalPath}`,
    `${text(ports, 'help.details.risk', 'Risk')}: ${localizedValue(ports, 'risk', metadata.risk)}`,
    `${text(ports, 'help.details.project', 'Project selection')}: ${localizedValue(ports, 'projectContext', metadata.projectContext)}`,
    `${text(ports, 'help.details.transport', 'Transport')}: ${localizedValue(ports, 'transport', metadata.transport)}`,
  ]
  if (metadata.method && metadata.path)
    details.push(`${text(ports, 'help.details.endpoint', 'Endpoint')}: ${metadata.method} ${metadata.path}`)
  if (metadata.scopes.length > 0)
    details.push(`${text(ports, 'help.details.scopes', 'Required scopes')}: ${metadata.scopes.join(', ')}`)

  return [
    '',
    `${text(ports, 'help.details.title', 'Command details:')}`,
    ...details.map(line => `  ${line}`),
    '',
    `${text(ports, 'help.parameters.title', 'Parameters:')}`,
    ...parameterLines,
    '',
    `${text(ports, 'help.examples.title', 'Examples:')}`,
    ...examples.map(example => `  ${example}`),
    '',
    `${text(ports, 'help.machineContract', 'Complete machine-readable contract:')}`,
    `  luna help command path=${metadata.canonicalPath} output=json interactive=false`,
  ].join('\n')
}

function parameterLine(parameter: CommandParameter, ports: RuntimePorts): string {
  const requirement = parameter.required
    ? text(ports, 'help.parameters.required', 'required')
    : text(ports, 'help.parameters.optional', 'optional')
  const type = schemaType(parameter)
  const sources = parameter.valueSources?.join('|') ?? 'inline'
  const attributes = [
    requirement,
    type,
    `${text(ports, 'help.parameters.sources', 'sources')}: ${sources}`,
  ]
  if (parameter.repeated)
    attributes.push(text(ports, 'help.parameters.repeated', 'repeatable'))
  if (parameter.sensitive)
    attributes.push(text(ports, 'help.parameters.sensitive', 'sensitive'))
  const description = text(
    ports,
    parameter.descriptionKey ?? `parameters.${parameter.name}`,
    parameter.description ?? '',
  )
  return `  ${parameter.name}=<value>  [${attributes.join(', ')}]${description ? `  ${description}` : ''}`
}

function commandExamples(
  metadata: NormalizedCommandMetadata,
  ports: RuntimePorts,
): readonly string[] {
  if (metadata.examples && metadata.examples.length > 0)
    return metadata.examples.map(ensureLunaPrefix)

  const required = metadata.parameters
    .filter(parameter => parameter.required)
    .map(parameter => `${parameter.name}=${sampleValue(parameter, metadata)}`)
  const invocation = ['luna', metadata.category, metadata.tool, ...required].join(' ')
  return [
    invocation,
    `${invocation} output=json interactive=false`,
    text(
      ports,
      'help.examples.fileHint',
      '# Replace large JSON or multiline values with parameter=@file.json.',
    ),
  ]
}

function schemaType(parameter: CommandParameter): string {
  const type = parameter.schema?.type
  if (Array.isArray(type))
    return type.join('|')
  if (typeof type === 'string')
    return type
  return 'string'
}

function sampleValue(
  parameter: CommandParameter,
  metadata: NormalizedCommandMetadata,
): string {
  if (parameter.sensitive || parameter.valueSources?.includes('stdin'))
    return '@-'
  const enumValues = parameter.schema?.enum
  if (Array.isArray(enumValues) && enumValues.length > 0)
    return String(enumValues[0])
  const type = parameter.schema?.type
  if (type === 'boolean')
    return 'true'
  if (type === 'integer' || type === 'number')
    return '1'
  if (parameter.location === 'body')
    return '@request.json'
  const lower = parameter.name.toLocaleLowerCase()
  if (metadata.canonicalPath === 'help.command' && lower === 'path')
    return 'project.get-projects'
  if (lower.includes('project'))
    return 'prj_example'
  if (lower === 'path')
    return '/api/v1/example'
  if (lower.endsWith('id'))
    return 'example-id'
  return 'value'
}

function ensureLunaPrefix(example: string): string {
  const trimmed = example.trim()
  if (
    trimmed.startsWith('luna ')
    || trimmed.startsWith('#')
    || /\|\s*luna\s/.test(trimmed)
  ) {
    return trimmed
  }
  return `luna ${trimmed}`
}

function text(ports: RuntimePorts, key: string, fallback: string): string {
  return ports.translate?.(key, fallback) ?? fallback
}

function localizedValue(
  ports: RuntimePorts,
  group: string,
  value: string,
): string {
  return text(ports, `help.values.${group}.${value}`, value)
}

function format(
  template: string,
  values: Readonly<Record<string, string>>,
): string {
  return Object.entries(values).reduce(
    (value, [key, replacement]) => value.replaceAll(`{{${key}}}`, replacement),
    template,
  )
}
