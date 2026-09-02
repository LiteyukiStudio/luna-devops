import { afterEach, describe, expect, it, vi } from 'vitest'
import { kubectlApi } from './kubectl'

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('kubectl API contract', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('creates one-time kubeconfig credentials without HTTP cache', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ credential: { id: 'tok_1' }, bindings: [], kubeconfig: 'apiVersion: v1' }))
    vi.stubGlobal('fetch', fetchMock)

    await kubectlApi.createKubeCredential({
      name: 'dev access',
      expiresInDays: 7,
      scopes: ['kube:read'],
      contexts: [{ projectId: 'prj_1', runtimeClusterId: 'clu_1' }],
    })

    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: 'POST',
      cache: 'no-store',
    })
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/kube-credentials')
  })

  it('keeps kube credential lists server-filtered and paginated', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({
      items: [],
      page: 2,
      pageSize: 20,
      sortBy: 'expiresAt',
      sortOrder: 'asc',
      total: 0,
      totalPages: 0,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await kubectlApi.listKubeCredentials({
      page: 2,
      pageSize: 20,
      search: 'dev',
      sortBy: 'expiresAt',
      sortOrder: 'asc',
      status: 'active',
    })

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost')
    expect(url.pathname).toContain('/kube-credentials')
    expect(Object.fromEntries(url.searchParams)).toMatchObject({
      page: '2',
      pageSize: '20',
      search: 'dev',
      sortBy: 'expiresAt',
      sortOrder: 'asc',
      status: 'active',
    })
  })

  it('requests gateway status without browser caching and updates it with a full replacement PUT', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ enabled: true, extraResourceRules: [], status: 'ready', observationCode: '' }))
      .mockResolvedValueOnce(jsonResponse({ enabled: true, extraResourceRules: [], status: 'reconciling', observationCode: '' }, 202))
    vi.stubGlobal('fetch', fetchMock)

    await kubectlApi.getRuntimeClusterKubeGateway('cluster/1')
    await kubectlApi.updateRuntimeClusterKubeGateway('cluster/1', {
      enabled: true,
      extraResourceRules: [],
    })

    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ cache: 'no-store' })
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/runtime/clusters/cluster%2F1/kube-gateway')
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({
      method: 'PUT',
      body: JSON.stringify({ enabled: true, extraResourceRules: [] }),
    })
  })

  it('normalizes nullable gateway rule arrays at the API boundary', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ enabled: false, extraResourceRules: null, status: 'disabled', observationCode: '' }))
      .mockResolvedValueOnce(jsonResponse({
        enabled: true,
        extraResourceRules: [{
          action: 'project:read',
          apiGroup: 'example.io',
          apiVersion: 'v1',
          resource: 'widgets',
          verbs: ['get'],
        }],
        status: 'reconciling',
        observationCode: '',
      }, 202))
    vi.stubGlobal('fetch', fetchMock)

    const emptyGateway = await kubectlApi.getRuntimeClusterKubeGateway('clu_empty')
    const gatewayWithoutSubresources = await kubectlApi.updateRuntimeClusterKubeGateway('clu_rule', {
      enabled: true,
      extraResourceRules: [],
    })

    expect(emptyGateway.extraResourceRules).toEqual([])
    expect(gatewayWithoutSubresources.extraResourceRules[0]?.subresources).toEqual([])
  })
})
