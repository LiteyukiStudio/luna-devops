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

  it('streams one complete import file with a single raw PUT', async () => {
    const requests: FakeUploadRequest[] = []
    class FakeUploadRequest extends EventTarget {
      body?: File
      headers = new Headers()
      method = ''
      response: unknown = { id: 'vtx_1', state: 'succeeded' }
      responseType: XMLHttpRequestResponseType = ''
      status = 200
      statusText = 'OK'
      upload = new EventTarget()
      url = ''
      withCredentials = false

      constructor() {
        super()
        requests.push(this)
      }

      open(method: string, url: string) {
        this.method = method
        this.url = url
      }

      setRequestHeader(name: string, value: string) {
        this.headers.set(name, value)
      }

      send(body: File) {
        this.body = body
        this.upload.dispatchEvent(new ProgressEvent('progress', { loaded: body.size, total: body.size }))
        this.dispatchEvent(new Event('load'))
      }

      abort() {
        this.dispatchEvent(new Event('abort'))
      }
    }
    vi.stubGlobal('XMLHttpRequest', FakeUploadRequest)
    const file = new File(['archive'], 'backup.tar.gz')
    const onProgress = vi.fn()

    await expect(volumesApi.uploadVolumeImportContent('project-1', 'vtx_1', file, 'a'.repeat(64), undefined, onProgress)).resolves.toMatchObject({ state: 'succeeded' })

    expect(requests[0]).toMatchObject({ body: file, method: 'PUT', withCredentials: true })
    expect(requests[0]?.url).toContain('/projects/project-1/volume-imports/vtx_1/content')
    expect(requests[0]?.headers.get('Content-Type')).toBe('application/octet-stream')
    expect(requests[0]?.headers.get('X-Content-SHA256')).toBe('a'.repeat(64))
    expect(onProgress).toHaveBeenCalledWith(file.size, file.size)
  })

  it('builds single-use ticket URLs without a resumable session exchange', () => {
    const contentURL = new URL(volumesApi.volumeTransferContentURL('project-1', 'vtx_1', 'ticket-1'), 'http://localhost')
    const manifestURL = new URL(volumesApi.volumeTransferManifestURL('project-1', 'vtx_1', 'ticket-2'), 'http://localhost')

    expect(contentURL.pathname).toContain('/volume-transfers/vtx_1/content')
    expect(contentURL.searchParams.get('ticket')).toBe('ticket-1')
    expect(manifestURL.pathname).toContain('/volume-transfers/vtx_1/manifest')
    expect(manifestURL.searchParams.get('ticket')).toBe('ticket-2')
    expect(contentURL.searchParams.has('offset')).toBe(false)
  })
})
