import type { CurrentUser, Project, RuntimeCluster, RuntimeClusterPressure } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useQuery } from '@tanstack/react-query'
import { FlaskConical, MoreHorizontal, Pencil, ShieldAlert, ShieldCheck, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { DataList } from '@/components/common/data-list'
import { RuntimeClusterPressureRings } from '@/components/common/runtime-cluster-pressure'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { canManageCluster, gatewayAccessSummary, scopeLabel } from './cluster-helpers'

export function RuntimeClusterTable({ clusters, loading, pagination, pressureByClusterId, pressureLoading, projects, user, onDelete, onEdit, onEgressExceptions, onTest }: {
  clusters: RuntimeCluster[]
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
  onDelete: (cluster: RuntimeCluster) => void
  onEdit: (cluster: RuntimeCluster) => void
  onTest: (clusterId: string) => void
  onEgressExceptions: (cluster: RuntimeCluster) => void
}) {
  const { t } = useTranslation()
  const projectMap = Object.fromEntries(projects.map(project => [project.id, project]))
  const columns: DataListColumn<RuntimeCluster>[] = [
    {
      key: 'name',
      header: t('common.name'),
      width: 'primary',
      render: item => (
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium">{item.name}</span>
            {item.isDefault && <StatusBadge tone="info">{t('clustersPage.defaultCluster')}</StatusBadge>}
            {canManageCluster(item, user?.id, user?.role) && (item.deleteStatus ?? 'active') === 'active'
              ? <PendingShareApprovalsBadge cluster={item} onOpen={onEgressExceptions} />
              : null}
          </div>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {scopeLabel(item, projectMap, t)}
            {' · '}
            {t('clustersPage.maxConcurrentBuilds')}
            {' '}
            {item.maxConcurrentBuilds || 4}
          </p>
        </div>
      ),
    },
    { key: 'pressure', header: t('clustersPage.resourcePressure'), width: 'secondary', render: item => <RuntimeClusterPressureRings loading={pressureLoading} pressure={pressureByClusterId[item.id]} /> },
    { key: 'gateway', header: t('clustersPage.gatewayExternalAccessConfig'), width: 'secondary', render: item => <span className="font-mono text-xs">{gatewayAccessSummary(item)}</span> },
    {
      key: 'status',
      header: t('common.status'),
      width: 'status',
      render: (item) => {
        const deleteStatus = item.deleteStatus ?? 'active'
        if (deleteStatus !== 'active')
          return <StatusValueBadge labelKeyPrefix="clustersPage.deleteStatuses" value={deleteStatus} />
        return <StatusValueBadge value={pressureByClusterId[item.id]?.status ?? item.status} />
      },
    },
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
          return <span className="text-xs text-muted-foreground">{t('clustersPage.deleteInProgress')}</span>

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
                        {t('clustersPage.retryDelete')}
                      </DropdownMenuItem>
                    )
                  : (
                      <>
                        <DropdownMenuItem onSelect={() => onTest(item.id)}>
                          <FlaskConical className="size-4" />
                          {t('common.test')}
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => onEdit(item)}>
                          <Pencil className="size-4" />
                          {t('common.edit')}
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => onEgressExceptions(item)}>
                          <ShieldCheck className="size-4" />
                          {t('clustersPage.egressExceptions.menu')}
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

/** 集群维护者可见：有待批准的跨项目共享时提示数量，点击打开“出站例外”窗口处理。 */
function PendingShareApprovalsBadge({ cluster, onOpen }: { cluster: RuntimeCluster, onOpen: (cluster: RuntimeCluster) => void }) {
  const { t } = useTranslation()
  // 与审批列表共用前缀，批准后一并失效刷新。
  const approvals = useQuery({
    queryKey: ['runtime-cluster-network-share-approvals', cluster.id, 'count'],
    queryFn: () => api.listRuntimeClusterNetworkShareApprovals(cluster.id, { page: 1, pageSize: 1 }),
  })
  const count = approvals.data?.total ?? 0
  if (count === 0)
    return null
  return (
    <Button className="h-auto p-0" size="sm" variant="link" onClick={() => onOpen(cluster)}>
      <StatusBadge tone="warning">
        <ShieldAlert className="size-3" />
        {t('clustersPage.shareApprovals.pendingCount', { count })}
      </StatusBadge>
    </Button>
  )
}
