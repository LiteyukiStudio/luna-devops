import type { TFunction } from 'i18next'

export interface UnitInputUnit {
  value: string
  label: string
  suffix?: string
}

/** 运行、构建与模板共用的 Kubernetes CPU 单位。 */
export function cpuUnits(t: TFunction): UnitInputUnit[] {
  return [
    { label: 'm', value: 'm' },
    { label: t('common.cpuUnitCore'), value: '' },
  ]
}

/** 运行、构建与模板共用的 Kubernetes 内存单位。 */
export const memoryUnits: UnitInputUnit[] = [
  { label: 'Mi', value: 'Mi' },
  { label: 'Gi', value: 'Gi' },
]

/** 数据卷与临时卷上限共用的 Kubernetes 存储容量单位（默认 Gi）。 */
export const storageUnits: UnitInputUnit[] = [
  { label: 'Gi', value: 'Gi' },
  { label: 'Mi', value: 'Mi' },
  { label: 'Ti', value: 'Ti' },
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

/** 校验正数 Kubernetes 资源量（如 `250m`、`0.5`、`1.5Gi`、`512Mi`、`1Ti`）。 */
export function isValidResourceQuantity(value: string, allowedUnits?: readonly string[]): boolean {
  const match = /^(\d+(?:\.\d+)?)([a-z]*)$/i.exec(value.trim())
  if (!match)
    return false
  const amount = Number(match[1])
  if (!Number.isFinite(amount) || amount <= 0)
    return false
  if (allowedUnits && !allowedUnits.includes(match[2]))
    return false
  return true
}

export function parseUnitInputValue(value: string, units: UnitInputUnit[]) {
  const normalized = String(value || '').trim()
  const fallbackUnit = units[0]?.value ?? ''
  if (!normalized)
    return { amount: '', unit: fallbackUnit }

  const suffixUnits = [...units]
    .map(unit => ({ ...unit, suffix: unit.suffix ?? unit.value }))
    .filter(unit => unit.suffix)
    .sort((a, b) => String(b.suffix).length - String(a.suffix).length)

  const matched = suffixUnits.find(unit => normalized.endsWith(unit.suffix ?? ''))
  if (matched)
    return { amount: normalizeUnitInputAmount(normalized.slice(0, -String(matched.suffix).length)), unit: matched.value }

  const bareUnit = units.find(unit => (unit.suffix ?? unit.value) === '')
  return { amount: normalizeUnitInputAmount(normalized), unit: bareUnit?.value ?? fallbackUnit }
}

export function formatUnitInputValue(amount: string, unitValue: string, units: UnitInputUnit[]) {
  const normalizedAmount = normalizeUnitInputAmount(amount)
  if (!normalizedAmount)
    return ''
  const unit = units.find(item => item.value === unitValue) ?? units[0]
  const suffix = unit ? (unit.suffix ?? unit.value) : ''
  return `${normalizedAmount}${suffix}`
}

export function normalizeUnitInputAmount(value: string) {
  const cleaned = value.replace(/[^\d.]/g, '')
  const dotIndex = cleaned.indexOf('.')
  if (dotIndex < 0)
    return cleaned
  const integerPart = cleaned.slice(0, dotIndex) || '0'
  const fractionPart = cleaned.slice(dotIndex + 1).replace(/\./g, '')
  return `${integerPart}.${fractionPart}`
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
