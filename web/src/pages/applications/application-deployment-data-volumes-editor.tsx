import type { ProjectVolume, RuntimeCluster } from '@/api'
import type { RuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'
import { useQuery } from '@tanstack/react-query'
import { ExternalLink, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { emptyRuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'

interface RuntimeDataVolumesEditorProps {
  enabled: boolean
  clusterId: string
  runtimeClusters: RuntimeCluster[]
  rows: RuntimeDataVolumeRow[]
  onChange: (rows: RuntimeDataVolumeRow[]) => void
}

export function RuntimeDataVolumesEditor({ clusterId, enabled, onChange, rows, runtimeClusters }: RuntimeDataVolumesEditorProps) {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()

  return (
    <Field hint={t('deploymentsPage.dataVolumesHint')} label={t('deploymentsPage.dataVolumes')}>
      <div className="grid gap-2 rounded-md border border-input bg-background p-3">
        <div className="hidden gap-2 px-1 text-xs font-medium text-muted-foreground md:grid md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1fr)_minmax(0,1.4fr)_minmax(10rem,0.8fr)_auto]">
          <span>{t('deploymentsPage.dataVolumeName')}</span>
          <span>{t('deploymentsPage.dataVolumeSourceType')}</span>
          <span>{t('deploymentsPage.dataMountPath')}</span>
          <span>{t('deploymentsPage.dataVolumeSourceDetail')}</span>
          <span className="sr-only">{t('common.actions')}</span>
        </div>
        {rows.map((volume, index) => (
          <div key={volume.id} className="grid gap-2 md:grid-cols-[minmax(7rem,0.7fr)_minmax(0,1fr)_minmax(0,1.4fr)_minmax(10rem,0.8fr)_auto]">
            <Input
              disabled={!enabled}
              placeholder={t('deploymentsPage.dataVolumeNamePlaceholder')}
              value={volume.name}
              onChange={(event) => {
                const nextRows = [...rows]
                nextRows[index] = { ...volume, name: event.target.value }
                onChange(nextRows)
              }}
            />
            <Select
              disabled={!enabled}
              value={volume.sourceType}
              onChange={(event) => {
                const nextRows = [...rows]
                nextRows[index] = { ...volume, sourceType: event.target.value as RuntimeDataVolumeRow['sourceType'] }
                onChange(nextRows)
              }}
            >
              <option value="projectVolume">{t('projectVolumes.deploymentSelectorLabel')}</option>
              <option value="emptyDir">{t('deploymentsPage.dataVolumeSourceEmptyDir')}</option>
            </Select>
            {volume.sourceType === 'projectVolume'
              ? (
                  <ProjectVolumePathInput
                    clusterId={clusterId}
                    currentVolumeId={volume.projectVolumeId}
                    devicePath={volume.devicePath}
                    enabled={enabled}
                    mountPath={volume.mountPath}
                    projectId={projectId}
                    onChange={(pathType, value) => {
                      const nextRows = [...rows]
                      nextRows[index] = {
                        ...volume,
                        devicePath: pathType === 'device' ? value : '',
                        mountPath: pathType === 'mount' ? value : '',
                      }
                      onChange(nextRows)
                    }}
                  />
                )
              : (
                  <Input
                    disabled={!enabled}
                    placeholder={t('deploymentsPage.dataMountPathPlaceholder')}
                    value={volume.mountPath}
                    onChange={(event) => {
                      const nextRows = [...rows]
                      nextRows[index] = { ...volume, mountPath: event.target.value }
                      onChange(nextRows)
                    }}
                  />
                )}
            {volume.sourceType === 'projectVolume'
              ? (
                  <ProjectVolumePicker
                    key={`${clusterId}-${volume.id}`}
                    clusterId={clusterId}
                    currentVolumeId={volume.projectVolumeId}
                    enabled={enabled}
                    projectId={projectId}
                    runtimeClusters={runtimeClusters}
                    onChange={(selected) => {
                      const nextRows = [...rows]
                      nextRows[index] = {
                        ...volume,
                        projectVolumeId: selected?.id ?? '',
                        devicePath: selected?.volumeMode === 'Block' ? (volume.devicePath || '/dev/data') : '',
                        mountPath: selected?.volumeMode === 'Block' ? '' : (volume.mountPath || '/data'),
                      }
                      onChange(nextRows)
                    }}
                  />
                )
              : volume.sourceType === 'emptyDir'
                ? (
                    <div className="grid gap-2 md:grid-cols-2">
                      <Select
                        disabled={!enabled}
                        value={volume.emptyDirMedium}
                        onChange={(event) => {
                          const nextRows = [...rows]
                          nextRows[index] = { ...volume, emptyDirMedium: event.target.value === 'Memory' ? 'Memory' : '' }
                          onChange(nextRows)
                        }}
                      >
                        <option value="">{t('deploymentsPage.emptyDirMediumDefault')}</option>
                        <option value="Memory">{t('deploymentsPage.emptyDirMediumMemory')}</option>
                      </Select>
                      <Input
                        disabled={!enabled}
                        placeholder={t('deploymentsPage.emptyDirSizeLimitPlaceholder')}
                        value={volume.emptyDirSizeLimit}
                        onChange={(event) => {
                          const nextRows = [...rows]
                          nextRows[index] = { ...volume, emptyDirSizeLimit: event.target.value }
                          onChange(nextRows)
                        }}
                      />
                    </div>
                  )
                : null}
            <Button
              aria-label={t('deploymentsPage.removeDataVolume')}
              disabled={!enabled}
              size="icon"
              type="button"
              variant="ghost"
              onClick={() => onChange(rows.filter(row => row.id !== volume.id))}
            >
              <Trash2 className="size-4" />
            </Button>
            {volume.sourceType === 'projectVolume' && (
              <label className="flex items-center gap-2 text-sm md:col-span-4">
                <Checkbox
                  checked={volume.readOnly}
                  disabled={!enabled}
                  onCheckedChange={(checked) => {
                    const nextRows = [...rows]
                    nextRows[index] = { ...volume, readOnly: checked === true }
                    onChange(nextRows)
                  }}
                />
                {t('projectVolumes.readOnly')}
              </label>
            )}
          </div>
        ))}
        <div>
          <Button
            disabled={!enabled}
            size="sm"
            type="button"
            variant="secondary"
            onClick={() => onChange([...rows, emptyRuntimeDataVolumeRow(rows.length)])}
          >
            <Plus className="size-4" />
            {t('deploymentsPage.addDataVolume')}
          </Button>
        </div>
      </div>
    </Field>
  )
}

function ProjectVolumePathInput({ clusterId, currentVolumeId, devicePath, enabled, mountPath, onChange, projectId }: {
  clusterId: string
  currentVolumeId: string
  devicePath: string
  enabled: boolean
  mountPath: string
  onChange: (pathType: 'device' | 'mount', value: string) => void
  projectId: string
}) {
  const { t } = useTranslation()
  const current = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-project-volume-current', projectId, clusterId, currentVolumeId],
    queryFn: () => api.getProjectVolume(projectId, currentVolumeId),
    enabled: enabled && Boolean(projectId && currentVolumeId),
  })
  const blockMode = current.data?.volumeMode === 'Block' || Boolean(devicePath)
  return (
    <Input
      disabled={!enabled || !currentVolumeId || current.isLoading || current.isError}
      placeholder={blockMode ? t('projectVolumes.devicePathPlaceholder') : t('deploymentsPage.dataMountPathPlaceholder')}
      value={blockMode ? devicePath : mountPath}
      onChange={event => onChange(blockMode ? 'device' : 'mount', event.target.value)}
    />
  )
}

function ProjectVolumePicker({ clusterId, currentVolumeId, enabled, onChange, projectId, runtimeClusters }: {
  clusterId: string
  currentVolumeId: string
  enabled: boolean
  onChange: (volume?: ProjectVolume) => void
  projectId: string
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const effectiveClusterId = clusterId || runtimeClusters.find(item => item.isDefault)?.id || ''
  const volumes = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-project-volumes', projectId, effectiveClusterId, currentVolumeId, page, search],
    queryFn: () => api.listProjectVolumes(projectId, {
      page,
      pageSize: 20,
      search: search.trim() || undefined,
      clusterId: effectiveClusterId,
      availability: 'available',
      sortBy: 'displayName',
      sortOrder: 'asc',
    }),
    enabled: enabled && Boolean(projectId && effectiveClusterId),
  })
  const current = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-project-volume-current', projectId, effectiveClusterId, currentVolumeId],
    queryFn: () => api.getProjectVolume(projectId, currentVolumeId),
    enabled: enabled && Boolean(projectId && currentVolumeId),
  })
  const options = [...(volumes.data?.items ?? [])]
  if (current.data?.clusterId === effectiveClusterId && !options.some(item => item.id === current.data?.id))
    options.unshift(current.data)
  const selected = options.find(item => item.id === currentVolumeId)
  return (
    <div className="grid min-w-0 gap-2">
      <Input
        disabled={!enabled || !effectiveClusterId}
        placeholder={t('projectVolumes.deploymentSelectorPlaceholder')}
        value={search}
        onChange={(event) => {
          setSearch(event.target.value)
          setPage(1)
        }}
      />
      <Select
        disabled={!enabled || !effectiveClusterId || volumes.isLoading || volumes.isError}
        value={currentVolumeId}
        onChange={event => onChange(options.find(item => item.id === event.target.value))}
      >
        <option value="">{t('projectVolumes.deploymentSelectorPlaceholder')}</option>
        {options.map(item => (
          <option key={item.id} value={item.id}>
            {item.displayName}
            {' '}
            ·
            {item.capacity}
            {item.id === currentVolumeId && item.availability !== 'available' ? ` · ${t('projectVolumes.deploymentSelectorCurrent')}` : ''}
          </option>
        ))}
      </Select>
      {!volumes.isLoading && !volumes.isError && options.length === 0 && <p className="text-xs text-muted-foreground">{t('projectVolumes.deploymentSelectorEmpty')}</p>}
      {volumes.isError && <p className="text-xs text-danger">{t('projectVolumes.loadFailedDescription')}</p>}
      <div className="flex items-center justify-between gap-2">
        <div className="flex gap-1">
          <Button disabled={page <= 1} size="sm" type="button" variant="ghost" onClick={() => setPage(value => Math.max(1, value - 1))}>{t('pagination.previous')}</Button>
          <Button disabled={page >= (volumes.data?.totalPages ?? 0)} size="sm" type="button" variant="ghost" onClick={() => setPage(value => value + 1)}>{t('pagination.next')}</Button>
        </div>
        <Button asChild size="sm" type="button" variant="ghost">
          <Link to={`/projects/${encodeURIComponent(projectId)}?tab=volumes`} target="_blank">
            <ExternalLink className="size-3.5" />
            {t('projectVolumes.openVolumeCenter')}
          </Link>
        </Button>
      </div>
      {selected && (
        <p className="truncate text-xs text-muted-foreground">
          {selected.namespace}
          /
          {selected.claimName}
        </p>
      )}
    </div>
  )
}
