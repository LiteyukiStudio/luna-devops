import type { BillingDisplay, BillingListPage } from './billing-list-cells'
import type { BillingUsageRecord, Project } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { BILLING_PAGE_SIZE, formatQuantity } from '../overview/billing-page-utils'
import { ApplicationCell, ProjectCell, ResourceCell } from './billing-list-cells'

export function BillingUsageList({
  billingDisplay,
  data,
  locale,
  page,
  projectMap,
  onPageChange,
}: {
  billingDisplay: BillingDisplay
  data?: BillingListPage<BillingUsageRecord>
  locale: string
  page: number
  projectMap: Map<string, Project>
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const columns = useMemo<DataListColumn<BillingUsageRecord>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => <ProjectCell project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      className: 'min-w-52',
      render: item => <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />,
    },
    {
      key: 'meter',
      header: t('billingPage.meter'),
      className: 'min-w-40',
      render: item => t(`billingPage.meters.${item.meter}`, { defaultValue: item.meter }),
    },
    {
      key: 'status',
      header: t('common.status'),
      className: 'w-28',
      render: item => <StatusValueBadge value={item.status} />,
    },
    {
      key: 'quantity',
      header: t('billingPage.quantity'),
      className: 'w-36',
      render: item => <span className="tabular-nums">{formatQuantity(item.quantity, item.unit, locale)}</span>,
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      className: 'w-40',
      render: item => <span className="font-medium tabular-nums text-destructive">{billingDisplay.formatAmountWithUnit(item.amountCredits)}</span>,
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
  ], [billingDisplay, locale, projectMap, t])

  return (
    <DataList
      columns={columns}
      emptyDescription={t('billingPage.emptyUsageDescription')}
      emptyTitle={t('billingPage.emptyUsageTitle')}
      items={data?.items ?? []}
      pagination={{
        page: data?.page ?? page,
        pageInfoLabel: t('billingPage.usagePageInfo', {
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
