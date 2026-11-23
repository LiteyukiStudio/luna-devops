import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingChartType, BillingFocusedBucket, BillingPeriodSelection } from './billing-page-utils'
import type {
  BillingSpendGranularity,
  BillingSpendTimeseries,
  BillingSpendTimeseriesBucket,
  BillingSummary as BillingSummaryData,
  GatewayTrafficStatus,
} from '@/api'
import { AlertTriangle, AreaChart, BarChart3, Coins, CreditCard, ExternalLink, TrendingDown, WalletCards, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  balanceStatusTone,
  browserTimeZone,
  bucketDisplayedCredits,
  categorySwatchCssValue,
  categoryTranslatedLabel,
  formatTimeseriesAxisLabel,
  formatTimeseriesBucketRange,
  gatewayTrafficStatusLabel,
  isGranularityAllowedForPeriod,
  normalizeBalanceStatus,
  periodSelectionForPreset,
} from './billing-page-utils'
import { BillingSpendChart } from './billing-spend-chart'

const DOCS_BASE_URL = String(import.meta.env.VITE_DOCS_BASE_URL || 'https://luna-devops.liteyuki.org').replace(/\/+$/, '')
const GATEWAY_TRAFFIC_METRICS_DOC_URL = `${DOCS_BASE_URL}/reference/gateway-traffic-probe`
const GRANULARITY_OPTIONS: readonly BillingSpendGranularity[] = ['hour', 'day', 'month']
const DEFAULT_PERIOD_FALLBACK = periodSelectionForPreset('thisMonth')

function granularityTranslatedLabel(granularity: BillingSpendGranularity, t: (key: string) => string) {
  switch (granularity) {
    case 'hour':
      return t('billingPage.granularities.hour')
    case 'month':
      return t('billingPage.granularities.month')
    case 'day':
    default:
      return t('billingPage.granularities.day')
  }
}

export function BillingSummary({
  accountError = false,
  accountLoading,
  accountRetrying = false,
  accountSummary,
  activeCategory = 'all',
  billingDisplay,
  billingPeriod = DEFAULT_PERIOD_FALLBACK,
  canManageBilling,
  chartType = 'bar',
  focusedBucket = null,
  gatewayTrafficStatus,
  gatewayTrafficStatusLoaded,
  gatewayTrafficStatusLoading = false,
  granularity = 'day',
  locale = 'zh-CN',
  scopedError = false,
  scopedFetching,
  scopedLoading,
  scopedRetrying = false,
  scopedSummary,
  timeseries,
  timeseriesError = false,
  timeseriesErrorMessage,
  timeseriesFetching = false,
  timeseriesLoading = false,
  timeseriesRetrying = false,
  timeZone,
  onActiveCategoryChange,
  onChartTypeChange,
  onClearFocusedBucket,
  onGranularityChange,
  onRetryAccount,
  onRetryScoped,
  onRetryTimeseries,
  onSelectBucket,
}: {
  accountError?: boolean
  accountLoading: boolean
  accountRetrying?: boolean
  accountSummary?: BillingSummaryData
  activeCategory?: string
  billingDisplay: BillingDisplay
  billingPeriod?: BillingPeriodSelection
  canManageBilling: boolean
  chartType?: BillingChartType
  focusedBucket?: BillingFocusedBucket | null
  gatewayTrafficStatus?: GatewayTrafficStatus
  gatewayTrafficStatusLoaded: boolean
  gatewayTrafficStatusLoading?: boolean
  granularity?: BillingSpendGranularity
  locale?: string
  scopedError?: boolean
  scopedFetching: boolean
  scopedLoading: boolean
  scopedRetrying?: boolean
  scopedSummary?: BillingSummaryData
  timeseries?: BillingSpendTimeseries
  timeseriesError?: boolean
  timeseriesErrorMessage?: string
  timeseriesFetching?: boolean
  timeseriesLoading?: boolean
  timeseriesRetrying?: boolean
  timeZone?: string
  onActiveCategoryChange?: (category: string) => void
  onChartTypeChange?: (chartType: BillingChartType) => void
  onClearFocusedBucket?: () => void
  onGranularityChange?: (granularity: BillingSpendGranularity) => void
  onRetryAccount?: () => void
  onRetryScoped?: () => void
  onRetryTimeseries?: () => void
  onSelectBucket?: (bucket: BillingSpendTimeseriesBucket) => void
}) {
  const { t } = useTranslation()
  const resolvedTimeZone = timeZone ?? timeseries?.timeZone ?? browserTimeZone()
  const platformTotal = accountSummary?.balanceScope === 'platform_total'
  const balanceStatus = normalizeBalanceStatus(accountSummary?.balanceStatus)
  const rawCategories = timeseries?.categories ?? scopedSummary?.periodCategories ?? []
  const showGatewayTrafficStatusCard = Boolean(gatewayTrafficStatusLoaded && gatewayTrafficStatus && !gatewayTrafficStatus.available)
  const visibleCategories = rawCategories.filter(category => Number.parseFloat(category.amountCredits) > 0)
  const effectiveActiveCategory = activeCategory === 'all' || visibleCategories.some(item => item.category === activeCategory)
    ? activeCategory
    : 'all'

  const buckets = timeseries?.buckets ?? []
  const totalPeriodNumeric = Number.parseFloat(timeseries?.totalCredits ?? scopedSummary?.periodSpend ?? '0')
  const hasSettledSpend = Number.isFinite(totalPeriodNumeric) && totalPeriodNumeric > 0
  const displayedTotalCredits = effectiveActiveCategory === 'all'
    ? (timeseries?.totalCredits ?? scopedSummary?.periodSpend ?? '0')
    : (visibleCategories.find(item => item.category === effectiveActiveCategory)?.amountCredits ?? '0')

  let peakBucket: BillingSpendTimeseriesBucket | undefined
  let peakValue = 0
  for (const bucket of buckets) {
    const value = bucketDisplayedCredits(bucket, effectiveActiveCategory)
    if (value > peakValue) {
      peakValue = value
      peakBucket = bucket
    }
  }

  const distributionError = scopedError || timeseriesError
  const distributionLoading = scopedLoading
    || timeseriesLoading
    || (scopedFetching && !scopedSummary && !scopedError)
    || (timeseriesFetching && !timeseries && !timeseriesError)
    || (!distributionError && rawCategories.length === 0 && gatewayTrafficStatusLoading)

  if (accountError && scopedError) {
    return (
      <ErrorState
        retrying={accountRetrying || scopedRetrying}
        title={t('errors.request.failed')}
        onRetry={() => {
          onRetryAccount?.()
          onRetryScoped?.()
          if (timeseriesError)
            onRetryTimeseries?.()
        }}
      />
    )
  }

  return (
    <>
      {accountError && (
        <ErrorState
          retrying={accountRetrying}
          title={t('errors.request.failed')}
          onRetry={onRetryAccount}
        />
      )}
      <MetricGroup className="@xl/metric-group:grid-cols-4">
        <MetricItem
          icon={<Coins className="size-4" />}
          label={platformTotal ? t('billingPage.platformWalletTotal') : t('billingPage.balance')}
          meta={accountLoading
            ? (canManageBilling ? <Skeleton className="h-4 w-16" /> : undefined)
            : !accountError && accountSummary && canManageBilling
                ? billingDisplay.formatFiatAmount(accountSummary.balanceCredits)
                : undefined}
          value={accountLoading
            ? <Skeleton className="h-7 w-24" />
            : accountError || !accountSummary
              ? t('common.unavailable')
              : billingDisplay.formatAmountWithUnit(accountSummary.balanceCredits)}
        />
        <MetricItem
          icon={<TrendingDown className="size-4" />}
          label={t('billingPage.periodSpend')}
          meta={scopedLoading
            ? (canManageBilling ? <Skeleton className="h-4 w-16" /> : undefined)
            : !scopedError && scopedSummary && canManageBilling
                ? billingDisplay.formatFiatAmount(scopedSummary.periodSpend)
                : undefined}
          value={scopedLoading
            ? <Skeleton className="h-7 w-24" />
            : scopedError || !scopedSummary
              ? t('common.unavailable')
              : billingDisplay.formatAmountWithUnit(scopedSummary.periodSpend)}
        />
        <MetricItem
          icon={<CreditCard className="size-4" />}
          label={t('billingPage.todaySpend')}
          meta={scopedLoading
            ? (canManageBilling ? <Skeleton className="h-4 w-16" /> : undefined)
            : !scopedError && scopedSummary && canManageBilling
                ? billingDisplay.formatFiatAmount(scopedSummary.todaySpend)
                : undefined}
          value={scopedLoading
            ? <Skeleton className="h-7 w-24" />
            : scopedError || !scopedSummary
              ? t('common.unavailable')
              : billingDisplay.formatAmountWithUnit(scopedSummary.todaySpend)}
        />
        <MetricItem
          icon={<WalletCards className="size-4" />}
          label={t('billingPage.pendingSpend')}
          meta={scopedLoading
            ? (canManageBilling ? <Skeleton className="h-4 w-16" /> : undefined)
            : !scopedError && scopedSummary && canManageBilling
                ? billingDisplay.formatFiatAmount(scopedSummary.pendingSpend)
                : undefined}
          value={scopedLoading
            ? <Skeleton className="h-7 w-24" />
            : scopedError || !scopedSummary
              ? t('common.unavailable')
              : billingDisplay.formatAmountWithUnit(scopedSummary.pendingSpend)}
        />
      </MetricGroup>

      <Section
        description={!distributionLoading && !distributionError && hasSettledSpend
          ? (
              <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs tabular-nums">
                <span>
                  {t('billingPage.spendDistributionTotal', {
                    amount: billingDisplay.formatAmountWithUnit(displayedTotalCredits),
                  })}
                </span>
                {peakBucket && peakValue > 0 && (
                  <button
                    className="inline-flex items-center rounded-control border border-border bg-surface-subtle px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:border-primary-border hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"
                    type="button"
                    onClick={() => onSelectBucket?.(peakBucket)}
                  >
                    {t('billingPage.spendPeakLabel', {
                      amount: billingDisplay.formatAmountWithUnit(peakValue),
                      bucket: formatTimeseriesAxisLabel(peakBucket.bucketStart, granularity, locale, resolvedTimeZone),
                    })}
                  </button>
                )}
              </div>
            )
          : undefined}
        title={t('billingPage.spendDistributionTitle')}
        tools={(
          <div className="flex flex-wrap items-center gap-2">
            {!accountLoading && !accountError && accountSummary && !platformTotal && (
              <StatusBadge tone={balanceStatusTone(balanceStatus)}>
                {t(`billingPage.balanceStatuses.${balanceStatus}`)}
              </StatusBadge>
            )}
            <div
              aria-label={t('billingPage.granularityGroupLabel')}
              className="inline-flex items-center rounded-full border border-border bg-surface-subtle p-0.5"
              role="group"
            >
              {GRANULARITY_OPTIONS.map((option) => {
                const allowed = isGranularityAllowedForPeriod(option, billingPeriod)
                const active = granularity === option
                return (
                  <button
                    key={option}
                    aria-pressed={active}
                    className={cn(
                      'h-7 rounded-full px-3 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-ring',
                      !allowed && 'cursor-not-allowed text-muted-foreground/50',
                      allowed && active && 'border border-primary-border bg-surface-raised text-primary-text shadow-sm',
                      allowed && !active && 'text-muted-foreground hover:text-foreground',
                    )}
                    disabled={!allowed}
                    title={!allowed ? t('billingPage.granularityDisabledHint') : undefined}
                    type="button"
                    onClick={() => {
                      if (allowed)
                        onGranularityChange?.(option)
                    }}
                  >
                    {granularityTranslatedLabel(option, t)}
                  </button>
                )
              })}
            </div>

            <div
              aria-label={t('billingPage.chartTypeGroupLabel')}
              className="inline-flex items-center rounded-full border border-border bg-surface-subtle p-0.5"
              role="group"
            >
              <button
                aria-pressed={chartType === 'bar'}
                className={cn(
                  'inline-flex h-7 items-center gap-1.5 rounded-full px-3 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-ring',
                  chartType === 'bar'
                    ? 'border border-primary-border bg-surface-raised text-primary-text shadow-sm'
                    : 'text-muted-foreground hover:text-foreground',
                )}
                type="button"
                onClick={() => onChartTypeChange?.('bar')}
              >
                <BarChart3 className="size-3.5" />
                <span>{t('billingPage.chartTypes.bar')}</span>
              </button>
              <button
                aria-pressed={chartType === 'area'}
                className={cn(
                  'inline-flex h-7 items-center gap-1.5 rounded-full px-3 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-ring',
                  chartType === 'area'
                    ? 'border border-primary-border bg-surface-raised text-primary-text shadow-sm'
                    : 'text-muted-foreground hover:text-foreground',
                )}
                type="button"
                onClick={() => onChartTypeChange?.('area')}
              >
                <AreaChart className="size-3.5" />
                <span>{t('billingPage.chartTypes.area')}</span>
              </button>
            </div>
          </div>
        )}
        variant="bordered"
      >
        {distributionLoading
          ? (
              <div aria-label={t('common.loading')} className="grid gap-4" role="status">
                <div className="flex flex-wrap gap-2">
                  {Array.from({ length: 4 }, (_, index) => (
                    <Skeleton key={index} className="h-8 w-28 rounded-control" />
                  ))}
                </div>
                <Skeleton className="h-64 w-full rounded-container" />
              </div>
            )
          : distributionError
            ? (
                <ErrorState
                  description={timeseriesError ? timeseriesErrorMessage : undefined}
                  retrying={scopedRetrying || timeseriesRetrying}
                  title={t('errors.request.failed')}
                  onRetry={() => {
                    if (scopedError)
                      onRetryScoped?.()
                    if (timeseriesError)
                      onRetryTimeseries?.()
                  }}
                />
              )
            : (
                <div className="grid gap-4">
                  {(visibleCategories.length > 0 || showGatewayTrafficStatusCard) && (
                    <div
                      aria-label={t('billingPage.categoryFilterGroupLabel')}
                      className="flex flex-wrap items-center gap-2"
                      role="group"
                    >
                      {visibleCategories.length > 0 && (
                        <button
                          aria-pressed={effectiveActiveCategory === 'all'}
                          className={cn(
                            'inline-flex h-8 items-center gap-2 rounded-control border px-3 text-xs tabular-nums transition-colors focus-visible:outline-2 focus-visible:outline-ring',
                            effectiveActiveCategory === 'all'
                              ? 'border-primary-border bg-primary-subtle font-medium text-foreground shadow-sm'
                              : 'border-border bg-surface-subtle text-muted-foreground hover:text-foreground',
                          )}
                          type="button"
                          onClick={() => onActiveCategoryChange?.('all')}
                        >
                          <span>{t('billingPage.allCategories')}</span>
                          <span className="font-semibold text-foreground">
                            {billingDisplay.formatAmountWithUnit(timeseries?.totalCredits ?? scopedSummary?.periodSpend ?? '0')}
                          </span>
                        </button>
                      )}
                      {visibleCategories.map((category) => {
                        const selected = effectiveActiveCategory === category.category
                        return (
                          <button
                            key={category.category}
                            aria-pressed={selected}
                            className={cn(
                              'inline-flex h-8 items-center gap-2 rounded-control border px-3 text-xs tabular-nums transition-colors focus-visible:outline-2 focus-visible:outline-ring',
                              selected
                                ? 'border-primary-border bg-primary-subtle font-medium text-foreground shadow-sm'
                                : 'border-border bg-surface-subtle text-muted-foreground hover:text-foreground',
                            )}
                            type="button"
                            onClick={() => onActiveCategoryChange?.(selected ? 'all' : category.category)}
                          >
                            <span
                              aria-hidden="true"
                              className="size-2.5 shrink-0 rounded-[2px]"
                              style={{ backgroundColor: categorySwatchCssValue(category.category) }}
                            />
                            <span>{categoryTranslatedLabel(category.category, t)}</span>
                            <span className="font-semibold text-foreground">
                              {billingDisplay.formatAmountWithUnit(category.amountCredits)}
                            </span>
                          </button>
                        )
                      })}
                      {showGatewayTrafficStatusCard && gatewayTrafficStatus && (
                        <div className="inline-flex min-h-8 flex-wrap items-center gap-2 rounded-control border border-dashed border-border bg-surface-subtle px-3 py-1 text-xs">
                          <span className="text-muted-foreground">{t('billingPage.categories.gateway')}</span>
                          <span className="font-medium text-foreground">{gatewayTrafficStatusLabel(gatewayTrafficStatus, t)}</span>
                          <a
                            className="inline-flex items-center gap-1 font-medium text-primary-text transition-colors hover:text-primary-text/80"
                            href={GATEWAY_TRAFFIC_METRICS_DOC_URL}
                            rel="noreferrer"
                            target="_blank"
                          >
                            {t('billingPage.gatewayTrafficMetricsDocs')}
                            <ExternalLink className="size-3" />
                          </a>
                        </div>
                      )}
                    </div>
                  )}

                  {hasSettledSpend && buckets.length > 0
                    ? (
                        <BillingSpendChart
                          activeCategory={effectiveActiveCategory}
                          billingDisplay={billingDisplay}
                          buckets={buckets}
                          canManageBilling={canManageBilling}
                          chartType={chartType}
                          focusedBucketStart={focusedBucket?.bucketStart}
                          granularity={granularity}
                          locale={locale}
                          timeZone={resolvedTimeZone}
                          onSelectBucket={onSelectBucket}
                        />
                      )
                    : (
                        <EmptyState
                          description={t('billingPage.emptyPeriodCategoriesDescription')}
                          icon={<BarChart3 className="size-5" />}
                          mode="filtered"
                          title={t('billingPage.emptyPeriodCategories')}
                          variant="plain"
                        />
                      )}

                  {focusedBucket && (
                    <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3 text-xs">
                      <span className="inline-flex items-center rounded-control bg-primary-subtle px-2.5 py-1 font-medium text-foreground tabular-nums">
                        {t('billingPage.focusedBucketBadge', {
                          period: formatTimeseriesBucketRange(focusedBucket, granularity, locale, resolvedTimeZone),
                        })}
                      </span>
                      <Button size="sm" type="button" variant="ghost" onClick={onClearFocusedBucket}>
                        <X className="size-3.5" />
                        {t('billingPage.clearFocusedBucket')}
                      </Button>
                    </div>
                  )}
                </div>
              )}
      </Section>
    </>
  )
}

export function BillingBalanceWarning({ accountSummary, billingDisplay }: {
  accountSummary?: BillingSummaryData
  billingDisplay: BillingDisplay
}) {
  const { t } = useTranslation()
  const balanceStatus = normalizeBalanceStatus(accountSummary?.balanceStatus)
  if (!accountSummary || accountSummary.balanceScope === 'platform_total' || balanceStatus === 'ok')
    return null

  return (
    <Alert variant={balanceStatus === 'insufficient' ? 'destructive' : 'warning'}>
      <AlertTriangle />
      <AlertTitle className="flex flex-wrap items-center gap-2 leading-5">
        <span>{t(`billingPage.balanceStatuses.${balanceStatus}`)}</span>
        <StatusBadge tone={balanceStatus === 'insufficient' ? 'danger' : 'warning'}>
          {billingDisplay.formatAmountWithUnit(accountSummary.availableCredits)}
        </StatusBadge>
      </AlertTitle>
      <AlertDescription className="min-w-0">
        {t('billingPage.balanceWarningDescription', {
          pending: billingDisplay.formatAmountWithUnit(accountSummary.pendingSpend),
          threshold: billingDisplay.formatAmountWithUnit(accountSummary.lowBalanceLimit),
        })}
      </AlertDescription>
    </Alert>
  )
}
