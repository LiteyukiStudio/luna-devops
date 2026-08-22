import type { PaginatedResponse, PaginationParams } from './types'

export type ProjectVolumeOwnershipMode = 'managed' | 'referenced'
export type ProjectVolumeSourceKind = 'archive_import' | 'blank' | 'existing_claim' | 'managed' | 'retained' | 'snapshot_restore'
export type ProjectVolumeLifecycleState = 'deleting' | 'error' | 'provisioning' | 'ready'
export type ProjectVolumeAvailability = 'available' | 'in_use' | 'reserved' | 'unavailable'
export type ProjectVolumeAccessMode = 'ReadOnlyMany' | 'ReadWriteMany' | 'ReadWriteOnce' | 'ReadWriteOncePod'
export type ProjectVolumeMode = 'Block' | 'Filesystem'
export type VolumeTransferDirection = 'export' | 'import'
export type VolumeTransferFormat = 'raw_zst' | 'tar_gz'
export type VolumeTransferConsistencyMode = 'live' | 'snapshot' | 'unmounted'
export type VolumeTransferState = 'cancelled' | 'created' | 'expired' | 'failed' | 'preparing' | 'ready' | 'streaming' | 'succeeded'

export interface ProjectVolumeObservation {
  status: ProjectVolumeAvailability
  exists: boolean
  phase: string
  capacity: string
  storageClassName: string
  accessModes: ProjectVolumeAccessMode[]
  volumeMode: ProjectVolumeMode
  boundVolumeName: string
  observedAt: string
  observationCode: string
}

export interface ProjectVolume {
  id: string
  projectId: string
  displayName: string
  clusterId: string
  namespace: string
  claimName: string
  ownershipMode: ProjectVolumeOwnershipMode
  sourceKind: ProjectVolumeSourceKind
  sourceSnapshotName?: string
  lifecycleState: ProjectVolumeLifecycleState
  pendingOperation?: 'delete' | 'expand' | 'import' | 'provision'
  availability: ProjectVolumeAvailability
  capacity: string
  capacityBytes: number
  storageClassName: string
  accessMode: ProjectVolumeAccessMode
  volumeMode: ProjectVolumeMode
  sourceApplicationId?: string
  sourceApplicationName?: string
  sourceDeploymentTargetId?: string
  bindingSummary: { active: number, reserved: number }
  revision: number
  lastErrorCode?: string
  observation: ProjectVolumeObservation
  createdAt: string
  updatedAt: string
}

export interface ProjectVolumeBinding {
  id: string
  applicationId: string
  deploymentTargetId: string
  logicalName: string
  sourceType: 'empty_dir' | 'project_volume'
  mountPath?: string
  devicePath?: string
  readOnly: boolean
  activationState: 'active' | 'error' | 'release_pending' | 'reserved'
  lastErrorCode?: string
  createdAt: string
  updatedAt: string
}

export interface VolumeTransfer {
  id: string
  projectId: string
  projectVolumeId: string
  direction: VolumeTransferDirection
  format: VolumeTransferFormat
  consistencyMode: VolumeTransferConsistencyMode
  state: VolumeTransferState
  sourceFilename?: string
  expectedBytes: number
  transferredBytes: number
  processedFiles: number
  phase?: string
  sha256: string
  logicalBytes: number
  dataSHA256: string
  actorId: string
  expiresAt: string
  startedAt?: string
  finishedAt?: string
  lastErrorCode: string
  createdAt: string
  updatedAt: string
}

export interface ProjectVolumeDetail extends ProjectVolume {
  bindings: ProjectVolumeBinding[]
  bindingPage: number
  bindingPageSize: number
  bindingTotal: number
  bindingTotalPages: number
  recentTransfers: VolumeTransfer[]
  transferPage: number
  transferPageSize: number
  transferTotal: number
  transferTotalPages: number
}

export interface ProjectVolumeDeletionPreview {
  volumeId: string
  ownershipMode: ProjectVolumeOwnershipMode
  dataAction: 'delete' | 'detach'
  hasActiveBindings: boolean
  hasRunningTransfers: boolean
  bindings: ProjectVolumeBinding[]
  runningTransfers: VolumeTransfer[]
  underlyingClaimWillBeDeleted: boolean
  observation: ProjectVolumeObservation
}

export interface ProjectVolumeStorageClass {
  name: string
  provisioner: string
  isDefault: boolean
  allowVolumeExpansion: boolean
  volumeBindingMode: 'Immediate' | 'WaitForFirstConsumer'
  reclaimPolicy: string
  snapshotSupported: boolean
}

interface ProjectVolumeCreateBase {
  displayName: string
  clusterId: string
}

interface ProjectVolumeManagedCreateSpec extends ProjectVolumeCreateBase {
  capacity: string
  storageClassName: string
  accessMode: ProjectVolumeAccessMode
  volumeMode: ProjectVolumeMode
}

export type ProjectVolumeCreateInput
  = | (ProjectVolumeManagedCreateSpec & { source: { type: 'blank' } })
    | (ProjectVolumeManagedCreateSpec & { source: { snapshotName: string, type: 'volumeSnapshot' } })
    | (ProjectVolumeCreateBase & { source: { claimName: string, ownershipMode: ProjectVolumeOwnershipMode, type: 'existingClaim' } })

export interface ProjectVolumeUpdateInput {
  displayName?: string
  capacity?: string
}

export interface ProjectVolumeListParams extends PaginationParams {
  availability?: ProjectVolumeAvailability
  lifecycleState?: ProjectVolumeLifecycleState
  clusterId?: string
  sourceKind?: ProjectVolumeSourceKind
  ownershipMode?: ProjectVolumeOwnershipMode
  volumeMode?: ProjectVolumeMode
  sortBy?: 'capacity' | 'createdAt' | 'displayName' | 'updatedAt'
}

export interface VolumeTransferListParams extends PaginationParams {
  createdBy?: string
  direction?: VolumeTransferDirection
  state?: VolumeTransferState
  volumeId?: string
  sortBy?: 'createdAt' | 'state' | 'transferredBytes' | 'updatedAt'
}

export interface VolumeImportCreateInput {
  displayName: string
  clusterId: string
  capacity: string
  storageClassName: string
  accessMode: ProjectVolumeAccessMode
  volumeMode: ProjectVolumeMode
  format: VolumeTransferFormat
  filename: string
  contentLength: number
  sha256: string
}

export interface VolumeImportCreateResponse {
  volume: ProjectVolume
  transfer: VolumeTransfer
}

export interface VolumeExportCreateInput {
  consistency: 'auto' | 'live' | 'snapshot'
  format: VolumeTransferFormat
}

export interface VolumeTransferDownloadAuthorization {
  ticket: string
  expiresAt: string
}

export type PaginatedProjectVolumes = PaginatedResponse<ProjectVolume>
export type PaginatedProjectVolumeStorageClasses = PaginatedResponse<ProjectVolumeStorageClass>
export type PaginatedVolumeTransfers = PaginatedResponse<VolumeTransfer>
