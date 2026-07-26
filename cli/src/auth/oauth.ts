import { CliCommandError } from '../commands/errors.js'

export type OAuthLoginMode = 'authorization_code_pkce' | 'device_code'

export interface OAuthLoginRequest {
  readonly server: string
  readonly scopes: readonly string[]
  readonly mode: OAuthLoginMode
}

export interface OAuthRefreshRequest {}

export interface OAuthRevokeRequest {}

export async function beginOAuthLogin(
  _request: OAuthLoginRequest,
): Promise<never> {
  throw oauthUnavailable('OAuth login')
}

export async function refreshOAuthCredential(
  _request: OAuthRefreshRequest,
): Promise<never> {
  throw oauthUnavailable('OAuth token refresh')
}

export async function revokeOAuthCredential(
  _request: OAuthRevokeRequest,
): Promise<never> {
  throw oauthUnavailable('OAuth token revocation')
}

function oauthUnavailable(capability: string): CliCommandError {
  return new CliCommandError(
    'oauth_server_capability_unavailable',
    `${capability} is unavailable until the Luna server exposes the native CLI OAuth endpoints.`,
    {
      status: 501,
      details: {
        capability,
        fallback: 'Use a personal access token through stdin or LUNA_TOKEN.',
      },
    },
  )
}
