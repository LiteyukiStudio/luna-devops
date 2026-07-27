import { describe, expect, it } from 'vitest'

import {
  beginOAuthLogin,
  getAuthStatus,
  refreshOAuthCredential,
  revokeOAuthCredential,
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

  it('authorizes with Device Code, opens the browser, and polls until approved', async () => {
    const calls: Array<{ url: string, form: URLSearchParams }> = []
    const responses = [
      jsonResponse({
        device_code: 'device-secret',
        user_code: 'LUNA-CODE',
        verification_uri: 'https://devops.example.com/device',
        verification_uri_complete: 'https://devops.example.com/device?user_code=LUNA-CODE',
        expires_in: 600,
        interval: 2,
      }),
      jsonResponse({ error: 'authorization_pending' }, 400),
      jsonResponse({
        access_token: 'access-secret',
        refresh_token: 'refresh-secret',
        token_type: 'Bearer',
        expires_in: 3600,
        scope: 'openid project:read',
        user: { id: 'usr_1', name: 'Luna' },
      }),
    ]
    const fetch = async (input: string | URL | Request, init?: RequestInit) => {
      calls.push({
        url: String(input),
        form: new URLSearchParams(String(init?.body ?? '')),
      })
      return responses.shift()!
    }
    const browserUrls: string[] = []
    const verifications: unknown[] = []
    const sleeps: number[] = []

    const result = await beginOAuthLogin({
      server: 'https://devops.example.com/',
      scopes: ['openid', 'project:read', 'openid'],
      mode: 'device_code',
      fetch,
      openBrowser: async (url) => {
        browserUrls.push(url)
        return true
      },
      onVerification: (verification) => {
        verifications.push(verification)
      },
      sleep: async (milliseconds) => {
        sleeps.push(milliseconds)
      },
      now: () => Date.parse('2030-01-01T00:00:00.000Z'),
    })

    expect(calls.map(call => call.url)).toEqual([
      'https://devops.example.com/api/v1/oauth/device/authorization',
      'https://devops.example.com/api/v1/oauth/token',
      'https://devops.example.com/api/v1/oauth/token',
    ])
    expect(Object.fromEntries(calls[0]!.form)).toEqual({
      client_id: 'luna-cli',
      scope: 'openid project:read',
    })
    expect(Object.fromEntries(calls[1]!.form)).toEqual({
      grant_type: 'urn:ietf:params:oauth:grant-type:device_code',
      device_code: 'device-secret',
      client_id: 'luna-cli',
    })
    expect(browserUrls).toEqual([
      'https://devops.example.com/device?user_code=LUNA-CODE',
    ])
    expect(sleeps).toEqual([2_000, 2_000])
    expect(verifications).toHaveLength(1)
    expect(result).toMatchObject({
      server: 'https://devops.example.com',
      accessToken: 'access-secret',
      refreshToken: 'refresh-secret',
      tokenType: 'Bearer',
      scopes: ['openid', 'project:read'],
      expiresAt: '2030-01-01T01:00:00.000Z',
      verification: {
        userCode: 'LUNA-CODE',
        browserOpened: true,
      },
    })
  })

  it('refreshes an OAuth credential and preserves a rotated or existing refresh token', async () => {
    const forms: URLSearchParams[] = []
    const credential = await refreshOAuthCredential({
      server: 'https://devops.example.com',
      refreshToken: 'refresh-secret',
      scopes: ['project:read'],
      fetch: async (_input, init) => {
        forms.push(new URLSearchParams(String(init?.body ?? '')))
        return jsonResponse({
          access_token: 'new-access-secret',
          token_type: 'Bearer',
          expires_in: 1800,
        })
      },
      now: () => Date.parse('2030-01-01T00:00:00.000Z'),
    })

    expect(Object.fromEntries(forms[0]!)).toEqual({
      grant_type: 'refresh_token',
      refresh_token: 'refresh-secret',
      client_id: 'luna-cli',
      scope: 'project:read',
    })
    expect(credential).toEqual({
      accessToken: 'new-access-secret',
      refreshToken: 'refresh-secret',
      tokenType: 'Bearer',
      scopes: ['project:read'],
      expiresAt: '2030-01-01T00:30:00.000Z',
      user: undefined,
    })
  })

  it('revokes a token with the RFC 7009 token type hint', async () => {
    const requests: Array<{ url: string, form: URLSearchParams }> = []

    await revokeOAuthCredential({
      server: 'https://devops.example.com',
      token: 'refresh-secret',
      tokenTypeHint: 'refresh_token',
      fetch: async (input, init) => {
        requests.push({
          url: String(input),
          form: new URLSearchParams(String(init?.body ?? '')),
        })
        return new Response(null, { status: 200 })
      },
    })

    expect(requests[0]?.url).toBe('https://devops.example.com/api/v1/oauth/revoke')
    expect(Object.fromEntries(requests[0]!.form)).toEqual({
      token: 'refresh-secret',
      client_id: 'luna-cli',
      token_type_hint: 'refresh_token',
    })
  })
})

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}
