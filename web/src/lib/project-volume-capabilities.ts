import type { ProjectVolume, VolumeTransfer } from '@/api'
import { ProjectAction, projectActionAllowed } from '@/lib/roles'

export interface ProjectVolumeCapabilities {
  canDelete: boolean
  canExport: boolean
  canImport: boolean
  canManageOtherTransfers: boolean
  canWrite: boolean
  /** 部署是否配置了数据卷直传；为 false 时导入导出入口说明原因并禁止提交。 */
  transferEnabled: boolean
  userId: string
}

/** 从项目详情返回的 currentUserActions 投影数据卷操作；角色策略只由后端 internal/authz 维护。 */
export function projectVolumeCapabilities(actions: readonly string[] | null | undefined, userId: string | null | undefined, transferEnabled: boolean | undefined): ProjectVolumeCapabilities {
  const canExport = projectActionAllowed(actions, ProjectAction.VolumeExport)
  return {
    canDelete: projectActionAllowed(actions, ProjectAction.VolumeDelete),
    canExport,
    canImport: projectActionAllowed(actions, ProjectAction.VolumeImport),
    // 后端以 volume:export 判断能否管理他人发起的传输任务。
    canManageOtherTransfers: canExport,
    canWrite: projectActionAllowed(actions, ProjectAction.VolumeWrite),
    // 项目详情加载前未知时不提前告警，只在后端明确返回未配置时禁用。
    transferEnabled: transferEnabled !== false,
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
  // A failed direct import may have partially changed its destination PVC.
  // Retrying it in place would mix old and new data; users must start a fresh import.
  return transfer.direction === 'export' && capabilities.canExport
}
