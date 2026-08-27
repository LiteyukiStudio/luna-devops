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
})
