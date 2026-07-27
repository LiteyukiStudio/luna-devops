import type { QueryInput } from '@luna-devops/api-client'
import type {
  CommandInvocation,
  NormalizedCommandMetadata,
  RuntimePorts,
} from './types.js'
import { resolveRuntimeContext } from '../config/resolve.js'
import { planOpenApiRequest } from './api.js'
import { CliCommandError } from './errors.js'

const LOCAL_PROTOCOL_PARAMETERS = new Set([
  'destination',
  'maxBytes',
  'maxEvents',
  'overwrite',
])
const OAUTH_REFRESH_SKEW_MS = 30_000

export interface ProtocolRequest {
  readonly response: Response
  readonly requestId?: string
  readonly server: string
}

export async function openProtocolRequest(
  invocation: CommandInvocation,
  ports: RuntimePorts,
  accept: string,
): Promise<ProtocolRequest> {
  if (invocation.globals.insecureSkipTlsVerify) {
    throw new CliCommandError(
      'insecure_tls_unsupported',
      'This runtime cannot safely isolate insecure TLS verification for one request.',
      {
        status: 501,
        details: { remediation: 'Configure a trusted CA for the selected instance.' },
      },
    )
  }

  const config = await ports.config.read()
  let runtime = resolveRuntimeContext(config, {
    server: invocation.globals.server,
    project: invocation.globals.project,
    output: invocation.globals.output,
    language: invocation.globals.lang,
    env: ports.env,
  })
  if (
    runtime.sources.credential === 'config'
    && runtime.credential?.type === 'oauth'
    && credentialNeedsRefresh(runtime.credential.expiresAt)
  ) {
    const refreshToken = runtime.credential.refreshToken
    if (!refreshToken || !ports.api.refreshOAuthCredential) {
      throw new CliCommandError(
        'oauth_refresh_token_required',
        'The stored OAuth credential expired and cannot be refreshed.',
        { status: 401 },
      )
    }
    const refreshed = await ports.api.refreshOAuthCredential({
      server: runtime.server,
      refreshToken,
      scopes: runtime.credential.scopes,
    })
    await ports.config.write({
      ...config,
      server: runtime.server,
      credential: {
        ...runtime.credential,
        type: 'oauth',
        accessToken: refreshed.accessToken,
        refreshToken: refreshed.refreshToken ?? refreshToken,
        tokenType: refreshed.tokenType ?? runtime.credential.tokenType,
        scopes: refreshed.scopes.length > 0
          ? refreshed.scopes
          : runtime.credential.scopes,
        expiresAt: refreshed.expiresAt,
      },
    })
    runtime = resolveRuntimeContext(await ports.config.read(), {
      server: invocation.globals.server,
      project: invocation.globals.project,
      output: invocation.globals.output,
      language: invocation.globals.lang,
      env: ports.env,
    })
  }

  const token = runtime.credential?.type === 'oauth'
    ? runtime.credential.accessToken
    : runtime.credential?.type === 'access_token'
      ? runtime.credential.token
      : undefined
  if (!token) {
    throw new CliCommandError(
      'authentication_required',
      'Sign in before using this protocol command.',
      { status: 401 },
    )
  }

  const metadata = requestMetadata(invocation.metadata)
  const planned = planOpenApiRequest({
    operationId: metadata.operationId ?? metadata.canonicalPath,
    metadata,
    params: requestParams(invocation.params),
    globals: invocation.globals,
  })
  const url = new URL(planned.path, `${runtime.server}/`)
  appendQuery(url, planned.query)
  const headers = new Headers(planned.headers)
  headers.set('accept', accept)
  headers.set('authorization', `Bearer ${token}`)
  if (invocation.globals.requestId)
    headers.set('x-request-id', invocation.globals.requestId)
  if (invocation.globals.idempotencyKey)
    headers.set('idempotency-key', invocation.globals.idempotencyKey)

  const controller = new AbortController()
  const connectionTimeout = setTimeout(
    () => controller.abort('timeout'),
    invocation.globals.timeoutMs,
  )
  let response: Response
  try {
    response = await (ports.protocol?.fetch ?? globalThis.fetch)(url, {
      method: planned.method,
      headers,
      redirect: 'manual',
      signal: controller.signal,
    })
  }
  catch (error) {
    if (controller.signal.aborted) {
      throw new CliCommandError('request_timeout', 'The protocol request timed out.', {
        status: 504,
        retryable: true,
        details: { timeoutMs: invocation.globals.timeoutMs },
        cause: error,
      })
    }
    throw new CliCommandError('network_error', 'The protocol request could not be sent.', {
      status: 503,
      retryable: true,
      cause: error,
    })
  }
  finally {
    // --timeout bounds connection establishment and response headers only.
    // Long-lived streams and downloads enforce their own read limits.
    clearTimeout(connectionTimeout)
  }

  if (response.status >= 300 && response.status < 400) {
    await cancelBody(response)
    throw new CliCommandError(
      'protocol_redirect_refused',
      'The protocol endpoint returned a redirect. Refusing to forward credentials.',
      {
        status: 502,
        details: {
          location: response.headers.get('location') ?? '',
          requestUrl: url.origin + url.pathname,
        },
      },
    )
  }
  if (!response.ok)
    throw await responseError(response)

  const requestId = response.headers.get('x-request-id') ?? undefined
  return {
    response,
    server: runtime.server,
    ...(requestId ? { requestId } : {}),
  }
}

function credentialNeedsRefresh(expiresAt: string | undefined): boolean {
  if (!expiresAt)
    return false
  const expiresAtMs = Date.parse(expiresAt)
  return Number.isFinite(expiresAtMs)
    && expiresAtMs <= Date.now() + OAUTH_REFRESH_SKEW_MS
}

function requestMetadata(metadata: NormalizedCommandMetadata): NormalizedCommandMetadata {
  return {
    ...metadata,
    parameters: metadata.parameters.filter(parameter =>
      !LOCAL_PROTOCOL_PARAMETERS.has(parameter.name)),
  }
}

function requestParams(
  params: Readonly<Record<string, unknown>>,
): Readonly<Record<string, unknown>> {
  return Object.fromEntries(
    Object.entries(params).filter(([name]) => !LOCAL_PROTOCOL_PARAMETERS.has(name)),
  )
}

function appendQuery(url: URL, query: QueryInput | undefined): void {
  if (!query)
    return
  if (query instanceof URLSearchParams) {
    for (const [name, value] of query)
      url.searchParams.append(name, value)
    return
  }
  for (const [name, value] of Object.entries(query)) {
    if (Array.isArray(value)) {
      for (const item of value)
        appendQueryValue(url, name, item)
    }
    else {
      appendQueryValue(url, name, value)
    }
  }
}

function appendQueryValue(url: URL, name: string, value: unknown): void {
  if (value === undefined || value === null)
    return
  url.searchParams.append(name, value instanceof Date ? value.toISOString() : String(value))
}

async function responseError(response: Response): Promise<CliCommandError> {
  const requestId = response.headers.get('x-request-id') ?? ''
  const contentType = response.headers.get('content-type') ?? ''
  let payload: unknown
  try {
    payload = contentType.includes('json')
      ? await response.json()
      : { message: (await response.text()).slice(0, 4096) }
  }
  catch {
    payload = {}
  }
  const root = asRecord(payload)
  const nested = asRecord(root.error)
  const code = stringValue(nested.code) ?? stringValue(root.code)
    ?? `http_${response.status}`
  const message = stringValue(nested.message) ?? stringValue(root.message)
    ?? `Protocol request failed with HTTP ${response.status}.`
  const purpose = stringValue(nested.purpose) ?? stringValue(root.purpose)
    ?? stringValue(asRecord(nested.details).purpose)
    ?? stringValue(asRecord(root.details).purpose)
  return new CliCommandError(code, message, {
    status: response.status,
    retryable: response.status === 429 || response.status >= 500,
    details: {
      ...asRecord(nested.details),
      ...asRecord(root.details),
      ...(requestId ? { requestId } : {}),
      ...(purpose ? { purpose } : {}),
    },
  })
}

async function cancelBody(response: Response): Promise<void> {
  try {
    await response.body?.cancel()
  }
  catch {
    // The response is already being torn down.
  }
}

function asRecord(value: unknown): Readonly<Record<string, unknown>> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Readonly<Record<string, unknown>>
    : {}
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined
}
