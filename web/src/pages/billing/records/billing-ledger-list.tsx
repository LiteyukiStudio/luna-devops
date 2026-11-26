import type { BillingLedgerCategoryFilter, BillingLedgerTypeFilter } from '../overview/billing-page-utils'
import type { BillingDisplay, BillingListError, BillingListPage } from './billing-list-cells'
import type { Application, BillingLedgerEntry, Project } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { ReceiptText, RotateCw } from 'lucide-react'
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
  BILLING_PROJECT_CATEGORY_ORDER,
  categoryTranslatedLabel,
  formatQuantity,
  ledgerDescription,
  ledgerReasonLabel,
} from '../overview/billing-page-utils'
import { ApplicationCell, ProjectCell, ResourceCell } from './billing-list-cells'

export function BillingLedgerList({
  applicationFilter = 'all',
  applications = [],
  applicationsError = false,
  applicationsLoading = false,
  applicationsRetrying = false,
  billingDisplay,
  categoryFilter,
  data,
  error,
  loading,
  locale,
  page,
  projectMap,
  projectMode = false,
  projectsError = false,
  projectsLoading = false,
  singleProjectSelected = false,
  typeFilter,
  onApplicationFilterChange,
  onCategoryFilterChange,
  onClearFilters,
  onPageChange,
  onRetryApplications,
  onTypeFilterChange,
}: {
  applicationFilter?: string
  applications?: Pick<Application, 'id' | 'identifier' | 'name'>[]
  applicationsError?: boolean
  applicationsLoading?: boolean
  applicationsRetrying?: boolean
  billingDisplay: BillingDisplay
  categoryFilter: BillingLedgerCategoryFilter
  data?: BillingListPage<BillingLedgerEntry>
  error?: BillingListError
  loading: boolean
  locale: string
  page: number
  projectMap: Map<string, Project>
  projectMode?: boolean
  projectsError?: boolean
  projectsLoading?: boolean
  singleProjectSelected?: boolean
  typeFilter: BillingLedgerTypeFilter
  onApplicationFilterChange?: (applicationId: string) => void
  onCategoryFilterChange: (category: BillingLedgerCategoryFilter) => void
  onClearFilters: () => void
  onPageChange: (page: number) => void
  onRetryApplications?: () => void
  onTypeFilterChange: (type: BillingLedgerTypeFilter) => void
}) {
  const { t } = useTranslation()
  const hasActiveFilters = (singleProjectSelected && applicationFilter !== 'all') || (!projectMode && typeFilter !== 'all') || categoryFilter !== 'all'
  const categoryOptions = projectMode ? BILLING_PROJECT_CATEGORY_ORDER : BILLING_CATEGORY_ORDER
  const columns = useMemo<DataListColumn<BillingLedgerEntry>[]>(() => {
    const baseColumns: DataListColumn<BillingLedgerEntry>[] = [
      {
        key: 'project',
        header: t('billingPage.project'),
        width: 'primary',
        render: item => item.projectId
          ? <ProjectCell loading={projectsLoading} project={projectMap.get(item.projectId)} unknownLabel={projectsError ? t('common.unavailable') : t('billingPage.unknownProject')} />
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
    ]
    if (!projectMode) {
      baseColumns.push({
        key: 'balance',
        header: t('billingPage.balanceAfter'),
        width: 'number',
        render: item => <span className="tabular-nums">{billingDisplay.formatAmountWithUnit(item.balanceAfterCredits)}</span>,
      })
    }
    baseColumns.push(
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
    )
    return baseColumns
  }, [billingDisplay, locale, projectMap, projectMode, projectsError, projectsLoading, t])

  return (
    <DataList
      columns={columns}
      emptyActions={hasActiveFilters
        ? (
            <Button size="sm" type="button" variant="outline" onClick={onClearFilters}>
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
          {singleProjectSelected && onApplicationFilterChange && (
            <>
              <Select disabled={applicationsLoading || applicationsError} value={applicationFilter} onValueChange={onApplicationFilterChange}>
                <SelectTrigger aria-label={t('billingPage.filterApplicationLabel')} className="h-8 w-44 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">
                    {applicationsError ? t('common.unavailable') : t('billingPage.filterApplicationAll')}
                  </SelectItem>
                  {applications.map(application => (
                    <SelectItem key={application.id} value={application.id}>
                      {application.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {!applicationsLoading && applicationsError && onRetryApplications && (
                <Button
                  className="h-8 shrink-0 text-xs"
                  disabled={applicationsRetrying}
                  size="sm"
                  type="button"
                  variant="outline"
                  onClick={onRetryApplications}
                >
                  <RotateCw className={applicationsRetrying ? 'size-3.5 motion-safe:animate-spin' : 'size-3.5'} />
                  {t('common.retry')}
                </Button>
              )}
            </>
          )}

          {!projectMode && (
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
          )}

          <Select value={categoryFilter} onValueChange={next => onCategoryFilterChange(next as BillingLedgerCategoryFilter)}>
            <SelectTrigger aria-label={t('billingPage.filterCategoryLabel')} className="h-8 w-36 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('billingPage.filterCategoryAll')}</SelectItem>
              {categoryOptions.map(category => (
                <SelectItem key={category} value={category}>
                  {categoryTranslatedLabel(category, t)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {hasActiveFilters && (
            <Button className="h-8 text-xs" size="sm" type="button" variant="ghost" onClick={onClearFilters}>
              {t('billingPage.clearLedgerFilters')}
            </Button>
          )}
        </div>
      )}
    />
  )
}
