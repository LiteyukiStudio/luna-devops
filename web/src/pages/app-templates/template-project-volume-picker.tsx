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
import { NativeSelect as Select } from '@/components/ui/native-select'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

/** 选择或就地创建与模板声明兼容（同集群、同卷模式、可用）的项目数据卷。 */
export function TemplateProjectVolumePicker({ canCreate, clusterId, clusterName, control, disabled, mode, onChange, projectId, value }: {
  canCreate: boolean
  clusterId: string
  clusterName: string
  control?: FormFieldControlProps
  disabled: boolean
  mode: 'Block' | 'Filesystem'
  onChange: (value: string) => void
  projectId: string
  value: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [createdVolume, setCreatedVolume] = useState<ProjectVolume | null>(null)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
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
    enabled: Boolean(projectId && clusterId),
  })
  const current = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['app-template-project-volume-current', projectId, clusterId, value],
    queryFn: () => api.getProjectVolume(projectId, value),
    enabled: Boolean(projectId && clusterId && value),
  })

  const options = [...(volumes.data?.items ?? [])]
  if (createdVolume?.clusterId === clusterId && createdVolume.volumeMode === mode && !options.some(item => item.id === createdVolume.id))
    options.unshift(createdVolume)
  if (current.data?.clusterId === clusterId && current.data.volumeMode === mode && !options.some(item => item.id === current.data?.id))
    options.unshift(current.data)
  const totalPages = volumes.data?.totalPages ?? 0
  return (
    <div className="grid min-w-0 gap-2">
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
        <Input
          aria-label={t('projectVolumes.deploymentSelectorPlaceholder')}
          disabled={disabled || !clusterId}
          placeholder={t('projectVolumes.deploymentSelectorPlaceholder')}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value)
            setPage(1)
          }}
        />
        {canCreate && (
          <Button disabled={disabled || !clusterId} type="button" variant="outline" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            {t('projectVolumes.create')}
          </Button>
        )}
      </div>
      <Select
        {...control}
        disabled={disabled || !clusterId || volumes.isLoading || volumes.isError}
        value={value}
        onChange={event => onChange(event.target.value)}
      >
        <option value="">{t('appTemplatesPage.selectProjectVolume')}</option>
        {options.map(volume => (
          <option key={volume.id} value={volume.id}>
            {volume.displayName}
            {' '}
            ·
            {' '}
            {volume.capacity}
          </option>
        ))}
      </Select>
      {!volumes.isLoading && !volumes.isError && options.length === 0 && <p className="text-xs text-muted-foreground">{t('projectVolumes.deploymentSelectorEmpty')}</p>}
      {volumes.isError && <p className="text-xs text-danger">{t('projectVolumes.loadFailedDescription')}</p>}
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
