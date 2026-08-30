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
import { PageShell } from '@/components/common/page-shell'
import { ProjectVolumeCreateDialog } from '@/components/common/project-volumes/project-volume-create-dialog'
import { useProjectRuntimeClusters } from '@/components/common/project-volumes/volume-resource-queries'
import { ProjectVolumeClusterSelect } from '@/components/common/project-volumes/volume-resource-selectors'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
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
    <div className="grid min-w-0 gap-2 md:flex md:flex-wrap">
      <div className="w-full md:w-44">
        <ProjectVolumeClusterSelect allowAll projectId={projectId} value={clusterId} onChange={onClusterChange} />
      </div>
      <NativeSelect aria-label={t('projectVolumes.allAvailability')} containerClassName="w-full md:w-40" value={availability} onChange={event => onAvailabilityChange(event.target.value as '' | ProjectVolumeAvailability)}>
        <option value="">{t('projectVolumes.allAvailability')}</option>
        <option value="available">{t('projectVolumes.availabilityStates.available')}</option>
        <option value="reserved">{t('projectVolumes.availabilityStates.reserved')}</option>
        <option value="in_use">{t('projectVolumes.availabilityStates.in_use')}</option>
        <option value="unavailable">{t('projectVolumes.availabilityStates.unavailable')}</option>
      </NativeSelect>
      <NativeSelect aria-label={t('projectVolumes.allSources')} containerClassName="w-full md:w-40" value={sourceKind} onChange={event => onSourceChange(event.target.value as '' | ProjectVolumeSourceKind)}>
        <option value="">{t('projectVolumes.allSources')}</option>
        {(['blank', 'managed', 'retained', 'archive_import', 'snapshot_restore', 'existing_claim'] as const).map(value => <option key={value} value={value}>{t(`projectVolumes.sourceKinds.${value}`)}</option>)}
      </NativeSelect>
      <NativeSelect aria-label={t('projectVolumes.allOwnership')} containerClassName="w-full md:w-40" value={ownership} onChange={event => onOwnershipChange(event.target.value as '' | ProjectVolumeOwnershipMode)}>
        <option value="">{t('projectVolumes.allOwnership')}</option>
        <option value="managed">{t('projectVolumes.ownershipModes.managed')}</option>
        <option value="referenced">{t('projectVolumes.ownershipModes.referenced')}</option>
      </NativeSelect>
      <NativeSelect aria-label={t('projectVolumes.sort')} containerClassName="w-full md:w-40" value={sortBy} onChange={event => onSortChange(event.target.value as typeof sortBy)}>
        <option value="createdAt">{t('projectVolumes.sortCreated')}</option>
        <option value="updatedAt">{t('projectVolumes.sortUpdated')}</option>
        <option value="displayName">{t('projectVolumes.sortName')}</option>
        <option value="capacity">{t('projectVolumes.sortCapacity')}</option>
      </NativeSelect>
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
      <PageShell>
        <ErrorState
          description={t(forbidden ? 'projectVolumes.forbiddenDescription' : 'projectVolumes.loadFailedDescription')}
          title={t(forbidden ? 'projectVolumes.forbiddenTitle' : 'projectVolumes.loadFailedTitle')}
        />
      </PageShell>
    )
  }

  const hasFilters = Boolean(search || clusterId || availability || sourceKind || ownership || sortBy !== 'createdAt')
  return (
    <PageShell>
      <Section description={t('projectVolumes.description')} icon={<HardDrive className="size-5" />} title={t('projectVolumes.title')} variant="plain">
        <div className="min-w-0 overflow-hidden rounded-container bg-surface-raised">
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
                <div className="hidden min-w-0 flex-1 md:block">{filters}</div>
                <Sheet open={filtersOpen} onOpenChange={setFiltersOpen}>
                  <SheetTrigger asChild>
                    <Button className="md:hidden" size="sm" variant="outline">
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
        </div>
      </Section>
      {capabilities.canWrite && <ProjectVolumeCreateDialog onOpenChange={setCreateOpen} open={createOpen} projectId={projectId} />}
      {capabilities.canImport && <ProjectVolumeImportDialog onOpenChange={setImportOpen} open={importOpen} projectId={projectId} />}
      <ProjectVolumeDetailSheet
        key={selectedVolume?.id ?? 'closed'}
        capabilities={capabilities}
        clusterId={selectedVolume?.clusterId ?? ''}
        projectId={projectId}
        volumeId={selectedVolume?.id ?? null}
        onOpenChange={open => !open && setSelectedVolume(null)}
      />
    </PageShell>
  )
}

export default ProjectVolumesPage
