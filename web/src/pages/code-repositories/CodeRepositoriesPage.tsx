import type { CredentialForm, ProviderForm } from './code-repositories-form-model'
import type { GitAccount, GitProvider } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { motion } from 'motion/react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { ErrorState } from '@/components/common/error-state'
import { Button } from '@/components/ui/button'
import { TabsContent } from '@/components/ui/tabs'
import { CredentialDialog, ProviderDialog } from './code-repositories-dialogs'
import { credentialDefaults, credentialSchema, providerDefaults, providerSchema } from './code-repositories-form-model'
import {
  CredentialsPanel,
  ProvidersPanel,
} from './code-repositories-panels'
import { normalizeGitBaseUrl, splitText } from './code-repositories-utils'

export function CodeRepositoriesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { user } = useSession()
  const [activeTab, setActiveTab] = useState('providers')
  const [providerDialogOpen, setProviderDialogOpen] = useState(false)
  const [credentialDialogOpen, setCredentialDialogOpen] = useState(false)
  const [editingProvider, setEditingProvider] = useState<GitProvider | null>(null)
  const [editingCredential, setEditingCredential] = useState<GitAccount | null>(null)
  const [providerToDelete, setProviderToDelete] = useState<GitProvider | null>(null)
  const [credentialToDelete, setCredentialToDelete] = useState<GitAccount | null>(null)
  const [providerPage, setProviderPage] = useState(1)
  const [providerPageSize, setProviderPageSize] = useState(10)
  const [credentialPage, setCredentialPage] = useState(1)
  const [credentialPageSize, setCredentialPageSize] = useState(10)
  const providers = useQuery({
    queryKey: ['git-providers', providerPage, providerPageSize],
    queryFn: () => api.listGitProvidersPage({ page: providerPage, pageSize: providerPageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const credentials = useQuery({
    queryKey: ['git-accounts', credentialPage, credentialPageSize],
    queryFn: () => api.listGitAccountsPage({ page: credentialPage, pageSize: credentialPageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const providerOptions = useQuery({ queryKey: ['git-providers', 'options'], queryFn: () => api.listGitProviders() })
  const providerOptionItems = useMemo(() => providerOptions.data ?? [], [providerOptions.data])
  const credentialItems = useMemo(() => credentials.data?.items ?? [], [credentials.data?.items])
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const canManageProviders = user?.permissions.includes('user.manage')
  const projectMap = useMemo(() => {
    const map: Record<string, string> = {}
    for (const project of projects.data ?? [])
      map[project.id] = project.name
    return map
  }, [projects.data])

  const providerForm = useForm<ProviderForm>({
    resolver: zodResolver(providerSchema),
    mode: 'onChange',
    defaultValues: providerDefaults,
  })
  const credentialForm = useForm<CredentialForm>({
    resolver: zodResolver(credentialSchema),
    mode: 'onChange',
    defaultValues: credentialDefaults,
  })
  const providerType = providerForm.watch('type')
  const isGithubProvider = providerType === 'github'
  const providerScope = providerForm.watch('scope')
  const credentialScope = credentialForm.watch('scope')
  const hasGithubProvider = providerOptionItems.some(provider => provider.type === 'github')
  const hasAnotherGithubProvider = useMemo(() => {
    if (!editingProvider)
      return hasGithubProvider
    return providerOptionItems.some(provider => provider.type === 'github' && provider.id !== editingProvider.id)
  }, [editingProvider, hasGithubProvider, providerOptionItems])

  useEffect(() => {
    if (!editingProvider) {
      providerForm.reset(providerDefaults)
      return
    }
    providerForm.reset({
      authType: editingProvider.authType === 'pat' ? 'pat' : 'oauth',
      baseUrl: editingProvider.baseUrl,
      scope: editingProvider.scope ?? 'user',
      ownerRef: editingProvider.ownerRef,
      projectIds: editingProvider.projectIds ?? [],
      clientId: editingProvider.clientId,
      clientSecret: '',
      enabled: editingProvider.enabled,
      name: editingProvider.name,
      type: editingProvider.type,
    })
  }, [editingProvider, providerForm])

  useEffect(() => {
    if (!editingCredential) {
      credentialForm.reset(credentialDefaults)
      return
    }
    credentialForm.reset({
      accessToken: '',
      avatarUrl: editingCredential.avatarUrl ?? '',
      externalUserId: editingCredential.externalUserId ?? '',
      ownerRef: editingCredential.ownerRef,
      projectIds: editingCredential.projectIds ?? [],
      providerId: editingCredential.providerId,
      refreshToken: '',
      scope: editingCredential.scope,
      scopesText: editingCredential.scopes,
      status: editingCredential.status,
      username: editingCredential.username,
    })
  }, [credentialForm, editingCredential])

  useEffect(() => {
    if (isGithubProvider) {
      providerForm.setValue('baseUrl', normalizeGitBaseUrl('github'), { shouldDirty: true, shouldValidate: true })
      providerForm.setValue('scope', 'global', { shouldDirty: true, shouldValidate: true })
      providerForm.setValue('ownerRef', '', { shouldDirty: true, shouldValidate: true })
      providerForm.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
    }
  }, [isGithubProvider, providerForm])

  useEffect(() => {
    if (providerScope !== 'project')
      providerForm.setValue('projectIds', [])
  }, [providerScope, providerForm])

  useEffect(() => {
    if (credentialScope !== 'project')
      credentialForm.setValue('projectIds', [])
  }, [credentialScope, credentialForm])

  const saveProvider = useMutation({
    mutationFn: (payload: ProviderForm) => {
      const providerPayload = {
        authType: payload.authType,
        baseUrl: normalizeGitBaseUrl(payload.type, payload.baseUrl),
        clientId: payload.clientId ?? '',
        clientSecret: payload.clientSecret ?? '',
        enabled: payload.enabled,
        name: payload.name,
        scope: payload.scope,
        ownerRef: '',
        projectIds: payload.scope === 'project' ? payload.projectIds : [],
        type: payload.type,
      }
      if (editingProvider)
        return api.updateGitProvider(editingProvider.id, providerPayload)
      return api.createGitProvider(providerPayload)
    },
    onSuccess: () => {
      toast.success(t(editingProvider ? 'codeRepositoriesView.providerUpdated' : 'codeRepositoriesView.providerCreated'))
      setProviderDialogOpen(false)
      setEditingProvider(null)
      providerForm.reset(providerDefaults)
      queryClient.invalidateQueries({ queryKey: ['git-providers'] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteProvider = useMutation({
    mutationFn: api.deleteGitProvider,
    onSuccess: () => {
      toast.success(t('codeRepositoriesView.providerDeleted'))
      setProviderToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['git-providers'] })
    },
    onError: error => toast.error(error.message),
  })

  const saveCredential = useMutation({
    mutationFn: (payload: CredentialForm) => {
      const accountPayload = {
        accessToken: payload.accessToken ?? '',
        avatarUrl: payload.avatarUrl ?? '',
        ownerRef: '',
        projectIds: payload.scope === 'project' ? payload.projectIds : [],
        externalUserId: payload.externalUserId ?? '',
        providerId: payload.providerId,
        refreshToken: payload.refreshToken ?? '',
        scope: payload.scope,
        scopes: splitText(payload.scopesText),
        status: payload.status,
        username: payload.username,
      }
      if (editingCredential)
        return api.updateGitAccount(editingCredential.id, accountPayload)
      return api.createGitAccount(accountPayload)
    },
    onSuccess: () => {
      toast.success(t(editingCredential ? 'codeRepositoriesView.credentialUpdated' : 'codeRepositoriesView.credentialCreated'))
      setCredentialDialogOpen(false)
      setEditingCredential(null)
      credentialForm.reset(credentialDefaults)
      queryClient.invalidateQueries({ queryKey: ['git-accounts'] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteCredential = useMutation({
    mutationFn: api.deleteGitAccount,
    onSuccess: () => {
      toast.success(t('codeRepositoriesView.credentialDeleted'))
      setCredentialToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['git-accounts'] })
    },
    onError: error => toast.error(error.message),
  })

  const refreshCredential = useMutation({
    mutationFn: api.refreshGitAccount,
    onSuccess: () => {
      toast.success(t('codeRepositoriesView.credentialReloaded'))
      queryClient.invalidateQueries({ queryKey: ['git-accounts'] })
    },
    onError: error => toast.error(error.message),
  })

  return (
    <div className="grid gap-6">
      {(providers.isError || credentials.isError) && (
        <ErrorState title={t('codeRepositoriesView.loadFailedTitle')} description={t('codeRepositoriesView.loadFailedDescription')} />
      )}

      <ContentTabs
        tabs={[
          { value: 'providers', label: t('codeRepositoriesView.providersTab') },
          { value: 'credentials', label: t('codeRepositoriesView.credentialsTab') },
        ]}
        tools={(
          activeTab === 'providers'
            ? (
                canManageProviders
                  ? (
                      <Button
                        onClick={() => {
                          setEditingProvider(null)
                          providerForm.reset(providerDefaults)
                          setProviderDialogOpen(true)
                        }}
                      >
                        <Plus size={16} />
                        {t('codeRepositoriesView.createProvider')}
                      </Button>
                    )
                  : undefined
              )
            : (
                <Button
                  onClick={() => {
                    setEditingCredential(null)
                    credentialForm.reset(credentialDefaults)
                    setCredentialDialogOpen(true)
                  }}
                >
                  <Plus size={16} />
                  {t('codeRepositoriesView.createCredential')}
                </Button>
              )
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value={activeTab}>
          <motion.div
            key={activeTab}
            animate={{ opacity: 1, y: 0 }}
            initial={{ opacity: 0, y: 6 }}
            transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
          >
            {activeTab === 'providers'
              ? (
                  <ProvidersPanel
                    canManage={Boolean(canManageProviders)}
                    page={providers.data?.page ?? providerPage}
                    pageSize={providers.data?.pageSize ?? providerPageSize}
                    providers={providerOptionItems}
                    projectMap={projectMap}
                    total={providers.data?.total ?? 0}
                    totalPages={providers.data?.totalPages ?? 0}
                    onDelete={setProviderToDelete}
                    onEdit={(provider) => {
                      setEditingProvider(provider)
                      setProviderDialogOpen(true)
                    }}
                    onPageChange={setProviderPage}
                    onPageSizeChange={(pageSize) => {
                      setProviderPageSize(pageSize)
                      setProviderPage(1)
                    }}
                  />
                )
              : (
                  <CredentialsPanel
                    credentials={credentialItems}
                    page={credentials.data?.page ?? credentialPage}
                    pageSize={credentials.data?.pageSize ?? credentialPageSize}
                    providers={providerOptionItems}
                    projectMap={projectMap}
                    refreshPending={refreshCredential.isPending}
                    total={credentials.data?.total ?? 0}
                    totalPages={credentials.data?.totalPages ?? 0}
                    onDelete={setCredentialToDelete}
                    onEdit={(credential) => {
                      setEditingCredential(credential)
                      setCredentialDialogOpen(true)
                    }}
                    onPageChange={setCredentialPage}
                    onPageSizeChange={(pageSize) => {
                      setCredentialPageSize(pageSize)
                      setCredentialPage(1)
                    }}
                    onRefresh={credential => refreshCredential.mutate(credential.id)}
                  />
                )}
          </motion.div>
        </TabsContent>
      </ContentTabs>

      <ProviderDialog
        open={providerDialogOpen}
        editingProvider={editingProvider}
        form={providerForm}
        hasAnotherGithubProvider={hasAnotherGithubProvider}
        pending={saveProvider.isPending}
        projects={projects.data ?? []}
        onSubmit={values => saveProvider.mutate(values)}
        onOpenChange={(open) => {
          setProviderDialogOpen(open)
          if (!open)
            setEditingProvider(null)
        }}
      />

      <CredentialDialog
        open={credentialDialogOpen}
        editingCredential={editingCredential}
        form={credentialForm}
        pending={saveCredential.isPending}
        projects={projects.data ?? []}
        providers={providerOptionItems}
        onOpenChange={(open) => {
          setCredentialDialogOpen(open)
          if (!open)
            setEditingCredential(null)
        }}
        onSubmit={values => saveCredential.mutate(values)}
      />

      <ConfirmDialog
        confirmText={t('codeRepositoriesView.deleteProviderConfirm')}
        description={t('codeRepositoriesView.deleteProviderDescription', { name: providerToDelete?.name ?? '' })}
        open={Boolean(providerToDelete)}
        pending={deleteProvider.isPending}
        title={t('codeRepositoriesView.deleteProviderTitle')}
        onConfirm={() => providerToDelete && deleteProvider.mutate(providerToDelete.id)}
        onOpenChange={open => !open && setProviderToDelete(null)}
      />
      <ConfirmDialog
        confirmText={t('codeRepositoriesView.deleteCredentialConfirm')}
        description={t('codeRepositoriesView.deleteCredentialDescription', { name: credentialToDelete?.username ?? '' })}
        open={Boolean(credentialToDelete)}
        pending={deleteCredential.isPending}
        title={t('codeRepositoriesView.deleteCredentialTitle')}
        onConfirm={() => credentialToDelete && deleteCredential.mutate(credentialToDelete.id)}
        onOpenChange={open => !open && setCredentialToDelete(null)}
      />
    </div>
  )
}
