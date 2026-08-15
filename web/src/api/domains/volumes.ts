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
  VolumeImportUploadOffset,
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

function volumeTransferTicketSuffix(ticket?: string) {
  if (!ticket)
    return ''
  const query = new URLSearchParams({ ticket })
  return `?${query.toString()}`
}

function requiredUploadHeader(response: Response, name: 'Upload-Chunk-Size' | 'Upload-Length' | 'Upload-Offset', minimum: number) {
  const value = Number(response.headers.get(name))
  if (!Number.isSafeInteger(value) || value < minimum) {
    throw new ApiError(i18next.t('errors.request.failed'), {
      code: 'volume_transfer.response_invalid',
      path: response.url,
      status: 502,
    })
  }
  return value
}

function retryAfterMilliseconds(value: string | null) {
  const seconds = Number(value)
  if (!Number.isSafeInteger(seconds) || seconds < 0)
    return undefined
  return Math.min(5_000, Math.max(250, seconds * 1_000))
}

async function rawVolumeRequest(path: string, options: RequestInit) {
  const telemetry = startAPIRequestSpan(options.method ?? 'GET', path)
  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      ...options,
      credentials: 'include',
      headers: {
        'Accept-Language': i18next.language,
        ...telemetry.headers,
        ...options.headers,
      },
    })
  }
  catch (error) {
    telemetry.fail(error)
    throw new ApiError(i18next.t('errors.network.failed'), {
      code: 'network.failed',
      detail: error instanceof Error ? error.message : String(error),
      path,
      status: 0,
    })
  }
  telemetry.finish(response)
  if (!response.ok) {
    const body = await response.clone().json().catch(() => ({})) as { code?: unknown, error?: unknown, requestId?: unknown }
    const code = typeof body.code === 'string' ? body.code : `http.${response.status}`
    const safeMessage = typeof body.error === 'string' && body.error.trim()
      ? body.error
      : i18next.t(response.status >= 500 ? 'errors.internal_error' : 'errors.request.failed')
    throw new ApiError(safeMessage, {
      code,
      path,
      requestId: typeof body.requestId === 'string' ? body.requestId : undefined,
      retryAfterMs: retryAfterMilliseconds(response.headers.get('Retry-After')),
      status: response.status,
    })
  }
  return response
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
  getVolumeImportUploadOffset: async (projectId: string, transferId: string, signal?: AbortSignal): Promise<VolumeImportUploadOffset> => {
    const response = await rawVolumeRequest(`/projects/${encodeURIComponent(projectId)}/volume-imports/${encodeURIComponent(transferId)}/content`, {
      method: 'HEAD',
      signal,
      headers: { 'Tus-Resumable': '1.0.0' },
    })
    return {
      chunkSize: requiredUploadHeader(response, 'Upload-Chunk-Size', 64 * 1024 * 1024),
      length: requiredUploadHeader(response, 'Upload-Length', 1),
      offset: requiredUploadHeader(response, 'Upload-Offset', 0),
    }
  },
  uploadVolumeImportChunk: async (projectId: string, transferId: string, chunk: Blob, offset: number, checksum: string, signal?: AbortSignal) => {
    const response = await rawVolumeRequest(`/projects/${encodeURIComponent(projectId)}/volume-imports/${encodeURIComponent(transferId)}/content`, {
      method: 'PATCH',
      body: chunk,
      signal,
      headers: {
        'Content-Type': 'application/offset+octet-stream',
        'Tus-Resumable': '1.0.0',
        'Upload-Checksum': `sha256 ${checksum}`,
        'Upload-Offset': String(offset),
      },
    })
    return {
      chunkSize: requiredUploadHeader(response, 'Upload-Chunk-Size', 64 * 1024 * 1024),
      offset: requiredUploadHeader(response, 'Upload-Offset', 0),
    }
  },
  completeVolumeImportUpload: (projectId: string, transferId: string, contentLength: number, sha256: string) =>
    request<VolumeTransfer>(`/projects/${encodeURIComponent(projectId)}/volume-imports/${encodeURIComponent(transferId)}/complete`, {
      method: 'POST',
      body: JSON.stringify({ contentLength, sha256 }),
    }),
  createVolumeExport: (projectId: string, volumeId: string, payload: VolumeExportCreateInput) =>
    request<VolumeTransfer>(`${volumePath(projectId, volumeId)}/exports`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey() },
      body: JSON.stringify(payload),
    }),
  listVolumeTransfers: (projectId: string, params: VolumeTransferListParams) =>
    request<PaginatedVolumeTransfers>(`/projects/${encodeURIComponent(projectId)}/volume-transfers?${transferListQuery(params)}`),
  getVolumeTransfer: (projectId: string, transferId: string) =>
    request<VolumeTransfer>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}`),
  retryVolumeTransfer: (projectId: string, transferId: string) =>
    request<VolumeTransfer>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}/retry`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey() },
    }),
  cancelVolumeTransfer: (projectId: string, transferId: string) =>
    request<VolumeTransfer>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}/cancel`, { method: 'POST' }),
  authorizeVolumeTransferDownload: (projectId: string, transferId: string) =>
    request<VolumeTransferDownloadAuthorization>(`/projects/${encodeURIComponent(projectId)}/volume-transfers/${encodeURIComponent(transferId)}/download-authorizations`, { method: 'POST' }),
  volumeTransferContentURL: (projectId: string, transferId: string) =>
    volumeTransferResourceURL(projectId, transferId, 'content'),
  volumeTransferManifestURL: (projectId: string, transferId: string) =>
    volumeTransferResourceURL(projectId, transferId, 'manifest'),
  headVolumeTransferContent: (projectId: string, transferId: string, ticket?: string, signal?: AbortSignal) =>
    rawVolumeRequest(`${volumeTransferResourcePath(projectId, transferId, 'content')}${volumeTransferTicketSuffix(ticket)}`, {
      method: 'HEAD',
      signal,
    }),
  headVolumeTransferManifest: (projectId: string, transferId: string, ticket?: string, signal?: AbortSignal) =>
    rawVolumeRequest(`${volumeTransferResourcePath(projectId, transferId, 'manifest')}${volumeTransferTicketSuffix(ticket)}`, {
      method: 'HEAD',
      signal,
    }),
  downloadVolumeTransferManifest: (projectId: string, transferId: string, ticket?: string, signal?: AbortSignal) =>
    rawVolumeRequest(`${volumeTransferResourcePath(projectId, transferId, 'manifest')}${volumeTransferTicketSuffix(ticket)}`, {
      method: 'GET',
      signal,
    }),
  downloadVolumeTransferContent: (projectId: string, transferId: string, ticket?: string, offset = 0, signal?: AbortSignal) => {
    return rawVolumeRequest(`${volumeTransferResourcePath(projectId, transferId, 'content')}${volumeTransferTicketSuffix(ticket)}`, {
      method: 'GET',
      signal,
      headers: offset > 0 ? { Range: `bytes=${offset}-` } : {},
    })
  },
}

export type ProjectVolumeStorageClassPage = PaginatedProjectVolumeStorageClasses
export type ProjectVolumeStorageClassItem = ProjectVolumeStorageClass
