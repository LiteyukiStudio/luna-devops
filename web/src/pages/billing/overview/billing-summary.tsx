import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingSummary as BillingSummaryData, GatewayTrafficStatus } from '@/api'
import { AlertTriangle, Coins, CreditCard, ExternalLink, TrendingDown, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Section } from '@/components/common/section'
import { StatusBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
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
  const balanceStatus = normalizeBalanceStatus(accountSummary?.balanceStatus)
  const periodCategories = scopedSummary?.periodCategories ?? []
  const showGatewayTrafficStatusCard = gatewayTrafficStatusLoaded && gatewayTrafficStatus && !gatewayTrafficStatus.available

  return (
    <>
      <MetricGroup className="md:grid-cols-4">
        <MetricItem
          icon={<Coins className="size-4" />}
          label={t('billingPage.balance')}
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
        tools={(
          <StatusBadge tone={balanceStatusTone(balanceStatus)}>
            {t(`billingPage.balanceStatuses.${balanceStatus}`)}
          </StatusBadge>
        )}
        variant="bordered"
      >
        <div className="grid min-h-[5.75rem] gap-3 md:grid-cols-3 xl:grid-cols-6">
          {periodCategories.length > 0
            ? periodCategories.filter(category => !(showGatewayTrafficStatusCard && category.category === 'gateway')).map(category => (
                <Surface
                  key={category.category}
                  className="p-3"
                  variant="inset"
                >
                  <p className="truncate text-xs text-muted-foreground">
                    {t(`billingPage.categories.${category.category}`, { defaultValue: category.category })}
                  </p>
                  <p className="mt-1 truncate text-lg font-semibold tabular-nums text-foreground">
                    {billingDisplay.formatAmountWithUnit(category.amountCredits)}
                  </p>
                </Surface>
              ))
            : null}
          {showGatewayTrafficStatusCard && (
            <Surface className="p-3" variant="inset">
              <p className="truncate text-xs text-muted-foreground">
                {t('billingPage.categories.gateway')}
              </p>
              <p className="mt-1 truncate text-lg font-semibold tabular-nums text-foreground">
                {gatewayTrafficStatusLabel(gatewayTrafficStatus, t)}
              </p>
              <a
                className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-primary-text transition-colors hover:text-primary-text/80"
                href={GATEWAY_TRAFFIC_METRICS_DOC_URL}
                rel="noreferrer"
                target="_blank"
              >
                {t('billingPage.gatewayTrafficMetricsDocs')}
                <ExternalLink className="size-3" />
              </a>
            </Surface>
          )}
          {periodCategories.length === 0 && !showGatewayTrafficStatusCard && (
            <div className="flex min-h-[5.75rem] items-center rounded-md border border-dashed border-border bg-muted/10 px-4 text-sm text-muted-foreground md:col-span-3 xl:col-span-6">
              {scopedFetching ? t('common.loading') : t('billingPage.emptyPeriodCategories')}
            </div>
          )}
        </div>
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
  if (!accountSummary || balanceStatus === 'ok')
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
