import type { TFunction } from 'i18next'
import type { ProjectVolume, ProjectVolumeStorageClass } from '@/api'

export type VolumeSnapshotSupport = 'loading' | 'supported' | 'unsupported' | 'unknown'

/**
 * 按数据卷 PVC 实际使用的 StorageClass（观察不到时用登记值，空值表示默认类）判断快照能力。
 * 与后端导出一致性解析使用同一事实：存储类实时列表里的 snapshotSupported。
 */
export function volumeSnapshotSupport(
  volume: Pick<ProjectVolume, 'storageClassName' | 'observation'>,
  storageClasses: { items: readonly ProjectVolumeStorageClass[], complete: boolean, status: 'pending' | 'error' | 'success' },
): VolumeSnapshotSupport {
  if (storageClasses.status === 'pending')
    return 'loading'
  if (storageClasses.status === 'error')
    return 'unknown'
  const className = volume.observation.storageClassName || volume.storageClassName
  const storageClass = storageClasses.items.find(item => className ? item.name === className : item.isDefault)
  if (!storageClass)
    return storageClasses.complete ? 'unsupported' : 'unknown'
  return storageClass.snapshotSupported ? 'supported' : 'unsupported'
}

export function volumeSnapshotNotice(t: TFunction, volumeMode: ProjectVolume['volumeMode'], support: VolumeSnapshotSupport): string | null {
  switch (support) {
    case 'unsupported':
      return volumeMode === 'Block' ? t('projectVolumes.consistencySnapshotUnsupportedBlock') : t('projectVolumes.consistencySnapshotUnsupported')
    case 'unknown':
      return t('projectVolumes.consistencySnapshotUnknown')
    case 'loading':
    case 'supported':
      return null
  }
}
