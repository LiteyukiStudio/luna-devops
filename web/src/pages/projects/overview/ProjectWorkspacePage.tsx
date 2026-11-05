import type { Application, BuildRun, GatewayRoute, PlatformEvent, Project, ProjectMember, Release } from '@/api'
import type { ApplicationsPageHandle } from '@/pages/applications/ApplicationsPage'
import type { ProjectBuildVariableSetsPageHandle } from '@/pages/projects/delivery-config/ProjectBuildVariableSetsPage'
import type { ProjectHooksPageHandle } from '@/pages/projects/delivery-config/ProjectHooksPage'
import type { ProjectRuntimeConfigSetsPageHandle } from '@/pages/projects/delivery-config/ProjectRuntimeConfigSetsPage'
import type { ProjectMembersPageHandle } from '@/pages/projects/governance/ProjectMembersPage'
import type { ProjectVolumesPageHandle } from '@/pages/projects/volumes/ProjectVolumesPage'
import { useQuery } from '@tanstack/react-query'
import { Activity, ArrowRight, FileCode2, Globe2, KeyRound, Package, Plus, Rocket, ScrollText, Upload, UserPlus } from 'lucide-react'
import { motion } from 'motion/react'
import { lazy, Suspense, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { DescriptionListItem } from '@/components/common/description-list-item'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { MetricGroup, MetricItem } from '@/components/common/metric-group'
import { PageChromeHeading, PageChromeTools } from '@/components/common/page-chrome'
import { PageShell } from '@/components/common/page-shell'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { projectVolumeCapabilities } from '@/lib/project-volume-capabilities'
import { isPlatformAdmin, ProjectRole } from '@/lib/roles'
import { BillingOwnerTransferDialog } from '@/pages/projects/governance/billing-owner-transfer-dialog'
import { normalizeProjectWorkspaceSection } from './project-workspace-sections'

const ApplicationsPage = lazy(() => import('@/pages/applications/ApplicationsPage').then(module => ({ default: module.ApplicationsPage })))
const ProjectBuildVariableSetsPage = lazy(() => import('@/pages/projects/delivery-config/ProjectBuildVariableSetsPage').then(module => ({ default: module.ProjectBuildVariableSetsPage })))
const ProjectHooksPage = lazy(() => import('@/pages/projects/delivery-config/ProjectHooksPage').then(module => ({ default: module.ProjectHooksPage })))
const ProjectMembersPage = lazy(() => import('@/pages/projects/governance/ProjectMembersPage').then(module => ({ default: module.ProjectMembersPage })))
const ProjectRuntimeConfigSetsPage = lazy(() => import('@/pages/projects/delivery-config/ProjectRuntimeConfigSetsPage').then(module => ({ default: module.ProjectRuntimeConfigSetsPage })))
const ProjectTopologyPanel = lazy(() => import('@/pages/projects/topology/project-topology-panel').then(module => ({ default: module.ProjectTopologyPanel })))
const ProjectVolumesPage = lazy(() => import('@/pages/projects/volumes/ProjectVolumesPage'))

export function ProjectWorkspacePage() {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()
  const [searchParams] = useSearchParams()
  const { user } = useSession()
  // 分区由侧栏导航写入 URL，页面只读取，不维护第二份状态。
  const activeTab = normalizeProjectWorkspaceSection(searchParams.get('tab'))
  const applicationsPageRef = useRef<ApplicationsPageHandle>(null)
  const buildVariableSetsPageRef = useRef<ProjectBuildVariableSetsPageHandle>(null)
  const hooksPageRef = useRef<ProjectHooksPageHandle>(null)
  const membersPageRef = useRef<ProjectMembersPageHandle>(null)
  const runtimeConfigSetsPageRef = useRef<ProjectRuntimeConfigSetsPageHandle>(null)
  const volumesPageRef = useRef<ProjectVolumesPageHandle>(null)
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId) })
  const isOverview = activeTab === 'overview'
  const applications = useQuery({ queryKey: ['applications', projectId], queryFn: () => api.listApplications(projectId), enabled: Boolean(projectId && (isOverview || activeTab === 'topology')) })
  const variableSets = useQuery({ queryKey: ['build-variable-sets', projectId], queryFn: () => api.listBuildVariableSets(projectId), enabled: Boolean(projectId && isOverview) })
  const runtimeConfigSets = useQuery({ queryKey: ['runtime-config-sets', projectId], queryFn: () => api.listProjectRuntimeConfigSets(projectId), enabled: Boolean(projectId && isOverview) })
  const members = useQuery({ queryKey: ['project-members', projectId], queryFn: () => api.listProjectMembers(projectId), enabled: Boolean(projectId && isOverview) })
  const recentBuilds = useQuery({ queryKey: ['project-overview-build-runs', projectId], queryFn: () => api.listBuildRunsPage(projectId, { page: 1, pageSize: 5, sortBy: 'createdAt', sortOrder: 'desc' }), enabled: Boolean(projectId && isOverview) })
  const recentEvents = useQuery({ queryKey: ['project-overview-events', projectId], queryFn: () => api.listPlatformEvents({ page: 1, pageSize: 5, projectId, sortBy: 'occurredAt', sortOrder: 'desc' }), enabled: Boolean(projectId && isOverview) })
  const releases = useQuery({ queryKey: ['project-overview-releases', projectId], queryFn: () => api.listReleases(projectId), enabled: Boolean(projectId && isOverview) })
  const routes = useQuery({
    queryKey: ['project-overview-gateway-routes', projectId],
    queryFn: () => api.listGatewayRoutes(projectId),
    enabled: Boolean(projectId && isOverview),
    ...liveObservationQueryPolicy,
  })
  if (project.isError)
    return <ErrorState title={t('projectSpaces.workspaceLoadFailedTitle')} description={t('projectSpaces.workspaceLoadFailedDescription')} />

  const currentProject = project.data
  const currentProjectRole = currentProject?.currentUserRole
  const canManageTopology = isPlatformAdmin(user?.role) || currentProjectRole === ProjectRole.Owner || currentProjectRole === ProjectRole.Admin
  const volumeCapabilities = projectVolumeCapabilities(user?.role, currentProjectRole, user?.id)
  const activeContent = (() => {
    switch (activeTab) {
      case 'apps':
        return <ApplicationsPage ref={applicationsPageRef} projectId={projectId} />
      case 'build-variables':
        return <ProjectBuildVariableSetsPage ref={buildVariableSetsPageRef} projectId={projectId} />
      case 'runtime-configs':
        return <ProjectRuntimeConfigSetsPage ref={runtimeConfigSetsPageRef} projectId={projectId} />
      case 'hooks':
        return <ProjectHooksPage ref={hooksPageRef} projectId={projectId} />
      case 'members':
        return <ProjectMembersPage ref={membersPageRef} embedded projectId={projectId} />
      case 'volumes':
        return (
          <Suspense fallback={<ToolViewportSkeleton />}>
            <ProjectVolumesPage key={projectId} ref={volumesPageRef} capabilities={volumeCapabilities} projectId={projectId} />
          </Suspense>
        )
      case 'topology':
        return (
          <Suspense fallback={<ToolViewportSkeleton />}>
            <ProjectTopologyPanel
              key={projectId}
              applications={applications.data ?? []}
              canManage={Boolean(canManageTopology)}
              projectId={projectId}
            />
          </Suspense>
        )
      default:
        return (
          <ProjectOverviewDashboard
            applications={applications.data ?? []}
            builds={recentBuilds.data?.items ?? []}
            buildTotal={recentBuilds.data?.total ?? 0}
            events={recentEvents.data?.items ?? []}
            members={members.data ?? []}
            project={currentProject}
            canTransferBillingOwnership={currentProjectRole === ProjectRole.Owner && currentProject?.billingOwnerUserId === user?.id}
            releases={releases.data ?? []}
            routes={routes.data ?? []}
            runtimeConfigSetCount={runtimeConfigSets.data?.length ?? 0}
            variableSetCount={variableSets.data?.length ?? 0}
          />
        )
    }
  })()

  const contentTools = (() => {
    if (activeTab === 'apps') {
      return (
        <Button type="button" onClick={() => applicationsPageRef.current?.openCreateDialog()}>
          <Plus size={16} />
          {t('apps.createTitle')}
        </Button>
      )
    }

    if (activeTab === 'members') {
      return (
        <Button type="button" onClick={() => membersPageRef.current?.openAddMemberDialog()}>
          <UserPlus size={16} />
          {t('projectMembers.addTitle')}
        </Button>
      )
    }

    if (activeTab === 'build-variables') {
      return (
        <Button type="button" onClick={() => buildVariableSetsPageRef.current?.openCreateDialog()}>
          <KeyRound size={16} />
          {t('buildsPage.createVariableSet')}
        </Button>
      )
    }

    if (activeTab === 'runtime-configs') {
      return (
        <Button type="button" onClick={() => runtimeConfigSetsPageRef.current?.openCreateDialog()}>
          <FileCode2 size={16} />
          {t('runtimeConfigSets.createTitle')}
        </Button>
      )
    }

    if (activeTab === 'hooks') {
      return (
        <Button type="button" onClick={() => hooksPageRef.current?.openCreateDialog()}>
          <ScrollText size={16} />
          {t('projectHooks.createTitle')}
        </Button>
      )
    }

    if (activeTab === 'volumes') {
      if (!volumeCapabilities.canImport && !volumeCapabilities.canWrite)
        return null
      return (
        <div className="flex items-center gap-2">
          {volumeCapabilities.canImport && (
            <Button type="button" variant="outline" onClick={() => volumesPageRef.current?.openImportDialog()}>
              <Upload size={16} />
              {t('projectVolumes.import')}
            </Button>
          )}
          {volumeCapabilities.canWrite && (
            <Button type="button" onClick={() => volumesPageRef.current?.openCreateDialog()}>
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

function ProjectOverviewDashboard({ applications, buildTotal, builds, canTransferBillingOwnership, events, members, project, releases, routes, runtimeConfigSetCount, variableSetCount }: {
  applications: Application[]
  buildTotal: number
  builds: BuildRun[]
  canTransferBillingOwnership: boolean
  events: PlatformEvent[]
  members: ProjectMember[]
  project?: Project
  releases: Release[]
  routes: GatewayRoute[]
  runtimeConfigSetCount: number
  variableSetCount: number
}) {
  const { t } = useTranslation()
  const failedBuilds = builds.filter(build => ['failed', 'lost', 'timeout'].includes(build.status)).length
  const activeReleases = releases.filter(release => release.status === 'pending' || release.status === 'running').length
  const failedReleases = releases.filter(release => release.status === 'failed').length
  const readyRoutes = routes.filter(route => route.status === 'ready').length
  const ownerCount = members.filter(member => member.role === ProjectRole.Owner).length

  return (
    <div className="grid gap-section">
      {project && <PageChromeHeading description={project.description || undefined} title={project.name} />}
      <MetricGroup>
        <MetricItem href="?tab=apps" icon={<Package className="size-4" />} label={t('projectSpaces.apps')} value={applications.length} />
        <MetricItem
          emphasis={buildTotal > 0}
          icon={<Activity className="size-4" />}
          label={t('projectSpaces.buildsMetric')}
          meta={builds.length > 0 ? t('projectSpaces.recentBuildsFailedMeta', { count: builds.length, failed: failedBuilds }) : undefined}
          tone={failedBuilds > 0 ? 'danger' : 'neutral'}
          value={buildTotal}
        />
        <MetricItem
          emphasis={activeReleases > 0 || failedReleases > 0}
          icon={<Rocket className="size-4" />}
          label={t('projectSpaces.activeReleasesMetric')}
          meta={failedReleases > 0 ? t('projectSpaces.failedReleasesMeta', { count: failedReleases }) : undefined}
          tone={failedReleases > 0 ? 'danger' : activeReleases > 0 ? 'warning' : 'neutral'}
          value={activeReleases}
        />
        <MetricItem
          emphasis={routes.length > 0}
          icon={<Globe2 className="size-4" />}
          label={t('projectSpaces.accessHealth')}
          tone={routes.length > 0 && readyRoutes < routes.length ? 'warning' : 'neutral'}
          value={`${readyRoutes}/${routes.length}`}
        />
      </MetricGroup>

      <div className="grid gap-section xl:grid-cols-[minmax(0,1.25fr)_minmax(18rem,0.75fr)]">
        <Section description={t('projectSpaces.recentBuildsDescription')} title={t('projectSpaces.recentBuilds')} variant="bordered">
          {builds.length > 0
            ? (
                <ul className="-my-2 divide-y divide-border">
                  {builds.map(build => (
                    <li key={build.id} className="flex min-w-0 items-center justify-between gap-3 py-2">
                      <div className="min-w-0">
                        <div className="truncate font-mono text-xs text-foreground" title={build.imageRef || build.targetRepository || build.id}>
                          {build.imageRef || build.targetRepository || build.id}
                        </div>
                        <div className="mt-0.5 truncate text-xs text-muted-foreground">
                          {projectBuildMeta(build, applications, t)}
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={build.status} />
                        <span className="text-xs text-muted-foreground">{formatSmartDateTime(build.createdAt, t)}</span>
                      </div>
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
              project={project}
            />
            <DescriptionListItem label={t('buildsPage.variablesAndSecrets')} value={t('projectSpaces.variableSetCount', { count: variableSetCount })} />
            <DescriptionListItem label={t('runtimeConfigSets.tab')} value={t('projectSpaces.runtimeConfigSetCount', { count: runtimeConfigSetCount })} />
            <DescriptionListItem label={t('projectSpaces.members')} value={t('projectSpaces.memberRoleMeta', { members: members.length, owners: ownerCount })} />
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
        {events.length > 0
          ? (
              <ul className="-my-2 divide-y divide-border">
                {events.map(event => (
                  <li key={event.id} className="flex min-w-0 items-center justify-between gap-3 py-2">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{t(`eventsPage.types.${event.type.replaceAll('.', '_')}`, { defaultValue: event.type })}</p>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">{event.message || t('eventsPage.noMessage')}</p>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={event.status} />
                      <span className="hidden text-xs text-muted-foreground sm:inline">{formatSmartDateTime(event.occurredAt, t)}</span>
                    </div>
                  </li>
                ))}
              </ul>
            )
          : <EmptyState title={t('projectSpaces.noRecentEvents')} variant="plain" />}
      </Section>
    </div>
  )
}

function ProjectBillingOwnerItem({ canTransfer, members, project }: { canTransfer: boolean, members: ProjectMember[], project?: Project }) {
  const { t } = useTranslation()
  const owner = project?.billingOwner
  return (
    <div className="grid min-w-0 gap-1" data-slot="description-list-item">
      <dt className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
        {t('projectSpaces.billingOwner')}
        {canTransfer && project && <BillingOwnerTransferDialog members={members} project={project} />}
      </dt>
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

function projectBuildMeta(build: BuildRun, applications: Application[], t: ReturnType<typeof useTranslation>['t']) {
  const app = applications.find(application => application.id === build.applicationId)
  const source = build.sourceTag || build.sourceBranch || build.sourceCommit || '-'
  return [app?.name ?? t('projectSpaces.unknownApplication'), source, build.triggerType].join(' · ')
}
