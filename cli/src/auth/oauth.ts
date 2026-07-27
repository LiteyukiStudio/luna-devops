import { spawn } from 'node:child_process'
import process from 'node:process'
import { CliCommandError } from '../commands/errors.js'
import { normalizeServerOrigin } from '../config/server.js'

export const DEFAULT_OAUTH_CLIENT_ID = 'luna-cli'
export const DEVICE_AUTHORIZATION_PATH = '/api/v1/oauth/device/authorization'
export const OAUTH_TOKEN_PATH = '/api/v1/oauth/token'
export const OAUTH_REVOKE_PATH = '/api/v1/oauth/revoke'

const DEVICE_CODE_GRANT_TYPE = 'urn:ietf:params:oauth:grant-type:device_code'
const DEFAULT_POLL_INTERVAL_SECONDS = 5
const SLOW_DOWN_SECONDS = 5

export type OAuthLoginMode = 'authorization_code_pkce' | 'device_code'

export interface OAuthVerification {
  readonly userCode: string
  readonly verificationUri: string
  readonly verificationUriComplete?: string
  readonly expiresIn: number
  readonly interval: number
  readonly browserOpened: boolean
}

export interface OAuthTokenCredential {
  readonly accessToken: string
  readonly refreshToken?: string
  readonly tokenType?: string
  readonly scopes: readonly string[]
  readonly expiresAt?: string
  readonly user?: Readonly<Record<string, unknown>>
}

export interface OAuthLoginResult extends OAuthTokenCredential {
  readonly server: string
  readonly verification: OAuthVerification
}

export interface OAuthLoginRequest {
  readonly server: string
  readonly scopes: readonly string[]
  readonly mode: OAuthLoginMode
  readonly clientId?: string
  readonly fetch?: typeof globalThis.fetch
  readonly openBrowser?: (url: string) => Promise<boolean> | boolean
  readonly onVerification?: (verification: OAuthVerification) => Promise<void> | void
  readonly sleep?: (milliseconds: number) => Promise<void>
  readonly now?: () => number
}

export interface OAuthRefreshRequest {
  readonly server: string
  readonly refreshToken: string
  readonly clientId?: string
  readonly scopes?: readonly string[]
  readonly fetch?: typeof globalThis.fetch
  readonly now?: () => number
}

export interface OAuthRevokeRequest {
  readonly server: string
  readonly token: string
  readonly tokenTypeHint?: 'access_token' | 'refresh_token'
  readonly clientId?: string
  readonly fetch?: typeof globalThis.fetch
}

export interface OAuthClient {
  beginOAuthLogin: (request: OAuthLoginRequest) => Promise<OAuthLoginResult>
  refreshOAuthCredential: (request: OAuthRefreshRequest) => Promise<OAuthTokenCredential>
  revokeOAuthCredential: (request: OAuthRevokeRequest) => Promise<void>
}

export async function beginOAuthLogin(
  request: OAuthLoginRequest,
): Promise<OAuthLoginResult> {
  if (request.mode !== 'device_code') {
    throw new CliCommandError(
      'oauth_login_mode_unsupported',
      'Only OAuth Device Code login is supported by this CLI.',
      { status: 422, details: { mode: request.mode } },
    )
  }

  const server = normalizeServerOrigin(request.server)
  const clientId = nonEmpty(request.clientId) ?? DEFAULT_OAUTH_CLIENT_ID
  const fetchImpl = request.fetch ?? globalThis.fetch
  const sleep = request.sleep ?? delay
  const now = request.now ?? Date.now
  const authorization = await requestOAuthForm(
    fetchImpl,
    endpoint(server, DEVICE_AUTHORIZATION_PATH),
    {
      client_id: clientId,
      ...(request.scopes.length > 0 ? { scope: normalizeScopes(request.scopes).join(' ') } : {}),
    },
  )
  const deviceCode = requiredString(authorization, 'device_code')
  const userCode = requiredString(authorization, 'user_code')
  const verificationUri = requiredString(authorization, 'verification_uri')
  const verificationUriComplete = optionalString(authorization.verification_uri_complete)
  const expiresIn = positiveNumber(authorization.expires_in, 'expires_in')
  let interval = optionalPositiveNumber(authorization.interval) ?? DEFAULT_POLL_INTERVAL_SECONDS
  const browserUrl = verificationUriComplete ?? verificationUri
  const browserOpened = await bestEffortOpenBrowser(
    request.openBrowser ?? openSystemBrowser,
    browserUrl,
  )
  const verification: OAuthVerification = {
    userCode,
    verificationUri,
    verificationUriComplete,
    expiresIn,
    interval,
    browserOpened,
  }
  await request.onVerification?.(verification)

  const deadline = now() + expiresIn * 1_000
  while (now() < deadline) {
    await sleep(interval * 1_000)
    if (now() >= deadline)
      break

    const response = await postOAuthForm(
      fetchImpl,
      endpoint(server, OAUTH_TOKEN_PATH),
      {
        grant_type: DEVICE_CODE_GRANT_TYPE,
        device_code: deviceCode,
        client_id: clientId,
      },
    )
    const body = await responseRecord(response)
    if (response.ok) {
      return {
        server,
        verification,
        ...parseTokenCredential(body, request.scopes, now),
      }
    }

    const oauthError = optionalString(body.error)
    if (oauthError === 'authorization_pending')
      continue
    if (oauthError === 'slow_down') {
      interval += SLOW_DOWN_SECONDS
      continue
    }
    throw oauthProtocolError(oauthError, body, response.status)
  }

  throw new CliCommandError(
    'oauth_device_code_expired',
    'The OAuth device code expired before authorization completed.',
    { status: 408, details: { server, verificationUri } },
  )
}

export async function refreshOAuthCredential(
  request: OAuthRefreshRequest,
): Promise<OAuthTokenCredential> {
  const refreshToken = nonEmpty(request.refreshToken)
  if (!refreshToken) {
    throw new CliCommandError(
      'oauth_refresh_token_required',
      'The OAuth credential does not contain a refresh token.',
      { status: 401 },
    )
  }
  const server = normalizeServerOrigin(request.server)
  const body = await requestOAuthForm(
    request.fetch ?? globalThis.fetch,
    endpoint(server, OAUTH_TOKEN_PATH),
    {
      grant_type: 'refresh_token',
      refresh_token: refreshToken,
      client_id: nonEmpty(request.clientId) ?? DEFAULT_OAUTH_CLIENT_ID,
      ...(request.scopes?.length
        ? { scope: normalizeScopes(request.scopes).join(' ') }
        : {}),
    },
  )
  const credential = parseTokenCredential(body, request.scopes ?? [], request.now ?? Date.now)
  return {
    ...credential,
    refreshToken: credential.refreshToken ?? refreshToken,
  }
}

export async function revokeOAuthCredential(
  request: OAuthRevokeRequest,
): Promise<void> {
  const token = nonEmpty(request.token)
  if (!token)
    return

  const response = await postOAuthForm(
    request.fetch ?? globalThis.fetch,
    endpoint(normalizeServerOrigin(request.server), OAUTH_REVOKE_PATH),
    {
      token,
      client_id: nonEmpty(request.clientId) ?? DEFAULT_OAUTH_CLIENT_ID,
      ...(request.tokenTypeHint ? { token_type_hint: request.tokenTypeHint } : {}),
    },
  )
  if (response.ok)
    return
  const body = await responseRecord(response)
  throw oauthProtocolError(optionalString(body.error), body, response.status)
}

export async function openSystemBrowser(url: string): Promise<boolean> {
  const command = process.platform === 'darwin'
    ? { executable: 'open', args: [url] }
    : process.platform === 'win32'
      ? { executable: 'rundll32', args: ['url.dll,FileProtocolHandler', url] }
      : { executable: 'xdg-open', args: [url] }

  return new Promise((resolve) => {
    try {
      const child = spawn(command.executable, command.args, {
        detached: true,
        stdio: 'ignore',
      })
      let settled = false
      child.once('error', () => {
        if (!settled) {
          settled = true
          resolve(false)
        }
      })
      child.once('spawn', () => {
        if (!settled) {
          settled = true
          child.unref()
          resolve(true)
        }
      })
    }
    catch {
      resolve(false)
    }
  })
}

async function requestOAuthForm(
  fetchImpl: typeof globalThis.fetch,
  url: string,
  fields: Readonly<Record<string, string>>,
): Promise<Readonly<Record<string, unknown>>> {
  const response = await postOAuthForm(fetchImpl, url, fields)
  const body = await responseRecord(response)
  if (!response.ok)
    throw oauthProtocolError(optionalString(body.error), body, response.status)
  return body
}

async function postOAuthForm(
  fetchImpl: typeof globalThis.fetch,
  url: string,
  fields: Readonly<Record<string, string>>,
): Promise<Response> {
  try {
    return await fetchImpl(url, {
      method: 'POST',
      headers: {
        'accept': 'application/json',
        'content-type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams(fields),
    })
  }
  catch (error) {
    throw new CliCommandError(
      'oauth_network_error',
      'The OAuth server could not be reached.',
      { status: 502, retryable: true, details: { url }, cause: error },
    )
  }
}

async function responseRecord(response: Response): Promise<Readonly<Record<string, unknown>>> {
  try {
    const value: unknown = await response.json()
    if (isRecord(value))
      return value
  }
  catch {
    // The stable error below intentionally avoids exposing an untrusted response body.
  }
  if (!response.ok) {
    throw new CliCommandError(
      'oauth_response_invalid',
      'The OAuth server returned an invalid response.',
      { status: response.status || 502 },
    )
  }
  return {}
}

function parseTokenCredential(
  body: Readonly<Record<string, unknown>>,
  fallbackScopes: readonly string[],
  now: () => number,
): OAuthTokenCredential {
  const expiresIn = optionalPositiveNumber(body.expires_in)
  const user = isRecord(body.user) ? body.user : undefined
  return {
    accessToken: requiredString(body, 'access_token'),
    refreshToken: optionalString(body.refresh_token),
    tokenType: optionalString(body.token_type),
    scopes: tokenScopes(body.scope, fallbackScopes),
    expiresAt: expiresIn === undefined
      ? undefined
      : new Date(now() + expiresIn * 1_000).toISOString(),
    user,
  }
}

function oauthProtocolError(
  error: string | undefined,
  body: Readonly<Record<string, unknown>>,
  status: number,
): CliCommandError {
  const code = error ? `oauth_${error}` : 'oauth_request_failed'
  const safeDescription = optionalString(body.error_description)
  return new CliCommandError(
    code,
    safeDescription ?? oauthErrorMessage(error),
    {
      status: normalizedOAuthStatus(error, status),
      retryable: error === 'temporarily_unavailable' || status >= 500,
      details: error ? { oauthError: error } : {},
    },
  )
}

function oauthErrorMessage(error: string | undefined): string {
  switch (error) {
    case 'access_denied':
      return 'OAuth authorization was denied.'
    case 'expired_token':
      return 'The OAuth device code has expired.'
    case 'invalid_client':
      return 'The Luna CLI OAuth client is not accepted by this server.'
    case 'invalid_grant':
      return 'The OAuth grant or refresh token is invalid.'
    default:
      return 'The OAuth request failed.'
  }
}

function normalizedOAuthStatus(error: string | undefined, status: number): number {
  if (error === 'access_denied')
    return 403
  if (error === 'expired_token' || error === 'invalid_grant')
    return 401
  return status || 502
}

function endpoint(server: string, path: string): string {
  return new URL(path, `${server}/`).toString()
}

function requiredString(
  body: Readonly<Record<string, unknown>>,
  field: string,
): string {
  const value = optionalString(body[field])
  if (value)
    return value
  throw new CliCommandError(
    'oauth_response_invalid',
    `The OAuth response is missing "${field}".`,
    { status: 502, details: { field } },
  )
}

function positiveNumber(value: unknown, field: string): number {
  const normalized = optionalPositiveNumber(value)
  if (normalized !== undefined)
    return normalized
  throw new CliCommandError(
    'oauth_response_invalid',
    `The OAuth response field "${field}" is invalid.`,
    { status: 502, details: { field } },
  )
}

function optionalPositiveNumber(value: unknown): number | undefined {
  const normalized = typeof value === 'number'
    ? value
    : typeof value === 'string'
      ? Number(value)
      : Number.NaN
  return Number.isFinite(normalized) && normalized > 0 ? normalized : undefined
}

function tokenScopes(value: unknown, fallback: readonly string[]): readonly string[] {
  if (typeof value === 'string')
    return normalizeScopes(value.split(/\s+/))
  if (Array.isArray(value))
    return normalizeScopes(value.filter((item): item is string => typeof item === 'string'))
  return normalizeScopes(fallback)
}

function normalizeScopes(scopes: readonly string[]): string[] {
  return [...new Set(scopes.map(scope => scope.trim()).filter(Boolean))]
}

function optionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function nonEmpty(value: string | undefined): string | undefined {
  return value?.trim() || undefined
}

function isRecord(value: unknown): value is Readonly<Record<string, unknown>> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function delay(milliseconds: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, milliseconds))
}

async function bestEffortOpenBrowser(
  opener: (url: string) => Promise<boolean> | boolean,
  url: string,
): Promise<boolean> {
  try {
    return await opener(url)
  }
  catch {
    return false
  }
}
