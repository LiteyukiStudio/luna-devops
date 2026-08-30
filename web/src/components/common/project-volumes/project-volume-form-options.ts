import type { TFunction } from 'i18next'
import type { ProjectVolumeAccessMode, ProjectVolumeMode } from '@/api'

interface ProjectVolumeOption<T extends string> {
  label: string
  value: T
}

export function projectVolumeAccessModeOptions(t: TFunction): ProjectVolumeOption<ProjectVolumeAccessMode>[] {
  return [
    { label: t('deploymentsPage.kubernetesValues.ReadWriteOnce'), value: 'ReadWriteOnce' },
    { label: t('deploymentsPage.kubernetesValues.ReadWriteOncePod'), value: 'ReadWriteOncePod' },
    { label: t('deploymentsPage.kubernetesValues.ReadOnlyMany'), value: 'ReadOnlyMany' },
    { label: t('deploymentsPage.kubernetesValues.ReadWriteMany'), value: 'ReadWriteMany' },
  ]
}

export function projectVolumeModeOptions(t: TFunction): ProjectVolumeOption<ProjectVolumeMode>[] {
  return [
    { label: t('deploymentsPage.kubernetesValues.Filesystem'), value: 'Filesystem' },
    { label: t('deploymentsPage.kubernetesValues.Block'), value: 'Block' },
  ]
}

export function projectVolumeModeLabel(t: TFunction, mode: ProjectVolumeMode) {
  return mode === 'Block'
    ? t('deploymentsPage.kubernetesValues.Block')
    : t('deploymentsPage.kubernetesValues.Filesystem')
}
