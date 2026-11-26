import type { Application, BuildRun, PlatformEvent, Project, ProjectMember, ProjectOverview } from '@/api'
import type { ApplicationsPageHandle } from '@/pages/applications/ApplicationsPage'
import type { ProjectMembersPageHandle } from '@/pages/projects/governance/ProjectMembersPage'
import type { ProjectVolumesPageHandle } from '@/pages/projects/volumes/ProjectVolumesPage'
import { useQuery } from '@tanstack/react-query'
import { Activity, ArrowRight, Globe2, Package, Plus, Rocket, Upload, UserPlus } from 'lucide-react'
import { motion } from 'motion/react'
import { lazy, Suspense, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useParams, useSearchParams } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { DescriptionListItem } from '@/components/common/description-list-item'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { OverviewSkeleton, ToolViewportSkeleton } from '@/components/common/loading-states'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { PageChromeHeading, PageChromeTools } from '@/components/common/page-chrome'
import { PageShell } from '@/components/common/page-shell'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { projectVolumeCapabilities } from '@/lib/project-volume-capabilities'
import { buildRunPath, platformEventDetailPath, platformEventResourcePath, projectEventsPath } from '@/lib/resource-routes'
import { isPlatformAdmin, ProjectAction, projectActionAllowed, ProjectRole } from '@/lib/roles'
import { ProjectSharedConfigPanel } from '@/pages/projects/delivery-config/ProjectSharedConfigPanel'
import { BillingOwnerTransferDialog } from '@/pages/projects/governance/billing-owner-transfer-dialog'
import { legacyProjectWorkspaceSearch, normalizeProjectWorkspaceSection, projectSharedConfigTabParam, projectWorkspaceSectionPath } from './project-workspace-sections'

// 整行可点击：负外边距让悬停底色铺满分隔线之间，内容仍与标题对齐。
const overviewRowLinkClassName = '-mx-2 flex min-w-0 items-center justify-between gap-3 rounded-control px-2 py-2 transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'
const overviewValueLinkClassName = 'rounded-sm hover:text-primary-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

const ApplicationsPage = lazy(() => import('@/pages/applications/ApplicationsPage').then(module => ({ default: module.ApplicationsPage })))
const BillingPage = lazy(() => import('@/pages/billing/BillingPage').then(module => ({ default: module.BillingPage })))
const ProjectMembersPage = lazy(() => import('@/pages/projects/governance/ProjectMembersPage').then(module => ({ default: module.ProjectMembersPage })))
const ProjectNetworkPolicyCard = lazy(() => import('@/pages/projects/topology/project-network-policy-card').then(module => ({ default: module.ProjectNetworkPolicyCard })))
const ProjectNetworkSharesCard = lazy(() => import('@/pages/projects/topology/project-network-shares-card').then(module => ({ default: module.ProjectNetworkSharesCard })))
const ProjectNotificationsPanel = lazy(() => import('@/pages/projects/notifications/ProjectNotificationsPanel').then(module => ({ default: module.ProjectNotificationsPanel })))
const ProjectAIAutomationPanel = lazy(() => import('@/pages/projects/notifications/project-ai-automation-panel').then(module => ({ default: module.ProjectAIAutomationPanel })))
const ProjectTopologyPanel = lazy(() => import('@/pages/projects/topology/project-topology-panel').then(module => ({ default: module.ProjectTopologyPanel })))
const ProjectVolumesPage = lazy(() => import('@/pages/projects/volumes/ProjectVolumesPage'))

function useDeferredPanelHandle<T>() {
  const handleRef = useRef<T | null>(null)
  const pendingRef = useRef<((handle: T) => void) | null>(null)
  const attach = useCallback((handle: T | null) => {
    handleRef.current = handle
    const pending = pendingRef.current
    if (handle && pending) {
      pendingRef.current = null
      pending(handle)
    }
  }, [])
  const run = useCallback((action: (handle: T) => void) => {
    if (handleRef.current)
      action(handleRef.current)
    else
      pendingRef.current = action
  }, [])
  return [attach, run] as const
}

export function ProjectWorkspacePage() {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()
  const [searchParams] = useSearchParams()
  const { user } = useSession()
  // 分区由侧栏导航写入 URL，页面只读取，不维护第二份状态。
  const activeTab = normalizeProjectWorkspaceSection(searchParams.get('tab'))
  // 构建变量、运行配置和钩子的旧独立分区链接改写到“共享配置”对应 Tab。
  const legacySearch = legacyProjectWorkspaceSearch(searchParams)
  const [attachApplicationsPage, runWithApplicationsPage] = useDeferredPanelHandle<ApplicationsPageHandle>()
  const [attachMembersPage, runWithMembersPage] = useDeferredPanelHandle<ProjectMembersPageHandle>()
  const [attachVolumesPage, runWithVolumesPage] = useDeferredPanelHandle<ProjectVolumesPageHandle>()
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId) })
  const isOverview = activeTab === 'overview'
  const applications = useQuery({ queryKey: ['applications', projectId], queryFn: () => api.listApplications(projectId), enabled: Boolean(projectId && (isOverview || activeTab === 'topology')) })
  const variableSets = useQuery({ queryKey: ['build-variable-sets', projectId], queryFn: () => api.listBuildVariableSets(projectId), enabled: Boolean(projectId && isOverview) })
  const runtimeConfigSets = useQuery({ queryKey: ['runtime-config-sets', projectId], queryFn: () => api.listProjectRuntimeConfigSets(projectId), enabled: Boolean(projectId && isOverview) })
  const members = useQuery({ queryKey: ['project-members', projectId], queryFn: () => api.listProjectMembers(projectId), enabled: Boolean(projectId && isOverview) })
  const recentBuilds = useQuery({ queryKey: ['project-overview-build-runs', projectId], queryFn: () => api.listBuildRunsPage(projectId, { page: 1, pageSize: 5, sortBy: 'createdAt', sortOrder: 'desc' }), enabled: Boolean(projectId && isOverview) })
  const recentEvents = useQuery({ queryKey: ['project-overview-events', projectId], queryFn: () => api.listPlatformEvents({ page: 1, pageSize: 5, projectId, sortBy: 'occurredAt', sortOrder: 'desc' }), enabled: Boolean(projectId && isOverview) })
  // 应用、发布与访问入口就绪数由后端按项目空间统计，不在前端对截断的列表计数；访问入口是网关实时观察。
  const overview = useQuery({
    queryKey: ['project-overview', projectId],
    queryFn: () => api.getProjectOverview(projectId),
    enabled: Boolean(projectId && isOverview),
    ...liveObservationQueryPolicy,
  })
  if (legacySearch)
    return <Navigate replace to={{ search: legacySearch }} />
  if (project.isError) {
    return (
      <PageShell>
        <ErrorState
          description={t('projectSpaces.workspaceLoadFailedDescription')}
          retrying={project.isFetching}
          title={t('projectSpaces.workspaceLoadFailedTitle')}
          onRetry={() => void project.refetch()}
        />
      </PageShell>
    )
  }
  if (project.isPending) {
    return (
      <PageShell>
        {isOverview ? <OverviewSkeleton /> : <ToolViewportSkeleton />}
      </PageShell>
    )
  }

  const currentProject = project.data
  const currentUserActions = currentProject?.currentUserActions
  // 授权结果只来自项目详情的 currentUserActions，不在前端按角色推导。
  const canManageTopology = projectActionAllowed(currentUserActions, ProjectAction.ProjectManage)
  const volumeCapabilities = projectVolumeCapabilities(currentProject?.currentUserActions, user?.id, currentProject?.volumeTransferEnabled)
  const activeContent = (() => {
    switch (activeTab) {
      case 'apps':
        return <ApplicationsPage ref={attachApplicationsPage} projectId={projectId} />
      case 'shared-config':
        return <ProjectSharedConfigPanel projectId={projectId} />
      case 'members':
        return <ProjectMembersPage ref={attachMembersPage} embedded projectId={projectId} />
      case 'volumes':
        return (
          <Suspense fallback={<ToolViewportSkeleton />}>
            <ProjectVolumesPage key={projectId} ref={attachVolumesPage} capabilities={volumeCapabilities} projectId={projectId} />
          </Suspense>
        )
      case 'topology':
        return (
          <Suspense fallback={<ToolViewportSkeleton />}>
            <div className="grid gap-4">
              <ProjectNetworkPolicyCard key={`network-${projectId}`} canManage={canManageTopology} projectId={projectId} />
              <ProjectNetworkSharesCard key={`network-shares-${projectId}`} canManage={canManageTopology} projectId={projectId} />
              <ProjectTopologyPanel
                key={projectId}
                applications={applications.data ?? []}
                applicationsError={applications.isError}
                applicationsLoading={Boolean(projectId) && applications.isPending}
                applicationsRetrying={applications.isFetching}
                canManage={canManageTopology}
                projectId={projectId}
                onRetryApplications={() => void applications.refetch()}
              />
            </div>
          </Suspense>
        )
      case 'notifications':
        return (
          <Suspense fallback={<ToolViewportSkeleton />}>
            <ProjectNotificationsPanel
              key={`notifications-${projectId}`}
              currentUserActions={currentUserActions}
              projectId={projectId}
            />
          </Suspense>
        )
      case 'ai-diagnosis':
        return (
          <Suspense fallback={<ToolViewportSkeleton />}>
            <ProjectAIAutomationPanel
              key={`ai-diagnosis-${projectId}`}
              canManage={isPlatformAdmin(user?.role) || projectActionAllowed(currentUserActions, ProjectAction.NotificationManage)}
              canRead={isPlatformAdmin(user?.role) || projectActionAllowed(currentUserActions, ProjectAction.NotificationRead)}
              projectId={projectId}
            />
          </Suspense>
        )
      case 'billing':
        return <BillingPage key={`billing-${projectId}`} projectId={projectId} />
      default:
        return (
          <ProjectOverviewDashboard
            applications={applications.data ?? []}
            applicationsError={applications.isError}
            applicationsLoading={applications.isPending}
            builds={recentBuilds.data?.items ?? []}
            buildsError={recentBuilds.isError
              ? {
                  onRetry: () => void recentBuilds.refetch(),
                  retrying: recentBuilds.isFetching,
                }
              : undefined}
            buildsLoading={recentBuilds.isPending}
            buildTotal={recentBuilds.data?.total ?? 0}
            events={recentEvents.data?.items ?? []}
            eventsError={recentEvents.isError
              ? {
                  onRetry: () => void recentEvents.refetch(),
                  retrying: recentEvents.isFetching,
                }
              : undefined}
            eventsLoading={recentEvents.isPending}
            initialLoading={overview.isPending && recentBuilds.isPending && recentEvents.isPending}
            members={members.data ?? []}
            membersError={members.isError}
            membersLoading={members.isPending}
            membersRetrying={members.isFetching}
            project={currentProject}
            canTransferBillingOwnership={projectActionAllowed(currentUserActions, ProjectAction.ProjectOwnerOnly) && currentProject?.billingOwnerUserId === user?.id}
            overview={overview.data}
            overviewLoading={overview.isPending}
            overviewUnavailable={overview.isError}
            runtimeConfigSetCount={runtimeConfigSets.data?.length ?? 0}
            runtimeConfigSetsError={runtimeConfigSets.isError}
            runtimeConfigSetsLoading={runtimeConfigSets.isPending}
            variableSetCount={variableSets.data?.length ?? 0}
            variableSetsError={variableSets.isError}
            variableSetsLoading={variableSets.isPending}
            onRetryMembers={() => void members.refetch()}
          />
        )
    }
  })()

  const contentTools = (() => {
    if (activeTab === 'apps') {
      return (
        <Button type="button" onClick={() => runWithApplicationsPage(handle => handle.openCreateDialog())}>
          <Plus size={16} />
          {t('apps.createTitle')}
        </Button>
      )
    }

    if (activeTab === 'members') {
      return (
        <Button type="button" onClick={() => runWithMembersPage(handle => handle.openAddMemberDialog())}>
          <UserPlus size={16} />
          {t('projectMembers.addTitle')}
        </Button>
      )
    }

    if (activeTab === 'volumes') {
      if (!volumeCapabilities.canImport && !volumeCapabilities.canWrite)
        return null
      return (
        <div className="flex items-center gap-2">
          {volumeCapabilities.canImport && (
            <Button type="button" variant="outline" onClick={() => runWithVolumesPage(handle => handle.openImportDialog())}>
              <Upload size={16} />
              {t('projectVolumes.import')}
            </Button>
          )}
          {volumeCapabilities.canWrite && (
            <Button type="button" onClick={() => runWithVolumesPage(handle => handle.openCreateDialog())}>
              <Plus size={16} />
              {t('projectVolumes.create')}
            </Button>
          )}
        </div>
      )
    }

    return null
  })()

  return (
    <PageShell>
      {currentProject && (
        <PageChromeHeading
          description={isOverview ? (currentProject.description || undefined) : undefined}
          title={currentProject.name}
        />
      )}
      <PageChromeTools>{contentTools}</PageChromeTools>
      <motion.div
        key={`${projectId}-${activeTab}`}
        animate={{ opacity: 1, y: 0 }}
        initial={{ opacity: 0, y: 6 }}
        transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
      >
        <Suspense fallback={<ToolViewportSkeleton />}>
          {activeContent}
        </Suspense>
      </motion.div>
    </PageShell>
  )
}

interface SectionRetryState {
  onRetry: () => void
  retrying: boolean
}

function ProjectOverviewDashboard({
  applications,
  applicationsError,
  applicationsLoading,
  buildTotal,
  builds,
  buildsError,
  buildsLoading,
  canTransferBillingOwnership,
  events,
  eventsError,
  eventsLoading,
  initialLoading,
  members,
  membersError,
  membersLoading,
  membersRetrying,
  onRetryMembers,
  overview,
  overviewLoading,
  overviewUnavailable,
  project,
  runtimeConfigSetCount,
  runtimeConfigSetsError,
  runtimeConfigSetsLoading,
  variableSetCount,
  variableSetsError,
  variableSetsLoading,
}: {
  applications: Application[]
  applicationsError?: boolean
  applicationsLoading?: boolean
  buildTotal: number
  builds: BuildRun[]
  buildsError?: SectionRetryState
  buildsLoading?: boolean
  canTransferBillingOwnership: boolean
  events: PlatformEvent[]
  eventsError?: SectionRetryState
  eventsLoading?: boolean
  initialLoading?: boolean
  members: ProjectMember[]
  membersError?: boolean
  membersLoading?: boolean
  membersRetrying?: boolean
  onRetryMembers?: () => void
  overview?: ProjectOverview
  overviewLoading?: boolean
  overviewUnavailable: boolean
  project?: Project
  runtimeConfigSetCount: number
  runtimeConfigSetsError?: boolean
  runtimeConfigSetsLoading?: boolean
  variableSetCount: number
  variableSetsError?: boolean
  variableSetsLoading?: boolean
}) {
  const { t } = useTranslation()
  const failedBuilds = builds.filter(build => ['failed', 'timeout'].includes(build.status)).length
  const activeReleases = overview?.activeReleases ?? 0
  const failedReleases = overview?.failedReleases ?? 0
  const accessRoutes = overview?.accessRoutes
  // 网关不可观察或概览请求失败时如实显示不可用，不把缺失的就绪数当作 0。
  const accessRoutesUnavailable = overviewUnavailable || accessRoutes?.status === 'unavailable'
  const ownerCount = members.filter(member => member.role === ProjectRole.Owner).length

  if (initialLoading)
    return <OverviewSkeleton />

  return (
    <div className="grid gap-section">
      <MetricGroup>
        <MetricItem
          emphasis={!overviewLoading && !overviewUnavailable && (overview?.applications ?? 0) > 0}
          href="?tab=apps"
          icon={<Package className="size-4" />}
          label={t('projectSpaces.apps')}
          value={overviewLoading
            ? <Skeleton className="h-8 w-16" />
            : overviewUnavailable
              ? t('common.unavailable')
              : (overview?.applications ?? 0)}
        />
        <MetricItem
          emphasis={!buildsLoading && !buildsError && buildTotal > 0}
          href={project && projectEventsPath(project.id, 'build')}
          icon={<Activity className="size-4" />}
          label={t('projectSpaces.buildsMetric')}
          meta={!buildsLoading && !buildsError && builds.length > 0 ? t('projectSpaces.recentBuildsFailedMeta', { count: builds.length, failed: failedBuilds }) : undefined}
          tone={!buildsLoading && !buildsError && failedBuilds > 0 ? 'danger' : 'neutral'}
          value={buildsLoading
            ? <Skeleton className="h-8 w-16" />
            : buildsError
              ? t('common.unavailable')
              : buildTotal}
        />
        <MetricItem
          emphasis={!overviewLoading && !overviewUnavailable && (activeReleases > 0 || failedReleases > 0)}
          href={project && projectEventsPath(project.id, 'release')}
          icon={<Rocket className="size-4" />}
          label={t('projectSpaces.activeReleasesMetric')}
          meta={!overviewLoading && !overviewUnavailable && failedReleases > 0 ? t('projectSpaces.failedReleasesMeta', { count: failedReleases, days: overview?.failedReleaseWindowDays }) : undefined}
          tone={!overviewLoading && !overviewUnavailable ? (failedReleases > 0 ? 'danger' : activeReleases > 0 ? 'warning' : 'neutral') : 'neutral'}
          value={overviewLoading
            ? <Skeleton className="h-8 w-16" />
            : overviewUnavailable
              ? t('common.unavailable')
              : activeReleases}
        />
        <MetricItem
          emphasis={!overviewLoading && !accessRoutesUnavailable && Boolean(accessRoutes && accessRoutes.total > 0)}
          href={project && projectEventsPath(project.id, 'gateway')}
          icon={<Globe2 className="size-4" />}
          label={t('projectSpaces.accessHealth')}
          meta={!overviewLoading && accessRoutesUnavailable ? t('projectSpaces.accessHealthUnavailable') : undefined}
          tone={!overviewLoading && (accessRoutesUnavailable || accessRoutes?.status === 'degraded') ? 'warning' : 'neutral'}
          value={overviewLoading
            ? <Skeleton className="h-8 w-16" />
            : accessRoutesUnavailable
              ? t('common.unavailable')
              : accessRoutes && accessRoutes.ready !== undefined
                ? `${accessRoutes.ready}/${accessRoutes.total}`
                : t('common.unavailable')}
        />
      </MetricGroup>

      <div className="grid gap-section @4xl/workspace:grid-cols-[minmax(0,1.25fr)_minmax(18rem,0.75fr)]">
        <Section description={t('projectSpaces.recentBuildsDescription')} title={t('projectSpaces.recentBuilds')} variant="bordered">
          {buildsError
            ? (
                <ErrorState
                  description={t('projectSpaces.recentBuildsLoadFailedDescription')}
                  retrying={buildsError.retrying}
                  title={t('projectSpaces.recentBuildsLoadFailedTitle')}
                  onRetry={buildsError.onRetry}
                />
              )
            : buildsLoading
              ? (
                  <div className="grid gap-2">
                    {Array.from({ length: 3 }, (_, index) => <Skeleton key={index} className="h-11 w-full" />)}
                  </div>
                )
              : builds.length > 0
                ? (
                    <ul className="-my-2 divide-y divide-border">
                      {builds.map(build => (
                        <li key={build.id}>
                          <Link className={overviewRowLinkClassName} to={buildRunPath(build)}>
                            <div className="min-w-0">
                              <div className="truncate font-mono text-xs text-foreground" title={build.imageRef || build.targetRepository || build.id}>
                                {build.imageRef || build.targetRepository || build.id}
                              </div>
                              <div className="mt-0.5 truncate text-xs text-muted-foreground">
                                {projectBuildMeta(build, applications, Boolean(applicationsLoading), Boolean(applicationsError), t)}
                              </div>
                            </div>
                            <div className="flex shrink-0 items-center gap-2">
                              <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={build.status} />
                              <span className="text-xs text-muted-foreground">{formatSmartDateTime(build.createdAt, t)}</span>
                            </div>
                          </Link>
                        </li>
                      ))}
                    </ul>
                  )
                : <EmptyState title={t('projectSpaces.noRecentBuilds')} variant="plain" />}
        </Section>

        <Section title={t('projectSpaces.projectOperations')} variant="bordered">
          <dl className="grid gap-4">
            <ProjectBillingOwnerItem
              canTransfer={canTransferBillingOwnership}
              members={members}
              membersError={membersError}
              membersLoading={membersLoading}
              membersRetrying={membersRetrying}
              project={project}
              onRetryMembers={onRetryMembers}
            />
            <DescriptionListItem
              label={t('buildsPage.variablesAndSecrets')}
              value={variableSetsLoading
                ? <Skeleton className="h-5 w-28" />
                : variableSetsError
                  ? <span className="text-muted-foreground">{t('common.unavailable')}</span>
                  : project
                    ? <Link className={overviewValueLinkClassName} to={projectWorkspaceSectionPath(project.id, 'shared-config')}>{t('projectSpaces.variableSetCount', { count: variableSetCount })}</Link>
                    : undefined}
            />
            <DescriptionListItem
              label={t('runtimeConfigSets.tab')}
              value={runtimeConfigSetsLoading
                ? <Skeleton className="h-5 w-28" />
                : runtimeConfigSetsError
                  ? <span className="text-muted-foreground">{t('common.unavailable')}</span>
                  : project
                    ? <Link className={overviewValueLinkClassName} to={`${projectWorkspaceSectionPath(project.id, 'shared-config')}&${projectSharedConfigTabParam}=runtime-configs`}>{t('projectSpaces.runtimeConfigSetCount', { count: runtimeConfigSetCount })}</Link>
                    : undefined}
            />
            <DescriptionListItem
              label={t('projectSpaces.members')}
              value={membersLoading
                ? <Skeleton className="h-5 w-36" />
                : membersError
                  ? <span className="text-muted-foreground">{t('common.unavailable')}</span>
                  : project
                    ? <Link className={overviewValueLinkClassName} to={projectWorkspaceSectionPath(project.id, 'members')}>{t('projectSpaces.memberRoleMeta', { members: members.length, owners: ownerCount })}</Link>
                    : undefined}
            />
          </dl>
        </Section>
      </div>

      <Section
        description={t('projectSpaces.recentEventsDescription')}
        title={t('projectSpaces.recentEvents')}
        tools={(
          <Link className="inline-flex items-center gap-1 rounded-sm text-sm text-primary-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" to={`/events?projectId=${encodeURIComponent(project?.id ?? '')}`}>
            {t('projectSpaces.viewAllEvents')}
            <ArrowRight className="size-4" />
          </Link>
        )}
        variant="bordered"
      >
        {eventsError
          ? (
              <ErrorState
                description={t('projectSpaces.recentEventsLoadFailedDescription')}
                retrying={eventsError.retrying}
                title={t('projectSpaces.recentEventsLoadFailedTitle')}
                onRetry={eventsError.onRetry}
              />
            )
          : eventsLoading
            ? (
                <div className="grid gap-2">
                  {Array.from({ length: 3 }, (_, index) => <Skeleton key={index} className="h-11 w-full" />)}
                </div>
              )
            : events.length > 0
              ? (
                  <ul className="-my-2 divide-y divide-border">
                    {events.map(event => (
                      <li key={event.id}>
                        <Link className={overviewRowLinkClassName} to={platformEventResourcePath(event) ?? platformEventDetailPath(event)}>
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium">{t(`eventsPage.types.${event.type.replaceAll('.', '_')}`, { defaultValue: event.type })}</p>
                            <p className="mt-0.5 truncate text-xs text-muted-foreground">{event.message || t('eventsPage.noMessage')}</p>
                          </div>
                          <div className="flex shrink-0 items-center gap-2">
                            <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={event.status} />
                            <span className="hidden text-xs text-muted-foreground @lg/workspace:inline">{formatSmartDateTime(event.occurredAt, t)}</span>
                          </div>
                        </Link>
                      </li>
                    ))}
                  </ul>
                )
              : <EmptyState title={t('projectSpaces.noRecentEvents')} variant="plain" />}
      </Section>
    </div>
  )
}

function ProjectBillingOwnerItem({
  canTransfer,
  members,
  membersError,
  membersLoading,
  membersRetrying,
  onRetryMembers,
  project,
}: {
  canTransfer: boolean
  members: ProjectMember[]
  membersError?: boolean
  membersLoading?: boolean
  membersRetrying?: boolean
  onRetryMembers?: () => void
  project?: Project
}) {
  const { t } = useTranslation()
  const owner = project?.billingOwner
  return (
    <div className="grid min-w-0 gap-1" data-slot="description-list-item">
      <div className="flex items-center justify-between gap-3">
        <dt className="text-xs text-muted-foreground">{t('projectSpaces.billingOwner')}</dt>
        {canTransfer && project && (
          <BillingOwnerTransferDialog
            members={members}
            membersError={membersError}
            membersLoading={membersLoading}
            membersRetrying={membersRetrying}
            project={project}
            onRetryMembers={onRetryMembers}
          />
        )}
      </div>
      <dd className="min-w-0 text-sm">
        {owner
          ? (
              <div className="flex min-w-0 items-center gap-2">
                <UserAvatar className="size-6" user={owner} />
                <span className="truncate font-medium" title={owner.email}>{owner.name || owner.email}</span>
              </div>
            )
          : <span className="text-muted-foreground">{t('projectSpaces.billingOwnerUnknown')}</span>}
      </dd>
    </div>
  )
}

function localizedBuildTrigger(triggerType: BuildRun['triggerType'], t: ReturnType<typeof useTranslation>['t']) {
  switch (triggerType) {
    case 'manual':
      return t('buildsPage.events.manual')
    case 'webhook':
      return t('buildsPage.events.webhook')
    case 'push':
      return t('buildsPage.events.push')
    case 'tag':
      return t('buildsPage.events.tag')
    case 'api':
      return t('buildsPage.events.api')
    case 'retry':
      return t('buildsPage.events.retry')
    default:
      return triggerType
  }
}

function projectBuildMeta(
  build: BuildRun,
  applications: Application[],
  applicationsLoading: boolean,
  applicationsError: boolean,
  t: ReturnType<typeof useTranslation>['t'],
) {
  const app = applications.find(application => application.id === build.applicationId)
  const source = build.sourceTag || build.sourceBranch || build.sourceCommit || '-'
  const trigger = localizedBuildTrigger(build.triggerType, t)
  if (!app && applicationsLoading) {
    return (
      <span className="inline-flex min-w-0 items-center gap-1.5 align-middle">
        <Skeleton className="inline-block h-3.5 w-20 shrink-0" />
        <span className="truncate">{`· ${source} · ${trigger}`}</span>
      </span>
    )
  }
  const appName = app?.name ?? (applicationsError ? t('common.unavailable') : t('projectSpaces.unknownApplication'))
  return [appName, source, trigger].join(' · ')
}
