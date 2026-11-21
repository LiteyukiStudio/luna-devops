import type { ProjectVolume, RuntimeCluster } from '@/api'
import type { RuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'
import { useQuery } from '@tanstack/react-query'
import { ExternalLink, Plus, RotateCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { StorageQuotaInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { Skeleton } from '@/components/ui/skeleton'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isValidResourceQuantity } from '@/lib/resource-units'
import { platformDefaultRuntimeCluster } from '@/lib/runtime-cluster-default'
import { emptyRuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'

interface RuntimeDataVolumesEditorProps {
  clusterId: string
  hideAddButton?: boolean
  projectId: string
  rowErrors?: Array<string | undefined>
  runtimeClusters: RuntimeCluster[]
  rows: RuntimeDataVolumeRow[]
  onChange: (rows: RuntimeDataVolumeRow[]) => void
}

export function RuntimeDataVolumesEditor({ clusterId, hideAddButton = false, onChange, projectId, rowErrors = [], rows, runtimeClusters }: RuntimeDataVolumesEditorProps) {
  const { t } = useTranslation()

  return (
    <div className="grid min-w-0 gap-3">
      {rows.length === 0 && <p className="text-sm text-muted-foreground">{t('deploymentTargetEditor.dataVolumesEmpty')}</p>}
      {rows.map((volume, index) => {
        const errorMessage = rowErrors[index]
        const hasError = Boolean(errorMessage)
        const errorId = `deployment-data-volume-${volume.id}-error`
        const nameMissing = !volume.name.trim()
        const emptyDirPathInvalid = volume.sourceType === 'emptyDir' && !volume.mountPath.trim().startsWith('/')
        const emptyDirSizeInvalid = volume.sourceType === 'emptyDir'
          && Boolean(volume.emptyDirSizeLimit.trim())
          && !isValidResourceQuantity(volume.emptyDirSizeLimit)
        const projectVolumeSelectionInvalid = volume.sourceType === 'projectVolume' && !volume.projectVolumeId.trim().startsWith('pvol_')
        const projectPath = (volume.devicePath || volume.mountPath).trim()
        const projectVolumePathInvalid = volume.sourceType === 'projectVolume'
          && !projectVolumeSelectionInvalid
          && (!projectPath.startsWith('/') || Boolean(volume.mountPath.trim()) === Boolean(volume.devicePath.trim()))
        const fallbackNameInvalid = hasError
          && !nameMissing
          && !emptyDirPathInvalid
          && !emptyDirSizeInvalid
          && !projectVolumeSelectionInvalid
          && !projectVolumePathInvalid

        return (
          <div
            key={volume.id}
            aria-label={t('deploymentTargetEditor.dataVolumeRowLabel', { index: index + 1 })}
            className="grid min-w-0 gap-3 rounded-control border border-border bg-surface-subtle/50 p-3 data-[invalid=true]:border-danger/60 data-[invalid=true]:ring-1 data-[invalid=true]:ring-danger/30 @lg/workspace:grid-cols-2"
            data-invalid={hasError}
            role="group"
          >
            <Field label={t('deploymentsPage.dataVolumeName')}>
              <Input
                aria-describedby={hasError ? errorId : undefined}
                aria-invalid={(hasError && (nameMissing || fallbackNameInvalid)) ? true : undefined}
                aria-label={t('deploymentsPage.dataVolumeName')}
                className="font-mono"
                placeholder={t('deploymentsPage.dataVolumeNamePlaceholder')}
                value={volume.name}
                onChange={(event) => {
                  const nextRows = [...rows]
                  nextRows[index] = { ...volume, name: event.target.value }
                  onChange(nextRows)
                }}
              />
            </Field>
            <Field label={t('deploymentsPage.dataVolumeSourceType')}>
              {fieldControl => (
                <Select
                  value={volume.sourceType}
                  onValueChange={(next) => {
                    const nextRows = [...rows]
                    nextRows[index] = { ...volume, sourceType: next as RuntimeDataVolumeRow['sourceType'] }
                    onChange(nextRows)
                  }}
                >
                  <SelectTrigger {...fieldControl} aria-label={t('deploymentsPage.dataVolumeSourceType')} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="projectVolume">{t('projectVolumes.deploymentSelectorLabel')}</SelectItem>
                    <SelectItem value="emptyDir">{t('deploymentsPage.dataVolumeSourceEmptyDir')}</SelectItem>
                  </SelectContent>
                </Select>
              )}
            </Field>
            <Field label={t('deploymentsPage.dataMountPath')}>
              {control => volume.sourceType === 'projectVolume'
                ? (
                    <ProjectVolumePathInput
                      ariaDescribedBy={hasError ? errorId : control['aria-describedby']}
                      ariaInvalid={hasError && projectVolumePathInvalid}
                      clusterId={clusterId}
                      currentVolumeId={volume.projectVolumeId}
                      devicePath={volume.devicePath}
                      id={control.id}
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
                      id={control.id}
                      aria-describedby={hasError ? errorId : control['aria-describedby']}
                      aria-invalid={(hasError && emptyDirPathInvalid) ? true : undefined}
                      aria-label={t('deploymentsPage.dataMountPath')}
                      className="font-mono"
                      placeholder={t('deploymentsPage.dataMountPathPlaceholder')}
                      value={volume.mountPath}
                      onChange={(event) => {
                        const nextRows = [...rows]
                        nextRows[index] = { ...volume, mountPath: event.target.value }
                        onChange(nextRows)
                      }}
                    />
                  )}
            </Field>
            <Field label={t('deploymentsPage.dataVolumeSourceDetail')}>
              {() => volume.sourceType === 'projectVolume'
                ? (
                    <ProjectVolumePicker
                      key={`${clusterId}-${volume.id}`}
                      ariaDescribedBy={hasError ? errorId : undefined}
                      ariaInvalid={hasError && projectVolumeSelectionInvalid}
                      clusterId={clusterId}
                      currentVolumeId={volume.projectVolumeId}
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
                : (
                    <div className="grid min-w-0 gap-2 @lg/workspace:grid-cols-2">
                      <Select
                        value={toSelectValue(volume.emptyDirMedium)}
                        onValueChange={(next) => {
                          const value = fromSelectValue(next)
                          const nextRows = [...rows]
                          nextRows[index] = { ...volume, emptyDirMedium: value === 'Memory' ? 'Memory' : '' }
                          onChange(nextRows)
                        }}
                      >
                        <SelectTrigger
                          aria-describedby={hasError ? errorId : undefined}
                          aria-label={t('deploymentTargetEditor.emptyDirMediumLabel')}
                          className="w-full"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={EMPTY_SELECT_VALUE}>{t('deploymentsPage.emptyDirMediumDefault')}</SelectItem>
                          <SelectItem value="Memory">{t('deploymentsPage.emptyDirMediumMemory')}</SelectItem>
                        </SelectContent>
                      </Select>
                      <StorageQuotaInput
                        inputProps={{
                          'aria-describedby': hasError ? errorId : undefined,
                          'aria-invalid': (hasError && emptyDirSizeInvalid) ? true : undefined,
                          'aria-label': t('deploymentTargetEditor.emptyDirSizeLimitLabel'),
                          'placeholder': t('deploymentTargetEditor.optional'),
                        }}
                        value={volume.emptyDirSizeLimit}
                        onChange={(value) => {
                          const nextRows = [...rows]
                          nextRows[index] = { ...volume, emptyDirSizeLimit: value }
                          onChange(nextRows)
                        }}
                      />
                    </div>
                  )}
            </Field>
            <div className="flex min-w-0 items-center justify-between gap-3 @lg/workspace:col-span-2">
              {volume.sourceType === 'projectVolume'
                ? (
                    <label className="flex min-w-0 items-center gap-2 text-sm">
                      <Checkbox
                        checked={volume.readOnly}
                        onCheckedChange={(checked) => {
                          const nextRows = [...rows]
                          nextRows[index] = { ...volume, readOnly: checked === true }
                          onChange(nextRows)
                        }}
                      />
                      {t('projectVolumes.readOnly')}
                    </label>
                  )
                : <span />}
              <Button
                aria-label={t('deploymentsPage.removeDataVolume')}
                size="icon"
                type="button"
                variant="ghost"
                onClick={() => onChange(rows.filter(row => row.id !== volume.id))}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
            {errorMessage && <p className="text-xs text-danger @lg/workspace:col-span-2" id={errorId} role="alert">{errorMessage}</p>}
          </div>
        )
      })}
      {!hideAddButton && (
        <div>
          <Button
            size="sm"
            type="button"
            variant="outline"
            onClick={() => onChange([...rows, emptyRuntimeDataVolumeRow(rows.length)])}
          >
            <Plus className="size-4" />
            {t('deploymentsPage.addDataVolume')}
          </Button>
        </div>
      )}
    </div>
  )
}

function ProjectVolumePathInput({
  ariaDescribedBy,
  ariaInvalid,
  clusterId,
  currentVolumeId,
  devicePath,
  id,
  mountPath,
  onChange,
  projectId,
}: {
  ariaDescribedBy?: string
  ariaInvalid?: boolean
  clusterId: string
  currentVolumeId: string
  devicePath: string
  id?: string
  mountPath: string
  onChange: (pathType: 'device' | 'mount', value: string) => void
  projectId: string
}) {
  const { t } = useTranslation()
  const current = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-project-volume-current', projectId, clusterId, currentVolumeId],
    queryFn: () => api.getProjectVolume(projectId, currentVolumeId),
    enabled: Boolean(projectId && currentVolumeId),
  })
  if (currentVolumeId && current.isLoading)
    return <Skeleton className="h-9 w-full rounded-control" />

  const blockMode = current.data?.volumeMode === 'Block' || Boolean(devicePath)
  return (
    <div className="grid gap-1.5">
      <Input
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid ? true : undefined}
        aria-label={t('deploymentsPage.dataMountPath')}
        className="font-mono"
        disabled={!currentVolumeId || current.isError}
        id={id}
        placeholder={blockMode ? t('projectVolumes.devicePathPlaceholder') : t('deploymentsPage.dataMountPathPlaceholder')}
        value={blockMode ? devicePath : mountPath}
        onChange={event => onChange(blockMode ? 'device' : 'mount', event.target.value)}
      />
      {current.isError && (
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-danger" role="alert">
          <span>{t('projectVolumes.loadFailedDescription')}</span>
          <Button disabled={current.isFetching} size="sm" type="button" variant="outline" onClick={() => void current.refetch()}>
            <RotateCw className="size-3.5" />
            {t('common.retry')}
          </Button>
        </div>
      )}
    </div>
  )
}

function ProjectVolumePicker({
  ariaDescribedBy,
  ariaInvalid,
  clusterId,
  currentVolumeId,
  onChange,
  projectId,
  runtimeClusters,
}: {
  ariaDescribedBy?: string
  ariaInvalid?: boolean
  clusterId: string
  currentVolumeId: string
  onChange: (volume?: ProjectVolume) => void
  projectId: string
  runtimeClusters: RuntimeCluster[]
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const effectiveClusterId = clusterId || platformDefaultRuntimeCluster(runtimeClusters)?.id || ''
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
    enabled: Boolean(projectId && effectiveClusterId),
  })
  const current = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-project-volume-current', projectId, effectiveClusterId, currentVolumeId],
    queryFn: () => api.getProjectVolume(projectId, currentVolumeId),
    enabled: Boolean(projectId && currentVolumeId),
  })
  const options = [...(volumes.data?.items ?? [])]
  if (current.data?.clusterId === effectiveClusterId && !options.some(item => item.id === current.data?.id))
    options.unshift(current.data)
  const selected = options.find(item => item.id === currentVolumeId)
  const selectDisabled = !effectiveClusterId || volumes.isLoading || volumes.isError

  return (
    <div className="grid min-w-0 gap-2">
      <Input
        aria-describedby={ariaInvalid && selectDisabled ? ariaDescribedBy : undefined}
        aria-invalid={(ariaInvalid && selectDisabled && Boolean(effectiveClusterId)) ? true : undefined}
        aria-label={t('deploymentTargetEditor.projectVolumeSearchLabel')}
        disabled={!effectiveClusterId}
        placeholder={t('projectVolumes.deploymentSelectorPlaceholder')}
        value={search}
        onChange={(event) => {
          setSearch(event.target.value)
          setPage(1)
        }}
      />
      {Boolean(effectiveClusterId) && volumes.isLoading
        ? <Skeleton className="h-9 w-full rounded-control" />
        : (
            <Select
              disabled={selectDisabled}
              value={currentVolumeId || undefined}
              onValueChange={next => onChange(options.find(item => item.id === next))}
            >
              <SelectTrigger
                aria-describedby={ariaDescribedBy}
                aria-invalid={(ariaInvalid && !selectDisabled) ? true : undefined}
                aria-label={t('projectVolumes.deploymentSelectorLabel')}
                className="w-full"
              >
                <SelectValue placeholder={t('projectVolumes.deploymentSelectorPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {options.map(item => (
                  <SelectItem key={item.id} value={item.id}>
                    {item.displayName}
                    {' '}
                    ·
                    {item.capacity}
                    {item.id === currentVolumeId && item.availability !== 'available' ? ` · ${t('projectVolumes.deploymentSelectorCurrent')}` : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
      {!volumes.isLoading && !volumes.isError && options.length === 0 && <p className="text-xs text-muted-foreground">{t('projectVolumes.deploymentSelectorEmpty')}</p>}
      {volumes.isError && (
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-danger" role="alert">
          <span>{t('projectVolumes.loadFailedDescription')}</span>
          <Button disabled={volumes.isFetching} size="sm" type="button" variant="outline" onClick={() => void volumes.refetch()}>
            <RotateCw className="size-3.5" />
            {t('common.retry')}
          </Button>
        </div>
      )}
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <div className="flex shrink-0 gap-1">
          <Button disabled={page <= 1} size="sm" type="button" variant="ghost" onClick={() => setPage(value => Math.max(1, value - 1))}>{t('pagination.previous')}</Button>
          <Button disabled={page >= (volumes.data?.totalPages ?? 0)} size="sm" type="button" variant="ghost" onClick={() => setPage(value => value + 1)}>{t('pagination.next')}</Button>
        </div>
        <Button asChild className="max-w-full" size="sm" type="button" variant="ghost">
          <Link className="min-w-0" to={`/projects/${encodeURIComponent(projectId)}?tab=volumes`} target="_blank">
            <ExternalLink className="size-3.5" />
            <span className="truncate">{t('projectVolumes.openVolumeCenter')}</span>
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
