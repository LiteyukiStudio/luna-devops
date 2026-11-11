import type { EgressException, RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ClusterEgressExceptionFormDialog } from './cluster-egress-exception-form-dialog'
import { ClusterNetworkShareApprovals } from './cluster-network-share-approvals'

/** 集群出站例外：只有集群维护者可见，用于放行平台默认拒绝的内网地址并授权给指定项目空间。 */
export function ClusterEgressExceptionsDialog({ cluster, onOpenChange }: {
  cluster: RuntimeCluster
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [editing, setEditing] = useState<EgressException | 'new' | null>(null)
  const [deleting, setDeleting] = useState<EgressException | null>(null)
  const queryKey = ['runtime-cluster-egress-exceptions', cluster.id]
  const exceptions = useQuery({
    queryKey: [...queryKey, page, pageSize],
    queryFn: () => api.listRuntimeClusterEgressExceptions(cluster.id, { page, pageSize }),
  })
  const remove = useMutation({
    mutationFn: (exception: EgressException) => api.deleteRuntimeClusterEgressException(cluster.id, exception.id),
    onSuccess: () => {
      toast.success(t('clustersPage.egressExceptions.deleted'))
      setDeleting(null)
      void queryClient.invalidateQueries({ queryKey })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t('clustersPage.egressExceptions.title', { name: cluster.name })}</DialogTitle>
          <DialogDescription>{t('clustersPage.egressExceptions.description')}</DialogDescription>
        </DialogHeader>
        <ClusterNetworkShareApprovals cluster={cluster} />
        <DataList
          columns={[
            { key: 'name', header: t('clustersPage.egressExceptions.name'), render: item => <span className="font-medium">{item.name}</span> },
            {
              key: 'target',
              header: t('clustersPage.egressExceptions.target'),
              render: item => (
                <span className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-xs">{item.cidr}</span>
                  <StatusBadge>
                    {t(`clustersPage.egressExceptions.protocols.${item.protocol}`)}
                    {item.ports ? ` ${item.ports}` : ''}
                  </StatusBadge>
                </span>
              ),
            },
            {
              key: 'grants',
              header: t('clustersPage.egressExceptions.grants'),
              render: item => item.grants.length > 0
                ? (
                    <span className="flex flex-wrap items-center gap-1">
                      {item.grants.map(grant => (
                        <span key={grant.projectId} className="inline-flex items-center gap-1">
                          {grant.projectName || grant.projectId}
                          {/* 持有例外的项目空间对外共享服务时，共享服务可能把内网访问转给其他项目空间。 */}
                          {grant.providedShares > 0 ? <StatusBadge tone="warning">{t('clustersPage.egressExceptions.providedShares', { count: grant.providedShares })}</StatusBadge> : null}
                        </span>
                      ))}
                    </span>
                  )
                : <span className="text-muted-foreground">{t('clustersPage.egressExceptions.noGrants')}</span>,
            },
            {
              key: 'actions',
              header: t('common.actions'),
              className: 'text-right whitespace-nowrap',
              width: 'actions',
              render: item => (
                <div className="flex justify-end gap-1">
                  <Button aria-label={t('clustersPage.egressExceptions.editNamed', { name: item.name })} size="icon" variant="ghost" onClick={() => setEditing(item)}>
                    <Pencil className="size-4" />
                  </Button>
                  <Button aria-label={t('clustersPage.egressExceptions.deleteNamed', { name: item.name })} size="icon" variant="ghost" onClick={() => setDeleting(item)}>
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              ),
            },
          ]}
          emptyDescription={t('clustersPage.egressExceptions.emptyDescription')}
          emptyTitle={t('clustersPage.egressExceptions.emptyTitle')}
          error={exceptions.isError ? { title: t('clustersPage.egressExceptions.loadFailed'), onRetry: () => void exceptions.refetch(), retrying: exceptions.isFetching } : undefined}
          items={exceptions.data?.items ?? []}
          loading={exceptions.isLoading}
          pagination={{
            page: exceptions.data?.page ?? page,
            pageSize: exceptions.data?.pageSize ?? pageSize,
            total: exceptions.data?.total ?? 0,
            totalPages: exceptions.data?.totalPages ?? 0,
            pageInfoLabel: t('pagination.pageInfo', { page: exceptions.data?.page ?? page, totalPages: exceptions.data?.totalPages ?? 0, total: exceptions.data?.total ?? 0 }),
            onPageChange: setPage,
            onPageSizeChange: (next) => {
              setPageSize(next)
              setPage(1)
            },
          }}
          rowKey={item => item.id}
          toolbarActions={(
            <Button onClick={() => setEditing('new')}>
              <Plus className="size-4" />
              {t('clustersPage.egressExceptions.create')}
            </Button>
          )}
        />
        {editing
          ? (
              <ClusterEgressExceptionFormDialog
                key={editing === 'new' ? 'new' : editing.id}
                cluster={cluster}
                exception={editing === 'new' ? null : editing}
                onOpenChange={open => !open && setEditing(null)}
                onSaved={() => {
                  setEditing(null)
                  void queryClient.invalidateQueries({ queryKey })
                }}
              />
            )
          : null}
        <ConfirmDialog
          cancelText={t('common.cancel')}
          confirmText={t('common.delete')}
          description={t('clustersPage.egressExceptions.deleteDescription', { name: deleting?.name ?? '' })}
          open={Boolean(deleting)}
          pending={remove.isPending}
          title={t('clustersPage.egressExceptions.deleteTitle')}
          onConfirm={() => deleting && remove.mutate(deleting)}
          onOpenChange={open => !open && setDeleting(null)}
        />
      </DialogContent>
    </Dialog>
  )
}
