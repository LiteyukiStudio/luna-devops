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

  it('uses the operation-scoped approval and exemption contracts', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(jsonResponse({ items: [{ operationId: 'deleteApplication', createdAt: '2026-08-20T00:00:00Z' }] }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(api.listAIToolApprovalExemptions()).resolves.toMatchObject({
      items: [{ operationId: 'deleteApplication' }],
    })
    await api.decideAIToolApproval('run/1', 'tool/1', { decision: 'approve_always', reason: 'confirmed' })
    await api.revokeAIToolApprovalExemption('delete/Application')

    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/ai/tool-approval-exemptions')
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ decision: 'approve_always', reason: 'confirmed' }),
    })
    expect(String(fetchMock.mock.calls[1]?.[0])).toContain('/ai/runs/run%2F1/approvals/tool%2F1/decision')
    expect(fetchMock.mock.calls[2]?.[1]).toMatchObject({ method: 'DELETE' })
    expect(String(fetchMock.mock.calls[2]?.[0])).toContain('/ai/tool-approval-exemptions/delete%2FApplication')
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
