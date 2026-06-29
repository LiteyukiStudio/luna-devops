import type { Ref } from 'react'
import type { ArtifactRegistry, BuildJob, BuildRun, DeploymentTarget, RepositoryBinding } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { useEffect, useImperativeHandle, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api, buildJobLogsStreamUrl } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { FormField as Field } from '@/components/common/form-field'
import { PaginationController } from '@/components/common/pagination'
import { SearchSelect } from '@/components/common/search-select'
import { TargetImageRefInput } from '@/components/common/target-image-ref-input'
import { UnitInput } from '@/components/common/unit-input'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { useBillingDisplay } from '@/lib/billing-display'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { defaultBuildCpuRequest, defaultBuildMemoryRequest } from './application-build-defaults'
import { ApplicationBuildLogPanel } from './application-build-log-panel'
import { ApplicationBuildRunFilterBar } from './application-build-run-filter-bar'
import { ApplicationBuildRunRow } from './application-build-run-row'
import { branchOptions, defaultTargetImageRef, deploymentTargetImageRef, firstSelectableDeploymentTarget, registryInputPrefix, registryOptionLabel } from './application-config-utils'

export interface BuildsPanelHandle {
  openTriggerDrawer: () => void
}

type TriggerForm = Partial<BuildRun>

const triggerDefaults: TriggerForm = { applicationId: '', buildEnvironmentId: '', buildCpuRequest: defaultBuildCpuRequest, buildMemoryRequest: defaultBuildMemoryRequest, deploymentTargetId: '', sourceBranch: '', targetImageRef: '', targetRegistryId: '', triggerType: 'manual' }

function uniqueBuildFilterValues(values: Array<string | undefined>) {
  return [...new Set(values.map(value => value?.trim()).filter((value): value is string => Boolean(value)))]
    .sort((left, right) => left.localeCompare(right))
}

export function ApplicationBuildsPanel({ applicationId, appSlug, binding, deploymentTargets, buildJobs, buildRuns, projectId, projectSlug, ref, registries, repositoryBindings }: {
  applicationId: string
  appSlug: string
  binding?: { defaultBranch: string, gitAccountId: string, owner: string, repo: string }
  repositoryBindings: RepositoryBinding[]
  deploymentTargets: DeploymentTarget[]
  buildJobs: BuildJob[]
  buildRuns: BuildRun[]
  projectId: string
  projectSlug: string
  ref?: Ref<BuildsPanelHandle>
  registries: ArtifactRegistry[]
}) {
  const { i18n, t } = useTranslation()
  const queryClient = useQueryClient()
  const billingDisplay = useBillingDisplay(i18n.language)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [branchSearch, setBranchSearch] = useState('')
  const [runSearch, setRunSearch] = useState('')
  const [runsPage, setRunsPage] = useState(1)
  const [runsPageSize, setRunsPageSize] = useState(10)
  const [eventFilter, setEventFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [branchFilter, setBranchFilter] = useState('')
  const [actorFilter, setActorFilter] = useState('')
  const [logJob, setLogJob] = useState<BuildJob | null>(null)
  const [logContent, setLogContent] = useState('')
  const [logStreaming, setLogStreaming] = useState(false)
  const form = useForm<TriggerForm>({ defaultValues: triggerDefaults, mode: 'onChange' })
  const selectedDeploymentTarget = deploymentTargets.find(config => config.id === form.watch('deploymentTargetId')) ?? firstSelectableDeploymentTarget(deploymentTargets)
  const selectedBinding = repositoryBindings.find(item => item.id === selectedDeploymentTarget?.repositoryBindingId) ?? binding
  const selectedRegistry = registries.find(registry => registry.id === form.watch('targetRegistryId'))
  const targetImagePrefix = selectedRegistry ? registryInputPrefix(selectedRegistry) : ''
  const buildMinuteCost = billingDisplay.buildMinuteCost(form.watch('buildCpuRequest'), form.watch('buildMemoryRequest'))
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
  const buildRunsPage = useQuery({
    queryKey: ['build-runs-page', projectId, applicationId, runsPage, runsPageSize, runSearch, eventFilter, statusFilter, branchFilter, actorFilter],
    queryFn: () => api.listBuildRunsPage(projectId, {
      applicationId,
      createdBy: actorFilter || undefined,
      page: runsPage,
      pageSize: runsPageSize,
      search: runSearch.trim() || undefined,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      sourceBranch: branchFilter || undefined,
      status: statusFilter ? statusFilter as BuildRun['status'] : undefined,
      triggerType: eventFilter ? eventFilter as BuildRun['triggerType'] : undefined,
    }),
    enabled: Boolean(projectId && applicationId),
    refetchInterval: projectId && applicationId ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const pagedRuns = buildRunsPage.data?.items ?? []
  const runsTotal = buildRunsPage.data?.total ?? 0
  const branchFilterOptions = useMemo(() => uniqueBuildFilterValues([
    selectedBinding?.defaultBranch,
    ...buildRuns.map(run => run.sourceBranch),
    branchFilter,
  ]), [selectedBinding?.defaultBranch, branchFilter, buildRuns])
  const actorFilterOptions = useMemo(() => uniqueBuildFilterValues([
    ...buildRuns.map(run => run.createdBy),
    actorFilter,
  ]), [actorFilter, buildRuns])
  const updateRunSearch = (value: string) => {
    setRunSearch(value)
    setRunsPage(1)
  }
  const updateEventFilter = (value: string) => {
    setEventFilter(value)
    setRunsPage(1)
  }
  const updateStatusFilter = (value: string) => {
    setStatusFilter(value)
    setRunsPage(1)
  }
  const updateBranchFilter = (value: string) => {
    setBranchFilter(value)
    setRunsPage(1)
  }
  const updateActorFilter = (value: string) => {
    setActorFilter(value)
    setRunsPage(1)
  }
  const branches = useQuery({
    queryKey: ['git-branches', selectedBinding?.gitAccountId, selectedBinding?.owner, selectedBinding?.repo, branchSearch],
    queryFn: () => api.listGitBranches(selectedBinding?.gitAccountId ?? '', selectedBinding?.owner ?? '', selectedBinding?.repo ?? '', { search: branchSearch, limit: 50 }),
    enabled: Boolean(selectedBinding),
  })
  const triggerBuild = useMutation({
    mutationFn: (values: TriggerForm) => api.triggerBuildRun(projectId, { ...values, applicationId }),
    onSuccess: () => {
      toast.success(t('buildsPage.buildQueued'))
      setDialogOpen(false)
      form.reset(triggerDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-runs', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-runs-page', projectId] })
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
      queryClient.invalidateQueries({ queryKey: ['build-runs-page', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const cancelBuild = useMutation({
    mutationFn: (runId: string) => api.cancelBuildRun(projectId, runId),
    onSuccess: () => {
      toast.success(t('buildsPage.cancelled'))
      queryClient.invalidateQueries({ queryKey: ['build-runs', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-runs-page', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteBuild = useMutation({
    mutationFn: (runId: string) => api.deleteBuildRun(projectId, runId),
    onSuccess: () => {
      toast.success(t('buildsPage.deleted'))
      queryClient.invalidateQueries({ queryKey: ['build-runs', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-runs-page', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
    },
    onError: error => toast.error(error.message),
  })
  useImperativeHandle(ref, () => ({
    openTriggerDrawer: () => setDialogOpen(true),
  }))
  useEffect(() => {
    const logJobId = logJob?.id
    if (!logJobId) {
      setLogContent('')
      setLogStreaming(false)
      return
    }
    setLogContent('')
    setLogStreaming(true)
    const stream = new EventSource(buildJobLogsStreamUrl(projectId, logJobId, 0), { withCredentials: true })
    const handleChunk = (event: Event) => {
      try {
        const payload = JSON.parse((event as MessageEvent).data) as { content?: string }
        if (payload.content)
          setLogContent(current => current + payload.content)
      }
      catch {
      }
    }
    const handleDone = () => {
      setLogStreaming(false)
      stream.close()
      queryClient.invalidateQueries({ queryKey: ['build-runs-page', projectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', projectId] })
    }
    stream.addEventListener('chunk', handleChunk)
    stream.addEventListener('done', handleDone)
    stream.onerror = () => {
      setLogStreaming(false)
      stream.close()
    }
    return () => {
      stream.removeEventListener('chunk', handleChunk)
      stream.removeEventListener('done', handleDone)
      stream.close()
    }
  }, [logJob, projectId, queryClient])

  useEffect(() => {
    if (!dialogOpen)
      return
    const defaultConfig = firstSelectableDeploymentTarget(deploymentTargets)
    form.reset({
      ...triggerDefaults,
      applicationId,
      deploymentTargetId: defaultConfig?.id ?? '',
      buildEnvironmentId: defaultConfig?.buildEnvironmentId || '',
      buildCpuRequest: defaultConfig?.buildCpuRequest || defaultBuildCpuRequest,
      buildMemoryRequest: defaultConfig?.buildMemoryRequest || defaultBuildMemoryRequest,
      sourceBranch: (repositoryBindings.find(item => item.id === defaultConfig?.repositoryBindingId) ?? binding)?.defaultBranch || 'main',
      targetImageRef: deploymentTargetImageRef(defaultConfig) || defaultTargetImageRef(undefined, projectSlug, appSlug),
      targetRegistryId: defaultConfig?.targetRegistryId ?? '',
    })
  }, [applicationId, appSlug, binding, deploymentTargets, dialogOpen, form, projectSlug, repositoryBindings])

  useEffect(() => {
    if (!dialogOpen || !selectedDeploymentTarget)
      return
    const nextBinding = repositoryBindings.find(item => item.id === selectedDeploymentTarget.repositoryBindingId) ?? binding
    if (nextBinding)
      form.setValue('sourceBranch', nextBinding.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
    if (selectedDeploymentTarget.targetRegistryId)
      form.setValue('targetRegistryId', selectedDeploymentTarget.targetRegistryId, { shouldDirty: true, shouldValidate: true })
    const configTargetImageRef = deploymentTargetImageRef(selectedDeploymentTarget)
    if (configTargetImageRef)
      form.setValue('targetImageRef', configTargetImageRef, { shouldDirty: true, shouldValidate: true })
    form.setValue('buildEnvironmentId', selectedDeploymentTarget.buildEnvironmentId || '', { shouldDirty: true, shouldValidate: true })
    form.setValue('buildCpuRequest', selectedDeploymentTarget.buildCpuRequest || defaultBuildCpuRequest, { shouldDirty: true, shouldValidate: true })
    form.setValue('buildMemoryRequest', selectedDeploymentTarget.buildMemoryRequest || defaultBuildMemoryRequest, { shouldDirty: true, shouldValidate: true })
  }, [binding, dialogOpen, form, repositoryBindings, selectedDeploymentTarget])

  useEffect(() => {
    if (!dialogOpen || !registries.length || form.getValues('targetRegistryId'))
      return
    const defaultRegistry = registries.find(registry => registry.credentialSet && registry.isDefault) ?? registries.find(registry => registry.credentialSet) ?? registries.find(registry => registry.isDefault) ?? registries[0]
    form.setValue('targetRegistryId', defaultRegistry.id, { shouldDirty: true, shouldValidate: true })
    if (!form.getValues('targetImageRef')) {
      form.setValue('targetImageRef', defaultTargetImageRef(defaultRegistry, projectSlug, appSlug), { shouldDirty: true, shouldValidate: true })
    }
  }, [appSlug, dialogOpen, form, projectSlug, registries])

  return (
    <div className="grid gap-4">
      {repositoryBindings.length || binding
        ? (
            <div className="overflow-hidden rounded-lg border border-border bg-card">
              <div className="flex flex-col gap-3 border-b border-border bg-muted/45 px-4 py-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                  <h2 className="text-base font-semibold">{t('buildsPage.workflowRunCount', { count: runsTotal })}</h2>
                  <p className="mt-1 text-sm text-muted-foreground">{t('buildsPage.applicationRunsDescription')}</p>
                </div>
                <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center">
                  <div className="relative min-w-0 sm:w-80">
                    <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      className="h-9 pl-9"
                      placeholder={t('buildsPage.filterRuns')}
                      value={runSearch}
                      onChange={event => updateRunSearch(event.target.value)}
                    />
                  </div>
                </div>
              </div>
              <ApplicationBuildRunFilterBar
                actor={actorFilter}
                actorOptions={actorFilterOptions}
                branch={branchFilter}
                branchOptions={branchFilterOptions}
                event={eventFilter}
                status={statusFilter}
                onActorChange={updateActorFilter}
                onBranchChange={updateBranchFilter}
                onEventChange={updateEventFilter}
                onStatusChange={updateStatusFilter}
              />
              {pagedRuns.length
                ? (
                    <div className="divide-y divide-border">
                      {pagedRuns.map((run) => {
                        const jobs = buildJobMap.get(run.id) ?? []
                        const latestJob = jobs[0]
                        const config = deploymentTargets.find(config => config.id === run.deploymentTargetId)
                        const rowBinding = repositoryBindings.find(binding => binding.id === config?.repositoryBindingId) ?? binding
                        if (!rowBinding)
                          return null
                        return (
                          <ApplicationBuildRunRow
                            key={run.id}
                            binding={rowBinding}
                            deploymentTargetName={config?.name}
                            jobs={jobs}
                            latestJob={latestJob}
                            run={run}
                            canceling={cancelBuild.isPending}
                            deleting={deleteBuild.isPending}
                            retrying={retryBuild.isPending}
                            onCancel={() => cancelBuild.mutate(run.id)}
                            onDelete={() => deleteBuild.mutate(run.id)}
                            onOpenLogs={job => setLogJob(job)}
                            onRetry={() => retryBuild.mutate(run.id)}
                          />
                        )
                      })}
                    </div>
                  )
                : <EmptyState title={t('buildsPage.emptyRuns')} variant="plain" />}
              <div className="border-t border-border px-4 py-4">
                <PaginationController
                  initialPage={runsPage}
                  pageSize={runsPageSize}
                  pageSizeOptions={[10, 20, 50]}
                  total={runsTotal}
                  onPageChange={setRunsPage}
                  onPageSizeChange={(pageSize) => {
                    setRunsPageSize(pageSize)
                    setRunsPage(1)
                  }}
                />
              </div>
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
            <Field hint={t('buildsPage.buildConfigHint')} label={t('buildsPage.buildConfig')} required>
              <Select {...form.register('deploymentTargetId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {deploymentTargets.map(config => <option key={config.id} value={config.id}>{config.name}</option>)}
              </Select>
            </Field>
            <Field label={t('repositories.defaultBranch')} required>
              <SearchSelect
                disabled={!selectedBinding}
                emptyLabel={selectedBinding ? t('common.noOptions') : t('buildsPage.repositoryBindingRequired')}
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
            <Field hint={t('buildsPage.inheritedModuleHint')} label={t('buildsPage.dockerfilePath')}>
              <Input readOnly value={selectedDeploymentTarget?.dockerfilePath || 'Dockerfile'} />
            </Field>
            <Field hint={t('buildsPage.inheritedModuleHint')} label={t('buildsPage.buildContext')}>
              <Input readOnly value={selectedDeploymentTarget?.buildContext || '.'} />
            </Field>
            <div className="grid gap-3">
              <div>
                <h3 className="text-sm font-semibold">{t('deploymentsPage.buildEnvironment')}</h3>
                <p className="mt-1 text-sm text-muted-foreground">{t('buildsPage.buildEnvironmentOverrideHint')}</p>
              </div>
              <div className="grid gap-3 md:grid-cols-2">
                <Field label={t('deploymentsPage.buildCpuRequest')} required>
                  <UnitInput
                    unitSelectLabel={t('deploymentsPage.buildCpuRequest')}
                    units={[
                      { label: 'm', value: 'm' },
                      { label: t('deploymentsPage.cpuUnits.core'), value: '' },
                    ]}
                    value={form.watch('buildCpuRequest') || defaultBuildCpuRequest}
                    onChange={value => form.setValue('buildCpuRequest', value, { shouldDirty: true, shouldValidate: true })}
                  />
                </Field>
                <Field label={t('deploymentsPage.buildMemoryRequest')} required>
                  <UnitInput
                    unitSelectLabel={t('deploymentsPage.buildMemoryRequest')}
                    units={[
                      { label: 'Mi', value: 'Mi' },
                      { label: 'Gi', value: 'Gi' },
                    ]}
                    value={form.watch('buildMemoryRequest') || defaultBuildMemoryRequest}
                    onChange={value => form.setValue('buildMemoryRequest', value, { shouldDirty: true, shouldValidate: true })}
                  />
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('deploymentsPage.buildEstimatedPrice', { price: billingDisplay.formatAmountWithUnit(buildMinuteCost) })}
                  </p>
                </Field>
              </div>
            </div>
            <DialogFooter><Button disabled={!form.formState.isValid || triggerBuild.isPending} type="submit">{t('buildsPage.queueBuild')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <ApplicationBuildLogPanel
        job={logJob}
        content={logContent}
        loading={logStreaming}
        onClose={() => setLogJob(null)}
      />
    </div>
  )
}
