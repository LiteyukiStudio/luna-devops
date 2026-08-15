import type { MFAChallenge } from './types'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { ApiError, registerMFAChallengeHandler, request } from './core'

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('mfa request retry flow', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('retries an MFA-protected request once after verification', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: 'mfa_required', purpose: 'secret_update' }, 403))
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
    const challengeHandler = vi.fn(async () => undefined)
    const unregister = registerMFAChallengeHandler(challengeHandler)
    vi.stubGlobal('fetch', fetchMock)

    try {
      await expect(request('/secrets', { method: 'POST' })).resolves.toEqual({ ok: true })
      expect(challengeHandler).toHaveBeenCalledOnce()
      expect(challengeHandler).toHaveBeenCalledWith({ purpose: 'secret_update' })
      expect(fetchMock).toHaveBeenCalledTimes(2)
    }
    finally {
      unregister()
    }
  })

  it('opens a challenge and retries password updates', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: 'mfa_required', purpose: 'password_update' }, 403))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    const challengeHandler = vi.fn(async () => undefined)
    const unregister = registerMFAChallengeHandler(challengeHandler)
    vi.stubGlobal('fetch', fetchMock)

    try {
      await expect(request('/users/me/password', { method: 'PUT' })).resolves.toBeUndefined()
      expect(challengeHandler).toHaveBeenCalledWith({ purpose: 'password_update' })
      expect(fetchMock).toHaveBeenCalledTimes(2)
    }
    finally {
      unregister()
    }
  })

  it('does not discard a stable purpose added by a newer backend', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ code: 'mfa_required', purpose: 'future_sensitive_action' }, 403))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    const challengeHandler = vi.fn(async () => undefined)
    const unregister = registerMFAChallengeHandler(challengeHandler)
    vi.stubGlobal('fetch', fetchMock)

    try {
      await expect(request('/future-sensitive-action', { method: 'POST' })).resolves.toBeUndefined()
      expect(challengeHandler).toHaveBeenCalledWith({ purpose: 'future_sensitive_action' })
      expect(fetchMock).toHaveBeenCalledTimes(2)
    }
    finally {
      unregister()
    }
  })

  it('does not start another challenge when the single retry still requires MFA', async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({ code: 'mfa_required', purpose: 'secret_update' }, 403))
    const challengeHandler = vi.fn(async () => undefined)
    const unregister = registerMFAChallengeHandler(challengeHandler)
    vi.stubGlobal('fetch', fetchMock)

    try {
      await expect(request('/secrets', { method: 'POST' })).rejects.toMatchObject({ code: 'mfa_required' })
      expect(challengeHandler).toHaveBeenCalledOnce()
      expect(fetchMock).toHaveBeenCalledTimes(2)
    }
    finally {
      unregister()
    }
  })

  it('coalesces concurrent challenges with the same purpose', async () => {
    const gate = deferred()
    const attempts = new Map<string, number>()
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      const path = String(input)
      const attempt = (attempts.get(path) ?? 0) + 1
      attempts.set(path, attempt)
      return attempt === 1
        ? jsonResponse({ code: 'mfa_required', purpose: 'runtime_exec' }, 403)
        : jsonResponse({ path })
    })
    const challengeHandler = vi.fn(() => gate.promise)
    const unregister = registerMFAChallengeHandler(challengeHandler)
    vi.stubGlobal('fetch', fetchMock)

    try {
      const requests = [request('/runtime/one'), request('/runtime/two')]
      await vi.waitFor(() => expect(challengeHandler).toHaveBeenCalledOnce())
      gate.resolve()

      await expect(Promise.all(requests)).resolves.toHaveLength(2)
      expect(fetchMock).toHaveBeenCalledTimes(4)
    }
    finally {
      unregister()
    }
  })

  it('sequences concurrent challenges with different purposes', async () => {
    const firstGate = deferred()
    const secondGate = deferred()
    const attempts = new Map<string, number>()
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      const path = String(input)
      const attempt = (attempts.get(path) ?? 0) + 1
      attempts.set(path, attempt)
      if (attempt > 1)
        return jsonResponse({ path })
      return path.endsWith('/exports')
        ? jsonResponse({ code: 'mfa_required', purpose: 'volume_export' }, 403)
        : jsonResponse({ code: 'mfa_required', purpose: 'secret_update' }, 403)
    })
    const purposes: string[] = []
    const challengeHandler = vi.fn((challenge: MFAChallenge) => {
      purposes.push(challenge.purpose)
      return challenge.purpose === 'volume_export' ? firstGate.promise : secondGate.promise
    })
    const unregister = registerMFAChallengeHandler(challengeHandler)
    vi.stubGlobal('fetch', fetchMock)

    try {
      const requests = [request('/exports'), request('/secrets')]
      await vi.waitFor(() => expect(purposes).toEqual(['volume_export']))
      firstGate.resolve()
      await vi.waitFor(() => expect(purposes).toEqual(['volume_export', 'secret_update']))
      secondGate.resolve()

      await expect(Promise.all(requests)).resolves.toHaveLength(2)
      expect(fetchMock).toHaveBeenCalledTimes(4)
    }
    finally {
      unregister()
    }
  })
})

describe('api error boundary', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('prefers the safe public error over development-only detail', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({
      code: 'provider.future_failure',
      detail: 'pq: relation secrets does not exist at /srv/luna/internal/provider/client.go',
      error: 'The service is temporarily unavailable.',
      requestId: 'req_safe_error',
    }, 500))
    vi.stubGlobal('fetch', fetchMock)

    const error = await request('/failure').catch((requestError: unknown) => requestError)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      code: 'provider.future_failure',
      detail: 'pq: relation secrets does not exist at /srv/luna/internal/provider/client.go',
      message: 'The service is temporarily unavailable.',
      requestId: 'req_safe_error',
      status: 500,
    })
  })

  it('localizes a stable build precondition instead of showing a generic conflict', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({
      code: 'build.registry_push_credential_required',
      error: 'The resource state conflicts with this request.',
      requestId: 'req_build_credential',
    }, 409))
    vi.stubGlobal('fetch', fetchMock)

    const error = await request('/projects/project/build-runs/trigger').catch((requestError: unknown) => requestError)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      code: 'build.registry_push_credential_required',
      message: i18next.t('errors.build.registry_push_credential_required'),
      requestId: 'req_build_credential',
      status: 409,
    })
  })

  it('does not present a non-JSON proxy response as a user-facing error', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(
      'dial tcp http://internal-provider.local: connection refused',
      { status: 502, headers: { 'Content-Type': 'text/plain' } },
    ))
    vi.stubGlobal('fetch', fetchMock)

    const error = await request('/failure').catch((requestError: unknown) => requestError)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({
      code: 'http.502',
      detail: 'dial tcp http://internal-provider.local: connection refused',
      status: 502,
    })
    expect((error as ApiError).message).not.toContain('internal-provider.local')
  })
})
