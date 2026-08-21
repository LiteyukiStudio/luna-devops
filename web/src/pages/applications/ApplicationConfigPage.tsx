import type { BuildsPanelHandle } from './application-builds-panel'
import type { DeploymentsPanelHandle } from './application-deployments-panel'
import type { ApplicationGatewayPanelHandle } from './application-gateway-panel'
import type { Application } from '@/api'
import type { RepositoryBindingsPageHandle } from '@/pages/repositories/RepositoryBindingsPage'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Globe2, Package, Play, Plus, Save, Upload } from 'lucide-react'
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useParams, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ApplicationBasicFields } from '@/components/common/application-basic-fields'
import { ContentTabs } from '@/components/common/content-tabs'
import { ErrorState } from '@/components/common/error-state'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { MotionItem, MotionList } from '@/components/common/motion'
import { PageShell } from '@/components/common/page-shell'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { TabsContent } from '@/components/ui/tabs'
import { APPLICATION_IDENTIFIER_MAX_LENGTH, APPLICATION_IDENTIFIER_MIN_LENGTH } from '@/lib/identifier-limits'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { statusRefetchInterval, WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { isPlatformAdmin } from '@/lib/roles'
import { firstReleaseReadyTarget } from './application-config-utils'
import { ApplicationOverviewPanel } from './application-overview-panel'

const RepositoryBindingsPage = lazy(() =>
  import('@/pages/repositories/RepositoryBindingsPage').then(module => ({ default: module.RepositoryBindingsPage })),
)
const ApplicationBuildsPanel = lazy(() =>
  import('./application-builds-panel').then(module => ({ default: module.ApplicationBuildsPanel })),
)
const ApplicationDeploymentsPanel = lazy(() =>
  import('./application-deployments-panel').then(module => ({ default: module.ApplicationDeploymentsPanel })),
)
const ApplicationGatewayPanel = lazy(() =>
  import('./application-gateway-panel').then(module => ({ default: module.ApplicationGatewayPanel })),
)
const ApplicationTopologyPanel = lazy(() =>
  import('./application-topology-panel').then(module => ({ default: module.ApplicationTopologyPanel })),
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
export function ApplicationConfigPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const { projectId = '', applicationId = '' } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState(() => searchParams.get('tab') || 'overview')
  const shouldPollWorkflowStatus = activeTab === 'builds' || activeTab === 'deployments'
  const needsOverviewData = activeTab === 'overview'
  const needsRepositoryBindings = needsOverviewData || activeTab === 'repositories' || activeTab === 'deployments'
  const needsBuildRuns = needsOverviewData || activeTab === 'builds' || activeTab === 'deployments'
  const needsDeploymentData = needsOverviewData || activeTab === 'deployments'
  const needsDeploymentTargets = needsDeploymentData || activeTab === 'builds' || activeTab === 'gateway'
  const needsRoutes = needsDeploymentData || activeTab === 'gateway'
  const buildsPanelRef = useRef<BuildsPanelHandle>(null)
  const deploymentsPanelRef = useRef<DeploymentsPanelHandle>(null)
  const gatewayPanelRef = useRef<ApplicationGatewayPanelHandle>(null)
  const repositoryBindingsPageRef = useRef<RepositoryBindingsPageHandle>(null)
  const application = useQuery({
    queryKey: ['application', projectId, applicationId],
    queryFn: () => api.getApplication(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
  })
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId && (activeTab === 'builds' || activeTab === 'deployments')) })
  const repositoryBindings = useQuery({ ...liveObservationQueryPolicy, queryKey: ['repository-bindings', projectId, applicationId], queryFn: () => api.listRepositoryBindings(projectId, applicationId), enabled: Boolean(projectId && applicationId && needsRepositoryBindings) })
  const registries = useQuery({ ...liveObservationQueryPolicy, queryKey: ['registries', projectId], queryFn: () => api.listRegistries(projectId), enabled: Boolean(projectId && activeTab === 'deployments') })
  const buildRuns = useQuery({
    queryKey: ['build-runs', projectId, applicationId],
    queryFn: () => api.listBuildRuns(projectId, applicationId),
    enabled: Boolean(projectId && applicationId && needsBuildRuns),
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
    enabled: Boolean(projectId && applicationId && needsDeploymentData),
    refetchInterval: query => activeTab === 'deployments'
      ? statusRefetchInterval((query.state.data ?? []).some(release => release.status === 'pending' || release.status === 'running'))
      : false,
  })
  const deploymentTargets = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-targets', projectId, applicationId],
    queryFn: () => api.listDeploymentTargets(projectId, applicationId),
    enabled: Boolean(projectId && applicationId && needsDeploymentTargets),
    refetchInterval: activeTab === 'deployments'
      ? statusRefetchInterval((releases.data ?? []).some(release => release.status === 'pending' || release.status === 'running'))
      : false,
  })
  const routes = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['gateway-routes', projectId, applicationId],
    queryFn: () => api.listGatewayRoutes(projectId, applicationId),
    enabled: Boolean(projectId && applicationId && needsRoutes),
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
  const runAfterTabChange = (tab: string, action: () => void) => {
    setActiveTab(tab)
    window.setTimeout(action, 0)
  }
  const openRepositoryBindingTask = () => runAfterTabChange('repositories', () => repositoryBindingsPageRef.current?.openCreateDialog())
  const openDeploymentTargetTask = () => runAfterTabChange('deployments', () => deploymentsPanelRef.current?.openTargetDialog())
  const openBuildTask = () => runAfterTabChange('builds', () => buildsPanelRef.current?.openTriggerDrawer())
  const openReleaseTask = () => {
    const target = firstReleaseReadyTarget(deploymentTargetRows, appBuildRuns)
    runAfterTabChange('deployments', () => target && deploymentsPanelRef.current?.openReleaseDialog('', target.id))
  }
  const openGatewayTask = () => runAfterTabChange('gateway', () => gatewayPanelRef.current?.openCreateDialog())
  if (application.isError)
    return <ErrorState title={t('apps.loadFailedTitle')} description={t('apps.appLoadFailedDescription')} />

  return (
    <PageShell spacing="compact" width="full">
      <ContentTabs
        tabs={[
          { label: t('apps.overview'), value: 'overview' },
          { label: t('apps.repoBinding'), value: 'repositories' },
          { label: t('builds'), value: 'builds' },
          { label: t('deployments'), value: 'deployments' },
          { label: t('gatewayRoutes'), value: 'gateway' },
          { label: t('apps.topology.title'), value: 'topology' },
          { label: t('apps.configTab'), value: 'settings' },
        ]}
        tools={(
          <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
            {activeTab === 'deployments' && (
              <>
                <Button variant="outline" onClick={() => deploymentsPanelRef.current?.openImportDialog()}>
                  <Upload size={16} />
                  {t('deploymentsPage.bundleImport.open')}
                </Button>
                <Button variant="outline" onClick={() => deploymentsPanelRef.current?.openTargetDialog()}>
                  <Plus size={16} />
                  {t('deploymentsPage.createDeploymentTarget')}
                </Button>
                <Button disabled={!releaseReadyTarget} onClick={() => releaseReadyTarget && deploymentsPanelRef.current?.openReleaseDialog('', releaseReadyTarget.id)}>
                  <Package size={16} />
                  {t('deploymentsPage.createRelease')}
                </Button>
              </>
            )}
            {activeTab === 'builds' && (
              <>
                <Button disabled={!deploymentTargets.data?.some(target => target.sourceType === 'repository' && target.repositoryBindingId)} onClick={() => buildsPanelRef.current?.openTriggerDrawer()}>
                  <Play size={16} />
                  {t('buildsPage.triggerBuild')}
                </Button>
              </>
            )}
            {activeTab === 'repositories' && (
              <Button disabled={!projectId || !applicationId} onClick={() => repositoryBindingsPageRef.current?.openCreateDialog()}>
                <Plus size={16} />
                {t('repositories.addRepository')}
              </Button>
            )}
            {activeTab === 'gateway' && (
              <Button disabled={!deploymentTargets.data?.length} onClick={() => gatewayPanelRef.current?.openCreateDialog()}>
                <Globe2 size={16} />
                {t('gatewayRoutesPage.createRoute')}
              </Button>
            )}
            {activeTab === 'settings' && (
              <Button disabled={updateApplication.isPending || !updateForm.formState.isValid} form={APPLICATION_CONFIG_FORM_ID} type="submit">
                <Save size={16} />
                {t('apps.saveConfig')}
              </Button>
            )}
          </div>
        )}
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
        <TabsContent value="overview">
          <ApplicationOverviewPanel
            app={application.data}
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
          <Card className="p-4">
            <div className="mb-4">
              <h3 className="text-base font-semibold">{t('apps.configTitle')}</h3>
              <p className="mt-1 text-sm text-muted-foreground">{t('apps.configDescription')}</p>
            </div>
            <form id={APPLICATION_CONFIG_FORM_ID} onSubmit={updateForm.handleSubmit(values => updateApplication.mutate(values))}>
              <MotionList className="grid gap-4">
                <MotionItem>
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
                </MotionItem>
              </MotionList>
            </form>
          </Card>
        </TabsContent>
        <TabsContent value="repositories">
          <Suspense fallback={<TabFallback />}>
            <RepositoryBindingsPage
              ref={repositoryBindingsPageRef}
              applicationId={applicationId}
              embedded
              projectId={projectId}
            />
          </Suspense>
        </TabsContent>
        <TabsContent value="builds">
          <Suspense fallback={<TabFallback />}>
            <ApplicationBuildsPanel
              ref={buildsPanelRef}
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
              ref={deploymentsPanelRef}
              applicationId={applicationId}
              applicationIdentifier={application.data?.identifier ?? ''}
              buildRuns={appBuildRuns}
              deploymentTargets={deploymentTargetRows}
              projectId={projectId}
              projectIdentifier={project.data?.identifier ?? ''}
              projectWebConsoleEnabled={project.data?.webConsoleEnabled ?? true}
              canManageRuntimeSecrets={isPlatformAdmin(user?.role) || ['owner', 'admin', 'developer'].includes(project.data?.currentUserRole ?? '')}
              registries={registries.data ?? []}
              repositoryBindings={appRepositoryBindings}
              releases={appReleases}
              routes={appRoutes}
            />
          </Suspense>
        </TabsContent>
        <TabsContent value="gateway">
          <Suspense fallback={<TabFallback />}>
            <ApplicationGatewayPanel
              ref={gatewayPanelRef}
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
