import type { KubeCredential, KubeCredentialBinding } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, ShieldX } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { NativeSelect as Select } from '@/components/ui/native-select'

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export function KubeCredentialsPanel({ featureEnabled }: { featureEnabled: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState('')
  const [selectedCredential, setSelectedCredential] = useState<KubeCredential | null>(null)

  const credentials = useQuery({
    queryKey: ['kube-credentials', page, pageSize, search, status],
    queryFn: () => api.listKubeCredentials({ page, pageSize, search, sortBy: 'createdAt', sortOrder: 'desc', status: status || undefined }),
    enabled: featureEnabled,
  })
  const revokeCredential = useMutation({
    mutationFn: api.revokeKubeCredential,
    onSuccess: () => {
      toast.success(t('kubectlAccess.revoked'))
      queryClient.invalidateQueries({ queryKey: ['kube-credentials'] })
      setSelectedCredential(null)
    },
    onError: error => toast.error(error.message),
  })

  if (!featureEnabled) {
    return <EmptyState description={t('kubectlAccess.featureDisabledDescription')} title={t('kubectlAccess.featureDisabledTitle')} variant="plain" />
  }

  const columns: DataListColumn<KubeCredential>[] = [
    {
      key: 'name',
      header: t('kubectlAccess.credentials.name'),
      width: 'primary',
      render: credential => <span className="font-medium">{credential.name}</span>,
    },
    {
      key: 'scopes',
      header: t('kubectlAccess.credentials.scopes'),
      width: 'secondary',
      render: credential => (
        <div className="flex flex-wrap gap-1">
          {credential.scopes.map(scope => (
            <StatusBadge key={scope}>{t(`kubectlAccess.scopeLabels.${scope}`)}</StatusBadge>
          ))}
        </div>
      ),
    },
    {
      key: 'bindingCount',
      header: t('kubectlAccess.credentials.bindingCount'),
      width: 'number',
      render: credential => credential.bindingCount,
    },
    {
      key: 'expiresAt',
      header: t('kubectlAccess.credentials.expiresAt'),
      width: 'compact',
      render: credential => formatAbsoluteDateTime(credential.expiresAt),
    },
    {
      key: 'status',
      header: t('common.status'),
      width: 'status',
      render: credential => <StatusValueBadge labelKeyPrefix="kubectlAccess.credentialStatuses" value={credential.status} />,
    },
    {
      key: 'actions',
      header: t('common.actions'),
      sticky: 'right',
      width: 'actions',
      render: credential => (
        <div className="flex justify-end gap-2">
          <Button size="sm" variant="ghost" onClick={() => setSelectedCredential(credential)}>
            <Eye className="size-4" />
            {t('kubectlAccess.credentials.viewBindings')}
          </Button>
          <ConfirmDialog
            confirmText={t('kubectlAccess.credentials.revoke')}
            description={t('kubectlAccess.credentials.revokeDescription', { name: credential.name })}
            pending={revokeCredential.isPending}
            title={t('kubectlAccess.credentials.revokeTitle')}
            onConfirm={() => revokeCredential.mutate(credential.id)}
          >
            <Button size="sm" variant="ghost">
              <ShieldX className="size-4" />
              {t('kubectlAccess.credentials.revoke')}
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ]

  return (
    <>
      <DataList
        columns={columns}
        emptyDescription={t('kubectlAccess.credentials.emptyDescription')}
        emptyTitle={t('kubectlAccess.credentials.emptyTitle')}
        items={credentials.data?.items ?? []}
        pagination={{
          page: credentials.data?.page ?? page,
          pageSize: credentials.data?.pageSize ?? pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: credentials.data?.total ?? 0,
          totalPages: credentials.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', {
            page: credentials.data?.page ?? page,
            totalPages: credentials.data?.totalPages ?? 0,
            total: credentials.data?.total ?? 0,
          }),
          onPageChange: setPage,
          onPageSizeChange: (nextPageSize) => {
            setPage(1)
            setPageSize(nextPageSize)
          },
        }}
        rowKey={credential => credential.id}
        search={{
          value: search,
          placeholder: t('kubectlAccess.credentials.searchPlaceholder'),
          onChange: (value) => {
            setPage(1)
            setSearch(value)
          },
        }}
        toolbar={(
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm text-muted-foreground">{t('kubectlAccess.credentials.statusFilter')}</span>
            <Select
              className="h-9 w-40"
              value={status}
              onChange={(event) => {
                setPage(1)
                setStatus(event.target.value)
              }}
            >
              <option value="">{t('kubectlAccess.credentials.statusAll')}</option>
              <option value="active">{t('kubectlAccess.credentialStatuses.active')}</option>
              <option value="expired">{t('kubectlAccess.credentialStatuses.expired')}</option>
              <option value="revoked">{t('kubectlAccess.credentialStatuses.revoked')}</option>
            </Select>
          </div>
        )}
      />
      <KubeCredentialBindingsDialog credential={selectedCredential} onOpenChange={open => !open && setSelectedCredential(null)} />
    </>
  )
}

function KubeCredentialBindingsDialog({
  credential,
  onOpenChange,
}: {
  credential: KubeCredential | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const bindings = useQuery({
    queryKey: ['kube-credentials', credential?.id, 'bindings', page, pageSize],
    queryFn: () => api.listKubeCredentialBindings(credential?.id ?? '', { page, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(credential?.id),
  })

  const columns: DataListColumn<KubeCredentialBinding>[] = [
    {
      key: 'contextName',
      header: t('kubectlAccess.bindings.contextName'),
      width: 'primary',
      render: binding => <span className="font-medium">{binding.contextName}</span>,
    },
    {
      key: 'namespace',
      header: t('kubectlAccess.bindings.namespace'),
      width: 'secondary',
      render: binding => binding.namespace,
    },
    {
      key: 'projectId',
      header: t('kubectlAccess.bindings.project'),
      width: 'secondary',
      render: binding => <code className="text-xs">{binding.projectId}</code>,
    },
    {
      key: 'runtimeClusterId',
      header: t('kubectlAccess.bindings.runtimeCluster'),
      width: 'secondary',
      render: binding => <code className="text-xs">{binding.runtimeClusterId}</code>,
    },
    {
      key: 'applicationId',
      header: t('kubectlAccess.bindings.application'),
      width: 'secondary',
      render: binding => binding.applicationId ? <code className="text-xs">{binding.applicationId}</code> : t('kubectlAccess.form.applicationAll'),
    },
    {
      key: 'createdAt',
      header: t('kubectlAccess.bindings.createdAt'),
      width: 'compact',
      render: binding => formatAbsoluteDateTime(binding.createdAt),
    },
  ]

  return (
    <Dialog
      open={Boolean(credential)}
      onOpenChange={(open) => {
        if (!open) {
          setPage(1)
          setPageSize(10)
        }
        onOpenChange(open)
      }}
    >
      <DialogContent className="flex max-h-[min(88vh,50rem)] w-[min(94vw,64rem)] max-w-[94vw] min-w-0 flex-col gap-4 overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t('kubectlAccess.bindings.title', { name: credential?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('kubectlAccess.bindings.description')}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 overflow-y-auto">
          <DataList
            columns={columns}
            emptyDescription={t('kubectlAccess.bindings.emptyDescription')}
            emptyTitle={t('kubectlAccess.bindings.emptyTitle')}
            items={bindings.data?.items ?? []}
            pagination={{
              page: bindings.data?.page ?? page,
              pageSize: bindings.data?.pageSize ?? pageSize,
              pageSizeOptions: PAGE_SIZE_OPTIONS,
              total: bindings.data?.total ?? 0,
              totalPages: bindings.data?.totalPages ?? 0,
              pageInfoLabel: t('pagination.pageInfo', {
                page: bindings.data?.page ?? page,
                totalPages: bindings.data?.totalPages ?? 0,
                total: bindings.data?.total ?? 0,
              }),
              onPageChange: setPage,
              onPageSizeChange: (nextPageSize) => {
                setPage(1)
                setPageSize(nextPageSize)
              },
            }}
            rowKey={binding => binding.id}
          />
        </div>
      </DialogContent>
    </Dialog>
  )
}
