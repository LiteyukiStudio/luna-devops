import type { BuildsPanelHandle } from './application-builds-panel'
import type { DeploymentsPanelHandle } from './application-deployments-panel'
import type { ApplicationGatewayPanelHandle } from './application-gateway-panel'
import type { Application } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Globe2, Package, Play, Plus, Save } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ApplicationBasicFields } from '@/components/common/application-basic-fields'
import { ContentTabs } from '@/components/common/content-tabs'
import { ErrorState } from '@/components/common/error-state'
import { MotionItem, MotionList } from '@/components/common/motion'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { TabsContent } from '@/components/ui/tabs'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { APPLICATION_SLUG_MAX_LENGTH } from '@/lib/slug-limits'
import { RepositoryBindingsPage } from '@/pages/repositories/RepositoryBindingsPage'
import { ApplicationBuildsPanel } from './application-builds-panel'
import { firstReleaseReadyTarget } from './application-config-utils'
import { ApplicationDeploymentsPanel } from './application-deployments-panel'
import { ApplicationGatewayPanel } from './application-gateway-panel'
import { ApplicationOverviewPanel } from './application-overview-panel'

const schema = z.object({
  name: z.string().min(1, i18next.t('apps.nameRequired')),
  slug: z.string().min(1, i18next.t('apps.slugRequired')).max(APPLICATION_SLUG_MAX_LENGTH, i18next.t('apps.slugMaxLength', { count: APPLICATION_SLUG_MAX_LENGTH })).regex(/^[a-z0-9-]+$/, i18next.t('common.lowercaseSlugOnly')),
  icon: z.string().default('box'),
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>
const APPLICATION_CONFIG_FORM_ID = 'application-config-form'
export function ApplicationConfigPage() {
  const { t } = useTranslation()
  const { projectId = '', applicationId = '' } = useParams()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('overview')
  const shouldPollWorkflowStatus = activeTab === 'builds' || activeTab === 'deployments'
  const buildsPanelRef = useRef<BuildsPanelHandle>(null)
  const deploymentsPanelRef = useRef<DeploymentsPanelHandle>(null)
  const gatewayPanelRef = useRef<ApplicationGatewayPanelHandle>(null)
  const application = useQuery({
    queryKey: ['application', projectId, applicationId],
    queryFn: () => api.getApplication(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
  })
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId) })
  const repositoryBindings = useQuery({ queryKey: ['repository-bindings', projectId], queryFn: () => api.listRepositoryBindings(projectId), enabled: Boolean(projectId) })
  const registries = useQuery({ queryKey: ['registries', projectId], queryFn: () => api.listRegistries(projectId), enabled: Boolean(projectId) })
  const buildRuns = useQuery({
    queryKey: ['build-runs', projectId],
    queryFn: () => api.listBuildRuns(projectId),
    enabled: Boolean(projectId),
    refetchInterval: shouldPollWorkflowStatus ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const buildJobs = useQuery({
    queryKey: ['build-jobs', projectId],
    queryFn: () => api.listBuildJobs(projectId),
    enabled: Boolean(projectId),
    refetchInterval: shouldPollWorkflowStatus ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const releases = useQuery({
    queryKey: ['releases', projectId],
    queryFn: () => api.listReleases(projectId),
    enabled: Boolean(projectId),
    refetchInterval: activeTab === 'deployments' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const deploymentTargets = useQuery({ queryKey: ['deployment-targets', projectId, applicationId], queryFn: () => api.listDeploymentTargets(projectId, applicationId), enabled: Boolean(projectId && applicationId) })
  const routes = useQuery({ queryKey: ['gateway-routes', projectId], queryFn: () => api.listGatewayRoutes(projectId), enabled: Boolean(projectId) })
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
    defaultValues: { icon: 'box', name: '', slug: '' },
  })

  useEffect(() => {
    if (!application.data)
      return
    updateForm.reset({
      name: application.data.name,
      slug: application.data.slug,
      icon: application.data.icon ?? 'box',
    })
  }, [application.data, updateForm])

  useEffect(() => {
    if (!projectId || !shouldPollWorkflowStatus)
      return
    queryClient.invalidateQueries({ queryKey: ['build-runs', projectId] })
    queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
    if (activeTab === 'deployments')
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
  }, [activeTab, projectId, queryClient, shouldPollWorkflowStatus])

  const updateApplication = useMutation({
    mutationFn: (payload: ApplicationForm) => api.updateApplication(projectId, applicationId, {
      name: payload.name,
      slug: payload.slug,
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
  if (application.isError)
    return <ErrorState title={t('apps.loadFailedTitle')} description={t('apps.appLoadFailedDescription')} />

  return (
    <div className="grid gap-4">
      <ContentTabs
        tabs={[
          { label: t('apps.overview'), value: 'overview' },
          { label: t('apps.repoBinding'), value: 'repositories' },
          { label: t('builds'), value: 'builds' },
          { label: t('deployments'), value: 'deployments' },
          { label: t('gatewayRoutes'), value: 'gateway' },
        ]}
        tools={(
          <div className="flex items-center gap-2">
            {activeTab === 'deployments' && (
              <>
                <Button onClick={() => deploymentsPanelRef.current?.openTargetDialog()}>
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
            {activeTab === 'gateway' && (
              <Button disabled={!deploymentTargets.data?.length} onClick={() => gatewayPanelRef.current?.openCreateDialog()}>
                <Globe2 size={16} />
                {t('gatewayRoutesPage.createRoute')}
              </Button>
            )}
            {activeTab === 'overview' && (
              <Button disabled={updateApplication.isPending || !updateForm.formState.isValid} form={APPLICATION_CONFIG_FORM_ID} type="submit">
                <Save size={16} />
                {t('apps.saveConfig')}
              </Button>
            )}
          </div>
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value="overview">
          <ApplicationOverviewPanel
            app={application.data}
            buildRuns={appBuildRuns}
            deploymentTargets={deploymentTargetRows}
            releases={appReleases}
            routes={appRoutes}
          />
          <Card className="mt-4 p-4">
            <form id={APPLICATION_CONFIG_FORM_ID} onSubmit={updateForm.handleSubmit(values => updateApplication.mutate(values))}>
              <MotionList className="grid gap-4">
                <MotionItem>
                  <ApplicationBasicFields
                    compact
                    icon={updateForm.watch('icon')}
                    nameError={updateForm.formState.errors.name?.message}
                    nameField={updateForm.register('name')}
                    slugError={updateForm.formState.errors.slug?.message}
                    slugField={updateForm.register('slug')}
                    slugMaxLength={APPLICATION_SLUG_MAX_LENGTH}
                    onIconChange={icon => updateForm.setValue('icon', icon, { shouldDirty: true, shouldValidate: true })}
                  />
                </MotionItem>
              </MotionList>
            </form>
          </Card>
        </TabsContent>
        <TabsContent value="repositories">
          <RepositoryBindingsPage
            applicationId={applicationId}
            applicationName={application.data?.name}
            embedded
            projectId={projectId}
          />
        </TabsContent>
        <TabsContent value="builds">
          <ApplicationBuildsPanel
            ref={buildsPanelRef}
            applicationId={applicationId}
            appSlug={application.data?.slug ?? ''}
            binding={binding}
            repositoryBindings={appRepositoryBindings}
            buildJobs={appBuildJobs}
            deploymentTargets={deploymentTargetRows}
            buildRuns={appBuildRuns}
            projectId={projectId}
            projectSlug={project.data?.slug ?? ''}
            registries={registries.data ?? []}
          />
        </TabsContent>
        <TabsContent value="deployments">
          <ApplicationDeploymentsPanel
            ref={deploymentsPanelRef}
            applicationId={applicationId}
            appSlug={application.data?.slug ?? ''}
            buildRuns={appBuildRuns}
            deploymentTargets={deploymentTargetRows}
            projectId={projectId}
            projectSlug={project.data?.slug ?? ''}
            registries={registries.data ?? []}
            repositoryBindings={appRepositoryBindings}
            releases={appReleases}
          />
        </TabsContent>
        <TabsContent value="gateway">
          <ApplicationGatewayPanel
            ref={gatewayPanelRef}
            applicationId={applicationId}
            deploymentTargets={deploymentTargetRows}
            projectId={projectId}
            routes={appRoutes}
          />
        </TabsContent>
      </ContentTabs>
    </div>
  )
}
