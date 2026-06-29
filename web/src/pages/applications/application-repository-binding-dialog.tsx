import type { UseFormReturn } from 'react-hook-form'
import type { GitAccount, GitBranch, GitProvider } from '@/api'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { GitRepositoryPicker } from '@/components/common/git-repository-picker'
import { SearchSelect } from '@/components/common/search-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { branchOptions } from './application-config-utils'

export interface RepositoryBindingDialogForm {
  autoConfigureWebhook: boolean
  cloneUrl?: string
  defaultBranch?: string
  gitAccountId: string
  owner: string
  repo: string
  webhookStatus: 'pending' | 'created' | 'disabled' | 'failed'
}

export interface RepositoryBindingDialogFormInput {
  autoConfigureWebhook?: boolean
  cloneUrl?: string
  defaultBranch?: string
  gitAccountId: string
  owner: string
  repo: string
  webhookStatus: 'pending' | 'created' | 'disabled' | 'failed'
}

interface ApplicationRepositoryBindingDialogProps {
  accounts: GitAccount[]
  branchLimited?: boolean
  branches: GitBranch[]
  branchSearch: string
  branchesLoading: boolean
  form: UseFormReturn<RepositoryBindingDialogFormInput, undefined, RepositoryBindingDialogForm>
  open: boolean
  pending: boolean
  providers: GitProvider[]
  onBranchSearchChange: (value: string) => void
  onOpenChange: (open: boolean) => void
  onSubmit: (values: RepositoryBindingDialogForm) => void
}

export function ApplicationRepositoryBindingDialog({
  accounts,
  branchLimited,
  branches,
  branchSearch,
  branchesLoading,
  form,
  onBranchSearchChange,
  onOpenChange,
  onSubmit,
  open,
  pending,
  providers,
}: ApplicationRepositoryBindingDialogProps) {
  const { t } = useTranslation()
  const selectedAccountId = form.watch('gitAccountId')
  const selectedOwner = form.watch('owner')
  const selectedRepo = form.watch('repo')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('repositories.bindRepoTitle')}</DialogTitle>
          <DialogDescription>{t('deploymentsPage.repositoryBindingDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <GitRepositoryPicker
            accounts={accounts}
            providers={providers}
            value={{
              cloneUrl: form.watch('cloneUrl') || '',
              defaultBranch: form.watch('defaultBranch') || 'main',
              gitAccountId: selectedAccountId || '',
              owner: selectedOwner || '',
              repo: selectedRepo || '',
            }}
            onChange={(next) => {
              form.setValue('gitAccountId', next.gitAccountId, { shouldDirty: true, shouldValidate: true })
              form.setValue('owner', next.owner, { shouldDirty: true, shouldValidate: true })
              form.setValue('repo', next.repo, { shouldDirty: true, shouldValidate: true })
              form.setValue('cloneUrl', next.cloneUrl, { shouldDirty: true, shouldValidate: true })
              form.setValue('defaultBranch', next.defaultBranch || 'main', { shouldDirty: true, shouldValidate: true })
              onBranchSearchChange('')
            }}
          />
          <div className="grid gap-3 md:grid-cols-3">
            <Field error={form.formState.errors.owner?.message} label={t('repositories.owner')} required>
              <Input {...form.register('owner')} aria-invalid={Boolean(form.formState.errors.owner)} placeholder={t('repositories.ownerPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.repo?.message} label={t('repositories.repo')} required>
              <Input {...form.register('repo')} aria-invalid={Boolean(form.formState.errors.repo)} placeholder={t('repositories.repoPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.defaultBranch?.message} label={t('repositories.defaultBranch')}>
              <SearchSelect
                disabled={!selectedAccountId || !selectedOwner || !selectedRepo}
                emptyLabel={t('repositories.noBranches')}
                limited={branchLimited}
                loading={branchesLoading}
                options={branchOptions(branches, form.watch('defaultBranch'))}
                placeholder={t('repositories.defaultBranchPlaceholder')}
                search={branchSearch}
                value={form.watch('defaultBranch') || ''}
                onSearchChange={onBranchSearchChange}
                onValueChange={value => form.setValue('defaultBranch', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <Field error={form.formState.errors.cloneUrl?.message} label={t('repositories.cloneUrl')}>
              <Input {...form.register('cloneUrl')} aria-invalid={Boolean(form.formState.errors.cloneUrl)} placeholder={t('repositories.cloneUrlPlaceholder')} />
            </Field>
            <CheckboxField
              className="rounded-md border border-border bg-muted/30 p-3"
              description={t('repositories.autoConfigureWebhookHint')}
              {...form.register('autoConfigureWebhook')}
            >
              {t('repositories.autoConfigureWebhook')}
            </CheckboxField>
          </div>
          <DialogFooter>
            <Button disabled={pending || accounts.length === 0 || !form.formState.isValid} type="submit">
              <Plus className="size-4" />
              {t('repositories.saveBinding')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
