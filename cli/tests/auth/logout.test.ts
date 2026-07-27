import { describe, expect, it } from 'vitest'

import {
  logoutLocal,
  storeValidatedAccessToken,
  storeValidatedOAuthCredential,
} from '../../src/auth/index.js'
import { MemoryConfigStore } from '../config/memory-store.js'

describe('logoutLocal', () => {
  it('removes the active credential and project while preserving the server', async () => {
    const store = new MemoryConfigStore()
    await storeValidatedAccessToken(store, {
      server: 'https://devops.example.com',
      token: 'secret',
      project: { id: 'prj_example' },
    })

    const result = await logoutLocal(store)

    expect(result).toEqual({
      server: 'https://devops.example.com',
      loggedOut: true,
      remoteRevocation: 'not_applicable',
    })
    expect(store.value.credential).toBeNull()
    expect(store.value.project).toBeNull()
    expect(store.value.server).toBe('https://devops.example.com')
  })

  it('attempts to revoke every OAuth token and always clears local credentials', async () => {
    const store = new MemoryConfigStore()
    await storeValidatedOAuthCredential(store, {
      server: 'https://devops.example.com',
      accessToken: 'access-secret',
      refreshToken: 'refresh-secret',
    })
    const revoked: string[] = []

    const result = await logoutLocal(store, {
      revoke: async ({ token }) => {
        revoked.push(token)
        if (token === 'refresh-secret')
          throw new Error('remote unavailable')
      },
    })

    expect(revoked).toEqual(['refresh-secret', 'access-secret'])
    expect(result).toEqual({
      server: 'https://devops.example.com',
      loggedOut: true,
      remoteRevocation: 'failed',
    })
    expect(store.value.credential).toBeNull()
    expect(store.value.project).toBeNull()
  })
})
