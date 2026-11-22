import type { ProjectVolume } from '@/api'
import type { FormFieldControlProps } from '@/components/common/form-field'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { ProjectVolumeCreateDialog } from '@/components/common/project-volumes/project-volume-create-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

/** 选择或就地创建与模板声明兼容（同集群、同卷模式、可用）的项目数据卷。 */
export function TemplateProjectVolumePicker({
  canCreate,
  clusterId,
  clusterName,
  control,
  disabled,
  mode,
  onChange,
  onRetryUpstream,
  projectId,
  upstreamError = false,
  upstreamLoading = false,
  upstreamRetrying = false,
  value,
}: {
  canCreate: boolean
  clusterId: string
  clusterName: string
  control?: FormFieldControlProps
  disabled: boolean
  mode: 'Block' | 'Filesystem'
  onChange: (value: string) => void
  onRetryUpstream?: () => void
  projectId: string
  upstreamError?: boolean
  upstreamLoading?: boolean
  upstreamRetrying?: boolean
  value: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [createdVolume, setCreatedVolume] = useState<ProjectVolume | null>(null)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const volumesEnabled = Boolean(projectId && clusterId)
  const volumes = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['app-template-project-volumes', projectId, clusterId, mode, page, search],
    queryFn: () => api.listProjectVolumes(projectId, {
      page,
      pageSize: 20,
      search: search.trim() || undefined,
      availability: 'available',
      clusterId,
      volumeMode: mode,
      sortBy: 'displayName',
      sortOrder: 'asc',
    }),
    enabled: volumesEnabled,
  })
  const alreadyInOptions = createdVolume?.id === value || Boolean(volumes.data?.items.some(item => item.id === value))
  const currentEnabled = Boolean(projectId && clusterId && value && !alreadyInOptions)
  const current = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['app-template-project-volume-current', projectId, clusterId, value],
    queryFn: () => api.getProjectVolume(projectId, value),
    enabled: currentEnabled,
  })

  if (upstreamLoading) {
    return (
      <div aria-busy="true" className="grid min-w-0 gap-2" data-slot="template-volume-picker-skeleton">
        <Skeleton className="h-9 w-full rounded-control" />
        <Skeleton className="h-9 w-full rounded-control" />
      </div>
    )
  }

  const options = [...(volumes.data?.items ?? [])]
  if (createdVolume?.clusterId === clusterId && createdVolume.volumeMode === mode && !options.some(item => item.id === createdVolume.id))
    options.unshift(createdVolume)
  if (current.data?.clusterId === clusterId && current.data.volumeMode === mode && !options.some(item => item.id === current.data?.id))
    options.unshift(current.data)
  const isLoading = (volumesEnabled && volumes.isPending) || (currentEnabled && current.isPending)
  const isError = upstreamError || (volumesEnabled && volumes.isError) || (currentEnabled && current.isError)
  const retrying = Boolean(
    (upstreamError && upstreamRetrying)
    || (volumesEnabled && volumes.isFetching)
    || (currentEnabled && current.isFetching),
  )
  const canRetry = !upstreamError || Boolean(onRetryUpstream)
  const handleRetry = () => {
    if (upstreamError)
      onRetryUpstream?.()
    if (volumesEnabled && volumes.isError)
      void volumes.refetch()
    if (currentEnabled && current.isError)
      void current.refetch()
  }
  const totalPages = volumes.data?.totalPages ?? 0
  return (
    <div className="grid min-w-0 gap-2">
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
        <Input
          aria-label={t('projectVolumes.deploymentSelectorPlaceholder')}
          disabled={disabled || !clusterId || upstreamLoading || upstreamError}
          placeholder={t('projectVolumes.deploymentSelectorPlaceholder')}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value)
            setPage(1)
          }}
        />
        {canCreate && (
          <Button disabled={disabled || !clusterId || upstreamLoading || upstreamError} type="button" variant="outline" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            {t('projectVolumes.create')}
          </Button>
        )}
      </div>
      <Select
        disabled={disabled || !clusterId || isLoading || isError}
        value={!isLoading && !isError && options.some(volume => volume.id === value) ? value : ''}
        onValueChange={(next) => {
          if (next)
            onChange(next)
        }}
      >
        <SelectTrigger {...control} className="w-full">
          <SelectValue
            placeholder={
              isLoading
                ? t('common.loading')
                : isError
                  ? t('common.unavailable')
                  : !clusterId
                      ? t('common.noOptions')
                      : t('appTemplatesPage.selectProjectVolume')
            }
          />
        </SelectTrigger>
        <SelectContent>
          {options.length === 0
            ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
            : options.map(volume => (
                <SelectItem key={volume.id} value={volume.id}>
                  {volume.displayName}
                  {' '}
                  ·
                  {' '}
                  {volume.capacity}
                </SelectItem>
              ))}
        </SelectContent>
      </Select>
      {isLoading && <Skeleton className="h-4 w-40" />}
      {!isLoading && isError && (
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-danger" role="alert">
          <span>{t('projectVolumes.loadFailedDescription')}</span>
          {canRetry && (
            <Button disabled={retrying} size="sm" type="button" variant="outline" onClick={handleRetry}>
              {t('common.retry')}
            </Button>
          )}
        </div>
      )}
      {!isLoading && !isError && Boolean(clusterId) && options.length === 0 && <p className="text-xs text-muted-foreground">{t('projectVolumes.deploymentSelectorEmpty')}</p>}
      {totalPages > 1 && (
        <div className="flex items-center justify-between gap-2">
          <Button disabled={disabled || page <= 1} size="sm" type="button" variant="ghost" onClick={() => setPage(currentPage => Math.max(1, currentPage - 1))}>
            {t('pagination.previous')}
          </Button>
          <Button disabled={disabled || page >= totalPages} size="sm" type="button" variant="ghost" onClick={() => setPage(currentPage => currentPage + 1)}>
            {t('pagination.next')}
          </Button>
        </div>
      )}
      {canCreate && (
        <ProjectVolumeCreateDialog
          key={`${projectId}:${clusterId}:${mode}`}
          deploymentContext={{ clusterId, clusterName, volumeMode: mode }}
          open={createOpen}
          projectId={projectId}
          onCreated={(volume) => {
            setCreatedVolume(volume)
            onChange(volume.id)
            void queryClient.invalidateQueries({ queryKey: ['app-template-project-volumes', projectId, clusterId, mode] })
          }}
          onOpenChange={setCreateOpen}
        />
      )}
    </div>
  )
}
