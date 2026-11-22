import type { UseFormReturn } from 'react-hook-form'
import type { GitAccount, GitBranch, GitProvider } from '@/api'
import { Plus } from 'lucide-react'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { GitRepositoryPicker } from '@/components/common/git-repository-picker'
import { SearchSelect } from '@/components/common/search-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { branchOptions } from '@/pages/applications/application-config-utils'

export interface RepositoryBindingDialogForm {
  autoConfigureWebhook: boolean
  cloneUrl?: string
  defaultBranch?: string
  gitAccountId: string
  owner: string
  providerId?: string
  repo: string
}

export interface RepositoryBindingDialogFormInput {
  autoConfigureWebhook?: boolean
  cloneUrl?: string
  defaultBranch?: string
  gitAccountId: string
  owner: string
  providerId?: string
  repo: string
}

interface ApplicationRepositoryBindingDialogProps {
  accounts: GitAccount[]
  accountsError?: boolean
  accountsLoading?: boolean
  accountsRetrying?: boolean
  branchLimited?: boolean
  branches: GitBranch[]
  branchesError?: boolean
  branchSearch: string
  branchesLoading: boolean
  form: UseFormReturn<RepositoryBindingDialogFormInput, undefined, RepositoryBindingDialogForm>
  open: boolean
  pending: boolean
  providers: GitProvider[]
  providersError?: boolean
  providersLoading?: boolean
  providersRetrying?: boolean
  onBranchSearchChange: (value: string) => void
  onOpenChange: (open: boolean) => void
  onRetryAccounts?: () => void
  onRetryBranches?: () => void
  onRetryProviders?: () => void
  onSubmit: (values: RepositoryBindingDialogForm) => void
}

export function ApplicationRepositoryBindingDialog({
  accounts,
  accountsError = false,
  accountsLoading = false,
  accountsRetrying = false,
  branchLimited,
  branches,
  branchesError = false,
  branchSearch,
  branchesLoading,
  form,
  onBranchSearchChange,
  onOpenChange,
  onRetryAccounts,
  onRetryBranches,
  onRetryProviders,
  onSubmit,
  open,
  pending,
  providers,
  providersError = false,
  providersLoading = false,
  providersRetrying = false,
}: ApplicationRepositoryBindingDialogProps) {
  const { t } = useTranslation()
  const selectedAccountId = form.watch('gitAccountId')
  const selectedOwner = form.watch('owner')
  const selectedRepo = form.watch('repo')
  const branchesEnabled = Boolean(selectedAccountId && selectedOwner && selectedRepo)
  const canSubmitAccount = selectedAccountId === 'anonymous' || (!accountsLoading && accounts.length > 0)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('repositories.bindRepoTitle')}</DialogTitle>
          <DialogDescription>{t('deploymentsPage.repositoryBindingDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          {/* 新开页面管理 Git Provider 与凭据，避免离开尚未保存的部署配置。 */}
          <div className="flex justify-end">
            <Link className="text-sm text-primary-text hover:underline" rel="noreferrer" target="_blank" to="/code-repositories">{t('repositories.manageCodeRepositories')}</Link>
          </div>
          <GitRepositoryPicker
            accounts={accounts}
            accountsError={accountsError}
            accountsLoading={accountsLoading}
            accountsRetrying={accountsRetrying}
            providers={providers}
            providersError={providersError}
            providersLoading={providersLoading}
            providersRetrying={providersRetrying}
            value={{
              cloneUrl: form.watch('cloneUrl') || '',
              defaultBranch: form.watch('defaultBranch') || 'main',
              gitAccountId: selectedAccountId || '',
              owner: selectedOwner || '',
              providerId: form.watch('providerId') || '',
              repo: selectedRepo || '',
            }}
            onChange={(next) => {
              form.setValue('gitAccountId', next.gitAccountId, { shouldDirty: true, shouldValidate: true })
              form.setValue('providerId', next.providerId ?? '', { shouldDirty: true, shouldValidate: true })
              form.setValue('owner', next.owner, { shouldDirty: true, shouldValidate: true })
              form.setValue('repo', next.repo, { shouldDirty: true, shouldValidate: true })
              form.setValue('cloneUrl', next.cloneUrl, { shouldDirty: true, shouldValidate: true })
              form.setValue('defaultBranch', next.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
              onBranchSearchChange('')
            }}
            onRetryAccounts={onRetryAccounts}
            onRetryProviders={onRetryProviders}
          />
          <div className="grid gap-3 md:grid-cols-3">
            <Field error={form.formState.errors.owner?.message} label={t('repositories.owner')} required>
              <Input {...form.register('owner')} aria-invalid={Boolean(form.formState.errors.owner)} placeholder={t('repositories.ownerPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.repo?.message} label={t('repositories.repo')} required>
              <Input {...form.register('repo')} aria-invalid={Boolean(form.formState.errors.repo)} placeholder={t('repositories.repoPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.defaultBranch?.message} label={t('repositories.defaultBranch')}>
              {fieldControl => (
                <div className="grid gap-1.5">
                  <SearchSelect
                    {...fieldControl}
                    disabled={!branchesEnabled}
                    emptyLabel={branchesError ? t('common.unavailable') : t('repositories.noBranches')}
                    error={branchesEnabled && branchesError}
                    limited={branchLimited}
                    loading={branchesLoading}
                    options={branchOptions(branches, form.watch('defaultBranch'))}
                    placeholder={branchesLoading ? t('common.loading') : t('repositories.defaultBranchPlaceholder')}
                    search={branchSearch}
                    value={form.watch('defaultBranch') || ''}
                    onSearchChange={onBranchSearchChange}
                    onValueChange={value => form.setValue('defaultBranch', value, { shouldDirty: true, shouldValidate: true })}
                  />
                  {branchesEnabled && branchesError && onRetryBranches && (
                    <div className="flex items-center justify-between gap-2 text-xs text-danger" role="alert">
                      <span>{t('common.unavailable')}</span>
                      <Button disabled={branchesLoading} size="sm" type="button" variant="outline" onClick={onRetryBranches}>
                        {t('common.retry')}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </Field>
          </div>
          <div className="grid gap-3">
            <Field error={form.formState.errors.cloneUrl?.message} label={t('repositories.cloneUrl')}>
              <Input {...form.register('cloneUrl')} aria-invalid={Boolean(form.formState.errors.cloneUrl)} placeholder={t('repositories.cloneUrlPlaceholder')} />
            </Field>
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
              <Button disabled={pending} type="button" variant="outline">{t('common.cancel')}</Button>
            </DialogClose>
            <Button disabled={pending || !canSubmitAccount || !form.formState.isValid} type="submit">
              <Plus className="size-4" />
              {t('repositories.bindRepository')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
