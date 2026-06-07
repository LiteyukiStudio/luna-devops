import type { ArtifactRegistry, BuildJob, BuildLog, BuildRun, GatewayRoute, Release } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { CalendarClock, CircleCheck, CircleX, Clock3, GitBranch, Globe2, LoaderCircle, MoreHorizontal, Package, Play, RotateCcw, Save, ScrollText, Search, SearchCheck, Trash2, X } from 'lucide-react'
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
import { StatusValueBadge } from '@/components/common/status-badge'
import { TargetImageRefInput } from '@/components/common/target-image-ref-input'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { TabsContent } from '@/components/ui/tabs'

const schema = z.object({
  name: z.string().min(1, i18next.t('apps.nameRequired')),
  slug: z.string().min(1, i18next.t('apps.slugRequired')).regex(/^[a-z0-9-]+$/, i18next.t('common.lowercaseSlugOnly')),
  sourceType: z.enum(['repository', 'image']),
  repositoryUrl: z.string().optional(),
  imageReference: z.string().optional(),
  targetImageRef: z.string().optional(),
  dockerfilePath: z.string().optional(),
  buildContext: z.string().optional(),
  buildLabels: z.string().optional(),
  servicePort: z.coerce.number().int(i18next.t('apps.integerPort')).positive(i18next.t('apps.positivePort')),
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>
type TriggerForm = Partial<BuildRun>
type ReleaseForm = Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>
type RouteForm = Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget'> & { applicationSlug?: string, stage?: string }

const triggerDefaults: TriggerForm = { applicationId: '', sourceBranch: '', targetImageRef: '', targetRegistryId: '', triggerType: 'manual' }
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
  const project = useQuery({ queryKey: ['project', projectId], queryFn: () => api.getProject(projectId), enabled: Boolean(projectId) })
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
    defaultValues: { buildContext: '.', buildLabels: '', dockerfilePath: 'Dockerfile', imageReference: '', name: '', repositoryUrl: '', servicePort: 8080, slug: '', sourceType: 'repository', targetImageRef: '' },
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
      targetImageRef: application.data.targetImageRef,
      dockerfilePath: application.data.dockerfilePath,
      buildContext: application.data.buildContext,
      buildLabels: application.data.buildLabels,
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
      targetImageRef: payload.targetImageRef ?? '',
      dockerfilePath: payload.dockerfilePath ?? 'Dockerfile',
      buildContext: payload.buildContext ?? '.',
      buildLabels: payload.buildLabels ?? '',
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
            appSlug={application.data?.slug ?? ''}
            buildContext={application.data?.buildContext || '.'}
            binding={binding}
            buildJobs={appBuildJobs}
            buildRuns={appBuildRuns}
            dockerfilePath={application.data?.dockerfilePath || 'Dockerfile'}
            projectId={projectId}
            projectSlug={project.data?.slug ?? ''}
            providers={providers.data ?? []}
            registries={registries.data ?? []}
            targetImageRef={application.data?.targetImageRef ?? ''}
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
                        <MotionItem><Field error={updateForm.formState.errors.targetImageRef?.message} hint={t('apps.targetImageRefHint')} label={t('apps.targetImageRef')}><Input {...updateForm.register('targetImageRef')} aria-invalid={Boolean(updateForm.formState.errors.targetImageRef)} placeholder={t('apps.targetImageRefPlaceholder')} /></Field></MotionItem>
                        <div className="grid gap-3 md:grid-cols-2">
                          <MotionItem><Field error={updateForm.formState.errors.dockerfilePath?.message} hint={t('apps.dockerfileHint')} label={t('apps.dockerfile')}><Input {...updateForm.register('dockerfilePath')} aria-invalid={Boolean(updateForm.formState.errors.dockerfilePath)} /></Field></MotionItem>
                          <MotionItem><Field error={updateForm.formState.errors.buildContext?.message} hint={t('apps.buildContextHint')} label={t('apps.buildContext')}><Input {...updateForm.register('buildContext')} aria-invalid={Boolean(updateForm.formState.errors.buildContext)} /></Field></MotionItem>
                        </div>
                        <MotionItem><Field error={updateForm.formState.errors.buildLabels?.message} hint={t('apps.buildLabelsHint')} label={t('apps.buildLabels')}><Input {...updateForm.register('buildLabels')} aria-invalid={Boolean(updateForm.formState.errors.buildLabels)} placeholder={t('apps.buildLabelsPlaceholder')} /></Field></MotionItem>
                      </>
                    )
                  : (
                      <>
                        <MotionItem><Field error={updateForm.formState.errors.imageReference?.message} hint={t('apps.imageReferenceHint')} label={t('apps.imageReference')}><Input {...updateForm.register('imageReference')} aria-invalid={Boolean(updateForm.formState.errors.imageReference)} placeholder={t('apps.imageReferencePlaceholder')} /></Field></MotionItem>
                        <MotionItem><Field error={updateForm.formState.errors.targetImageRef?.message} hint={t('apps.targetImageRefHint')} label={t('apps.targetImageRef')}><Input {...updateForm.register('targetImageRef')} aria-invalid={Boolean(updateForm.formState.errors.targetImageRef)} placeholder={t('apps.targetImageRefPlaceholder')} /></Field></MotionItem>
                      </>
                    )}
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

function ApplicationBuildsPanel({ applicationId, appSlug, binding, buildContext, buildJobs, buildRuns, dockerfilePath, projectId, projectSlug, providers, registries, targetImageRef }: {
  applicationId: string
  appSlug: string
  binding?: { defaultBranch: string, gitAccountId: string, owner: string, repo: string }
  buildContext: string
  buildJobs: BuildJob[]
  buildRuns: BuildRun[]
  dockerfilePath: string
  projectId: string
  projectSlug: string
  providers: Array<{ id: string, name: string }>
  registries: ArtifactRegistry[]
  targetImageRef: string
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [branchSearch, setBranchSearch] = useState('')
  const [runSearch, setRunSearch] = useState('')
  const [logJob, setLogJob] = useState<BuildJob | null>(null)
  const form = useForm<TriggerForm>({ defaultValues: triggerDefaults, mode: 'onChange' })
  const selectedRegistry = registries.find(registry => registry.id === form.watch('targetRegistryId'))
  const targetImagePrefix = selectedRegistry ? registryInputPrefix(selectedRegistry) : ''
  const buildJobMap = useMemo(() => {
    const grouped = new Map<string, BuildJob[]>()
    for (const job of buildJobs) {
      const jobs = grouped.get(job.buildRunId) ?? []
      jobs.push(job)
      grouped.set(job.buildRunId, jobs)
    }
    for (const jobs of grouped.values()) {
      jobs.sort((left, right) => new Date(right.createdAt ?? '').getTime() - new Date(left.createdAt ?? '').getTime())
    }
    return grouped
  }, [buildJobs])
  const filteredRuns = useMemo(() => {
    const keyword = runSearch.trim().toLowerCase()
    if (!keyword)
      return buildRuns
    return buildRuns.filter((run) => {
      const target = buildRunImageRef(run)
      return [run.id, run.status, run.sourceBranch, run.sourceTag, run.sourceCommit, target].some(value => String(value ?? '').toLowerCase().includes(keyword))
    })
  }, [buildRuns, runSearch])
  const branches = useQuery({
    queryKey: ['git-branches', binding?.gitAccountId, binding?.owner, binding?.repo, branchSearch],
    queryFn: () => api.listGitBranches(binding?.gitAccountId ?? '', binding?.owner ?? '', binding?.repo ?? '', { search: branchSearch, limit: 50 }),
    enabled: Boolean(binding),
  })
  const triggerBuild = useMutation({
    mutationFn: (values: TriggerForm) => api.triggerBuildRun(projectId, { ...values, applicationId }),
    onSuccess: () => {
      toast.success(t('buildsPage.buildQueued'))
      setDialogOpen(false)
      form.reset(triggerDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-runs', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
      queryClient.invalidateQueries({ queryKey: ['application', projectId, applicationId] })
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const retryBuild = useMutation({
    mutationFn: (runId: string) => api.retryBuildRun(projectId, runId),
    onSuccess: () => {
      toast.success(t('buildsPage.retryQueued'))
      queryClient.invalidateQueries({ queryKey: ['build-runs', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const buildLog = useQuery({
    queryKey: ['build-job-log', projectId, logJob?.id],
    queryFn: () => api.getBuildJobLogs(projectId, logJob?.id ?? ''),
    enabled: Boolean(logJob),
    refetchInterval: logJob && ['queued', 'running'].includes(logJob.status) ? 2000 : false,
  })

  useEffect(() => {
    if (!dialogOpen)
      return
    form.reset({ ...triggerDefaults, applicationId, sourceBranch: binding?.defaultBranch || 'main', targetImageRef: targetImageRef || defaultTargetImageRef(undefined, projectSlug, appSlug) })
  }, [applicationId, appSlug, binding?.defaultBranch, dialogOpen, form, projectSlug, targetImageRef])

  useEffect(() => {
    if (!dialogOpen || !registries.length || form.getValues('targetRegistryId'))
      return
    const defaultRegistry = registries.find(registry => registry.credentialSet && registry.isDefault) ?? registries.find(registry => registry.credentialSet) ?? registries.find(registry => registry.isDefault) ?? registries[0]
    form.setValue('targetRegistryId', defaultRegistry.id, { shouldDirty: true, shouldValidate: true })
    if (!targetImageRef) {
      form.setValue('targetImageRef', defaultTargetImageRef(defaultRegistry, projectSlug, appSlug), { shouldDirty: true, shouldValidate: true })
    }
  }, [appSlug, dialogOpen, form, projectSlug, registries, targetImageRef])

  return (
    <div className="grid gap-4">
      {binding
        ? (
            <div className="overflow-hidden rounded-lg border border-border bg-card">
              <div className="flex flex-col gap-3 border-b border-border bg-muted/45 px-4 py-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                  <h2 className="text-base font-semibold">{t('buildsPage.workflowRunCount', { count: buildRuns.length })}</h2>
                  <p className="mt-1 text-sm text-muted-foreground">{t('buildsPage.applicationRunsDescription')}</p>
                </div>
                <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center">
                  <div className="relative min-w-0 sm:w-80">
                    <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      className="h-9 pl-9"
                      placeholder={t('buildsPage.filterRuns')}
                      value={runSearch}
                      onChange={event => setRunSearch(event.target.value)}
                    />
                  </div>
                  <Button disabled={!binding} onClick={() => setDialogOpen(true)}>
                    <Play className="size-4" />
                    {t('buildsPage.triggerBuild')}
                  </Button>
                </div>
              </div>
              {filteredRuns.length
                ? (
                    <div className="divide-y divide-border">
                      {filteredRuns.map((run) => {
                        const jobs = buildJobMap.get(run.id) ?? []
                        const latestJob = jobs[0]
                        return (
                          <BuildRunRow
                            key={run.id}
                            binding={binding}
                            jobs={jobs}
                            latestJob={latestJob}
                            run={run}
                            retrying={retryBuild.isPending}
                            onOpenLogs={job => setLogJob(job)}
                            onRetry={() => retryBuild.mutate(run.id)}
                          />
                        )
                      })}
                    </div>
                  )
                : <EmptyState title={t('buildsPage.emptyRuns')} />}
            </div>
          )
        : <EmptyState title={t('buildsPage.repositoryBindingRequired')} />}
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
            <Field hint={t('buildsPage.targetImageRefHint')} label={t('buildsPage.targetImageRef')} required>
              <TargetImageRefInput
                placeholder={t('buildsPage.targetImageRefPlaceholder')}
                prefix={targetImagePrefix}
                register={form.register('targetImageRef', { required: true })}
              />
            </Field>
            <Field hint={t('buildsPage.inheritedBuildConfigHint')} label={t('buildsPage.dockerfilePath')}>
              <Input readOnly value={dockerfilePath || 'Dockerfile'} />
            </Field>
            <Field hint={t('buildsPage.inheritedBuildConfigHint')} label={t('buildsPage.buildContext')}>
              <Input readOnly value={buildContext || '.'} />
            </Field>
            <DialogFooter><Button disabled={!form.formState.isValid || triggerBuild.isPending} type="submit">{t('buildsPage.queueBuild')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <BuildLogPanel
        job={logJob}
        log={buildLog.data}
        loading={buildLog.isFetching}
        onClose={() => setLogJob(null)}
      />
    </div>
  )
}

function BuildRunRow({ binding, jobs, latestJob, onOpenLogs, onRetry, retrying, run }: {
  binding: { defaultBranch: string, gitAccountId: string, owner: string, repo: string }
  jobs: BuildJob[]
  latestJob?: BuildJob
  onOpenLogs: (job: BuildJob) => void
  onRetry: () => void
  retrying: boolean
  run: BuildRun
}) {
  const { t } = useTranslation()
  const branch = run.sourceBranch || run.sourceTag || binding.defaultBranch || 'main'
  const targetImage = buildRunImageRef(run)
  const commit = shortCommit(run.sourceCommit)
  return (
    <div className="grid gap-3 px-4 py-4 transition-colors hover:bg-muted/35 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
      <div className="flex min-w-0 gap-3">
        <BuildRunStatusIcon status={run.status} />
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <h3 className="truncate text-sm font-semibold text-foreground" title={buildRunTitle(run, t)}>
              {buildRunTitle(run, t)}
            </h3>
            <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={run.status} />
          </div>
          <div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
            <span className="font-medium text-foreground/80">
              {binding.owner}
              /
              {binding.repo}
            </span>
            <span>
              #
              {shortBuildId(run.id)}
            </span>
            {commit && (
              <>
                <span>{t('buildsPage.commitLabel')}</span>
                <span className="font-mono text-foreground/80">{commit}</span>
              </>
            )}
          </div>
          <div className="mt-2 flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="inline-flex min-w-0 items-center gap-1 rounded-md bg-primary/10 px-2 py-0.5 font-mono text-primary">
              <GitBranch className="size-3.5 shrink-0" />
              <span className="truncate">{branch}</span>
            </span>
            <span className="inline-flex min-w-0 items-center gap-1">
              <Package className="size-3.5 shrink-0" />
              <span className="truncate" title={targetImage || t('common.none')}>{targetImage || t('common.none')}</span>
            </span>
          </div>
        </div>
      </div>
      <div className="flex items-start justify-between gap-2 lg:min-w-64">
        <div className="grid gap-1 text-sm text-muted-foreground lg:justify-items-start">
          <span className="inline-flex items-center gap-2">
            <CalendarClock className="size-4" />
            {formatBuildDate(run.createdAt)}
          </span>
          <span className="inline-flex items-center gap-2">
            <Clock3 className="size-4" />
            {latestJob
              ? t('buildsPage.latestJobSummary', { attempts: latestJob.attempts, id: shortBuildId(latestJob.id) })
              : t('buildsPage.noBuildJob')}
          </span>
          {jobs.length > 1 && <span className="text-xs">{t('buildsPage.jobCount', { count: jobs.length })}</span>}
        </div>
        <Popover>
          <PopoverTrigger asChild>
            <Button aria-label={t('buildsPage.runActions')} size="icon" variant="ghost">
              <MoreHorizontal className="size-4" />
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" className="w-44 p-1">
            <Button className="w-full justify-start gap-2" disabled={retrying} variant="ghost" onClick={onRetry}>
              <RotateCcw className="size-4" />
              {t('buildsPage.retry')}
            </Button>
            <Button className="w-full justify-start gap-2" disabled={!latestJob} variant="ghost" onClick={() => latestJob && onOpenLogs(latestJob)}>
              <ScrollText className="size-4" />
              {t('buildsPage.viewLogsStream')}
            </Button>
          </PopoverContent>
        </Popover>
      </div>
    </div>
  )
}

function BuildLogPanel({ job, loading, log, onClose }: {
  job: BuildJob | null
  loading: boolean
  log?: BuildLog
  onClose: () => void
}) {
  const { t } = useTranslation()
  if (!job)
    return null
  return (
    <div className="fixed inset-0 z-50 bg-black/20" onClick={onClose}>
      <aside
        className="absolute right-0 top-0 flex h-full w-full max-w-3xl flex-col border-l border-border bg-background shadow-xl"
        onClick={event => event.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold">{t('buildsPage.logsTitle', { id: shortBuildId(job.id) })}</h2>
            <p className="text-sm text-muted-foreground">{loading ? t('buildsPage.logsStreaming') : t('buildsPage.logsUpdated')}</p>
          </div>
          <Button aria-label={t('common.close')} size="icon" variant="ghost" onClick={onClose}>
            <X className="size-4" />
          </Button>
        </div>
        <pre className="min-h-0 flex-1 overflow-auto bg-zinc-950 p-4 font-mono text-sm leading-6 text-zinc-100">
          {log?.content || t('buildsPage.noLogs')}
        </pre>
      </aside>
    </div>
  )
}

function BuildRunStatusIcon({ status }: { status: string }) {
  if (status === 'succeeded')
    return <CircleCheck className="mt-0.5 size-5 shrink-0 text-emerald-600" />
  if (status === 'failed')
    return <CircleX className="mt-0.5 size-5 shrink-0 text-rose-600" />
  if (status === 'running')
    return <LoaderCircle className="mt-0.5 size-5 shrink-0 animate-spin text-primary" />
  return <Clock3 className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
}

function buildRunTitle(run: BuildRun, t: ReturnType<typeof useTranslation>['t']) {
  if (run.triggerType === 'webhook' || run.triggerType === 'push')
    return t('buildsPage.runTitlePush')
  if (run.triggerType === 'tag')
    return t('buildsPage.runTitleTag')
  if (run.triggerType === 'api')
    return t('buildsPage.runTitleApi')
  if (run.triggerType === 'retry')
    return t('buildsPage.runTitleRetry')
  return t('buildsPage.runTitleManual')
}

function shortCommit(value: string) {
  return value ? value.slice(0, 7) : ''
}

function shortBuildId(value: string) {
  const index = value.indexOf('_')
  if (index >= 0)
    return value.slice(index + 1, index + 9)
  return value.slice(0, 8)
}

function formatBuildDate(value: string) {
  if (!value)
    return '-'
  return new Date(value).toLocaleString(undefined, {
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    month: '2-digit',
  })
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
          { key: 'status', header: t('common.status'), render: item => <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={item.status} /> },
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
          { key: 'status', header: t('common.status'), render: item => <StatusValueBadge value={item.status} /> },
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

function registryOptionLabel(registry: ArtifactRegistry) {
  return [registry.name, registry.provider].filter(Boolean).join(' · ')
}

function registryInputPrefix(registry: ArtifactRegistry) {
  if (isDockerHubRegistry(registry))
    return ''
  const host = registryHost(registry.endpoint)
  return host ? `${host}/` : ''
}

function buildRunImageRef(run: BuildRun) {
  if (run.imageRef)
    return run.imageRef
  if (run.targetRepository)
    return `${run.targetRepository}:${run.targetTag || 'latest'}`
  return ''
}

function defaultTargetImageRef(registry: ArtifactRegistry | undefined, projectSlug: string, appSlug: string) {
  const imageName = [slugSegment(projectSlug), slugSegment(appSlug)].filter(Boolean).join('-')
  if (!imageName)
    return ''
  const namespace = registry?.namespace?.trim().replace(/^\/+|\/+$/g, '')
  return `${namespace ? `${namespace}/` : ''}${imageName}:latest`
}

function isDockerHubRegistry(registry: ArtifactRegistry) {
  if (registry.provider === 'dockerhub')
    return true
  const host = registryHost(registry.endpoint)
  return ['docker.io', 'registry-1.docker.io', 'index.docker.io'].includes(host)
}

function registryHost(endpoint: string) {
  try {
    return new URL(endpoint).host.toLowerCase()
  }
  catch {
    return endpoint.replace(/^https?:\/\//, '').replace(/\/.*$/, '').toLowerCase()
  }
}

function slugSegment(value: string) {
  return value.trim().replace(/^\/+|\/+$/g, '').toLowerCase()
}

function buildRunOptionLabel(run: BuildRun) {
  const branch = run.sourceBranch || run.sourceTag || '-'
  const image = buildRunImageRef(run) || run.targetRepository || run.id
  return `${branch} · ${run.status} · ${image}`
}

function withCurrentOption(values: string[], current?: string) {
  const options = values.map(value => ({ value, label: value }))
  const normalized = current?.trim()
  if (normalized && !options.some(option => option.value === normalized))
    options.unshift({ value: normalized, label: normalized })
  return options
}
