import type { BillingDisplay } from '../records/billing-list-cells'
import type { BillingRateRule } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
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
          <span className="block truncate font-medium">
            {t(`billingPage.meters.${rule.meter}`, { defaultValue: rule.meter })}
          </span>
          <span className="block truncate font-mono text-xs text-muted-foreground">{rule.meter}</span>
        </span>
      ),
    },
    {
      key: 'price',
      header: t('billingPage.unitPrice'),
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
    {
      key: 'description',
      header: t('billingPage.descriptionLabel'),
      width: 'normal',
      render: rule => (
        <span className="text-muted-foreground">
          {t(`settings.billingRateRuleDescriptions.${rule.meter}`, { defaultValue: rule.description })}
        </span>
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
    <div className="grid gap-3">
      <p className="text-sm text-muted-foreground">{t('billingPage.priceTableDescription')}</p>
      <p className="text-sm text-muted-foreground">{t('billingPage.aiModelPricingNote')}</p>
      <DataList
        columns={columns}
        emptyDescription={t('billingPage.emptyPriceTableDescription')}
        emptyTitle={t('billingPage.emptyPriceTableTitle')}
        items={resourceRateRules}
        loading={rateRules.isLoading}
        rowKey={rule => rule.meter}
      />
    </div>
  )
}
