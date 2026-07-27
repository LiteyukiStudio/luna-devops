import type { ConfigPort } from '../commands/types.js'
import type { OAuthRevokeRequest } from './oauth.js'
import type { LogoutLocalResult } from './types.js'
import { updateConfig } from '../config/store.js'

export interface LogoutLocalOptions {
  readonly revoke?: (request: OAuthRevokeRequest) => Promise<void>
}

export async function logoutLocal(
  store: ConfigPort,
  options: LogoutLocalOptions = {},
): Promise<LogoutLocalResult> {
  const current = await store.read()
  const loggedOut = current.credential !== null && current.credential !== undefined
  let remoteRevocation: LogoutLocalResult['remoteRevocation'] = 'not_applicable'

  if (current.credential?.type === 'oauth' && options.revoke) {
    const tokens = [
      {
        token: stringValue(current.credential.refreshToken),
        tokenTypeHint: 'refresh_token' as const,
      },
      {
        token: stringValue(current.credential.accessToken),
        tokenTypeHint: 'access_token' as const,
      },
    ].filter((entry): entry is { token: string, tokenTypeHint: 'access_token' | 'refresh_token' } =>
      entry.token !== undefined,
    )
    const results = await Promise.allSettled(
      tokens.map(entry => options.revoke!({
        server: current.server,
        token: entry.token,
        tokenTypeHint: entry.tokenTypeHint,
      })),
    )
    remoteRevocation = results.some(result => result.status === 'rejected')
      ? 'failed'
      : 'succeeded'
  }

  await updateConfig(store, (config) => {
    config.credential = null
    config.project = null
  })
  return {
    server: current.server,
    loggedOut,
    remoteRevocation,
  }
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value : undefined
}
