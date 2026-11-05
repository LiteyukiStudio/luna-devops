import type { BillingDisplay, BillingListPage } from './billing-list-cells'
import type { BillingUsageRecord, Project } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { Gauge } from 'lucide-react'
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
  loading,
  locale,
  page,
  projectMap,
  onPageChange,
}: {
  billingDisplay: BillingDisplay
  data?: BillingListPage<BillingUsageRecord>
  loading: boolean
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
      width: 'primary',
      render: item => <ProjectCell project={projectMap.get(item.projectId)} unknownLabel={t('billingPage.unknownProject')} />,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      width: 'normal',
      render: item => <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />,
    },
    {
      key: 'meter',
      header: t('billingPage.meter'),
      width: 'secondary',
      render: item => t(`billingPage.meters.${item.meter}`, { defaultValue: item.meter }),
    },
    {
      key: 'status',
      header: t('common.status'),
      width: 'status',
      render: item => <StatusValueBadge value={item.status} />,
    },
    {
      key: 'quantity',
      header: t('billingPage.quantity'),
      width: 'number',
      render: item => <span className="tabular-nums">{formatQuantity(item.quantity, item.unit, locale)}</span>,
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      width: 'number',
      render: item => <span className="font-medium tabular-nums text-destructive">{billingDisplay.formatAmountWithUnit(item.amountCredits)}</span>,
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
      render: item => formatSmartDateTime(item.createdAt, t),
    },
  ], [billingDisplay, locale, projectMap, t])

  return (
    <DataList
      columns={columns}
      emptyDescription={t('billingPage.emptyUsageDescription')}
      emptyTitle={t('billingPage.emptyUsageTitle')}
      emptyIcon={<Gauge className="size-5" />}
      items={data?.items ?? []}
      loading={loading}
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
