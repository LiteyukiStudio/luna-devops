import { describe, expect, it, vi } from 'vitest'
import { startNativeVolumeTransferDownload, verifyVolumeTransferDownload } from './volume-transfer-download'

describe('native volume transfer download', () => {
  it('exchanges the ticket with HEAD before starting a cookie-authenticated native download', async () => {
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
      exchangeSession: async (_projectId, _transferId, resource, ticket) => {
        calls.push(`head:${resource}:${ticket}`)
      },
      resourceURL: () => {
        calls.push('url')
        return '/api/v1/projects/prj_1/volume-transfers/vtx_1/content'
      },
      triggerDownload: (url, filename) => {
        calls.push('download')
        triggerDownload(url, filename)
      },
    })

    expect(calls).toEqual(['authorize', 'head:content:ticket_1', 'url', 'download'])
    expect(triggerDownload).toHaveBeenCalledWith(
      '/api/v1/projects/prj_1/volume-transfers/vtx_1/content',
      'database.raw.zst',
    )
  })

  it('uses the same ticket-session sequence for a block manifest', async () => {
    const exchangeSession = vi.fn().mockResolvedValue(undefined)
    const triggerDownload = vi.fn()

    await startNativeVolumeTransferDownload({
      filename: 'database.raw.zst.manifest.json',
      projectId: 'prj_1',
      resource: 'manifest',
      transferId: 'vtx_1',
    }, {
      authorize: vi.fn().mockResolvedValue({ ticket: 'ticket_2' }),
      exchangeSession,
      resourceURL: () => '/api/v1/projects/prj_1/volume-transfers/vtx_1/manifest',
      triggerDownload,
    })

    expect(exchangeSession).toHaveBeenCalledWith('prj_1', 'vtx_1', 'manifest', 'ticket_2')
    expect(triggerDownload).toHaveBeenCalledWith(
      '/api/v1/projects/prj_1/volume-transfers/vtx_1/manifest',
      'database.raw.zst.manifest.json',
    )
  })

  it('verifies the completed picker download against authoritative size and checksum', async () => {
    const content = new Blob(['abc'])
    await expect(verifyVolumeTransferDownload(
      content,
      content.size,
      'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
    )).resolves.toMatchObject({ status: 'verified', bytes: 3 })
    await expect(verifyVolumeTransferDownload(content, 4, 'a'.repeat(64))).resolves.toMatchObject({ status: 'length_mismatch' })
    await expect(verifyVolumeTransferDownload(content, 3, 'a'.repeat(64))).resolves.toMatchObject({ status: 'checksum_mismatch' })
    await expect(verifyVolumeTransferDownload(content, 3, '')).resolves.toEqual({ status: 'metadata_invalid' })
  })
})
