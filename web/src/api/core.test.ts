import type { MFAChallenge, MFAPurpose } from './types'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { registerMFAChallengeHandler, request } from './core'

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
        ? jsonResponse({ code: 'mfa_required', purpose: 'data_export' }, 403)
        : jsonResponse({ code: 'mfa_required', purpose: 'secret_update' }, 403)
    })
    const purposes: MFAPurpose[] = []
    const challengeHandler = vi.fn((challenge: MFAChallenge) => {
      purposes.push(challenge.purpose)
      return challenge.purpose === 'data_export' ? firstGate.promise : secondGate.promise
    })
    const unregister = registerMFAChallengeHandler(challengeHandler)
    vi.stubGlobal('fetch', fetchMock)

    try {
      const requests = [request('/exports'), request('/secrets')]
      await vi.waitFor(() => expect(purposes).toEqual(['data_export']))
      firstGate.resolve()
      await vi.waitFor(() => expect(purposes).toEqual(['data_export', 'secret_update']))
      secondGate.resolve()

      await expect(Promise.all(requests)).resolves.toHaveLength(2)
      expect(fetchMock).toHaveBeenCalledTimes(4)
    }
    finally {
      unregister()
    }
  })
})
