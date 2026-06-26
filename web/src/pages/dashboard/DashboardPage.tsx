import type { ReactNode } from 'react'
import type { BuildRun, Project, ProjectPin } from '@/api'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, AppWindow, Boxes, CheckCircle2, Container, FolderKanban, GitBranch, Pin, Server, ShieldAlert } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { api } from '@/api'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatCompactDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

const PROJECT_AGGREGATION_LIMIT = 8
const PROJECT_SHORTCUT_LIMIT = 16
const RECENT_BUILD_LIMIT = 20

export function DashboardPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const projects = useQuery({
    queryKey: ['projects', 'dashboard', 'useCount'],
    queryFn: () => api.listProjectsPage({ page: 1, pageSize: PROJECT_SHORTCUT_LIMIT, sortBy: 'useCount', sortOrder: 'desc' }),
  })
  const projectPins = useQuery({ queryKey: ['project-pins'], queryFn: api.listProjectPins })
  const registries = useQuery({ queryKey: ['registries'], queryFn: () => api.listRegistries() })
  const clusters = useQuery({ queryKey: ['runtime-clusters'], queryFn: () => api.listRuntimeClusters() })
  const projectItems = useMemo(() => projects.data?.items ?? [], [projects.data])
  const visibleProjects = useMemo(() => projectItems.slice(0, PROJECT_AGGREGATION_LIMIT), [projectItems])
  const applicationQueries = useQueries({
    queries: visibleProjects.map(project => ({
      queryKey: ['applications', project.id],
      queryFn: () => api.listApplications(project.id),
    })),
  })
  const buildRunQueries = useQueries({
    queries: visibleProjects.map(project => ({
      queryKey: ['dashboard-build-runs', project.id],
      queryFn: () => api.listBuildRunsPage(project.id, { page: 1, pageSize: RECENT_BUILD_LIMIT, sortBy: 'createdAt', sortOrder: 'desc' }),
      refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
    })),
  })

  const summary = useMemo(() => {
    const applicationsByProject = new Map<string, number>()
    const applicationNames = new Map<string, string>()
    visibleProjects.forEach((project, index) => {
      const applications = applicationQueries[index]?.data ?? []
      applicationsByProject.set(project.id, applications.length)
      applications.forEach((application) => {
        applicationNames.set(application.id, application.name)
      })
    })
    const recentBuilds = buildRunQueries.flatMap((query, index) => {
      const project = visibleProjects[index]
      return (query.data?.items ?? []).map(run => ({ project, run }))
    }).sort((left, right) => new Date(right.run.createdAt).getTime() - new Date(left.run.createdAt).getTime()).slice(0, RECENT_BUILD_LIMIT)
    const activeBuilds = recentBuilds.filter(item => item.run.status === 'queued' || item.run.status === 'running').length
    const failedBuilds = recentBuilds.filter(item => buildRunNeedsAttention(item.run.status)).length
    return {
      activeBuilds,
      applicationNames,
      applicationsByProject,
      failedBuilds,
      recentBuilds,
      totalApplications: Array.from(applicationsByProject.values()).reduce((sum, count) => sum + count, 0),
    }
  }, [applicationQueries, buildRunQueries, visibleProjects])

  const healthyClusters = clusters.data?.filter(cluster => cluster.status === 'ready' || cluster.status === 'connected').length ?? 0
  const issueCount = summary.failedBuilds + Math.max(0, (clusters.data?.length ?? 0) - healthyClusters)
  const projectShortcuts = useMemo(() => buildProjectShortcuts(projectPins.data ?? [], projectItems), [projectPins.data, projectItems])
  const pinnedProjectIds = useMemo(() => new Set((projectPins.data ?? []).map(project => project.id)), [projectPins.data])
  const projectTotal = projects.data?.total ?? projectItems.length
  const hasMoreProjects = projectTotal > projectShortcuts.length
  const toggleProjectPin = useMutation<ProjectPin | void, Error, { pinned: boolean, projectId: string }>({
    mutationFn: ({ pinned, projectId }: { pinned: boolean, projectId: string }) => pinned ? api.unpinProject(projectId) : api.pinProject(projectId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      queryClient.invalidateQueries({ queryKey: ['project-pins'] })
    },
  })
  return (
    <div className="grid min-w-0 gap-4">
      <Card className="min-w-0 max-w-full p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <SectionTitle icon={<FolderKanban size={18} />} title={t('dashboardPage.projectShortcuts')} description={t('dashboardPage.projectShortcutsDescription')} />
          {hasMoreProjects && (
            <Link className="text-sm font-medium text-muted-foreground transition hover:text-primary" to="/projects">
              {t('dashboardPage.viewAllProjects')}
            </Link>
          )}
        </div>
        {projectShortcuts.length
          ? (
              <div className="mt-4 min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-2">
                <div className="inline-flex min-w-max gap-3">
                  {projectShortcuts.map(project => (
                    <ProjectShortcutCard
                      key={project.id}
                      appCount={summary.applicationsByProject.get(project.id) ?? 0}
                      isPinPending={toggleProjectPin.isPending}
                      latestBuild={summary.recentBuilds.find(item => item.project.id === project.id)?.run}
                      pinned={pinnedProjectIds.has(project.id)}
                      project={project}
                      onTogglePin={(projectId, pinned) => toggleProjectPin.mutate({ pinned, projectId })}
                    />
                  ))}
                </div>
              </div>
            )
          : <p className="py-4 text-sm text-muted-foreground">{projects.isLoading || projectPins.isLoading ? t('common.loading') : t('projectSpaces.emptyTitle')}</p>}
      </Card>

      <Card className="overflow-hidden">
        <div className="grid gap-5 p-5 xl:grid-cols-[minmax(0,1fr)_340px]">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <StatusBadge tone={issueCount ? 'warning' : 'success'}>{issueCount ? t('dashboardPage.needsAttention') : t('dashboardPage.healthy')}</StatusBadge>
              <StatusBadge>{t('dashboardPage.projectSample', { count: visibleProjects.length })}</StatusBadge>
            </div>
            <h2 className="mt-3 text-2xl font-semibold tracking-normal">{t('dashboardPage.heading')}</h2>
            <p className="mt-1 max-w-3xl text-sm text-muted-foreground">{t('dashboardPage.subtitle')}</p>
            <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <DashboardMetric icon={<FolderKanban size={18} />} label={t('dashboardPage.projects')} to="/projects" value={projectTotal} />
              <DashboardMetric icon={<AppWindow size={18} />} label={t('dashboardPage.applications')} to="/projects" value={summary.totalApplications} />
              <DashboardMetric icon={<Activity size={18} />} label={t('dashboardPage.activeBuilds')} to={activeBuildTarget(summary.recentBuilds)} value={summary.activeBuilds} />
              <DashboardMetric icon={<Server size={18} />} label={t('dashboardPage.healthyClusters')} to="/clusters" value={`${healthyClusters}/${clusters.data?.length ?? 0}`} />
            </div>
          </div>
          <div className="rounded-md border border-border bg-muted/30 p-4">
            <div className="flex items-center gap-2 text-sm font-medium">
              <ShieldAlert size={16} />
              {t('dashboardPage.attention')}
            </div>
            <div className="mt-3 grid gap-2">
              {summary.failedBuilds > 0 && <LinkedStatusBadge to={failedBuildTarget(summary.recentBuilds)} tone="danger">{t('dashboardPage.failedBuilds', { count: summary.failedBuilds })}</LinkedStatusBadge>}
              {(clusters.data?.length ?? 0) > healthyClusters && <LinkedStatusBadge to="/clusters" tone="warning">{t('dashboardPage.unhealthyClusters', { count: (clusters.data?.length ?? 0) - healthyClusters })}</LinkedStatusBadge>}
              {issueCount === 0 && (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <CheckCircle2 size={16} className="text-emerald-600" />
                  {t('dashboardPage.noIssues')}
                </div>
              )}
            </div>
          </div>
        </div>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <Card className="p-5">
          <SectionTitle icon={<GitBranch size={18} />} title={t('dashboardPage.recentBuilds')} description={t('dashboardPage.recentBuildsDescription')} />
          <div className="mt-4 h-[17.5rem] overflow-y-auto pr-1">
            {summary.recentBuilds.length
              ? (
                  <div className="divide-y divide-border">
                    {summary.recentBuilds.map(item => (
                      <RecentBuildRow key={item.run.id} applicationName={summary.applicationNames.get(item.run.applicationId)} project={item.project} run={item.run} />
                    ))}
                  </div>
                )
              : <p className="py-4 text-sm text-muted-foreground">{projects.isLoading ? t('common.loading') : t('dashboardPage.noBuilds')}</p>}
          </div>
        </Card>

        <Card className="p-5">
          <SectionTitle icon={<Boxes size={18} />} title={t('dashboardPage.platformReadiness')} description={t('dashboardPage.platformReadinessDescription')} />
          <div className="mt-4 grid gap-3">
            <ReadinessRow icon={<Container size={16} />} label={t('registries')} value={registries.data?.length ?? 0} detail={t('dashboardPage.availableGlobalAndScoped')} to="/registries" tone={(registries.data?.length ?? 0) ? 'success' : 'warning'} />
            <ReadinessRow icon={<Server size={16} />} label={t('clusters')} value={clusters.data?.length ?? 0} detail={t('dashboardPage.availableGlobalAndScoped')} to="/clusters" tone={(clusters.data?.length ?? 0) ? 'success' : 'warning'} />
          </div>
        </Card>
      </div>

    </div>
  )
}

function ProjectShortcutCard({ appCount, isPinPending, latestBuild, onTogglePin, pinned, project }: { appCount: number, isPinPending: boolean, latestBuild?: BuildRun, onTogglePin: (projectId: string, pinned: boolean) => void, pinned: boolean, project: Project }) {
  const { t } = useTranslation()
  return (
    <Link
      className="group relative grid min-h-32 w-64 flex-none gap-3 rounded-md border border-border bg-background p-3 transition-all duration-150 hover:border-primary/50 hover:bg-muted/35 hover:text-primary"
      to={`/projects/${project.id}`}
    >
      <div className="min-w-0">
        <span className="block truncate pr-9 font-medium">{project.name}</span>
        <p className="mt-1 line-clamp-2 text-sm text-muted-foreground transition-colors group-hover:text-primary/80">{project.description || t('common.noDescription')}</p>
      </div>
      <Button
        aria-label={pinned ? t('common.unpinProject') : t('common.pinProject')}
        className={`absolute right-2 top-2 size-8 transition-opacity ${pinned ? 'text-primary opacity-100 hover:text-primary' : 'opacity-0 group-hover:opacity-100 focus-visible:opacity-100'}`}
        disabled={isPinPending}
        size="icon"
        type="button"
        variant="ghost"
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          onTogglePin(project.id, pinned)
        }}
      >
        <Pin className={`size-4 ${pinned ? 'fill-current' : ''}`} />
      </Button>
      <div className="flex flex-wrap items-center gap-2 self-end">
        <StatusBadge>{t('dashboardPage.appsCount', { count: appCount })}</StatusBadge>
        {latestBuild ? <StatusValueBadge value={latestBuild.status} /> : <StatusBadge tone="neutral">{t('dashboardPage.noBuild')}</StatusBadge>}
        <span className="text-xs text-muted-foreground transition-colors group-hover:text-primary/80">{latestBuild ? formatCompactDateTime(latestBuild.createdAt) : t('common.none')}</span>
      </div>
    </Link>
  )
}

function DashboardMetric({ icon, label, to, value }: { icon: ReactNode, label: string, to?: string, value: number | string }) {
  const content = (
    <>
      <div className="flex items-center gap-2 text-sm text-muted-foreground transition-colors group-hover:text-primary">
        {icon}
        <span>{label}</span>
      </div>
      <p className="mt-2 text-2xl font-semibold">{value}</p>
    </>
  )
  const className = 'group rounded-md border border-border bg-background p-3 transition hover:border-primary/50 hover:text-primary'
  return to
    ? <Link className={className} to={to}>{content}</Link>
    : <div className={className}>{content}</div>
}

function SectionTitle({ description, icon, title }: { description: string, icon: ReactNode, title: string }) {
  return (
    <div className="flex items-start gap-2">
      <div className="mt-0.5 text-muted-foreground">{icon}</div>
      <div className="min-w-0">
        <h3 className="text-base font-semibold">{title}</h3>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
    </div>
  )
}

function ReadinessRow({ detail, icon, label, to, tone, value }: { detail: string, icon: ReactNode, label: string, to?: string, tone: 'success' | 'warning', value: number | string }) {
  const content = (
    <>
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-sm font-medium">
          {icon}
          <span className="truncate">{label}</span>
        </div>
        <p className="mt-0.5 truncate text-xs text-muted-foreground transition-colors group-hover:text-primary/80">{detail}</p>
      </div>
      <StatusBadge tone={tone}>{value}</StatusBadge>
    </>
  )
  const className = 'group flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2.5 transition hover:border-primary/50 hover:text-primary'
  return to
    ? <Link className={className} to={to}>{content}</Link>
    : <div className={className}>{content}</div>
}

function RecentBuildRow({ applicationName, project, run }: { applicationName?: string, project: Project, run: BuildRun }) {
  const { t } = useTranslation()
  const displayName = applicationName || run.applicationId
  return (
    <Link className="group grid gap-2 py-3 transition-colors first:pt-0 hover:text-primary sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center" to={buildRunTarget(project.id, run.applicationId)}>
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-medium">{project.name}</span>
          <span className="text-muted-foreground">·</span>
          <span className="truncate font-medium">{displayName}</span>
          <StatusValueBadge value={run.status} />
        </div>
        <p className="mt-1 truncate text-sm text-muted-foreground transition-colors group-hover:text-primary/80">
          {t('dashboardPage.buildMeta', { branch: run.sourceBranch || run.sourceTag || t('common.unknown'), id: shortId(run.id), image: run.imageRef || run.targetImageRef || t('common.none') })}
        </p>
      </div>
      <span className="text-xs text-muted-foreground transition-colors group-hover:text-primary/80">{formatCompactDateTime(run.createdAt)}</span>
    </Link>
  )
}

function LinkedStatusBadge({ children, to, tone }: { children: ReactNode, to: string, tone: 'danger' | 'warning' }) {
  return (
    <Link className="w-fit rounded-md transition hover:text-primary" to={to}>
      <StatusBadge className="transition hover:border-primary/50 hover:text-primary" tone={tone}>{children}</StatusBadge>
    </Link>
  )
}

function activeBuildTarget(items: Array<{ project: Project, run: BuildRun }>) {
  const active = items.find(item => item.run.status === 'queued' || item.run.status === 'running')
  return active ? buildRunTarget(active.project.id, active.run.applicationId) : '/projects'
}

function failedBuildTarget(items: Array<{ project: Project, run: BuildRun }>) {
  const failed = items.find(item => buildRunNeedsAttention(item.run.status))
  return failed ? buildRunTarget(failed.project.id, failed.run.applicationId) : '/projects'
}

function buildRunNeedsAttention(status: BuildRun['status']) {
  return status === 'failed' || status === 'lost' || status === 'timeout'
}

function buildRunTarget(projectId: string, applicationId: string) {
  if (!applicationId)
    return `/projects/${projectId}`
  return `/projects/${projectId}/apps/${applicationId}#tab=builds`
}

function shortId(id: string) {
  return id.replace(/^bldr?_?/, '').slice(0, 8)
}

function buildProjectShortcuts(pinnedProjects: ProjectPin[], projects: Project[]) {
  const result: Project[] = []
  const seen = new Set<string>()
  const add = (project: Project | undefined) => {
    if (!project || seen.has(project.id) || result.length >= PROJECT_SHORTCUT_LIMIT)
      return
    seen.add(project.id)
    result.push(project)
  }
  const compareUsage = (left: Project, right: Project) => {
    const useCountDiff = (right.useCount ?? 0) - (left.useCount ?? 0)
    if (useCountDiff !== 0)
      return useCountDiff
    return new Date(right.lastUsedAt ?? right.createdAt).getTime() - new Date(left.lastUsedAt ?? left.createdAt).getTime()
  }
  ;[...pinnedProjects].sort(compareUsage).forEach(project => add(project))
  ;[...projects].sort(compareUsage).forEach(project => add(project))
  return result
}
