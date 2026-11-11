import type { NetworkServiceShare, NetworkServiceShareDirection, NetworkServiceSharePayload } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, ApiError } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { EgressExceptionWarning } from '@/components/common/egress-exception-warning'
import { FormField } from '@/components/common/form-field'
import { SearchSelect } from '@/components/common/search-select'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

const statusTone = { active: 'success', pending_acceptance: 'info', pending_approval: 'warning' } as const

function shareQueryKey(projectId: string) {
  return ['project-network-shares', projectId]
}

/** 跨项目共享：本项目空间提供给其他项目空间的服务，以及其他项目空间共享给本项目空间的服务。只打通网络，不注入凭据。 */
export function ProjectNetworkSharesCard({ canManage, projectId }: { canManage: boolean, projectId: string }) {
  const { t } = useTranslation()
  return (
    <Card className="grid gap-4 p-5">
      <div className="grid gap-1">
        <h2 className="text-base font-semibold">{t('projectTopology.networkShares.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('projectTopology.networkShares.description')}</p>
      </div>
      <ShareList canManage={canManage} direction="provided" projectId={projectId} />
      <ShareList canManage={canManage} direction="consumed" projectId={projectId} />
    </Card>
  )
}

function ShareList({ canManage, direction, projectId }: { canManage: boolean, direction: NetworkServiceShareDirection, projectId: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [creating, setCreating] = useState(false)
  const [removing, setRemoving] = useState<NetworkServiceShare | null>(null)
  const shares = useQuery({
    queryKey: [...shareQueryKey(projectId), direction, page, pageSize],
    queryFn: () => api.listProjectNetworkShares(projectId, direction, { page, pageSize }),
  })
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: shareQueryKey(projectId) })
  const accept = useMutation({
    mutationFn: (share: NetworkServiceShare) => api.acceptProjectNetworkShare(projectId, share.id),
    onSuccess: (share) => {
      toast.success(t(share.status === 'active' ? 'projectTopology.networkShares.accepted' : 'projectTopology.networkShares.acceptedPendingApproval'))
      invalidate()
    },
    onError: error => toast.error(error.message),
  })
  const remove = useMutation({
    mutationFn: (share: NetworkServiceShare) => api.deleteProjectNetworkShare(projectId, share.id),
    onSuccess: () => {
      toast.success(t(`projectTopology.networkShares.${direction}.removed`))
      setRemoving(null)
      invalidate()
    },
    onError: error => toast.error(error.message),
  })
  const service = (share: NetworkServiceShare) => (
    <span className="grid gap-0.5">
      <span className="font-medium">{`${share.applicationName || share.deploymentTargetId} / ${share.deploymentTargetName}`}</span>
      {direction === 'consumed' && share.address ? <span className="font-mono text-xs text-muted-foreground">{share.address}</span> : null}
    </span>
  )
  return (
    <>
      <DataList
        columns={[
          { key: 'service', header: t('projectTopology.networkShares.service'), render: service },
          {
            key: 'project',
            header: t(`projectTopology.networkShares.${direction}.project`),
            render: share => direction === 'provided' ? share.consumerProjectName : share.providerProjectName,
          },
          {
            key: 'status',
            header: t('common.status'),
            render: share => <StatusBadge tone={statusTone[share.status]}>{t(`projectTopology.networkShares.statuses.${share.status}`)}</StatusBadge>,
          },
          ...(canManage
            ? [{
                key: 'actions',
                header: t('common.actions'),
                className: 'text-right whitespace-nowrap',
                width: 'actions' as const,
                render: (share: NetworkServiceShare) => (
                  <div className="flex justify-end gap-1">
                    {direction === 'consumed' && !share.acceptedAt
                      ? (
                          <Button disabled={accept.isPending} size="sm" variant="outline" onClick={() => accept.mutate(share)}>
                            <Check className="size-4" />
                            {t('projectTopology.networkShares.accept')}
                          </Button>
                        )
                      : null}
                    <Button aria-label={t(`projectTopology.networkShares.${direction}.removeNamed`, { name: share.deploymentTargetName })} size="icon" variant="ghost" onClick={() => setRemoving(share)}>
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ),
              }]
            : []),
        ]}
        emptyDescription={t(`projectTopology.networkShares.${direction}.emptyDescription`)}
        emptyTitle={t(`projectTopology.networkShares.${direction}.emptyTitle`)}
        error={shares.isError ? { title: t('projectTopology.networkShares.loadFailed'), onRetry: () => void shares.refetch(), retrying: shares.isFetching } : undefined}
        items={shares.data?.items ?? []}
        loading={shares.isLoading}
        pagination={{
          page: shares.data?.page ?? page,
          pageSize: shares.data?.pageSize ?? pageSize,
          total: shares.data?.total ?? 0,
          totalPages: shares.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', { page: shares.data?.page ?? page, totalPages: shares.data?.totalPages ?? 0, total: shares.data?.total ?? 0 }),
          onPageChange: setPage,
          onPageSizeChange: (next) => {
            setPageSize(next)
            setPage(1)
          },
        }}
        rowKey={share => share.id}
        title={t(`projectTopology.networkShares.${direction}.title`)}
        toolbarActions={direction === 'provided' && canManage
          ? (
              <Button onClick={() => setCreating(true)}>
                <Plus className="size-4" />
                {t('projectTopology.networkShares.create')}
              </Button>
            )
          : undefined}
      />
      {creating
        ? (
            <ShareFormDialog
              projectId={projectId}
              onOpenChange={open => !open && setCreating(false)}
              onSaved={() => {
                setCreating(false)
                invalidate()
              }}
            />
          )
        : null}
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t(`projectTopology.networkShares.${direction}.remove`)}
        description={t(`projectTopology.networkShares.${direction}.removeDescription`, { name: removing?.deploymentTargetName ?? '' })}
        open={Boolean(removing)}
        pending={remove.isPending}
        title={t(`projectTopology.networkShares.${direction}.removeTitle`)}
        onConfirm={() => removing && remove.mutate(removing)}
        onOpenChange={open => !open && setRemoving(null)}
      />
    </>
  )
}

function shareSchema(message: { target: string, consumer: string }) {
  return z.object({
    deploymentTargetId: z.string().min(1, message.target),
    consumerProjectIdentifier: z.string().trim().min(1, message.consumer),
  })
}

const errorFields: Partial<Record<string, keyof NetworkServiceSharePayload>> = {
  'network_share.target_invalid': 'deploymentTargetId',
  'network_share.limit_exceeded': 'deploymentTargetId',
  'network_share.consumer_invalid': 'consumerProjectIdentifier',
  'network_share.duplicate': 'consumerProjectIdentifier',
}

/** 提供方选择一个部署配置并填写使用方的项目空间标识；使用方确认后才生效。 */
function ShareFormDialog({ onOpenChange, onSaved, projectId }: { onOpenChange: (open: boolean) => void, onSaved: () => void, projectId: string }) {
  const { t } = useTranslation()
  const schema = useMemo(() => shareSchema({
    target: t('projectTopology.networkShares.targetRequired'),
    consumer: t('projectTopology.networkShares.consumerRequired'),
  }), [t])
  const form = useForm<NetworkServiceSharePayload>({ resolver: zodResolver(schema), defaultValues: { deploymentTargetId: '', consumerProjectIdentifier: '' } })
  const topology = useQuery({ queryKey: ['project-topology', projectId, 'network-share-targets'], queryFn: () => api.getProjectTopology(projectId) })
  const targetOptions = (topology.data?.nodes ?? []).flatMap(node => node.deploymentTargets.map(target => ({
    label: `${node.name} / ${target.name}`,
    value: target.id,
    description: target.stage,
  })))
  const save = useMutation({
    mutationFn: (values: NetworkServiceSharePayload) => api.createProjectNetworkShare(projectId, values),
    onSuccess: (share) => {
      toast.success(t(share.approvalRequired ? 'projectTopology.networkShares.createdNeedsApproval' : 'projectTopology.networkShares.created'))
      onSaved()
    },
    onError: (error) => {
      const field = error instanceof ApiError ? errorFields[error.code] : undefined
      if (field)
        form.setError(field, { message: error.message })
      toast.error(error.message)
    },
  })
  const errors = form.formState.errors
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <form className="grid gap-4" onSubmit={form.handleSubmit(values => save.mutate(values))}>
          <DialogHeader>
            <DialogTitle>{t('projectTopology.networkShares.createTitle')}</DialogTitle>
            <DialogDescription>{t('projectTopology.networkShares.createDescription')}</DialogDescription>
          </DialogHeader>
          <EgressExceptionWarning context="share" projectId={projectId} />
          <FormField error={errors.deploymentTargetId?.message} label={t('projectTopology.networkShares.target')} required>
            <Controller
              control={form.control}
              name="deploymentTargetId"
              render={({ field }) => (
                <SearchSelect
                  ariaLabel={t('projectTopology.networkShares.target')}
                  loading={topology.isFetching}
                  options={targetOptions}
                  value={field.value}
                  onValueChange={field.onChange}
                />
              )}
            />
          </FormField>
          <FormField error={errors.consumerProjectIdentifier?.message} hint={t('projectTopology.networkShares.consumerHint')} label={t('projectTopology.networkShares.consumer')} required>
            <Input className="font-mono" {...form.register('consumerProjectIdentifier')} />
          </FormField>
          <DialogFooter>
            <DialogClose asChild><Button disabled={save.isPending} type="button" variant="outline">{t('common.cancel')}</Button></DialogClose>
            <Button disabled={save.isPending} type="submit">{t('projectTopology.networkShares.create')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
