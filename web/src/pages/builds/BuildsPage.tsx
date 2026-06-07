import type { ArtifactRegistry, BuildJob, BuildProvider, BuildRun, BuildVariableSet } from '@/api/client'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, Play, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { EmptyState } from '@/components/common/empty-state'
import { FormField as Field } from '@/components/common/form-field'
import { ProjectSpaceSelect } from '@/components/common/project-space-select'
import { SearchSelect } from '@/components/common/search-select'
import { StatusValueBadge } from '@/components/common/status-badge'
import { TargetImageRefInput } from '@/components/common/target-image-ref-input'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'

type ProviderForm = Omit<BuildProvider, 'id' | 'createdBy' | 'createdAt'>
interface VariableSetForm {
  name: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  variablesText: string
  enabled: boolean
}
type TriggerForm = Partial<BuildRun>

const providerDefaults: ProviderForm = { config: '{}', enabled: true, name: '', ownerRef: '', scope: 'global', type: 'platform' }
const variableSetDefaults: VariableSetForm = { enabled: true, name: '', ownerRef: '', scope: 'user', variablesText: '' }
const triggerDefaults: TriggerForm = { applicationId: '', buildVariableSetIds: [], sourceBranch: '', targetImageRef: '', targetRegistryId: '', triggerType: 'manual' }
const PAGE_SIZE_OPTIONS = [10, 20, 50]

export function BuildsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('providers')
  const [selectedProjectId, setSelectedProjectId] = useState('')
  const [providerDialogOpen, setProviderDialogOpen] = useState(false)
  const [triggerDialogOpen, setTriggerDialogOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<BuildProvider | null>(null)
  const [providerToDelete, setProviderToDelete] = useState<BuildProvider | null>(null)
  const [variableSetDialogOpen, setVariableSetDialogOpen] = useState(false)
  const [editingVariableSet, setEditingVariableSet] = useState<BuildVariableSet | null>(null)
  const [variableSetToDelete, setVariableSetToDelete] = useState<BuildVariableSet | null>(null)
  const [logJob, setLogJob] = useState<BuildJob | null>(null)
  const [branchSearch, setBranchSearch] = useState('')
  const [runsPage, setRunsPage] = useState(1)
  const [runsPageSize, setRunsPageSize] = useState(10)
  const [jobsPage, setJobsPage] = useState(1)
  const [jobsPageSize, setJobsPageSize] = useState(10)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const applications = useQuery({ queryKey: ['applications', selectedProjectId], queryFn: () => api.listApplications(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const repositoryBindings = useQuery({ queryKey: ['repository-bindings', selectedProjectId], queryFn: () => api.listRepositoryBindings(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const registries = useQuery({ queryKey: ['registries', selectedProjectId], queryFn: () => api.listRegistries(selectedProjectId), enabled: Boolean(selectedProjectId) })
  const providers = useQuery({ queryKey: ['build-providers', selectedProjectId], queryFn: () => api.listBuildProviders(selectedProjectId || undefined) })
  const variableSets = useQuery({ queryKey: ['build-variable-sets', selectedProjectId], queryFn: () => api.listBuildVariableSets(selectedProjectId || undefined) })
  const runs = useQuery({
    queryKey: ['build-runs-page', selectedProjectId, runsPage, runsPageSize],
    queryFn: () => api.listBuildRunsPage(selectedProjectId, { page: runsPage, pageSize: runsPageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(selectedProjectId),
    refetchInterval: selectedProjectId ? 5000 : false,
  })
  const jobs = useQuery({
    queryKey: ['build-jobs-page', selectedProjectId, jobsPage, jobsPageSize],
    queryFn: () => api.listBuildJobsPage(selectedProjectId, { page: jobsPage, pageSize: jobsPageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(selectedProjectId),
    refetchInterval: selectedProjectId ? 5000 : false,
  })
  const selectedLog = useQuery({ queryKey: ['build-job-log', selectedProjectId, logJob?.id], queryFn: () => api.getBuildJobLogs(selectedProjectId, logJob?.id ?? ''), enabled: Boolean(selectedProjectId && logJob), refetchInterval: logJob?.status === 'running' ? 3000 : false })
  const projectOptions = useMemo(() => projects.data ?? [], [projects.data])
  const projectMap = useMemo(() => Object.fromEntries(projectOptions.map(project => [project.id, project])), [projectOptions])
  const applicationMap = useMemo(() => Object.fromEntries((applications.data ?? []).map(application => [application.id, application])), [applications.data])

  const providerForm = useForm<ProviderForm>({ defaultValues: providerDefaults, mode: 'onChange' })
  const variableSetForm = useForm<VariableSetForm>({ defaultValues: variableSetDefaults, mode: 'onChange' })
  const triggerForm = useForm<TriggerForm>({ defaultValues: triggerDefaults, mode: 'onChange' })
  const selectedApplicationId = triggerForm.watch('applicationId') ?? ''
  const selectedApplication = applicationMap[selectedApplicationId]
  const selectedRegistry = (registries.data ?? []).find(registry => registry.id === triggerForm.watch('targetRegistryId'))
  const selectedProject = projectMap[selectedProjectId]
  const targetImagePrefix = selectedRegistry ? registryInputPrefix(selectedRegistry) : ''
  const imagePreview = selectedRegistry && selectedApplication ? buildImageRefPreview(selectedRegistry, triggerForm.watch('targetImageRef') || defaultTargetImageRef(selectedRegistry, selectedProject?.slug ?? '', selectedApplication.slug)) : ''
  const selectedBinding = useMemo(
    () => (repositoryBindings.data ?? []).find(binding => binding.applicationId === selectedApplicationId),
    [repositoryBindings.data, selectedApplicationId],
  )
  const branches = useQuery({
    queryKey: ['git-branches', selectedBinding?.gitAccountId, selectedBinding?.owner, selectedBinding?.repo, branchSearch],
    queryFn: () => api.listGitBranches(selectedBinding?.gitAccountId ?? '', selectedBinding?.owner ?? '', selectedBinding?.repo ?? '', { search: branchSearch, limit: 50 }),
    enabled: Boolean(selectedBinding),
  })

  useEffect(() => {
    if (!selectedBinding)
      return
    if (!triggerForm.getValues('sourceBranch'))
      triggerForm.setValue('sourceBranch', selectedBinding.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
  }, [selectedBinding, triggerForm])

  useEffect(() => {
    if (!triggerDialogOpen || !registries.data?.length || triggerForm.getValues('targetRegistryId'))
      return
    const defaultRegistry = registries.data.find(registry => registry.credentialSet && registry.isDefault)
      ?? registries.data.find(registry => registry.credentialSet)
      ?? registries.data.find(registry => registry.isDefault)
      ?? registries.data[0]
    triggerForm.setValue('targetRegistryId', defaultRegistry.id, { shouldDirty: true, shouldValidate: true })
    if (selectedApplication && !selectedApplication.targetImageRef) {
      triggerForm.setValue('targetImageRef', defaultTargetImageRef(defaultRegistry, selectedProject?.slug ?? '', selectedApplication.slug), { shouldDirty: true, shouldValidate: true })
    }
  }, [registries.data, selectedApplication, selectedProject?.slug, triggerDialogOpen, triggerForm])

  const saveProvider = useMutation({
    mutationFn: (values: ProviderForm) => editingProvider ? api.updateBuildProvider(editingProvider.id, values) : api.createBuildProvider(values),
    onSuccess: () => {
      toast.success(t(editingProvider ? 'buildsPage.providerUpdated' : 'buildsPage.providerCreated'))
      setProviderDialogOpen(false)
      setEditingProvider(null)
      providerForm.reset(providerDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-providers'] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteProvider = useMutation({
    mutationFn: api.deleteBuildProvider,
    onSuccess: () => {
      toast.success(t('buildsPage.providerDeleted'))
      setProviderToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['build-providers'] })
    },
    onError: error => toast.error(error.message),
  })
  const saveVariableSet = useMutation({
    mutationFn: (values: VariableSetForm) => {
      const payload = {
        enabled: values.enabled,
        name: values.name,
        ownerRef: values.scope === 'project' ? values.ownerRef : '',
        scope: values.scope,
        variables: variableTextToRecord(values.variablesText),
      }
      return editingVariableSet ? api.updateBuildVariableSet(editingVariableSet.id, payload) : api.createBuildVariableSet(payload)
    },
    onSuccess: () => {
      toast.success(t(editingVariableSet ? 'buildsPage.variableSetUpdated' : 'buildsPage.variableSetCreated'))
      setVariableSetDialogOpen(false)
      setEditingVariableSet(null)
      variableSetForm.reset(variableSetDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-variable-sets'] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteVariableSet = useMutation({
    mutationFn: api.deleteBuildVariableSet,
    onSuccess: () => {
      toast.success(t('buildsPage.variableSetDeleted'))
      setVariableSetToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['build-variable-sets'] })
    },
    onError: error => toast.error(error.message),
  })
  const triggerBuild = useMutation({
    mutationFn: (values: TriggerForm) => api.triggerBuildRun(selectedProjectId, values),
    onSuccess: () => {
      toast.success(t('buildsPage.buildQueued'))
      setTriggerDialogOpen(false)
      triggerForm.reset(triggerDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-runs', selectedProjectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs', selectedProjectId] })
      queryClient.invalidateQueries({ queryKey: ['build-runs-page', selectedProjectId] })
      queryClient.invalidateQueries({ queryKey: ['build-jobs-page', selectedProjectId] })
      queryClient.invalidateQueries({ queryKey: ['applications', selectedProjectId] })
    },
    onError: error => toast.error(error.message),
  })

  function openProviderDialog(provider?: BuildProvider) {
    setEditingProvider(provider ?? null)
    providerForm.reset(provider ? { config: provider.config, enabled: provider.enabled, name: provider.name, ownerRef: provider.ownerRef, scope: provider.scope, type: provider.type } : providerDefaults)
    setProviderDialogOpen(true)
  }

  function openVariableSetDialog(set?: BuildVariableSet) {
    setEditingVariableSet(set ?? null)
    variableSetForm.reset(set
      ? { enabled: set.enabled, name: set.name, ownerRef: set.ownerRef, scope: set.scope, variablesText: variableRecordToText(set.variables) }
      : variableSetDefaults)
    setVariableSetDialogOpen(true)
  }

  return (
    <div className="grid gap-4">
      <ContentTabs
        tabs={[
          { label: t('buildsPage.providers'), value: 'providers' },
          { label: t('buildsPage.variableSets'), value: 'variables' },
          { label: t('buildsPage.runs'), value: 'runs' },
          { label: t('buildsPage.jobs'), value: 'jobs' },
        ]}
        tools={(
          <>
            <ProjectSpaceSelect
              projects={projectOptions}
              value={selectedProjectId}
              onChange={(projectId) => {
                setSelectedProjectId(projectId)
                setRunsPage(1)
                setJobsPage(1)
              }}
            />
            {activeTab === 'providers' && (
              <Button onClick={() => openProviderDialog()}>
                <Plus className="size-4" />
                {t('buildsPage.createProvider')}
              </Button>
            )}
            {activeTab === 'variables' && (
              <Button onClick={() => openVariableSetDialog()}>
                <Plus className="size-4" />
                {t('buildsPage.createVariableSet')}
              </Button>
            )}
            {activeTab === 'runs' && (
              <Button disabled={!selectedProjectId} onClick={() => setTriggerDialogOpen(true)}>
                <Play className="size-4" />
                {t('buildsPage.triggerBuild')}
              </Button>
            )}
          </>
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value="providers">
          <DataList
            columns={[
              { key: 'name', header: t('common.name'), render: item => item.name },
              { key: 'type', header: t('common.type'), render: item => item.type },
              { key: 'scope', header: t('common.scope'), render: item => item.scope === 'project' ? projectMap[item.ownerRef]?.name ?? item.ownerRef : item.scope },
              { key: 'enabled', header: t('common.status'), render: item => <StatusValueBadge value={item.enabled ? 'enabled' : 'disabled'} /> },
              { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
                <div className="flex justify-end gap-2">
                  <EditActionButton label={t('common.edit')} onClick={() => openProviderDialog(item)} />
                  <Button size="sm" variant="ghost" onClick={() => setProviderToDelete(item)}>
                    <Trash2 className="size-4" />
                    {t('common.delete')}
                  </Button>
                </div>
              ) },
            ]}
            emptyTitle={t('buildsPage.emptyProviders')}
            items={providers.data ?? []}
            rowKey={item => item.id}
            variant="plain"
          />
        </TabsContent>
        <TabsContent value="runs">
          {selectedProjectId
            ? (
                <DataList
                  columns={[
                    { key: 'id', header: t('common.id'), render: item => item.id },
                    { key: 'status', header: t('common.status'), render: item => <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={item.status} /> },
                    { key: 'target', header: t('buildsPage.targetImage'), render: item => item.imageRef || `${item.targetRepository}:${item.targetTag}` },
                    { key: 'commit', header: t('buildsPage.sourceCommit'), render: item => item.sourceCommit || '-' },
                  ]}
                  emptyTitle={t('buildsPage.emptyRuns')}
                  items={runs.data?.items ?? []}
                  pagination={{
                    page: runs.data?.page ?? runsPage,
                    pageSize: runs.data?.pageSize ?? runsPageSize,
                    pageSizeOptions: PAGE_SIZE_OPTIONS,
                    total: runs.data?.total ?? 0,
                    totalPages: runs.data?.totalPages ?? 0,
                    pageInfoLabel: t('pagination.pageInfo', {
                      page: runs.data?.page ?? runsPage,
                      totalPages: runs.data?.totalPages ?? 0,
                      total: runs.data?.total ?? 0,
                    }),
                    onPageChange: setRunsPage,
                    onPageSizeChange: (nextPageSize) => {
                      setRunsPageSize(nextPageSize)
                      setRunsPage(1)
                    },
                  }}
                  rowKey={item => item.id}
                  variant="plain"
                />
              )
            : <EmptyState title={t('buildsPage.selectProject')} />}
        </TabsContent>
        <TabsContent value="jobs">
          <DataList
            columns={[
              { key: 'id', header: t('common.id'), render: item => item.id },
              { key: 'run', header: t('buildsPage.buildRun'), render: item => item.buildRunId },
              { key: 'status', header: t('common.status'), render: item => <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={item.status} /> },
              { key: 'attempts', header: t('buildsPage.attempts'), render: item => item.attempts },
              { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
                <div className="flex justify-end">
                  <Button size="sm" variant="ghost" onClick={() => setLogJob(item)}>
                    <Eye className="size-4" />
                    {t('buildsPage.viewLogs')}
                  </Button>
                </div>
              ) },
            ]}
            emptyTitle={t('buildsPage.emptyJobs')}
            items={jobs.data?.items ?? []}
            pagination={{
              page: jobs.data?.page ?? jobsPage,
              pageSize: jobs.data?.pageSize ?? jobsPageSize,
              pageSizeOptions: PAGE_SIZE_OPTIONS,
              total: jobs.data?.total ?? 0,
              totalPages: jobs.data?.totalPages ?? 0,
              pageInfoLabel: t('pagination.pageInfo', {
                page: jobs.data?.page ?? jobsPage,
                totalPages: jobs.data?.totalPages ?? 0,
                total: jobs.data?.total ?? 0,
              }),
              onPageChange: setJobsPage,
              onPageSizeChange: (nextPageSize) => {
                setJobsPageSize(nextPageSize)
                setJobsPage(1)
              },
            }}
            rowKey={item => item.id}
            variant="plain"
          />
        </TabsContent>
        <TabsContent value="variables">
          <DataList
            columns={[
              { key: 'name', header: t('common.name'), render: item => item.name },
              { key: 'scope', header: t('common.scope'), render: item => item.scope === 'project' ? projectMap[item.ownerRef]?.name ?? item.ownerRef : t(`codeRepositoriesView.scope${capitalizeScope(item.scope)}`) },
              { key: 'variables', header: t('buildsPage.variables'), render: item => t('buildsPage.variableCount', { count: variableCount(item.variables) }) },
              { key: 'enabled', header: t('common.status'), render: item => <StatusValueBadge value={item.enabled ? 'enabled' : 'disabled'} /> },
              { key: 'actions', header: t('common.actions'), className: 'text-right whitespace-nowrap', render: item => (
                <div className="flex justify-end gap-2">
                  <EditActionButton label={t('common.edit')} onClick={() => openVariableSetDialog(item)} />
                  <Button size="sm" variant="ghost" onClick={() => setVariableSetToDelete(item)}>
                    <Trash2 className="size-4" />
                    {t('common.delete')}
                  </Button>
                </div>
              ) },
            ]}
            emptyTitle={t('buildsPage.emptyVariableSets')}
            items={variableSets.data ?? []}
            rowKey={item => item.id}
            variant="plain"
          />
        </TabsContent>
      </ContentTabs>

      <Dialog open={providerDialogOpen} onOpenChange={setProviderDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingProvider ? t('buildsPage.editProvider') : t('buildsPage.createProvider')}</DialogTitle>
            <DialogDescription>{t('buildsPage.providerDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={providerForm.handleSubmit(values => saveProvider.mutate(values))}>
            <Field label={t('common.name')} required><Input {...providerForm.register('name', { required: true })} /></Field>
            <Field label={t('common.type')}>
              <Select {...providerForm.register('type')}>
                <option value="platform">{t('buildsPage.typePlatform')}</option>
              </Select>
            </Field>
            <Field label={t('common.scope')}>
              <Select {...providerForm.register('scope')}>
                <option value="global">{t('codeRepositoriesView.scopeGlobal')}</option>
                <option value="project">{t('codeRepositoriesView.scopeProject')}</option>
                <option value="user">{t('codeRepositoriesView.scopeUser')}</option>
              </Select>
            </Field>
            {providerForm.watch('scope') === 'project' && (
              <Field label={t('projectSpaces.title')} required>
                <Select {...providerForm.register('ownerRef', { required: true })}>
                  <option value="">{t('common.select')}</option>
                  {projectOptions.map(project => <option key={project.id} value={project.id}>{project.name}</option>)}
                </Select>
              </Field>
            )}
            <Field label={t('buildsPage.config')}><Input {...providerForm.register('config')} /></Field>
            <DialogFooter><Button disabled={!providerForm.formState.isValid || saveProvider.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={variableSetDialogOpen} onOpenChange={setVariableSetDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingVariableSet ? t('buildsPage.editVariableSet') : t('buildsPage.createVariableSet')}</DialogTitle>
            <DialogDescription>{t('buildsPage.variableSetDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={variableSetForm.handleSubmit(values => saveVariableSet.mutate(values))}>
            <Field label={t('common.name')} required><Input {...variableSetForm.register('name', { required: true })} /></Field>
            <Field label={t('common.scope')}>
              <Select {...variableSetForm.register('scope')}>
                <option value="global">{t('codeRepositoriesView.scopeGlobal')}</option>
                <option value="project">{t('codeRepositoriesView.scopeProject')}</option>
                <option value="user">{t('codeRepositoriesView.scopeUser')}</option>
              </Select>
            </Field>
            {variableSetForm.watch('scope') === 'project' && (
              <Field label={t('projectSpaces.title')} required>
                <Select {...variableSetForm.register('ownerRef', { required: true })}>
                  <option value="">{t('common.select')}</option>
                  {projectOptions.map(project => <option key={project.id} value={project.id}>{project.name}</option>)}
                </Select>
              </Field>
            )}
            <Field hint={t('buildsPage.variablesHint')} label={t('buildsPage.variables')} required>
              <textarea
                className="min-h-36 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none transition focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/20"
                {...variableSetForm.register('variablesText', { required: true })}
              />
            </Field>
            <label className="flex items-center gap-2 text-sm text-foreground">
              <input className="size-4 accent-primary" type="checkbox" {...variableSetForm.register('enabled')} />
              {t('common.enabled')}
            </label>
            <DialogFooter><Button disabled={!variableSetForm.formState.isValid || saveVariableSet.isPending} type="submit">{t('common.save')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={triggerDialogOpen} onOpenChange={setTriggerDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('buildsPage.triggerBuild')}</DialogTitle>
            <DialogDescription>{t('buildsPage.triggerDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={triggerForm.handleSubmit(values => triggerBuild.mutate(values))}>
            <Field label={t('apps.title')} required>
              <Select
                {...triggerForm.register('applicationId', {
                  required: true,
                  onChange: (event: { target: { value: string } }) => {
                    triggerForm.setValue('sourceBranch', '', { shouldDirty: true, shouldValidate: true })
                    const application = applicationMap[event.target.value]
                    triggerForm.setValue('targetImageRef', application?.targetImageRef || defaultTargetImageRef(selectedRegistry, selectedProject?.slug ?? '', application?.slug ?? ''), { shouldDirty: true, shouldValidate: true })
                    setBranchSearch('')
                  },
                })}
              >
                <option value="">{t('common.select')}</option>
                {(applications.data ?? []).map(app => <option key={app.id} value={app.id}>{app.name}</option>)}
              </Select>
            </Field>
            <Field label={t('repositories.defaultBranch')} required>
              <SearchSelect
                disabled={!selectedBinding}
                emptyLabel={selectedBinding ? t('common.noOptions') : t('buildsPage.repositoryBindingRequired')}
                limited={branches.data?.limited}
                loading={branches.isFetching}
                options={branchOptions(branches.data?.items ?? [], triggerForm.watch('sourceBranch'))}
                placeholder={t('repositories.defaultBranchPlaceholder')}
                search={branchSearch}
                value={triggerForm.watch('sourceBranch') || ''}
                onSearchChange={setBranchSearch}
                onValueChange={value => triggerForm.setValue('sourceBranch', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
            <Field label={t('buildsPage.provider')}>
              <Select {...triggerForm.register('buildProviderId')}>
                <option value="">{t('common.none')}</option>
                {(providers.data ?? []).map(provider => <option key={provider.id} value={provider.id}>{provider.name}</option>)}
              </Select>
            </Field>
            <Field hint={t('buildsPage.variableSetsHint')} label={t('buildsPage.variableSets')}>
              <div className="grid gap-2 rounded-md border border-border p-3">
                {(variableSets.data ?? []).length > 0
                  ? (variableSets.data ?? []).map(set => (
                      <label key={set.id} className="flex items-center justify-between gap-3 rounded-md px-2 py-1 text-sm transition hover:bg-muted/60">
                        <span className="grid">
                          <span className="font-medium text-foreground">{set.name}</span>
                          <span className="text-xs text-muted-foreground">{t('buildsPage.variableCount', { count: variableCount(set.variables) })}</span>
                        </span>
                        <input
                          checked={selectedBuildVariableSetIds(triggerForm.watch('buildVariableSetIds')).includes(set.id)}
                          className="size-4 accent-primary"
                          type="checkbox"
                          onChange={(event) => {
                            const current = selectedBuildVariableSetIds(triggerForm.getValues('buildVariableSetIds'))
                            triggerForm.setValue('buildVariableSetIds', event.target.checked ? [...current, set.id] : current.filter(id => id !== set.id), { shouldDirty: true, shouldValidate: true })
                          }}
                        />
                      </label>
                    ))
                  : <span className="text-sm text-muted-foreground">{t('buildsPage.emptyVariableSets')}</span>}
              </div>
            </Field>
            <Field label={t('buildsPage.targetRegistry')} required>
              <Select {...triggerForm.register('targetRegistryId', { required: true })}>
                <option value="">{t('common.select')}</option>
                {(registries.data ?? []).map(registry => <option key={registry.id} value={registry.id}>{registryOptionLabel(registry)}</option>)}
              </Select>
            </Field>
            <Field hint={t('buildsPage.targetImageRefHint')} label={t('buildsPage.targetImageRef')} required>
              <TargetImageRefInput
                placeholder={t('buildsPage.targetImageRefPlaceholder')}
                prefix={targetImagePrefix}
                register={triggerForm.register('targetImageRef', { required: true })}
              />
            </Field>
            {imagePreview && (
              <Field hint={t('buildsPage.targetImagePreviewHint')} label={t('buildsPage.targetImagePreview')}>
                <Input readOnly value={imagePreview} />
              </Field>
            )}
            <Field hint={t('buildsPage.inheritedBuildConfigHint')} label={t('buildsPage.dockerfilePath')}>
              <Input readOnly value={selectedApplication?.dockerfilePath || 'Dockerfile'} />
            </Field>
            <Field hint={t('buildsPage.inheritedBuildConfigHint')} label={t('buildsPage.buildContext')}>
              <Input readOnly value={selectedApplication?.buildContext || '.'} />
            </Field>
            <DialogFooter><Button disabled={!selectedProjectId || !triggerForm.formState.isValid || triggerBuild.isPending} type="submit">{t('buildsPage.queueBuild')}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('buildsPage.deleteProviderDescription')}
        open={Boolean(providerToDelete)}
        title={t('buildsPage.deleteProviderTitle')}

        onConfirm={() => providerToDelete && deleteProvider.mutate(providerToDelete.id)}
        onOpenChange={open => !open && setProviderToDelete(null)}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('buildsPage.deleteVariableSetDescription')}
        open={Boolean(variableSetToDelete)}
        title={t('buildsPage.deleteVariableSetTitle')}
        onConfirm={() => variableSetToDelete && deleteVariableSet.mutate(variableSetToDelete.id)}
        onOpenChange={open => !open && setVariableSetToDelete(null)}
      />
      <Dialog open={Boolean(logJob)} onOpenChange={open => !open && setLogJob(null)}>
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t('buildsPage.buildLogs')}</DialogTitle>
            <DialogDescription>{logJob?.id}</DialogDescription>
          </DialogHeader>
          <pre className="max-h-[60vh] overflow-auto rounded-md border border-border bg-muted p-3 text-xs leading-relaxed text-foreground">
            {selectedLog.data?.content || t('buildsPage.emptyLogs')}
          </pre>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function branchOptions(branches: Array<{ name: string }>, current?: string) {
  return withCurrentOption(branches.map(branch => branch.name), current)
}

function registryOptionLabel(registry: ArtifactRegistry) {
  return [
    registry.name,
    registry.provider,
  ].filter(Boolean).join(' · ')
}

function registryInputPrefix(registry: ArtifactRegistry) {
  if (isDockerHubRegistry(registry))
    return ''
  const host = registryHost(registry.endpoint)
  return host ? `${host}/` : ''
}

function buildImageRefPreview(registry: ArtifactRegistry, targetImageRef: string) {
  const { repository, tag } = splitTargetImageRef(targetImageRef)
  if (!repository)
    return ''
  if (hasRegistryHost(repository) || isDockerHubRegistry(registry))
    return `${repository}:${tag}`
  const host = registryHost(registry.endpoint)
  return host ? `${host}/${repository}:${tag}` : `${repository}:${tag}`
}

function splitTargetImageRef(value: string) {
  const normalized = value.trim().replace(/^\/+|\/+$/g, '')
  const lastSlash = normalized.lastIndexOf('/')
  const lastColon = normalized.lastIndexOf(':')
  if (lastColon > lastSlash) {
    return {
      repository: normalized.slice(0, lastColon).replace(/^\/+|\/+$/g, ''),
      tag: normalized.slice(lastColon + 1).trim() || 'latest',
    }
  }
  return { repository: normalized, tag: 'latest' }
}

function defaultTargetImageRef(registry: ArtifactRegistry | undefined, projectSlug: string, appSlug: string) {
  const imageName = [slugSegment(projectSlug), slugSegment(appSlug)].filter(Boolean).join('-')
  if (!imageName)
    return ''
  const namespace = registry?.namespace?.trim().replace(/^\/+|\/+$/g, '')
  return `${namespace ? `${namespace}/` : ''}${imageName}:latest`
}

function hasRegistryHost(repository: string) {
  const first = repository.trim().replace(/^\/+|\/+$/g, '').split('/')[0] ?? ''
  return first === 'localhost' || first.includes('.') || first.includes(':')
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

function variableTextToRecord(value: string) {
  return Object.fromEntries(
    value
      .split('\n')
      .map(line => line.trim())
      .filter(Boolean)
      .map((line) => {
        const index = line.indexOf('=')
        if (index < 0)
          return [line, '']
        return [line.slice(0, index).trim(), line.slice(index + 1).trim()]
      })
      .filter(([key]) => Boolean(key)),
  )
}

function variableRecordToText(value: BuildVariableSet['variables']) {
  const variables = typeof value === 'string' ? parseVariableRecord(value) : value
  return Object.entries(variables ?? {}).map(([key, content]) => `${key}=${content}`).join('\n')
}

function parseVariableRecord(value: string) {
  try {
    const parsed = JSON.parse(value)
    return typeof parsed === 'object' && parsed ? parsed as Record<string, string> : {}
  }
  catch {
    return {}
  }
}

function variableCount(value: BuildVariableSet['variables']) {
  const variables = typeof value === 'string' ? parseVariableRecord(value) : value
  return Object.keys(variables ?? {}).length
}

function selectedBuildVariableSetIds(value: BuildRun['buildVariableSetIds'] | undefined) {
  if (Array.isArray(value))
    return value
  if (!value)
    return []
  try {
    const parsed = JSON.parse(value)
    return Array.isArray(parsed) ? parsed.filter(item => typeof item === 'string') : []
  }
  catch {
    return value.split(',').map(item => item.trim()).filter(Boolean)
  }
}

function capitalizeScope(scope: string) {
  return scope.charAt(0).toUpperCase() + scope.slice(1)
}

function withCurrentOption(values: string[], current?: string) {
  const options = values.map(value => ({ value, label: value }))
  const normalized = current?.trim()
  if (normalized && !options.some(option => option.value === normalized))
    options.unshift({ value: normalized, label: normalized })
  return options
}
