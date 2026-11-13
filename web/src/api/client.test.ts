import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'
import { dashboardApi } from './domains/dashboard'

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    status: 200,
  })
}

describe('api client', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('unwraps the bounded selector page', async () => {
    const project = { id: 'project-1', name: 'Project 1' }
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({
      items: [project],
      page: 1,
      pageSize: 100,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.listProjects()).resolves.toEqual([project])
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/projects?page=1&pageSize=100')
  })

  it('sends one visibility parameter across discovery APIs', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({
      items: [],
      page: 1,
      pageSize: 100,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await api.listProjects('all')
    await api.getDashboard('all')
    await api.listPlatformEvents({ page: 1, pageSize: 20, visibility: 'all' })
    await api.listRuntimeClusters(undefined, 'all')
    await api.listRuntimeClusterResourcesPage('cluster-1', { resourceCategory: 'workloads', page: 1, pageSize: 20, visibility: 'all' })
    await api.listGitProviders(undefined, 'all')
    await api.listGitAccounts(undefined, 'all')
    await api.listRegistries(undefined, 'all')
    await api.listRegistryCredentialsPage('registry-1', { page: 1, pageSize: 20, visibility: 'all' })
    await api.listAllRegistryCredentialsPage({ page: 1, pageSize: 20, visibility: 'all' })
    await api.listContainerImages({ page: 1, pageSize: 20, visibility: 'all' })
    await api.listBuildVariableSets(undefined, 'all')
    await api.searchPlatformResources({ q: 'alpha', visibility: 'all' })

    expect(fetchMock).toHaveBeenCalledTimes(13)
    for (const [input] of fetchMock.mock.calls) {
      const url = new URL(String(input), 'https://luna.devops.local')
      expect(url.searchParams.get('visibility'), url.pathname).toBe('all')
      expect(url.searchParams.has('scope'), url.pathname).toBe(false)
    }
  })

  it('exposes domain methods without a proxy wrapper', () => {
    expect(api.getDashboard).toBe(dashboardApi.getDashboard)
  })

  it('requests API metadata without caching', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({ serverVersion: 'commit-1' }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.getAPIMeta()).resolves.toEqual({ serverVersion: 'commit-1' })
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/meta'),
      expect.objectContaining({ cache: 'no-store' }),
    )
  })

  it('uses the operation-scoped approval contract', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await api.decideAIToolApproval('run/1', 'tool/1', { decision: 'approve' })

    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ decision: 'approve' }),
    })
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/ai/runs/run%2F1/approvals/tool%2F1/decision')
  })

  it('keeps application detail aggregation bounded and server-filtered', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({
      items: [],
      page: 1,
      pageSize: 100,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 0,
      totalPages: 0,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await Promise.all([
      api.listRepositoryBindings('project-1', 'app-1'),
      api.listBuildRuns('project-1', 'app-1'),
      api.listBuildJobs('project-1', undefined, 'app-1'),
      api.listReleases('project-1', 'app-1'),
      api.listGatewayRoutes('project-1', 'app-1'),
    ])

    expect(fetchMock).toHaveBeenCalledTimes(5)
    for (const [input] of fetchMock.mock.calls) {
      const url = String(input)
      expect(url).toContain('page=1')
      expect(url).toContain('pageSize=100')
      expect(url).toContain('applicationId=app-1')
    }
  })

  it('requests runtime pods in the recommended order for the selected release', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({ items: [], page: 1, pageSize: 100, sortBy: 'recommended', sortOrder: 'desc', total: 0, totalPages: 0, status: 'ready', observationCode: 'deployment_target.runtime_pods_ready', observedAt: '2026-09-06T00:00:00Z' }))
    vi.stubGlobal('fetch', fetchMock)

    await api.listDeploymentTargetRuntimePods('project-1', 'app-1', 'target-1', { releaseId: 'rel-1', search: ' api ' })
    await api.getReleaseRuntimeLogs('project-1', 'rel-1', { pod: 'api-a', tailLines: 500 })

    const podsUrl = new URL(String(fetchMock.mock.calls[0]?.[0]), window.location.origin)
    expect(podsUrl.pathname).toMatch(/\/projects\/project-1\/applications\/app-1\/deployment-targets\/target-1\/pods$/)
    expect(Object.fromEntries(podsUrl.searchParams)).toEqual({ page: '1', pageSize: '100', sortBy: 'recommended', search: 'api', releaseId: 'rel-1' })
    expect(String(fetchMock.mock.calls[1]?.[0])).toContain('/releases/rel-1/runtime-logs?pod=api-a&tailLines=500')
  })

  it('supports deployment target events and previous crash log parameters', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse({ items: [], page: 1, pageSize: 20, sortBy: 'lastSeen', sortOrder: 'desc', total: 0, totalPages: 0, status: 'ready', observationCode: 'deployment_target.events_ready', observedAt: '2026-09-06T00:00:00Z' }))
    vi.stubGlobal('fetch', fetchMock)

    await api.listDeploymentTargetEvents('project-1', 'app-1', 'target-1', { pageSize: 20, type: 'Warning' })
    await api.getReleaseRuntimeLogs('project-1', 'rel-1', { pod: 'api-a', previous: true, timestamps: true, tailLines: 200 })
    await api.getDeploymentTargetRuntimeLogs('project-1', 'app-1', 'target-1', { preferAbnormal: true, previous: true, tailLines: 200 })

    const eventsUrl = new URL(String(fetchMock.mock.calls[0]?.[0]), window.location.origin)
    expect(eventsUrl.pathname).toMatch(/\/projects\/project-1\/applications\/app-1\/deployment-targets\/target-1\/events$/)
    expect(Object.fromEntries(eventsUrl.searchParams)).toEqual({ page: '1', pageSize: '20', sortBy: 'lastSeen', sortOrder: 'desc', type: 'Warning' })
    expect(String(fetchMock.mock.calls[1]?.[0])).toContain('/releases/rel-1/runtime-logs?pod=api-a&tailLines=200&previous=true&timestamps=true')
    expect(String(fetchMock.mock.calls[2]?.[0])).toContain('/deployment-targets/target-1/runtime-logs?tailLines=200&previous=true&preferAbnormal=true')
  })
})
