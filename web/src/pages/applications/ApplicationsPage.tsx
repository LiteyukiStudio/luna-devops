import type { Ref } from 'react'
import type { Application, GitAccount, GitProvider, GitRepository, GitRepositoryBuildOptions, RepositoryBinding } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Box, GitBranch, Plus, Save, Search, Trash2 } from 'lucide-react'
import { useEffect, useImperativeHandle, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { EditActionButton } from '@/components/common/edit-action-button'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { MotionItem, MotionList } from '@/components/common/motion'
import { PageHeader } from '@/components/common/page-header'
import { SearchSelect } from '@/components/common/search-select'
import { Alert as UiAlert, AlertDescription as UiAlertDescription, AlertTitle as UiAlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

const schema = z.object({
  name: z.string().min(1, i18next.t('apps.nameRequired')),
  slug: z.string().min(1, i18next.t('apps.slugRequired')).regex(/^[a-z0-9-]+$/, i18next.t('common.lowercaseSlugOnly')),
  sourceType: z.enum(['repository', 'image']),
  repositoryUrl: z.string().optional(),
  imageReference: z.string().optional(),
  dockerfilePath: z.string().optional(),
  buildContext: z.string().optional(),
  servicePort: z.coerce.number().int(i18next.t('apps.integerPort')).positive(i18next.t('apps.positivePort')),
  gitAccountId: z.string().optional(),
  repositoryOwner: z.string().optional(),
  repositoryName: z.string().optional(),
  cloneUrl: z.string().optional(),
  defaultBranch: z.string().optional(),
  webhookStatus: z.enum(['pending', 'created', 'disabled', 'failed']).default('pending'),
}).superRefine((value, ctx) => {
  if (value.sourceType !== 'repository')
    return

  if (!value.gitAccountId?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      message: i18next.t('repositories.gitAccountRequired'),
      path: ['gitAccountId'],
    })
  }

  if (!value.repositoryOwner?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      message: i18next.t('repositories.ownerRequired'),
      path: ['repositoryOwner'],
    })
  }

  if (!value.repositoryName?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      message: i18next.t('repositories.repoRequired'),
      path: ['repositoryName'],
    })
  }

  if (!value.dockerfilePath?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      message: i18next.t('apps.dockerfileRequired'),
      path: ['dockerfilePath'],
    })
  }

  if (!value.buildContext?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      message: i18next.t('apps.buildContextRequired'),
      path: ['buildContext'],
    })
  }
})

type ApplicationFormInput = z.input<typeof schema>
type ApplicationForm = z.output<typeof schema>

export interface ApplicationsPageHandle {
  openCreateDialog: () => void
}

interface ApplicationsPageProps {
  embedded?: boolean
  projectId?: string
  ref?: Ref<ApplicationsPageHandle>
}

export function ApplicationsPage({ embedded = false, projectId: projectIdProp, ref }: ApplicationsPageProps = {}) {
  const { t } = useTranslation()
  const { projectId: routeProjectId = '' } = useParams()
  const projectId = projectIdProp ?? routeProjectId
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [repoSearch, setRepoSearch] = useState('')
  const [repoResultsOpen, setRepoResultsOpen] = useState(false)
  const [branchSearch, setBranchSearch] = useState('')
  const [editingApplication, setEditingApplication] = useState<Application | null>(null)
  const [editingBinding, setEditingBinding] = useState<RepositoryBinding | null>(null)

  const applications = useQuery({
    queryKey: ['applications', projectId],
    queryFn: () => api.listApplications(projectId),
    enabled: Boolean(projectId),
  })
  const repositoryBindings = useQuery({
    queryKey: ['repository-bindings', projectId],
    queryFn: () => api.listRepositoryBindings(projectId),
    enabled: Boolean(projectId),
  })
  const providers = useQuery({ queryKey: ['git-providers'], queryFn: () => api.listGitProviders() })
  const accounts = useQuery({ queryKey: ['git-accounts'], queryFn: () => api.listGitAccounts() })

  const form = useForm<ApplicationFormInput, undefined, ApplicationForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      name: '',
      slug: '',
      sourceType: 'repository',
      repositoryUrl: '',
      imageReference: '',
      dockerfilePath: 'Dockerfile',
      buildContext: '.',
      servicePort: 8080,
      gitAccountId: '',
      repositoryOwner: '',
      repositoryName: '',
      cloneUrl: '',
      defaultBranch: 'main',
      webhookStatus: 'pending',
    },
  })

  const sourceType = form.watch('sourceType')
  const selectedAccountId = form.watch('gitAccountId')
  const selectedOwner = form.watch('repositoryOwner')
  const selectedRepo = form.watch('repositoryName')
  const selectedBranch = form.watch('defaultBranch')
  const gitAccountField = form.register('gitAccountId')

  useEffect(() => {
    if (sourceType === 'image') {
      form.setValue('gitAccountId', '')
      form.setValue('repositoryOwner', '')
      form.setValue('repositoryName', '')
      form.setValue('cloneUrl', '')
      form.setValue('defaultBranch', 'main')
      form.setValue('dockerfilePath', 'Dockerfile')
      form.setValue('buildContext', '.')
      form.setValue('webhookStatus', 'pending')
    }
  }, [sourceType, form])

  const repositories = useQuery({
    queryKey: ['git-repositories', selectedAccountId, repoSearch],
    queryFn: () => api.listGitRepositories(selectedAccountId || '', { page: 1, pageSize: 50, search: repoSearch }),
    enabled: Boolean(selectedAccountId),
  })
  const branches = useQuery({
    queryKey: ['git-branches', selectedAccountId, selectedOwner, selectedRepo, branchSearch],
    queryFn: () => api.listGitBranches(selectedAccountId || '', selectedOwner || '', selectedRepo || '', { search: branchSearch, limit: 50 }),
    enabled: Boolean(selectedAccountId && selectedOwner && selectedRepo),
  })
  const buildOptions = useQuery({
    queryKey: ['git-build-options', selectedAccountId, selectedOwner, selectedRepo, selectedBranch],
    queryFn: () => api.getGitRepositoryBuildOptions(selectedAccountId || '', selectedOwner || '', selectedRepo || '', selectedBranch || 'main'),
    enabled: Boolean(selectedAccountId && selectedOwner && selectedRepo && selectedBranch),
  })

  useEffect(() => {
    if (!buildOptions.data || sourceType !== 'repository')
      return

    const currentDockerfile = form.getValues('dockerfilePath')?.trim()
    const dockerfiles = dockerfileOptions(buildOptions.data, currentDockerfile)
    if (!currentDockerfile && dockerfiles.length > 0) {
      const dockerfile = dockerfiles[0]
      form.setValue('dockerfilePath', dockerfile, { shouldDirty: true, shouldValidate: true })
      form.setValue('buildContext', dockerfileBuildContext(dockerfile), { shouldDirty: true, shouldValidate: true })
      return
    }

    const currentContext = form.getValues('buildContext')?.trim()
    const contexts = buildContextOptions(buildOptions.data, currentContext)
    if (!currentContext && contexts.length > 0)
      form.setValue('buildContext', contexts[0], { shouldDirty: true, shouldValidate: true })
  }, [buildOptions.data, form, sourceType])

  const resetApplicationForm = (application?: Application, binding?: RepositoryBinding | null) => {
    const repositoryReference = binding ? `${binding.owner}/${binding.repo}` : ''
    form.reset({
      name: application?.name ?? '',
      slug: application?.slug ?? '',
      sourceType: application?.sourceType ?? 'repository',
      repositoryUrl: application?.repositoryUrl ?? '',
      imageReference: application?.imageReference ?? '',
      dockerfilePath: application?.dockerfilePath ?? 'Dockerfile',
      buildContext: application?.buildContext ?? '.',
      servicePort: application?.servicePort ?? 8080,
      gitAccountId: binding?.gitAccountId ?? application?.gitAccountId ?? '',
      repositoryOwner: binding?.owner ?? parseRepositoryReference(application?.repositoryUrl).owner,
      repositoryName: binding?.repo ?? parseRepositoryReference(application?.repositoryUrl).repo,
      cloneUrl: binding?.cloneUrl ?? application?.repositoryUrl ?? '',
      defaultBranch: binding?.defaultBranch ?? 'main',
      webhookStatus: binding?.webhookStatus ?? 'pending',
    })
    setRepoSearch(repositoryReference)
    setRepoResultsOpen(false)
  }

  const openCreateDialog = () => {
    setEditingApplication(null)
    setEditingBinding(null)
    resetApplicationForm()
    setDialogOpen(true)
  }

  useImperativeHandle(ref, () => ({ openCreateDialog }))

  const saveRepositoryBinding = async (applicationId: string, payload: ApplicationForm) => {
    if (payload.sourceType !== 'repository')
      return
    const bindingPayload = {
      applicationId,
      cloneUrl: payload.cloneUrl ?? '',
      defaultBranch: payload.defaultBranch ?? 'main',
      gitAccountId: payload.gitAccountId || '',
      owner: payload.repositoryOwner || '',
      repo: payload.repositoryName || '',
      webhookStatus: payload.webhookStatus || 'pending',
    }

    if (editingBinding)
      await api.updateRepositoryBinding(projectId, editingBinding.id, bindingPayload)
    else
      await api.createRepositoryBinding(projectId, bindingPayload)
  }

  const clearRepositoryBinding = async () => {
    if (!editingBinding)
      return
    await api.deleteRepositoryBinding(projectId, editingBinding.id)
  }

  const createApplication = useMutation({
    mutationFn: (payload: ApplicationForm) =>
      (async () => {
        const appPayload = {
          name: payload.name,
          slug: payload.slug,
          sourceType: payload.sourceType,
          gitAccountId: payload.sourceType === 'repository' ? (payload.gitAccountId ?? '') : '',
          repositoryUrl: payload.sourceType === 'repository' ? (payload.cloneUrl ?? '') : '',
          imageReference: payload.imageReference ?? '',
          dockerfilePath: payload.dockerfilePath ?? 'Dockerfile',
          buildContext: payload.buildContext ?? '.',
          servicePort: payload.servicePort,
        }
        const application = await api.createApplication(projectId, appPayload)
        if (appPayload.sourceType === 'repository')
          await saveRepositoryBinding(application.id, payload)
        return application
      })(),
    onSuccess: () => {
      toast.success(t('apps.created'))
      form.reset()
      setEditingBinding(null)
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const updateApplication = useMutation({
    mutationFn: (payload: ApplicationForm) =>
      (async () => {
        if (!editingApplication)
          throw new Error(t('apps.editMissing'))

        const appPayload = {
          name: payload.name,
          slug: payload.slug,
          sourceType: payload.sourceType,
          gitAccountId: payload.sourceType === 'repository' ? (payload.gitAccountId ?? '') : '',
          repositoryUrl: payload.sourceType === 'repository' ? (payload.cloneUrl ?? '') : '',
          imageReference: payload.imageReference ?? '',
          dockerfilePath: payload.dockerfilePath ?? 'Dockerfile',
          buildContext: payload.buildContext ?? '.',
          servicePort: payload.servicePort,
        }
        const result = await api.updateApplication(projectId, editingApplication.id, appPayload)
        if (payload.sourceType === 'repository')
          await saveRepositoryBinding(result.id, payload)
        else
          await clearRepositoryBinding()
        return result
      })(),
    onSuccess: () => {
      toast.success(t('apps.updated'))
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
      setEditingApplication(null)
      setEditingBinding(null)
      setDialogOpen(false)
      resetApplicationForm()
    },
    onError: error => toast.error(error.message),
  })

  const deleteApplication = useMutation({
    mutationFn: (applicationId: string) => api.deleteApplication(projectId, applicationId),
    onSuccess: () => {
      toast.success(t('apps.deleted'))
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const selectRepository = (repository: GitRepository) => {
    form.setValue('repositoryOwner', repository.owner, { shouldDirty: true, shouldValidate: true })
    form.setValue('repositoryName', repository.name, { shouldDirty: true, shouldValidate: true })
    form.setValue('cloneUrl', repository.cloneUrl, { shouldDirty: true, shouldValidate: true })
    form.setValue('defaultBranch', repository.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
    form.setValue('dockerfilePath', '', { shouldDirty: true, shouldValidate: true })
    form.setValue('buildContext', '', { shouldDirty: true, shouldValidate: true })
    setBranchSearch('')
    setRepoSearch(repository.fullName)
    setRepoResultsOpen(false)
  }

  return (
    <div className="grid gap-6">
      {!embedded && (
        <PageHeader
          actions={(
            <div className="flex items-center gap-3">
              <Button onClick={openCreateDialog}>
                <Plus size={16} />
                {t('apps.createTitle')}
              </Button>
              <Link className="text-sm text-primary hover:underline" to="/projects">{t('backToProjectSpaces')}</Link>
            </div>
          )}
          description={t('apps.description')}
          title={t('apps.title')}
        />
      )}

      <div className="grid gap-4">
        <MotionList className="grid gap-3">
          {applications.isError && <ErrorState title={t('apps.loadFailedTitle')} description={t('apps.loadFailedDescription')} />}
          {(applications.data ?? []).map(application => (
            <MotionItem key={application.id}>
              <ApplicationRow
                application={application}
                binding={(repositoryBindings.data ?? []).find(binding => binding.applicationId === application.id)}
                deletePending={deleteApplication.isPending}
                onDelete={() => deleteApplication.mutate(application.id)}
                onEdit={() => {
                  const binding = (repositoryBindings.data ?? []).find(item => item.applicationId === application.id) ?? null
                  setEditingApplication(application)
                  setEditingBinding(binding)
                  resetApplicationForm(application, binding)
                  setDialogOpen(true)
                }}
              />
            </MotionItem>
          ))}
          {applications.data?.length === 0 && <EmptyState title={t('apps.emptyTitle')} description={t('apps.emptyDescription')} />}
        </MotionList>
      </div>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) {
            setEditingApplication(null)
            setEditingBinding(null)
            resetApplicationForm()
          }
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editingApplication ? t('apps.editTitle') : t('apps.createTitle')}</DialogTitle>
            <DialogDescription>{t('apps.description')}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-3"
            onSubmit={form.handleSubmit((values) => {
              if (editingApplication)
                updateApplication.mutate(values)
              else
                createApplication.mutate(values)
            })}
          >
            <div className="grid grid-cols-2 gap-3">
              <Field error={form.formState.errors.name?.message} hint={t('apps.nameHint')} label={t('apps.name')} required>
                <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('apps.namePlaceholder')} />
              </Field>
              <Field error={form.formState.errors.slug?.message} hint={t('apps.slugHint')} label={t('apps.slug')} required>
                <Input {...form.register('slug')} aria-invalid={Boolean(form.formState.errors.slug)} placeholder={t('apps.slugPlaceholder')} />
              </Field>
            </div>
            <Field error={form.formState.errors.sourceType?.message} hint={t('apps.sourceTypeHint')} label={t('apps.sourceType')} required>
              <Select {...form.register('sourceType')} aria-invalid={Boolean(form.formState.errors.sourceType)}>
                <option value="repository">{t('apps.repository')}</option>
                <option value="image">{t('apps.image')}</option>
              </Select>
            </Field>
            {sourceType === 'repository'
              ? (
                  <div className="grid gap-3">
                    <Field error={form.formState.errors.gitAccountId?.message} hint={t('repositories.gitAccountHint')} label={t('repositories.gitAccount')} required>
                      <Select
                        {...gitAccountField}
                        aria-invalid={Boolean(form.formState.errors.gitAccountId)}
                        onChange={(event) => {
                          gitAccountField.onChange(event)
                          form.setValue('repositoryOwner', '', { shouldDirty: true, shouldValidate: true })
                          form.setValue('repositoryName', '', { shouldDirty: true, shouldValidate: true })
                          form.setValue('cloneUrl', '', { shouldDirty: true, shouldValidate: true })
                          form.setValue('defaultBranch', 'main', { shouldDirty: true, shouldValidate: true })
                          form.setValue('dockerfilePath', '', { shouldDirty: true, shouldValidate: true })
                          form.setValue('buildContext', '', { shouldDirty: true, shouldValidate: true })
                          setRepoSearch('')
                          setBranchSearch('')
                          setRepoResultsOpen(Boolean(event.target.value))
                        }}
                      >
                        <option value="">{t('repositories.selectAccount')}</option>
                        {(accounts.data ?? []).map(account => (
                          <option key={account.id} value={account.id}>
                            {accountLabel(account, providers.data ?? [], t)}
                          </option>
                        ))}
                      </Select>
                    </Field>
                    <UiAlert>
                      <GitBranch />
                      <UiAlertTitle>{t('repositories.repositoryInputTitle')}</UiAlertTitle>
                      <UiAlertDescription>{t('repositories.repositoryInputDescription')}</UiAlertDescription>
                    </UiAlert>
                    <Field hint={t('repositories.repositorySearchHint')} label={t('repositories.repositorySearch')}>
                      <div className="flex gap-2">
                        <Input
                          value={repoSearch}
                          placeholder={t('repositories.repositorySearchPlaceholder')}
                          onChange={(event) => {
                            setRepoSearch(event.target.value)
                            setRepoResultsOpen(Boolean(selectedAccountId))
                          }}
                          onFocus={() => setRepoResultsOpen(Boolean(selectedAccountId))}
                        />
                        <Button
                          disabled={!selectedAccountId || repositories.isFetching}
                          type="button"
                          variant="secondary"
                          onClick={() => {
                            setRepoResultsOpen(Boolean(selectedAccountId))
                            repositories.refetch()
                          }}
                        >
                          <Search size={16} />
                          {t('repositories.search')}
                        </Button>
                      </div>
                    </Field>
                    {selectedAccountId && repoResultsOpen && (
                      <div className="grid max-h-52 gap-2 overflow-y-auto rounded-md border border-border p-2">
                        {(repositories.data?.items ?? []).map(repository => (
                          <button
                            key={repository.fullName}
                            className="rounded-md px-3 py-2 text-left hover:bg-muted"
                            type="button"
                            onClick={() => selectRepository(repository)}
                          >
                            <span className="block text-sm font-medium">{repository.fullName}</span>
                            <span className="block text-xs text-muted-foreground">{repository.cloneUrl}</span>
                          </button>
                        ))}
                        {repositories.data?.items.length === 0 && (
                          <EmptyState
                            title={t('repositories.noRepositoriesTitle')}
                            description={t('repositories.noRepositoriesDescription')}
                          />
                        )}
                      </div>
                    )}
                    {selectedOwner && selectedRepo && (
                      <div className="grid gap-2 rounded-md border border-border bg-muted/30 p-3 text-sm">
                        <div className="grid gap-1 md:grid-cols-3">
                          <p className="min-w-0">
                            <span className="block text-xs text-muted-foreground">{t('repositories.owner')}</span>
                            <span className="block truncate font-medium">{selectedOwner}</span>
                          </p>
                          <p className="min-w-0">
                            <span className="block text-xs text-muted-foreground">{t('repositories.repo')}</span>
                            <span className="block truncate font-medium">{selectedRepo}</span>
                          </p>
                          <p className="min-w-0">
                            <span className="block text-xs text-muted-foreground">{t('repositories.cloneUrl')}</span>
                            <span className="block truncate font-medium">{form.watch('cloneUrl')}</span>
                          </p>
                        </div>
                      </div>
                    )}
                    <div className="grid gap-3 md:grid-cols-3">
                      <Field
                        error={form.formState.errors.defaultBranch?.message}
                        label={t('repositories.defaultBranch')}
                      >
                        <SearchSelect
                          disabled={!selectedAccountId || !selectedOwner || !selectedRepo}
                          emptyLabel={t('repositories.noBranches')}
                          limited={branches.data?.limited}
                          loading={branches.isFetching}
                          options={branchOptions(branches.data?.items ?? [], form.watch('defaultBranch'))}
                          placeholder={t('repositories.defaultBranchPlaceholder')}
                          search={branchSearch}
                          value={form.watch('defaultBranch') || ''}
                          onSearchChange={setBranchSearch}
                          onValueChange={value => form.setValue('defaultBranch', value, { shouldDirty: true, shouldValidate: true })}
                        />
                      </Field>
                      <Field error={form.formState.errors.dockerfilePath?.message} hint={t('apps.dockerfileHint')} label={t('apps.dockerfile')} required>
                        <Input
                          {...form.register('dockerfilePath', {
                            onChange: (event) => {
                              const context = dockerfileBuildContext(event.target.value)
                              if ((buildContextOptions(buildOptions.data, form.watch('buildContext')).includes(context)))
                                form.setValue('buildContext', context, { shouldDirty: true, shouldValidate: true })
                            },
                          })}
                          aria-invalid={Boolean(form.formState.errors.dockerfilePath)}
                          disabled={!selectedOwner || !selectedRepo}
                          list="application-dockerfile-options"
                          placeholder={buildOptions.isFetching ? t('apps.detectingRepository') : t('apps.dockerfilePlaceholder')}
                        />
                        <datalist id="application-dockerfile-options">
                          {dockerfileOptions(buildOptions.data, form.watch('dockerfilePath')).map(path => (
                            <option key={path} value={path}>{path}</option>
                          ))}
                        </datalist>
                        {buildOptions.isSuccess && dockerfileOptions(buildOptions.data, form.watch('dockerfilePath')).length === 0 && (
                          <p className="text-xs text-muted-foreground">{t('apps.noDockerfilesDetected')}</p>
                        )}
                      </Field>
                      <Field error={form.formState.errors.buildContext?.message} hint={t('apps.buildContextHint')} label={t('apps.buildContext')} required>
                        <Input
                          {...form.register('buildContext')}
                          aria-invalid={Boolean(form.formState.errors.buildContext)}
                          disabled={!selectedOwner || !selectedRepo}
                          list="application-build-context-options"
                          placeholder={buildOptions.isFetching ? t('apps.detectingRepository') : t('apps.buildContextPlaceholder')}
                        />
                        <datalist id="application-build-context-options">
                          {buildContextOptions(buildOptions.data, form.watch('buildContext')).map(path => (
                            <option key={path} value={path}>{path}</option>
                          ))}
                        </datalist>
                      </Field>
                    </div>
                  </div>
                )
              : (
                  <>
                    <Field error={form.formState.errors.imageReference?.message} hint={t('apps.imageReferenceHint')} label={t('apps.imageReference')}>
                      <Input
                        {...form.register('imageReference')}
                        aria-invalid={Boolean(form.formState.errors.imageReference)}
                        placeholder={t('apps.imageReferencePlaceholder')}
                      />
                    </Field>
                    <Field error={form.formState.errors.dockerfilePath?.message} hint={t('apps.dockerfileHint')} label={t('apps.dockerfile')}>
                      <Input
                        {...form.register('dockerfilePath')}
                        aria-invalid={Boolean(form.formState.errors.dockerfilePath)}
                        placeholder={t('apps.dockerfilePlaceholder')}
                      />
                    </Field>
                    <Field error={form.formState.errors.buildContext?.message} hint={t('apps.buildContextHint')} label={t('apps.buildContext')}>
                      <Input
                        {...form.register('buildContext')}
                        aria-invalid={Boolean(form.formState.errors.buildContext)}
                        placeholder={t('apps.buildContextPlaceholder') || '.'}
                      />
                    </Field>
                  </>
                )}
            <Field error={form.formState.errors.servicePort?.message} hint={t('apps.servicePortHint')} label={t('apps.servicePort')} required>
              <Input type="number" {...form.register('servicePort')} aria-invalid={Boolean(form.formState.errors.servicePort)} />
            </Field>
            <DialogFooter>
              <Button
                disabled={createApplication.isPending || updateApplication.isPending || !form.formState.isValid}
                type="submit"
              >
                {editingApplication ? <Save size={16} /> : <Plus size={16} />}
                {editingApplication ? t('save') : t('apps.createTitle')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ApplicationRow({
  application,
  binding,
  deletePending,
  onDelete,
  onEdit,
}: {
  application: Application
  binding?: RepositoryBinding
  deletePending?: boolean
  onDelete: () => void
  onEdit: () => void
}) {
  const { t } = useTranslation()
  const sourceMeta = (() => {
    if (application.sourceType !== 'repository')
      return application.imageReference
    if (!binding)
      return application.repositoryUrl || t('apps.repositoryUrlPlaceholder')
    const providerName = binding.providerName || binding.gitProviderId
    const repoRef = `${binding.owner}/${binding.repo}`
    const accountOwner = binding.accountOwnerName || binding.accountOwnerEmail
    return [providerName, repoRef, accountOwner, `${t('apps.buildContext')}: ${application.buildContext || '.'}`].filter(Boolean).join(' · ')
  })()

  return (
    <Card className="flex items-center justify-between gap-4">
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <Box size={18} />
        </span>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="truncate font-medium">{application.name}</h3>
            <span className="inline-flex h-6 items-center rounded-full border border-border bg-muted px-2 text-xs text-muted-foreground">
              {application.sourceType}
            </span>
          </div>
          <p className="truncate text-sm text-muted-foreground">{sourceMeta}</p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button type="button" variant="secondary">
          <span className="truncate">
            {application.servicePort}
            /tcp
          </span>
        </Button>
        <EditActionButton aria-label={t('apps.editAria')} onClick={onEdit} label={t('edit')} />
        <ConfirmDialog
          confirmText={t('apps.deleteConfirm')}
          description={t('apps.deleteDescription', { name: application.name })}
          pending={deletePending}
          title={t('apps.deleteTitle')}
          onConfirm={onDelete}
        >
          <Button aria-label={t('apps.deleteAria')} variant="ghost">
            <Trash2 size={16} />
          </Button>
        </ConfirmDialog>
      </div>
    </Card>
  )
}

function accountLabel(account: GitAccount, providers: GitProvider[], t: (key: string) => string) {
  const provider = providers.find(item => item.id === account.providerId)
  const scope = account.accessScope === 'provider' ? t('codeRepositoriesView.providerScope') : t('codeRepositoriesView.personalScope')
  return `${provider?.name ?? account.providerId} / ${account.username} (${scope})`
}

function branchOptions(branches: Array<{ name: string }>, current?: string) {
  const options = branches.map(branch => ({ value: branch.name, label: branch.name }))
  const normalized = current?.trim()
  if (normalized && !options.some(option => option.value === normalized))
    options.unshift({ value: normalized, label: normalized })
  return options
}

function dockerfileBuildContext(path: string) {
  const normalized = path.trim().replace(/^\/+|\/+$/g, '')
  if (!normalized || !normalized.includes('/'))
    return '.'
  return normalized.split('/').slice(0, -1).join('/')
}

function dockerfileOptions(options?: GitRepositoryBuildOptions, current?: string) {
  return withCurrentOption(options?.dockerfiles ?? [], current)
}

function buildContextOptions(options?: GitRepositoryBuildOptions, current?: string) {
  return withCurrentOption(options?.directories ?? ['.'], current)
}

function withCurrentOption(options: string[], current?: string) {
  const values = new Set(options)
  const normalized = current?.trim()
  if (normalized)
    values.add(normalized)
  return sortBuildPaths([...values])
}

function sortBuildPaths(paths: string[]) {
  return paths.sort((left, right) => {
    if (left === '.')
      return -1
    if (right === '.')
      return 1
    return left.localeCompare(right)
  })
}

function parseRepositoryReference(value?: string) {
  const fallback = { owner: '', repo: '' }
  const trimmed = value?.trim()
  if (!trimmed)
    return fallback
  try {
    const url = new URL(trimmed)
    const [owner, repoWithSuffix] = url.pathname.replace(/^\/+/, '').split('/')
    return { owner: owner ?? '', repo: (repoWithSuffix ?? '').replace(/\.git$/, '') }
  }
  catch {
    const [owner, repo] = trimmed.split('/').map(part => part.trim())
    return { owner: owner ?? '', repo: (repo ?? '').replace(/\.git$/, '') }
  }
}
