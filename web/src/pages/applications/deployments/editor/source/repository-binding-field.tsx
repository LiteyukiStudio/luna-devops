import type { DeploymentTargetFormValues } from '../deployment-target-form'
import type { RepositoryBindingDialogForm, RepositoryBindingDialogFormInput } from './application-repository-binding-dialog'
import type { RepositoryBinding } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { FormField } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { DeferredRepositoryBindingDialog } from '@/pages/applications/deployments/application-deployment-dialogs'
import { repositoryBindingItems } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { useFieldError } from '../deployment-target-editor-utils'

const repositoryBindingDefaults: RepositoryBindingDialogFormInput = {
  autoConfigureWebhook: true,
  cloneUrl: '',
  defaultBranch: 'main',
  gitAccountId: '',
  owner: '',
  repo: '',
}

/** 代码仓库选择 + 就地绑定新仓库；绑定成功后自动选中。 */
export function RepositoryBindingField({ applicationId, onRepositoryChange, projectId, repositoryBindings }: {
  applicationId: string
  projectId: string
  repositoryBindings: RepositoryBinding[]
  onRepositoryChange: (binding: RepositoryBinding | undefined) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { register, setValue } = useFormContext<DeploymentTargetFormValues>()
  const error = useFieldError('repositoryBindingId')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [branchSearch, setBranchSearch] = useState('')
  const schema = useMemo(() => z.object({
    autoConfigureWebhook: z.boolean().default(true),
    cloneUrl: z.string().optional(),
    defaultBranch: z.string().optional(),
    gitAccountId: z.string().min(1, t('repositories.gitAccountRequired')),
    owner: z.string().min(1, t('repositories.ownerRequired')),
    repo: z.string().min(1, t('repositories.repoRequired')),
  }), [t])
  const bindingForm = useForm<RepositoryBindingDialogFormInput, undefined, RepositoryBindingDialogForm>({
    defaultValues: repositoryBindingDefaults,
    mode: 'onChange',
    resolver: zodResolver(schema),
  })
  const accountId = bindingForm.watch('gitAccountId')
  const owner = bindingForm.watch('owner')
  const repo = bindingForm.watch('repo')
  const gitProviders = useQuery({ queryKey: ['git-providers', 'project', projectId], queryFn: () => api.listGitProviders(projectId), enabled: dialogOpen })
  const gitAccounts = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['git-accounts', 'project', projectId],
    queryFn: () => api.listGitAccounts(projectId),
    enabled: dialogOpen,
  })
  const branches = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['git-branches', accountId, owner, repo, branchSearch],
    queryFn: () => api.listGitBranches(accountId || '', owner || '', repo || '', { search: branchSearch, limit: 50 }),
    enabled: Boolean(dialogOpen && accountId && owner && repo),
  })
  const registration = register('repositoryBindingId')

  const closeDialog = () => {
    setDialogOpen(false)
    setBranchSearch('')
    bindingForm.reset(repositoryBindingDefaults)
  }

  const createBinding = useMutation({
    mutationFn: (values: RepositoryBindingDialogForm) => api.createRepositoryBinding(projectId, {
      applicationId,
      autoConfigureWebhook: values.autoConfigureWebhook,
      cloneUrl: values.cloneUrl ?? '',
      defaultBranch: values.defaultBranch || 'main',
      gitAccountId: values.gitAccountId,
      owner: values.owner,
      repo: values.repo,
    }),
    onSuccess: (binding) => {
      toast.success(t('repositories.bindingSaved'))
      queryClient.setQueryData<RepositoryBinding[]>(['repository-bindings', projectId, applicationId], items => [
        ...repositoryBindingItems(items).filter(item => item.id !== binding.id),
        binding,
      ])
      setValue('repositoryBindingId', binding.id, { shouldDirty: true, shouldValidate: true })
      onRepositoryChange(binding)
      closeDialog()
      void queryClient.invalidateQueries({ queryKey: ['repository-bindings', projectId] })
      void queryClient.invalidateQueries({ queryKey: ['applications', projectId] })
    },
    onError: mutationError => toast.error(mutationError.message),
  })

  return (
    <FormField error={error} label={t('apps.repository')} required>
      {control => (
        <div className="flex flex-col gap-2 sm:flex-row">
          <NativeSelect
            {...registration}
            {...control}
            containerClassName="min-w-0 flex-1"
            onChange={(event) => {
              void registration.onChange(event)
              onRepositoryChange(repositoryBindings.find(binding => binding.id === event.target.value))
            }}
          >
            <option value="">{t('common.select')}</option>
            {repositoryBindings.map(binding => (
              <option key={binding.id} value={binding.id}>{`${binding.owner}/${binding.repo}`}</option>
            ))}
          </NativeSelect>
          <Button className="shrink-0" type="button" variant="outline" onClick={() => setDialogOpen(true)}>
            <Plus className="size-4" />
            {t('deploymentsPage.bindRepositoryInTarget')}
          </Button>
          <DeferredRepositoryBindingDialog
            accounts={gitAccounts.data ?? []}
            branchLimited={branches.data?.limited}
            branches={branches.data?.items ?? []}
            branchSearch={branchSearch}
            branchesLoading={branches.isFetching}
            form={bindingForm}
            open={dialogOpen}
            pending={createBinding.isPending}
            providers={gitProviders.data ?? []}
            onBranchSearchChange={setBranchSearch}
            onOpenChange={open => open ? setDialogOpen(true) : closeDialog()}
            onSubmit={values => createBinding.mutate(values)}
          />
        </div>
      )}
    </FormField>
  )
}
