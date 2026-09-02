import { afterEach, describe, expect, it, vi } from 'vitest'
import { runtimeApi } from './runtime'

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('runtime API contract', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('requests batched kubectl gateway statuses with repeated cluster IDs and no browser cache', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ items: [] }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await runtimeApi.observeRuntimeClusterKubeGatewayStatuses(['clu_alpha', 'clu_beta'])

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost')
    expect(result).toEqual([])
    expect(url.pathname).toContain('/runtime/clusters/kube-gateway-status')
    expect(url.searchParams.getAll('clusterId')).toEqual(['clu_alpha', 'clu_beta'])
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ cache: 'no-store' })
  })
})
