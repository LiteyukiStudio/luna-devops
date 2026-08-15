import { afterEach, describe, expect, it, vi } from 'vitest'
import { volumesApi } from './volumes'

function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
}

describe('volumes API contract', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('keeps volume selection server-filtered and paginated', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({
      items: [],
      page: 2,
      pageSize: 20,
      sortBy: 'displayName',
      sortOrder: 'asc',
      total: 0,
      totalPages: 0,
    }))
    vi.stubGlobal('fetch', fetchMock)

    await volumesApi.listProjectVolumes('project/one', {
      page: 2,
      pageSize: 20,
      search: 'cache',
      clusterId: 'cluster-1',
      availability: 'available',
      sortBy: 'displayName',
      sortOrder: 'asc',
    })

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost')
    expect(url.pathname).toContain('/projects/project%2Fone/volumes')
    expect(Object.fromEntries(url.searchParams)).toMatchObject({
      page: '2',
      pageSize: '20',
      search: 'cache',
      clusterId: 'cluster-1',
      availability: 'available',
      sortBy: 'displayName',
      sortOrder: 'asc',
    })
  })

  it('sends authoritative revision for updates', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ id: 'pvol_1' }))
    vi.stubGlobal('fetch', fetchMock)

    await volumesApi.updateProjectVolume('project-1', 'pvol_1', 7, { displayName: 'cache' })

    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: 'PATCH' })
    expect(new Headers(fetchMock.mock.calls[0]?.[1]?.headers).get('If-Match')).toBe('7')
  })

  it('uses TUS headers and the server upload offset', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 200, headers: { 'Upload-Chunk-Size': String(64 * 1024 * 1024), 'Upload-Length': '20', 'Upload-Offset': '8' } }))
      .mockResolvedValueOnce(new Response(null, { status: 204, headers: { 'Upload-Chunk-Size': String(64 * 1024 * 1024), 'Upload-Offset': '12' } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(volumesApi.getVolumeImportUploadOffset('project-1', 'vtx_1')).resolves.toEqual({ chunkSize: 64 * 1024 * 1024, length: 20, offset: 8 })
    await expect(volumesApi.uploadVolumeImportChunk('project-1', 'vtx_1', new Blob(['data']), 8, 'checksum')).resolves.toEqual({ chunkSize: 64 * 1024 * 1024, offset: 12 })

    const headers = new Headers(fetchMock.mock.calls[1]?.[1]?.headers)
    expect(fetchMock.mock.calls[1]?.[1]?.method).toBe('PATCH')
    expect(headers.get('Tus-Resumable')).toBe('1.0.0')
    expect(headers.get('Upload-Offset')).toBe('8')
    expect(headers.get('Upload-Checksum')).toBe('sha256 checksum')
  })

  it('rejects a resumable upload response without the server-required chunk size', async () => {
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockResolvedValue(new Response(null, {
      status: 200,
      headers: { 'Upload-Length': '20', 'Upload-Offset': '0' },
    })))

    await expect(volumesApi.getVolumeImportUploadOffset('project-1', 'vtx_1')).rejects.toMatchObject({
      code: 'volume_transfer.response_invalid',
      status: 502,
    })
  })

  it('publishes a bounded Retry-After delay for an in-progress upload part', async () => {
    vi.stubGlobal('fetch', vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({
      code: 'volume_transfer.part_in_progress',
      error: 'part is in progress',
    }, 409, { 'Retry-After': '99' })))

    await expect(volumesApi.uploadVolumeImportChunk('project-1', 'vtx_1', new Blob(['data']), 0, 'checksum')).rejects.toMatchObject({
      code: 'volume_transfer.part_in_progress',
      retryAfterMs: 5_000,
      status: 409,
    })
  })

  it('resumes export downloads with an HTTP byte range', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(new Uint8Array([1, 2]), {
      status: 206,
      headers: { 'Content-Range': 'bytes 8-9/10' },
    }))
    vi.stubGlobal('fetch', fetchMock)

    await volumesApi.downloadVolumeTransferContent('project-1', 'vtx_1', 'ticket-1', 8)

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost')
    expect(url.searchParams.get('ticket')).toBe('ticket-1')
    expect(new Headers(fetchMock.mock.calls[0]?.[1]?.headers).get('Range')).toBe('bytes=8-')
  })

  it('uses the HttpOnly download session cookie after ticket exchange', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(new Uint8Array([1]), { status: 206 }))
    vi.stubGlobal('fetch', fetchMock)

    await volumesApi.downloadVolumeTransferContent('project-1', 'vtx_1', undefined, 9)

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost')
    expect(url.searchParams.has('ticket')).toBe(false)
    expect(fetchMock.mock.calls[0]?.[1]?.credentials).toBe('include')
    expect(new Headers(fetchMock.mock.calls[0]?.[1]?.headers).get('Range')).toBe('bytes=9-')
  })

  it('exchanges a content ticket with HEAD before a native browser download', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await volumesApi.headVolumeTransferContent('project-1', 'vtx_1', 'ticket-1')

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost')
    expect(url.pathname).toContain('/projects/project-1/volume-transfers/vtx_1/content')
    expect(url.searchParams.get('ticket')).toBe('ticket-1')
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ credentials: 'include', method: 'HEAD' })
    expect(volumesApi.volumeTransferContentURL('project-1', 'vtx_1')).not.toContain('ticket')
  })

  it('uses the shared download authorization protocol for the Block manifest', async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 200 }))
      .mockResolvedValueOnce(jsonResponse({ schemaVersion: 1 }))
    vi.stubGlobal('fetch', fetchMock)

    await volumesApi.headVolumeTransferManifest('project-1', 'vtx_1', 'ticket-2')
    await volumesApi.downloadVolumeTransferManifest('project-1', 'vtx_1')

    const headURL = new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost')
    const getURL = new URL(String(fetchMock.mock.calls[1]?.[0]), 'http://localhost')
    expect(headURL.pathname).toContain('/volume-transfers/vtx_1/manifest')
    expect(headURL.searchParams.get('ticket')).toBe('ticket-2')
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('HEAD')
    expect(getURL.searchParams.has('ticket')).toBe(false)
    expect(fetchMock.mock.calls[1]?.[1]?.method).toBe('GET')
    expect(volumesApi.volumeTransferManifestURL('project-1', 'vtx_1')).not.toContain('ticket')
  })
})
