import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingRateRule } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useQuery } from '@tanstack/react-query'
import { ReceiptText } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { HelpTip } from '@/components/common/help-tip'
import { StatusBadge } from '@/components/common/status-badge'

type PriceDisplay = Pick<BillingDisplay, 'formatAmountWithUnit' | 'formatFiatAmount'>

export function BillingPriceList({ billingDisplay }: { billingDisplay: PriceDisplay }) {
  const { t } = useTranslation()
  const rateRules = useQuery({ queryKey: ['billing-rate-rules'], queryFn: api.listBillingRateRules })
  const resourceRateRules = useMemo(() => (rateRules.data ?? []).filter(rule => !rule.meter.startsWith('ai.')), [rateRules.data])
  const columns = useMemo<DataListColumn<BillingRateRule>[]>(() => [
    {
      key: 'meter',
      header: t('billingPage.priceItem'),
      width: 'primary',
      render: rule => (
        <span className="block min-w-0">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="truncate font-medium">
              {t(`billingPage.meters.${rule.meter}`, { defaultValue: rule.meter })}
            </span>
            <HelpTip label={t(`billingPage.meters.${rule.meter}`, { defaultValue: rule.meter })}>
              {t(`settings.billingRateRuleDescriptions.${rule.meter}`, { defaultValue: rule.description })}
            </HelpTip>
          </span>
          <span className="block truncate font-mono text-xs text-muted-foreground">{rule.meter}</span>
        </span>
      ),
    },
    {
      key: 'price',
      header: (
        <span className="inline-flex items-center gap-1.5">
          {t('billingPage.unitPrice')}
          <HelpTip label={t('billingPage.unitPrice')}>
            {t('billingPage.priceTableDescription')}
            {' '}
            {t('billingPage.aiModelPricingNote')}
          </HelpTip>
        </span>
      ),
      width: 'secondary',
      render: (rule) => {
        const fiatPrice = billingDisplay.formatFiatAmount(rule.creditsPerUnit)
        return (
          <span className="block min-w-0 tabular-nums">
            <span className="block truncate font-medium">{billingDisplay.formatAmountWithUnit(rule.creditsPerUnit)}</span>
            {fiatPrice && <span className="block truncate text-xs text-muted-foreground">{fiatPrice}</span>}
          </span>
        )
      },
    },
    {
      key: 'unit',
      header: t('billingPage.billingUnit'),
      width: 'secondary',
      render: rule => (
        <span className="text-muted-foreground" title={rule.unit}>
          {t(`settings.billingRateUnits.${rule.unit}`, { defaultValue: rule.unit })}
        </span>
      ),
    },
    {
      key: 'state',
      header: t('billingPage.priceState'),
      width: 'status',
      render: rule => (
        <StatusBadge tone={rule.enabled ? 'success' : 'neutral'}>
          {rule.enabled ? t('common.enabled') : t('common.disabled')}
        </StatusBadge>
      ),
    },
  ], [billingDisplay, t])

  if (rateRules.isError) {
    return (
      <ErrorState
        description={t('billingPage.priceTableLoadFailedDescription')}
        title={t('billingPage.priceTableLoadFailedTitle')}
      />
    )
  }

  return (
    <DataList
      columns={columns}
      emptyDescription={t('billingPage.emptyPriceTableDescription')}
      emptyIcon={<ReceiptText className="size-5" />}
      emptyTitle={t('billingPage.emptyPriceTableTitle')}
      items={resourceRateRules}
      loading={rateRules.isLoading}
      rowKey={rule => rule.meter}
    />
  )
}
