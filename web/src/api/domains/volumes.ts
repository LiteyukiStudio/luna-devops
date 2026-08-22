import type {
  PaginatedProjectVolumes,
  PaginatedProjectVolumeStorageClasses,
  PaginatedVolumeTransfers,
  ProjectVolume,
  ProjectVolumeCreateInput,
  ProjectVolumeDeletionPreview,
  ProjectVolumeDetail,
  ProjectVolumeListParams,
  ProjectVolumeStorageClass,
  ProjectVolumeUpdateInput,
  VolumeExportCreateInput,
  VolumeImportCreateInput,
  VolumeImportCreateResponse,
  VolumeTransfer,
  VolumeTransferDownloadAuthorization,
  VolumeTransferListParams,
} from '../volume-types'
import i18next from '@/i18n'
import { startAPIRequestSpan } from '@/lib/telemetry'
import { API_BASE_URL, ApiError, paginationQuery, request } from '../core'

function volumeListQuery(params: ProjectVolumeListParams) {
  const query = new URLSearchParams(paginationQuery(params))
  if (params.availability)
    query.set('availability', params.availability)
  if (params.lifecycleState)
    query.set('lifecycleState', params.lifecycleState)
  if (params.clusterId)
    query.set('clusterId', params.clusterId)
  if (params.sourceKind)
    query.set('sourceKind', params.sourceKind)
  if (params.ownershipMode)
    query.set('ownershipMode', params.ownershipMode)
  if (params.volumeMode)
    query.set('volumeMode', params.volumeMode)
  return query.toString()
}

function transferListQuery(params: VolumeTransferListParams) {
  const query = new URLSearchParams(paginationQuery(params))
  if (params.createdBy)
    query.set('createdBy', params.createdBy)
  if (params.direction)
    query.set('direction', params.direction)
  if (params.state)
    query.set('state', params.state)
  if (params.volumeId)
    query.set('volumeId', params.volumeId)
  return query.toString()
}

function idempotencyKey() {
  return globalThis.crypto?.randomUUID?.() ?? `volume-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function volumePath(projectId: string, volumeId: string) {
  return `/projects/${encodeURIComponent(projectId)}/volumes/${encodeURIComponent(volumeId)}`
}

function volumeTransferResourcePath(projectId: string, transferId: string, resource: 'content' | 'manifest') {
  return `/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}/${resource}`
}

function volumeTransferResourceURL(projectId: string, transferId: string, resource: 'content' | 'manifest') {
  return `${API_BASE_URL}${volumeTransferResourcePath(projectId, transferId, resource)}`
}

function volumeTransferResourceURLWithTicket(projectId: string, transferId: string, resource: 'content' | 'manifest', ticket: string) {
  const query = new URLSearchParams({ ticket })
  return `${volumeTransferResourceURL(projectId, transferId, resource)}?${query.toString()}`
}

function uploadVolumeImportContent(
  projectId: string,
  transferId: string,
  file: File,
  sha256: string,
  signal?: AbortSignal,
  onProgress?: (transferredBytes: number, totalBytes: number) => void,
): Promise<VolumeTransfer> {
  const path = `/projects/${encodeURIComponent(projectId)}/volume-imports/${encodeURIComponent(transferId)}/content`
  const telemetry = startAPIRequestSpan('PUT', path)
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    let settled = false
    const abort = () => xhr.abort()
    const cleanup = () => signal?.removeEventListener('abort', abort)
    const finish = (action: () => void) => {
      if (settled)
        return
      settled = true
      cleanup()
      action()
    }
    xhr.open('PUT', `${API_BASE_URL}${path}`)
    xhr.withCredentials = true
    xhr.responseType = 'json'
    xhr.setRequestHeader('Accept-Language', i18next.language)
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    xhr.setRequestHeader('X-Content-SHA256', sha256)
    for (const [name, value] of Object.entries(telemetry.headers))
      xhr.setRequestHeader(name, value)
    xhr.upload.addEventListener('progress', event => onProgress?.(event.loaded, file.size))
    xhr.addEventListener('load', () => finish(() => {
      const response = new Response(null, { status: xhr.status, statusText: xhr.statusText })
      telemetry.finish(response)
      const body = (xhr.response && typeof xhr.response === 'object' ? xhr.response : {}) as Partial<VolumeTransfer> & { code?: unknown, error?: unknown, requestId?: unknown }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(body as VolumeTransfer)
        return
      }
      const code = typeof body.code === 'string' ? body.code : `http.${xhr.status}`
      const translated = i18next.exists(`errors.${code}`) ? i18next.t(`errors.${code}`) : ''
      reject(new ApiError(translated || (typeof body.error === 'string' ? body.error : i18next.t(xhr.status >= 500 ? 'errors.internal_error' : 'errors.request.failed')), {
        code,
        path,
        requestId: typeof body.requestId === 'string' ? body.requestId : undefined,
        status: xhr.status,
      }))
    }))
    xhr.addEventListener('error', () => finish(() => {
      const error = new ApiError(i18next.t('errors.network.failed'), { code: 'network.failed', path, status: 0 })
      telemetry.fail(error)
      reject(error)
    }))
    xhr.addEventListener('abort', () => finish(() => {
      const error = signal?.reason ?? new DOMException('The operation was aborted.', 'AbortError')
      telemetry.fail(error)
      reject(error)
    }))
    signal?.addEventListener('abort', abort, { once: true })
    if (signal?.aborted) {
      abort()
      return
    }
    xhr.send(file)
  })
}

export const volumesApi = {
  listProjectVolumes: (projectId: string, params: ProjectVolumeListParams) =>
    request<PaginatedProjectVolumes>(`/projects/${encodeURIComponent(projectId)}/volumes?${volumeListQuery(params)}`),
  createProjectVolume: (projectId: string, payload: ProjectVolumeCreateInput) =>
    request<ProjectVolume>(`/projects/${encodeURIComponent(projectId)}/volumes`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey() },
      body: JSON.stringify(payload),
    }),
  listProjectVolumeStorageClasses: (projectId: string, clusterId: string, params: { page: number, pageSize: number, sortBy?: 'name' | 'provisioner', sortOrder?: 'asc' | 'desc' }) => {
    const query = new URLSearchParams(paginationQuery(params))
    query.set('clusterId', clusterId)
    return request<PaginatedProjectVolumeStorageClasses>(`/projects/${encodeURIComponent(projectId)}/volume-storage-classes?${query.toString()}`)
  },
  getProjectVolume: (projectId: string, volumeId: string, params: { bindingPage?: number, bindingPageSize?: number, transferPage?: number, transferPageSize?: number } = {}) => {
    const query = new URLSearchParams()
    if (params.bindingPage)
      query.set('bindingPage', String(params.bindingPage))
    if (params.bindingPageSize)
      query.set('bindingPageSize', String(params.bindingPageSize))
    if (params.transferPage)
      query.set('transferPage', String(params.transferPage))
    if (params.transferPageSize)
      query.set('transferPageSize', String(params.transferPageSize))
    const suffix = query.size > 0 ? `?${query.toString()}` : ''
    return request<ProjectVolumeDetail>(`${volumePath(projectId, volumeId)}${suffix}`)
  },
  updateProjectVolume: (projectId: string, volumeId: string, revision: number, payload: ProjectVolumeUpdateInput) =>
    request<ProjectVolume>(volumePath(projectId, volumeId), {
      method: 'PATCH',
      headers: { 'If-Match': String(revision) },
      body: JSON.stringify(payload),
    }),
  previewProjectVolumeDeletion: (projectId: string, volumeId: string) =>
    request<ProjectVolumeDeletionPreview>(`${volumePath(projectId, volumeId)}/deletion-preview`, { method: 'POST' }),
  deleteProjectVolume: (projectId: string, volumeId: string, revision: number, dataAction: 'delete' | 'detach') =>
    request<ProjectVolume>(`${volumePath(projectId, volumeId)}?dataAction=${dataAction}`, {
      method: 'DELETE',
      headers: { 'If-Match': String(revision) },
    }),
  retryProjectVolumeOperation: (projectId: string, volumeId: string, revision: number) =>
    request<ProjectVolume>(`${volumePath(projectId, volumeId)}/retry`, {
      method: 'POST',
      headers: { 'If-Match': String(revision) },
    }),
  createVolumeImport: (projectId: string, payload: VolumeImportCreateInput) =>
    request<VolumeImportCreateResponse>(`/projects/${encodeURIComponent(projectId)}/volume-imports`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey() },
      body: JSON.stringify(payload),
    }),
  uploadVolumeImportContent,
  createVolumeExport: (projectId: string, volumeId: string, payload: VolumeExportCreateInput) =>
    request<VolumeTransfer>(`${volumePath(projectId, volumeId)}/exports`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey() },
      body: JSON.stringify(payload),
    }),
  listVolumeTransfers: (projectId: string, params: VolumeTransferListParams) =>
    request<PaginatedVolumeTransfers>(`/projects/${encodeURIComponent(projectId)}/volume-transfers?${transferListQuery(params)}`),
  getVolumeTransfer: (projectId: string, transferId: string, signal?: AbortSignal) =>
    request<VolumeTransfer>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}`, { signal }),
  retryVolumeTransfer: (projectId: string, transferId: string) =>
    request<VolumeTransfer>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}/retry`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey() },
    }),
  cancelVolumeTransfer: (projectId: string, transferId: string) =>
    request<VolumeTransfer>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}/cancel`, { method: 'POST' }),
  authorizeVolumeTransferDownload: (projectId: string, transferId: string) =>
    request<VolumeTransferDownloadAuthorization>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}/download-authorizations`, { method: 'POST' }),
  volumeTransferContentURL: (projectId: string, transferId: string, ticket: string) =>
    volumeTransferResourceURLWithTicket(projectId, transferId, 'content', ticket),
  volumeTransferManifestURL: (projectId: string, transferId: string, ticket: string) =>
    volumeTransferResourceURLWithTicket(projectId, transferId, 'manifest', ticket),
}

export type ProjectVolumeStorageClassPage = PaginatedProjectVolumeStorageClasses
export type ProjectVolumeStorageClassItem = ProjectVolumeStorageClass
