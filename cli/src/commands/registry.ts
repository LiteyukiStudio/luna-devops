import type {
  CommandCatalogMetadata,
  CommandHandler,
  CommandMetadata,
  NormalizedCommandMetadata,
  RegisteredCommand,
} from './types.js'
import { CliCommandError } from './errors.js'
import {
  prepareProtocolRegistration,
  protocolCommandDefinitions,
} from './protocol.js'

const NAME_PATTERN = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/

export class CommandRegistry {
  readonly catalogMetadata: CommandCatalogMetadata
  readonly #commands = new Map<string, RegisteredCommand>()
  readonly #aliases = new Map<string, string>()

  constructor(metadata: Partial<CommandCatalogMetadata> = {}) {
    this.catalogMetadata = {
      catalogVersion: metadata.catalogVersion ?? 'cli.luna.devops/catalog/v1',
      openapiDigest: metadata.openapiDigest ?? 'unavailable',
      schemaDigest: metadata.schemaDigest ?? 'unavailable',
    }
    for (const definition of protocolCommandDefinitions())
      this.register(definition.metadata, definition.handler)
  }

  register(metadata: CommandMetadata, handler: CommandHandler): RegisteredCommand {
    const prepared = prepareProtocolRegistration(metadata, handler)
    const normalized = normalizeMetadata(prepared.metadata)
    const path = normalized.canonicalPath
    if (this.#commands.has(path)) {
      throw new CliCommandError(
        'duplicate_command',
        `Command "${path}" is registered more than once.`,
        { status: 409, details: { path } },
      )
    }
    // A real command always wins over a previously registered convenience
    // alias. OpenAPI semantic aliases may intentionally overlap a hand-written
    // local command such as auth.login.
    this.#aliases.delete(path)

    validateSource(normalized)
    const registered = { metadata: normalized, handler: prepared.handler }
    this.#commands.set(path, registered)

    for (const alias of normalized.aliases) {
      const aliasPath = `${normalized.category}.${alias}`
      this.#registerAlias(aliasPath, path)
    }
    for (const categoryAlias of normalized.categoryAliases) {
      this.#registerAlias(`${categoryAlias}.${normalized.tool}`, path)
      for (const alias of normalized.aliases) {
        this.#registerAlias(`${categoryAlias}.${alias}`, path)
      }
    }

    return registered
  }

  get(path: string, allowAlias = false): RegisteredCommand | undefined {
    const normalizedPath = normalizePath(path)
    const canonicalPath = allowAlias
      ? (this.#aliases.get(normalizedPath) ?? normalizedPath)
      : normalizedPath
    return this.#commands.get(canonicalPath)
  }

  require(path: string, allowAlias = false): RegisteredCommand {
    const command = this.get(path, allowAlias)
    if (!command) {
      throw new CliCommandError('unknown_command', `Unknown command "${path}".`, {
        status: 400,
        exitCode: 2,
        details: { path },
      })
    }
    return command
  }

  list(options: {
    query?: string
    category?: string
    risk?: string
    scope?: string
    transport?: string
    includeHidden?: boolean
  } = {}): readonly RegisteredCommand[] {
    const query = options.query?.trim().toLocaleLowerCase()
    return [...this.#commands.values()]
      .filter(({ metadata }) => options.includeHidden || !metadata.hidden)
      .filter(({ metadata }) => !options.category || metadata.category === options.category)
      .filter(({ metadata }) => !options.risk || metadata.risk === options.risk)
      .filter(({ metadata }) => !options.scope || metadata.scopes.includes(options.scope))
      .filter(({ metadata }) => !options.transport || metadata.transport === options.transport)
      .filter(({ metadata }) => {
        if (!query)
          return true
        return [
          metadata.canonicalPath,
          metadata.summary,
          metadata.description,
          metadata.operationId,
          ...metadata.scopes,
        ].some(value => value?.toLocaleLowerCase().includes(query))
      })
      .sort((left, right) =>
        left.metadata.canonicalPath.localeCompare(right.metadata.canonicalPath),
      )
  }

  categories(): readonly string[] {
    return [...new Set(this.list({ includeHidden: true }).map(item => item.metadata.category))]
      .sort()
  }

  categoryAliases(category: string): readonly string[] {
    return [
      ...new Set(
        this.list({ includeHidden: true })
          .filter(item => item.metadata.category === category)
          .flatMap(item => item.metadata.categoryAliases),
      ),
    ].sort()
  }

  #registerAlias(aliasPath: string, canonicalPath: string): void {
    const existing = this.#aliases.get(aliasPath)
    if (existing && existing !== canonicalPath) {
      throw new CliCommandError(
        'duplicate_command_alias',
        `Command alias "${aliasPath}" is ambiguous.`,
        { status: 409, details: { aliasPath, commands: [existing, canonicalPath] } },
      )
    }
    if (this.#commands.has(aliasPath) && aliasPath !== canonicalPath) {
      return
    }
    this.#aliases.set(aliasPath, canonicalPath)
  }
}

export function normalizeMetadata(metadata: CommandMetadata): NormalizedCommandMetadata {
  validateName(metadata.category, 'category')
  validateName(metadata.tool, 'tool')
  const canonicalPath = `${metadata.category}.${metadata.tool}`
  if (metadata.canonicalPath && normalizePath(metadata.canonicalPath) !== canonicalPath) {
    throw new CliCommandError(
      'invalid_command_path',
      `Canonical path must be "${canonicalPath}".`,
      { status: 400, details: { canonicalPath: metadata.canonicalPath } },
    )
  }

  for (const alias of metadata.aliases ?? []) validateName(alias, 'tool alias')
  for (const alias of metadata.categoryAliases ?? []) validateName(alias, 'category alias')

  return Object.freeze({
    ...metadata,
    canonicalPath,
    aliases: Object.freeze([...(metadata.aliases ?? [])]),
    categoryAliases: Object.freeze([...(metadata.categoryAliases ?? [])]),
    parameters: Object.freeze([...(metadata.parameters ?? [])]),
    scopes: Object.freeze([...(metadata.scopes ?? [])]),
    risk: metadata.risk ?? 'low',
    transport: metadata.transport ?? (metadata.source === 'local' ? 'local' : 'http'),
    projectContext: metadata.projectContext ?? 'none',
    agentAllowed: metadata.agentAllowed ?? true,
  })
}

export function normalizePath(path: string): string {
  const parts = path.trim().split('.')
  if (parts.length !== 2) {
    throw new CliCommandError(
      'invalid_command_path',
      'Commands must use the fixed <category>.<tool> path.',
      { status: 400, exitCode: 2, details: { path } },
    )
  }
  validateName(parts[0]!, 'category')
  validateName(parts[1]!, 'tool')
  return `${parts[0]}.${parts[1]}`
}

function validateName(value: string, label: string): void {
  if (!NAME_PATTERN.test(value)) {
    throw new CliCommandError(
      'invalid_command_name',
      `Invalid ${label} "${value}".`,
      { status: 400, exitCode: 2, details: { label, value } },
    )
  }
}

function validateSource(metadata: NormalizedCommandMetadata): void {
  if (metadata.source === 'openapi' && !metadata.operationId) {
    throw new CliCommandError(
      'missing_operation_id',
      `OpenAPI command "${metadata.canonicalPath}" must declare operationId.`,
      { status: 400, details: { path: metadata.canonicalPath } },
    )
  }
  if (metadata.source === 'local' && metadata.operationId) {
    throw new CliCommandError(
      'invalid_local_operation',
      `Local command "${metadata.canonicalPath}" cannot declare operationId.`,
      { status: 400, details: { path: metadata.canonicalPath } },
    )
  }
  if (metadata.source === 'protocol' && !(metadata.consumedOperations?.length)) {
    throw new CliCommandError(
      'missing_protocol_operations',
      `Protocol command "${metadata.canonicalPath}" must declare consumed operations.`,
      { status: 400, details: { path: metadata.canonicalPath } },
    )
  }
}
