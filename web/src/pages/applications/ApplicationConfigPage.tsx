import type { ApplicationGatewayPanelHandle } from './gateway/application-gateway-panel'
import type { Application } from '@/api'
import type { BuildsPanelHandle } from '@/pages/applications/builds/application-builds-panel'
import type { DeploymentsPanelHandle } from '@/pages/applications/deployments/application-deployments-panel'
import type { RepositoryBindingsPageHandle } from '@/pages/repositories/RepositoryBindingsPage'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Globe2, Play, Plus, Rocket, Save, Upload } from 'lucide-react'
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ApplicationBasicFields } from '@/components/common/application-basic-fields'
import { ContentTabs } from '@/components/common/content-tabs'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { PageShell } from '@/components/common/page-shell'
import { Section } from '@/components/common/section'
import { Button } from '@/components/ui/button'
import { TabsContent } from '@/components/ui/tabs'
import { APPLICATION_IDENTIFIER_MAX_LENGTH, APPLICATION_IDENTIFIER_MIN_LENGTH } from '@/lib/identifier-limits'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { statusRefetchInterval, WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { firstReleaseReadyTarget } from './application-config-utils'
import { ApplicationFactStrip } from './overview/application-fact-strip'
import { ApplicationOverviewPanel } from './overview/application-overview-panel'

const RepositoryBindingsPage = lazy(() =>
  import('@/pages/repositories/RepositoryBindingsPage').then(module => ({ default: module.RepositoryBindingsPage })),
)
const ApplicationBuildsPanel = lazy(() =>
  import('@/pages/applications/builds/application-builds-panel').then(module => ({ default: module.ApplicationBuildsPanel })),
)
const ApplicationDeploymentsPanel = lazy(() =>
  import('@/pages/applications/deployments/application-deployments-panel').then(module => ({ default: module.ApplicationDeploymentsPanel })),
)
const ApplicationGatewayPanel = lazy(() =>
  import('./gateway/application-gateway-panel').then(module => ({ default: module.ApplicationGatewayPanel })),
)
const ApplicationTopologyPanel = lazy(() =>
  import('@/pages/applications/runtime/application-topology-panel').then(module => ({ default: module.ApplicationTopologyPanel })),
)

const schema = z.object({
  name: z.string().min(1, i18next.t('apps.nameRequired')),
  identifier: z.string()
    .min(APPLICATION_IDENTIFIER_MIN_LENGTH, i18next.t('apps.identifierLength', { min: APPLICATION_IDENTIFIER_MIN_LENGTH, max: APPLICATION_IDENTIFIER_MAX_LENGTH }))
    .max(APPLICATION_IDENTIFIER_MAX_LENGTH, i18next.t('apps.identifierLength', { min: APPLICATION_IDENTIFIER_MIN_LENGTH, max: APPLICATION_IDENTIFIER_MAX_LENGTH }))
    .regex(/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/, i18next.t('common.identifierFormat')),
  icon: z.string().default('box'),
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>
const APPLICATION_CONFIG_FORM_ID = 'application-config-form'
const applicationTabs = ['overview', 'deployments', 'builds', 'gateway', 'topology', 'settings'] as const
type ApplicationTab = typeof applicationTabs[number]

/** 读取 URL 中的 Tab；旧的“仓库”Tab 已并入设置。 */
function normalizeApplicationTab(value: string | null): ApplicationTab {
  if (value === 'repositories')
    return 'settings'
  return applicationTabs.find(tab => tab === value) ?? 'overview'
}

/**
 * 惰性 Tab 面板的命令句柄。面板在切换 Tab 后才挂载，此时句柄尚不存在；
 * 先暂存动作，待面板挂载、ref 回调拿到句柄后立即执行，避免依赖定时器猜测挂载时机。
 */
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

export function ApplicationConfigPage() {
  const { t } = useTranslation()
  const { projectId = '', applicationId = '' } = useParams()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState<ApplicationTab>(() => normalizeApplicationTab(searchParams.get('tab')))
  const shouldPollWorkflowStatus = activeTab === 'builds' || activeTab === 'deployments'
  // 事实条在所有 Tab 显示，部署配置、发布、构建和访问入口在每个 Tab 都需要。
  const needsRepositoryBindings = activeTab === 'overview' || activeTab === 'builds' || activeTab === 'deployments'
  const [attachBuildsPanel, runWithBuildsPanel] = useDeferredPanelHandle<BuildsPanelHandle>()
  const [attachDeploymentsPanel, runWithDeploymentsPanel] = useDeferredPanelHandle<DeploymentsPanelHandle>()
  const [attachGatewayPanel, runWithGatewayPanel] = useDeferredPanelHandle<ApplicationGatewayPanelHandle>()
  const [attachRepositoryBindings, runWithRepositoryBindings] = useDeferredPanelHandle<RepositoryBindingsPageHandle>()
  const application = useQuery({
    queryKey: ['application', projectId, applicationId],
    queryFn: () => api.getApplication(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
  })
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId && (activeTab === 'builds' || activeTab === 'deployments' || activeTab === 'gateway')) })
  const repositoryBindings = useQuery({ ...liveObservationQueryPolicy, queryKey: ['repository-bindings', projectId, applicationId], queryFn: () => api.listRepositoryBindings(projectId, applicationId), enabled: Boolean(projectId && applicationId && needsRepositoryBindings) })
  const registries = useQuery({ ...liveObservationQueryPolicy, queryKey: ['registries', projectId], queryFn: () => api.listRegistries(projectId), enabled: Boolean(projectId && (activeTab === 'builds' || activeTab === 'deployments')) })
  const buildRuns = useQuery({
    queryKey: ['build-runs', projectId, applicationId],
    queryFn: () => api.listBuildRuns(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
    refetchInterval: query => shouldPollWorkflowStatus
      ? statusRefetchInterval((query.state.data ?? []).some(run => run.status === 'queued' || run.status === 'running'))
      : false,
  })
  const buildJobs = useQuery({
    queryKey: ['build-jobs', projectId, applicationId],
    queryFn: () => api.listBuildJobs(projectId, undefined, applicationId),
    enabled: Boolean(projectId && applicationId && activeTab === 'builds'),
    refetchInterval: query => activeTab === 'builds'
      ? statusRefetchInterval((query.state.data ?? []).some(job => job.status === 'queued' || job.status === 'running'))
      : false,
  })
  const releases = useQuery({
    queryKey: ['releases', projectId, applicationId],
    queryFn: () => api.listReleases(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
    refetchInterval: query => activeTab === 'deployments'
      ? statusRefetchInterval((query.state.data ?? []).some(release => release.status === 'pending' || release.status === 'running'))
      : false,
  })
  const deploymentTargets = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-targets', projectId, applicationId],
    queryFn: () => api.listDeploymentTargets(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
    refetchInterval: activeTab === 'deployments'
      ? statusRefetchInterval((releases.data ?? []).some(release => release.status === 'pending' || release.status === 'running'))
      : false,
  })
  const routes = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['gateway-routes', projectId, applicationId],
    queryFn: () => api.listGatewayRoutes(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
    refetchInterval: query => activeTab === 'gateway' && (query.state.data ?? []).some(route => route.certificateStatus === 'pending')
      ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS
      : false,
  })
  const deploymentTargetRows = deploymentTargets.data ?? []

  const binding = useMemo(() => (repositoryBindings.data ?? []).find(item => item.applicationId === applicationId), [applicationId, repositoryBindings.data])
  const appRepositoryBindings = useMemo(() => (repositoryBindings.data ?? []).filter(item => item.applicationId === applicationId), [applicationId, repositoryBindings.data])
  const appBuildRuns = useMemo(() => (buildRuns.data ?? []).filter(run => run.applicationId === applicationId), [applicationId, buildRuns.data])
  const releaseReadyTarget = firstReleaseReadyTarget(deploymentTargetRows, appBuildRuns)
  const appBuildRunIds = useMemo(() => new Set(appBuildRuns.map(run => run.id)), [appBuildRuns])
  const appBuildJobs = useMemo(() => (buildJobs.data ?? []).filter(job => appBuildRunIds.has(job.buildRunId)), [appBuildRunIds, buildJobs.data])
  const appReleases = useMemo(() => (releases.data ?? []).filter(release => release.applicationId === applicationId), [applicationId, releases.data])
  const appRoutes = useMemo(() => (routes.data ?? []).filter(route => route.applicationId === applicationId), [applicationId, routes.data])

  const updateForm = useForm<ApplicationFormInput, undefined, ApplicationForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { icon: 'box', name: '', identifier: '' },
  })

  useEffect(() => {
    if (!application.data)
      return
    updateForm.reset({
      name: application.data.name,
      identifier: application.data.identifier,
      icon: application.data.icon ?? 'box',
    })
  }, [application.data, updateForm])

  useEffect(() => {
    if (!projectId || !shouldPollWorkflowStatus)
      return
    queryClient.invalidateQueries({ queryKey: ['build-runs', projectId, applicationId] })
    queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId, applicationId] })
    if (activeTab === 'deployments')
      queryClient.invalidateQueries({ queryKey: ['releases', projectId, applicationId] })
  }, [activeTab, applicationId, projectId, queryClient, shouldPollWorkflowStatus])

  const updateApplication = useMutation({
    mutationFn: (payload: ApplicationForm) => api.updateApplication(projectId, applicationId, {
      name: payload.name,
      identifier: payload.identifier,
      icon: payload.icon,
    }),
    onSuccess: (result) => {
      toast.success(t('apps.configSaved'))
      queryClient.setQueryData(['application', projectId, applicationId], result)
      queryClient.setQueryData(['applications', projectId], (items?: Application[]) => (items ?? []).map(item => item.id === result.id ? result : item))
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const changeTab = (value: ApplicationTab) => {
    setActiveTab(value)
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      if (value === 'overview')
        next.delete('tab')
      else
        next.set('tab', value)
      return next
    }, { replace: true })
  }
  const openRepositoryBindingTask = () => {
    changeTab('settings')
    runWithRepositoryBindings(handle => handle.openCreateDialog())
  }
  const openDeploymentTargetTask = () => navigate(`/projects/${projectId}/apps/${applicationId}/deployments/new`)
  const openBuildTask = () => {
    changeTab('builds')
    runWithBuildsPanel(handle => handle.openTriggerDrawer())
  }
  const openReleaseTask = () => {
    const target = firstReleaseReadyTarget(deploymentTargetRows, appBuildRuns)
    changeTab('deployments')
    runWithDeploymentsPanel(handle => handle.openReleaseDialog(target?.id))
  }
  const openGatewayTask = () => {
    changeTab('gateway')
    runWithGatewayPanel(handle => handle.openCreateDialog())
  }
  if (application.isError)
    return <ErrorState title={t('apps.loadFailedTitle')} description={t('apps.appLoadFailedDescription')} />

  return (
    <PageShell spacing="compact" width="full">
      <ApplicationFactStrip
        buildRuns={appBuildRuns}
        deploymentTargets={deploymentTargetRows}
        loading={deploymentTargets.isLoading || releases.isLoading || routes.isLoading || buildRuns.isLoading}
        releases={appReleases}
        routes={appRoutes}
      />
      <ContentTabs
        tabs={[
          { label: t('apps.overview'), value: 'overview' },
          { label: t('deployments'), value: 'deployments' },
          { label: t('builds'), value: 'builds' },
          { label: t('gatewayRoutes'), value: 'gateway' },
          { label: t('apps.topology.title'), value: 'topology' },
          { label: t('apps.configTab'), value: 'settings' },
        ]}
        tools={activeTab === 'settings'
          ? (
              <Button disabled={updateApplication.isPending || !updateForm.formState.isValid} form={APPLICATION_CONFIG_FORM_ID} type="submit">
                <Save size={16} />
                {t('apps.saveConfig')}
              </Button>
            )
          : (
              <>
                {activeTab === 'deployments' && (
                  <>
                    <Button variant="outline" onClick={() => runWithDeploymentsPanel(handle => handle.openImportDialog())}>
                      <Upload size={16} />
                      {t('deploymentsPage.bundleImport.open')}
                    </Button>
                    <Button variant="outline" onClick={openDeploymentTargetTask}>
                      <Plus size={16} />
                      {t('deploymentsPage.createDeploymentTarget')}
                    </Button>
                  </>
                )}
                {activeTab === 'builds' && (
                  <Button disabled={!deploymentTargetRows.some(target => target.sourceType === 'repository' && target.repositoryBindingId)} variant="outline" onClick={openBuildTask}>
                    <Play size={16} />
                    {t('buildsPage.triggerBuild')}
                  </Button>
                )}
                {activeTab === 'gateway' && (
                  <Button disabled={!deploymentTargetRows.length} variant="outline" onClick={openGatewayTask}>
                    <Globe2 size={16} />
                    {t('gatewayRoutesPage.createRoute')}
                  </Button>
                )}
                <Button disabled={!releaseReadyTarget} onClick={openReleaseTask}>
                  <Rocket size={16} />
                  {t('deploymentsPage.createRelease')}
                </Button>
              </>
            )}
        value={activeTab}
        onValueChange={value => changeTab(normalizeApplicationTab(value))}
      >
        <TabsContent value="overview">
          <ApplicationOverviewPanel
            buildRuns={appBuildRuns}
            deploymentTargets={deploymentTargetRows}
            releases={appReleases}
            repositoryBindings={appRepositoryBindings}
            routes={appRoutes}
            onBindRepository={openRepositoryBindingTask}
            onCreateDeploymentTarget={openDeploymentTargetTask}
            onCreateGatewayRoute={openGatewayTask}
            onCreateRelease={openReleaseTask}
            onTriggerBuild={openBuildTask}
          />
        </TabsContent>
        <TabsContent value="settings">
          <div className="grid max-w-5xl gap-section">
            <Section description={t('apps.configDescription')} title={t('apps.basicInfo')} variant="bordered">
              <form id={APPLICATION_CONFIG_FORM_ID} onSubmit={updateForm.handleSubmit(values => updateApplication.mutate(values))}>
                <ApplicationBasicFields
                  compact
                  icon={updateForm.watch('icon')}
                  nameError={updateForm.formState.errors.name?.message}
                  nameField={updateForm.register('name')}
                  identifierError={updateForm.formState.errors.identifier?.message}
                  identifierField={updateForm.register('identifier')}
                  identifierMaxLength={APPLICATION_IDENTIFIER_MAX_LENGTH}
                  identifierReadOnly
                  onIconChange={icon => updateForm.setValue('icon', icon, { shouldDirty: true, shouldValidate: true })}
                />
              </form>
            </Section>
            <Section
              description={t('apps.repositorySectionDescription')}
              title={t('apps.repoBinding')}
              tools={(
                <Button disabled={!projectId || !applicationId} variant="outline" onClick={() => runWithRepositoryBindings(handle => handle.openCreateDialog())}>
                  <Plus size={16} />
                  {t('repositories.addRepository')}
                </Button>
              )}
            >
              <Suspense fallback={<TabFallback />}>
                <RepositoryBindingsPage
                  ref={attachRepositoryBindings}
                  applicationId={applicationId}
                  projectId={projectId}
                />
              </Suspense>
            </Section>
          </div>
        </TabsContent>
        <TabsContent value="builds">
          <Suspense fallback={<TabFallback />}>
            <ApplicationBuildsPanel
              ref={attachBuildsPanel}
              applicationId={applicationId}
              applicationIdentifier={application.data?.identifier ?? ''}
              binding={binding}
              repositoryBindings={appRepositoryBindings}
              buildJobs={appBuildJobs}
              deploymentTargets={deploymentTargetRows}
              buildRuns={appBuildRuns}
              projectId={projectId}
              projectIdentifier={project.data?.identifier ?? ''}
              registries={registries.data ?? []}
            />
          </Suspense>
        </TabsContent>
        <TabsContent value="deployments">
          <Suspense fallback={<TabFallback />}>
            <ApplicationDeploymentsPanel
              ref={attachDeploymentsPanel}
              applicationId={applicationId}
              buildRuns={appBuildRuns}
              deploymentTargets={deploymentTargetRows}
              projectId={projectId}
              projectWebConsoleEnabled={project.data?.webConsoleEnabled ?? true}
              releases={appReleases}
              routes={appRoutes}
            />
          </Suspense>
        </TabsContent>
        <TabsContent value="gateway">
          <Suspense fallback={<TabFallback />}>
            <ApplicationGatewayPanel
              ref={attachGatewayPanel}
              applicationId={applicationId}
              applicationIdentifier={application.data?.identifier ?? ''}
              deploymentTargets={deploymentTargetRows}
              projectId={projectId}
              projectIdentifier={project.data?.identifier ?? ''}
              routes={appRoutes}
            />
          </Suspense>
        </TabsContent>
        <TabsContent value="topology">
          <Suspense fallback={<TabFallback />}>
            <ApplicationTopologyPanel applicationId={applicationId} projectId={projectId} />
          </Suspense>
        </TabsContent>
      </ContentTabs>
    </PageShell>
  )
}

function TabFallback() {
  return <ToolViewportSkeleton />
}
