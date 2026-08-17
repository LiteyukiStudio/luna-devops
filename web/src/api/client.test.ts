import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { 'Content-Type': 'application/json' },
    status: 200,
  })
}

describe('lazy API client', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('loads the owning domain and unwraps the bounded selector page', async () => {
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

  it('returns a stable wrapper for query and mutation callbacks', () => {
    expect(api.getDashboard).toBe(api.getDashboard)
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
