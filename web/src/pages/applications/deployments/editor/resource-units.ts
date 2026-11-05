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

/** 运行规格快捷预设：只写入 cpuRequest / memoryRequest，不引入新的配置字段。 */
export const runtimeSizePresets = [
  { id: 's', cpuRequest: '250m', memoryRequest: '256Mi', cores: 0.25 },
  { id: 'm', cpuRequest: '500m', memoryRequest: '512Mi', cores: 0.5 },
  { id: 'l', cpuRequest: '1', memoryRequest: '1Gi', cores: 1 },
] as const

export type RuntimeSizePresetId = typeof runtimeSizePresets[number]['id']

/** 按数值比较当前规格与预设，`0.5` 与 `500m`、`1024Mi` 与 `1Gi` 视为相同。 */
export function matchRuntimeSizePreset(cpuRequest: string, memoryRequest: string): RuntimeSizePresetId | undefined {
  const cpu = cpuMillicores(cpuRequest)
  const memory = memoryMebibytes(memoryRequest)
  return runtimeSizePresets.find(preset => cpuMillicores(preset.cpuRequest) === cpu && memoryMebibytes(preset.memoryRequest) === memory)?.id
}

function cpuMillicores(value: string) {
  const match = /^(\d+(?:\.\d+)?)(m?)$/.exec(value.trim())
  if (!match)
    return Number.NaN
  return match[2] ? Number(match[1]) : Number(match[1]) * 1000
}

function memoryMebibytes(value: string) {
  const match = /^(\d+(?:\.\d+)?)(Mi|Gi)$/.exec(value.trim())
  if (!match)
    return Number.NaN
  return match[2] === 'Gi' ? Number(match[1]) * 1024 : Number(match[1])
}
