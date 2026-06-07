import type { ArtifactRegistry, BuildRun, GatewayRoute, GitRepositoryBuildOptions, Release } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Globe2, Play, RotateCcw, Save, SearchCheck, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { MotionItem, MotionList } from '@/components/common/motion'
import { SearchSelect } from '@/components/common/search-select'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'

const schema = z.object({
  name: z.string().min(1, i18next.t('apps.nameRequired')),
  slug: z.string().min(1, i18next.t('apps.slugRequired')).regex(/^[a-z0-9-]+$/, i18next.t('common.lowercaseSlugOnly')),
  sourceType: z.enum(['repository', 'image']),
  repositoryUrl: z.string().optional(),
  imageReference: z.string().optional(),
  dockerfilePath: z.string().optional(),
  buildContext: z.string().optional(),
  servicePort: z.coerce.number().int(i18next.t('apps.integerPort')).positive(i18next.t('apps.positivePort')),
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>
type TriggerForm = Partial<BuildRun>
type ReleaseForm = Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>
type RouteForm = Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget'> & { applicationSlug?: string, stage?: string }

const triggerDefaults: TriggerForm = { applicationId: '', buildContext: '.', dockerfilePath: 'Dockerfile', sourceBranch: '', targetRegistryId: '', targetTag: 'latest', triggerType: 'manual' }
const releaseDefaults: ReleaseForm = { applicationId: '', buildRunId: '', environmentId: '', imageRef: '', message: '', revision: 1, status: 'pending', type: 'deploy' }
const routeDefaults: RouteForm = { applicationId: '', applicationSlug: '', certificateStatus: 'disabled', dnsStatus: 'pending', environmentId: '', host: '', isDefault: false, path: '/', servicePort: 8080, stage: 'dev', status: 'pending', tlsMode: 'http-only' }

export function ApplicationConfigPage() {
  const { t } = useTranslation()
  const { projectId = '', applicationId = '' } = useParams()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('overview')
  const application = useQuery({
    queryKey: ['application', projectId, applicationId],
    queryFn: () => api.getApplication(projectId, applicationId),
    enabled: Boolean(projectId && applicationId),
  })
  const repositoryBindings = useQuery({ queryKey: ['repository-bindings', projectId], queryFn: () => api.listRepositoryBindings(projectId), enabled: Boolean(projectId) })
  const registries = useQuery({ queryKey: ['registries', projectId], queryFn: () => api.listRegistries(projectId), enabled: Boolean(projectId) })
  const providers = useQuery({ queryKey: ['build-providers', projectId], queryFn: () => api.listBuildProviders(projectId), enabled: Boolean(projectId) })
  const buildRuns = useQuery({ queryKey: ['build-runs', projectId], queryFn: () => api.listBuildRuns(projectId), enabled: Boolean(projectId) })
  const buildJobs = useQuery({ queryKey: ['build-jobs', projectId], queryFn: () => api.listBuildJobs(projectId), enabled: Boolean(projectId) })
  const environments = useQuery({ queryKey: ['environments', projectId], queryFn: () => api.listEnvironments(projectId), enabled: Boolean(projectId) })
  const releases = useQuery({ queryKey: ['releases', projectId], queryFn: () => api.listReleases(projectId), enabled: Boolean(projectId) })
  const routes = useQuery({ queryKey: ['gateway-routes', projectId], queryFn: () => api.listGatewayRoutes(projectId), enabled: Boolean(projectId) })

  const binding = useMemo(() => (repositoryBindings.data ?? []).find(item => item.applicationId === applicationId), [applicationId, repositoryBindings.data])
  const appBuildRuns = useMemo(() => (buildRuns.data ?? []).filter(run => run.applicationId === applicationId), [applicationId, buildRuns.data])
  const appBuildRunIds = useMemo(() => new Set(appBuildRuns.map(run => run.id)), [appBuildRuns])
  const appBuildJobs = useMemo(() => (buildJobs.data ?? []).filter(job => appBuildRunIds.has(job.buildRunId)), [appBuildRunIds, buildJobs.data])
  const appReleases = useMemo(() => (releases.data ?? []).filter(release => release.applicationId === applicationId), [applicationId, releases.data])
  const appRoutes = useMemo(() => (routes.data ?? []).filter(route => route.applicationId === applicationId), [applicationId, routes.data])

  const updateForm = useForm<ApplicationFormInput, undefined, ApplicationForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { buildContext: '.', dockerfilePath: 'Dockerfile', imageReference: '', name: '', repositoryUrl: '', servicePort: 8080, slug: '', sourceType: 'repository' },
  })
  const sourceType = updateForm.watch('sourceType')

  useEffect(() => {
    if (!application.data)
      return
    updateForm.reset({
      name: application.data.name,
      slug: application.data.slug,
      sourceType: application.data.sourceType,
      repositoryUrl: application.data.repositoryUrl,
      imageReference: application.data.imageReference,
      dockerfilePath: application.data.dockerfilePath,
      buildContext: application.data.buildContext,
      servicePort: application.data.servicePort,
    })
  }, [application.data, updateForm])

  const updateApplication = useMutation({
    mutationFn: (payload: ApplicationForm) => api.updateApplication(projectId, applicationId, {
      name: payload.name,
      slug: payload.slug,
      sourceType: payload.sourceType,
      gitAccountId: application.data?.gitAccountId ?? '',
      repositoryUrl: payload.repositoryUrl ?? '',
      imageReference: payload.imageReference ?? '',
      dockerfilePath: payload.dockerfilePath ?? 'Dockerfile',
      buildContext: payload.buildContext ?? '.',
      servicePort: payload.servicePort,
    }),
    onSuccess: (result) => {
      toast.success(t('apps.configSaved'))
      queryClient.setQueryData(['application', projectId, applicationId], result)
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
          { label: t('builds'), value: 'builds' },
          { label: t('deployments'), value: 'deployments' },
          { label: t('gatewayRoutes'), value: 'gateway' },
          { label: t('apps.configTab'), value: 'config' },
        ]}
        tools={<Link className="text-sm text-primary hover:underline" to={`/projects/${projectId}`}>{t('backToApps')}</Link>}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value="overview">
          <Card className="grid gap-4 p-4 md:grid-cols-2">
            <OverviewItem label={t('apps.name')} value={application.data?.name ?? t('common.loading')} />
            <OverviewItem label={t('common.slug')} value={application.data?.slug ?? '-'} />
            <OverviewItem label={t('apps.sourceType')} value={application.data ? t(`apps.${application.data.sourceType}`) : '-'} />
            <OverviewItem label={t('apps.servicePort')} value={String(application.data?.servicePort ?? '-')} />
            <OverviewItem label={t('repositories.defaultBranch')} value={binding?.defaultBranch || '-'} />
            <OverviewItem label={t('apps.repositoryUrl')} value={binding ? `${binding.owner}/${binding.repo}` : application.data?.repositoryUrl || '-'} />
          </Card>
        </TabsContent>
        <TabsContent value="builds">
          <ApplicationBuildsPanel
            applicationId={applicationId}
            binding={binding}
            buildJobs={appBuildJobs}
            buildRuns={appBuildRuns}
            projectId={projectId}
            providers={providers.data ?? []}
            registries={registries.data ?? []}
          />
        </TabsContent>
        <TabsContent value="deployments">
          <ApplicationDeploymentsPanel
            buildRuns={appBuildRuns}
            environments={environments.data ?? []}
            projectId={projectId}
            releases={appReleases}
          />
        </TabsContent>
        <TabsContent value="gateway">
          <ApplicationGatewayPanel
            applicationId={applicationId}
            applicationSlug={application.data?.slug ?? ''}
            environments={environments.data ?? []}
            projectId={projectId}
            routes={appRoutes}
            servicePort={application.data?.servicePort ?? 8080}
          />
        </TabsContent>
        <TabsContent value="config">
          <Card className="max-w-2xl p-4">
            <form onSubmit={updateForm.handleSubmit(values => updateApplication.mutate(values))}>
              <MotionList className="grid gap-4">
                <div className="grid gap-3 md:grid-cols-2">
                  <MotionItem><Field error={updateForm.formState.errors.name?.message} hint={t('apps.nameHint')} label={t('apps.name')} required><Input {...updateForm.register('name')} aria-invalid={Boolean(updateForm.formState.errors.name)} /></Field></MotionItem>
                  <MotionItem><Field error={updateForm.formState.errors.slug?.message} hint={t('apps.slugHint')} label={t('apps.slug')} required><Input {...updateForm.register('slug')} aria-invalid={Boolean(updateForm.formState.errors.slug)} /></Field></MotionItem>
                </div>
                <MotionItem>
                  <Field error={updateForm.formState.errors.sourceType?.message} hint={t('apps.sourceTypeHint')} label={t('apps.sourceType')} required>
                    <Select {...updateForm.register('sourceType')} aria-invalid={Boolean(updateForm.formState.errors.sourceType)}>
                      <option value="repository">{t('apps.repository')}</option>
                      <option value="image">{t('apps.image')}</option>
                    </Select>
                  </Field>
                </MotionItem>
                {sourceType === 'repository'
                  ? (
                      <>
                        <MotionItem><Field error={updateForm.formState.errors.repositoryUrl?.message} hint={t('apps.repositoryUrlHint')} label={t('apps.repositoryUrl')}><Input {...updateForm.register('repositoryUrl')} aria-invalid={Boolean(updateForm.formState.errors.repositoryUrl)} placeholder={t('apps.repositoryUrlPlaceholder')} /></Field></MotionItem>
                        <div className="grid gap-3 md:grid-cols-2">
                          <MotionItem><Field error={updateForm.formState.errors.dockerfilePath?.message} hint={t('apps.dockerfileHint')} label={t('apps.dockerfile')}><Input {...updateForm.register('dockerfilePath')} aria-invalid={Boolean(updateForm.formState.errors.dockerfilePath)} /></Field></MotionItem>
                          <MotionItem><Field error={updateForm.formState.errors.buildContext?.message} hint={t('apps.buildContextHint')} label={t('apps.buildContext')}><Input {...updateForm.register('buildContext')} aria-invalid={Boolean(updateForm.formState.errors.buildContext)} /></Field></MotionItem>
                        </div>
                      </>
                    )
                  : <MotionItem><Field error={updateForm.formState.errors.imageReference?.message} hint={t('apps.imageReferenceHint')} label={t('apps.imageReference')}><Input {...updateForm.register('imageReference')} aria-invalid={Boolean(updateForm.formState.errors.imageReference)} placeholder={t('apps.imageReferencePlaceholder')} /></Field></MotionItem>}
                <MotionItem><Field error={updateForm.formState.errors.servicePort?.message} hint={t('apps.servicePortHint')} label={t('apps.servicePort')} required><Input type="number" {...updateForm.register('servicePort')} aria-invalid={Boolean(updateForm.formState.errors.servicePort)} /></Field></MotionItem>
                <MotionItem>
                  <Button className="w-fit" disabled={updateApplication.isPending || !updateForm.formState.isValid} type="submit">
                    <Save size={16} />
                    {t('apps.saveConfig')}
                  </Button>
                </MotionItem>
              </MotionList>
            </form>
          </Card>
        </TabsContent>
      </ContentTabs>
    </div>
  )
}

function OverviewItem({ label, value }: { label: string, value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-xs font-medium uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-sm text-foreground" title={value}>{value}</div>
    </div>
  )
}

function ApplicationBuildsPanel({ applicationId, binding, buildJobs, buildRuns, projectId, providers, registries }: {
  applicationId: string
  binding?: { defaultBranch: string, gitAccountId: string, owner: string, repo: string }
  buildJobs: Array<{ id: string, buildRunId: string, status: string, attempts: number }>
  buildRuns: BuildRun[]
  projectId: string
  providers: Array<{ id: string, name: string }>
  registries: ArtifactRegistry[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [branchSearch, setBranchSearch] = useState('')
  const form = useForm<TriggerForm>({ defaultValues: triggerDefaults, mode: 'onChange' })
  const selectedBranch = form.watch('sourceBranch') ?? ''
  const branches = useQuery({
    queryKey: ['git-branches', binding?.gitAccountId, binding?.owner, binding?.repo, branchSearch],
    queryFn: () => api.listGitBranches(binding?.gitAccountId ?? '', binding?.owner ?? '', binding?.repo ?? '', { search: branchSearch, limit: 50 }),
    enabled: Boolean(binding),
  })
  const buildOptions = useQuery({
    queryKey: ['git-build-options', binding?.gitAccountId, binding?.owner, binding?.repo, selectedBranch],
    queryFn: () => api.getGitRepositoryBuildOptions(binding?.gitAccountId ?? '', binding?.owner ?? '', binding?.repo ?? '', selectedBranch || binding?.defaultBranch || 'main'),
    enabled: Boolean(binding && (selectedBranch || binding.defaultBranch)),
  })
  const triggerBuild = useMutation({
    mutationFn: (values: TriggerForm) => api.triggerBuildRun(projectId, { ...values, applicationId }),
    onSuccess: () => {
      toast.success(t('buildsPage.buildQueued'))
      setDialogOpen(false)
      form.reset(triggerDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-runs', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  useEffect(() => {
    if (!dialogOpen)
      return
    form.reset({ ...triggerDefaults, applicationId, buildContext: '.', dockerfilePath: 'Dockerfile', sourceBranch: binding?.defaultBranch || 'main' })
  }, [applicationId, binding?.defaultBranch, dialogOpen, form])

  useEffect(() => {
    if (!dialogOpen || !registries.length || form.getValues('targetRegistryId'))
      return
    const defaultRegistry = registries.find(registry => registry.credentialSet && registry.isDefault) ?? registries.find(registry => registry.credentialSet) ?? registries.find(registry => registry.isDefault) ?? registries[0]
    form.setValue('targetRegistryId', defaultRegistry.id, { shouldDirty: true, shouldValidate: true })
  }, [dialogOpen, form, registries])

  return (
    <div className="grid gap-4">
      <div className="flex justify-end">
        <Button disabled={!binding} onClick={() => setDialogOpen(true)}>
          <Play className="size-4" />
          {t('buildsPage.triggerBuild')}
        </Button>
      </div>
      {binding
        ? (
            <DataList
              columns={[
                { key: 'id', header: t('common.id'), render: item => item.id },
                { key: 'status', header: t('common.status'), render: item => <BuildStatusBadge status={item.status} /> },
                { key: 'target', header: t('buildsPage.targetImage'), render: item => buildRunImageRef(item) || '-' },
                { key: 'commit', header: t('buildsPage.sourceCommit'), render: item => item.sourceCommit || '-' },
              ]}
              emptyTitle={t('buildsPage.emptyRuns')}
              items={buildRuns}
              rowKey={item => item.id}
              variant="plain"
            />
          )
        : <EmptyState title={t('buildsPage.repositoryBindingRequired')} />}
      <DataList
        columns={[
          { key: 'id', header: t('common.id'), render: item => item.id },
          { key: 'run', header: t('buildsPage.buildRun'), render: item => item.buildRunId },
          { key: 'status', header: t('common.status'), render: item => <BuildStatusBadge status={item.status} /> },
          { key: 'attempts', header: t('buildsPage.attempts'), render: item => item.attempts },
        ]}
        emptyTitle={t('buildsPage.emptyJobs')}
        items={buildJobs}
        rowKey={item => item.id}
        variant="plain"
      />
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('buildsPage.triggerBuild')}</DialogTitle>
            <DialogDescription>{t('buildsPage.triggerDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => triggerBuild.mutate(values))}>
            <Field label={t('repositories.defaultBranch')} required>
              <SearchSelect
                disabled={!binding}
                emptyLabel={binding ? t('common.noOptions') : t('buildsPage.repositoryBindingRequired')}
                limited={branches.data?.limited}
                loading={branches.isFetching}
                options={branchOptions(branches.data?.items ?? [], form.watch('sourceBranch'))}
                placeholder={t('repositories.defaultBranchPlaceholder')}
                search={branchSearch}
                value={form.watch('sourceBranch') || ''}
                onSearchChange={setBranchSearch}
                onValueChange={value => form.setValue('sourceBranch', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
            <Field label={t('buildsPage.provider')}>
              <Select {...form.register('buildProviderId')}>
                <option value="">{t('common.none')}</option>
                {providers.map(provider => <option key={provider.id} value={provider.id}>{provider.name}</option>)}
              </Select>
            </Field>
            <Field label={t('buildsPage.targetRegistry')} required>
              <Select {...form.register('targetRegistryId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {registries.map(registry => <option key={registry.id} value={registry.id}>{registryOptionLabel(registry)}</option>)}
              </Select>
            </Field>
            <Field label={t('buildsPage.targetRepository')} required><Input {...form.register('targetRepository', { required: true })} /></Field>
            <Field label={t('buildsPage.targetTag')}><Input {...form.register('targetTag')} /></Field>
            <Field hint={t('buildsPage.dockerfileLookupHint')} label={t('buildsPage.dockerfilePath')}>
              <Input {...form.register('dockerfilePath')} list="app-build-run-dockerfile-options" />
              <datalist id="app-build-run-dockerfile-options">
                {dockerfileOptions(buildOptions.data, form.watch('dockerfilePath')).map(option => <option key={option.value} value={option.value} />)}
              </datalist>
            </Field>
            <Field hint={t('buildsPage.buildContextLookupHint')} label={t('buildsPage.buildContext')}>
              <Input {...form.register('buildContext')} list="app-build-run-context-options" />
              <datalist id="app-build-run-context-options">
                {buildContextOptions(buildOptions.data, form.watch('buildContext')).map(option => <option key={option.value} value={option.value} />)}
              </datalist>
            </Field>
            <DialogFooter><Button disabled={!form.formState.isValid || triggerBuild.isPending} type="submit">{t('buildsPage.queueBuild')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ApplicationDeploymentsPanel({ buildRuns, environments, projectId, releases }: {
  buildRuns: BuildRun[]
  environments: Array<{ id: string, name: string }>
  projectId: string
  releases: Release[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const form = useForm<ReleaseForm>({ defaultValues: releaseDefaults, mode: 'onChange' })
  const buildRunMap = useMemo(() => Object.fromEntries(buildRuns.map(run => [run.id, run])), [buildRuns])
  const selectedBuildRun = buildRunMap[form.watch('buildRunId')]
  useEffect(() => {
    if (!selectedBuildRun)
      return
    form.setValue('applicationId', selectedBuildRun.applicationId, { shouldDirty: true, shouldValidate: true })
    form.setValue('imageRef', buildRunImageRef(selectedBuildRun), { shouldDirty: true, shouldValidate: true })
  }, [form, selectedBuildRun])
  const createRelease = useMutation({
    mutationFn: (values: ReleaseForm) => api.createRelease(projectId, values),
    onSuccess: () => {
      toast.success(t('deploymentsPage.releaseCreated'))
      setDialogOpen(false)
      form.reset(releaseDefaults)
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const rollbackRelease = useMutation({
    mutationFn: (releaseId: string) => api.rollbackRelease(projectId, releaseId),
    onSuccess: () => {
      toast.success(t('deploymentsPage.rollbackQueued'))
      queryClient.invalidateQueries({ queryKey: ['releases', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  return (
    <div className="grid gap-4">
      <div className="flex justify-end">
        <Button disabled={!buildRuns.length || !environments.length} onClick={() => setDialogOpen(true)}>
          <RotateCcw className="size-4" />
          {t('deploymentsPage.createRelease')}
        </Button>
      </div>
      <DataList
        columns={[
          { key: 'id', header: t('common.id'), render: item => item.id },
          { key: 'image', header: t('deploymentsPage.image'), render: item => item.imageRef },
          { key: 'branch', header: t('deploymentsPage.sourceBranch'), render: item => buildRunMap[item.buildRunId]?.sourceBranch || '-' },
          { key: 'status', header: t('common.status'), render: item => item.status },
          { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
            <Button size="sm" variant="ghost" onClick={() => rollbackRelease.mutate(item.id)}>
              <RotateCcw className="size-4" />
              {t('deploymentsPage.rollback')}
            </Button>
          ) },
        ]}
        emptyTitle={t('deploymentsPage.emptyReleases')}
        items={releases}
        rowKey={item => item.id}
        variant="plain"
      />
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('deploymentsPage.createRelease')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.releaseDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => createRelease.mutate(values))}>
            <Field hint={t('deploymentsPage.buildRunHint')} label={t('deploymentsPage.buildRun')} required>
              <Select {...form.register('buildRunId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {buildRuns.map(run => <option key={run.id} value={run.id}>{buildRunOptionLabel(run)}</option>)}
              </Select>
            </Field>
            <Field label={t('deploymentsPage.environment')} required>
              <Select {...form.register('environmentId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {environments.map(environment => <option key={environment.id} value={environment.id}>{environment.name}</option>)}
              </Select>
            </Field>
            <Field label={t('deploymentsPage.image')} required><Input {...form.register('imageRef', { required: true })} /></Field>
            <DialogFooter><Button disabled={!form.formState.isValid || createRelease.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ApplicationGatewayPanel({ applicationId, applicationSlug, environments, projectId, routes, servicePort }: {
  applicationId: string
  applicationSlug: string
  environments: Array<{ id: string, name: string }>
  projectId: string
  routes: GatewayRoute[]
  servicePort: number
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRoute, setEditingRoute] = useState<GatewayRoute | null>(null)
  const [routeToDelete, setRouteToDelete] = useState<GatewayRoute | null>(null)
  const form = useForm<RouteForm>({ defaultValues: routeDefaults, mode: 'onChange' })
  const saveRoute = useMutation({
    mutationFn: (values: RouteForm) => {
      const payload = { ...values, applicationId, applicationSlug, servicePort: values.servicePort || servicePort }
      return editingRoute ? api.updateGatewayRoute(projectId, editingRoute.id, payload) : api.createGatewayRoute(projectId, payload)
    },
    onSuccess: () => {
      toast.success(t(editingRoute ? 'gatewayRoutesPage.routeUpdated' : 'gatewayRoutesPage.routeCreated'))
      setDialogOpen(false)
      setEditingRoute(null)
      form.reset(routeDefaults)
      queryClient.invalidateQueries({ queryKey: ['gateway-routes', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteRoute = useMutation({
    mutationFn: (routeId: string) => api.deleteGatewayRoute(projectId, routeId),
    onSuccess: () => {
      toast.success(t('gatewayRoutesPage.routeDeleted'))
      setRouteToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['gateway-routes', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const checkDomain = useMutation({
    mutationFn: (host: string) => api.checkGatewayDomain(projectId, host),
    onSuccess: result => toast.success(result.available ? t('gatewayRoutesPage.domainAvailable') : t('gatewayRoutesPage.domainUnavailable')),
    onError: error => toast.error(error.message),
  })
  function openRouteDialog(route?: GatewayRoute) {
    setEditingRoute(route ?? null)
    form.reset(route ? { ...route, applicationSlug, stage: 'dev' } : { ...routeDefaults, applicationId, applicationSlug, servicePort })
    setDialogOpen(true)
  }
  return (
    <div className="grid gap-4">
      <div className="flex justify-end">
        <Button onClick={() => openRouteDialog()}>
          <Globe2 className="size-4" />
          {t('gatewayRoutesPage.createRoute')}
        </Button>
      </div>
      <DataList
        columns={[
          { key: 'host', header: t('gatewayRoutesPage.host'), render: item => item.host },
          { key: 'path', header: t('gatewayRoutesPage.path'), render: item => item.path },
          { key: 'tls', header: t('gatewayRoutesPage.tlsMode'), render: item => item.tlsMode },
          { key: 'status', header: t('common.status'), render: item => item.status },
          { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
            <div className="flex justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => checkDomain.mutate(item.host)}>
                <SearchCheck className="size-4" />
                {t('gatewayRoutesPage.checkDomain')}
              </Button>
              <EditActionButton label={t('common.edit')} onClick={() => openRouteDialog(item)} />
              <Button size="sm" variant="ghost" onClick={() => setRouteToDelete(item)}>
                <Trash2 className="size-4" />
                {t('common.delete')}
              </Button>
            </div>
          ) },
        ]}
        emptyTitle={t('gatewayRoutesPage.emptyRoutes')}
        items={routes}
        rowKey={item => item.id}
        variant="plain"
      />
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingRoute ? t('gatewayRoutesPage.editRoute') : t('gatewayRoutesPage.createRoute')}</DialogTitle>
            <DialogDescription>{t('gatewayRoutesPage.routeDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={form.handleSubmit(values => saveRoute.mutate(values))}>
            <Field label={t('deploymentsPage.environment')}>
              <Select {...form.register('environmentId')}>
                <option value="">{t('common.none')}</option>
                {environments.map(environment => <option key={environment.id} value={environment.id}>{environment.name}</option>)}
              </Select>
            </Field>
            <Field hint={t('gatewayRoutesPage.hostHint')} label={t('gatewayRoutesPage.host')} required><Input {...form.register('host', { required: true })} /></Field>
            <Field label={t('deploymentsPage.stage')}>
              <Select {...form.register('stage')}>
                <option value="dev">{t('deploymentsPage.stageDev')}</option>
                <option value="test">{t('deploymentsPage.stageTest')}</option>
                <option value="staging">{t('deploymentsPage.stageStaging')}</option>
                <option value="prod">{t('deploymentsPage.stageProd')}</option>
              </Select>
            </Field>
            <Field label={t('gatewayRoutesPage.path')}><Input {...form.register('path')} /></Field>
            <Field label={t('gatewayRoutesPage.servicePort')}><Input {...form.register('servicePort', { valueAsNumber: true })} type="number" /></Field>
            <Field label={t('gatewayRoutesPage.tlsMode')}>
              <Select {...form.register('tlsMode')}>
                <option value="http-only">{t('gatewayRoutesPage.tlsHttpOnly')}</option>
                <option value="http-challenge">{t('gatewayRoutesPage.tlsHttpChallenge')}</option>
                <option value="manual-cert">{t('gatewayRoutesPage.tlsManualCert')}</option>
              </Select>
            </Field>
            <DialogFooter><Button disabled={!form.formState.isValid || saveRoute.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ConfirmDialog cancelText={t('common.cancel')} confirmText={t('common.delete')} description={t('gatewayRoutesPage.deleteRouteDescription')} open={Boolean(routeToDelete)} title={t('gatewayRoutesPage.deleteRouteTitle')} onConfirm={() => routeToDelete && deleteRoute.mutate(routeToDelete.id)} onOpenChange={open => !open && setRouteToDelete(null)} />
    </div>
  )
}

function branchOptions(branches: Array<{ name: string }>, current?: string) {
  return withCurrentOption(branches.map(branch => branch.name), current)
}

function dockerfileOptions(options?: GitRepositoryBuildOptions, current?: string) {
  return withCurrentOption(options?.dockerfiles ?? [], current)
}

function buildContextOptions(options?: GitRepositoryBuildOptions, current?: string) {
  return withCurrentOption(options?.directories ?? ['.'], current)
}

function registryOptionLabel(registry: ArtifactRegistry) {
  return [registry.name, registry.provider, registry.namespace ? `/${registry.namespace}` : ''].filter(Boolean).join(' · ')
}

function buildRunImageRef(run: BuildRun) {
  if (run.imageRef)
    return run.imageRef
  if (run.targetRepository)
    return `${run.targetRepository}:${run.targetTag || 'latest'}`
  return ''
}

function buildRunOptionLabel(run: BuildRun) {
  const branch = run.sourceBranch || run.sourceTag || '-'
  const image = buildRunImageRef(run) || run.targetRepository || run.id
  return `${branch} · ${run.status} · ${image}`
}

function BuildStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <StatusBadge className={buildStatusClassName(status)}>
      {t(`buildsPage.statuses.${status}`, { defaultValue: status })}
    </StatusBadge>
  )
}

function buildStatusClassName(status: string) {
  switch (status) {
    case 'queued':
    case 'pending':
      return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/60 dark:bg-sky-950/40 dark:text-sky-300'
    case 'running':
      return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-300'
    case 'succeeded':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/40 dark:text-emerald-300'
    case 'failed':
      return 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900/60 dark:bg-rose-950/40 dark:text-rose-300'
    case 'canceled':
      return 'border-zinc-200 bg-zinc-50 text-zinc-700 dark:border-zinc-800 dark:bg-zinc-900/60 dark:text-zinc-300'
    default:
      return 'border-border bg-muted text-muted-foreground'
  }
}

function withCurrentOption(values: string[], current?: string) {
  const options = values.map(value => ({ value, label: value }))
  const normalized = current?.trim()
  if (normalized && !options.some(option => option.value === normalized))
    options.unshift({ value: normalized, label: normalized })
  return options
}
