import type { ReactNode } from 'react'
import type { Application, BuildRun, GatewayRoute, PlatformEvent, Project, ProjectMember, Release } from '@/api'
import type { ApplicationsPageHandle } from '@/pages/applications/ApplicationsPage'
import type { ProjectBuildVariableSetsPageHandle } from '@/pages/projects/ProjectBuildVariableSetsPage'
import type { ProjectHooksPageHandle } from '@/pages/projects/ProjectHooksPage'
import type { ProjectMembersPageHandle } from '@/pages/projects/ProjectMembersPage'
import type { ProjectRuntimeConfigSetsPageHandle } from '@/pages/projects/ProjectRuntimeConfigSetsPage'
import { useQuery } from '@tanstack/react-query'
import { Activity, ArrowRight, FileCode2, Globe2, KeyRound, Package, Plus, Rocket, ScrollText, UserPlus } from 'lucide-react'
import { motion } from 'motion/react'
import { lazy, Suspense, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ContentTabs } from '@/components/common/content-tabs'
import { ErrorState } from '@/components/common/error-state'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatSmartDateTime } from '@/components/common/time-format'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { TabsContent } from '@/components/ui/tabs'
import { ApplicationsPage } from '@/pages/applications/ApplicationsPage'
import { ProjectBuildVariableSetsPage } from '@/pages/projects/ProjectBuildVariableSetsPage'
import { ProjectHooksPage } from '@/pages/projects/ProjectHooksPage'
import { ProjectMembersPage } from '@/pages/projects/ProjectMembersPage'
import { ProjectRuntimeConfigSetsPage } from '@/pages/projects/ProjectRuntimeConfigSetsPage'

const ProjectTopologyPanel = lazy(() => import('@/pages/projects/project-topology-panel').then(module => ({ default: module.ProjectTopologyPanel })))

export function ProjectWorkspacePage() {
  const { t } = useTranslation()
  const { projectId = '' } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const { user } = useSession()
  const [activeTab, setActiveTab] = useState(() => searchParams.get('tab') || 'overview')
  const applicationsPageRef = useRef<ApplicationsPageHandle>(null)
  const buildVariableSetsPageRef = useRef<ProjectBuildVariableSetsPageHandle>(null)
  const hooksPageRef = useRef<ProjectHooksPageHandle>(null)
  const membersPageRef = useRef<ProjectMembersPageHandle>(null)
  const runtimeConfigSetsPageRef = useRef<ProjectRuntimeConfigSetsPageHandle>(null)
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId) })
  const applications = useQuery({ queryKey: ['applications', projectId], queryFn: () => api.listApplications(projectId), enabled: Boolean(projectId) })
  const variableSets = useQuery({ queryKey: ['build-variable-sets', projectId], queryFn: () => api.listBuildVariableSets(projectId), enabled: Boolean(projectId) })
  const runtimeConfigSets = useQuery({ queryKey: ['runtime-config-sets', projectId], queryFn: () => api.listProjectRuntimeConfigSets(projectId), enabled: Boolean(projectId) })
  const members = useQuery({ queryKey: ['project-members', projectId], queryFn: () => api.listProjectMembers(projectId), enabled: Boolean(projectId) })
  const recentBuilds = useQuery({ queryKey: ['project-overview-build-runs', projectId], queryFn: () => api.listBuildRunsPage(projectId, { page: 1, pageSize: 5, sortBy: 'createdAt', sortOrder: 'desc' }), enabled: Boolean(projectId) })
  const recentEvents = useQuery({ queryKey: ['project-overview-events', projectId], queryFn: () => api.listPlatformEvents({ page: 1, pageSize: 5, projectId, sortBy: 'occurredAt', sortOrder: 'desc' }), enabled: Boolean(projectId) })
  const releases = useQuery({ queryKey: ['project-overview-releases', projectId], queryFn: () => api.listReleases(projectId), enabled: Boolean(projectId) })
  const routes = useQuery({ queryKey: ['project-overview-gateway-routes', projectId], queryFn: () => api.listGatewayRoutes(projectId), enabled: Boolean(projectId) })
  if (project.isError)
    return <ErrorState title={t('projectSpaces.workspaceLoadFailedTitle')} description={t('projectSpaces.workspaceLoadFailedDescription')} />

  const currentProject = project.data
  const currentMember = members.data?.find(member => member.userId === user?.id)
  const canManageTopology = user?.role === 'platform_admin' || currentMember?.role === 'owner' || currentMember?.role === 'admin'
  const activeContent = (() => {
    switch (activeTab) {
      case 'apps':
        return <ApplicationsPage ref={applicationsPageRef} embedded projectId={projectId} projectName={currentProject?.name} />
      case 'build-variables':
        return <ProjectBuildVariableSetsPage ref={buildVariableSetsPageRef} projectId={projectId} />
      case 'runtime-configs':
        return <ProjectRuntimeConfigSetsPage ref={runtimeConfigSetsPageRef} projectId={projectId} />
      case 'hooks':
        return <ProjectHooksPage ref={hooksPageRef} projectId={projectId} />
      case 'members':
        return <ProjectMembersPage ref={membersPageRef} embedded projectId={projectId} />
      case 'topology':
        return (
          <Suspense fallback={<div className="grid min-h-80 place-items-center text-sm text-muted-foreground">{t('common.loading')}</div>}>
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
            events={recentEvents.data?.items ?? []}
            members={members.data ?? []}
            project={currentProject}
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

    return null
  })()

  return (
    <div className="grid gap-6">
      <ContentTabs
        tabs={[
          { value: 'overview', label: t('projectSpaces.overviewTab') },
          { value: 'apps', label: t('projectSpaces.apps') },
          { value: 'build-variables', label: t('buildsPage.variablesAndSecrets') },
          { value: 'runtime-configs', label: t('runtimeConfigSets.tab') },
          { value: 'hooks', label: t('projectHooks.tab') },
          { value: 'members', label: t('projectSpaces.members') },
          { value: 'topology', label: t('projectTopology.tab') },
        ]}
        tools={contentTools}
        value={activeTab}
        onValueChange={(value) => {
          setActiveTab(value)
          setSearchParams((current) => {
            const next = new URLSearchParams(current)
            if (value === 'overview')
              next.delete('tab')
            else
              next.set('tab', value)
            return next
          }, { replace: true })
        }}
      >
        <TabsContent value={activeTab}>
          <motion.div
            key={`${projectId}-${activeTab}`}
            animate={{ opacity: 1, y: 0 }}
            initial={{ opacity: 0, y: 6 }}
            transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
          >
            {activeContent}
          </motion.div>
        </TabsContent>
      </ContentTabs>
    </div>
  )
}

function ProjectOverviewDashboard({ applications, builds, events, members, project, releases, routes, runtimeConfigSetCount, variableSetCount }: {
  applications: Application[]
  builds: BuildRun[]
  events: PlatformEvent[]
  members: ProjectMember[]
  project?: Project
  releases: Release[]
  routes: GatewayRoute[]
  runtimeConfigSetCount: number
  variableSetCount: number
}) {
  const { t } = useTranslation()
  const succeededBuilds = builds.filter(build => build.status === 'succeeded').length
  const failedBuilds = builds.filter(build => ['failed', 'lost', 'timeout'].includes(build.status)).length
  const activeReleases = releases.filter(release => release.status === 'pending' || release.status === 'running').length
  const failedReleases = releases.filter(release => release.status === 'failed').length
  const readyRoutes = routes.filter(route => route.status === 'ready').length
  const ownerCount = members.filter(member => member.role === 'owner').length
  const latestRelease = [...releases].sort((left, right) => right.createdAt.localeCompare(left.createdAt))[0]

  return (
    <div className="grid gap-4">
      <Card className="grid gap-4 p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h2 className="truncate text-lg font-semibold">{project?.name ?? t('projectSpaces.title')}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{project?.description || t('common.noDescription')}</p>
          </div>
          <StatusBadge>{project?.namespaceStrategy === 'project' ? t('projectSpaces.namespaceProject') : project?.namespaceStrategy ?? t('projectSpaces.namespaceProject')}</StatusBadge>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <ProjectMetric icon={<Package className="size-4" />} label={t('projectSpaces.apps')} meta={t('projectSpaces.projectAppsMeta', { count: applications.length })} value={applications.length} />
          <ProjectMetric icon={<Activity className="size-4" />} label={t('projectSpaces.buildHealth')} meta={failedBuilds > 0 ? t('projectSpaces.failedBuildsMeta', { count: failedBuilds }) : t('projectSpaces.succeededBuildsMeta', { count: succeededBuilds })} value={builds.length} />
          <ProjectMetric icon={<Rocket className="size-4" />} label={t('projectSpaces.releaseHealth')} meta={failedReleases > 0 ? t('projectSpaces.failedReleasesMeta', { count: failedReleases }) : t('projectSpaces.activeReleasesMeta', { count: activeReleases })} value={releases.length} />
          <ProjectMetric icon={<Globe2 className="size-4" />} label={t('projectSpaces.accessHealth')} meta={t('projectSpaces.readyRoutesMeta', { ready: readyRoutes, total: routes.length })} value={routes.length} />
        </div>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(18rem,0.75fr)]">
        <Card className="min-w-0 p-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold">{t('projectSpaces.recentBuilds')}</h3>
              <p className="mt-1 text-xs text-muted-foreground">{t('projectSpaces.recentBuildsDescription')}</p>
            </div>
          </div>
          <div className="grid gap-2">
            {builds.length > 0
              ? builds.map(build => (
                  <div key={build.id} className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-border bg-background px-3 py-2">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium" title={build.imageRef || build.targetRepository || build.id}>
                        {build.imageRef || build.targetRepository || build.id}
                      </div>
                      <div className="mt-1 truncate text-xs text-muted-foreground">
                        {projectBuildMeta(build, applications, t)}
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={build.status} />
                      <span className="text-xs text-muted-foreground">{formatSmartDateTime(build.createdAt, t)}</span>
                    </div>
                  </div>
                ))
              : <p className="rounded-md border border-dashed border-border px-3 py-6 text-sm text-muted-foreground">{t('projectSpaces.noRecentBuilds')}</p>}
          </div>
        </Card>

        <Card className="min-w-0 p-4">
          <h3 className="text-sm font-semibold">{t('projectSpaces.projectOperations')}</h3>
          <div className="mt-3 grid gap-3">
            <ProjectBillingOwnerItem owner={project?.billingOwner} />
            <ProjectOverviewItem label={t('buildsPage.variablesAndSecrets')} value={t('projectSpaces.variableSetCount', { count: variableSetCount })} />
            <ProjectOverviewItem label={t('runtimeConfigSets.tab')} value={t('projectSpaces.runtimeConfigSetCount', { count: runtimeConfigSetCount })} />
            <ProjectOverviewItem label={t('projectSpaces.members')} value={t('projectSpaces.memberRoleMeta', { members: members.length, owners: ownerCount })} />
            <ProjectOverviewItem label={t('projectSpaces.latestRelease')} value={latestRelease ? formatSmartDateTime(latestRelease.createdAt, t) : t('projectSpaces.noRelease')} />
            <ProjectOverviewItem label={t('projectSpaces.accessHealth')} value={t('projectSpaces.readyRoutesMeta', { ready: readyRoutes, total: routes.length })} />
          </div>
        </Card>
      </div>

      <Card className="min-w-0 p-4">
        <div className="mb-3 flex items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">{t('projectSpaces.recentEvents')}</h3>
            <p className="mt-1 text-xs text-muted-foreground">{t('projectSpaces.recentEventsDescription')}</p>
          </div>
          <Link className="inline-flex h-9 shrink-0 items-center gap-2 rounded-md px-3 text-sm text-primary-text transition hover:bg-muted" to={`/events?projectId=${encodeURIComponent(project?.id ?? '')}`}>
            {t('projectSpaces.viewAllEvents')}
            <ArrowRight className="size-4" />
          </Link>
        </div>
        <div className="grid gap-2">
          {events.length > 0
            ? events.map(event => (
                <div key={event.id} className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-border bg-background px-3 py-2">
                  <div className="flex min-w-0 items-center gap-3">
                    <ScrollText className="size-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{t(`eventsPage.types.${event.type.replaceAll('.', '_')}`, { defaultValue: event.type })}</p>
                      <p className="mt-1 truncate text-xs text-muted-foreground">{event.message || t('eventsPage.noMessage')}</p>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <StatusValueBadge labelKeyPrefix="eventsPage.statuses" value={event.status} />
                    <span className="hidden text-xs text-muted-foreground sm:inline">{formatSmartDateTime(event.occurredAt, t)}</span>
                  </div>
                </div>
              ))
            : <p className="rounded-md border border-dashed border-border px-3 py-6 text-sm text-muted-foreground">{t('projectSpaces.noRecentEvents')}</p>}
        </div>
      </Card>
    </div>
  )
}

function ProjectMetric({ icon, label, meta, value }: { icon: ReactNode, label: string, meta: string, value: number }) {
  return (
    <div className="rounded-md border border-border bg-background p-3">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        {icon}
        <span>{label}</span>
      </div>
      <p className="mt-3 text-2xl font-semibold">{value}</p>
      <p className="mt-1 text-xs text-muted-foreground">{meta}</p>
    </div>
  )
}

function ProjectBillingOwnerItem({ owner }: { owner?: Project['billingOwner'] }) {
  const { t } = useTranslation()
  return (
    <div className="rounded-md border border-border bg-background px-3 py-2">
      <p className="text-xs text-muted-foreground">{t('projectSpaces.billingOwner')}</p>
      {owner
        ? (
            <div className="mt-2 flex min-w-0 items-center gap-3">
              <UserAvatar className="size-8" user={owner} />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium" title={owner.name || owner.email}>{owner.name || owner.email}</p>
                <p className="truncate text-xs text-muted-foreground" title={owner.email}>{owner.email}</p>
              </div>
            </div>
          )
        : <p className="mt-1 truncate text-sm font-medium">{t('projectSpaces.billingOwnerUnknown')}</p>}
    </div>
  )
}

function ProjectOverviewItem({ label, value }: { label: string, value: string }) {
  return (
    <div className="rounded-md border border-border bg-background px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 truncate text-sm font-medium" title={value}>{value}</p>
    </div>
  )
}

function projectBuildMeta(build: BuildRun, applications: Application[], t: ReturnType<typeof useTranslation>['t']) {
  const app = applications.find(application => application.id === build.applicationId)
  const source = build.sourceTag || build.sourceBranch || build.sourceCommit || '-'
  return [app?.name ?? t('projectSpaces.unknownApplication'), source, build.triggerType].join(' · ')
}
