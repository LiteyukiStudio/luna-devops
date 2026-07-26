import { describe, expect, it } from 'vitest'

import {
  accessTokenFromEnvironment,
  getAuthStatus,
  storeValidatedAccessToken,
} from '../../src/auth/index.js'
import { MemoryConfigStore } from '../config/memory-store.js'

describe('access-token authentication', () => {
  it('stores one validated credential bound to its normalized server', async () => {
    const store = new MemoryConfigStore()

    await storeValidatedAccessToken(store, {
      server: 'https://devops.example.com/',
      token: ' secret-token ',
      scopes: ['project:read', ' project:read ', 'build:write'],
      user: { id: 'usr_1', name: 'Luna' },
      project: { id: 'prj_1', identifier: 'platform' },
    })

    expect(store.value.server).toBe('https://devops.example.com')
    expect(store.value.project).toEqual({
      id: 'prj_1',
      identifier: 'platform',
    })
    expect(store.value.credential).toMatchObject({
      type: 'access_token',
      token: 'secret-token',
      scopes: ['build:write', 'project:read'],
      user: { id: 'usr_1', name: 'Luna' },
    })
  })

  it('keeps LUNA_TOKEN process-local and reports it as an environment override', async () => {
    const store = new MemoryConfigStore()
    await storeValidatedAccessToken(store, {
      server: 'https://devops.example.com',
      token: 'stored-token',
    })
    const before = structuredClone(store.value)

    expect(accessTokenFromEnvironment({ LUNA_TOKEN: 'temporary-token' })).toEqual({
      type: 'access_token',
      token: 'temporary-token',
      scopes: [],
    })
    const status = await getAuthStatus(store, {
      env: { LUNA_TOKEN: 'temporary-token' },
    })

    expect(status.credential).toMatchObject({
      source: 'environment',
      type: 'access_token',
    })
    expect(store.value).toEqual(before)
    expect(JSON.stringify(status)).not.toContain('temporary-token')
  })

  it('does not expose stored token values through auth status', async () => {
    const store = new MemoryConfigStore()
    await storeValidatedAccessToken(store, {
      server: 'https://devops.example.com',
      token: 'stored-secret',
      scopes: ['project:read'],
    })

    const status = await getAuthStatus(store, { env: {} })

    expect(status.authenticated).toBe(true)
    expect(status.credential).toMatchObject({
      source: 'stored',
      scopes: ['project:read'],
    })
    expect(JSON.stringify(status)).not.toContain('stored-secret')
  })
})
