import { afterEach, describe, expect, it, vi } from 'vitest'
import { sha256File, uploadVolumeImportWithApi, waitForVolumeTransferReadyWithApi } from './volume-transfer-upload'

describe('direct project volume import', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('hashes archive content incrementally', async () => {
    const file = new Blob([new TextEncoder().encode('abc')])
    await expect(sha256File(file)).resolves.toBe('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  })

  it('hashes a large archive in bounded slices without reading the whole Blob at once', async () => {
    const blob = new Blob([new Uint8Array(8 * 1024 * 1024 + 1).fill(0x61)])
    Object.defineProperty(blob, 'arrayBuffer', {
      value: () => Promise.reject(new Error('whole blob read is forbidden')),
    })
    await expect(sha256File(blob)).resolves.toBe('c92697f4cc3b569dff3d484285d22487e523d4b439ae7c9a6747dc258e35b275')
  })

  it('waits for the worker-prepared transfer before opening the content stream', async () => {
    const getVolumeTransfer = vi.fn()
      .mockResolvedValueOnce({ id: 'vtx_1', state: 'created' })
      .mockResolvedValueOnce({ id: 'vtx_1', state: 'preparing' })
      .mockResolvedValueOnce({ id: 'vtx_1', state: 'ready' })

    await expect(waitForVolumeTransferReadyWithApi({
      pollIntervalMs: 0,
      projectId: 'project-1',
      transferId: 'vtx_1',
    }, { getVolumeTransfer } as never)).resolves.toMatchObject({ state: 'ready' })

    expect(getVolumeTransfer).toHaveBeenCalledTimes(3)
  })

  it.each(['failed', 'cancelled', 'expired'] as const)('stops waiting when preparation becomes %s', async (state) => {
    const getVolumeTransfer = vi.fn().mockResolvedValue({ id: 'vtx_1', state, lastErrorCode: `volume_transfer.${state}` })

    await expect(waitForVolumeTransferReadyWithApi({
      pollIntervalMs: 0,
      projectId: 'project-1',
      transferId: 'vtx_1',
    }, { getVolumeTransfer } as never)).rejects.toMatchObject({ code: `volume_transfer.${state}` })
  })

  it('cancels a readiness wait without polling again', async () => {
    const controller = new AbortController()
    const getVolumeTransfer = vi.fn().mockResolvedValue({ id: 'vtx_1', state: 'preparing' })
    const waiting = waitForVolumeTransferReadyWithApi({
      pollIntervalMs: 10_000,
      projectId: 'project-1',
      signal: controller.signal,
      transferId: 'vtx_1',
    }, { getVolumeTransfer } as never)
    await vi.waitFor(() => expect(getVolumeTransfer).toHaveBeenCalledTimes(1))
    controller.abort()

    await expect(waiting).rejects.toMatchObject({ name: 'AbortError' })
    expect(getVolumeTransfer).toHaveBeenCalledTimes(1)
  })

  it('uploads the complete File exactly once and reports transport progress', async () => {
    const file = new File(['archive'], 'backup.tar.gz')
    const onProgress = vi.fn()
    const uploadVolumeImportContent = vi.fn(async (_projectId, _transferId, uploadedFile: File, _sha256, _signal, progress: ((transferred: number, total: number) => void) | undefined) => {
      progress?.(uploadedFile.size, uploadedFile.size)
      return { id: 'vtx_1', state: 'succeeded' }
    })

    await expect(uploadVolumeImportWithApi({
      file,
      onProgress,
      projectId: 'project-1',
      sha256: 'a'.repeat(64),
      transferId: 'vtx_1',
    }, { uploadVolumeImportContent } as never)).resolves.toMatchObject({ state: 'succeeded' })

    expect(uploadVolumeImportContent).toHaveBeenCalledTimes(1)
    expect(uploadVolumeImportContent).toHaveBeenCalledWith('project-1', 'vtx_1', file, 'a'.repeat(64), undefined, expect.any(Function))
    expect(onProgress).toHaveBeenCalledWith({ percent: 100, total: file.size, transferredBytes: file.size })
  })
})
