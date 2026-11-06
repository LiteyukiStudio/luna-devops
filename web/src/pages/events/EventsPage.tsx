import type { ReactNode } from 'react'
import type { PlatformEvent, PlatformEventSnapshot } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import type { SearchSelectOption } from '@/components/common/search-select'
import { useQueries, useQuery } from '@tanstack/react-query'
import { Activity, ExternalLink, FileKey2, Globe2, Hammer, RefreshCw, Rocket, SearchX, SlidersHorizontal, Workflow, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { ResultVisibilitySelect } from '@/components/common/result-visibility-select'
import { SearchMultiSelect } from '@/components/common/search-select'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime, formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin } from '@/lib/roles'
import { useResultVisibility } from '@/lib/use-result-visibility'
import { initialEventFilterValues, initialEventSeverityFilters } from './event-filter-defaults'

export function EventsPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const [searchParams] = useSearchParams()
  const platformAdmin = isPlatformAdmin(user?.role)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [search, setSearch] = useState('')
  const [visibility, setVisibility] = useResultVisibility(platformAdmin)
  const [projectIds, setProjectIds] = useState(() => initialEventFilterValues(searchParams, 'projectIds', 'projectId'))
  const [applicationIds, setApplicationIds] = useState(() => initialEventFilterValues(searchParams, 'applicationIds', 'applicationId'))
  const [deploymentTargetIds, setDeploymentTargetIds] = useState(() => initialEventFilterValues(searchParams, 'deploymentTargetIds', 'deploymentTargetId'))
  const [categories, setCategories] = useState(() => initialEventFilterValues(searchParams, 'categories', 'category'))
  const [eventTypes, setEventTypes] = useState(() => initialEventFilterValues(searchParams, 'types', 'type'))
  const [severities, setSeverities] = useState(() => initialEventSeverityFilters(searchParams))
  const [statuses, setStatuses] = useState(() => initialEventFilterValues(searchParams, 'statuses', 'status'))
  const [dateFrom, setDateFrom] = useState(() => dateDaysAgo(7))
  const [dateTo, setDateTo] = useState(() => dateDaysAgo(0))
  const [selectedEventId, setSelectedEventId] = useState('')

  const projects = useQuery({ queryKey: ['projects', 'options', visibility], queryFn: () => api.listProjects(visibility) })
  const applicationQueries = useQueries({
    queries: projectIds.map(projectId => ({
      queryKey: ['applications', projectId],
      queryFn: () => api.listApplications(projectId),
    })),
  })
  const applications = useMemo(
    () => uniqueById(applicationQueries.flatMap(query => query.data ?? [])),
    [applicationQueries],
  )
  const selectedApplications = useMemo(
    () => applications.filter(application => applicationIds.includes(application.id)),
    [applicationIds, applications],
  )
  const deploymentTargetQueries = useQueries({
    queries: selectedApplications.map(application => ({
      queryKey: ['deployment-targets', application.projectId, application.id],
      queryFn: () => api.listDeploymentTargets(application.projectId, application.id),
      ...liveObservationQueryPolicy,
    })),
  })
  const deploymentTargets = useMemo(
    () => uniqueById(deploymentTargetQueries.flatMap(query => query.data ?? [])),
    [deploymentTargetQueries],
  )
  const catalog = useQuery({ queryKey: ['platform-event-catalog'], queryFn: api.listPlatformEventCatalog })
  const events = useQuery({
    queryKey: ['platform-events', page, pageSize, search, visibility, projectIds, applicationIds, deploymentTargetIds, categories, eventTypes, severities, statuses, dateFrom, dateTo],
    queryFn: () => api.listPlatformEvents({
      page,
      pageSize,
      search: search || undefined,
      sortBy: 'occurredAt',
      sortOrder: 'desc',
      visibility,
      projectIds,
      applicationIds,
      deploymentTargetIds,
      categories,
      types: eventTypes,
      severities,
      statuses,
      dateFrom: dateFrom || undefined,
      dateTo: dateTo || undefined,
    }),
  })
  const selectedEvent = useQuery({
    queryKey: ['platform-event', selectedEventId],
    queryFn: () => api.getPlatformEvent(selectedEventId),
    enabled: Boolean(selectedEventId),
  })

  const categoryValues = useMemo(() => [...new Set((catalog.data ?? []).map(item => item.category))], [catalog.data])
  const projectOptions = useMemo<SearchSelectOption[]>(() => (projects.data ?? []).map(project => ({
    description: project.identifier,
    keywords: project.description,
    label: project.name,
    value: project.id,
  })), [projects.data])
  const applicationOptions = useMemo<SearchSelectOption[]>(() => applications.map(application => ({
    description: application.identifier,
    label: application.name,
    value: application.id,
  })), [applications])
  const deploymentTargetOptions = useMemo<SearchSelectOption[]>(() => deploymentTargets.map(target => ({
    description: target.stage,
    label: target.name,
    value: target.id,
  })), [deploymentTargets])
  const categoryOptions = useMemo<SearchSelectOption[]>(() => categoryValues.map(value => ({
    label: t(`eventsPage.categories.${value}`, { defaultValue: value }),
    value,
  })), [categoryValues, t])
  const eventTypeOptions = useMemo<SearchSelectOption[]>(() => (catalog.data ?? [])
    .filter(item => categories.length === 0 || categories.includes(item.category))
    .map(item => ({
      description: t(`eventsPage.categories.${item.category}`, { defaultValue: item.category }),
      label: eventTypeLabel(t, item.type),
      value: item.type,
    })), [catalog.data, categories, t])
  const severityOptions = useMemo<SearchSelectOption[]>(() => ['info', 'warning', 'error'].map(value => ({
    label: t(`eventsPage.severities.${value}`),
    value,
  })), [t])
  const statusOptions = useMemo<SearchSelectOption[]>(() => ['in_progress', 'succeeded', 'failed', 'canceled'].map(value => ({
    label: t(`eventsPage.statuses.${value}`),
    value,
  })), [t])
  const resetPage = () => setPage(1)
  const columns = useMemo<DataListColumn<PlatformEvent>[]>(() => [
    {
      key: 'event',
      header: t('eventsPage.columns.event'),
      width: 'primary',
      render: event => (
        <div className="flex min-w-0 items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-control bg-surface-subtle text-muted-foreground">
            {categoryIcon(event.category)}
          </div>
          <div className="min-w-0">
            <p className="truncate font-medium">{eventTypeLabel(t, event.type)}</p>
            <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{event.message || t('eventsPage.noMessage')}</p>
            <div className="mt-2 flex flex-wrap items-center gap-2 @xl/data-list:hidden">
              <StatusValueBadge labelKeyPrefix="eventsPage.severities" value={event.severity} />
              <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={event.status} />
              <span className="text-xs text-muted-foreground">{formatSmartDateTime(event.occurredAt, t)}</span>
            </div>
          </div>
        </div>
      ),
    },
    {
      key: 'resource',
      header: t('eventsPage.columns.resource'),
      mobile: 'hidden',
      width: 'normal',
      render: event => <EventResource event={event} />,
    },
    {
      key: 'severity',
      header: t('eventsPage.columns.severity'),
      mobile: 'hidden',
      width: 'status',
      render: event => <StatusValueBadge labelKeyPrefix="eventsPage.severities" value={event.severity} />,
    },
    {
      key: 'status',
      header: t('eventsPage.columns.status'),
      mobile: 'hidden',
      width: 'status',
      render: event => <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={event.status} />,
    },
    {
      key: 'occurredAt',
      header: t('eventsPage.columns.time'),
      mobile: 'hidden',
      width: 'compact',
      render: event => <span className="whitespace-nowrap text-sm text-muted-foreground">{formatSmartDateTime(event.occurredAt, t)}</span>,
    },
  ], [t])
  const advancedFilterCount = projectIds.length + applicationIds.length + deploymentTargetIds.length + eventTypes.length
  const activeFilterCount = advancedFilterCount + categories.length + severities.length + statuses.length + (visibility === 'all' ? 1 : 0)
  const filtered = Boolean(search) || activeFilterCount > 0
  const clearFilters = () => {
    setVisibility('related')
    setProjectIds([])
    setApplicationIds([])
    setDeploymentTargetIds([])
    setCategories([])
    setEventTypes([])
    setSeverities([])
    setStatuses([])
    setDateFrom('')
    setDateTo('')
    resetPage()
  }
  const advancedFilterFields = (
    <div className="grid gap-3 sm:grid-cols-2">
      <EventFilterMultiSelect
        label={t('eventsPage.filters.project')}
        loading={projects.isLoading}
        options={projectOptions}
        placeholder={t('eventsPage.filters.allProjects')}
        value={projectIds}
        onChange={(values) => {
          const retainedApplicationIds = applicationIds.filter((applicationId) => {
            const application = applications.find(item => item.id === applicationId)
            return application && values.includes(application.projectId)
          })
          const retainedTargetIds = deploymentTargetIds.filter((targetId) => {
            const target = deploymentTargets.find(item => item.id === targetId)
            return target && retainedApplicationIds.includes(target.applicationId)
          })
          setProjectIds(values)
          setApplicationIds(retainedApplicationIds)
          setDeploymentTargetIds(retainedTargetIds)
          resetPage()
        }}
      />
      <EventFilterMultiSelect
        disabled={projectIds.length === 0}
        label={t('eventsPage.filters.application')}
        loading={applicationQueries.some(query => query.isLoading)}
        options={applicationOptions}
        placeholder={t('eventsPage.filters.allApplications')}
        value={applicationIds}
        onChange={(values) => {
          const retainedTargetIds = deploymentTargetIds.filter((targetId) => {
            const target = deploymentTargets.find(item => item.id === targetId)
            return target && values.includes(target.applicationId)
          })
          setApplicationIds(values)
          setDeploymentTargetIds(retainedTargetIds)
          resetPage()
        }}
      />
      <EventFilterMultiSelect
        disabled={applicationIds.length === 0}
        label={t('eventsPage.filters.deploymentTarget')}
        loading={deploymentTargetQueries.some(query => query.isLoading)}
        options={deploymentTargetOptions}
        placeholder={t('eventsPage.filters.allDeploymentTargets')}
        value={deploymentTargetIds}
        onChange={(values) => {
          setDeploymentTargetIds(values)
          resetPage()
        }}
      />
      <EventFilterMultiSelect
        label={t('eventsPage.filters.type')}
        options={eventTypeOptions}
        placeholder={t('eventsPage.filters.allTypes')}
        value={eventTypes}
        onChange={(values) => {
          setEventTypes(values)
          resetPage()
        }}
      />
      <label className="grid gap-1.5 text-xs text-muted-foreground">
        {t('eventsPage.filters.dateFrom')}
        <Input
          className="h-8"
          max={dateTo}
          type="date"
          value={dateFrom}
          onChange={(event) => {
            setDateFrom(event.target.value)
            resetPage()
          }}
        />
      </label>
      <label className="grid gap-1.5 text-xs text-muted-foreground">
        {t('eventsPage.filters.dateTo')}
        <Input
          className="h-8"
          min={dateFrom}
          type="date"
          value={dateTo}
          onChange={(event) => {
            setDateTo(event.target.value)
            resetPage()
          }}
        />
      </label>
    </div>
  )

  if (events.isError) {
    return (
      <ErrorState
        description={t('eventsPage.loadFailedDescription')}
        title={t('eventsPage.loadFailedTitle')}
      />
    )
  }

  return (
    <>
      <DataList
        columns={columns}
        emptyActions={filtered
          ? (
              <Button
                variant="outline"
                onClick={() => {
                  setSearch('')
                  clearFilters()
                }}
              >
                {t('eventsPage.filters.clearAll')}
              </Button>
            )
          : undefined}
        emptyDescription={t(filtered ? 'eventsPage.filteredEmptyDescription' : 'eventsPage.emptyDescription')}
        emptyIcon={filtered ? <SearchX className="size-5" /> : <Activity className="size-5" />}
        emptyMode={filtered ? 'filtered' : 'actionable'}
        emptyTitle={t(filtered ? 'eventsPage.filteredEmptyTitle' : 'eventsPage.emptyTitle')}
        items={events.data?.items ?? []}
        loading={events.isLoading}
        rowActionLabel={() => t('eventsPage.viewDetails')}
        viewportOffset={24}
        pagination={{
          page: events.data?.page ?? page,
          pageSize,
          total: events.data?.total ?? 0,
          totalPages: events.data?.totalPages ?? 0,
          pageInfoLabel: t('pagination.pageInfo', { page: events.data?.page ?? page, totalPages: events.data?.totalPages ?? 0, total: events.data?.total ?? 0 }),
          onPageChange: setPage,
          onPageSizeChange: (value) => {
            setPageSize(value)
            setPage(1)
          },
        }}
        rowKey={event => event.id}
        search={{
          value: search,
          placeholder: t('eventsPage.searchPlaceholder'),
          onChange: (value) => {
            setSearch(value)
            resetPage()
          },
        }}
        toolbar={(
          <>
            <ResultVisibilitySelect
              canViewAll={platformAdmin}
              containerClassName="w-full @lg/data-list:w-36"
              value={visibility}
              onChange={(nextVisibility) => {
                setVisibility(nextVisibility)
                setProjectIds([])
                setApplicationIds([])
                setDeploymentTargetIds([])
                resetPage()
              }}
            />
            <SearchMultiSelect
              ariaLabel={t('eventsPage.filters.category')}
              className="w-full @lg/data-list:w-36"
              options={categoryOptions}
              placeholder={t('eventsPage.filters.allCategories')}
              size="sm"
              value={categories}
              onValueChange={(values) => {
                const allowedTypes = new Set((catalog.data ?? [])
                  .filter(item => values.length === 0 || values.includes(item.category))
                  .map(item => item.type))
                setCategories(values)
                setEventTypes(current => current.filter(value => allowedTypes.has(value)))
                resetPage()
              }}
            />
            <SearchMultiSelect
              ariaLabel={t('eventsPage.filters.severity')}
              className="w-full @lg/data-list:w-36"
              options={severityOptions}
              placeholder={t('eventsPage.filters.allSeverities')}
              size="sm"
              value={severities}
              onValueChange={(values) => {
                setSeverities(values)
                resetPage()
              }}
            />
            <SearchMultiSelect
              ariaLabel={t('eventsPage.filters.status')}
              className="w-full @lg/data-list:w-36"
              options={statusOptions}
              placeholder={t('eventsPage.filters.allStatuses')}
              size="sm"
              value={statuses}
              onValueChange={(values) => {
                setStatuses(values)
                resetPage()
              }}
            />
            <Popover>
              <PopoverTrigger asChild>
                <Button className="gap-1.5" size="sm" variant="outline">
                  <SlidersHorizontal />
                  {t('eventsPage.filters.more')}
                  {advancedFilterCount > 0 && <StatusBadge tone="info">{advancedFilterCount}</StatusBadge>}
                </Button>
              </PopoverTrigger>
              <PopoverContent align="end" className="grid w-[min(36rem,calc(100vw-2rem))] gap-4">
                <p className="text-xs text-muted-foreground">{t('eventsPage.filters.selectedCount', { count: activeFilterCount })}</p>
                {advancedFilterFields}
              </PopoverContent>
            </Popover>
            {activeFilterCount > 0 && (
              <Button size="sm" variant="ghost" onClick={clearFilters}>
                <X />
                {t('eventsPage.filters.clearAll')}
              </Button>
            )}
            <Button
              aria-label={t('common.refresh')}
              className="size-8"
              disabled={events.isFetching}
              size="icon"
              variant="ghost"
              onClick={() => events.refetch()}
            >
              <RefreshCw className={events.isFetching ? 'animate-spin' : undefined} />
            </Button>
          </>
        )}
        onRowClick={event => setSelectedEventId(event.id)}
      />

      <EventDetailSheet
        event={selectedEvent.data}
        loading={selectedEvent.isLoading}
        open={Boolean(selectedEventId)}
        onOpenChange={open => !open && setSelectedEventId('')}
      />
    </>
  )
}

function EventFilterMultiSelect({ disabled, label, loading, options, placeholder, value, onChange }: {
  disabled?: boolean
  label: string
  loading?: boolean
  options: SearchSelectOption[]
  placeholder: string
  value: string[]
  onChange: (value: string[]) => void
}) {
  return (
    <label className="grid gap-1.5 text-xs text-muted-foreground">
      {label}
      <SearchMultiSelect
        disabled={disabled}
        loading={loading}
        options={options}
        placeholder={placeholder}
        size="sm"
        value={value}
        onValueChange={onChange}
      />
    </label>
  )
}

function EventResource({ event }: { event: PlatformEvent }) {
  const { t } = useTranslation()
  const detail = event.detail ?? {}
  const primary = detail.application?.name || detail.project?.name || event.resourceId
  const secondary = detail.deploymentTarget?.name || detail.project?.name || event.resourceType
  return (
    <div className="min-w-0">
      <p className="truncate text-sm">{primary || t('eventsPage.platformResource')}</p>
      <p className="mt-1 truncate text-xs text-muted-foreground">{secondary || event.resourceType || t('eventsPage.platformResource')}</p>
    </div>
  )
}

function EventDetailSheet({ event, loading, onOpenChange, open }: { event?: PlatformEvent, loading: boolean, onOpenChange: (open: boolean) => void, open: boolean }) {
  const { t } = useTranslation()
  const detail = event?.detail ?? {}
  const links = event?.links ?? {}
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader className="border-b border-border pr-10">
          <SheetTitle>{event ? eventTypeLabel(t, event.type) : t('eventsPage.detailsTitle')}</SheetTitle>
          <SheetDescription>{event?.message || (loading ? t('common.loading') : t('eventsPage.noMessage'))}</SheetDescription>
        </SheetHeader>
        {event && (
          <div className="space-y-5 px-4 pb-6">
            <div className="flex flex-wrap gap-2">
              <StatusValueBadge labelKeyPrefix="eventsPage.severities" value={event.severity} />
              <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={event.status} />
            </div>
            <DetailSection title={t('eventsPage.details.context')}>
              <DetailRow label={t('eventsPage.details.time')} value={formatAbsoluteDateTime(event.occurredAt)} />
              <DetailRow label={t('eventsPage.details.project')} value={detail.project?.name} />
              <DetailRow label={t('eventsPage.details.application')} value={detail.application?.name} />
              <DetailRow label={t('eventsPage.details.deploymentTarget')} value={detail.deploymentTarget?.name} />
              <DetailRow label={t('eventsPage.details.actor')} value={detail.actor?.name || detail.actor?.email} />
            </DetailSection>
            <DetailSection title={t('eventsPage.details.identifiers')}>
              <DetailRow label={t('eventsPage.details.eventId')} value={event.id} mono />
              <DetailRow label={t('eventsPage.details.resource')} value={[event.resourceType, event.resourceId].filter(Boolean).join(' / ')} mono />
              <DetailRow label={t('eventsPage.details.correlationId')} value={event.correlationId} mono />
              <DetailRow label={t('eventsPage.details.notificationDeliveries')} value={String(event.deliveryCount)} />
            </DetailSection>
            <EventSpecificDetails detail={detail} />
            {Object.keys(links).length > 0 && (
              <DetailSection title={t('eventsPage.details.links')}>
                <div className="flex flex-wrap gap-2">
                  {Object.entries(links).map(([key, href]) => (
                    <Button key={key} asChild size="sm" variant="outline">
                      <a href={href}>
                        {t(`eventsPage.linkNames.${key}`, { defaultValue: key })}
                        <ExternalLink />
                      </a>
                    </Button>
                  ))}
                </div>
              </DetailSection>
            )}
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}

function EventSpecificDetails({ detail }: { detail: PlatformEventSnapshot }) {
  const { t } = useTranslation()
  const context = detail.build || detail.release || detail.hook || detail.certificate || detail.gateway
  if (!context)
    return null
  const entries = Object.entries(context).filter(([, value]) => value !== '' && value !== null && value !== undefined)
  if (entries.length === 0)
    return null
  return (
    <DetailSection title={t('eventsPage.details.eventData')}>
      {entries.map(([key, value]) => (
        <DetailRow key={key} label={t(`eventsPage.detailFields.${key}`, { defaultValue: key })} value={formatDetailValue(value)} mono={key.toLowerCase().includes('id')} />
      ))}
    </DetailSection>
  )
}

function DetailSection({ children, title }: { children: ReactNode, title: string }) {
  return (
    <section className="space-y-3 border-t border-border pt-4">
      <h3 className="text-sm font-semibold">{title}</h3>
      {children}
    </section>
  )
}

function DetailRow({ label, mono, value }: { label: string, mono?: boolean, value?: string }) {
  if (!value)
    return null
  return (
    <div className="grid gap-1 sm:grid-cols-[9rem_minmax(0,1fr)] sm:gap-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={`min-w-0 break-words text-sm ${mono ? 'font-mono text-xs' : ''}`}>{value}</dd>
    </div>
  )
}

function eventTypeLabel(t: ReturnType<typeof useTranslation>['t'], type: string) {
  return t(`eventsPage.types.${type.replaceAll('.', '_')}`, { defaultValue: type })
}

function categoryIcon(category: string) {
  const className = 'size-4'
  if (category === 'build')
    return <Hammer className={className} />
  if (category === 'release')
    return <Rocket className={className} />
  if (category === 'hook')
    return <Workflow className={className} />
  if (category === 'gateway')
    return <Globe2 className={className} />
  if (category === 'certificate')
    return <FileKey2 className={className} />
  return <Activity className={className} />
}

function formatDetailValue(value: unknown) {
  if (typeof value === 'string')
    return value
  if (typeof value === 'number' || typeof value === 'boolean')
    return String(value)
  return JSON.stringify(value)
}

function dateDaysAgo(days: number) {
  const date = new Date()
  date.setDate(date.getDate() - days)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

function uniqueById<T extends { id: string }>(items: T[]) {
  return [...new Map(items.map(item => [item.id, item])).values()]
}
