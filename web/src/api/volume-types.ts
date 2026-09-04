import type { components, operations } from './generated/openapi.js'

type VolumeSchemas = components['schemas']
type ProjectVolumeQuery = NonNullable<operations['listProjectVolumes']['parameters']['query']>

export type ProjectVolumeOwnershipMode = VolumeSchemas['ProjectVolumeOwnershipMode']
export type ProjectVolumeSourceKind = VolumeSchemas['ProjectVolumeSourceKind']
export type ProjectVolumeLifecycleState = VolumeSchemas['ProjectVolumeLifecycleState']
export type ProjectVolumeAvailability = VolumeSchemas['ProjectVolumeAvailability']
export type ProjectVolumeAccessMode = VolumeSchemas['ProjectVolumeAccessMode']
export type ProjectVolumeMode = VolumeSchemas['ProjectVolumeMode']
export type VolumeTransferDirection = VolumeSchemas['VolumeTransferDirection']
export type VolumeTransferFormat = VolumeSchemas['VolumeTransferFormat']
export type VolumeTransferConsistencyMode = VolumeSchemas['VolumeTransferConsistencyMode']
export type VolumeTransferState = VolumeSchemas['VolumeTransferState']

export type ProjectVolumeObservation = VolumeSchemas['ProjectVolumeObservation']
export type ProjectVolume = VolumeSchemas['ProjectVolume']
export type ProjectVolumeBinding = VolumeSchemas['ProjectVolumeBinding']
export type VolumeTransfer = VolumeSchemas['VolumeTransfer']
export type ProjectVolumeDetail = VolumeSchemas['ProjectVolumeDetail']
export type ProjectVolumeDeletionPreview = VolumeSchemas['ProjectVolumeDeletionPreview']
export type ProjectVolumeStorageClass = VolumeSchemas['ProjectVolumeStorageClass']
export type ProjectVolumeCreateInput = VolumeSchemas['ProjectVolumeCreateInput']
export type ProjectVolumeUpdateInput = VolumeSchemas['ProjectVolumeUpdateInput']
export type VolumeImportCreateInput = VolumeSchemas['VolumeImportCreateInput']
export type VolumeImportCreateResponse = VolumeSchemas['VolumeImportCreateResponse']
export type VolumeExportCreateInput = VolumeSchemas['VolumeExportCreateInput']
export type VolumeTransferDownloadAuthorization = VolumeSchemas['VolumeTransferDownloadAuthorization']
export type PaginatedProjectVolumes = VolumeSchemas['PaginatedProjectVolumes']
export type PaginatedProjectVolumeStorageClasses = VolumeSchemas['PaginatedProjectVolumeStorageClasses']

// Console lists always choose a page explicitly, while the HTTP query may omit
// pagination and use server defaults.
export type ProjectVolumeListParams = ProjectVolumeQuery & Required<Pick<ProjectVolumeQuery, 'page' | 'pageSize'>>
