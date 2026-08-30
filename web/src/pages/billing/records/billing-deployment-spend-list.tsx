import type { BillingDisplay, BillingListPage } from './billing-list-cells'
import type { BillingDeploymentSpend, Project } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { BILLING_PAGE_SIZE } from '../overview/billing-page-utils'
import { ApplicationCell, DeploymentTargetCell, ProjectCell, SpendAmount } from './billing-list-cells'

export function BillingDeploymentSpendList({
  billingDisplay,
  data,
  page,
  projectMap,
  onPageChange,
}: {
  billingDisplay: BillingDisplay
  data?: BillingListPage<BillingDeploymentSpend>
  page: number
  projectMap: Map<string, Project>
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const columns = useMemo<DataListColumn<BillingDeploymentSpend>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      className: 'min-w-56',
      render: item => <ProjectCell project={projectMap.get(item.projectId)} fallbackName={item.projectName} fallbackIdentifier={item.projectIdentifier} unknownLabel={t('billingPage.unknownProject')} />,
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      className: 'min-w-52',
      render: item => <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />,
    },
    {
      key: 'deploymentTarget',
      header: t('billingPage.deploymentTarget'),
      className: 'min-w-48',
      render: item => <DeploymentTargetCell item={item} unassignedLabel={t('billingPage.unassignedDeploymentTarget')} />,
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      className: 'w-40',
      render: item => <SpendAmount value={item.amountCredits} billingDisplay={billingDisplay} strong />,
    },
    {
      key: 'build',
      header: t('billingPage.buildSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.buildCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'runtime',
      header: t('billingPage.runtimeSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.runtimeCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'storage',
      header: t('billingPage.storageSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.storageCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'gateway',
      header: t('billingPage.gatewaySpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.gatewayCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'other',
      header: t('billingPage.otherSpend'),
      className: 'w-32',
      render: item => <SpendAmount value={item.otherCredits} billingDisplay={billingDisplay} />,
    },
  ], [billingDisplay, projectMap, t])

  return (
    <DataList
      columns={columns}
      emptyDescription={t('billingPage.emptyDeploymentSpendDescription')}
      emptyTitle={t('billingPage.emptyDeploymentSpendTitle')}
      items={data?.items ?? []}
      pagination={{
        page: data?.page ?? page,
        pageInfoLabel: t('billingPage.deploymentSpendPageInfo', {
          page: data?.page ?? page,
          total: data?.total ?? 0,
          totalPages: data?.totalPages ?? 1,
        }),
        pageSize: data?.pageSize ?? BILLING_PAGE_SIZE,
        total: data?.total ?? 0,
        totalPages: data?.totalPages ?? 1,
        onPageChange,
      }}
      rowKey={item => `${item.projectId}:${item.applicationId || 'unassigned'}:${item.deploymentTargetId || 'unassigned'}`}
    />
  )
}
