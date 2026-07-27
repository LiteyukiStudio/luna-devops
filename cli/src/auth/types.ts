import type { ProjectContextSnapshot } from '../commands/types.js'
import type { LunaCredential } from '../config/schema.js'

export interface AuthUserSnapshot {
  readonly id: string
  readonly name?: string
  readonly [key: string]: unknown
}

export interface StoreAccessTokenInput {
  readonly server: string
  readonly token: string
  readonly scopes?: readonly string[]
  readonly user?: AuthUserSnapshot
  readonly expiresAt?: string
  readonly project?: ProjectContextSnapshot | null
}

export interface StoreOAuthCredentialInput {
  readonly server: string
  readonly accessToken: string
  readonly refreshToken?: string
  readonly tokenType?: string
  readonly scopes?: readonly string[]
  readonly user?: AuthUserSnapshot
  readonly expiresAt?: string
  readonly project?: ProjectContextSnapshot | null
}

export interface AuthStatusEntry {
  readonly server: string
  readonly authenticated: boolean
  readonly credential?: {
    readonly type: LunaCredential['type']
    readonly scopes: readonly string[]
    readonly user?: AuthUserSnapshot
    readonly expiresAt?: string
    readonly expired: boolean
    readonly source: 'stored' | 'environment'
  }
}

export interface LogoutLocalResult {
  readonly server: string
  readonly loggedOut: boolean
  readonly remoteRevocation: 'not_applicable' | 'succeeded' | 'failed'
}
