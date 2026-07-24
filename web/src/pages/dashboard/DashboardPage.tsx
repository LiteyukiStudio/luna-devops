import type { ReactNode } from 'react'
import type { DashboardActivity, DashboardAttentionItem, DashboardProjectShortcut, DashboardReadinessItem } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, AppWindow, ArrowRight, Boxes, Container, FileKey2, FolderKanban, Globe2, Hammer, Pin, Rocket, ScrollText, Server, ShieldAlert, Workflow } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { OverviewSkeleton } from '@/components/common/loading-states'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { Notice } from '@/components/common/notice'
import { PageShell } from '@/components/common/page-shell'
import { Section } from '@/components/common/section'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { formatCompactDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

export function DashboardPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const dashboard = useQuery({
    queryKey: ['dashboard'],
    queryFn: api.getDashboard,
    refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
  })
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

  if (dashboard.isError) {
    return (
      <ErrorState
        description={t('dashboardPage.loadFailedDescription')}
        title={t('dashboardPage.loadFailedTitle')}
      />
    )
  }

  if (!dashboard.data) {
    return <PageShell width="content"><OverviewSkeleton /></PageShell>
  }

  const overview = dashboard.data
  const activeTasks = overview.summary.activeBuilds + overview.summary.activeReleases
  const hasMoreProjects = overview.summary.projects > overview.projects.length

  return (
    <PageShell width="content">
      {overview.attention.length > 0 && <AttentionPanel items={overview.attention} />}

      <section className="grid gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge>
              {overview.summary.attentionItems ? t('dashboardPage.needsAttention') : t('dashboardPage.healthy')}
            </StatusBadge>
            <span className="text-sm text-muted-foreground">
              {t('dashboardPage.resourceTotals', { applications: overview.summary.applications, projects: overview.summary.projects })}
            </span>
          </div>
          {activeTasks > 0 && (
            <span className="text-xs text-muted-foreground">{t('dashboardPage.activeTasksTotal', { count: activeTasks })}</span>
          )}
        </div>
        <Surface className="overflow-hidden" data-slot="dashboard-overview" variant="bordered">
          <div className="p-4 sm:p-6">
            <MetricGroup>
              <MetricItem emphasis={overview.summary.activeBuilds > 0} href="/events?categories=build&statuses=in_progress" icon={<Hammer size={18} />} label={t('dashboardPage.activeBuilds')} surface="neutral" value={overview.summary.activeBuilds} />
              <MetricItem emphasis={overview.summary.activeReleases > 0} href="/events?categories=release&statuses=in_progress" icon={<Rocket size={18} />} label={t('dashboardPage.activeReleases')} surface="neutral" value={overview.summary.activeReleases} />
              <MetricItem emphasis={overview.summary.attentionItems > 0} href="/events?severities=error&severities=warning" icon={<ShieldAlert size={18} />} label={t('dashboardPage.attentionItems')} surface="neutral" tone={overview.summary.attentionItems ? 'danger' : 'neutral'} value={overview.summary.attentionItems} />
              <MetricItem emphasis={overview.summary.totalClusters > 0} href="/clusters" icon={<Server size={18} />} label={t('dashboardPage.healthyClusters')} surface="neutral" tone={overview.summary.healthyClusters < overview.summary.totalClusters ? 'warning' : 'neutral'} value={`${overview.summary.healthyClusters}/${overview.summary.totalClusters}`} />
            </MetricGroup>
          </div>

          <div className="grid min-w-0 border-t border-border xl:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]">
            <Section
              className="min-w-0 p-5 sm:p-6"
              icon={<ScrollText size={18} />}
              title={t('dashboardPage.recentActivity')}
              tools={(
                <Link className="text-sm font-medium text-muted-foreground transition hover:text-primary-text" to="/events">
                  {t('dashboardPage.viewAllEvents')}
                </Link>
              )}
            >
              <div className={overview.activities.length > 5 ? 'max-h-80 overflow-y-auto pr-1' : ''}>
                {overview.activities.length
                  ? (
                      <div className="divide-y divide-border">
                        {overview.activities.map(activity => <ActivityRow key={activity.id} activity={activity} />)}
                      </div>
                    )
                  : (
                      <EmptyState
                        description={t('dashboardPage.noActivityDescription')}
                        icon={<Activity className="size-5" />}
                        title={t('dashboardPage.noActivity')}
                        variant="plain"
                      />
                    )}
              </div>
            </Section>

            <Section className="border-t border-border p-5 sm:p-6 xl:border-l xl:border-t-0" icon={<Boxes size={18} />} title={t('dashboardPage.platformReadiness')}>
              <div className="grid gap-3">
                <ReadinessRow icon={<Container size={16} />} item={overview.readiness.registries} kind="registries" label={t('registries')} to="/registries" />
                <ReadinessRow icon={<Server size={16} />} item={overview.readiness.clusters} kind="clusters" label={t('clusters')} to="/clusters" />
              </div>
            </Section>
          </div>
        </Surface>
      </section>

      <Section
        icon={<FolderKanban size={18} />}
        title={t('dashboardPage.projectShortcuts')}
        tools={hasMoreProjects && (
          <Link className="text-sm font-medium text-muted-foreground transition hover:text-primary-text" to="/projects">
            {t('dashboardPage.viewAllProjects')}
          </Link>
        )}
      >
        {overview.projects.length
          ? (
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                {overview.projects.slice(0, 6).map(project => (
                  <ProjectShortcutCard
                    key={project.id}
                    isPinPending={toggleProjectPin.isPending}
                    project={project}
                    onTogglePin={(projectId, pinned) => toggleProjectPin.mutate({ pinned, projectId })}
                  />
                ))}
              </div>
            )
          : (
              <EmptyState
                description={t('dashboardPage.noProjectsDescription')}
                icon={<FolderKanban className="size-5" />}
                title={t('projectSpaces.emptyTitle')}
                variant="plain"
              />
            )}
      </Section>
    </PageShell>
  )
}

function ProjectShortcutCard({ isPinPending, onTogglePin, project }: { isPinPending: boolean, onTogglePin: (projectId: string, pinned: boolean) => void, project: DashboardProjectShortcut }) {
  const { t } = useTranslation()
  return (
    <Link
      className="group relative grid min-h-36 min-w-0 gap-4 rounded-container bg-surface-raised p-4 transition-colors hover:bg-surface-subtle"
      to={`/projects/${project.id}`}
    >
      <div className="min-w-0">
        <span className="block truncate pr-9 font-medium">{project.name}</span>
        <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{project.description || t('common.noDescription')}</p>
      </div>
      <Button
        aria-label={project.pinned ? t('common.unpinProject') : t('common.pinProject')}
        className={`absolute right-2 top-2 size-8 transition-opacity ${project.pinned ? 'text-primary-text opacity-100 hover:text-primary-text' : 'opacity-0 group-hover:opacity-100 focus-visible:opacity-100'}`}
        disabled={isPinPending}
        size="icon"
        type="button"
        variant="ghost"
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          onTogglePin(project.id, project.pinned)
        }}
      >
        <Pin className={`size-4 ${project.pinned ? 'fill-current' : ''}`} />
      </Button>
      <div className="flex min-w-0 items-end justify-between gap-3 self-end">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <StatusBadge>{t('dashboardPage.appsCount', { count: project.applicationCount })}</StatusBadge>
          {project.latestActivity
            ? <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={project.latestActivity.status} />
            : <span className="text-xs text-muted-foreground">{t('dashboardPage.noActivityShort')}</span>}
          {project.latestActivity && <span className="text-xs text-muted-foreground">{formatCompactDateTime(project.latestActivity.occurredAt)}</span>}
        </div>
        <ArrowRight className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-primary-text" />
      </div>
    </Link>
  )
}

function AttentionPanel({ items }: { items: DashboardAttentionItem[] }) {
  const { t } = useTranslation()
  const tone = items.some(item => item.severity === 'error') ? 'danger' : 'warning'
  return (
    <Notice icon={<ShieldAlert size={18} />} title={t('dashboardPage.attention')} tone={tone} variant="neutral">
      <div className="flex min-w-0 flex-wrap gap-2">
        {items.slice(0, 4).map(item => (
          <Link key={item.key} className="group flex min-h-8 min-w-0 max-w-full items-center gap-2 rounded-md bg-surface-subtle px-2 py-1.5 transition-colors hover:bg-surface-inset" to={activityTarget(item.latest)}>
            <span className="shrink-0 transition-colors group-hover:text-primary-text">{categoryIcon(item.category)}</span>
            <span className="truncate text-sm text-foreground">{eventTypeLabel(t, item.latest.type)}</span>
            {item.occurrences > 1 && <StatusBadge>{t('dashboardPage.occurrences', { count: item.occurrences })}</StatusBadge>}
          </Link>
        ))}
        {items.length > 4 && <Link className="flex min-h-8 items-center px-2 text-sm font-medium text-primary-text" to="/events?severities=error&severities=warning">{t('dashboardPage.moreAttention', { count: items.length - 4 })}</Link>}
      </div>
    </Notice>
  )
}

function ActivityRow({ activity }: { activity: DashboardActivity }) {
  const { t } = useTranslation()
  return (
    <Link className="group grid gap-2 py-3 transition-colors first:pt-0 hover:text-primary-text sm:flex sm:items-center sm:justify-between" to={activityTarget(activity)}>
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          {categoryIcon(activity.category)}
        </div>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="truncate font-medium">{eventTypeLabel(t, activity.type)}</span>
            <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={activity.status} />
          </div>
          <p className="mt-1 truncate text-sm text-muted-foreground">
            {activityContext(activity) || activity.message || t('eventsPage.noMessage')}
          </p>
        </div>
      </div>
      <span className="pl-11 text-xs text-muted-foreground sm:pl-0">{formatCompactDateTime(activity.occurredAt)}</span>
    </Link>
  )
}

function ReadinessRow({ icon, item, kind, label, to }: { icon: ReactNode, item: DashboardReadinessItem, kind: 'clusters' | 'registries', label: string, to: string }) {
  const { t } = useTranslation()
  const value = kind === 'clusters' ? `${item.available}/${item.total}` : item.total
  return (
    <Link className="group flex items-center justify-between gap-3 rounded-md bg-surface-subtle px-3 py-3 transition-colors hover:bg-surface-inset" to={to}>
      <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
        <span className="text-muted-foreground">{icon}</span>
        <span className="truncate">{label}</span>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <StatusValueBadge labelKeyPrefix="dashboardPage.readinessStatuses" value={item.status} />
        <span className="text-sm tabular-nums text-muted-foreground" title={t('dashboardPage.availableCount')}>{value}</span>
      </div>
    </Link>
  )
}

function eventTypeLabel(t: ReturnType<typeof useTranslation>['t'], type: string) {
  return t(`eventsPage.types.${type.replaceAll('.', '_')}`, { defaultValue: type })
}

function activityContext(activity: DashboardActivity) {
  return [activity.project?.name, activity.application?.name, activity.deploymentTarget?.name].filter(Boolean).join(' · ')
}

function activityTarget(activity: DashboardActivity) {
  const primary = activity.links.primary
  if (primary?.startsWith('/'))
    return primary
  if (activity.project && activity.application) {
    const tab = activity.category === 'build' ? 'builds' : activity.category === 'gateway' || activity.category === 'certificate' ? 'gateway' : 'deployments'
    return `/projects/${activity.project.id}/apps/${activity.application.id}#tab=${tab}`
  }
  if (activity.project)
    return `/projects/${activity.project.id}`
  return '/events'
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
  if (category === 'application')
    return <AppWindow className={className} />
  return <Activity className={className} />
}
