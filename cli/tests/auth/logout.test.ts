import { describe, expect, it } from 'vitest'

import {
  logoutLocal,
  storeValidatedAccessToken,
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
    })
    expect(store.value.credential).toBeNull()
    expect(store.value.project).toBeNull()
    expect(store.value.server).toBe('https://devops.example.com')
  })
})
