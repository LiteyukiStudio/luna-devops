import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingSummary as BillingSummaryData, GatewayTrafficStatus } from '@/api'
import { AlertTriangle, Coins, CreditCard, ExternalLink, TrendingDown, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ErrorState } from '@/components/common/error-state'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { balanceStatusTone, gatewayTrafficStatusLabel, normalizeBalanceStatus } from './billing-page-utils'

const DOCS_BASE_URL = String(import.meta.env.VITE_DOCS_BASE_URL || 'https://luna-devops.liteyuki.org').replace(/\/+$/, '')
const GATEWAY_TRAFFIC_METRICS_DOC_URL = `${DOCS_BASE_URL}/reference/gateway-traffic-probe`

export function BillingSummary({
  accountError = false,
  accountLoading,
  accountRetrying = false,
  accountSummary,
  billingDisplay,
  canManageBilling,
  gatewayTrafficStatus,
  gatewayTrafficStatusLoaded,
  gatewayTrafficStatusLoading = false,
  scopedError = false,
  scopedFetching,
  scopedLoading,
  scopedRetrying = false,
  scopedSummary,
  onRetryAccount,
  onRetryScoped,
}: {
  accountError?: boolean
  accountLoading: boolean
  accountRetrying?: boolean
  accountSummary?: BillingSummaryData
  billingDisplay: BillingDisplay
  canManageBilling: boolean
  gatewayTrafficStatus?: GatewayTrafficStatus
  gatewayTrafficStatusLoaded: boolean
  gatewayTrafficStatusLoading?: boolean
  scopedError?: boolean
  scopedFetching: boolean
  scopedLoading: boolean
  scopedRetrying?: boolean
  scopedSummary?: BillingSummaryData
  onRetryAccount?: () => void
  onRetryScoped?: () => void
}) {
  const { t } = useTranslation()
  const platformTotal = accountSummary?.balanceScope === 'platform_total'
  const balanceStatus = normalizeBalanceStatus(accountSummary?.balanceStatus)
  const periodCategories = scopedSummary?.periodCategories ?? []
  const showGatewayTrafficStatusCard = gatewayTrafficStatusLoaded && gatewayTrafficStatus && !gatewayTrafficStatus.available

  if (accountError && scopedError) {
    return (
      <ErrorState
        retrying={accountRetrying || scopedRetrying}
        title={t('errors.request.failed')}
        onRetry={() => {
          onRetryAccount?.()
          onRetryScoped?.()
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
        title={t('billingPage.periodCategoriesTitle')}
        tools={!accountLoading && !accountError && accountSummary && !platformTotal
          ? (
              <StatusBadge tone={balanceStatusTone(balanceStatus)}>
                {t(`billingPage.balanceStatuses.${balanceStatus}`)}
              </StatusBadge>
            )
          : undefined}
        variant="bordered"
      >
        {scopedLoading || (scopedFetching && !scopedSummary && !scopedError) || (!scopedError && periodCategories.length === 0 && gatewayTrafficStatusLoading)
          ? (
              <div aria-label={t('common.loading')} className="grid gap-x-6 gap-y-4 py-1 @lg/workspace:grid-cols-3 @4xl/workspace:grid-cols-6" role="status">
                {Array.from({ length: 4 }, (_, index) => (
                  <div key={index} className="grid gap-1.5">
                    <Skeleton className="h-3.5 w-16" />
                    <Skeleton className="h-6 w-24" />
                  </div>
                ))}
              </div>
            )
          : scopedError
            ? (
                <ErrorState
                  retrying={scopedRetrying}
                  title={t('errors.request.failed')}
                  onRetry={onRetryScoped}
                />
              )
            : periodCategories.length === 0 && !showGatewayTrafficStatusCard
              ? (
                  <p className="py-4 text-sm text-muted-foreground">
                    {t('billingPage.emptyPeriodCategories')}
                  </p>
                )
              : (
                  <dl className="grid gap-x-6 gap-y-4 @lg/workspace:grid-cols-3 @4xl/workspace:grid-cols-6">
                    {periodCategories.filter(category => !(showGatewayTrafficStatusCard && category.category === 'gateway')).map(category => (
                      <div key={category.category} className="min-w-0">
                        <dt className="truncate text-xs text-muted-foreground">
                          {t(`billingPage.categories.${category.category}`, { defaultValue: category.category })}
                        </dt>
                        <dd className="mt-1 truncate text-lg font-semibold tabular-nums text-foreground">
                          {billingDisplay.formatAmountWithUnit(category.amountCredits)}
                        </dd>
                      </div>
                    ))}
                    {showGatewayTrafficStatusCard && (
                      <div className="min-w-0">
                        <dt className="truncate text-xs text-muted-foreground">
                          {t('billingPage.categories.gateway')}
                        </dt>
                        <dd className="mt-1 grid gap-1">
                          <span className="truncate text-lg font-semibold text-foreground">
                            {gatewayTrafficStatusLabel(gatewayTrafficStatus, t)}
                          </span>
                          <a
                            className="inline-flex w-fit items-center gap-1 text-xs font-medium text-primary-text transition-colors hover:text-primary-text/80"
                            href={GATEWAY_TRAFFIC_METRICS_DOC_URL}
                            rel="noreferrer"
                            target="_blank"
                          >
                            {t('billingPage.gatewayTrafficMetricsDocs')}
                            <ExternalLink className="size-3" />
                          </a>
                        </dd>
                      </div>
                    )}
                  </dl>
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
