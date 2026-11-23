import type { BillingLedgerCategoryFilter, BillingLedgerTypeFilter } from '../overview/billing-page-utils'
import type { BillingDisplay, BillingListError, BillingListPage } from './billing-list-cells'
import type { BillingLedgerEntry, Project } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { ReceiptText } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { StatusBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import {
  amountToneClass,
  BILLING_CATEGORY_ORDER,
  BILLING_PAGE_SIZE,
  categoryTranslatedLabel,
  formatQuantity,
  ledgerDescription,
  ledgerReasonLabel,
} from '../overview/billing-page-utils'
import { ApplicationCell, ProjectCell, ResourceCell } from './billing-list-cells'

export function BillingLedgerList({
  billingDisplay,
  categoryFilter,
  data,
  error,
  loading,
  locale,
  page,
  projectMap,
  projectsLoading = false,
  typeFilter,
  onCategoryFilterChange,
  onClearFilters,
  onPageChange,
  onTypeFilterChange,
}: {
  billingDisplay: BillingDisplay
  categoryFilter: BillingLedgerCategoryFilter
  data?: BillingListPage<BillingLedgerEntry>
  error?: BillingListError
  loading: boolean
  locale: string
  page: number
  projectMap: Map<string, Project>
  projectsLoading?: boolean
  typeFilter: BillingLedgerTypeFilter
  onCategoryFilterChange: (category: BillingLedgerCategoryFilter) => void
  onClearFilters: () => void
  onPageChange: (page: number) => void
  onTypeFilterChange: (type: BillingLedgerTypeFilter) => void
}) {
  const { t } = useTranslation()
  const hasActiveFilters = typeFilter !== 'all' || categoryFilter !== 'all'
  const columns = useMemo<DataListColumn<BillingLedgerEntry>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      width: 'primary',
      render: item => item.projectId
        ? <ProjectCell loading={projectsLoading} project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />
        : <span className="text-sm text-muted-foreground">{t('billingPage.accountTransaction')}</span>,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      width: 'normal',
      render: item => item.projectId
        ? <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />
        : <span className="text-sm text-muted-foreground">-</span>,
    },
    {
      key: 'type',
      header: t('billingPage.type'),
      width: 'status',
      render: item => (
        <StatusBadge tone={item.type === 'debit' ? 'danger' : item.type === 'credit' ? 'success' : 'neutral'}>
          {t(`billingPage.types.${item.type}`, { defaultValue: item.type })}
        </StatusBadge>
      ),
    },
    {
      key: 'reason',
      header: t('billingPage.reason'),
      width: 'primary',
      render: (item) => {
        const reasonLabel = ledgerReasonLabel(item, t)
        const description = ledgerDescription(item, reasonLabel)
        return (
          <div className="min-w-0 space-y-1">
            <div className="text-sm font-medium">{reasonLabel}</div>
            {description && (
              <div className="line-clamp-2 text-xs leading-relaxed text-muted-foreground" title={description}>
                {description}
              </div>
            )}
          </div>
        )
      },
    },
    {
      key: 'quantity',
      header: t('billingPage.quantity'),
      width: 'number',
      render: (item) => {
        if (!item.usageQuantity)
          return <span className="text-sm text-muted-foreground">-</span>
        const unitLabel = item.usageUnit
          ? t(`settings.billingRateUnits.${item.usageUnit}`, { defaultValue: item.usageUnit })
          : ''
        return <span className="tabular-nums text-muted-foreground">{formatQuantity(item.usageQuantity, unitLabel, locale)}</span>
      },
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      width: 'number',
      render: item => (
        <span className={cn('font-medium tabular-nums', amountToneClass(item.amountCredits))}>
          {billingDisplay.formatSignedAmountWithUnit(item.amountCredits)}
        </span>
      ),
    },
    {
      key: 'balance',
      header: t('billingPage.balanceAfter'),
      width: 'number',
      render: item => <span className="tabular-nums">{billingDisplay.formatAmountWithUnit(item.balanceAfterCredits)}</span>,
    },
    {
      key: 'resource',
      header: t('billingPage.resource'),
      width: 'primary',
      render: item => <ResourceCell resourceId={item.resourceId} resourceType={item.resourceType} />,
    },
    {
      key: 'time',
      header: t('billingPage.time'),
      width: 'compact',
      render: item => formatSmartDateTime(item.occurredAt, t),
    },
  ], [billingDisplay, locale, projectMap, projectsLoading, t])

  return (
    <DataList
      columns={columns}
      emptyActions={hasActiveFilters
        ? (
            <Button size="sm" variant="outline" onClick={onClearFilters}>
              {t('billingPage.clearLedgerFilters')}
            </Button>
          )
        : undefined}
      emptyDescription={hasActiveFilters ? t('billingPage.emptyFilteredLedgerDescription') : t('billingPage.emptyLedgerDescription')}
      emptyIcon={<ReceiptText className="size-5" />}
      emptyMode={hasActiveFilters ? 'filtered' : 'actionable'}
      emptyTitle={hasActiveFilters ? t('billingPage.emptyFilteredLedgerTitle') : t('billingPage.emptyLedgerTitle')}
      error={error}
      items={data?.items ?? []}
      loading={loading && !error}
      pagination={{
        page: data?.page ?? page,
        pageInfoLabel: t('billingPage.ledgerPageInfo', {
          page: data?.page ?? page,
          total: data?.total ?? 0,
          totalPages: data?.totalPages ?? 1,
        }),
        pageSize: data?.pageSize ?? BILLING_PAGE_SIZE,
        total: data?.total ?? 0,
        totalPages: data?.totalPages ?? 1,
        onPageChange,
      }}
      rowKey={item => item.id}
      toolbar={(
        <div className="flex flex-wrap items-center gap-2">
          <Select value={typeFilter} onValueChange={next => onTypeFilterChange(next as BillingLedgerTypeFilter)}>
            <SelectTrigger aria-label={t('billingPage.filterTypeLabel')} className="h-8 w-36 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('billingPage.filterTypeAll')}</SelectItem>
              <SelectItem value="debit">{t('billingPage.types.debit')}</SelectItem>
              <SelectItem value="credit">{t('billingPage.types.credit')}</SelectItem>
              <SelectItem value="adjustment">{t('billingPage.types.adjustment')}</SelectItem>
            </SelectContent>
          </Select>

          <Select value={categoryFilter} onValueChange={next => onCategoryFilterChange(next as BillingLedgerCategoryFilter)}>
            <SelectTrigger aria-label={t('billingPage.filterCategoryLabel')} className="h-8 w-36 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('billingPage.filterCategoryAll')}</SelectItem>
              {BILLING_CATEGORY_ORDER.map(category => (
                <SelectItem key={category} value={category}>
                  {categoryTranslatedLabel(category, t)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {hasActiveFilters && (
            <Button className="h-8 text-xs" size="sm" variant="ghost" onClick={onClearFilters}>
              {t('billingPage.clearLedgerFilters')}
            </Button>
          )}
        </div>
      )}
    />
  )
}
