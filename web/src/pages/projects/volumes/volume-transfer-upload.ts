import type { VolumeImportResumeRecord, VolumeTransfer } from '@/api'
import { api, ApiError } from '@/api'
import i18next from '@/i18n'

const HASH_READ_SIZE = 8 * 1024 * 1024
const MIN_UPLOAD_CHUNK_SIZE = 64 * 1024 * 1024
const MAX_UPLOAD_CHUNK_SIZE = 5 * 1024 * 1024 * 1024
const DEFAULT_PART_RETRY_DELAY_MS = 1_000
const MAX_PART_IN_PROGRESS_RETRIES = 5
const RESUME_STORAGE_PREFIX = 'luna.project-volume-import.v1'

export interface VolumeUploadProgress {
  offset: number
  total: number
  percent: number
}

interface VolumeUploadInput {
  file: File
  onProgress?: (progress: VolumeUploadProgress) => void
  projectId: string
  sha256: string
  signal?: AbortSignal
  transfer: VolumeTransfer
}

type VolumeUploadApi = Pick<typeof api, 'completeVolumeImportUpload' | 'getVolumeImportUploadOffset' | 'uploadVolumeImportChunk'>

function rotateRight(value: number, bits: number) {
  return (value >>> bits) | (value << (32 - bits))
}

const SHA256_ROUND_CONSTANTS = new Uint32Array([
  0x428A2F98,
  0x71374491,
  0xB5C0FBCF,
  0xE9B5DBA5,
  0x3956C25B,
  0x59F111F1,
  0x923F82A4,
  0xAB1C5ED5,
  0xD807AA98,
  0x12835B01,
  0x243185BE,
  0x550C7DC3,
  0x72BE5D74,
  0x80DEB1FE,
  0x9BDC06A7,
  0xC19BF174,
  0xE49B69C1,
  0xEFBE4786,
  0x0FC19DC6,
  0x240CA1CC,
  0x2DE92C6F,
  0x4A7484AA,
  0x5CB0A9DC,
  0x76F988DA,
  0x983E5152,
  0xA831C66D,
  0xB00327C8,
  0xBF597FC7,
  0xC6E00BF3,
  0xD5A79147,
  0x06CA6351,
  0x14292967,
  0x27B70A85,
  0x2E1B2138,
  0x4D2C6DFC,
  0x53380D13,
  0x650A7354,
  0x766A0ABB,
  0x81C2C92E,
  0x92722C85,
  0xA2BFE8A1,
  0xA81A664B,
  0xC24B8B70,
  0xC76C51A3,
  0xD192E819,
  0xD6990624,
  0xF40E3585,
  0x106AA070,
  0x19A4C116,
  0x1E376C08,
  0x2748774C,
  0x34B0BCB5,
  0x391C0CB3,
  0x4ED8AA4A,
  0x5B9CCA4F,
  0x682E6FF3,
  0x748F82EE,
  0x78A5636F,
  0x84C87814,
  0x8CC70208,
  0x90BEFFFA,
  0xA4506CEB,
  0xBEF9A3F7,
  0xC67178F2,
])

class IncrementalSha256 {
  private readonly block = new Uint8Array(64)
  private readonly words = new Uint32Array(64)
  private readonly state = new Uint32Array([
    0x6A09E667,
    0xBB67AE85,
    0x3C6EF372,
    0xA54FF53A,
    0x510E527F,
    0x9B05688C,
    0x1F83D9AB,
    0x5BE0CD19,
  ])

  private blockLength = 0
  private byteLength = 0

  update(input: Uint8Array) {
    this.byteLength += input.byteLength
    let offset = 0
    while (offset < input.byteLength) {
      const copied = Math.min(64 - this.blockLength, input.byteLength - offset)
      this.block.set(input.subarray(offset, offset + copied), this.blockLength)
      this.blockLength += copied
      offset += copied
      if (this.blockLength === 64) {
        this.compress()
        this.blockLength = 0
      }
    }
  }

  digestHex() {
    const bitLength = BigInt(this.byteLength) * 8n
    this.block[this.blockLength++] = 0x80
    if (this.blockLength > 56) {
      this.block.fill(0, this.blockLength)
      this.compress()
      this.blockLength = 0
    }
    this.block.fill(0, this.blockLength, 56)
    for (let index = 0; index < 8; index++)
      this.block[63 - index] = Number((bitLength >> BigInt(index * 8)) & 0xFFn)
    this.compress()
    return [...this.state].map(value => value.toString(16).padStart(8, '0')).join('')
  }

  private compress() {
    const words = this.words
    for (let index = 0; index < 16; index++) {
      const offset = index * 4
      words[index] = (this.block[offset]! << 24) | (this.block[offset + 1]! << 16) | (this.block[offset + 2]! << 8) | this.block[offset + 3]!
    }
    for (let index = 16; index < 64; index++) {
      const left = words[index - 15]!
      const right = words[index - 2]!
      const sigma0 = rotateRight(left, 7) ^ rotateRight(left, 18) ^ (left >>> 3)
      const sigma1 = rotateRight(right, 17) ^ rotateRight(right, 19) ^ (right >>> 10)
      words[index] = (words[index - 16]! + sigma0 + words[index - 7]! + sigma1) >>> 0
    }

    let [a, b, c, d, e, f, g, h] = this.state
    for (let index = 0; index < 64; index++) {
      const sum1 = rotateRight(e!, 6) ^ rotateRight(e!, 11) ^ rotateRight(e!, 25)
      const choice = (e! & f!) ^ (~e! & g!)
      const temp1 = (h! + sum1 + choice + SHA256_ROUND_CONSTANTS[index]! + words[index]!) >>> 0
      const sum0 = rotateRight(a!, 2) ^ rotateRight(a!, 13) ^ rotateRight(a!, 22)
      const majority = (a! & b!) ^ (a! & c!) ^ (b! & c!)
      const temp2 = (sum0 + majority) >>> 0
      h = g
      g = f
      f = e
      e = (d! + temp1) >>> 0
      d = c
      c = b
      b = a
      a = (temp1 + temp2) >>> 0
    }
    this.state[0] = (this.state[0]! + a!) >>> 0
    this.state[1] = (this.state[1]! + b!) >>> 0
    this.state[2] = (this.state[2]! + c!) >>> 0
    this.state[3] = (this.state[3]! + d!) >>> 0
    this.state[4] = (this.state[4]! + e!) >>> 0
    this.state[5] = (this.state[5]! + f!) >>> 0
    this.state[6] = (this.state[6]! + g!) >>> 0
    this.state[7] = (this.state[7]! + h!) >>> 0
  }
}

export async function sha256File(file: Blob, onProgress?: (processed: number, total: number) => void) {
  const hash = new IncrementalSha256()
  for (let offset = 0; offset < file.size; offset += HASH_READ_SIZE) {
    const bytes = new Uint8Array(await file.slice(offset, offset + HASH_READ_SIZE).arrayBuffer())
    hash.update(bytes)
    onProgress?.(Math.min(offset + bytes.byteLength, file.size), file.size)
  }
  return hash.digestHex()
}

export async function sha256BlobBase64(blob: Blob) {
  const hash = new IncrementalSha256()
  for (let offset = 0; offset < blob.size; offset += HASH_READ_SIZE) {
    const bytes = new Uint8Array(await blob.slice(offset, offset + HASH_READ_SIZE).arrayBuffer())
    hash.update(bytes)
  }
  const digestHex = hash.digestHex()
  let binary = ''
  for (let offset = 0; offset < digestHex.length; offset += 2)
    binary += String.fromCharCode(Number.parseInt(digestHex.slice(offset, offset + 2), 16))
  return btoa(binary)
}

function validUploadChunkSize(value: number) {
  return Number.isSafeInteger(value) && value >= MIN_UPLOAD_CHUNK_SIZE && value <= MAX_UPLOAD_CHUNK_SIZE
}

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

export function volumeImportResumeStorageKey(projectId: string) {
  return `${RESUME_STORAGE_PREFIX}.${projectId}`
}

export function readVolumeImportResumeRecord(projectId: string): VolumeImportResumeRecord | null {
  try {
    const raw = localStorage.getItem(volumeImportResumeStorageKey(projectId))
    if (!raw)
      return null
    const value = JSON.parse(raw) as Partial<VolumeImportResumeRecord>
    if (value.projectId !== projectId
      || typeof value.transferId !== 'string'
      || !value.transferId.startsWith('vtx_')
      || typeof value.volumeId !== 'string'
      || !value.volumeId.startsWith('pvol_')
      || typeof value.filename !== 'string'
      || typeof value.size !== 'number'
      || !Number.isFinite(value.size)
      || value.size <= 0
      || typeof value.lastModified !== 'number'
      || !Number.isFinite(value.lastModified)
      || typeof value.sha256 !== 'string'
      || !/^[a-f0-9]{64}$/.test(value.sha256)
      || (value.format !== 'tar_gz' && value.format !== 'raw_zst')
      || typeof value.createdAt !== 'string') {
      return null
    }
    return value as VolumeImportResumeRecord
  }
  catch {
    return null
  }
}

export function writeVolumeImportResumeRecord(record: VolumeImportResumeRecord) {
  try {
    localStorage.setItem(volumeImportResumeStorageKey(record.projectId), JSON.stringify(record))
  }
  catch {
    // Resuming is optional when browser storage is unavailable; the active upload can continue.
  }
}

export function clearVolumeImportResumeRecord(projectId: string, transferId?: string) {
  try {
    if (!transferId || readVolumeImportResumeRecord(projectId)?.transferId === transferId)
      localStorage.removeItem(volumeImportResumeStorageKey(projectId))
  }
  catch {
    // Nothing else is required when browser storage is unavailable.
  }
}

export function fileMatchesResumeRecord(file: File, record: VolumeImportResumeRecord) {
  return file.name === record.filename && file.size === record.size && file.lastModified === record.lastModified
}

export function uploadVolumeImport(input: VolumeUploadInput) {
  return uploadVolumeImportWithApi(input, api)
}

export async function uploadVolumeImportWithApi({ file, onProgress, projectId, sha256, signal, transfer }: VolumeUploadInput, uploadApi: VolumeUploadApi) {
  const remote = await uploadApi.getVolumeImportUploadOffset(projectId, transfer.id, signal)
  if (remote.length > 0 && remote.length !== file.size)
    throw new ApiError('upload_length_mismatch', { code: 'volume_transfer.offset_mismatch', path: '/volume-imports/content', status: 409 })
  if (!validUploadChunkSize(transfer.chunkSize)
    || !validUploadChunkSize(remote.chunkSize)
    || transfer.chunkSize !== remote.chunkSize) {
    throw new ApiError(i18next.t('errors.request.failed'), { code: 'volume_transfer.response_invalid', path: '/volume-imports/content', status: 502 })
  }

  const chunkSize = remote.chunkSize
  let offset = remote.offset
  while (offset < file.size) {
    signal?.throwIfAborted()
    const chunk = file.slice(offset, Math.min(offset + chunkSize, file.size))
    const checksum = await sha256BlobBase64(chunk)
    const attemptedOffset = offset
    let partInProgressRetries = 0
    while (true) {
      try {
        const uploaded = await uploadApi.uploadVolumeImportChunk(projectId, transfer.id, chunk, attemptedOffset, checksum, signal)
        if (uploaded.chunkSize !== chunkSize || uploaded.offset !== attemptedOffset + chunk.size)
          throw new ApiError(i18next.t('errors.request.failed'), { code: 'volume_transfer.response_invalid', path: '/volume-imports/content', status: 502 })
        offset = uploaded.offset
        break
      }
      catch (error) {
        if (!(error instanceof ApiError))
          throw error
        if (error.code !== 'volume_transfer.offset_mismatch' && error.code !== 'volume_transfer.part_in_progress')
          throw error
        if (error.code === 'volume_transfer.part_in_progress') {
          if (partInProgressRetries >= MAX_PART_IN_PROGRESS_RETRIES)
            throw error
          partInProgressRetries += 1
          await abortableDelay(error.retryAfterMs ?? DEFAULT_PART_RETRY_DELAY_MS, signal)
        }
        const refreshed = await uploadApi.getVolumeImportUploadOffset(projectId, transfer.id, signal)
        if (refreshed.chunkSize !== chunkSize || refreshed.length !== file.size || refreshed.offset < attemptedOffset || refreshed.offset > attemptedOffset + chunk.size) {
          throw new ApiError(i18next.t('errors.request.failed'), { code: 'volume_transfer.response_invalid', path: '/volume-imports/content', status: 502 })
        }
        if (refreshed.offset > attemptedOffset) {
          offset = refreshed.offset
          break
        }
        if (error.code === 'volume_transfer.offset_mismatch')
          throw error
      }
    }
    onProgress?.({ offset, total: file.size, percent: file.size > 0 ? (offset / file.size) * 100 : 100 })
  }
  return uploadApi.completeVolumeImportUpload(projectId, transfer.id, file.size, sha256)
}
