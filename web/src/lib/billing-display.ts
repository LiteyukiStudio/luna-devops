import type { BillingRateRule, RuntimeCluster } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { usePublicConfig } from '@/app/public-config-context'

export type RuntimeResourcePolicy = Pick<RuntimeCluster, 'cpuRequestPercent' | 'memoryRequestPercent' | 'cpuLimitPercent' | 'memoryLimitPercent'>

export interface RuntimeHourCostInput {
  /** Per-replica quota entered in the editor; the cluster policy derives requests and limits from it. */
  cpuQuota: string | undefined
  memoryQuota: string | undefined
  /** Replicas for the request floor (minimum replicas with autoscaling). */
  floorReplicas: number | undefined
  /** Replicas for the limit ceiling (maximum replicas with autoscaling). */
  ceilingReplicas: number | undefined
  policy: RuntimeResourcePolicy
}

/**
 * Runtime is billed per minute as max(request × desired replicas, actual usage) for CPU and memory.
 * Actual usage cannot exceed the limit, so the hourly cost lies between the request floor and the
 * limit ceiling. ceiling is null when the policy leaves a billed resource without a limit.
 */
export interface RuntimeHourCostRange {
  floor: number
  ceiling: number | null
}

const DEFAULT_CURRENCY_UNIT = 'Credits'
const DEFAULT_FIAT_CURRENCY_UNIT = 'CNY'
const CREDIT_NUMBER_FORMAT_OPTIONS = { maximumFractionDigits: 2, minimumFractionDigits: 0 } as const

export function useBillingDisplay(locale: string) {
  const rateRules = useQuery({ queryKey: ['billing-rate-rules'], queryFn: api.listBillingRateRules })
  const amountDisplay = useBillingAmountDisplay(locale)

  return {
    ...amountDisplay,
    buildMinuteCost: (cpuRequest: string | undefined, memoryRequest: string | undefined) =>
      estimateBuildMinuteCost(rateRules.data ?? [], cpuRequest, memoryRequest),
    runtimeHourCostRange: (input: RuntimeHourCostInput) => estimateRuntimeHourCostRange(rateRules.data ?? [], input),
  }
}

export function useBillingAmountDisplay(locale: string) {
  const configs = usePublicConfig()
  const currencyUnit = configs['billing.creditsDisplayName']?.trim() || DEFAULT_CURRENCY_UNIT
  const fiatCurrencyUnit = configs['billing.fiatCurrencyUnit']?.trim() || DEFAULT_FIAT_CURRENCY_UNIT
  const creditsPerFiatUnit = parsePositiveNumber(configs['billing.creditsPerFiatUnit']?.trim() || '0')

  return {
    creditsPerFiatUnit,
    currencyUnit,
    fiatCurrencyUnit,
    formatFiatAmount: (value: string | number | undefined) =>
      formatFiatAmount(value, locale, fiatCurrencyUnit, creditsPerFiatUnit),
    formatAmount: (value: string | number | undefined) => formatBillingNumber(value, locale),
    formatAmountWithUnit: (value: string | number | undefined) => `${formatBillingNumber(value, locale)} ${currencyUnit}`,
    formatSignedAmountWithUnit: (value: string | number | undefined) => formatSignedBillingNumber(value, locale, currencyUnit),
  }
}

function formatFiatAmount(value: string | number | undefined, locale: string, fiatCurrencyUnit: string, creditsPerFiatUnit: number) {
  const numeric = parseBillingNumber(value)
  if (!Number.isFinite(numeric) || !Number.isFinite(creditsPerFiatUnit) || creditsPerFiatUnit <= 0)
    return ''
  const converted = numeric / creditsPerFiatUnit
  return `${converted.toLocaleString(locale, CREDIT_NUMBER_FORMAT_OPTIONS)} ${fiatCurrencyUnit}`
}

export function formatBillingNumber(value: string | number | undefined, locale: string) {
  const numeric = parseBillingNumber(value)
  if (!Number.isFinite(numeric))
    return '0'
  return numeric.toLocaleString(locale, CREDIT_NUMBER_FORMAT_OPTIONS)
}

function formatSignedBillingNumber(value: string | number | undefined, locale: string, currencyUnit: string) {
  const numeric = parseBillingNumber(value)
  if (!Number.isFinite(numeric))
    return `${value ?? '0'} ${currencyUnit}`
  const formatted = Math.abs(numeric).toLocaleString(locale, CREDIT_NUMBER_FORMAT_OPTIONS)
  if (numeric > 0)
    return `+${formatted} ${currencyUnit}`
  if (numeric < 0)
    return `-${formatted} ${currencyUnit}`
  return `${formatted} ${currencyUnit}`
}

function parseBillingNumber(value: string | number | undefined) {
  return typeof value === 'number' ? value : Number.parseFloat(value ?? '0')
}

function estimateBuildMinuteCost(rules: BillingRateRule[], cpuRequest: string | undefined, memoryRequest: string | undefined) {
  return estimateCost([
    [parseCPUToCores(cpuRequest), rateForMeter(rules, 'build.cpu_vcpu_minute')],
    [parseMemoryToGiB(memoryRequest), rateForMeter(rules, 'build.memory_gib_minute')],
  ])
}

export function estimateRuntimeHourCostRange(rules: BillingRateRule[], input: RuntimeHourCostInput): RuntimeHourCostRange {
  const cpuCores = parseCPUToCores(input.cpuQuota)
  const memoryGiB = parseMemoryToGiB(input.memoryQuota)
  const cpuRate = rateForMeter(rules, 'runtime.cpu_vcpu_hour')
  const memoryRate = rateForMeter(rules, 'runtime.memory_gib_hour')
  const floorReplicas = replicaCount(input.floorReplicas)
  const ceilingReplicas = replicaCount(input.ceilingReplicas)
  const floor = estimateCost([
    [cpuCores * percentRatio(input.policy.cpuRequestPercent) * floorReplicas, cpuRate],
    [memoryGiB * percentRatio(input.policy.memoryRequestPercent) * floorReplicas, memoryRate],
  ])
  const cpuUnbounded = input.policy.cpuLimitPercent <= 0 && cpuCores > 0 && cpuRate > 0
  const memoryUnbounded = input.policy.memoryLimitPercent <= 0 && memoryGiB > 0 && memoryRate > 0
  if (ceilingReplicas > 0 && (cpuUnbounded || memoryUnbounded))
    return { floor, ceiling: null }
  const ceiling = estimateCost([
    [cpuCores * percentRatio(input.policy.cpuLimitPercent) * ceilingReplicas, cpuRate],
    [memoryGiB * percentRatio(input.policy.memoryLimitPercent) * ceilingReplicas, memoryRate],
  ])
  return { floor, ceiling: Math.max(floor, ceiling) }
}

function replicaCount(value: number | undefined) {
  return Number.isFinite(value) && (value ?? 0) > 0 ? Math.floor(value ?? 0) : 0
}

function percentRatio(percent: number) {
  return Number.isFinite(percent) && percent > 0 ? percent / 100 : 0
}

function estimateCost(items: Array<[number, number]>) {
  let total = 0
  for (const [quantity, rate] of items)
    total += quantity * rate
  return Number.isFinite(total) ? total : 0
}

function rateForMeter(rules: BillingRateRule[], meter: string) {
  const rule = rules.find(item => item.meter === meter)
  if (!rule?.enabled)
    return 0
  const rate = Number.parseFloat(rule.creditsPerUnit)
  return Number.isFinite(rate) ? rate : 0
}

function parseCPUToCores(value: string | undefined) {
  const normalized = value?.trim() ?? ''
  if (!normalized)
    return 0
  if (normalized.endsWith('m'))
    return parsePositiveNumber(normalized.slice(0, -1)) / 1000
  return parsePositiveNumber(normalized)
}

function parseMemoryToGiB(value: string | undefined) {
  const normalized = value?.trim() ?? ''
  if (!normalized)
    return 0

  const match = /^(\d+(?:\.\d+)?)([KMGTP]i?)?$/i.exec(normalized)
  if (!match)
    return 0

  const amount = parsePositiveNumber(match[1])
  const unit = (match[2] ?? 'Gi').toLowerCase()
  if (unit === 'ki' || unit === 'k')
    return amount / 1024 / 1024
  if (unit === 'mi' || unit === 'm')
    return amount / 1024
  if (unit === 'gi' || unit === 'g')
    return amount
  if (unit === 'ti' || unit === 't')
    return amount * 1024
  if (unit === 'pi' || unit === 'p')
    return amount * 1024 * 1024
  return 0
}

function parsePositiveNumber(value: string) {
  const numeric = Number.parseFloat(value)
  if (!Number.isFinite(numeric) || numeric <= 0)
    return 0
  return numeric
}
