import type { ProjectVolume, ProjectVolumeDeletionPreview, ProjectVolumeUpdateInput, VolumeExportCreateInput, VolumeTransfer } from '@/api'
import type { ProjectVolumeCapabilities } from '@/lib/project-volume-capabilities'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, FileJson, Pencil, RefreshCcw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DescriptionListItem } from '@/components/common/description-list-item'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { canCancelVolumeTransfer, canRetryProjectVolume, canRetryVolumeTransfer } from '@/lib/project-volume-capabilities'
import { startNativeVolumeTransferDownload } from './volume-transfer-download'

function formatBytes(value: number) {
  if (value <= 0)
    return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

function transferProgress(transfer: VolumeTransfer) {
  if (transfer.expectedBytes <= 0)
    return 0
  return Math.min(100, (transfer.transferredBytes / transfer.expectedBytes) * 100)
}

export function ProjectVolumeDetailSheet({ capabilities, clusterId, onOpenChange, projectId, volumeId }: {
  capabilities: ProjectVolumeCapabilities
  clusterId: string
  onOpenChange: (open: boolean) => void
  projectId: string
  volumeId: string | null
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [exportOpen, setExportOpen] = useState(false)
  const [deletePreview, setDeletePreview] = useState<ProjectVolumeDeletionPreview | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const detail = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-volume-detail', projectId, clusterId, volumeId],
    queryFn: () => api.getProjectVolume(projectId, volumeId!, { bindingPage: 1, bindingPageSize: 20, transferPage: 1, transferPageSize: 20 }),
    enabled: Boolean(projectId && volumeId),
    refetchInterval: query => query.state.data?.recentTransfers.some(item => ['created', 'preparing', 'ready', 'streaming'].includes(item.state)) ? 3000 : false,
  })

  const retryVolume = useMutation({
    mutationFn: (volume: ProjectVolume) => api.retryProjectVolumeOperation(projectId, volume.id, volume.revision),
    onSuccess: () => {
      toast.success(t('projectVolumes.retrySucceeded'))
      invalidateVolumeQueries(queryClient, projectId, volumeId)
    },
    onError: error => toast.error(error.message),
  })
  const previewDeletion = useMutation({
    mutationFn: (id: string) => api.previewProjectVolumeDeletion(projectId, id),
    onSuccess: (preview) => {
      setDeletePreview(preview)
      setDeleteOpen(true)
    },
    onError: error => toast.error(error.message),
  })
  const deleteVolume = useMutation({
    mutationFn: (volume: ProjectVolume) => api.deleteProjectVolume(projectId, volume.id, volume.revision, deletePreview?.dataAction ?? (volume.ownershipMode === 'managed' ? 'delete' : 'detach')),
    onSuccess: () => {
      toast.success(t('projectVolumes.deleteSucceeded'))
      setDeleteOpen(false)
      onOpenChange(false)
      invalidateVolumeQueries(queryClient, projectId, volumeId)
    },
    onError: error => toast.error(error.message),
  })

  const volume = detail.data
  const deletionBlocked = Boolean(deletePreview?.hasActiveBindings || deletePreview?.hasRunningTransfers)
  const retryAllowed = Boolean(volume && canRetryProjectVolume(capabilities, volume))
  return (
    <>
      <Sheet open={Boolean(volumeId)} onOpenChange={onOpenChange}>
        <SheetContent className="w-[min(96vw,64rem)] max-w-none gap-0 overflow-y-auto p-0 sm:max-w-none" side="right">
          <SheetHeader className="border-b border-border p-6 pr-14">
            <SheetTitle>{volume?.displayName ?? t('projectVolumes.details')}</SheetTitle>
            <SheetDescription>{t('projectVolumes.detailsDescription')}</SheetDescription>
          </SheetHeader>
          <div key={volumeId ?? 'none'} className="grid gap-4 p-6">
            {detail.isError && <ErrorState description={detail.error.message} title={t('projectVolumes.loadFailedTitle')} />}
            {detail.isLoading && <div className="h-40 animate-pulse rounded-container bg-muted" />}
            {volume && (
              <>
                <div className="flex flex-wrap items-center gap-2">
                  <StatusValueBadge labelKeyPrefix="projectVolumes.lifecycleStates" value={volume.lifecycleState} />
                  <StatusValueBadge labelKeyPrefix="projectVolumes.availabilityStates" value={volume.availability} />
                  {capabilities.canWrite && (
                    <Button size="sm" variant="outline" onClick={() => setEditOpen(true)}>
                      <Pencil className="size-4" />
                      {t('projectVolumes.edit')}
                    </Button>
                  )}
                  {capabilities.canExport && <Button size="sm" variant="outline" onClick={() => setExportOpen(true)}>{t('projectVolumes.export')}</Button>}
                  {volume.lifecycleState === 'error' && retryAllowed && (
                    <Button disabled={retryVolume.isPending} size="sm" variant="outline" onClick={() => retryVolume.mutate(volume)}>
                      <RefreshCcw className="size-4" />
                      {t('projectVolumes.retry')}
                    </Button>
                  )}
                  {capabilities.canDelete && (
                    <Button disabled={previewDeletion.isPending} size="sm" variant="destructive" onClick={() => previewDeletion.mutate(volume.id)}>
                      <Trash2 className="size-4" />
                      {t('projectVolumes.delete')}
                    </Button>
                  )}
                </div>

                <Section title={t('projectVolumes.desiredState')} variant="bordered">
                  <dl className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4 [&_[data-slot=description-list-value]]:break-words [&_[data-slot=description-list-value]]:font-medium">
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.claim')} value={`${volume.namespace}/${volume.claimName}`} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.capacity')} value={volume.capacity} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.storageClass')} value={volume.storageClassName} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.accessMode')} value={volume.accessMode} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.volumeMode')} value={volume.volumeMode} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.cluster')} value={volume.clusterId} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.source')} value={t(`projectVolumes.sourceKinds.${volume.sourceKind}`)} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.ownership')} value={t(`projectVolumes.ownershipModes.${volume.ownershipMode}`)} />
                  </dl>
                </Section>

                <Section title={t('projectVolumes.liveState')} variant="bordered">
                  <dl className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4 [&_[data-slot=description-list-value]]:break-words [&_[data-slot=description-list-value]]:font-medium">
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.observation')} value={<StatusValueBadge labelKeyPrefix="projectVolumes.availabilityStates" value={volume.observation.status} />} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.capacity')} value={volume.observation.capacity} />
                    <DescriptionListItem emptyFallback="—" label={t('projectVolumes.storageClass')} value={volume.observation.storageClassName} />
                    <DescriptionListItem
                      emptyFallback="—"
                      label={t('projectVolumes.observationResult')}
                      value={volume.observation.observationCode
                        ? t(`errors.${volume.observation.observationCode}`, { defaultValue: t('errors.request.failed') })
                        : t('common.ready')}
                    />
                  </dl>
                </Section>

                <Section title={t('projectVolumes.bindings')} variant="bordered">
                  {volume.bindings.length === 0
                    ? <p className="text-sm text-muted-foreground">{t('projectVolumes.noBindings')}</p>
                    : (
                        <div className="divide-y divide-border">
                          {volume.bindings.map(binding => (
                            <div key={binding.id} className="flex flex-wrap items-center justify-between gap-3 py-3 first:pt-0 last:pb-0">
                              <div className="min-w-0">
                                <p className="truncate text-sm font-medium">{binding.logicalName}</p>
                                <p className="mt-1 text-xs text-muted-foreground">{t('projectVolumes.bindingTarget', { applicationId: binding.applicationId, targetId: binding.deploymentTargetId })}</p>
                                <p className="mt-1 text-xs text-muted-foreground">{t('projectVolumes.bindingPath', { path: binding.mountPath || binding.devicePath || '—' })}</p>
                              </div>
                              <StatusValueBadge labelKeyPrefix="projectVolumes.bindingStates" value={binding.activationState} />
                            </div>
                          ))}
                        </div>
                      )}
                </Section>

                <Section title={t('projectVolumes.transfers')} variant="bordered">
                  {volume.recentTransfers.length === 0
                    ? <p className="text-sm text-muted-foreground">{t('projectVolumes.noTransfers')}</p>
                    : (
                        <div className="grid gap-3">
                          {volume.recentTransfers.map(transfer => (
                            <TransferRow key={transfer.id} capabilities={capabilities} projectId={projectId} transfer={transfer} volumeName={volume.displayName} />
                          ))}
                        </div>
                      )}
                </Section>
              </>
            )}
          </div>
        </SheetContent>
      </Sheet>
      {volume && capabilities.canWrite && <ProjectVolumeEditDialog onOpenChange={setEditOpen} open={editOpen} projectId={projectId} volume={volume} />}
      {volume && capabilities.canExport && <ProjectVolumeExportDialog onOpenChange={setExportOpen} open={exportOpen} projectId={projectId} volume={volume} />}
      {volume && capabilities.canDelete && (
        <ConfirmDialog
          closeOnConfirm={false}
          confirmDisabled={deletionBlocked}
          confirmText={deletePreview?.dataAction === 'detach' ? t('projectVolumes.dataActionDetach') : t('projectVolumes.dataActionDelete')}
          description={deletePreview?.dataAction === 'detach' ? t('projectVolumes.deleteDescriptionReferenced') : t('projectVolumes.deleteDescriptionManaged')}
          open={deleteOpen}
          pending={deleteVolume.isPending}
          title={t('projectVolumes.deletePreview')}
          content={deletePreview && (
            <div className="grid gap-2 rounded-md bg-muted p-3 text-sm">
              <p>{t('projectVolumes.deleteBindings', { count: deletePreview.bindings.length })}</p>
              <p>{t('projectVolumes.deleteTransfers', { count: deletePreview.runningTransfers.length })}</p>
              {deletionBlocked && <p className="font-medium text-danger">{t('projectVolumes.deleteBlocked')}</p>}
            </div>
          )}
          onConfirm={() => deleteVolume.mutateAsync(volume).then(() => undefined)}
          onOpenChange={setDeleteOpen}
        />
      )}
    </>
  )
}

function invalidateVolumeQueries(queryClient: ReturnType<typeof useQueryClient>, projectId: string, volumeId: string | null) {
  queryClient.invalidateQueries({ queryKey: ['project-volumes', projectId] })
  if (volumeId)
    queryClient.invalidateQueries({ queryKey: ['project-volume-detail', projectId] })
}

function ProjectVolumeEditDialog({ onOpenChange, open, projectId, volume }: { onOpenChange: (open: boolean) => void, open: boolean, projectId: string, volume: ProjectVolume }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const form = useForm<{ capacity: string, displayName: string }>({
    defaultValues: { displayName: volume.displayName, capacity: volume.capacity },
    mode: 'onChange',
    resolver: zodResolver(z.object({
      displayName: z.string().trim().min(1, t('common.required')).max(120, t('projectVolumes.nameTooLong')),
      capacity: z.string().trim().min(1, t('common.required')),
    })),
  })
  const update = useMutation({
    mutationFn: (values: ProjectVolumeUpdateInput) => api.updateProjectVolume(projectId, volume.id, volume.revision, values),
    onSuccess: () => {
      toast.success(t('projectVolumes.updateSucceeded'))
      onOpenChange(false)
      invalidateVolumeQueries(queryClient, projectId, volume.id)
    },
    onError: error => toast.error(error.message),
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('projectVolumes.edit')}</DialogTitle>
          <DialogDescription>{t('projectVolumes.editDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={form.handleSubmit(values => update.mutate(values))}>
          <Field error={form.formState.errors.displayName?.message} label={t('projectVolumes.name')} required><Input {...form.register('displayName')} aria-invalid={Boolean(form.formState.errors.displayName)} /></Field>
          <Field error={form.formState.errors.capacity?.message} label={t('projectVolumes.capacity')} required><Input {...form.register('capacity')} aria-invalid={Boolean(form.formState.errors.capacity)} /></Field>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
            <Button disabled={!form.formState.isValid || update.isPending} type="submit">{t('common.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ProjectVolumeExportDialog({ onOpenChange, open, projectId, volume }: { onOpenChange: (open: boolean) => void, open: boolean, projectId: string, volume: ProjectVolume }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const format: VolumeExportCreateInput['format'] = volume.volumeMode === 'Block' ? 'raw_zst' : 'tar_gz'
  const form = useForm<VolumeExportCreateInput>({ defaultValues: { consistency: 'auto', format } })
  const create = useMutation({
    mutationFn: (values: VolumeExportCreateInput) => api.createVolumeExport(projectId, volume.id, values),
    onSuccess: () => {
      toast.success(t('projectVolumes.exportPreparing'))
      onOpenChange(false)
      invalidateVolumeQueries(queryClient, projectId, volume.id)
    },
    onError: error => toast.error(error.message),
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('projectVolumes.export')}</DialogTitle>
          <DialogDescription>{t('projectVolumes.exportDescription')}</DialogDescription>
        </DialogHeader>
        <Alert>
          <Download />
          <AlertTitle>{t('projectVolumes.directTransfer')}</AlertTitle>
          <AlertDescription>{t('projectVolumes.exportNoResume')}</AlertDescription>
        </Alert>
        <form className="grid gap-4" onSubmit={form.handleSubmit(values => create.mutate(values))}>
          <Field label={t('projectVolumes.archiveFormat')}>
            <input type="hidden" {...form.register('format')} />
            <NativeSelect disabled value={format}>
              {format === 'tar_gz'
                ? <option value="tar_gz">{t('projectVolumes.formatTarGz')}</option>
                : <option value="raw_zst">{t('projectVolumes.formatRawZst')}</option>}
            </NativeSelect>
          </Field>
          <Field label={t('projectVolumes.consistency')}>
            <NativeSelect {...form.register('consistency')}>
              <option value="auto">{t('projectVolumes.consistencyAuto')}</option>
              <option value="snapshot">{t('projectVolumes.consistencySnapshot')}</option>
              <option value="live">{t('projectVolumes.consistencyLive')}</option>
            </NativeSelect>
          </Field>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
            <Button disabled={create.isPending} type="submit">{t('projectVolumes.export')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function TransferRow({ capabilities, projectId, transfer, volumeName }: { capabilities: ProjectVolumeCapabilities, projectId: string, transfer: VolumeTransfer, volumeName: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [downloadStage, setDownloadStage] = useState<'idle' | 'starting'>('idle')
  const cancel = useMutation({
    mutationFn: () => api.cancelVolumeTransfer(projectId, transfer.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['project-volume-detail', projectId] }),
    onError: error => toast.error(error.message),
  })
  const retry = useMutation({
    mutationFn: () => api.retryVolumeTransfer(projectId, transfer.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['project-volume-detail', projectId] }),
    onError: error => toast.error(error.message),
  })

  async function download() {
    const filename = transfer.sourceFilename || `${volumeName}.${transfer.format === 'tar_gz' ? 'tar.gz' : 'raw.zst'}`
    setDownloadStage('starting')
    try {
      await startNativeVolumeTransferDownload({ filename, projectId, resource: 'content', transferId: transfer.id })
      toast.success(t('projectVolumes.downloadStarted'))
      queryClient.invalidateQueries({ queryKey: ['project-volume-detail', projectId] })
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
    }
    finally {
      setDownloadStage('idle')
    }
  }

  const manifestDownload = useMutation({
    mutationFn: () => startNativeVolumeTransferDownload({
      filename: `${volumeName}.raw.zst.manifest.json`,
      projectId,
      resource: 'manifest',
      transferId: transfer.id,
    }),
    onSuccess: () => toast.success(t('projectVolumes.manifestDownloadSucceeded')),
    onError: error => toast.error(error instanceof Error ? error.message : t('errors.request.failed')),
  })

  const active = ['created', 'preparing', 'ready', 'streaming'].includes(transfer.state)
  const manifestAvailable = transfer.direction === 'export' && transfer.format === 'raw_zst' && transfer.state === 'succeeded'
    && transfer.logicalBytes > 0 && Boolean(transfer.dataSHA256)
  const canDownload = capabilities.canExport
  const canCancel = canCancelVolumeTransfer(capabilities, transfer)
  const canRetry = canRetryVolumeTransfer(capabilities, transfer)
  return (
    <div className="grid gap-2 rounded-md bg-muted p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <div className="flex items-center gap-2">
            <StatusValueBadge labelKeyPrefix="projectVolumes.transferDirections" value={transfer.direction} />
            <StatusValueBadge labelKeyPrefix="projectVolumes.transferStates" value={transfer.state} />
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{formatSmartDateTime(transfer.updatedAt, t)}</p>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          {canDownload && manifestAvailable && (
            <Button disabled={manifestDownload.isPending} size="sm" variant="outline" onClick={() => manifestDownload.mutate()}>
              <FileJson className="size-4" />
              {t('projectVolumes.downloadManifest')}
            </Button>
          )}
          {canDownload && transfer.state === 'ready' && transfer.direction === 'export' && (
            <Button disabled={downloadStage === 'starting'} size="sm" variant="outline" onClick={download}>
              <Download className="size-4" />
              {downloadStage === 'starting' ? t('projectVolumes.startingDownload') : t('projectVolumes.download')}
            </Button>
          )}
          {active && canCancel && <Button disabled={cancel.isPending} size="sm" variant="ghost" onClick={() => cancel.mutate()}>{t('projectVolumes.cancelTransfer')}</Button>}
          {(transfer.state === 'failed' || transfer.state === 'cancelled' || transfer.state === 'expired') && canRetry && <Button disabled={retry.isPending} size="sm" variant="ghost" onClick={() => retry.mutate()}>{t('projectVolumes.retryTransfer')}</Button>}
        </div>
      </div>
      {transfer.expectedBytes > 0 && <div className="h-1.5 overflow-hidden rounded-full bg-background"><div className="h-full bg-primary" style={{ width: `${transferProgress(transfer)}%` }} /></div>}
      <p className="text-xs text-muted-foreground">
        {transfer.expectedBytes > 0
          ? t('projectVolumes.transferProgress', { transferred: formatBytes(transfer.transferredBytes), total: formatBytes(transfer.expectedBytes) })
          : t('projectVolumes.transferredBytes', { transferred: formatBytes(transfer.transferredBytes) })}
      </p>
      {transfer.lastErrorCode && (
        <Alert variant="destructive">
          <AlertTitle>{t('projectVolumes.transferError')}</AlertTitle>
          <AlertDescription>
            {t(`errors.${transfer.lastErrorCode}`, { defaultValue: t('errors.request.failed') })}
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
