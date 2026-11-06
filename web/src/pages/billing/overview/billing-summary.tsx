import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingSummary as BillingSummaryData, GatewayTrafficStatus } from '@/api'
import { AlertTriangle, Coins, CreditCard, ExternalLink, TrendingDown, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { balanceStatusTone, gatewayTrafficStatusLabel, normalizeBalanceStatus } from './billing-page-utils'

const DOCS_BASE_URL = String(import.meta.env.VITE_DOCS_BASE_URL || 'https://luna-devops.liteyuki.org').replace(/\/+$/, '')
const GATEWAY_TRAFFIC_METRICS_DOC_URL = `${DOCS_BASE_URL}/reference/gateway-traffic-probe`

export function BillingSummary({
  accountLoading,
  accountSummary,
  billingDisplay,
  canManageBilling,
  gatewayTrafficStatus,
  gatewayTrafficStatusLoaded,
  scopedFetching,
  scopedLoading,
  scopedSummary,
}: {
  accountLoading: boolean
  accountSummary?: BillingSummaryData
  billingDisplay: BillingDisplay
  canManageBilling: boolean
  gatewayTrafficStatus?: GatewayTrafficStatus
  gatewayTrafficStatusLoaded: boolean
  scopedFetching: boolean
  scopedLoading: boolean
  scopedSummary?: BillingSummaryData
}) {
  const { t } = useTranslation()
  const platformTotal = accountSummary?.balanceScope === 'platform_total'
  const balanceStatus = normalizeBalanceStatus(accountSummary?.balanceStatus)
  const periodCategories = scopedSummary?.periodCategories ?? []
  const showGatewayTrafficStatusCard = gatewayTrafficStatusLoaded && gatewayTrafficStatus && !gatewayTrafficStatus.available

  return (
    <>
      <MetricGroup className="@xl/metric-group:grid-cols-4">
        <MetricItem
          icon={<Coins className="size-4" />}
          label={platformTotal ? t('billingPage.platformWalletTotal') : t('billingPage.balance')}
          meta={!accountLoading && canManageBilling ? billingDisplay.formatFiatAmount(accountSummary?.balanceCredits) : undefined}
          value={accountLoading ? '-' : billingDisplay.formatAmountWithUnit(accountSummary?.balanceCredits)}
        />
        <MetricItem
          icon={<TrendingDown className="size-4" />}
          label={t('billingPage.periodSpend')}
          meta={!scopedLoading && canManageBilling ? billingDisplay.formatFiatAmount(scopedSummary?.periodSpend) : undefined}
          value={scopedLoading ? '-' : billingDisplay.formatAmountWithUnit(scopedSummary?.periodSpend)}
        />
        <MetricItem
          icon={<CreditCard className="size-4" />}
          label={t('billingPage.todaySpend')}
          meta={!scopedLoading && canManageBilling ? billingDisplay.formatFiatAmount(scopedSummary?.todaySpend) : undefined}
          value={scopedLoading ? '-' : billingDisplay.formatAmountWithUnit(scopedSummary?.todaySpend)}
        />
        <MetricItem
          icon={<WalletCards className="size-4" />}
          label={t('billingPage.pendingSpend')}
          meta={!scopedLoading && canManageBilling ? billingDisplay.formatFiatAmount(scopedSummary?.pendingSpend) : undefined}
          value={scopedLoading ? '-' : billingDisplay.formatAmountWithUnit(scopedSummary?.pendingSpend)}
        />
      </MetricGroup>

      <Section
        title={t('billingPage.periodCategoriesTitle')}
        tools={accountSummary && !platformTotal
          ? (
              <StatusBadge tone={balanceStatusTone(balanceStatus)}>
                {t(`billingPage.balanceStatuses.${balanceStatus}`)}
              </StatusBadge>
            )
          : undefined}
        variant="bordered"
      >
        {periodCategories.length === 0 && !showGatewayTrafficStatusCard
          ? (
              <p className="py-4 text-sm text-muted-foreground">
                {scopedFetching ? t('common.loading') : t('billingPage.emptyPeriodCategories')}
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
