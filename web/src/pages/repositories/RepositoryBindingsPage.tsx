import type { GitAccount, GitProvider, RepositoryBinding } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { GitBranch, LinkIcon, MoreHorizontal, Pencil, Save, Unlink } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataList } from '@/components/common/data-list'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { GitRepositoryPicker } from '@/components/common/git-repository-picker'
import { SearchSelect } from '@/components/common/search-select'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

const bindingSchema = z.object({
  applicationId: z.string().min(1, { error: () => i18next.t('repositories.applicationRequired') }),
  gitAccountId: z.string().min(1, { error: () => i18next.t('repositories.gitAccountRequired') }),
  owner: z.string().min(1, { error: () => i18next.t('repositories.ownerRequired') }),
  repo: z.string().min(1, { error: () => i18next.t('repositories.repoRequired') }),
  cloneUrl: z.string().optional(),
  defaultBranch: z.string().optional(),
  autoConfigureWebhook: z.boolean().default(true),
})

type BindingFormInput = z.input<typeof bindingSchema>
type BindingForm = z.output<typeof bindingSchema>
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

/**
 * 已绑定代码仓库的查看、维护与解绑。绑定或切换仓库的唯一入口是部署配置编辑器的“来源”。
 */
export function RepositoryBindingsPage({ applicationId, projectId: projectIdProp }: { applicationId?: string, projectId?: string } = {}) {
  const { t } = useTranslation()
  const { projectId: routeProjectId = '' } = useParams()
  const projectId = projectIdProp ?? routeProjectId
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingBinding, setEditingBinding] = useState<RepositoryBinding | null>(null)
  const [bindingToDelete, setBindingToDelete] = useState<RepositoryBinding | null>(null)
  const [branchSearch, setBranchSearch] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const providers = useQuery({ queryKey: ['git-providers', 'project', projectId], queryFn: () => api.listGitProviders(projectId), enabled: Boolean(projectId) })
  const accounts = useQuery({ ...liveObservationQueryPolicy, queryKey: ['git-accounts', 'project', projectId], queryFn: () => api.listGitAccounts(projectId), enabled: Boolean(projectId) })
  const applications = useQuery({
    queryKey: ['applications', projectId],
    queryFn: () => api.listApplications(projectId),
    enabled: Boolean(projectId && !applicationId),
  })
  const bindingsPage = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['repository-bindings', projectId, page, pageSize],
    queryFn: () => api.listRepositoryBindingsPage(projectId, { page, pageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(projectId && !applicationId),
  })
  const allBindings = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['repository-bindings', projectId, 'all'],
    queryFn: () => api.listRepositoryBindings(projectId),
    enabled: Boolean(projectId && (applicationId || dialogOpen)),
  })
  const visibleBindings = useMemo(() => {
    const items = applicationId ? (allBindings.data ?? []) : (bindingsPage.data?.items ?? [])
    return applicationId ? items.filter(binding => binding.applicationId === applicationId) : items
  }, [allBindings.data, applicationId, bindingsPage.data?.items])

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
      autoConfigureWebhook: true,
    },
  })
  const selectedAccountId = form.watch('gitAccountId')
  const selectedApplicationId = applicationId ?? form.watch('applicationId')
  const selectedOwner = form.watch('owner')
  const selectedRepo = form.watch('repo')
  const selectedProviderId = useMemo(() => {
    const selectedAccount = (accounts.data ?? []).find(account => account.id === selectedAccountId)
    return selectedAccount?.providerId ?? editingBinding?.gitProviderId ?? ''
  }, [accounts.data, editingBinding?.gitProviderId, selectedAccountId])
  const duplicateBinding = useMemo(() => {
    if (!selectedApplicationId || !selectedProviderId || !selectedOwner || !selectedRepo)
      return undefined
    return (allBindings.data ?? bindingsPage.data?.items ?? []).find(binding =>
      binding.applicationId === selectedApplicationId
      && binding.gitProviderId === selectedProviderId
      && normalizeRepositoryPart(binding.owner) === normalizeRepositoryPart(selectedOwner)
      && normalizeRepositoryName(binding.repo) === normalizeRepositoryName(selectedRepo)
      && binding.id !== editingBinding?.id,
    )
  }, [allBindings.data, bindingsPage.data?.items, editingBinding?.id, selectedApplicationId, selectedOwner, selectedProviderId, selectedRepo])
  const branches = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['git-branches', selectedAccountId, selectedOwner, selectedRepo, branchSearch],
    queryFn: () => api.listGitBranches(selectedAccountId || '', selectedOwner || '', selectedRepo || '', { search: branchSearch, limit: 50 }),
    enabled: Boolean(selectedAccountId && selectedOwner && selectedRepo),
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
        autoConfigureWebhook: payload.autoConfigureWebhook,
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

  function resetBindingForm(binding?: RepositoryBinding | null) {
    form.reset({
      applicationId: binding?.applicationId ?? applicationId ?? '',
      gitAccountId: binding?.gitAccountId ?? '',
      owner: binding?.owner ?? '',
      repo: binding?.repo ?? '',
      cloneUrl: binding?.cloneUrl ?? '',
      defaultBranch: binding?.defaultBranch ?? 'main',
      autoConfigureWebhook: binding?.webhookEnabled ?? true,
    })
    setBranchSearch('')
  }

  const openEditDialog = (binding: RepositoryBinding) => {
    setEditingBinding(binding)
    resetBindingForm(binding)
    setDialogOpen(true)
  }

  return (
    <div className="grid gap-6">
      {(providers.isError || accounts.isError || bindingsPage.isError || allBindings.isError) && (
        <ErrorState title={t('repositories.loadFailedTitle')} description={t('repositories.loadFailedDescription')} />
      )}

      <DataList
        columns={[
          {
            key: 'repo',
            header: t('apps.repoBinding'),
            render: binding => (
              <div className="flex min-w-0 items-center gap-3">
                <GitBranch aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0">
                  <div className="truncate font-mono text-sm font-medium">{`${binding.owner}/${binding.repo}`}</div>
                  <p className="truncate font-mono text-xs text-muted-foreground">{binding.cloneUrl}</p>
                </div>
              </div>
            ),
          },
          ...(!applicationId
            ? [{ key: 'application', header: t('repositories.application'), render: (binding: RepositoryBinding) => binding.applicationName ?? binding.applicationId }]
            : []),
          { key: 'branch', header: t('repositories.defaultBranch'), width: 'compact', render: binding => <span className="font-mono text-xs">{binding.defaultBranch}</span> },
          { key: 'provider', header: t('codeRepositoriesView.provider'), render: binding => providerBindingLabel(binding, providers.data ?? [], accounts.data ?? []) },
          { key: 'webhook', header: t('repositories.webhookStatus'), render: binding => <StatusValueBadge value={binding.webhookStatus} /> },
          {
            key: 'actions',
            header: t('common.actions'),
            mobileActions: 'inline',
            width: 'actions',
            render: binding => (
              <div className="flex items-center justify-end gap-1">
                {binding.webhookStatus !== 'ready' && (
                  <Button disabled={createWebhook.isPending} size="sm" type="button" variant="outline" onClick={() => createWebhook.mutate(binding.id)}>
                    <LinkIcon size={16} />
                    {t('repositories.createWebhook')}
                  </Button>
                )}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button aria-label={t('common.actions')} size="icon" type="button" variant="ghost">
                      <MoreHorizontal size={16} />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onSelect={() => openEditDialog(binding)}>
                      <Pencil size={16} />
                      {t('repositories.editAria')}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onSelect={() => setBindingToDelete(binding)}>
                      <Unlink size={16} />
                      {t('repositories.deleteAria')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            ),
          },
        ]}
        emptyTitle={t('repositories.emptyTitle')}
        emptyDescription={t('repositories.emptyDescription')}
        items={visibleBindings}
        pagination={!applicationId
          ? {
              page: bindingsPage.data?.page ?? page,
              pageSize: bindingsPage.data?.pageSize ?? pageSize,
              pageSizeOptions: PAGE_SIZE_OPTIONS,
              total: bindingsPage.data?.total ?? 0,
              totalPages: bindingsPage.data?.totalPages ?? 0,
              pageInfoLabel: t('pagination.pageInfo', {
                page: bindingsPage.data?.page ?? page,
                totalPages: bindingsPage.data?.totalPages ?? 0,
                total: bindingsPage.data?.total ?? 0,
              }),
              onPageChange: setPage,
              onPageSizeChange: (nextPageSize) => {
                setPageSize(nextPageSize)
                setPage(1)
              },
            }
          : undefined}
        rowKey={binding => binding.id}
      />

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
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t('repositories.editBinding')}</DialogTitle>
            <DialogDescription>{t('repositories.bindingDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-3"
            onSubmit={form.handleSubmit(values => updateBinding.mutate(values))}
          >
            <div className="flex justify-end">
              <Link className="text-sm text-primary-text hover:underline" to="/code-repositories">{t('repositories.manageCodeRepositories')}</Link>
            </div>
            {applicationId
              ? <input type="hidden" {...form.register('applicationId')} value={applicationId} />
              : (
                  <Field error={form.formState.errors.applicationId?.message} label={t('repositories.application')} required>
                    <Select {...form.register('applicationId')} aria-invalid={Boolean(form.formState.errors.applicationId)}>
                      <option value="">{t('repositories.selectApplication')}</option>
                      {(applications.data ?? []).map(application => (
                        <option key={application.id} value={application.id}>{application.name}</option>
                      ))}
                    </Select>
                  </Field>
                )}
            <GitRepositoryPicker
              accounts={accounts.data ?? []}
              providers={providers.data ?? []}
              value={{
                gitAccountId: form.watch('gitAccountId') || '',
                owner: form.watch('owner') || '',
                repo: form.watch('repo') || '',
                cloneUrl: form.watch('cloneUrl') || '',
                defaultBranch: form.watch('defaultBranch') || 'main',
              }}
              onChange={(next) => {
                form.setValue('gitAccountId', next.gitAccountId, { shouldDirty: true, shouldValidate: true })
                form.setValue('owner', next.owner, { shouldDirty: true, shouldValidate: true })
                form.setValue('repo', next.repo, { shouldDirty: true, shouldValidate: true })
                form.setValue('cloneUrl', next.cloneUrl, { shouldDirty: true, shouldValidate: true })
                form.setValue('defaultBranch', next.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
                setBranchSearch('')
              }}
            />
            {duplicateBinding && (
              <p className="text-sm text-destructive">{t('repositories.duplicateBinding')}</p>
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
            <div className="grid gap-3">
              <Field error={form.formState.errors.cloneUrl?.message} label={t('repositories.cloneUrl')}><Input {...form.register('cloneUrl')} aria-invalid={Boolean(form.formState.errors.cloneUrl)} placeholder={t('repositories.cloneUrlPlaceholder')} /></Field>
              <Controller
                control={form.control}
                name="autoConfigureWebhook"
                render={({ field }) => (
                  <ControlledCheckboxField
                    description={t('repositories.autoConfigureWebhookHint')}
                    field={field}
                  >
                    {t('repositories.autoConfigureWebhook')}
                  </ControlledCheckboxField>
                )}
              />
            </div>
            <DialogFooter>
              <DialogClose asChild>
                <Button disabled={updateBinding.isPending} type="button" variant="outline">{t('common.cancel')}</Button>
              </DialogClose>
              <Button disabled={updateBinding.isPending || (accounts.data ?? []).length === 0 || !form.formState.isValid || Boolean(duplicateBinding)} type="submit">
                <Save size={16} />
                {t('save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function providerBindingLabel(binding: RepositoryBinding, providers: GitProvider[], accounts: GitAccount[]) {
  const provider = providers.find(item => item.id === binding.gitProviderId)
  const account = accounts.find(item => item.id === binding.gitAccountId)

  return (
    <span className="block max-w-64 truncate text-sm text-muted-foreground">
      {provider?.name ?? binding.providerName ?? binding.gitProviderId}
      {' · '}
      {account?.username ?? binding.accountUsername ?? binding.gitAccountId}
      {binding.accountOwnerEmail ? ` · ${binding.accountOwnerName || binding.accountOwnerEmail}` : ''}
    </span>
  )
}

function branchOptions(branches: Array<{ name: string }>, current?: string) {
  const options = branches.map(branch => ({ value: branch.name, label: branch.name }))
  const normalized = current?.trim()
  if (normalized && !options.some(option => option.value === normalized))
    options.unshift({ value: normalized, label: normalized })
  return options
}

function normalizeRepositoryPart(value: string) {
  return value.trim().toLowerCase()
}

function normalizeRepositoryName(value: string) {
  return normalizeRepositoryPart(value).replace(/\.git$/, '')
}
