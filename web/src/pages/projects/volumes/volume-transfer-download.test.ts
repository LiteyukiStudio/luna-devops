import { describe, expect, it, vi } from 'vitest'
import { startNativeVolumeTransferDownload } from './volume-transfer-download'

describe('native volume transfer download', () => {
  it('places the one-time ticket on the native download URL without a preflight request', async () => {
    const calls: string[] = []
    const triggerDownload = vi.fn()

    await startNativeVolumeTransferDownload({
      filename: 'database.raw.zst',
      projectId: 'prj_1',
      resource: 'content',
      transferId: 'vtx_1',
    }, {
      authorize: async () => {
        calls.push('authorize')
        return { ticket: 'ticket_1' }
      },
      resourceURL: (_projectId, _transferId, resource, ticket) => {
        calls.push(`url:${resource}:${ticket}`)
        calls.push('url')
        return `/api/v1/projects/prj_1/volume-transfers/vtx_1/content?ticket=${ticket}`
      },
      triggerDownload: (url, filename) => {
        calls.push('download')
        triggerDownload(url, filename)
      },
    })

    expect(calls).toEqual(['authorize', 'url:content:ticket_1', 'url', 'download'])
    expect(triggerDownload).toHaveBeenCalledWith(
      '/api/v1/projects/prj_1/volume-transfers/vtx_1/content?ticket=ticket_1',
      'database.raw.zst',
    )
  })

  it('uses the same single-request authorization for a block manifest', async () => {
    const triggerDownload = vi.fn()

    await startNativeVolumeTransferDownload({
      filename: 'database.raw.zst.manifest.json',
      projectId: 'prj_1',
      resource: 'manifest',
      transferId: 'vtx_1',
    }, {
      authorize: vi.fn().mockResolvedValue({ ticket: 'ticket_2' }),
      resourceURL: (_projectId, _transferId, _resource, ticket) => `/api/v1/projects/prj_1/volume-transfers/vtx_1/manifest?ticket=${ticket}`,
      triggerDownload,
    })

    expect(triggerDownload).toHaveBeenCalledWith(
      '/api/v1/projects/prj_1/volume-transfers/vtx_1/manifest?ticket=ticket_2',
      'database.raw.zst.manifest.json',
    )
  })
})
