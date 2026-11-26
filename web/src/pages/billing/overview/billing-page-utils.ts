import type { BillingLedgerEntry, BillingSpendCategory, BillingSpendGranularity, BillingSpendTimeseriesBucket, GatewayTrafficStatus } from '@/api'
import type { StatusTone } from '@/components/common/status-tone'
import { formatBillingNumber } from '@/lib/billing-display'
import { safeStorageGet, safeStorageRemove, safeStorageSet } from '@/lib/safe-storage'

const BILLING_PROJECT_SCOPE_CACHE_KEY = 'luna.billing.projectScope'
export const BILLING_PAGE_SIZE = 20
const MAX_TIMESERIES_BUCKETS = 800

export type BillingLedgerTypeFilter = 'all' | 'debit' | 'credit' | 'adjustment'
export type BillingLedgerCategoryFilter = 'all' | (typeof BILLING_CATEGORY_ORDER)[number]

export type BillingPeriodPreset
  = | 'last24Hours'
    | 'today'
    | 'thisWeek'
    | 'last7Days'
    | 'thisMonth'
    | 'last30Days'
    | 'thisYear'
    | 'lastYear'
    | 'custom'

export type BillingChartType = 'bar' | 'area'

export interface BillingPeriodSelection {
  endDate: string
  preset: BillingPeriodPreset
  startDate: string
  /** Exact RFC3339 bounds for rolling sub-day presets such as last24Hours. */
  windowEndIso?: string
  windowStartIso?: string
}

export interface BillingFocusedBucket {
  bucketStart: string
  periodEnd: string
  periodStart: string
}

export type BalanceStatus = 'ok' | 'low' | 'insufficient'

export const BILLING_CATEGORY_ORDER = ['runtime', 'ai', 'build', 'storage', 'gateway', 'adjustment', 'other'] as const
export const BILLING_PROJECT_CATEGORY_ORDER = ['runtime', 'build', 'storage', 'gateway', 'other'] as const
export const BILLING_DEPLOYMENT_STAGE_OPTIONS = ['dev', 'test', 'staging', 'prod'] as const
export type BillingDeploymentStageFilter = 'all' | (typeof BILLING_DEPLOYMENT_STAGE_OPTIONS)[number]

export const BILLING_CATEGORY_COLOR_VARS: Record<string, { fallback: string, variable: string }> = {
  adjustment: { fallback: '#e5484d', variable: '--status-danger-solid' },
  ai: { fallback: '#5b5bd6', variable: '--topology-cat-core' },
  build: { fallback: '#009e73', variable: '--topology-cat-support' },
  gateway: { fallback: '#0072b2', variable: '--topology-cat-access' },
  other: { fallback: '#8b94a3', variable: '--topology-cat-infra' },
  runtime: { fallback: '#2563eb', variable: '--primary' },
  storage: { fallback: '#d97706', variable: '--status-warning-solid' },
}

export function categorySwatchCssValue(category: string) {
  const entry = BILLING_CATEGORY_COLOR_VARS[category] ?? BILLING_CATEGORY_COLOR_VARS.other
  return `var(${entry.variable}, ${entry.fallback})`
}

export function categoryTranslatedLabel(category: string, t: (key: string, options?: Record<string, unknown>) => string) {
  switch (category) {
    case 'build':
      return t('billingPage.categories.build')
    case 'runtime':
      return t('billingPage.categories.runtime')
    case 'storage':
      return t('billingPage.categories.storage')
    case 'gateway':
      return t('billingPage.categories.gateway')
    case 'ai':
      return t('billingPage.categories.ai')
    case 'adjustment':
      return t('billingPage.categories.adjustment')
    case 'other':
      return t('billingPage.categories.other')
    default:
      return category
  }
}

export function deploymentStageTranslatedLabel(stage: string, t: (key: string) => string) {
  switch (stage) {
    case 'dev':
      return t('deploymentsPage.stageLabels.dev')
    case 'test':
      return t('deploymentsPage.stageLabels.test')
    case 'staging':
      return t('deploymentsPage.stageLabels.staging')
    case 'prod':
      return t('deploymentsPage.stageLabels.prod')
    default:
      return stage
  }
}

export function gatewayTrafficStatusLabel(status: GatewayTrafficStatus, t: (key: string, options?: Record<string, unknown>) => string) {
  if (!status.installed)
    return t('billingPage.gatewayTrafficProbeStates.notInstalled')
  if (status.status === 'ready' && !status.available)
    return t('billingPage.gatewayTrafficProbeStates.waitingReport')
  return t(`billingPage.gatewayTrafficStatuses.${status.status || 'unknown'}`, { defaultValue: status.status || t('common.unknown') })
}

export function ledgerReasonLabel(item: BillingLedgerEntry, t: (key: string, options?: Record<string, unknown>) => string) {
  if ((item.reason.endsWith('.usage') || item.reason.endsWith('_usage')) && item.meter)
    return t(`billingPage.meters.${item.meter}`, { defaultValue: item.meter })

  return t(`billingPage.reasons.${item.reason}`, { defaultValue: item.reason || '-' })
}

export function ledgerDescription(item: BillingLedgerEntry, reasonLabel: string) {
  const description = item.description.trim()
  if (!description || description.localeCompare(reasonLabel.trim(), undefined, { sensitivity: 'accent' }) === 0)
    return ''
  return description
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
    return 'text-success'
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
  try {
    const cached = safeStorageGet(BILLING_PROJECT_SCOPE_CACHE_KEY, () => window.sessionStorage)
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
  const normalized = projectIds.map(item => item.trim()).filter(Boolean)
  if (normalized.length === 0) {
    safeStorageRemove(BILLING_PROJECT_SCOPE_CACHE_KEY, () => window.sessionStorage)
    return
  }
  safeStorageSet(BILLING_PROJECT_SCOPE_CACHE_KEY, JSON.stringify(normalized), () => window.sessionStorage)
}

export function periodSelectionForPreset(preset: BillingPeriodPreset, now = new Date()): BillingPeriodSelection {
  const today = startOfLocalDay(now)
  const tomorrow = addDays(today, 1)
  switch (preset) {
    case 'last24Hours': {
      const endHour = new Date(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours() + 1, 0, 0, 0)
      const startHour = new Date(endHour.getTime() - 24 * 3_600_000)
      return {
        endDate: formatDateInput(now),
        preset: 'last24Hours',
        startDate: formatDateInput(startHour),
        windowEndIso: endHour.toISOString(),
        windowStartIso: startHour.toISOString(),
      }
    }
    case 'today':
      return periodSelection('today', today, today)
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

export function browserTimeZone() {
  return new Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
}

export function billingPeriodToQuery(period: BillingPeriodSelection) {
  if (period.windowStartIso && period.windowEndIso) {
    return {
      periodEnd: period.windowEndIso,
      periodStart: period.windowStartIso,
    }
  }
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

export function periodDurationHours(period: BillingPeriodSelection) {
  const query = billingPeriodToQuery(period)
  if (!query.periodStart || !query.periodEnd)
    return 24 * 30
  const startMs = Date.parse(query.periodStart)
  const endMs = Date.parse(query.periodEnd)
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs)
    return 24
  return (endMs - startMs) / 3_600_000
}

function periodCalendarMonths(period: BillingPeriodSelection) {
  const query = billingPeriodToQuery(period)
  if (!query.periodStart || !query.periodEnd)
    return 1
  const start = new Date(query.periodStart)
  const endInclusive = new Date(Date.parse(query.periodEnd) - 1)
  if (Number.isNaN(start.getTime()) || Number.isNaN(endInclusive.getTime()))
    return 1
  return Math.max(1, (endInclusive.getFullYear() - start.getFullYear()) * 12 + (endInclusive.getMonth() - start.getMonth()) + 1)
}

export function isGranularityAllowedForPeriod(granularity: BillingSpendGranularity, period: BillingPeriodSelection) {
  const hours = periodDurationHours(period)
  if (granularity === 'hour')
    return hours <= MAX_TIMESERIES_BUCKETS
  if (granularity === 'day')
    return hours / 24 <= MAX_TIMESERIES_BUCKETS
  return periodCalendarMonths(period) <= MAX_TIMESERIES_BUCKETS
}

export function defaultGranularityForPeriod(period: BillingPeriodSelection): BillingSpendGranularity {
  if (period.preset === 'last24Hours' || period.preset === 'today')
    return 'hour'
  const hours = periodDurationHours(period)
  if (hours <= 72)
    return 'hour'
  if (hours > 92 * 24)
    return 'month'
  return 'day'
}

export function bucketCategoryAmount(categories: readonly BillingSpendCategory[], category: string) {
  const found = categories.find(item => item.category === category)
  if (!found)
    return 0
  const parsed = Number.parseFloat(found.amountCredits)
  return Number.isFinite(parsed) ? parsed : 0
}

export function bucketDisplayedCredits(bucket: BillingSpendTimeseriesBucket, activeCategory: string) {
  if (activeCategory === 'all') {
    const parsed = Number.parseFloat(bucket.totalCredits)
    return Number.isFinite(parsed) ? parsed : 0
  }
  return bucketCategoryAmount(bucket.categories, activeCategory)
}

export function formatTimeseriesAxisLabel(iso: string, granularity: BillingSpendGranularity, locale: string, timeZone: string) {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime()))
    return iso
  try {
    if (granularity === 'hour') {
      return new Intl.DateTimeFormat(locale, {
        day: '2-digit',
        hour: '2-digit',
        hourCycle: 'h23',
        minute: '2-digit',
        month: '2-digit',
        timeZone,
      }).format(date)
    }
    if (granularity === 'month') {
      return new Intl.DateTimeFormat(locale, {
        month: '2-digit',
        timeZone,
        year: 'numeric',
      }).format(date)
    }
    return new Intl.DateTimeFormat(locale, {
      day: '2-digit',
      month: '2-digit',
      timeZone,
    }).format(date)
  }
  catch {
    return iso.slice(0, 16)
  }
}

export function formatTimeseriesBucketRange(
  bucket: Pick<BillingSpendTimeseriesBucket, 'bucketStart' | 'periodEnd' | 'periodStart'>,
  granularity: BillingSpendGranularity,
  locale: string,
  timeZone: string,
) {
  const bucketStart = new Date(bucket.bucketStart)
  const start = new Date(bucket.periodStart)
  const end = new Date(bucket.periodEnd)
  if (Number.isNaN(bucketStart.getTime()) || Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()))
    return bucket.bucketStart

  const durationHours = (end.getTime() - start.getTime()) / 3_600_000
  const startClamped = start.getTime() !== bucketStart.getTime()

  try {
    if (granularity === 'hour') {
      const startText = new Intl.DateTimeFormat(locale, {
        day: '2-digit',
        hour: '2-digit',
        hourCycle: 'h23',
        minute: '2-digit',
        month: '2-digit',
        timeZone,
      }).format(start)
      const endText = new Intl.DateTimeFormat(locale, {
        hour: '2-digit',
        hourCycle: 'h23',
        minute: '2-digit',
        timeZone,
      }).format(end)
      return `${startText} – ${endText}`
    }

    if (granularity === 'day') {
      // Sub-day clipped bucket (e.g. rolling last24Hours viewed in daily mode)
      if (startClamped || durationHours < 23) {
        const fmt = new Intl.DateTimeFormat(locale, {
          day: '2-digit',
          hour: '2-digit',
          hourCycle: 'h23',
          minute: '2-digit',
          month: '2-digit',
          timeZone,
        })
        return `${fmt.format(start)} – ${fmt.format(end)}`
      }
      return new Intl.DateTimeFormat(locale, {
        day: '2-digit',
        month: '2-digit',
        timeZone,
        year: 'numeric',
      }).format(bucketStart)
    }

    // Monthly bucket: check whether periodStart or periodEnd clipped the calendar month
    const endIsFirstOfMonthMidnight = isFirstOfMonthMidnightInTimeZone(end, timeZone)
    if (startClamped || !endIsFirstOfMonthMidnight) {
      const lastInclusive = new Date(Math.max(start.getTime(), end.getTime() - 1))
      const dateFmt = new Intl.DateTimeFormat(locale, {
        day: '2-digit',
        month: '2-digit',
        timeZone,
        year: 'numeric',
      })
      const startDay = dateFmt.format(start)
      const endDay = dateFmt.format(lastInclusive)
      return startDay === endDay ? startDay : `${startDay} – ${endDay}`
    }

    return new Intl.DateTimeFormat(locale, {
      month: 'short',
      timeZone,
      year: 'numeric',
    }).format(bucketStart)
  }
  catch {
    return bucket.bucketStart.slice(0, 10)
  }
}

function isFirstOfMonthMidnightInTimeZone(date: Date, timeZone: string) {
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      day: 'numeric',
      hour: 'numeric',
      hourCycle: 'h23',
      minute: 'numeric',
      second: 'numeric',
      timeZone,
    }).formatToParts(date)
    const getNum = (type: string) => Number.parseInt(parts.find(p => p.type === type)?.value ?? '-1', 10)
    return getNum('day') === 1 && getNum('hour') === 0 && getNum('minute') === 0 && getNum('second') === 0
  }
  catch {
    return false
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
