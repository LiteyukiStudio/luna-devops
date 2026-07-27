export type CommandSource = 'local' | 'openapi' | 'protocol'
export type CommandRisk = 'low' | 'medium' | 'high' | 'critical'
export type CommandTransport
  = | 'local'
    | 'http'
    | 'sse'
    | 'websocket'
    | 'download'
    | 'upload'
export type OutputFormat = 'table' | 'json' | 'raw-json' | 'yaml' | 'jsonl' | 'name'
export type DryRunMode = 'client' | 'server'

export interface JsonSchema {
  readonly [key: string]: unknown
}

export interface CommandParameter {
  readonly name: string
  readonly location?: 'query' | 'header' | 'path' | 'cookie' | 'body'
  readonly description?: string
  readonly descriptionKey?: string
  readonly required?: boolean
  readonly repeated?: boolean
  readonly sensitive?: boolean
  readonly valueSources?: readonly ('inline' | 'file' | 'stdin')[]
  readonly schema?: JsonSchema
}

export interface CommandMetadata {
  readonly category: string
  readonly tool: string
  readonly canonicalPath?: string
  readonly categoryAliases?: readonly string[]
  readonly aliases?: readonly string[]
  readonly source: CommandSource
  readonly operationId?: string
  readonly method?: string
  readonly path?: string
  readonly consumedOperations?: readonly string[]
  readonly summary?: string
  readonly summaryKey?: string
  readonly description?: string
  readonly descriptionKey?: string
  readonly parameters?: readonly CommandParameter[]
  readonly inputSchema?: JsonSchema
  readonly outputSchema?: JsonSchema
  readonly errorSchema?: JsonSchema
  readonly schemaVersion?: string
  readonly schemaDigest?: string
  readonly scopes?: readonly string[]
  readonly mfaPurpose?: string
  readonly risk?: CommandRisk
  readonly transport?: CommandTransport
  readonly projectContext?: 'required' | 'optional' | 'none'
  readonly streaming?: boolean
  readonly hidden?: boolean
  readonly agentAllowed?: boolean
  readonly examples?: readonly string[]
}

export interface NormalizedCommandMetadata extends CommandMetadata {
  readonly canonicalPath: string
  readonly source: CommandSource
  readonly risk: CommandRisk
  readonly transport: CommandTransport
  readonly projectContext: 'required' | 'optional' | 'none'
  readonly agentAllowed: boolean
  readonly parameters: readonly CommandParameter[]
  readonly aliases: readonly string[]
  readonly categoryAliases: readonly string[]
  readonly scopes: readonly string[]
}

export interface CommandCatalogMetadata {
  readonly catalogVersion: string
  readonly openapiDigest: string
  readonly schemaDigest: string
}

export interface CommandCatalogEntry extends CommandMetadata {
  readonly method?: string
  readonly path?: string
}

export interface CommandExecutionGlobals {
  readonly server?: string
  readonly project?: string
  readonly output: OutputFormat
  readonly lang?: string
  readonly color: boolean
  readonly interactive: boolean
  readonly yes: boolean
  readonly quiet: boolean
  readonly agent: boolean
  readonly dryRun?: DryRunMode
  readonly timeoutMs: number
  readonly debug: boolean
  readonly requestId?: string
  readonly idempotencyKey?: string
  readonly insecureSkipTlsVerify: boolean
}

export interface CommandInvocation {
  readonly metadata: NormalizedCommandMetadata
  readonly params: Readonly<Record<string, unknown>>
  readonly globals: CommandExecutionGlobals
  readonly explicitGlobalKeys: ReadonlySet<string>
  readonly canonicalGlobalValues: Readonly<Record<string, string>>
}

export interface CommandResult {
  readonly data: unknown
  readonly schemaVersion?: string
  readonly meta?: Readonly<Record<string, unknown>>
}

export interface ProjectContextSnapshot {
  readonly id: string
  readonly name?: string
  readonly identifier?: string
  readonly [key: string]: unknown
}

export interface LunaConfigDocument {
  readonly version: number
  readonly server: string
  readonly credential?: LunaCredentialRecord | null
  readonly project?: ProjectContextSnapshot | null
  readonly language?: string
  readonly output?: OutputFormat | ''
}

export interface LunaCredentialRecord {
  readonly type?: string
  readonly token?: string
  readonly accessToken?: string
  readonly [key: string]: unknown
}

export interface ConfigPort {
  read: () => Promise<LunaConfigDocument>
  write: (config: LunaConfigDocument) => Promise<void>
  path?: string
}

export interface InputPort {
  parse: (
    tokens: readonly string[],
    metadata: NormalizedCommandMetadata,
  ) => Promise<Readonly<Record<string, unknown>>>
  confirm?: (message: string) => Promise<boolean>
}

export interface OutputPort {
  writeSuccess: (
    metadata: NormalizedCommandMetadata,
    result: CommandResult,
    globals: CommandExecutionGlobals,
  ) => Promise<void> | void
  writeError: (error: unknown, globals?: Partial<CommandExecutionGlobals>) => Promise<void> | void
  writeInfo?: (
    message: string,
    globals?: Partial<CommandExecutionGlobals>,
  ) => Promise<void> | void
}

export interface ApiExecutionRequest {
  readonly operationId: string
  readonly params: Readonly<Record<string, unknown>>
  readonly globals: CommandExecutionGlobals
  readonly metadata: NormalizedCommandMetadata
}

export interface ApiDiagnosticRequest {
  readonly method: string
  readonly path: string
  readonly params: Readonly<Record<string, unknown>>
  readonly globals: CommandExecutionGlobals
}

export interface LunaApiMeta {
  readonly apiVersion: string
  readonly serverVersion: string
  readonly openapiDigest: string
  readonly minimumCliVersion: string
  readonly features: Readonly<Record<string, boolean>>
}

export interface ApiPort {
  execute: (request: ApiExecutionRequest) => Promise<CommandResult | unknown>
  request: (request: ApiDiagnosticRequest) => Promise<CommandResult | unknown>
  validateAccessToken?: (
    server: string,
    token: string,
    globals: CommandExecutionGlobals,
  ) => Promise<Readonly<Record<string, unknown>>>
  resolveProject?: (
    value: string,
    globals: CommandExecutionGlobals,
  ) => Promise<ProjectContextSnapshot>
  getMeta?: (
    server: string | undefined,
    globals: CommandExecutionGlobals,
  ) => Promise<LunaApiMeta>
  beginOAuthLogin?: (
    request: import('../auth/oauth.js').OAuthLoginRequest,
  ) => Promise<import('../auth/oauth.js').OAuthLoginResult>
  refreshOAuthCredential?: (
    request: import('../auth/oauth.js').OAuthRefreshRequest,
  ) => Promise<import('../auth/oauth.js').OAuthTokenCredential>
  revokeOAuthCredential?: (
    request: import('../auth/oauth.js').OAuthRevokeRequest,
  ) => Promise<void>
}

export interface ProtocolPort {
  readonly fetch?: typeof globalThis.fetch
  readonly createWebSocket?: (url: string) => ProtocolWebSocket
  readonly stdin?: ProtocolInputStream
  readonly stdout?: ProtocolOutputStream
  readonly onInterrupt?: (listener: () => void) => () => void
}

export interface ProtocolInputStream {
  readonly isTTY?: boolean
  readonly isRaw?: boolean
  setRawMode?: (enabled: boolean) => void
  resume?: () => void
  pause?: () => void
  on: (event: string, listener: (...args: unknown[]) => void) => unknown
  off?: (event: string, listener: (...args: unknown[]) => void) => unknown
}

export interface ProtocolOutputStream {
  readonly isTTY?: boolean
  readonly columns?: number
  readonly rows?: number
  write: (chunk: string | Uint8Array) => boolean
  on?: (event: string, listener: (...args: unknown[]) => void) => unknown
  off?: (event: string, listener: (...args: unknown[]) => void) => unknown
}

export interface ProtocolWebSocketEvent {
  readonly data?: unknown
  readonly code?: number
  readonly reason?: string
}

export interface ProtocolWebSocket {
  readonly readyState: number
  binaryType: string
  send: (data: string | ArrayBuffer | ArrayBufferView) => void
  close: (code?: number, reason?: string) => void
  addEventListener: (
    event: string,
    listener: (event: ProtocolWebSocketEvent) => void,
  ) => void
  removeEventListener?: (
    event: string,
    listener: (event: ProtocolWebSocketEvent) => void,
  ) => void
}

export interface RuntimePorts {
  readonly config: ConfigPort
  readonly input: InputPort
  readonly output: OutputPort
  readonly api: ApiPort
  readonly protocol?: ProtocolPort
  readonly env?: Readonly<Record<string, string | undefined>>
  readonly isTTY?: boolean
  readonly version?: string
  readonly distribution?: 'npm' | 'binary' | 'source'
  readonly translate?: (key: string, fallback: string, locale?: string) => string
}

export type CommandHandler = (
  invocation: CommandInvocation,
  ports: RuntimePorts,
) => Promise<CommandResult | unknown>

export interface RegisteredCommand {
  readonly metadata: NormalizedCommandMetadata
  readonly handler: CommandHandler
}
