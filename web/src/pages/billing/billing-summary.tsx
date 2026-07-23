import type { ReactNode } from 'react'
import type { BillingDisplay } from './billing-list-cells'
import type { BillingSummary as BillingSummaryData, GatewayTrafficStatus } from '@/api'
import { AlertTriangle, Coins, CreditCard, ExternalLink, TrendingDown, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/common/status-badge'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { balanceStatusTone, gatewayTrafficStatusLabel, normalizeBalanceStatus } from './billing-page-utils'

const DOCS_BASE_URL = String(import.meta.env.VITE_DOCS_BASE_URL || 'https://luna-devops.liteyuki.org').replace(/\/+$/, '')
const GATEWAY_TRAFFIC_METRICS_DOC_URL = `${DOCS_BASE_URL}/operations/billing#traefik-prometheus-metrics-for-gateway-traffic-probe`

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
      <div className="grid gap-3 md:grid-cols-4">
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(accountSummary?.balanceCredits) : ''}
          icon={<Coins className="size-5" />}
          label={t('billingPage.balance')}
          loading={accountLoading}
          value={billingDisplay.formatAmountWithUnit(accountSummary?.balanceCredits)}
        />
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(scopedSummary?.periodSpend) : ''}
          icon={<TrendingDown className="size-5" />}
          label={t('billingPage.periodSpend')}
          loading={scopedLoading}
          value={billingDisplay.formatAmountWithUnit(scopedSummary?.periodSpend)}
        />
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(scopedSummary?.todaySpend) : ''}
          icon={<CreditCard className="size-5" />}
          label={t('billingPage.todaySpend')}
          loading={scopedLoading}
          value={billingDisplay.formatAmountWithUnit(scopedSummary?.todaySpend)}
        />
        <MetricCard
          fiatValue={canManageBilling ? billingDisplay.formatFiatAmount(scopedSummary?.pendingSpend) : ''}
          icon={<WalletCards className="size-5" />}
          label={t('billingPage.pendingSpend')}
          loading={scopedLoading}
          value={billingDisplay.formatAmountWithUnit(scopedSummary?.pendingSpend)}
        />
      </div>

      <Card className="rounded-lg p-5">
        <div className="flex min-w-0 flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <h3 className="text-base font-semibold text-foreground">{t('billingPage.periodCategoriesTitle')}</h3>
          </div>
          <StatusBadge tone={balanceStatusTone(balanceStatus)}>
            {t(`billingPage.balanceStatuses.${balanceStatus}`)}
          </StatusBadge>
        </div>
        <div className="mt-4 grid min-h-[5.75rem] gap-3 md:grid-cols-3 xl:grid-cols-6">
          {periodCategories.length > 0
            ? periodCategories.filter(category => !(showGatewayTrafficStatusCard && category.category === 'gateway')).map(category => (
                <SpendCategoryCard
                  key={category.category}
                  label={t(`billingPage.categories.${category.category}`, { defaultValue: category.category })}
                  value={billingDisplay.formatAmountWithUnit(category.amountCredits)}
                />
              ))
            : null}
          {showGatewayTrafficStatusCard && (
            <SpendCategoryCard
              action={(
                <a
                  className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-primary-text transition-colors hover:text-primary-text/80"
                  href={GATEWAY_TRAFFIC_METRICS_DOC_URL}
                  rel="noreferrer"
                  target="_blank"
                >
                  {t('billingPage.gatewayTrafficMetricsDocs')}
                  <ExternalLink className="size-3" />
                </a>
              )}
              label={t('billingPage.categories.gateway')}
              value={gatewayTrafficStatusLabel(gatewayTrafficStatus, t)}
            />
          )}
          {periodCategories.length === 0 && !showGatewayTrafficStatusCard && (
            <div className="flex min-h-[5.75rem] items-center rounded-md border border-dashed border-border bg-muted/10 px-4 text-sm text-muted-foreground md:col-span-3 xl:col-span-6">
              {scopedFetching ? t('common.loading') : t('billingPage.emptyPeriodCategories')}
            </div>
          )}
        </div>
      </Card>
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
    <Card className={cn(
      'flex min-w-0 items-start gap-3 rounded-lg border p-4',
      balanceStatus === 'insufficient'
        ? 'border-destructive/30 bg-destructive/5 text-destructive'
        : 'border-amber-300/60 bg-amber-50 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200',
    )}
    >
      <AlertTriangle className="mt-0.5 size-5 shrink-0" />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium">{t(`billingPage.balanceStatuses.${balanceStatus}`)}</p>
          <StatusBadge tone={balanceStatus === 'insufficient' ? 'danger' : 'warning'}>
            {billingDisplay.formatAmountWithUnit(accountSummary.availableCredits)}
          </StatusBadge>
        </div>
        <p className="mt-1 text-sm opacity-80">
          {t('billingPage.balanceWarningDescription', {
            pending: billingDisplay.formatAmountWithUnit(accountSummary.pendingSpend),
            threshold: billingDisplay.formatAmountWithUnit(accountSummary.lowBalanceLimit),
          })}
        </p>
      </div>
    </Card>
  )
}

function MetricCard({ fiatValue, icon, label, loading, value }: { fiatValue?: string, icon: ReactNode, label: string, loading: boolean, value: string }) {
  return (
    <Card className="flex min-w-0 items-center gap-4 rounded-lg p-5">
      <div className="flex size-11 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary-text">
        {icon}
      </div>
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <p className="shrink-0 text-sm text-muted-foreground">{label}</p>
          {!loading && fiatValue && (
            <span className="min-w-0 truncate text-xs tabular-nums text-muted-foreground/80">
              {fiatValue}
            </span>
          )}
        </div>
        <p className="mt-1 truncate text-2xl font-semibold tabular-nums">
          {loading ? '-' : value}
        </p>
      </div>
    </Card>
  )
}

function SpendCategoryCard({ action, label, value }: { action?: ReactNode, label: string, value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-border bg-muted/20 p-3">
      <p className="truncate text-xs text-muted-foreground">
        {label}
      </p>
      <p className="mt-1 truncate text-lg font-semibold tabular-nums text-foreground">
        {value}
      </p>
      {action}
    </div>
  )
}
