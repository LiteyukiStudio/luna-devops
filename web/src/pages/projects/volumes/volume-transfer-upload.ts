import type { VolumeTransfer } from '@/api'
import { api, ApiError } from '@/api'
import i18next from '@/i18n'

const DEFAULT_READY_POLL_INTERVAL_MS = 1_000

export interface VolumeUploadProgress {
  transferredBytes: number
  total: number
  percent: number
}

export interface VolumeUploadInput {
  file: File
  onProgress?: (progress: VolumeUploadProgress) => void
  projectId: string
  signal?: AbortSignal
  transferId: string
}

interface WaitForReadyInput {
  pollIntervalMs?: number
  projectId: string
  signal?: AbortSignal
  transferId: string
}

type VolumeUploadApi = Pick<typeof api, 'getVolumeTransfer' | 'uploadVolumeImportContent'>

function abortableDelay(milliseconds: number, signal?: AbortSignal) {
  signal?.throwIfAborted()
  return new Promise<void>((resolve, reject) => {
    let timeout: ReturnType<typeof globalThis.setTimeout>
    const onAbort = () => {
      globalThis.clearTimeout(timeout)
      reject(signal?.reason ?? new DOMException('The operation was aborted.', 'AbortError'))
    }
    timeout = globalThis.setTimeout(() => {
      signal?.removeEventListener('abort', onAbort)
      resolve()
    }, milliseconds)
    signal?.addEventListener('abort', onAbort, { once: true })
  })
}

export function uploadVolumeImport(input: VolumeUploadInput) {
  return uploadVolumeImportWithApi(input, api)
}

export function waitForVolumeTransferReady(input: WaitForReadyInput) {
  return waitForVolumeTransferReadyWithApi(input, api)
}

export async function waitForVolumeTransferReadyWithApi(
  { pollIntervalMs = DEFAULT_READY_POLL_INTERVAL_MS, projectId, signal, transferId }: WaitForReadyInput,
  transferApi: Pick<VolumeUploadApi, 'getVolumeTransfer'>,
): Promise<VolumeTransfer> {
  while (true) {
    signal?.throwIfAborted()
    const transfer = await transferApi.getVolumeTransfer(projectId, transferId, signal)
    if (transfer.state === 'ready')
      return transfer
    if (transfer.state === 'failed' || transfer.state === 'cancelled' || transfer.state === 'expired') {
      const code = transfer.lastErrorCode || `volume_transfer.${transfer.state}`
      const message = i18next.exists(`errors.${code}`) ? i18next.t(`errors.${code}`) : i18next.t('errors.request.failed')
      throw new ApiError(message, { code, path: '/volume-transfers', status: 409 })
    }
    if (transfer.state === 'streaming' || transfer.state === 'succeeded') {
      throw new ApiError(i18next.t('errors.request.failed'), {
        code: 'volume_transfer.invalid_state',
        path: '/volume-transfers',
        status: 409,
      })
    }
    await abortableDelay(pollIntervalMs, signal)
  }
}

export function uploadVolumeImportWithApi(
  { file, onProgress, projectId, signal, transferId }: VolumeUploadInput,
  uploadApi: Pick<VolumeUploadApi, 'uploadVolumeImportContent'>,
) {
  signal?.throwIfAborted()
  return uploadApi.uploadVolumeImportContent(projectId, transferId, file, signal, (transferredBytes, total) => {
    onProgress?.({
      transferredBytes,
      total,
      percent: total > 0 ? (transferredBytes / total) * 100 : 0,
    })
  })
}
