import type { HttpMethod, QueryInput, QueryPrimitive, QueryValue } from '@luna-devops/api-client'
import type {
  OAuthClient,
  OAuthLoginRequest,
  OAuthLoginResult,
  OAuthRefreshRequest,
  OAuthRevokeRequest,
  OAuthTokenCredential,
} from '../auth/oauth.js'
import type {
  ServerCompatibilityRequirements,
} from './compatibility.js'
import type {
  ApiDiagnosticRequest,
  ApiExecutionRequest,
  ApiPort,
  CommandExecutionGlobals,
  CommandResult,
  ConfigPort,
  LunaApiMeta,
  NormalizedCommandMetadata,
  ProjectContextSnapshot,
} from './types.js'
import process from 'node:process'
import {
  LunaClient,
} from '@luna-devops/api-client'
import {
  beginOAuthLogin,
  refreshOAuthCredential,
  revokeOAuthCredential,
  storeValidatedOAuthCredential,
} from '../auth/index.js'
import { resolveRuntimeContext } from '../config/index.js'
import {
  assertServerCompatibility,
} from './compatibility.js'
import { CliCommandError, toCliCommandError } from './errors.js'

const HTTP_METHODS = new Set<HttpMethod>([
  'DELETE',
  'GET',
  'HEAD',
  'OPTIONS',
  'PATCH',
  'POST',
  'PUT',
])
const QUERY_METHODS = new Set<HttpMethod>(['DELETE', 'GET', 'HEAD', 'OPTIONS'])
const PROJECT_PARAMETER_NAMES = new Set(['project', 'projectId', 'projectID'])
const OAUTH_REFRESH_SKEW_MS = 30_000

const defaultOAuthClient: OAuthClient = {
  beginOAuthLogin,
  refreshOAuthCredential,
  revokeOAuthCredential,
}

export interface ApiAdapterOptions {
  readonly config: ConfigPort
  readonly env?: Readonly<Record<string, string | undefined>>
  readonly clientFactory?: (options: ConstructorParameters<typeof LunaClient>[0]) => LunaClient
  readonly compatibility?: ServerCompatibilityRequirements
  readonly oauthClient?: OAuthClient
  readonly now?: () => number
}

export interface PlannedApiRequest {
  readonly method: HttpMethod
  readonly path: string
  readonly query?: QueryInput
  readonly headers?: Readonly<Record<string, string>>
  readonly body?: unknown
}

export class LunaApiAdapter implements ApiPort {
  readonly #config: ConfigPort
  readonly #env: Readonly<Record<string, string | undefined>>
  readonly #clientFactory: (options: ConstructorParameters<typeof LunaClient>[0]) => LunaClient
  readonly #compatibility?: ServerCompatibilityRequirements
  readonly #oauthClient: OAuthClient
  readonly #now: () => number
  readonly #compatibleServers = new Set<string>()

  constructor(options: ApiAdapterOptions) {
    this.#config = options.config
    this.#env = options.env ?? process.env
    this.#clientFactory = options.clientFactory ?? (clientOptions => new LunaClient(clientOptions))
    this.#compatibility = options.compatibility
    this.#oauthClient = options.oauthClient ?? defaultOAuthClient
    this.#now = options.now ?? Date.now
  }

  beginOAuthLogin(request: OAuthLoginRequest): Promise<OAuthLoginResult> {
    return this.#oauthClient.beginOAuthLogin(request)
  }

  refreshOAuthCredential(request: OAuthRefreshRequest): Promise<OAuthTokenCredential> {
    return this.#oauthClient.refreshOAuthCredential(request)
  }

  revokeOAuthCredential(request: OAuthRevokeRequest): Promise<void> {
    return this.#oauthClient.revokeOAuthCredential(request)
  }

  async execute(request: ApiExecutionRequest): Promise<CommandResult> {
    if (!request.metadata.path || !request.metadata.method) {
      throw new CliCommandError(
        'command_transport_invalid',
        `Command "${request.metadata.canonicalPath}" has no HTTP method or path.`,
        { status: 500, details: { command: request.metadata.canonicalPath } },
      )
    }
    assertSupportedTransport(request.metadata)
    const planned = planOpenApiRequest(request)
    if (request.globals.dryRun === 'client') {
      return {
        schemaVersion: request.metadata.schemaVersion ?? 'dry-run/v1',
        data: {
          dryRun: 'client',
          operationId: request.operationId,
          request: planned,
        },
      }
    }
    await this.#ensureServerCompatibility(request.globals)
    const result = await this.#send(planned, request.globals)
    const projectId = requestProjectId(request)
    return {
      schemaVersion: request.metadata.schemaVersion,
      data: result.data,
      meta: {
        requestId: result.requestId,
        status: result.status,
        ...(projectId ? { projectId } : {}),
      },
    }
  }

  async request(request: ApiDiagnosticRequest): Promise<CommandResult> {
    const method = normalizeMethod(request.method)
    const { body, ...remaining } = request.params
    const planned: PlannedApiRequest = {
      method,
      path: validateDiagnosticPath(request.path),
      ...(body !== undefined ? { body } : {}),
      ...(Object.keys(remaining).length > 0
        ? QUERY_METHODS.has(method)
          ? { query: toQueryInput(remaining) }
          : { body: mergeDiagnosticBody(body, remaining) }
        : {}),
    }
    if (request.globals.dryRun === 'client') {
      return {
        schemaVersion: 'api.request/v1',
        data: { dryRun: 'client', request: planned },
      }
    }
    const result = await this.#send(planned, request.globals)
    return {
      schemaVersion: 'api.request/v1',
      data: result.data,
      meta: {
        requestId: result.requestId,
        status: result.status,
      },
    }
  }

  async validateAccessToken(
    server: string,
    token: string,
    globals: CommandExecutionGlobals,
  ): Promise<Readonly<Record<string, unknown>>> {
    const client = this.#clientFactory({
      baseUrl: server,
      timeoutMs: globals.timeoutMs,
      tokenProvider: { getAccessToken: () => token },
    })
    const result = await client.request<unknown>({
      method: 'GET',
      path: '/api/v1/users/me',
      requestId: globals.requestId,
      timeoutMs: globals.timeoutMs,
    })
    if (!result.ok)
      throw apiFailure(result.error)
    return asRecord(result.data)
  }

  async getMeta(
    server: string | undefined,
    globals: CommandExecutionGlobals,
  ): Promise<LunaApiMeta> {
    const baseUrl = server ?? await this.#configuredServer(globals)
    const client = this.#clientFactory({
      baseUrl,
      timeoutMs: globals.timeoutMs,
    })
    const result = await client.request<unknown>({
      method: 'GET',
      path: '/api/v1/meta',
      requestId: globals.requestId,
      timeoutMs: globals.timeoutMs,
    })
    if (!result.ok)
      throw apiFailure(result.error)

    const data = asRecord(result.data)
    const features = asRecord(data.features)
    return {
      apiVersion: requiredMetaString(data.apiVersion, 'apiVersion'),
      serverVersion: requiredMetaString(data.serverVersion, 'serverVersion'),
      openapiDigest: requiredMetaString(data.openapiDigest, 'openapiDigest'),
      minimumCliVersion: requiredMetaString(data.minimumCliVersion, 'minimumCliVersion'),
      features: Object.fromEntries(
        Object.entries(features)
          .filter((entry): entry is [string, boolean] => typeof entry[1] === 'boolean'),
      ),
    }
  }

  async resolveProject(
    value: string,
    globals: CommandExecutionGlobals,
  ): Promise<ProjectContextSnapshot> {
    const client = await this.#client(globals)
    const result = await client.request<unknown>({
      method: 'GET',
      path: '/api/v1/projects',
      query: {
        page: 1,
        pageSize: 100,
        query: value,
      },
      requestId: globals.requestId,
      timeoutMs: globals.timeoutMs,
    })
    if (!result.ok)
      throw apiFailure(result.error)

    const candidates = listItems(result.data)
      .filter(project => [project.id, project.identifier, project.slug, project.name]
        .includes(value))
    if (candidates.length === 0) {
      throw new CliCommandError('project_not_found', `Project "${value}" was not found.`, {
        status: 404,
        details: { value },
      })
    }
    if (candidates.length > 1) {
      throw new CliCommandError('project_ambiguous', `Project "${value}" is ambiguous.`, {
        status: 409,
        details: {
          value,
          candidates: candidates.map(project => project.id),
        },
      })
    }
    const project = candidates[0]!
    return {
      id: project.id,
      ...(project.name ? { name: project.name } : {}),
      ...(project.identifier ?? project.slug
        ? { identifier: project.identifier ?? project.slug }
        : {}),
    }
  }

  async #send(
    planned: PlannedApiRequest,
    globals: CommandExecutionGlobals,
  ): Promise<{ data: unknown, requestId: string, status: number }> {
    const client = await this.#client(globals)
    const headers = new Headers(planned.headers)
    if (globals.idempotencyKey)
      headers.set('idempotency-key', globals.idempotencyKey)
    const result = await client.request({
      method: planned.method,
      path: planned.path,
      query: planned.query,
      body: planned.body,
      headers,
      requestId: globals.requestId,
      timeoutMs: globals.timeoutMs,
    })
    if (!result.ok)
      throw apiFailure(result.error)
    return result
  }

  async #ensureServerCompatibility(
    globals: CommandExecutionGlobals,
  ): Promise<void> {
    if (!this.#compatibility)
      return
    const server = await this.#configuredServer(globals)
    const cacheKey = new URL(server).origin
    if (this.#compatibleServers.has(cacheKey))
      return

    let meta: LunaApiMeta
    try {
      meta = await this.getMeta(server, globals)
    }
    catch (error) {
      const normalized = toCliCommandError(error)
      throw new CliCommandError(
        'server_capability_negotiation_failed',
        'The Luna server compatibility metadata could not be verified.',
        {
          status: 502,
          retryable: normalized.retryable,
          details: {
            server,
            causeCode: normalized.code,
            causeStatus: normalized.status,
          },
          cause: error,
        },
      )
    }
    assertServerCompatibility(meta, this.#compatibility)
    this.#compatibleServers.add(cacheKey)
  }

  async #client(globals: CommandExecutionGlobals): Promise<LunaClient> {
    if (globals.insecureSkipTlsVerify) {
      throw new CliCommandError(
        'insecure_tls_unsupported',
        'This runtime cannot safely isolate insecure TLS verification for one request.',
        {
          status: 501,
          details: {
            remediation: 'Configure a trusted CA for the selected instance.',
          },
        },
      )
    }
    let config = await this.#config.read()
    let runtime = resolveRuntimeContext(config, {
      server: globals.server,
      project: globals.project,
      output: globals.output,
      language: globals.lang,
      env: this.#env,
    })
    if (
      runtime.sources.credential === 'config'
      && runtime.credential?.type === 'oauth'
      && oauthCredentialNeedsRefresh(runtime.credential.expiresAt, this.#now())
    ) {
      if (!runtime.credential.refreshToken) {
        throw new CliCommandError(
          'oauth_refresh_token_required',
          'The stored OAuth credential expired and does not contain a refresh token.',
          { status: 401 },
        )
      }
      const refreshed = await this.#oauthClient.refreshOAuthCredential({
        server: runtime.server,
        refreshToken: runtime.credential.refreshToken,
        scopes: runtime.credential.scopes,
      })
      await storeValidatedOAuthCredential(this.#config, {
        server: runtime.server,
        accessToken: refreshed.accessToken,
        refreshToken: refreshed.refreshToken,
        tokenType: refreshed.tokenType ?? runtime.credential.tokenType,
        scopes: refreshed.scopes.length > 0
          ? refreshed.scopes
          : runtime.credential.scopes,
        user: runtime.credential.user,
        expiresAt: refreshed.expiresAt,
        project: runtime.project ?? null,
      })
      config = await this.#config.read()
      runtime = resolveRuntimeContext(config, {
        server: globals.server,
        project: globals.project,
        output: globals.output,
        language: globals.lang,
        env: this.#env,
      })
    }
    const credential = runtime.credential
    const token = credential?.type === 'oauth'
      ? credential.accessToken
      : credential?.type === 'access_token'
        ? credential.token
        : undefined
    return this.#clientFactory({
      baseUrl: runtime.server,
      timeoutMs: globals.timeoutMs,
      tokenProvider: token
        ? { getAccessToken: () => token }
        : undefined,
    })
  }

  async #configuredServer(globals: CommandExecutionGlobals): Promise<string> {
    const config = await this.#config.read()
    const runtime = resolveRuntimeContext(config, {
      server: globals.server,
      project: globals.project,
      output: globals.output,
      language: globals.lang,
      env: this.#env,
    })
    return runtime.server
  }
}

function oauthCredentialNeedsRefresh(
  expiresAt: string | undefined,
  now: number,
): boolean {
  if (!expiresAt)
    return false
  const expiresAtMs = Date.parse(expiresAt)
  return Number.isFinite(expiresAtMs) && expiresAtMs <= now + OAUTH_REFRESH_SKEW_MS
}

function requiredMetaString(
  value: unknown,
  field: string,
): string {
  if (typeof value === 'string' && value.trim())
    return value
  throw new CliCommandError(
    'server_meta_invalid',
    `Server metadata field "${field}" is missing or invalid.`,
    { status: 502, details: { field } },
  )
}

function requestProjectId(request: ApiExecutionRequest): string | undefined {
  for (const parameter of request.metadata.parameters) {
    if (!PROJECT_PARAMETER_NAMES.has(parameter.name))
      continue
    const value = request.params[parameter.name]
    if (typeof value === 'string' && value.trim())
      return value
  }
  return undefined
}

export function planOpenApiRequest(request: ApiExecutionRequest): PlannedApiRequest {
  const method = normalizeMethod(request.metadata.method)
  const pathParameters: Record<string, unknown> = {}
  const query: Record<string, QueryValue> = {}
  const headers: Record<string, string> = {}
  const bodyFields: Record<string, unknown> = {}
  const consumed = new Set<string>()
  let explicitBody: unknown

  for (const parameter of request.metadata.parameters) {
    const value = parameterValue(parameter, request.params, request.globals)
    if (value === undefined)
      continue
    consumed.add(parameter.name)
    switch (parameter.location) {
      case 'path':
        pathParameters[parameter.name] = value
        break
      case 'header':
        headers[parameter.name] = headerValue(value, parameter.name)
        break
      case 'cookie':
        throw new CliCommandError(
          'cookie_parameter_unsupported',
          `Cookie parameter "${parameter.name}" is not supported by the CLI.`,
          { status: 501, details: { parameter: parameter.name } },
        )
      case 'body':
        if (parameter.name === 'body')
          explicitBody = value
        else bodyFields[parameter.name] = value
        break
      case 'query':
        query[parameter.name] = queryValue(value, parameter.name)
        break
      default:
        if (QUERY_METHODS.has(method))
          query[parameter.name] = queryValue(value, parameter.name)
        else bodyFields[parameter.name] = value
    }
  }

  for (const [name, value] of Object.entries(request.params)) {
    if (consumed.has(name))
      continue
    if (name === 'params' && isRecord(value)) {
      for (const [nestedName, nestedValue] of Object.entries(value)) {
        if (QUERY_METHODS.has(method))
          query[nestedName] = queryValue(nestedValue, nestedName)
        else bodyFields[nestedName] = nestedValue
      }
      continue
    }
    if (QUERY_METHODS.has(method))
      query[name] = queryValue(value, name)
    else bodyFields[name] = value
  }

  if (request.globals.dryRun === 'server')
    query.dryRun = true
  const path = interpolatePath(request.metadata.path!, pathParameters)
  const body = explicitBody === undefined
    ? Object.keys(bodyFields).length > 0 ? bodyFields : undefined
    : Object.keys(bodyFields).length > 0
      ? mergeBody(explicitBody, bodyFields)
      : explicitBody

  return {
    method,
    path,
    ...(Object.keys(query).length > 0 ? { query } : {}),
    ...(Object.keys(headers).length > 0 ? { headers } : {}),
    ...(body !== undefined ? { body } : {}),
  }
}

function assertSupportedTransport(metadata: NormalizedCommandMetadata): void {
  if (metadata.transport !== 'http') {
    throw new CliCommandError(
      'transport_not_implemented',
      `Transport "${metadata.transport}" is not implemented by the generic executor.`,
      {
        status: 501,
        details: {
          command: metadata.canonicalPath,
          transport: metadata.transport,
        },
      },
    )
  }
}

function parameterValue(
  parameter: NormalizedCommandMetadata['parameters'][number],
  params: Readonly<Record<string, unknown>>,
  globals: CommandExecutionGlobals,
): unknown {
  const { name } = parameter
  if (Object.hasOwn(params, name))
    return params[name]
  if (parameter.required && PROJECT_PARAMETER_NAMES.has(name))
    return globals.project
  return undefined
}

function interpolatePath(
  template: string,
  values: Readonly<Record<string, unknown>>,
): string {
  const missing: string[] = []
  const path = template.replace(/\{([^}]+)\}/g, (_match, name: string) => {
    const value = values[name]
    if (value === undefined || value === null || value === '') {
      missing.push(name)
      return ''
    }
    return encodeURIComponent(String(value))
  })
  if (missing.length > 0) {
    throw new CliCommandError(
      'missing_path_parameter',
      `Missing path parameter${missing.length > 1 ? 's' : ''}: ${missing.join(', ')}.`,
      { status: 400, exitCode: 2, details: { parameters: missing } },
    )
  }
  return path
}

function normalizeMethod(value: string | undefined): HttpMethod {
  const method = value?.toUpperCase() as HttpMethod | undefined
  if (!method || !HTTP_METHODS.has(method)) {
    throw new CliCommandError('http_method_invalid', `Unsupported HTTP method "${value ?? ''}".`, {
      status: 400,
      exitCode: 2,
      details: { method: value },
    })
  }
  return method
}

function validateDiagnosticPath(value: string): string {
  if (!value.startsWith('/api/') || value.startsWith('//') || value.includes('\\')
    || /^[a-z][a-z0-9+.-]*:/i.test(value)) {
    throw new CliCommandError(
      'diagnostic_path_invalid',
      'Diagnostic paths must be relative Luna API paths beginning with /api/.',
      { status: 400, exitCode: 2, details: { path: value } },
    )
  }
  return value
}

function queryValue(value: unknown, name: string): QueryValue {
  if (value === undefined || value === null)
    return value
  if (Array.isArray(value)) {
    if (value.every(isQueryPrimitive))
      return value
  }
  else if (isQueryPrimitive(value)) {
    return value
  }
  throw new CliCommandError(
    'query_parameter_invalid',
    `Query parameter "${name}" must be a primitive value or a primitive array.`,
    { status: 400, exitCode: 2, details: { parameter: name } },
  )
}

function toQueryInput(values: Readonly<Record<string, unknown>>): QueryInput {
  return Object.fromEntries(
    Object.entries(values).map(([name, value]) => [name, queryValue(value, name)]),
  )
}

function isQueryPrimitive(value: unknown): value is QueryPrimitive {
  return typeof value === 'string'
    || typeof value === 'number'
    || typeof value === 'boolean'
    || value instanceof Date
}

function headerValue(value: unknown, name: string): string {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    const rendered = String(value)
    if (!/[\r\n]/.test(rendered))
      return rendered
  }
  throw new CliCommandError(
    'header_parameter_invalid',
    `Header parameter "${name}" must be a scalar value without line breaks.`,
    { status: 400, exitCode: 2, details: { parameter: name } },
  )
}

function mergeBody(explicitBody: unknown, fields: Readonly<Record<string, unknown>>): unknown {
  if (!isRecord(explicitBody)) {
    throw new CliCommandError(
      'request_body_conflict',
      'A non-object request body cannot be combined with body fields.',
      { status: 400, exitCode: 2 },
    )
  }
  return { ...explicitBody, ...fields }
}

function mergeDiagnosticBody(
  explicitBody: unknown,
  fields: Readonly<Record<string, unknown>>,
): unknown {
  return explicitBody === undefined ? fields : mergeBody(explicitBody, fields)
}

function apiFailure(error: {
  code: string
  message: string
  status: number
  retryable: boolean
  requestId: string
  details: Readonly<Record<string, unknown>>
  purpose?: string
}): CliCommandError {
  return new CliCommandError(error.code, error.message, {
    status: error.status,
    retryable: error.retryable,
    details: {
      ...error.details,
      requestId: error.requestId,
      ...(error.purpose ? { purpose: error.purpose } : {}),
    },
  })
}

function listItems(value: unknown): Array<{
  id: string
  name?: string
  identifier?: string
  slug?: string
}> {
  const array = Array.isArray(value)
    ? value
    : isRecord(value) && Array.isArray(value.items)
      ? value.items
      : []
  return array
    .filter(isRecord)
    .flatMap((item) => {
      if (typeof item.id !== 'string')
        return []
      return [{
        id: item.id,
        ...(typeof item.name === 'string' ? { name: item.name } : {}),
        ...(typeof item.identifier === 'string' ? { identifier: item.identifier } : {}),
        ...(typeof item.slug === 'string' ? { slug: item.slug } : {}),
      }]
    })
}

function asRecord(value: unknown): Readonly<Record<string, unknown>> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new CliCommandError(
      'api_response_invalid',
      'The Luna server returned an invalid current-user response.',
      { status: 502 },
    )
  }
  return value as Readonly<Record<string, unknown>>
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
