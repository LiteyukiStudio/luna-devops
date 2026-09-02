import type { CurrentUser, Project, RuntimeCluster, RuntimeClusterPressure } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { FlaskConical, MoreHorizontal, Pencil, Shield, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DataList } from '@/components/common/data-list'
import { RuntimeClusterPressureRings } from '@/components/common/runtime-cluster-pressure'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { canManageCluster, clusterTypeLabel, gatewayDomainSuffixSummary, gatewayPublicPortSummary, scopeLabel } from './cluster-helpers'

export function RuntimeClusterTable({ clusters, kubeGatewayStatusByClusterId, loading, pagination, pressureByClusterId, pressureLoading, projects, user, kubectlGatewayAvailable, onDelete, onEdit, onConfigureKubeGateway, onTest }: {
  clusters: RuntimeCluster[]
  kubeGatewayStatusByClusterId: Record<string, string>
  loading: boolean
  pagination: {
    page: number
    pageSize: number
    total: number
    totalPages: number
    onPageChange: (page: number) => void
    onPageSizeChange: (pageSize: number) => void
  }
  pressureByClusterId: Record<string, RuntimeClusterPressure>
  pressureLoading: boolean
  projects: Project[]
  user?: CurrentUser
  kubectlGatewayAvailable?: boolean
  onDelete: (cluster: RuntimeCluster) => void
  onEdit: (cluster: RuntimeCluster) => void
  onConfigureKubeGateway: (cluster: RuntimeCluster) => void
  onTest: (clusterId: string) => void
}) {
  const { t } = useTranslation()
  const projectMap = Object.fromEntries(projects.map(project => [project.id, project]))
  const columns: DataListColumn<RuntimeCluster>[] = [
    { key: 'name', header: t('common.name'), width: 'primary', render: item => item.name },
    { key: 'type', header: t('common.type'), width: 'secondary', render: item => clusterTypeLabel(item.type, t) },
    { key: 'scope', header: t('common.scope'), width: 'status', render: item => scopeLabel(item, projectMap, t) },
    { key: 'default', header: t('clustersPage.defaultCluster'), width: 'status', render: item => item.isDefault ? t('common.yes') : t('common.no') },
    { key: 'buildConcurrency', header: t('clustersPage.maxConcurrentBuilds'), width: 'number', render: item => item.maxConcurrentBuilds || 4 },
    { key: 'pressure', header: t('clustersPage.resourcePressure'), width: 'secondary', render: item => <RuntimeClusterPressureRings loading={pressureLoading} pressure={pressureByClusterId[item.id]} /> },
    { key: 'gatewayRootDomain', header: t('clustersPage.gatewayDomainSuffixes'), width: 'secondary', render: item => gatewayDomainSuffixSummary(item) },
    { key: 'gatewayPublicScheme', header: t('clustersPage.gatewayPublicScheme'), width: 'compact', render: item => item.gatewayPublicScheme || 'http' },
    { key: 'gatewayPublicPort', header: t('clustersPage.gatewayPublicPort'), width: 'compact', render: item => gatewayPublicPortSummary(item) },
    {
      key: 'status',
      header: t('common.status'),
      width: 'status',
      render: (item) => {
        const deleteStatus = item.deleteStatus ?? 'active'
        if (deleteStatus !== 'active')
          return <StatusValueBadge labelKeyPrefix="kubectlAccess.clusterDeleteStatuses" value={deleteStatus} />
        return <StatusValueBadge value={pressureByClusterId[item.id]?.status ?? item.status} />
      },
    },
    ...(kubectlGatewayAvailable
      ? [{
          key: 'kubeGatewayStatus',
          header: t('kubectlAccess.gatewayStatusLabel'),
          minWidth: 144,
          width: 'status' as const,
          render: (item: RuntimeCluster) => isKubeGatewayEligible(item)
            ? (
                <StatusValueBadge
                  labelKeyPrefix="kubectlAccess.gatewayStatuses"
                  value={kubeGatewayStatusByClusterId[item.id]?.trim() || 'checking'}
                />
              )
            : <span className="text-muted-foreground">—</span>,
        }]
      : []),
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right whitespace-nowrap',
      mobileActions: 'inline',
      sticky: 'right',
      width: 'actions',
      render: (item) => {
        if (!canManageCluster(item, user?.id, user?.role))
          return <span className="text-xs text-muted-foreground">{t('common.viewOnly')}</span>

        const deleteStatus = item.deleteStatus ?? 'active'
        if (deleteStatus === 'deleting')
          return <span className="text-xs text-muted-foreground">{t('kubectlAccess.clusterDeleteInProgress')}</span>

        return (
          <div className="flex justify-end">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button aria-label={t('clustersPage.clusterActions', { name: item.name })} size="icon" variant="ghost">
                  <MoreHorizontal className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {deleteStatus === 'delete_failed'
                  ? (
                      <DropdownMenuItem variant="destructive" onSelect={() => onDelete(item)}>
                        <Trash2 className="size-4" />
                        {t('kubectlAccess.retryDelete')}
                      </DropdownMenuItem>
                    )
                  : (
                      <>
                        <DropdownMenuItem onSelect={() => onTest(item.id)}>
                          <FlaskConical className="size-4" />
                          {t('common.test')}
                        </DropdownMenuItem>
                        {kubectlGatewayAvailable && isKubeGatewayEligible(item) && (
                          <DropdownMenuItem onSelect={() => onConfigureKubeGateway(item)}>
                            <Shield className="size-4" />
                            {t('kubectlAccess.gatewayAction')}
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem onSelect={() => onEdit(item)}>
                          <Pencil className="size-4" />
                          {t('common.edit')}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem variant="destructive" onSelect={() => onDelete(item)}>
                          <Trash2 className="size-4" />
                          {t('common.delete')}
                        </DropdownMenuItem>
                      </>
                    )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )
      },
    },
  ]

  return (
    <DataList
      columns={columns}
      emptyTitle={t('deploymentsPage.emptyClusters')}
      items={clusters}
      loading={loading}
      pagination={{
        ...pagination,
        pageInfoLabel: t('pagination.pageInfo', {
          page: pagination.page,
          totalPages: pagination.totalPages,
          total: pagination.total,
        }),
        pageSizeOptions: [10, 20, 50, 100],
      }}
      rowKey={item => item.id}
    />
  )
}

function isKubeGatewayEligible(cluster: RuntimeCluster) {
  return (cluster.deleteStatus ?? 'active') === 'active'
    && (cluster.type === 'kubernetes' || cluster.type === 'k3s')
}
