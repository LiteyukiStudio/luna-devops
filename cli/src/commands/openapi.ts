import type {
  CommandCatalogEntry,
  CommandCatalogMetadata,
  CommandParameter,
  CommandResult,
  JsonSchema,
} from './types.js'
import { CliCommandError } from './errors.js'
import { CommandRegistry } from './registry.js'

interface CatalogLike {
  readonly metadata?: Partial<CommandCatalogMetadata>
  readonly catalogVersion?: string
  readonly openapiDigest?: string
  readonly schemaDigest?: string
  readonly commands?: readonly unknown[]
  readonly entries?: readonly unknown[]
}

export function createRegistryFromContract(contractModule: unknown): CommandRegistry {
  const catalog = extractCatalog(contractModule)
  const registry = new CommandRegistry(catalog.metadata)
  registerOpenApiCommands(registry, catalog.entries)
  return registry
}

export function extractCatalog(contractModule: unknown): {
  metadata: Partial<CommandCatalogMetadata>
  entries: readonly CommandCatalogEntry[]
} {
  const moduleRecord = asRecord(contractModule)
  const candidate
    = moduleRecord.OPERATION_CATALOG
      ?? moduleRecord.commandCatalog
      ?? moduleRecord.COMMAND_CATALOG
      ?? moduleRecord.cliCommandCatalog
      ?? moduleRecord.default
  const value = typeof candidate === 'function' ? candidate() : candidate
  const catalog = asRecord(value) as CatalogLike
  const exportedMetadata = asRecord(moduleRecord.OPERATION_CATALOG_METADATA)
  const entries = Array.isArray(value)
    ? value
    : Array.isArray(catalog.commands)
      ? catalog.commands
      : Array.isArray(catalog.entries)
        ? catalog.entries
        : []

  return {
    metadata: {
      catalogVersion:
        stringValue(exportedMetadata.catalogVersion)
        ?? stringValue(catalog.metadata?.catalogVersion)
        ?? stringValue(catalog.catalogVersion),
      openapiDigest:
        stringValue(exportedMetadata.openapiDigest)
        ?? stringValue(catalog.metadata?.openapiDigest)
        ?? stringValue(catalog.openapiDigest),
      schemaDigest:
        stringValue(exportedMetadata.catalogDigest)
        ?? stringValue(exportedMetadata.schemaDigest)
        ?? stringValue(catalog.metadata?.schemaDigest)
        ?? stringValue(catalog.schemaDigest),
    },
    entries: entries.map(normalizeCatalogEntry),
  }
}

export function registerOpenApiCommands(
  registry: CommandRegistry,
  entries: readonly CommandCatalogEntry[],
): void {
  for (const entry of entries) {
    if (entry.source !== 'openapi' || entry.hidden)
      continue
    registry.register(entry, async (invocation, ports) => {
      if (!entry.operationId) {
        throw new CliCommandError(
          'missing_operation_id',
          `Command "${invocation.metadata.canonicalPath}" has no operationId.`,
          { status: 500 },
        )
      }
      const result = await ports.api.execute({
        operationId: entry.operationId,
        params: invocation.params,
        globals: invocation.globals,
        metadata: invocation.metadata,
      })
      return asCommandResult(result, invocation.metadata.schemaVersion)
    })
  }
}

function normalizeCatalogEntry(value: unknown): CommandCatalogEntry {
  const entry = asRecord(value)
  const command = asRecord(entry.command)
  const extension = asRecord(entry.xLunaCli ?? entry['x-luna-cli'] ?? entry.cli)
  const category = requiredString(entry.category ?? command.category ?? extension.category, 'category')
  const tool = requiredString(entry.tool ?? command.tool ?? extension.tool, 'tool')
  const parameters = parameterArray(entry.parameters)
  const requestBody = asRecord(entry.requestBody)
  if (Object.keys(requestBody).length > 0) {
    parameters.push({
      name: 'body',
      location: 'body',
      description: 'OpenAPI request body.',
      required: booleanValue(requestBody.required),
      valueSources: ['file', 'stdin'],
      schema: {
        type: ['object', 'array', 'string', 'null'],
        contentTypes: stringArray(requestBody.contentTypes),
        schemaRefs: stringArray(requestBody.schemaRefs),
      },
    })
  }

  return {
    category,
    tool,
    canonicalPath: stringValue(entry.canonicalPath ?? command.canonicalPath),
    categoryAliases: stringArray(
      entry.categoryAliases ?? command.categoryAliases ?? extension.categoryAliases,
    ),
    aliases: stringArray(entry.aliases ?? command.aliases ?? extension.aliases),
    source: 'openapi',
    operationId: stringValue(entry.operationId),
    consumedOperations: stringArray(entry.consumedOperations),
    summary: stringValue(entry.summary),
    summaryKey: stringValue(entry.summaryKey),
    description: stringValue(entry.description),
    descriptionKey: stringValue(entry.descriptionKey),
    parameters,
    inputSchema: schemaValue(entry.inputSchema),
    outputSchema: schemaValue(entry.outputSchema),
    errorSchema: schemaValue(entry.errorSchema),
    schemaVersion: stringValue(entry.schemaVersion),
    schemaDigest: stringValue(entry.schemaDigest),
    scopes: stringArray(entry.scopes ?? command.requiredScopes ?? extension.scopes),
    mfaPurpose: stringValue(
      entry.mfaPurpose ?? command.mfaPurpose ?? extension.mfaPurpose,
    ),
    risk: riskValue(entry.risk ?? command.risk ?? extension.risk),
    transport: transportValue(entry.transport ?? command.transport ?? extension.transport),
    projectContext: projectContextValue(
      entry.projectContext
      ?? command.projectContext
      ?? extension.projectContext
      ?? asRecord(extension.projectContext).mode,
    ) ?? inferProjectContext(parameters),
    streaming: booleanValue(
      entry.streaming ?? command.streaming ?? extension.streaming,
    ),
    hidden: booleanValue(entry.hidden ?? command.hidden),
    agentAllowed:
      typeof (entry.agentAllowed ?? command.agentAllowed ?? extension.agentAllowed) === 'boolean'
        ? booleanValue(entry.agentAllowed ?? command.agentAllowed ?? extension.agentAllowed)
        : undefined,
    examples: stringArray(entry.examples ?? command.examples ?? extension.examples),
    method: stringValue(entry.method),
    path: stringValue(entry.path),
  }
}

function parameterArray(value: unknown): CommandParameter[] {
  if (!Array.isArray(value))
    return []
  return value.map((item) => {
    const parameter = asRecord(item)
    return {
      name: requiredString(parameter.name, 'parameter name'),
      location: parameterLocation(parameter.in ?? parameter.location),
      description: stringValue(parameter.description),
      descriptionKey: stringValue(parameter.descriptionKey),
      required: booleanValue(parameter.required),
      repeated: booleanValue(parameter.repeated),
      sensitive: booleanValue(parameter.sensitive),
      valueSources: valueSourceArray(parameter.valueSources),
      schema: schemaValue(parameter.schema),
    }
  })
}

function valueSourceArray(
  value: unknown,
): readonly ('inline' | 'file' | 'stdin')[] | undefined {
  if (!Array.isArray(value))
    return undefined
  return value.filter(
    (item): item is 'inline' | 'file' | 'stdin' =>
      item === 'inline' || item === 'file' || item === 'stdin',
  )
}

function asCommandResult(value: unknown, schemaVersion?: string): CommandResult {
  const record = asRecord(value)
  if ('data' in record && ('schemaVersion' in record || 'meta' in record)) {
    return value as CommandResult
  }
  return { data: value, schemaVersion }
}

function riskValue(value: unknown): 'low' | 'medium' | 'high' | 'critical' | undefined {
  return value === 'low' || value === 'medium' || value === 'high' || value === 'critical'
    ? value
    : undefined
}

function transportValue(
  value: unknown,
): 'local' | 'http' | 'sse' | 'websocket' | 'download' | 'upload' | undefined {
  return value === 'local'
    || value === 'http'
    || value === 'sse'
    || value === 'websocket'
    || value === 'download'
    || value === 'upload'
    ? value
    : undefined
}

function inferProjectContext(
  parameters: readonly CommandParameter[],
): 'required' | 'optional' | 'none' {
  const projectParameter = parameters.find(parameter =>
    parameter.name === 'project'
    || parameter.name === 'projectId'
    || parameter.name === 'projectID',
  )
  if (!projectParameter)
    return 'none'
  return projectParameter.required ? 'required' : 'optional'
}

function parameterLocation(
  value: unknown,
): 'query' | 'header' | 'path' | 'cookie' | 'body' | undefined {
  return value === 'query'
    || value === 'header'
    || value === 'path'
    || value === 'cookie'
    || value === 'body'
    ? value
    : undefined
}

function projectContextValue(
  value: unknown,
): 'required' | 'optional' | 'none' | undefined {
  return value === 'required' || value === 'optional' || value === 'none'
    ? value
    : undefined
}

function schemaValue(value: unknown): JsonSchema | undefined {
  return typeof value === 'object' && value !== null
    ? value as JsonSchema
    : undefined
}

function requiredString(value: unknown, label: string): string {
  const result = stringValue(value)
  if (!result) {
    throw new CliCommandError('invalid_command_catalog', `Missing ${label}.`, {
      status: 500,
    })
  }
  return result
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 ? value : undefined
}

function booleanValue(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
}

function stringArray(value: unknown): readonly string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : []
}

function asRecord(value: unknown): Record<string, unknown> {
  return typeof value === 'object' && value !== null
    ? value as Record<string, unknown>
    : {}
}
