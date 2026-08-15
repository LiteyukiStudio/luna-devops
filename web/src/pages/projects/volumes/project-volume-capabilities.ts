import type { ProjectVolume, VolumeTransfer } from '@/api'
import type { ProjectRoleValue } from '@/lib/roles'
import { isPlatformAdmin, ProjectRole } from '@/lib/roles'

export interface ProjectVolumeCapabilities {
  canDelete: boolean
  canExport: boolean
  canImport: boolean
  canManageOtherTransfers: boolean
  canWrite: boolean
  userId: string
}

export function projectVolumeCapabilities(platformRole: string | null | undefined, projectRole: ProjectRoleValue | null | undefined, userId: string | null | undefined): ProjectVolumeCapabilities {
  const platformAdmin = isPlatformAdmin(platformRole)
  const projectAdmin = projectRole === ProjectRole.Owner || projectRole === ProjectRole.Admin
  const projectWriter = projectAdmin || projectRole === ProjectRole.Developer
  return {
    canDelete: platformAdmin || projectAdmin,
    canExport: platformAdmin || projectAdmin,
    canImport: platformAdmin || projectWriter,
    canManageOtherTransfers: platformAdmin || projectAdmin,
    canWrite: platformAdmin || projectWriter,
    userId: userId ?? '',
  }
}

export function canRetryProjectVolume(capabilities: ProjectVolumeCapabilities, volume: ProjectVolume): boolean {
  if (volume.pendingOperation === 'delete')
    return capabilities.canDelete
  return (volume.pendingOperation === 'provision' || volume.pendingOperation === 'expand') && capabilities.canWrite
}

export function canCancelVolumeTransfer(capabilities: ProjectVolumeCapabilities, transfer: VolumeTransfer): boolean {
  return capabilities.canManageOtherTransfers || Boolean(capabilities.userId && transfer.actorId === capabilities.userId)
}

export function canRetryVolumeTransfer(capabilities: ProjectVolumeCapabilities, transfer: VolumeTransfer): boolean {
  return transfer.direction === 'import' ? capabilities.canImport : capabilities.canExport
}
