import type { TFunction } from 'i18next'
import type { UnitInputUnit } from '@/components/common/unit-input'

/** 运行与构建规格共用的 Kubernetes 资源单位。 */
export function cpuUnits(t: TFunction): UnitInputUnit[] {
  return [
    { label: 'm', value: 'm' },
    { label: t('deploymentsPage.cpuUnits.core'), value: '' },
  ]
}

export const memoryUnits: UnitInputUnit[] = [
  { label: 'Mi', value: 'Mi' },
  { label: 'Gi', value: 'Gi' },
]
