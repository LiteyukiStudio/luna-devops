import type { GitAccount, GitProvider, GitRepository, RepositoryBinding } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { GitBranch, LinkIcon, Plus, Search, Trash2 } from 'lucide-react'
import { useState } from 'react'
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
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'

const bindingSchema = z.object({
  applicationId: z.string().min(1, i18next.t('repositories.applicationRequired')),
  gitAccountId: z.string().min(1, i18next.t('repositories.gitAccountRequired')),
  owner: z.string().min(1, i18next.t('repositories.ownerRequired')),
  repo: z.string().min(1, i18next.t('repositories.repoRequired')),
  cloneUrl: z.string().optional(),
  defaultBranch: z.string().optional(),
  webhookStatus: z.enum(['pending', 'created', 'disabled', 'failed']),
})

type BindingFormInput = z.input<typeof bindingSchema>
type BindingForm = z.output<typeof bindingSchema>

export function RepositoryBindingsPage({ embedded = false, projectId: projectIdProp }: { embedded?: boolean, projectId?: string } = {}) {
  const { t } = useTranslation()
  const { projectId: routeProjectId = '' } = useParams()
  const projectId = projectIdProp ?? routeProjectId
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingBinding, setEditingBinding] = useState<RepositoryBinding | null>(null)
  const [bindingToDelete, setBindingToDelete] = useState<RepositoryBinding | null>(null)
  const [repoSearch, setRepoSearch] = useState('')
  const [branchSearch, setBranchSearch] = useState('')
  const providers = useQuery({ queryKey: ['git-providers'], queryFn: () => api.listGitProviders() })
  const accounts = useQuery({ queryKey: ['git-accounts'], queryFn: () => api.listGitAccounts() })
  const applications = useQuery({
    queryKey: ['applications', projectId],
    queryFn: () => api.listApplications(projectId),
    enabled: Boolean(projectId),
  })
  const bindings = useQuery({
    queryKey: ['repository-bindings', projectId],
    queryFn: () => api.listRepositoryBindings(projectId),
    enabled: Boolean(projectId),
  })

  const form = useForm<BindingFormInput, undefined, BindingForm>({
    resolver: zodResolver(bindingSchema),
    mode: 'onChange',
    defaultValues: {
      applicationId: '',
      gitAccountId: '',
      owner: '',
      repo: '',
      cloneUrl: '',
      defaultBranch: 'main',
      webhookStatus: 'pending',
    },
  })
  const selectedAccountId = form.watch('gitAccountId')
  const selectedOwner = form.watch('owner')
  const selectedRepo = form.watch('repo')
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

  const createBinding = useMutation({
    mutationFn: (payload: BindingForm) => api.createRepositoryBinding(projectId, {
      applicationId: payload.applicationId,
      gitAccountId: payload.gitAccountId,
      owner: payload.owner,
      repo: payload.repo,
      cloneUrl: payload.cloneUrl ?? '',
      defaultBranch: payload.defaultBranch ?? 'main',
      webhookStatus: payload.webhookStatus,
    }),
    onSuccess: () => {
      toast.success(t('repositories.bindingSaved'))
      resetBindingForm()
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const updateBinding = useMutation({
    mutationFn: (payload: BindingForm) => {
      if (!editingBinding)
        throw new Error(t('repositories.editMissing'))
      return api.updateRepositoryBinding(projectId, editingBinding.id, {
        applicationId: payload.applicationId,
        gitAccountId: payload.gitAccountId,
        owner: payload.owner,
        repo: payload.repo,
        cloneUrl: payload.cloneUrl ?? '',
        defaultBranch: payload.defaultBranch ?? 'main',
        webhookStatus: payload.webhookStatus,
      })
    },
    onSuccess: () => {
      toast.success(t('repositories.bindingSaved'))
      resetBindingForm()
      setEditingBinding(null)
      setDialogOpen(false)
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
      queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteBinding = useMutation({
    mutationFn: (bindingId: string) => api.deleteRepositoryBinding(projectId, bindingId),
    onSuccess: () => {
      toast.success(t('repositories.bindingDeleted'))
      setBindingToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const createWebhook = useMutation({
    mutationFn: (bindingId: string) => api.createRepositoryWebhook(projectId, bindingId),
    onSuccess: () => {
      toast.success(t('repositories.webhookCreated'))
      queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  const selectRepository = (repository: GitRepository) => {
    form.setValue('owner', repository.owner, { shouldDirty: true, shouldValidate: true })
    form.setValue('repo', repository.name, { shouldDirty: true, shouldValidate: true })
    form.setValue('cloneUrl', repository.cloneUrl, { shouldDirty: true, shouldValidate: true })
    form.setValue('defaultBranch', repository.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
    setBranchSearch('')
  }

  function resetBindingForm(binding?: RepositoryBinding | null) {
    form.reset({
      applicationId: binding?.applicationId ?? '',
      gitAccountId: binding?.gitAccountId ?? '',
      owner: binding?.owner ?? '',
      repo: binding?.repo ?? '',
      cloneUrl: binding?.cloneUrl ?? '',
      defaultBranch: binding?.defaultBranch ?? 'main',
      webhookStatus: binding?.webhookStatus ?? 'pending',
    })
    setRepoSearch(binding ? `${binding.owner}/${binding.repo}` : '')
    setBranchSearch('')
  }

  const openCreateDialog = () => {
    setEditingBinding(null)
    resetBindingForm()
    setDialogOpen(true)
  }

  const openEditDialog = (binding: RepositoryBinding) => {
    setEditingBinding(binding)
    resetBindingForm(binding)
    setDialogOpen(true)
  }

  return (
    <div className="grid gap-6">
      <PageHeader
        actions={!embedded
          ? (
              <div className="flex items-center gap-3">
                <Button type="button" onClick={openCreateDialog}>
                  <Plus size={16} />
                  {t('repositories.bindRepoTitle')}
                </Button>
                <Link className="text-sm text-primary hover:underline" to={`/projects/${projectId}/apps`}>{t('backToApps')}</Link>
              </div>
            )
          : undefined}
        description={t('repositories.description')}
        title={t('repositories.title')}
      />

      {(providers.isError || accounts.isError || bindings.isError) && (
        <ErrorState title={t('repositories.loadFailedTitle')} description={t('repositories.loadFailedDescription')} />
      )}

      <MotionList className="grid gap-3">
        {(bindings.data ?? []).map(binding => (
          <MotionItem key={binding.id}>
            <BindingRow
              accounts={accounts.data ?? []}
              binding={binding}
              providers={providers.data ?? []}
              webhookPending={createWebhook.isPending}
              onDelete={() => setBindingToDelete(binding)}
              onEdit={() => openEditDialog(binding)}
              onCreateWebhook={() => createWebhook.mutate(binding.id)}
            />
          </MotionItem>
        ))}
        {bindings.data?.length === 0 && <EmptyState title={t('repositories.emptyTitle')} description={t('repositories.emptyDescription')} />}
      </MotionList>

      <ConfirmDialog
        confirmText={t('repositories.deleteConfirm')}
        description={t('repositories.deleteDescription', { repo: `${bindingToDelete?.owner ?? ''}/${bindingToDelete?.repo ?? ''}` })}
        open={Boolean(bindingToDelete)}
        pending={deleteBinding.isPending}
        title={t('repositories.deleteTitle')}
        onConfirm={() => bindingToDelete && deleteBinding.mutate(bindingToDelete.id)}
        onOpenChange={open => !open && setBindingToDelete(null)}
      />

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) {
            setEditingBinding(null)
            resetBindingForm()
          }
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{editingBinding ? t('repositories.editBinding') : t('repositories.bindRepoTitle')}</DialogTitle>
            <DialogDescription>{t('repositories.bindingDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-3"
            onSubmit={form.handleSubmit((values) => {
              if (editingBinding)
                updateBinding.mutate(values)
              else
                createBinding.mutate(values)
            })}
          >
            <div className="flex justify-end">
              <Link className="text-sm text-primary hover:underline" to="/code-repositories">{t('repositories.manageCodeRepositories')}</Link>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <Field error={form.formState.errors.applicationId?.message} label={t('repositories.application')} required>
                <Select {...form.register('applicationId')} aria-invalid={Boolean(form.formState.errors.applicationId)}>
                  <option value="">{t('repositories.selectApplication')}</option>
                  {(applications.data ?? []).map(application => (
                    <option key={application.id} value={application.id}>{application.name}</option>
                  ))}
                </Select>
              </Field>
              <Field error={form.formState.errors.gitAccountId?.message} label={t('repositories.gitAccount')} required>
                <Select {...form.register('gitAccountId')} aria-invalid={Boolean(form.formState.errors.gitAccountId)}>
                  <option value="">{t('repositories.selectAccount')}</option>
                  {(accounts.data ?? []).map(account => (
                    <option key={account.id} value={account.id}>
                      {accountLabel(account, providers.data ?? [], t)}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <Field hint={t('repositories.repositorySearchHint')} label={t('repositories.repositorySearch')}>
              <div className="flex gap-2">
                <Input value={repoSearch} placeholder={t('repositories.repositorySearchPlaceholder')} onChange={event => setRepoSearch(event.target.value)} />
                <Button disabled={!selectedAccountId || repositories.isFetching} type="button" variant="secondary" onClick={() => repositories.refetch()}>
                  <Search size={16} />
                  {t('repositories.search')}
                </Button>
              </div>
            </Field>
            {selectedAccountId && (
              <div className="grid max-h-56 gap-2 overflow-y-auto rounded-md border border-border p-2">
                {(repositories.data?.items ?? []).map(repository => (
                  <button key={repository.fullName} className="rounded-md px-3 py-2 text-left hover:bg-muted" type="button" onClick={() => selectRepository(repository)}>
                    <span className="block text-sm font-medium">{repository.fullName}</span>
                    <span className="block text-xs text-muted-foreground">{repository.cloneUrl}</span>
                  </button>
                ))}
                {repositories.data?.items.length === 0 && <EmptyState title={t('repositories.noRepositoriesTitle')} description={t('repositories.noRepositoriesDescription')} />}
              </div>
            )}
            <div className="grid gap-3 md:grid-cols-3">
              <Field error={form.formState.errors.owner?.message} label={t('repositories.owner')} required><Input {...form.register('owner')} aria-invalid={Boolean(form.formState.errors.owner)} placeholder={t('repositories.ownerPlaceholder')} /></Field>
              <Field error={form.formState.errors.repo?.message} label={t('repositories.repo')} required><Input {...form.register('repo')} aria-invalid={Boolean(form.formState.errors.repo)} placeholder={t('repositories.repoPlaceholder')} /></Field>
              <Field error={form.formState.errors.defaultBranch?.message} label={t('repositories.defaultBranch')}>
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
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <Field error={form.formState.errors.cloneUrl?.message} label={t('repositories.cloneUrl')}><Input {...form.register('cloneUrl')} aria-invalid={Boolean(form.formState.errors.cloneUrl)} placeholder={t('repositories.cloneUrlPlaceholder')} /></Field>
              <Field error={form.formState.errors.webhookStatus?.message} label={t('repositories.webhookStatus')} required>
                <Select {...form.register('webhookStatus')} aria-invalid={Boolean(form.formState.errors.webhookStatus)}>
                  <option value="pending">{t('common.pending')}</option>
                  <option value="created">{t('common.createdStatus')}</option>
                  <option value="disabled">{t('common.disabled')}</option>
                  <option value="failed">{t('common.failed')}</option>
                </Select>
              </Field>
            </div>
            <DialogFooter>
              <Button disabled={createBinding.isPending || updateBinding.isPending || (accounts.data ?? []).length === 0 || !form.formState.isValid} type="submit">
                <Plus size={16} />
                {t('repositories.saveBinding')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function BindingRow({
  accounts,
  binding,
  providers,
  webhookPending,
  onCreateWebhook,
  onDelete,
  onEdit,
}: {
  accounts: GitAccount[]
  binding: RepositoryBinding
  providers: GitProvider[]
  webhookPending: boolean
  onCreateWebhook: () => void
  onDelete: () => void
  onEdit: () => void
}) {
  const { t } = useTranslation()
  const provider = providers.find(item => item.id === binding.gitProviderId)
  const account = accounts.find(item => item.id === binding.gitAccountId)

  return (
    <Card className="flex items-center justify-between gap-4">
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <GitBranch size={18} />
        </span>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="truncate font-medium">
              {binding.owner}
              /
              {binding.repo}
            </h3>
            <StatusBadge>{binding.defaultBranch}</StatusBadge>
            <StatusBadge>{binding.webhookStatus}</StatusBadge>
          </div>
          <p className="truncate text-sm text-muted-foreground">
            {binding.applicationName ?? binding.applicationId}
            {' · '}
            {provider?.name ?? binding.providerName ?? binding.gitProviderId}
            {' · '}
            {account?.username ?? binding.accountUsername ?? binding.gitAccountId}
            {binding.accountOwnerEmail && (
              <>
                {' · '}
                {binding.accountOwnerName || binding.accountOwnerEmail}
              </>
            )}
          </p>
          <p className="truncate text-xs text-muted-foreground">{binding.cloneUrl}</p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button disabled={webhookPending || binding.webhookStatus === 'created'} type="button" variant="secondary" onClick={onCreateWebhook}>
          <LinkIcon size={16} />
          {t('repositories.createWebhook')}
        </Button>
        <EditActionButton aria-label={t('repositories.editAria')} onClick={onEdit} label={t('edit')} />
        <Button aria-label={t('repositories.deleteAria')} variant="ghost" onClick={onDelete}>
          <Trash2 size={16} />
        </Button>
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
