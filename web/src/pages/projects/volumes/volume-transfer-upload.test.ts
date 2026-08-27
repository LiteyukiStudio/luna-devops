import { afterEach, describe, expect, it, vi } from 'vitest'
import { uploadVolumeImportWithApi, waitForVolumeTransferReadyWithApi } from './volume-transfer-upload'

describe('direct project volume import', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
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
    const uploadVolumeImportContent = vi.fn(async (_projectId, _transferId, uploadedFile: File, _signal, progress: ((transferred: number, total: number) => void) | undefined) => {
      progress?.(uploadedFile.size, uploadedFile.size)
      return { id: 'vtx_1', state: 'succeeded' }
    })

    await expect(uploadVolumeImportWithApi({
      file,
      onProgress,
      projectId: 'project-1',
      transferId: 'vtx_1',
    }, { uploadVolumeImportContent } as never)).resolves.toMatchObject({ state: 'succeeded' })

    expect(uploadVolumeImportContent).toHaveBeenCalledTimes(1)
    expect(uploadVolumeImportContent).toHaveBeenCalledWith('project-1', 'vtx_1', file, undefined, expect.any(Function))
    expect(onProgress).toHaveBeenCalledWith({ percent: 100, total: file.size, transferredBytes: file.size })
  })
})
