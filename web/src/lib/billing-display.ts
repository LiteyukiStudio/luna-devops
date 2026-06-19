import type { BillingRateRule } from '@/api/client'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { usePublicConfig } from '@/app/public-config-context'

const DEFAULT_CURRENCY_UNIT = 'Credits'
const CREDIT_NUMBER_FORMAT_OPTIONS = { maximumFractionDigits: 2, minimumFractionDigits: 0 } as const

export function useBillingDisplay(locale: string) {
  const configs = usePublicConfig()
  const rateRules = useQuery({ queryKey: ['billing-rate-rules'], queryFn: api.listBillingRateRules })
  const currencyUnit = configs['billing.creditsDisplayName']?.trim() || DEFAULT_CURRENCY_UNIT

  return {
    currencyUnit,
    buildMinuteCost: (cpuRequest: string | undefined, memoryRequest: string | undefined) =>
      estimateBuildMinuteCost(rateRules.data ?? [], cpuRequest, memoryRequest),
    formatAmount: (value: string | number | undefined) => formatBillingNumber(value, locale),
    formatAmountWithUnit: (value: string | number | undefined) => `${formatBillingNumber(value, locale)} ${currencyUnit}`,
    formatSignedAmountWithUnit: (value: string | number | undefined) => formatSignedBillingNumber(value, locale, currencyUnit),
    runtimeHourCost: (replicas: number | undefined, cpuRequest: string | undefined, memoryRequest: string | undefined) =>
      estimateRuntimeHourCost(rateRules.data ?? [], replicas, cpuRequest, memoryRequest),
  }
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

function estimateRuntimeHourCost(rules: BillingRateRule[], replicas: number | undefined, cpuRequest: string | undefined, memoryRequest: string | undefined) {
  const replicaCount = Number.isFinite(replicas) && (replicas ?? 0) > 0 ? replicas ?? 1 : 1
  return estimateCost([
    [parseCPUToCores(cpuRequest) * replicaCount, rateForMeter(rules, 'runtime.cpu_vcpu_hour')],
    [parseMemoryToGiB(memoryRequest) * replicaCount, rateForMeter(rules, 'runtime.memory_gib_hour')],
  ])
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
