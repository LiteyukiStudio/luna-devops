import type { BillingDeploymentStageFilter } from '../overview/billing-page-utils'
import type { BillingDisplay, BillingListError, BillingListPage } from './billing-list-cells'
import type { Application, BillingDeploymentSpend, Project } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { Rocket, RotateCw } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  BILLING_DEPLOYMENT_STAGE_OPTIONS,
  BILLING_PAGE_SIZE,
  deploymentStageTranslatedLabel,
} from '../overview/billing-page-utils'
import { ApplicationCell, DeploymentTargetCell, ProjectCell, SpendAmount } from './billing-list-cells'

export function BillingDeploymentSpendList({
  applicationFilter = 'all',
  applications = [],
  applicationsError = false,
  applicationsLoading = false,
  applicationsRetrying = false,
  billingDisplay,
  data,
  error,
  loading,
  page,
  projectMap,
  projectsError = false,
  projectsLoading = false,
  singleProjectSelected = false,
  stageFilter = 'all',
  onApplicationFilterChange,
  onClearFilters,
  onPageChange,
  onRetryApplications,
  onStageFilterChange,
}: {
  applicationFilter?: string
  applications?: Pick<Application, 'id' | 'identifier' | 'name'>[]
  applicationsError?: boolean
  applicationsLoading?: boolean
  applicationsRetrying?: boolean
  billingDisplay: BillingDisplay
  data?: BillingListPage<BillingDeploymentSpend>
  error?: BillingListError
  loading: boolean
  page: number
  projectMap: Map<string, Project>
  projectsError?: boolean
  projectsLoading?: boolean
  singleProjectSelected?: boolean
  stageFilter?: BillingDeploymentStageFilter
  onApplicationFilterChange?: (applicationId: string) => void
  onClearFilters?: () => void
  onPageChange: (page: number) => void
  onRetryApplications?: () => void
  onStageFilterChange?: (stage: BillingDeploymentStageFilter) => void
}) {
  const { t } = useTranslation()
  const hasActiveFilters = (singleProjectSelected && applicationFilter !== 'all') || stageFilter !== 'all'
  const showToolbar = Boolean(onStageFilterChange || (singleProjectSelected && onApplicationFilterChange))
  const columns = useMemo<DataListColumn<BillingDeploymentSpend>[]>(() => [
    {
      key: 'project',
      header: t('billingPage.project'),
      width: 'primary',
      render: item => (
        <ProjectCell
          fallbackIdentifier={item.projectIdentifier}
          fallbackName={item.projectName}
          loading={Boolean(item.projectId) && projectsLoading}
          project={projectMap.get(item.projectId)}
          unknownLabel={projectsError ? t('common.unavailable') : t('billingPage.unknownProject')}
        />
      ),
    },
    {
      key: 'application',
      header: t('billingPage.application'),
      width: 'normal',
      render: item => <ApplicationCell item={item} unassignedLabel={t('billingPage.unassignedApplication')} />,
    },
    {
      key: 'deploymentTarget',
      header: t('billingPage.deploymentTarget'),
      width: 'normal',
      render: item => (
        <DeploymentTargetCell
          item={item}
          projectStorageHint={t('billingPage.projectStorageGroupHint')}
          projectStorageLabel={t('billingPage.projectStorageGroup')}
          stageLabel={item.deploymentTargetStage ? deploymentStageTranslatedLabel(item.deploymentTargetStage, t) : undefined}
          unassignedLabel={t('billingPage.unassignedDeploymentTarget')}
        />
      ),
    },
    {
      key: 'amount',
      header: t('billingPage.amount'),
      width: 'number',
      render: item => <SpendAmount value={item.amountCredits} billingDisplay={billingDisplay} strong />,
    },
    {
      key: 'build',
      header: t('billingPage.buildSpend'),
      width: 'number',
      render: item => <SpendAmount value={item.buildCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'runtime',
      header: t('billingPage.runtimeSpend'),
      width: 'number',
      render: item => <SpendAmount value={item.runtimeCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'storage',
      header: t('billingPage.storageSpend'),
      width: 'number',
      render: item => <SpendAmount value={item.storageCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'gateway',
      header: t('billingPage.gatewaySpend'),
      width: 'number',
      render: item => <SpendAmount value={item.gatewayCredits} billingDisplay={billingDisplay} />,
    },
    {
      key: 'other',
      header: t('billingPage.otherSpend'),
      width: 'number',
      render: item => <SpendAmount value={item.otherCredits} billingDisplay={billingDisplay} />,
    },
  ], [billingDisplay, projectMap, projectsError, projectsLoading, t])

  return (
    <DataList
      columns={columns}
      emptyActions={hasActiveFilters && onClearFilters
        ? (
            <Button size="sm" type="button" variant="outline" onClick={onClearFilters}>
              {t('billingPage.clearLedgerFilters')}
            </Button>
          )
        : undefined}
      emptyDescription={hasActiveFilters
        ? (singleProjectSelected
            ? t('billingPage.emptyFilteredDeploymentSpendDescription')
            : t('billingPage.emptyStageFilteredDeploymentSpendDescription'))
        : t('billingPage.emptyDeploymentSpendDescription')}
      emptyTitle={hasActiveFilters ? t('billingPage.emptyFilteredDeploymentSpendTitle') : t('billingPage.emptyDeploymentSpendTitle')}
      emptyIcon={<Rocket className="size-5" />}
      emptyMode={hasActiveFilters ? 'filtered' : 'actionable'}
      error={error}
      items={data?.items ?? []}
      loading={loading && !error}
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
      rowKey={item => `${item.projectId}:${item.applicationId || 'unassigned'}:${item.spendGroup}:${item.deploymentTargetId || 'unassigned'}`}
      toolbar={showToolbar
        ? (
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

              {onStageFilterChange && (
                <Select value={stageFilter} onValueChange={next => onStageFilterChange(next as BillingDeploymentStageFilter)}>
                  <SelectTrigger aria-label={t('billingPage.filterStageLabel')} className="h-8 w-36 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t('billingPage.filterStageAll')}</SelectItem>
                    {BILLING_DEPLOYMENT_STAGE_OPTIONS.map(stage => (
                      <SelectItem key={stage} value={stage}>
                        {deploymentStageTranslatedLabel(stage, t)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}

              {hasActiveFilters && onClearFilters && (
                <Button className="h-8 text-xs" size="sm" type="button" variant="ghost" onClick={onClearFilters}>
                  {t('billingPage.clearLedgerFilters')}
                </Button>
              )}
            </div>
          )
        : undefined}
    />
  )
}
