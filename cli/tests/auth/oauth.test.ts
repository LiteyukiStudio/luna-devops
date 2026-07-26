import { describe, expect, it } from 'vitest'

import {
  beginOAuthLogin,
  getAuthStatus,
  storeValidatedOAuthCredential,
} from '../../src/auth/index.js'
import { MemoryConfigStore } from '../config/memory-store.js'

describe('oAuth authentication', () => {
  it('stores one OAuth credential without exposing it through status', async () => {
    const store = new MemoryConfigStore()

    await storeValidatedOAuthCredential(store, {
      server: 'https://devops.example.com',
      accessToken: 'access-secret',
      refreshToken: 'refresh-secret',
      tokenType: 'Bearer',
      scopes: ['openid', 'project:read'],
      expiresAt: '2030-01-01T00:00:00.000Z',
      user: { id: 'usr_1', name: 'Luna' },
    })

    const credential = store.value.credential
    expect(credential).toMatchObject({
      type: 'oauth',
      accessToken: 'access-secret',
      refreshToken: 'refresh-secret',
      tokenType: 'Bearer',
      scopes: ['openid', 'project:read'],
    })

    const status = await getAuthStatus(store, { env: {} })
    expect(status.credential).toMatchObject({
      type: 'oauth',
      scopes: ['openid', 'project:read'],
    })
    expect(JSON.stringify(status)).not.toContain('access-secret')
    expect(JSON.stringify(status)).not.toContain('refresh-secret')
  })

  it('reports expired OAuth credentials without exposing their token values', async () => {
    const store = new MemoryConfigStore()
    await storeValidatedOAuthCredential(store, {
      server: 'https://devops.example.com',
      accessToken: 'expired-access-secret',
      refreshToken: 'expired-refresh-secret',
      expiresAt: '2029-01-01T00:00:00.000Z',
    })

    const status = await getAuthStatus(store, {
      env: {},
      now: new Date('2030-01-01T00:00:00.000Z'),
    })

    expect(status.authenticated).toBe(false)
    expect(status.credential).toMatchObject({
      type: 'oauth',
      expired: true,
      expiresAt: '2029-01-01T00:00:00.000Z',
    })
    expect(JSON.stringify(status)).not.toContain('expired-access-secret')
    expect(JSON.stringify(status)).not.toContain('expired-refresh-secret')
  })

  it('returns a stable typed error while server OAuth endpoints are unavailable', async () => {
    await expect(
      beginOAuthLogin({
        server: 'https://devops.example.com',
        scopes: ['openid'],
        mode: 'device_code',
      }),
    ).rejects.toMatchObject({
      code: 'oauth_server_capability_unavailable',
      status: 501,
      retryable: false,
    })
  })
})
