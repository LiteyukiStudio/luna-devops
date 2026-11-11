import type { NetworkServiceShare, RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

/**
 * 待批准的跨项目共享：提供方在本集群持有出站例外，共享服务可能把内网访问转给其他项目空间，
 * 需要集群维护者确认后才生效。没有待批准项时不渲染。
 */
export function ClusterNetworkShareApprovals({ cluster }: { cluster: RuntimeCluster }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [approving, setApproving] = useState<NetworkServiceShare | null>(null)
  const queryKey = ['runtime-cluster-network-share-approvals', cluster.id]
  const approvals = useQuery({
    queryKey: [...queryKey, page, pageSize],
    queryFn: () => api.listRuntimeClusterNetworkShareApprovals(cluster.id, { page, pageSize }),
  })
  const approve = useMutation({
    mutationFn: (share: NetworkServiceShare) => api.approveRuntimeClusterNetworkShare(cluster.id, share.id),
    onSuccess: () => {
      toast.success(t('clustersPage.shareApprovals.approved'))
      setApproving(null)
      void queryClient.invalidateQueries({ queryKey })
    },
    onError: error => toast.error(error.message),
  })
  if (!approvals.isError && (approvals.data?.total ?? 0) === 0)
    return null
  return (
    <div className="grid gap-3">
      <Alert variant="warning">
        <ShieldAlert />
        <AlertTitle>{t('clustersPage.shareApprovals.title')}</AlertTitle>
        <AlertDescription>{t('clustersPage.shareApprovals.description')}</AlertDescription>
      </Alert>
      <DataList
        columns={[
          { key: 'provider', header: t('clustersPage.shareApprovals.provider'), render: share => share.providerProjectName },
          { key: 'service', header: t('clustersPage.shareApprovals.service'), render: share => `${share.applicationName || share.deploymentTargetId} / ${share.deploymentTargetName}` },
          { key: 'consumer', header: t('clustersPage.shareApprovals.consumer'), render: share => share.consumerProjectName },
          {
            key: 'actions',
            header: t('common.actions'),
            className: 'text-right whitespace-nowrap',
            width: 'actions',
            render: share => <Button size="sm" variant="outline" onClick={() => setApproving(share)}>{t('clustersPage.shareApprovals.approve')}</Button>,
          },
        ]}
        emptyTitle={t('clustersPage.shareApprovals.emptyTitle')}
        error={approvals.isError ? { title: t('clustersPage.shareApprovals.loadFailed'), onRetry: () => void approvals.refetch(), retrying: approvals.isFetching } : undefined}
        items={approvals.data?.items ?? []}
        loading={approvals.isLoading}
        pagination={{
          page: approvals.data?.page ?? page,
          pageSize: approvals.data?.pageSize ?? pageSize,
          total: approvals.data?.total ?? 0,
          totalPages: approvals.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', { page: approvals.data?.page ?? page, totalPages: approvals.data?.totalPages ?? 0, total: approvals.data?.total ?? 0 }),
          onPageChange: setPage,
          onPageSizeChange: (next) => {
            setPageSize(next)
            setPage(1)
          },
        }}
        rowKey={share => share.id}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('clustersPage.shareApprovals.approve')}
        description={t('clustersPage.shareApprovals.confirmDescription', { provider: approving?.providerProjectName ?? '', consumer: approving?.consumerProjectName ?? '' })}
        open={Boolean(approving)}
        pending={approve.isPending}
        title={t('clustersPage.shareApprovals.confirmTitle')}
        onConfirm={() => approving && approve.mutate(approving)}
        onOpenChange={open => !open && setApproving(null)}
      />
    </div>
  )
}
