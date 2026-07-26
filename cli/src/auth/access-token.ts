import type { ConfigPort } from '../commands/types.js'
import type { AccessTokenCredential, StoredLunaConfig } from '../config/schema.js'
import type { StoreAccessTokenInput } from './types.js'
import process from 'node:process'
import { CliCommandError } from '../commands/errors.js'
import { normalizeServerOrigin } from '../config/server.js'
import { updateConfig } from '../config/store.js'
import {
  assertIsoDate,
  normalizeScopes,
} from './validation.js'

export async function storeValidatedAccessToken(
  store: ConfigPort,
  input: StoreAccessTokenInput,
): Promise<StoredLunaConfig> {
  const token = input.token.trim()
  if (!token) {
    throw new CliCommandError(
      'access_token_required',
      'A validated access token is required.',
      { status: 422 },
    )
  }
  assertIsoDate(input.expiresAt)

  return updateConfig(store, (config) => {
    const credential: AccessTokenCredential = {
      type: 'access_token',
      token,
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

export function accessTokenFromEnvironment(
  env: Readonly<Record<string, string | undefined>> = process.env,
): AccessTokenCredential | undefined {
  const token = env.LUNA_TOKEN?.trim()
  if (!token)
    return undefined
  return {
    type: 'access_token',
    token,
    scopes: [],
  }
}
