import type { Application, DashboardActivity, DashboardAttentionItem, DashboardOverview, DashboardProjectShortcut, DashboardReadinessItem, ResultVisibility } from '@/api'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { AppWindow, LayoutTemplate, Pin, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ApplicationIcon } from '@/components/common/application-icon-picker'
import { DataList } from '@/components/common/data-list'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { OverviewSkeleton } from '@/components/common/loading-states'
import { PageChromeHeading, PageChromeTools } from '@/components/common/page-chrome'
import { PageShell } from '@/components/common/page-shell'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { StatusDot } from '@/components/common/status-dot'
import { isInProgressStatus, statusToneFor } from '@/components/common/status-tone'
import { formatCompactDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { shortImageRef } from '@/lib/image-ref'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { statusRefetchInterval } from '@/lib/polling'
import { withResultVisibility } from '@/lib/result-visibility'
import { isPlatformAdmin } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { effectiveReplicaState, replicaStateTone } from '@/pages/applications/deployments/operations/application-deployment-runtime-utils'
import { AccessUrlLink } from '@/pages/applications/gateway/access-url-link'
import { httpAccessUrl } from '@/pages/applications/gateway/gateway-route-url'

const dashboardVisibility: ResultVisibility = 'related'
// 首页只为最常用的项目空间读取应用实时状态，避免一次扇出过多 Kubernetes 观测请求。
const RECENT_APP_PROJECT_LIMIT = 6
const RECENT_APP_PAGE_SIZE = 20
const RECENT_APP_LIMIT = 10
const ATTENTION_LIMIT = 4

interface RecentApplicationRow {
  application: Application
  latestActivity?: DashboardActivity
  project: DashboardProjectShortcut
}

export function DashboardPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const dashboard = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['dashboard', dashboardVisibility],
    queryFn: () => api.getDashboard(dashboardVisibility),
    refetchInterval: query => statusRefetchInterval(Boolean(
      query.state.data
      && (query.state.data.summary.activeBuilds > 0 || query.state.data.summary.activeReleases > 0),
    )),
  })
  const recentProjects = (dashboard.data?.projects ?? []).slice(0, RECENT_APP_PROJECT_LIMIT)
  const applicationQueries = useQueries({
    queries: recentProjects.map(project => ({
      ...liveObservationQueryPolicy,
      queryKey: ['dashboard-applications', project.id, RECENT_APP_PAGE_SIZE],
      queryFn: () => api.listApplicationsPage(project.id, { includeRuntime: true, page: 1, pageSize: RECENT_APP_PAGE_SIZE, sortBy: 'createdAt', sortOrder: 'desc' }),
    })),
  })

  if (dashboard.isError) {
    return (
      <ErrorState
        description={t('dashboardPage.loadFailedDescription')}
        title={t('dashboardPage.loadFailedTitle')}
      />
    )
  }

  if (!dashboard.data) {
    return <PageShell width="full"><OverviewSkeleton /></PageShell>
  }

  const overview = dashboard.data
  const activeTasks = overview.summary.activeBuilds + overview.summary.activeReleases
  const recentApplications = recentApplicationRows(overview, recentProjects, applicationQueries.map(query => query.data?.items ?? []))
  const applicationsLoading = applicationQueries.some(query => query.isLoading)
  const applicationsFailed = applicationQueries.some(query => query.isError)
  const summary = [
    t('dashboardPage.resourceTotals', { applications: overview.summary.applications, projects: overview.summary.projects }),
    activeTasks > 0 ? t('dashboardPage.activeTasksTotal', { count: activeTasks }) : '',
  ].filter(Boolean).join(' · ')

  return (
    <PageShell width="full">
      <PageChromeHeading description={summary} title={t('dashboard')} />
      <PageChromeTools>
        <Link className={buttonVariants()} to="/app-templates">
          <LayoutTemplate size={16} />
          {t('dashboardPage.startFromTemplate')}
        </Link>
      </PageChromeTools>

      {overview.attention.length > 0 && <AttentionPanel items={overview.attention} visibility={dashboardVisibility} />}

      <div className="grid min-w-0 items-start gap-4 @4xl/workspace:grid-cols-[minmax(0,2fr)_minmax(20rem,1fr)]">
        <RecentApplications failed={applicationsFailed} loading={applicationsLoading} rows={recentApplications} visibility={dashboardVisibility} />
        <div className="grid min-w-0 content-start gap-section">
          <RecentActivity activities={overview.activities} visibility={dashboardVisibility} />
          <ProjectShortcuts hasMore={overview.summary.projects > overview.projects.length} projects={overview.projects} visibility={dashboardVisibility} />
          {isPlatformAdmin(user?.role) && <PlatformReadiness overview={overview} visibility={dashboardVisibility} />}
        </div>
      </div>
    </PageShell>
  )
}

function AttentionPanel({ items, visibility }: { items: DashboardAttentionItem[], visibility: ResultVisibility }) {
  const { t } = useTranslation()
  return (
    <Section
      className="border-danger-border"
      title={t('dashboardPage.attention')}
      tools={items.length > ATTENTION_LIMIT && (
        <Link className="text-sm text-primary-text hover:underline" to={withResultVisibility('/events?severities=error&severities=warning', visibility)}>
          {t('dashboardPage.moreAttention', { count: items.length - ATTENTION_LIMIT })}
        </Link>
      )}
      variant="bordered"
    >
      <ul className="-my-3 divide-y divide-border" data-slot="dashboard-attention">
        {items.slice(0, ATTENTION_LIMIT).map(item => (
          <li key={item.key} className="flex min-w-0 flex-col gap-2 py-3 @lg/workspace:flex-row @lg/workspace:items-center @lg/workspace:justify-between">
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                <StatusDot label={eventTypeLabel(t, item.latest.type)} tone={item.severity === 'error' || item.latest.status === 'failed' ? 'danger' : 'warning'} />
                {item.occurrences > 1 && <span className="text-xs text-muted-foreground">{t('dashboardPage.occurrences', { count: item.occurrences })}</span>}
              </div>
              <p className="mt-0.5 truncate pl-3.5 text-xs text-muted-foreground">
                {[activityContext(item.latest), item.latest.message, formatCompactDateTime(item.latest.occurredAt)].filter(Boolean).join(' · ')}
              </p>
            </div>
            <Link className={cn(buttonVariants({ size: 'sm', variant: 'outline' }), 'shrink-0 self-start @lg/workspace:self-center')} to={withResultVisibility(activityTarget(item.latest), visibility)}>
              {attentionActionLabel(t, item.category)}
            </Link>
          </li>
        ))}
      </ul>
    </Section>
  )
}

function RecentApplications({ failed, loading, rows, visibility }: { failed: boolean, loading: boolean, rows: RecentApplicationRow[], visibility: ResultVisibility }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const detailPath = (row: RecentApplicationRow) => `/projects/${row.project.id}/apps/${row.application.id}`
  return (
    <Section className="content-start" title={t('dashboardPage.recentApps')} variant="bordered">
      <DataList
        framed={false}
        columns={[
          {
            key: 'application',
            header: t('dashboardPage.applicationColumn'),
            width: 'primary',
            render: row => (
              <div className="flex min-w-0 items-center gap-3">
                <span className="flex size-8 shrink-0 items-center justify-center rounded-control bg-muted text-muted-foreground">
                  <ApplicationIcon name={row.application.icon} />
                </span>
                <div className="min-w-0">
                  <Link className="block truncate rounded-sm font-medium hover:text-primary-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" to={detailPath(row)}>
                    {row.application.name}
                  </Link>
                  <span className="block truncate text-xs text-muted-foreground">{row.project.name}</span>
                </div>
              </div>
            ),
          },
          { key: 'status', header: t('dashboardPage.statusColumn'), width: 'status', render: row => <DashboardRuntimeStatus application={row.application} /> },
          { key: 'accessUrl', header: t('dashboardPage.accessUrlColumn'), width: 'normal', mobile: 'hidden', render: row => (
            // 访问地址与当前版本放在同一格的两行，避免两栏布局下表格被撑出横向滚动
            <div className="grid min-w-0 gap-0.5">
              <DashboardAccessUrl application={row.application} />
              <DashboardCurrentVersion application={row.application} />
            </div>
          ) },
          {
            key: 'activity',
            header: t('dashboardPage.activityColumn'),
            width: 'normal',
            mobile: 'hidden',
            render: row => row.latestActivity
              ? (
                  <div className="grid min-w-0 gap-0.5">
                    <span className="truncate text-sm">{eventTypeLabel(t, row.latestActivity.type)}</span>
                    <span className="text-xs text-muted-foreground">{formatCompactDateTime(row.latestActivity.occurredAt)}</span>
                  </div>
                )
              : <span className="text-sm text-muted-foreground">-</span>,
          },
        ]}
        emptyActions={(
          <Link className={buttonVariants({ variant: 'outline' })} to={withResultVisibility('/projects', visibility)}>
            {t('dashboardPage.viewAllProjects')}
          </Link>
        )}
        emptyDescription={t('dashboardPage.noRecentAppsDescription')}
        emptyIcon={<AppWindow className="size-5" />}
        emptyTitle={t('dashboardPage.noRecentApps')}
        items={rows}
        loading={loading && rows.length === 0}
        rowActionLabel={row => t('dashboardPage.openApplication', { name: row.application.name })}
        rowKey={row => row.application.id}
        onRowClick={row => navigate(detailPath(row))}
      />
      {failed && <p className="text-xs text-muted-foreground">{t('dashboardPage.appsPartiallyUnavailable')}</p>}
    </Section>
  )
}

function DashboardRuntimeStatus({ application }: { application: Application }) {
  const { t } = useTranslation()
  const summary = application.deploymentSummary
  if (!summary)
    return <span className="text-sm text-muted-foreground">-</span>
  // 后端已统一判定是否部署（status 为 not-deployed），这里只展示。
  const state = effectiveReplicaState({
    desiredReplicas: summary.desiredReplicas,
    readyReplicas: summary.readyReplicas,
    status: summary.status,
  })
  return (
    <StatusDot
      detail={state.showReplicas ? `${summary.readyReplicas}/${summary.desiredReplicas}` : undefined}
      label={t(`dashboardPage.runtimeStatuses.${state.value}`, { defaultValue: state.value })}
      pulse={isInProgressStatus(state.value)}
      tone={replicaStateTone(state.value)}
    />
  )
}

function DashboardAccessUrl({ application }: { application: Application }) {
  const url = httpAccessUrl(application.primaryAccessUrl?.trim())
  if (!url)
    return <span className="text-sm text-muted-foreground">-</span>
  return <AccessUrlLink url={url} />
}

function DashboardCurrentVersion({ application }: { application: Application }) {
  const { t } = useTranslation()
  const release = application.currentRelease
  if (!release)
    return null
  return (
    <span
      className="truncate font-mono text-xs text-muted-foreground"
      title={`${release.imageRef} · ${t('dashboardPage.versionReleasedAt', { time: formatCompactDateTime(release.deployedAt) })}`}
    >
      {shortImageRef(release.imageRef)}
    </span>
  )
}

function RecentActivity({ activities, visibility }: { activities: DashboardActivity[], visibility: ResultVisibility }) {
  const { t } = useTranslation()
  return (
    <Section
      title={t('dashboardPage.recentActivity')}
      tools={(
        <Link className="text-sm text-primary-text hover:underline" to={withResultVisibility('/events', visibility)}>
          {t('dashboardPage.viewAllEvents')}
        </Link>
      )}
      variant="bordered"
    >
      {activities.length
        ? (
            <ul className="-my-2 divide-y divide-border">
              {activities.slice(0, 8).map(activity => (
                <li key={activity.id}>
                  <Link className="group flex min-w-0 items-start justify-between gap-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" to={withResultVisibility(activityTarget(activity), visibility)}>
                    <div className="min-w-0">
                      <StatusDot label={eventTypeLabel(t, activity.type)} pulse={isInProgressStatus(activity.status)} tone={statusToneFor(activity.status)} />
                      <p className="truncate pl-3.5 text-xs text-muted-foreground group-hover:text-foreground">
                        {activityContext(activity) || activity.message || t('eventsPage.noMessage')}
                      </p>
                    </div>
                    <span className="shrink-0 text-xs text-muted-foreground">{formatCompactDateTime(activity.occurredAt)}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )
        : (
            <EmptyState
              description={t('dashboardPage.noActivityDescription')}
              title={t('dashboardPage.noActivity')}
              variant="plain"
            />
          )}
    </Section>
  )
}

function ProjectShortcuts({ hasMore, projects, visibility }: { hasMore: boolean, projects: DashboardProjectShortcut[], visibility: ResultVisibility }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const toggleProjectPin = useMutation<void, Error, { pinned: boolean, projectId: string }>({
    mutationFn: async ({ pinned, projectId }) => {
      if (pinned)
        await api.unpinProject(projectId)
      else
        await api.pinProject(projectId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      queryClient.invalidateQueries({ queryKey: ['project-pins'] })
    },
  })
  return (
    <Section
      title={t('dashboardPage.projectShortcuts')}
      tools={hasMore && (
        <Link className="text-sm text-primary-text hover:underline" to={withResultVisibility('/projects', visibility)}>
          {t('dashboardPage.viewAllProjects')}
        </Link>
      )}
      variant="bordered"
    >
      {projects.length
        ? (
            <ul className="-my-2 divide-y divide-border">
              {projects.slice(0, 6).map(project => (
                <li key={project.id} className="flex min-w-0 items-center justify-between gap-2 py-1.5">
                  <Link className="min-w-0 flex-1 rounded-sm py-0.5 hover:text-primary-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" to={withResultVisibility(`/projects/${project.id}`, visibility)}>
                    <span className="block truncate text-sm font-medium">{project.name}</span>
                    <span className="block text-xs text-muted-foreground">{t('dashboardPage.appsCount', { count: project.applicationCount })}</span>
                  </Link>
                  <Button
                    aria-label={project.pinned ? t('common.unpinProject') : t('common.pinProject')}
                    aria-pressed={project.pinned}
                    className={project.pinned ? 'text-primary-text' : 'text-muted-foreground'}
                    disabled={toggleProjectPin.isPending}
                    size="icon"
                    type="button"
                    variant="ghost"
                    onClick={() => toggleProjectPin.mutate({ pinned: project.pinned, projectId: project.id })}
                  >
                    <Pin className={cn('size-4', project.pinned && 'fill-current')} />
                  </Button>
                </li>
              ))}
            </ul>
          )
        : (
            <EmptyState
              actions={(
                <Link className={buttonVariants({ variant: 'outline' })} to="/projects?create=1">
                  <Plus className="size-4" />
                  {t('projectSpaces.createTitle')}
                </Link>
              )}
              description={t('dashboardPage.noProjectsDescription')}
              title={t('projectSpaces.emptyTitle')}
              variant="plain"
            />
          )}
    </Section>
  )
}

function PlatformReadiness({ overview, visibility }: { overview: DashboardOverview, visibility: ResultVisibility }) {
  const { t } = useTranslation()
  return (
    <Section description={t('dashboardPage.platformReadinessDescription')} title={t('dashboardPage.platformReadiness')} variant="bordered">
      <dl className="grid gap-2">
        <ReadinessRow item={overview.readiness.registries} kind="registries" label={t('registries')} to={withResultVisibility('/registries', visibility)} />
        <ReadinessRow item={overview.readiness.clusters} kind="clusters" label={t('clusters')} to="/clusters" />
      </dl>
    </Section>
  )
}

function ReadinessRow({ item, kind, label, to }: { item: DashboardReadinessItem, kind: 'clusters' | 'registries', label: string, to: string }) {
  const { t } = useTranslation()
  const value = kind === 'clusters' ? `${item.available}/${item.total}` : item.total
  return (
    <div className="flex items-center justify-between gap-3">
      <dt>
        <Link className="text-sm hover:text-primary-text hover:underline" to={to}>{label}</Link>
      </dt>
      <dd className="flex shrink-0 items-center gap-2">
        <StatusValueBadge labelKeyPrefix="dashboardPage.readinessStatuses" value={item.status} />
        <span className="text-sm tabular-nums text-muted-foreground" title={t('dashboardPage.availableCount')}>{value}</span>
      </dd>
    </div>
  )
}

/**
 * 首页应用列表：来自最常用项目空间的应用，按后端返回的各应用最近活动时间排序；没有活动的应用按项目空间顺序排在后面。
 */
function recentApplicationRows(overview: DashboardOverview, projects: DashboardProjectShortcut[], applicationPages: Application[][]): RecentApplicationRow[] {
  const latestActivityByApplication = new Map<string, DashboardActivity>()
  for (const activity of overview.applicationActivities) {
    const applicationId = activity.application?.id
    if (applicationId && !latestActivityByApplication.has(applicationId))
      latestActivityByApplication.set(applicationId, activity)
  }
  const rows = projects.flatMap((project, index) => (applicationPages[index] ?? [])
    .filter(application => application.deleteStatus !== 'deleting' && application.deleteStatus !== 'deleted')
    .map(application => ({ application, latestActivity: latestActivityByApplication.get(application.id), project })))
  return rows
    .map((row, order) => ({ order, row }))
    .sort((left, right) => {
      const leftTime = left.row.latestActivity?.occurredAt ?? ''
      const rightTime = right.row.latestActivity?.occurredAt ?? ''
      if (leftTime !== rightTime)
        return rightTime.localeCompare(leftTime)
      return left.order - right.order
    })
    .slice(0, RECENT_APP_LIMIT)
    .map(item => item.row)
}

function eventTypeLabel(t: ReturnType<typeof useTranslation>['t'], type: string) {
  return t(`eventsPage.types.${type.replaceAll('.', '_')}`, { defaultValue: type })
}

function attentionActionLabel(t: ReturnType<typeof useTranslation>['t'], category: string) {
  if (category === 'build')
    return t('dashboardPage.viewBuild')
  if (category === 'release' || category === 'hook')
    return t('dashboardPage.viewDeployment')
  if (category === 'gateway' || category === 'certificate')
    return t('dashboardPage.viewAccess')
  return t('dashboardPage.viewDetails')
}

function activityContext(activity: DashboardActivity) {
  return [activity.project?.name, activity.application?.name, activity.deploymentTarget?.name].filter(Boolean).join(' / ')
}

function activityTarget(activity: DashboardActivity) {
  const primary = activity.links.primary
  if (primary?.startsWith('/'))
    return primary
  if (activity.project && activity.application) {
    const tab = activity.category === 'build' ? 'builds' : activity.category === 'gateway' || activity.category === 'certificate' ? 'gateway' : 'deployments'
    return `/projects/${activity.project.id}/apps/${activity.application.id}?tab=${tab}`
  }
  if (activity.project)
    return `/projects/${activity.project.id}`
  return '/events'
}
