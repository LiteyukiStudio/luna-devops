import type { ConfigPort } from '../commands/types.js'
import type { LunaCredential } from '../config/schema.js'
import type { AuthStatusEntry } from './types.js'
import { parseConfigDocument } from '../config/schema.js'
import { normalizeServerOrigin } from '../config/server.js'
import { accessTokenFromEnvironment } from './access-token.js'

export interface AuthStatusOptions {
  readonly now?: Date
  readonly env?: Readonly<Record<string, string | undefined>>
}

export async function getAuthStatus(
  store: ConfigPort,
  options: AuthStatusOptions = {},
): Promise<AuthStatusEntry> {
  const config = parseConfigDocument(await store.read())
  const environmentCredential = accessTokenFromEnvironment(options.env)
  const credential = environmentCredential ?? config.credential ?? undefined
  const source = environmentCredential ? 'environment' : 'stored'
  return {
    server: normalizeServerOrigin(config.server),
    authenticated: credential !== undefined && !isExpired(credential, options.now),
    credential: credential
      ? {
          type: credential.type,
          scopes: [...credential.scopes],
          user: credential.user,
          expiresAt: credential.expiresAt,
          expired: isExpired(credential, options.now),
          source,
        }
      : undefined,
  }
}

function isExpired(credential: LunaCredential, now = new Date()): boolean {
  return credential.expiresAt !== undefined
    && Date.parse(credential.expiresAt) <= now.getTime()
}
