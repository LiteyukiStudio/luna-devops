import type { ConfigPort } from '../commands/types.js'
import type { OAuthCredential, StoredLunaConfig } from '../config/schema.js'
import type { StoreOAuthCredentialInput } from './types.js'
import { CliCommandError } from '../commands/errors.js'
import { normalizeServerOrigin } from '../config/server.js'
import { updateConfig } from '../config/store.js'
import {
  assertIsoDate,
  normalizeScopes,
} from './validation.js'

export async function storeValidatedOAuthCredential(
  store: ConfigPort,
  input: StoreOAuthCredentialInput,
): Promise<StoredLunaConfig> {
  const accessToken = input.accessToken.trim()
  const refreshToken = input.refreshToken?.trim() || undefined
  if (!accessToken) {
    throw new CliCommandError(
      'oauth_access_token_required',
      'A validated OAuth access token is required.',
      { status: 422 },
    )
  }
  assertIsoDate(input.expiresAt)

  return updateConfig(store, (config) => {
    const credential: OAuthCredential = {
      type: 'oauth',
      accessToken,
      refreshToken,
      tokenType: input.tokenType?.trim() || undefined,
      scopes: normalizeScopes(input.scopes),
      user: input.user,
      expiresAt: input.expiresAt,
      createdAt: new Date().toISOString(),
    }
    config.server = normalizeServerOrigin(input.server)
    config.credential = credential
    config.project = input.project ?? null
  })
}
