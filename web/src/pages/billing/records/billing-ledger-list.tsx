import type { BillingDisplay, BillingListPage } from './billing-list-cells'
import type { BillingLedgerEntry, Project } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { StatusBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { cn } from '@/lib/utils'
import { amountToneClass, BILLING_PAGE_SIZE, ledgerDescription, ledgerReasonLabel } from '../overview/billing-page-utils'
import { ApplicationCell, ProjectCell, ResourceCell } from './billing-list-cells'

export function BillingLedgerList({
  billingDisplay,
  data,
  page,
  projectMap,
  onPageChange,
}: {
  billingDisplay: BillingDisplay
  data?: BillingListPage<BillingLedgerEntry>
  page: number
  projectMap: Map<string, Project>
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const columns = useMemo<DataListColumn<BillingLedgerEntry>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => item.projectId
        ? <ProjectCell project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />
        : <span className="text-sm text-muted-foreground">{t('billingPage.accountTransaction')}</span>,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      className: 'min-w-52',
      render: item => item.projectId
        ? <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />
        : <span className="text-sm text-muted-foreground">-</span>,
    },
    {
      key: 'type',
      header: t('billingPage.type'),
      className: 'w-28',
      render: item => (
        <StatusBadge tone={item.type === 'debit' ? 'danger' : item.type === 'credit' ? 'success' : 'neutral'}>
          {t(`billingPage.types.${item.type}`, { defaultValue: item.type })}
        </StatusBadge>
      ),
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      className: 'w-40',
      render: item => (
        <span className={cn('font-medium tabular-nums', amountToneClass(item.amountCredits))}>
          {billingDisplay.formatSignedAmountWithUnit(item.amountCredits)}
        </span>
      ),
    },
    {
      key: 'balance',
      header: t('billingPage.balanceAfter'),
      className: 'w-40',
      render: item => <span className="tabular-nums">{billingDisplay.formatAmountWithUnit(item.balanceAfterCredits)}</span>,
    },
    {
      key: 'reason',
      header: t('billingPage.reason'),
      className: 'min-w-56',
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
      key: 'resource',
      header: t('billingPage.resource'),
      className: 'min-w-56',
      render: item => <ResourceCell resourceId={item.resourceId} resourceType={item.resourceType} />,
    },
    {
      key: 'time',
      header: t('billingPage.time'),
      className: 'w-44',
      render: item => formatSmartDateTime(item.createdAt, t),
    },
  ], [billingDisplay, projectMap, t])

  return (
    <DataList
      columns={columns}
      emptyDescription={t('billingPage.emptyLedgerDescription')}
      emptyTitle={t('billingPage.emptyLedgerTitle')}
      items={data?.items ?? []}
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
    />
  )
}
