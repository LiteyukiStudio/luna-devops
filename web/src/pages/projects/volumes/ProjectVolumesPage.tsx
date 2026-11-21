import type { Ref } from 'react'
import type { ProjectVolume, ProjectVolumeAvailability, ProjectVolumeOwnershipMode, ProjectVolumeSourceKind } from '@/api'
import type { ProjectVolumeCapabilities } from '@/lib/project-volume-capabilities'
import { useQuery } from '@tanstack/react-query'
import { Filter, HardDrive, RefreshCcw } from 'lucide-react'
import { useDeferredValue, useImperativeHandle, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, ApiError } from '@/api'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { ProjectVolumeCreateDialog } from '@/components/common/project-volumes/project-volume-create-dialog'
import { useProjectRuntimeClusters } from '@/components/common/project-volumes/volume-resource-queries'
import { ProjectVolumeClusterSelect } from '@/components/common/project-volumes/volume-resource-selectors'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { ProjectVolumeDetailSheet } from './project-volume-detail-sheet'
import { ProjectVolumeImportDialog } from './project-volume-import-dialog'

export interface ProjectVolumesPageHandle {
  openCreateDialog: () => void
  openImportDialog: () => void
}

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

function VolumeFilters({ availability, clusterId, onAvailabilityChange, onClusterChange, onOwnershipChange, onSortChange, onSourceChange, ownership, projectId, sortBy, sourceKind }: {
  availability: '' | ProjectVolumeAvailability
  clusterId: string
  onAvailabilityChange: (value: '' | ProjectVolumeAvailability) => void
  onClusterChange: (value: string) => void
  onOwnershipChange: (value: '' | ProjectVolumeOwnershipMode) => void
  onSortChange: (value: 'capacity' | 'createdAt' | 'displayName' | 'updatedAt') => void
  onSourceChange: (value: '' | ProjectVolumeSourceKind) => void
  ownership: '' | ProjectVolumeOwnershipMode
  projectId: string
  sortBy: 'capacity' | 'createdAt' | 'displayName' | 'updatedAt'
  sourceKind: '' | ProjectVolumeSourceKind
}) {
  const { t } = useTranslation()
  return (
    <div className="grid min-w-0 gap-2 @xl/data-list:contents">
      <div className="w-full @xl/data-list:w-40">
        <ProjectVolumeClusterSelect allowAll projectId={projectId} value={clusterId} onChange={onClusterChange} />
      </div>
      <Select
        value={toSelectValue(availability)}
        onValueChange={next => onAvailabilityChange(fromSelectValue(next) as '' | ProjectVolumeAvailability)}
      >
        <SelectTrigger aria-label={t('projectVolumes.allAvailability')} className="w-full @xl/data-list:w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={EMPTY_SELECT_VALUE}>{t('projectVolumes.allAvailability')}</SelectItem>
          <SelectItem value="available">{t('projectVolumes.availabilityStates.available')}</SelectItem>
          <SelectItem value="reserved">{t('projectVolumes.availabilityStates.reserved')}</SelectItem>
          <SelectItem value="in_use">{t('projectVolumes.availabilityStates.in_use')}</SelectItem>
          <SelectItem value="unavailable">{t('projectVolumes.availabilityStates.unavailable')}</SelectItem>
        </SelectContent>
      </Select>
      <Select
        value={toSelectValue(sourceKind)}
        onValueChange={next => onSourceChange(fromSelectValue(next) as '' | ProjectVolumeSourceKind)}
      >
        <SelectTrigger aria-label={t('projectVolumes.allSources')} className="w-full @xl/data-list:w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={EMPTY_SELECT_VALUE}>{t('projectVolumes.allSources')}</SelectItem>
          {(['blank', 'managed', 'retained', 'archive_import', 'snapshot_restore', 'existing_claim'] as const).map(value => (
            <SelectItem key={value} value={value}>{t(`projectVolumes.sourceKinds.${value}`)}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={toSelectValue(ownership)}
        onValueChange={next => onOwnershipChange(fromSelectValue(next) as '' | ProjectVolumeOwnershipMode)}
      >
        <SelectTrigger aria-label={t('projectVolumes.allOwnership')} className="w-full @xl/data-list:w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={EMPTY_SELECT_VALUE}>{t('projectVolumes.allOwnership')}</SelectItem>
          <SelectItem value="managed">{t('projectVolumes.ownershipModes.managed')}</SelectItem>
          <SelectItem value="referenced">{t('projectVolumes.ownershipModes.referenced')}</SelectItem>
        </SelectContent>
      </Select>
      <Select
        value={sortBy}
        onValueChange={next => onSortChange(next as typeof sortBy)}
      >
        <SelectTrigger aria-label={t('projectVolumes.sort')} className="w-full @xl/data-list:w-36">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="createdAt">{t('projectVolumes.sortCreated')}</SelectItem>
          <SelectItem value="updatedAt">{t('projectVolumes.sortUpdated')}</SelectItem>
          <SelectItem value="displayName">{t('projectVolumes.sortName')}</SelectItem>
          <SelectItem value="capacity">{t('projectVolumes.sortCapacity')}</SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}

export function ProjectVolumesPage({ capabilities, projectId, ref }: { capabilities: ProjectVolumeCapabilities, projectId: string, ref?: Ref<ProjectVolumesPageHandle> }) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [search, setSearch] = useState('')
  const deferredSearch = useDeferredValue(search.trim())
  const [clusterId, setClusterId] = useState('')
  const [availability, setAvailability] = useState<'' | ProjectVolumeAvailability>('')
  const [sourceKind, setSourceKind] = useState<'' | ProjectVolumeSourceKind>('')
  const [ownership, setOwnership] = useState<'' | ProjectVolumeOwnershipMode>('')
  const [sortBy, setSortBy] = useState<'capacity' | 'createdAt' | 'displayName' | 'updatedAt'>('createdAt')
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [selectedVolume, setSelectedVolume] = useState<ProjectVolume | null>(null)
  const clusters = useProjectRuntimeClusters(projectId)
  const clusterNames = useMemo(() => Object.fromEntries((clusters.data?.pages.flatMap(item => item.items) ?? []).map(item => [item.id, item.name])), [clusters.data?.pages])

  useImperativeHandle(ref, () => ({
    openCreateDialog: () => capabilities.canWrite && setCreateOpen(true),
    openImportDialog: () => capabilities.canImport && setImportOpen(true),
  }), [capabilities.canImport, capabilities.canWrite])

  const volumes = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-volumes', projectId, { page, pageSize, search: deferredSearch, clusterId, availability, sourceKind, ownership, sortBy }],
    queryFn: () => api.listProjectVolumes(projectId, {
      page,
      pageSize,
      search: deferredSearch || undefined,
      clusterId: clusterId || undefined,
      availability: availability || undefined,
      sourceKind: sourceKind || undefined,
      ownershipMode: ownership || undefined,
      sortBy,
      sortOrder: sortBy === 'displayName' ? 'asc' : 'desc',
    }),
    enabled: Boolean(projectId),
  })

  function resetPageAnd(run: () => void) {
    setPage(1)
    run()
  }
  const clearFilters = () => {
    setPage(1)
    setSearch('')
    setClusterId('')
    setAvailability('')
    setSourceKind('')
    setOwnership('')
    setSortBy('createdAt')
  }
  const filters = (
    <VolumeFilters
      availability={availability}
      clusterId={clusterId}
      ownership={ownership}
      projectId={projectId}
      sortBy={sortBy}
      sourceKind={sourceKind}
      onAvailabilityChange={value => resetPageAnd(() => setAvailability(value))}
      onClusterChange={value => resetPageAnd(() => setClusterId(value))}
      onOwnershipChange={value => resetPageAnd(() => setOwnership(value))}
      onSortChange={value => resetPageAnd(() => setSortBy(value))}
      onSourceChange={value => resetPageAnd(() => setSourceKind(value))}
    />
  )

  if (volumes.isError) {
    const forbidden = volumes.error instanceof ApiError && volumes.error.status === 403
    return (
      <div className="grid min-w-0 gap-section">
        <ErrorState
          description={t(forbidden ? 'projectVolumes.forbiddenDescription' : 'projectVolumes.loadFailedDescription')}
          title={t(forbidden ? 'projectVolumes.forbiddenTitle' : 'projectVolumes.loadFailedTitle')}
        />
      </div>
    )
  }

  const hasFilters = Boolean(search || clusterId || availability || sourceKind || ownership || sortBy !== 'createdAt')
  return (
    <div className="grid min-w-0 gap-section">
      <DataList
        columns={[
          { key: 'name', header: t('projectVolumes.name'), width: 'primary', render: item => (
            <div className="min-w-0">
              <p className="truncate font-medium">{item.displayName}</p>
              <p className="mt-1 truncate text-xs text-muted-foreground">
                {item.namespace}
                /
                {item.claimName}
              </p>
            </div>
          ) },
          { key: 'capacity', header: t('projectVolumes.capacity'), width: 'number', render: item => (
            <div>
              <p>{item.capacity}</p>
              <p className="mt-1 truncate text-xs text-muted-foreground">{item.storageClassName}</p>
            </div>
          ) },
          { key: 'cluster', header: t('projectVolumes.cluster'), mobile: 'hidden', render: item => clusterNames[item.clusterId] ?? item.clusterId },
          { key: 'source', header: t('projectVolumes.source'), mobile: 'hidden', render: item => (
            <div className="grid gap-1">
              <span>{t(`projectVolumes.sourceKinds.${item.sourceKind}`)}</span>
              <span className="text-xs text-muted-foreground">{t(`projectVolumes.ownershipModes.${item.ownershipMode}`)}</span>
            </div>
          ) },
          { key: 'availability', header: t('projectVolumes.usage'), render: item => <StatusValueBadge labelKeyPrefix="projectVolumes.availabilityStates" value={item.availability} /> },
          { key: 'observation', header: t('projectVolumes.observation'), render: item => (
            <div className="grid gap-1">
              <StatusValueBadge labelKeyPrefix="projectVolumes.lifecycleStates" value={item.lifecycleState} />
              {item.lastErrorCode && (
                <span className="truncate text-xs text-danger">
                  {t(`errors.${item.lastErrorCode}`, { defaultValue: t('errors.request.failed') })}
                </span>
              )}
            </div>
          ) },
          { key: 'updatedAt', header: t('projectVolumes.updatedAt'), mobile: 'hidden', render: item => formatSmartDateTime(item.updatedAt, t) },
          { key: 'actions', header: t('common.actions'), sticky: 'right', width: 'actions', render: item => <Button size="sm" variant="ghost" onClick={() => setSelectedVolume(item)}>{t('projectVolumes.details')}</Button> },
        ]}
        emptyActions={hasFilters
          ? <Button variant="outline" onClick={clearFilters}>{t('projectVolumes.clearFilters')}</Button>
          : capabilities.canWrite ? <Button onClick={() => setCreateOpen(true)}>{t('projectVolumes.create')}</Button> : undefined}
        emptyDescription={t(hasFilters ? 'projectVolumes.noMatches' : 'projectVolumes.emptyDescription')}
        emptyIcon={<HardDrive className="size-6" />}
        emptyMode={hasFilters ? 'filtered' : 'actionable'}
        emptyTitle={t(hasFilters ? 'projectVolumes.noMatches' : 'projectVolumes.emptyTitle')}
        items={volumes.data?.items ?? []}
        loading={volumes.isLoading}
        pagination={{
          page: volumes.data?.page ?? page,
          pageSize: volumes.data?.pageSize ?? pageSize,
          pageSizeOptions: PAGE_SIZE_OPTIONS,
          total: volumes.data?.total ?? 0,
          totalPages: volumes.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', { page: volumes.data?.page ?? page, totalPages: volumes.data?.totalPages ?? 0, total: volumes.data?.total ?? 0 }),
          onPageChange: setPage,
          onPageSizeChange: (value) => {
            setPageSize(value)
            setPage(1)
          },
        }}
        rowActionLabel={item => t('projectVolumes.openDetailsFor', { name: item.displayName })}
        rowKey={item => item.id}
        search={{
          value: search,
          placeholder: t('projectVolumes.searchPlaceholder'),
          onChange: (value) => {
            setSearch(value)
            setPage(1)
          },
        }}
        toolbar={(
          <>
            <div className="hidden @xl/data-list:contents">{filters}</div>
            <Sheet open={filtersOpen} onOpenChange={setFiltersOpen}>
              <SheetTrigger asChild>
                <Button className="@xl/data-list:hidden" size="sm" variant="outline">
                  <Filter className="size-4" />
                  {t('projectVolumes.filters')}
                </Button>
              </SheetTrigger>
              <SheetContent className="w-[min(92vw,28rem)] max-w-none sm:max-w-md">
                <SheetHeader>
                  <SheetTitle>{t('projectVolumes.filters')}</SheetTitle>
                  <SheetDescription>{t('projectVolumes.description')}</SheetDescription>
                </SheetHeader>
                <div className="grid gap-3 p-4">
                  {filters}
                  <Button variant="outline" onClick={clearFilters}>{t('projectVolumes.clearFilters')}</Button>
                </div>
              </SheetContent>
            </Sheet>
          </>
        )}
        toolbarActions={<Button aria-label={t('projectVolumes.refresh')} size="icon" variant="ghost" onClick={() => volumes.refetch()}><RefreshCcw className="size-4" /></Button>}
        onRowClick={(item: ProjectVolume) => setSelectedVolume(item)}
      />
      {capabilities.canWrite && <ProjectVolumeCreateDialog onOpenChange={setCreateOpen} open={createOpen} projectId={projectId} />}
      {capabilities.canImport && <ProjectVolumeImportDialog onOpenChange={setImportOpen} open={importOpen} projectId={projectId} transferEnabled={capabilities.transferEnabled} />}
      <ProjectVolumeDetailSheet
        key={selectedVolume?.id ?? 'closed'}
        capabilities={capabilities}
        clusterId={selectedVolume?.clusterId ?? ''}
        projectId={projectId}
        volumeId={selectedVolume?.id ?? null}
        onOpenChange={open => !open && setSelectedVolume(null)}
      />
    </div>
  )
}

export default ProjectVolumesPage
