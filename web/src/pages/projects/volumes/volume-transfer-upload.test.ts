import type { VolumeImportResumeRecord } from '@/api'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api'
import {
  clearVolumeImportResumeRecord,
  fileMatchesResumeRecord,
  readVolumeImportResumeRecord,
  sha256BlobBase64,
  sha256File,
  uploadVolumeImportWithApi,
  writeVolumeImportResumeRecord,
} from './volume-transfer-upload'

describe('project volume import resume state', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('hashes archive content incrementally', async () => {
    const file = new Blob([new TextEncoder().encode('abc')])
    await expect(sha256File(file)).resolves.toBe('ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  })

  it('hashes an upload part in bounded slices without reading the whole Blob at once', async () => {
    const blob = new Blob([new Uint8Array(8 * 1024 * 1024 + 1).fill(0x61)])
    Object.defineProperty(blob, 'arrayBuffer', {
      value: () => Promise.reject(new Error('whole blob read is forbidden')),
    })
    const expected = 'ySaX9Mw7Vp3/PUhChdIkh+Uj1LQ5rnyaZ0fcJY41snU='

    await expect(sha256BlobBase64(blob)).resolves.toBe(expected)
  })

  it('persists only resumable file metadata', () => {
    const record: VolumeImportResumeRecord = {
      projectId: 'project-1',
      transferId: 'vtx_1',
      volumeId: 'pvol_1',
      filename: 'backup.tar.gz',
      size: 42,
      lastModified: 123,
      sha256: 'a'.repeat(64),
      format: 'tar_gz',
      createdAt: '2026-08-15T00:00:00Z',
    }
    writeVolumeImportResumeRecord(record)

    expect(readVolumeImportResumeRecord('project-1')).toEqual(record)
    const stored = localStorage.getItem('luna.project-volume-import.v1.project-1') ?? ''
    expect(stored).not.toMatch(/token|cookie|authorization/i)

    clearVolumeImportResumeRecord('project-1', 'vtx_1')
    expect(readVolumeImportResumeRecord('project-1')).toBeNull()
  })

  it('ignores malformed browser resume metadata', () => {
    localStorage.setItem('luna.project-volume-import.v1.project-1', JSON.stringify({ projectId: 'project-1', transferId: 'other', filename: 'backup.tar.gz', size: 42 }))
    expect(readVolumeImportResumeRecord('project-1')).toBeNull()
  })

  it('requires the exact same local file before resuming', () => {
    const record = {
      projectId: 'project-1',
      transferId: 'vtx_1',
      volumeId: 'pvol_1',
      filename: 'backup.tar.gz',
      size: 3,
      lastModified: 123,
      sha256: 'abc',
      format: 'tar_gz' as const,
      createdAt: '2026-08-15T00:00:00Z',
    }
    expect(fileMatchesResumeRecord(new File(['abc'], 'backup.tar.gz', { lastModified: 123 }), record)).toBe(true)
    expect(fileMatchesResumeRecord(new File(['abcd'], 'backup.tar.gz', { lastModified: 123 }), record)).toBe(false)
  })

  it('waits, HEADs, and retries a part that another request is still persisting', async () => {
    const file = new File(['abc'], 'backup.tar.gz')
    const getVolumeImportUploadOffset = vi.fn()
      .mockResolvedValueOnce({ chunkSize: 64 * 1024 * 1024, length: file.size, offset: 0 })
      .mockResolvedValueOnce({ chunkSize: 64 * 1024 * 1024, length: file.size, offset: 0 })
    const uploadVolumeImportChunk = vi.fn()
      .mockRejectedValueOnce(new ApiError('in progress', {
        code: 'volume_transfer.part_in_progress',
        path: '/volume-imports/content',
        retryAfterMs: 1,
        status: 409,
      }))
      .mockResolvedValueOnce({ chunkSize: 64 * 1024 * 1024, offset: file.size })
    const completeVolumeImportUpload = vi.fn().mockResolvedValue({ id: 'vtx_1' })

    await expect(uploadVolumeImportWithApi({
      file,
      projectId: 'project-1',
      sha256: 'a'.repeat(64),
      transfer: { id: 'vtx_1', chunkSize: 64 * 1024 * 1024 } as never,
    }, { completeVolumeImportUpload, getVolumeImportUploadOffset, uploadVolumeImportChunk } as never)).resolves.toMatchObject({ id: 'vtx_1' })

    expect(getVolumeImportUploadOffset).toHaveBeenCalledTimes(2)
    expect(uploadVolumeImportChunk).toHaveBeenCalledTimes(2)
  })

  it('cancels an in-progress part wait without issuing another HEAD or PATCH', async () => {
    const file = new File(['abc'], 'backup.tar.gz')
    const controller = new AbortController()
    const getVolumeImportUploadOffset = vi.fn()
      .mockResolvedValue({ chunkSize: 64 * 1024 * 1024, length: file.size, offset: 0 })
    const uploadVolumeImportChunk = vi.fn().mockRejectedValue(new ApiError('in progress', {
      code: 'volume_transfer.part_in_progress',
      path: '/volume-imports/content',
      retryAfterMs: 10_000,
      status: 409,
    }))
    const upload = uploadVolumeImportWithApi({
      file,
      projectId: 'project-1',
      sha256: 'a'.repeat(64),
      signal: controller.signal,
      transfer: { id: 'vtx_1', chunkSize: 64 * 1024 * 1024 } as never,
    }, {
      completeVolumeImportUpload: vi.fn(),
      getVolumeImportUploadOffset,
      uploadVolumeImportChunk,
    } as never)
    await vi.waitFor(() => expect(uploadVolumeImportChunk).toHaveBeenCalledTimes(1))
    controller.abort()

    await expect(upload).rejects.toMatchObject({ name: 'AbortError' })
    expect(getVolumeImportUploadOffset).toHaveBeenCalledTimes(1)
    expect(uploadVolumeImportChunk).toHaveBeenCalledTimes(1)
  })
})
