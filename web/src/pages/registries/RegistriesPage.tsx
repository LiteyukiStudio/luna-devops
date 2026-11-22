import type { CredentialForm, CredentialWithRegistry, ImageForm, RegistryForm } from './registry-form-model'
import type { ArtifactRegistry, RegistryCredential, RegistryRepositoryItem } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, PackagePlus, Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { PageShell } from '@/components/common/page-shell'
import { ResultVisibilitySelect } from '@/components/common/result-visibility-select'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { TabsContent } from '@/components/ui/tabs'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { isPlatformAdmin } from '@/lib/roles'
import { useResultVisibility } from '@/lib/use-result-visibility'
import { CredentialDialog, ImageDialog, RegistryDialog } from './registry-dialogs'
import { credentialDefaults, credentialSchema, imageSchema, registryDefaults, registrySchema, splitText } from './registry-form-model'
import { CredentialsPanel, ImagesPanel, RegistriesPanel } from './registry-list-panels'

export function RegistriesPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [editingRegistry, setEditingRegistry] = useState<ArtifactRegistry | null>(null)
  const [editingCredential, setEditingCredential] = useState<CredentialWithRegistry | null>(null)
  const [registryToDelete, setRegistryToDelete] = useState<ArtifactRegistry | null>(null)
  const [credentialToDelete, setCredentialToDelete] = useState<RegistryCredential | null>(null)
  const [credentialRegistryFilterId, setCredentialRegistryFilterId] = useState('')
  const [activeTab, setActiveTab] = useState('registries')
  const [registryDialogOpen, setRegistryDialogOpen] = useState(false)
  const [credentialDialogOpen, setCredentialDialogOpen] = useState(false)
  const [imageDialogOpen, setImageDialogOpen] = useState(false)
  const [registryPage, setRegistryPage] = useState(1)
  const [registryPageSize, setRegistryPageSize] = useState(10)
  const [credentialPage, setCredentialPage] = useState(1)
  const [credentialPageSize, setCredentialPageSize] = useState(10)
  const [imagePage, setImagePage] = useState(1)
  const [imagePageSize, setImagePageSize] = useState(20)
  const [imageRepositorySearch, setImageRepositorySearch] = useState('')
  const [imageRepositoryResultsOpen, setImageRepositoryResultsOpen] = useState(false)
  const canViewAll = isPlatformAdmin(user?.role)
  const [effectiveVisibility, setVisibility] = useResultVisibility(canViewAll)
  const projects = useQuery({ queryKey: ['projects', 'options', effectiveVisibility], queryFn: () => api.listProjects(effectiveVisibility) })
  const registries = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['registries', registryPage, registryPageSize, effectiveVisibility],
    queryFn: () => api.listRegistriesPage({ page: registryPage, pageSize: registryPageSize, visibility: effectiveVisibility, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const registryItems = registries.data?.items ?? []
  const registryOptions = useQuery({ ...liveObservationQueryPolicy, queryKey: ['registries', 'options', effectiveVisibility], queryFn: () => api.listRegistries(undefined, effectiveVisibility) })
  const images = useQuery({
    queryKey: ['container-images', imagePage, imagePageSize, effectiveVisibility],
    queryFn: () => api.listContainerImages({ page: imagePage, pageSize: imagePageSize, visibility: effectiveVisibility, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const projectMap = useMemo(() => Object.fromEntries((projects.data ?? []).map(project => [project.id, project])), [projects.data])
  const allCredentials = useQuery({
    queryKey: ['registry-credentials', 'all', credentialPage, credentialPageSize, effectiveVisibility],
    queryFn: () => api.listAllRegistryCredentialsPage({ page: credentialPage, pageSize: credentialPageSize, visibility: effectiveVisibility, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const credentials = useQuery({
    queryKey: ['registry-credentials', credentialRegistryFilterId, credentialPage, credentialPageSize, effectiveVisibility],
    queryFn: () => api.listRegistryCredentialsPage(credentialRegistryFilterId, { page: credentialPage, pageSize: credentialPageSize, visibility: effectiveVisibility, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(credentialRegistryFilterId),
  })

  const defaultRegistryScope: RegistryForm['scope'] = canViewAll ? 'global' : 'user'
  const registryForm = useForm<RegistryForm>({
    resolver: zodResolver(registrySchema),
    mode: 'onChange',
    defaultValues: { ...registryDefaults, scope: defaultRegistryScope },
  })
  const credentialForm = useForm<CredentialForm>({
    resolver: zodResolver(credentialSchema),
    mode: 'onChange',
    defaultValues: credentialDefaults,
  })
  const imageForm = useForm<ImageForm>({
    resolver: zodResolver(imageSchema),
    mode: 'onChange',
    defaultValues: {
      projectId: '',
      applicationId: '',
      registryId: '',
      repository: '',
      tag: 'latest',
      digest: '',
      sourceCommit: '',
      buildRunId: '',
      sourceType: 'manual-image',
      scanStatus: 'unknown',
    },
  })
  const imageRegistryId = imageForm.watch('registryId')
  const imageRepository = imageForm.watch('repository')
  const imageRepositoryResults = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['registry-repositories', imageRegistryId, imageRepositorySearch],
    queryFn: () => api.searchRegistryRepositories(imageRegistryId, { search: imageRepositorySearch, page: 1, pageSize: 10 }),
    enabled: Boolean(imageRegistryId && imageRepositorySearch.trim().length >= 2),
  })
  const imageTags = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['registry-tags', imageRegistryId, imageRepository],
    queryFn: () => api.listRegistryRepositoryTags(imageRegistryId, imageRepository, 20),
    enabled: Boolean(imageRegistryId && imageRepository.trim()),
  })

  const saveRegistry = useMutation({
    mutationFn: (values: RegistryForm) => {
      const payload = {
        name: values.name,
        provider: values.provider,
        endpoint: values.endpoint,
        scope: values.scope,
        ownerRef: '',
        projectIds: values.scope === 'project' ? values.projectIds : [],
        isDefault: values.isDefault,
        capabilities: splitText(values.capabilitiesText),
      }
      if (editingRegistry)
        return api.updateRegistry(editingRegistry.id, payload)
      return api.createRegistry(payload)
    },
    onSuccess: () => {
      toast.success(editingRegistry ? t('registriesPage.registryUpdated') : t('registriesPage.registryCreated'))
      setRegistryDialogOpen(false)
      setEditingRegistry(null)
      registryForm.reset({ ...registryDefaults, scope: defaultRegistryScope })
      queryClient.invalidateQueries({ queryKey: ['registries'] })
    },
    onError: error => toast.error(error.message),
  })

  const saveCredential = useMutation({
    mutationFn: (values: CredentialForm) => {
      const payload = {
        ...values,
        projectIds: values.scope === 'project' ? values.projectIds : [],
      }
      if (editingCredential)
        return api.updateRegistryCredential(values.registryId, editingCredential.id, payload)
      return api.createRegistryCredential(values.registryId, payload)
    },
    onSuccess: (_, values) => {
      toast.success(t('registriesPage.credentialSaved'))
      setCredentialDialogOpen(false)
      setEditingCredential(null)
      credentialForm.reset({ ...credentialDefaults, registryId: values.registryId })
      queryClient.invalidateQueries({ queryKey: ['registry-credentials', values.registryId] })
      queryClient.invalidateQueries({ queryKey: ['registry-credentials', 'all'] })
      queryClient.invalidateQueries({ queryKey: ['registries'] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteRegistry = useMutation({
    mutationFn: (registryId: string) => api.deleteRegistry(registryId),
    onSuccess: () => {
      toast.success(t('registriesPage.registryDeleted'))
      setRegistryToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['registries'] })
    },
    onError: error => toast.error(error.message),
  })

  const deleteCredential = useMutation({
    mutationFn: ({ registryId, credentialId }: { registryId: string, credentialId: string }) => api.deleteRegistryCredential(registryId, credentialId),
    onSuccess: (_, values) => {
      toast.success(t('registriesPage.credentialDeleted'))
      setCredentialToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['registry-credentials', values.registryId] })
      queryClient.invalidateQueries({ queryKey: ['registry-credentials', 'all'] })
      queryClient.invalidateQueries({ queryKey: ['registries'] })
    },
    onError: error => toast.error(error.message),
  })

  const testRegistry = useMutation({
    mutationFn: api.testRegistry,
    onSuccess: (result) => {
      if (result.success) {
        toast.success(result.message)
        return
      }
      toast.error(result.message)
    },
    onError: error => toast.error(error.message),
  })

  const createImage = useMutation({
    mutationFn: api.createContainerImage,
    onSuccess: () => {
      toast.success(t('registriesPage.imageCreated'))
      setImageDialogOpen(false)
      imageForm.reset()
      queryClient.invalidateQueries({ queryKey: ['container-images'] })
    },
    onError: error => toast.error(error.message),
  })
  const beginEdit = (registry: ArtifactRegistry) => {
    setEditingRegistry(registry)
    registryForm.reset({
      name: registry.name,
      provider: registry.provider,
      endpoint: registry.endpoint,
      scope: registry.scope,
      ownerRef: registry.ownerRef,
      projectIds: registry.projectIds ?? [],
      isDefault: registry.isDefault,
      capabilitiesText: registry.capabilities.join(', '),
    })
    setRegistryDialogOpen(true)
  }

  const beginEditCredential = (credential: CredentialWithRegistry) => {
    setEditingCredential(credential)
    credentialForm.reset({
      registryId: credential.registryId,
      name: credential.name,
      username: credential.username,
      password: '',
      token: '',
      usage: credential.usage,
      scope: credential.scope,
      projectIds: credential.projectIds ?? [],
      repositoryTemplate: credential.repositoryTemplate,
      tagTemplate: credential.tagTemplate,
    })
    setCredentialDialogOpen(true)
  }

  const registryLookupItems = registryOptions.data ?? registryItems
  const registriesLookupLoading = registryOptions.isPending && registries.isPending
  const selectedRegistry = registryLookupItems.find(registry => registry.id === credentialRegistryFilterId)
  const credentialPageData = credentialRegistryFilterId
    ? credentials.data
      ? {
          ...credentials.data,
          items: credentials.data.items.map(credential => ({
            ...credential,
            registryName: selectedRegistry?.name ?? (registryOptions.isPending ? '' : credential.registryId),
          })),
        }
      : undefined
    : allCredentials.data
      ? {
          ...allCredentials.data,
          items: allCredentials.data.items.map(credential => ({
            ...credential,
            registryName: registryLookupItems.find(registry => registry.id === credential.registryId)?.name
              ?? (registryOptions.isPending ? '' : credential.registryId),
          })),
        }
      : undefined
  const visibleCredentials: CredentialWithRegistry[] = credentialPageData?.items ?? []

  const selectImageRepository = (repository: RegistryRepositoryItem) => {
    imageForm.setValue('repository', repository.name, { shouldDirty: true, shouldValidate: true })
    imageForm.setValue('tag', '', { shouldDirty: true, shouldValidate: true })
    setImageRepositorySearch(repository.name)
    setImageRepositoryResultsOpen(false)
  }

  const visibilitySelect = (
    <ResultVisibilitySelect
      canViewAll={canViewAll}
      containerClassName="w-full @lg/data-list:w-36"
      value={effectiveVisibility}
      onChange={(nextVisibility) => {
        setVisibility(nextVisibility)
        setRegistryPage(1)
        setCredentialPage(1)
        setImagePage(1)
        setCredentialRegistryFilterId('')
      }}
    />
  )

  const openCreateRegistryDialog = () => {
    setEditingRegistry(null)
    registryForm.reset({ ...registryDefaults, scope: canViewAll ? 'global' : 'user' })
    setRegistryDialogOpen(true)
  }

  const openCreateCredentialDialog = () => {
    setEditingCredential(null)
    credentialForm.reset({ ...credentialDefaults, registryId: credentialRegistryFilterId })
    credentialForm.setValue('registryId', credentialRegistryFilterId, { shouldValidate: true })
    credentialForm.setValue('scope', 'user', { shouldValidate: true })
    setCredentialDialogOpen(true)
  }

  const openRecordImageDialog = () => {
    imageForm.setValue('registryId', credentialRegistryFilterId, { shouldValidate: true })
    setImageDialogOpen(true)
  }

  return (
    <PageShell spacing="compact" width="full">
      <ContentTabs
        tabs={[
          { value: 'registries', label: t('registriesPage.registriesTab') },
          { value: 'credentials', label: t('registriesPage.credentialsTab') },
          { value: 'images', label: t('registriesPage.imagesTab') },
        ]}
        tools={(
          <>
            {activeTab === 'registries' && (
              <Button onClick={openCreateRegistryDialog}>
                <Plus />
                {t('registriesPage.createRegistry')}
              </Button>
            )}
            {activeTab === 'credentials' && (
              <Button onClick={openCreateCredentialDialog}>
                <KeyRound />
                {t('registriesPage.createCredentialTitle')}
              </Button>
            )}
            {activeTab === 'images' && (
              <Button onClick={openRecordImageDialog}>
                <PackagePlus />
                {t('registriesPage.recordImage')}
              </Button>
            )}
          </>
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >

        <TabsContent value="registries">
          <RegistriesPanel
            canManageGlobal={canViewAll}
            error={registries.isError
              ? {
                  onRetry: () => void registries.refetch(),
                  retrying: registries.isFetching,
                }
              : undefined}
            items={registryItems}
            loading={registries.isPending}
            toolbar={visibilitySelect}
            projectMap={projectMap}
            projectsLoading={projects.isPending}
            pagination={{
              data: registries.data,
              page: registryPage,
              pageSize: registryPageSize,
              onPageChange: setRegistryPage,
              onPageSizeChange: (nextPageSize) => {
                setRegistryPageSize(nextPageSize)
                setRegistryPage(1)
              },
            }}
            testing={testRegistry.isPending}
            onCreate={openCreateRegistryDialog}
            onSelectCredentials={(registryId) => {
              setCredentialRegistryFilterId(registryId)
              setCredentialPage(1)
              setActiveTab('credentials')
            }}
            onEdit={beginEdit}
            onTest={registryId => testRegistry.mutate(registryId)}
            onDelete={setRegistryToDelete}
          />
        </TabsContent>

        <TabsContent value="credentials">
          <CredentialsPanel
            canManageGlobal={canViewAll}
            error={(credentialRegistryFilterId ? credentials.isError : allCredentials.isError)
              ? {
                  onRetry: () => void (credentialRegistryFilterId ? credentials.refetch() : allCredentials.refetch()),
                  retrying: credentialRegistryFilterId ? credentials.isFetching : allCredentials.isFetching,
                }
              : undefined}
            items={visibleCredentials}
            loading={credentialRegistryFilterId ? credentials.isPending : allCredentials.isPending}
            projectMap={projectMap}
            projectsLoading={projects.isPending}
            registriesLoading={registryOptions.isPending}
            toolbar={(
              <>
                {visibilitySelect}
                <Select
                  value={toSelectValue(credentialRegistryFilterId)}
                  onValueChange={(next) => {
                    setCredentialRegistryFilterId(fromSelectValue(next))
                    setCredentialPage(1)
                  }}
                >
                  <SelectTrigger
                    aria-label={t('registriesPage.selectRegistryTitle')}
                    className="w-full @lg/data-list:w-40"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={EMPTY_SELECT_VALUE}>{t('registriesPage.allRegistries')}</SelectItem>
                    {registriesLookupLoading
                      ? <SelectItem disabled value="__loading__">{t('common.loading')}</SelectItem>
                      : registryLookupItems.map(registry => (
                          <SelectItem key={registry.id} value={registry.id}>{registry.name}</SelectItem>
                        ))}
                  </SelectContent>
                </Select>
              </>
            )}
            pagination={{
              data: credentialPageData,
              page: credentialPage,
              pageSize: credentialPageSize,
              onPageChange: setCredentialPage,
              onPageSizeChange: (nextPageSize) => {
                setCredentialPageSize(nextPageSize)
                setCredentialPage(1)
              },
            }}
            onCreate={openCreateCredentialDialog}
            onDelete={setCredentialToDelete}
            onEdit={beginEditCredential}
          />
        </TabsContent>

        <TabsContent value="images">
          <ImagesPanel
            error={images.isError
              ? {
                  onRetry: () => void images.refetch(),
                  retrying: images.isFetching,
                }
              : undefined}
            images={images.data?.items ?? []}
            loading={images.isPending}
            toolbar={visibilitySelect}
            registries={registryLookupItems}
            registriesLoading={registryOptions.isPending}
            pagination={{
              data: images.data,
              page: imagePage,
              pageSize: imagePageSize,
              onPageChange: setImagePage,
              onPageSizeChange: (nextPageSize) => {
                setImagePageSize(nextPageSize)
                setImagePage(1)
              },
            }}
            onCreate={openRecordImageDialog}
          />
        </TabsContent>

      </ContentTabs>

      <RegistryDialog
        canManageGlobal={canViewAll}
        open={registryDialogOpen}
        editingRegistry={editingRegistry}
        form={registryForm}
        pending={saveRegistry.isPending}
        projects={projects.data ?? []}
        projectsError={projects.isError}
        projectsLoading={projects.isPending}
        projectsRetrying={projects.isFetching}
        onRetryProjects={() => void projects.refetch()}
        onSubmit={values => saveRegistry.mutate(values)}
        onOpenChange={(open) => {
          setRegistryDialogOpen(open)
          if (!open)
            setEditingRegistry(null)
        }}
      />

      <CredentialDialog
        canManageGlobal={canViewAll}
        open={credentialDialogOpen}
        editingCredential={editingCredential}
        form={credentialForm}
        pending={saveCredential.isPending}
        registries={registryOptions.data ?? (registryOptions.isError ? [] : registryItems)}
        registriesError={registryOptions.isError}
        registriesLoading={registryOptions.isPending}
        registriesRetrying={registryOptions.isFetching}
        projects={projects.data ?? []}
        projectsError={projects.isError}
        projectsLoading={projects.isPending}
        projectsRetrying={projects.isFetching}
        defaultRegistryId={credentialRegistryFilterId}
        onOpenChange={(open) => {
          setCredentialDialogOpen(open)
          if (!open)
            setEditingCredential(null)
        }}
        onRetryProjects={() => void projects.refetch()}
        onRetryRegistries={() => void registryOptions.refetch()}
        onSubmit={values => saveCredential.mutate(values)}
      />

      <ImageDialog
        open={imageDialogOpen}
        form={imageForm}
        pending={createImage.isPending}
        registries={registryOptions.data ?? (registryOptions.isError ? [] : registryItems)}
        registriesError={registryOptions.isError}
        registriesLoading={registryOptions.isPending}
        registriesRetrying={registryOptions.isFetching}
        onRetryRegistries={() => void registryOptions.refetch()}
        repositoryResults={{
          items: imageRepositoryResults.data?.items ?? [],
          isFetching: imageRepositoryResults.isFetching,
          isSuccess: imageRepositoryResults.isSuccess,
          isError: imageRepositoryResults.isError,
          refetch: () => {
            imageRepositoryResults.refetch()
          },
        }}
        repositoryResultsOpen={imageRepositoryResultsOpen}
        repositorySearch={imageRepositorySearch}
        tagResults={{
          items: imageTags.data?.items ?? [],
          isFetching: imageTags.isFetching,
        }}
        onOpenChange={setImageDialogOpen}
        onRepositoryResultsOpenChange={setImageRepositoryResultsOpen}
        onRepositorySearchChange={setImageRepositorySearch}
        onSelectRepository={selectImageRepository}
        onSubmit={values => createImage.mutate(values)}
      />

      <ConfirmDialog
        confirmText={t('registriesPage.deleteRegistryConfirm')}
        description={t('registriesPage.deleteRegistryDescription', { name: registryToDelete?.name ?? '' })}
        open={Boolean(registryToDelete)}
        pending={deleteRegistry.isPending}
        title={t('registriesPage.deleteRegistryTitle')}
        onConfirm={() => registryToDelete && deleteRegistry.mutate(registryToDelete.id)}
        onOpenChange={open => !open && setRegistryToDelete(null)}
      />
      <ConfirmDialog
        confirmText={t('registriesPage.deleteCredentialConfirm')}
        description={t('registriesPage.deleteCredentialDescription', { name: credentialToDelete?.name ?? '' })}
        open={Boolean(credentialToDelete)}
        pending={deleteCredential.isPending}
        title={t('registriesPage.deleteCredentialTitle')}
        onConfirm={() => credentialToDelete && deleteCredential.mutate({ registryId: credentialToDelete.registryId, credentialId: credentialToDelete.id })}
        onOpenChange={open => !open && setCredentialToDelete(null)}
      />
    </PageShell>
  )
}
