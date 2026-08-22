import { api } from '@/api'

export type NativeVolumeTransferResource = 'content' | 'manifest'

interface NativeVolumeTransferDownloadInput {
  filename: string
  projectId: string
  resource: NativeVolumeTransferResource
  transferId: string
}

interface NativeVolumeTransferDownloadDependencies {
  authorize: (projectId: string, transferId: string) => Promise<{ ticket: string }>
  resourceURL: (projectId: string, transferId: string, resource: NativeVolumeTransferResource, ticket: string) => string
  triggerDownload: (url: string, filename: string) => void
}

const nativeDownloadDependencies: NativeVolumeTransferDownloadDependencies = {
  authorize: (projectId, transferId) => api.authorizeVolumeTransferDownload(projectId, transferId),
  resourceURL: (projectId, transferId, resource, ticket) => resource === 'manifest'
    ? api.volumeTransferManifestURL(projectId, transferId, ticket)
    : api.volumeTransferContentURL(projectId, transferId, ticket),
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

// Native navigation lets the browser stream a large response directly to disk.
// The ticket is single-use and bound to the selected transfer and resource.
export async function startNativeVolumeTransferDownload(
  input: NativeVolumeTransferDownloadInput,
  dependencies: NativeVolumeTransferDownloadDependencies = nativeDownloadDependencies,
) {
  const authorization = await dependencies.authorize(input.projectId, input.transferId)
  dependencies.triggerDownload(
    dependencies.resourceURL(input.projectId, input.transferId, input.resource, authorization.ticket),
    input.filename,
  )
}
