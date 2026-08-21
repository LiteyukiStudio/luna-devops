import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { ApiError, request, runtimeClusterResourceListQuery } from './core'

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

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

  it.each([
    'billing.wallet_unavailable',
    'ai.model_context_limit_invalid',
    'ai.model_context_insufficient',
    'ai.model_output_limit_invalid',
    'ai.wallet_balance_insufficient',
  ])('localizes the stable model execution code %s', async (code) => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ code, error: 'generic backend error' }, 422))
    vi.stubGlobal('fetch', fetchMock)

    const error = await request('/ai/model').catch((requestError: unknown) => requestError)

    expect(error).toMatchObject({ code, message: i18next.t(`errors.${code}`) })
  })

  it.each([
    'cluster.resource_category_invalid',
    'cluster.resource_kind_invalid',
    'cluster.resource_name_required',
    'runtime_cluster.forbidden',
  ])('localizes the runtime resource contract code %s', async (code) => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ code, error: 'generic backend error' }, 400))
    vi.stubGlobal('fetch', fetchMock)

    const error = await request('/runtime/clusters/cluster/resources').catch((requestError: unknown) => requestError)

    expect(error).toMatchObject({ code, message: i18next.t(`errors.${code}`) })
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

describe('runtime resource query contract', () => {
  it('uses resourceCategory for collection queries and does not emit the ambiguous kind key', () => {
    const query = new URLSearchParams(runtimeClusterResourceListQuery({
      resourceCategory: 'workloads',
      page: 1,
      pageSize: 20,
      projectId: 'project-1',
    }))

    expect(query.get('resourceCategory')).toBe('workloads')
    expect(query.has('kind')).toBe(false)
    expect(query.get('projectId')).toBe('project-1')
  })
})
