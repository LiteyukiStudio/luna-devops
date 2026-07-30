import type { BillingLedgerEntry, GatewayTrafficStatus } from '@/api'
import type { StatusTone } from '@/components/common/status-tone'
import { formatBillingNumber } from '@/lib/billing-display'

const BILLING_PROJECT_SCOPE_CACHE_KEY = 'luna.billing.projectScope'
export const BILLING_PAGE_SIZE = 10

export type BillingPeriodPreset = 'thisWeek' | 'last7Days' | 'thisMonth' | 'last30Days' | 'thisYear' | 'lastYear' | 'custom'

export interface BillingPeriodSelection {
  endDate: string
  preset: BillingPeriodPreset
  startDate: string
}

export type BalanceStatus = 'ok' | 'low' | 'insufficient'

export function gatewayTrafficStatusLabel(status: GatewayTrafficStatus, t: (key: string, options?: Record<string, unknown>) => string) {
  if (!status.installed)
    return t('billingPage.gatewayTrafficProbeStates.notInstalled')
  if (status.status === 'ready' && !status.available)
    return t('billingPage.gatewayTrafficProbeStates.waitingReport')
  return t(`billingPage.gatewayTrafficStatuses.${status.status || 'unknown'}`, { defaultValue: status.status || t('common.unknown') })
}

export function ledgerReasonLabel(item: BillingLedgerEntry, t: (key: string, options?: Record<string, unknown>) => string) {
  if (item.reason.endsWith('.usage') && item.meter)
    return t(`billingPage.meters.${item.meter}`, { defaultValue: item.meter })

  return t(`billingPage.reasons.${item.reason}`, { defaultValue: item.reason || '-' })
}

export function formatQuantity(value: string, unit: string, locale: string) {
  const formatted = formatBillingNumber(value, locale)
  return unit ? `${formatted} ${unit}` : formatted
}

export function amountToneClass(value: string) {
  const numeric = Number.parseFloat(value)
  if (numeric < 0)
    return 'text-destructive'
  if (numeric > 0)
    return 'text-emerald-600 dark:text-emerald-400'
  return 'text-muted-foreground'
}

export function normalizeBalanceStatus(status: string | undefined): BalanceStatus {
  if (status === 'low' || status === 'insufficient')
    return status
  return 'ok'
}

export function balanceStatusTone(status: BalanceStatus): StatusTone {
  if (status === 'insufficient')
    return 'danger'
  if (status === 'low')
    return 'warning'
  return 'success'
}

export function readCachedBillingProjectScope() {
  if (typeof window === 'undefined')
    return []
  try {
    const cached = window.sessionStorage.getItem(BILLING_PROJECT_SCOPE_CACHE_KEY)
    if (!cached)
      return []
    const parsed = JSON.parse(cached)
    if (!Array.isArray(parsed))
      return []
    return parsed.map(item => String(item).trim()).filter(Boolean)
  }
  catch {
    return []
  }
}

export function writeCachedBillingProjectScope(projectIds: string[]) {
  if (typeof window === 'undefined')
    return
  try {
    const normalized = projectIds.map(item => item.trim()).filter(Boolean)
    if (normalized.length === 0) {
      window.sessionStorage.removeItem(BILLING_PROJECT_SCOPE_CACHE_KEY)
      return
    }
    window.sessionStorage.setItem(BILLING_PROJECT_SCOPE_CACHE_KEY, JSON.stringify(normalized))
  }
  catch {
    // Ignore storage errors so private mode or quota issues do not break billing.
  }
}

export function periodSelectionForPreset(preset: BillingPeriodPreset): BillingPeriodSelection {
  const today = startOfLocalDay(new Date())
  const tomorrow = addDays(today, 1)
  switch (preset) {
    case 'thisWeek':
      return periodSelection(preset, startOfLocalWeek(today), today)
    case 'last7Days':
      return periodSelection(preset, addDays(today, -6), today)
    case 'last30Days':
      return periodSelection(preset, addDays(today, -29), today)
    case 'thisYear':
      return periodSelection(preset, new Date(today.getFullYear(), 0, 1), today)
    case 'lastYear':
      return periodSelection(preset, new Date(today.getFullYear() - 1, 0, 1), addDays(new Date(today.getFullYear(), 0, 1), -1))
    case 'custom':
    case 'thisMonth':
    default:
      return {
        endDate: formatDateInput(addDays(tomorrow, -1)),
        preset: 'thisMonth',
        startDate: formatDateInput(new Date(today.getFullYear(), today.getMonth(), 1)),
      }
  }
}

export function billingPeriodToQuery(period: BillingPeriodSelection) {
  const start = parseDateInput(period.startDate)
  const endInclusive = parseDateInput(period.endDate)
  if (!start || !endInclusive)
    return {}
  const endExclusive = addDays(endInclusive, 1)
  return {
    periodEnd: endExclusive.toISOString(),
    periodStart: start.toISOString(),
  }
}

function periodSelection(preset: BillingPeriodPreset, start: Date, endInclusive: Date): BillingPeriodSelection {
  return {
    endDate: formatDateInput(endInclusive),
    preset,
    startDate: formatDateInput(start),
  }
}

function startOfLocalDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

function startOfLocalWeek(date: Date) {
  const dayOffset = (date.getDay() + 6) % 7
  return addDays(startOfLocalDay(date), -dayOffset)
}

function addDays(date: Date, days: number) {
  const next = new Date(date)
  next.setDate(next.getDate() + days)
  return next
}

function formatDateInput(date: Date) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function parseDateInput(value: string) {
  const [year, month, day] = value.split('-').map(part => Number.parseInt(part, 10))
  if (!year || !month || !day)
    return undefined
  return new Date(year, month - 1, day)
}
