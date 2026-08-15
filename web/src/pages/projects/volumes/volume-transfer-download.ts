import { api } from '@/api'
import { sha256File } from './volume-transfer-upload'

export type NativeVolumeTransferResource = 'content' | 'manifest'

interface NativeVolumeTransferDownloadInput {
  filename: string
  projectId: string
  resource: NativeVolumeTransferResource
  transferId: string
}

interface NativeVolumeTransferDownloadDependencies {
  authorize: (projectId: string, transferId: string) => Promise<{ ticket: string }>
  exchangeSession: (projectId: string, transferId: string, resource: NativeVolumeTransferResource, ticket: string) => Promise<unknown>
  resourceURL: (projectId: string, transferId: string, resource: NativeVolumeTransferResource) => string
  triggerDownload: (url: string, filename: string) => void
}

const nativeDownloadDependencies: NativeVolumeTransferDownloadDependencies = {
  authorize: (projectId, transferId) => api.authorizeVolumeTransferDownload(projectId, transferId),
  exchangeSession: (projectId, transferId, resource, ticket) => resource === 'manifest'
    ? api.headVolumeTransferManifest(projectId, transferId, ticket)
    : api.headVolumeTransferContent(projectId, transferId, ticket),
  resourceURL: (projectId, transferId, resource) => resource === 'manifest'
    ? api.volumeTransferManifestURL(projectId, transferId)
    : api.volumeTransferContentURL(projectId, transferId),
  triggerDownload: (url, filename) => {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = filename
    anchor.rel = 'noopener'
    anchor.hidden = true
    document.body.append(anchor)
    anchor.click()
    anchor.remove()
  },
}

// The HEAD exchange consumes the one-time ticket and installs the scoped
// HttpOnly session cookie. Native navigation then lets the browser stream a
// large response directly to disk instead of retaining it in JavaScript memory.
export async function startNativeVolumeTransferDownload(
  input: NativeVolumeTransferDownloadInput,
  dependencies: NativeVolumeTransferDownloadDependencies = nativeDownloadDependencies,
) {
  const authorization = await dependencies.authorize(input.projectId, input.transferId)
  await dependencies.exchangeSession(input.projectId, input.transferId, input.resource, authorization.ticket)
  dependencies.triggerDownload(
    dependencies.resourceURL(input.projectId, input.transferId, input.resource),
    input.filename,
  )
}

export async function verifyVolumeTransferDownload(file: Blob, expectedBytes: number, expectedSha256: string) {
  const normalizedChecksum = expectedSha256.trim().toLowerCase()
  if (!Number.isSafeInteger(expectedBytes) || expectedBytes < 1 || !/^[a-f0-9]{64}$/.test(normalizedChecksum))
    return { status: 'metadata_invalid' as const }
  if (file.size !== expectedBytes) {
    return {
      status: 'length_mismatch' as const,
      actualBytes: file.size,
      expectedBytes,
    }
  }
  const actualSha256 = await sha256File(file)
  if (actualSha256 !== normalizedChecksum) {
    return {
      status: 'checksum_mismatch' as const,
      actualSha256,
      expectedSha256: normalizedChecksum,
    }
  }
  return { status: 'verified' as const, bytes: file.size, sha256: actualSha256 }
}
