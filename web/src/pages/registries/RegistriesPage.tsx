import type { CredentialForm, CredentialWithRegistry, ImageForm, RegistryForm } from './registry-form-model'
import type { ArtifactRegistry, RegistryCredential, RegistryRepositoryItem } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Container, KeyRound, Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { Button } from '@/components/ui/button'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { CredentialDialog, ImageDialog, RegistryDialog } from './registry-dialogs'
import { credentialDefaults, credentialSchema, imageSchema, registryDefaults, registrySchema, splitText } from './registry-form-model'
import { CredentialsPanel, ImagesPanel, RegistriesPanel } from './registry-list-panels'

export function RegistriesPage() {
  const { t } = useTranslation()
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
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const registries = useQuery({
    queryKey: ['registries', registryPage, registryPageSize],
    queryFn: () => api.listRegistriesPage({ page: registryPage, pageSize: registryPageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const registryItems = registries.data?.items ?? []
  const registryOptions = useQuery({ queryKey: ['registries', 'options'], queryFn: () => api.listRegistries() })
  const registryOptionItems = useMemo(() => registryOptions.data ?? [], [registryOptions.data])
  const images = useQuery({
    queryKey: ['container-images', imagePage, imagePageSize],
    queryFn: () => api.listContainerImages({ page: imagePage, pageSize: imagePageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const projectMap = useMemo(() => Object.fromEntries((projects.data ?? []).map(project => [project.id, project])), [projects.data])
  const allCredentials = useQuery({
    queryKey: ['registry-credentials', 'all', registryOptionItems.map(registry => registry.id).join(',')],
    queryFn: async () => {
      const results = await Promise.all(registryOptionItems.map(async (registry) => {
        try {
          const items = await api.listRegistryCredentials(registry.id)
          return items.map(credential => ({ ...credential, registryName: registry.name }))
        }
        catch {
          return [] as CredentialWithRegistry[]
        }
      }))
      return results.flat().sort((left, right) => new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime())
    },
    enabled: registryOptions.isSuccess,
  })
  const credentials = useQuery({
    queryKey: ['registry-credentials', credentialRegistryFilterId, credentialPage, credentialPageSize],
    queryFn: () => api.listRegistryCredentialsPage(credentialRegistryFilterId, { page: credentialPage, pageSize: credentialPageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
    enabled: Boolean(credentialRegistryFilterId),
  })

  const registryForm = useForm<RegistryForm>({
    resolver: zodResolver(registrySchema),
    mode: 'onChange',
    defaultValues: registryDefaults,
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
    queryKey: ['registry-repositories', imageRegistryId, imageRepositorySearch],
    queryFn: () => api.searchRegistryRepositories(imageRegistryId, { search: imageRepositorySearch, page: 1, pageSize: 10 }),
    enabled: Boolean(imageRegistryId && imageRepositorySearch.trim().length >= 2),
  })
  const imageTags = useQuery({
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
      registryForm.reset(registryDefaults)
      queryClient.invalidateQueries({ queryKey: ['registries'] })
    },
    onError: error => toast.error(error.message),
  })

  const saveCredential = useMutation({
    mutationFn: (values: CredentialForm) => {
      const registry = (registryOptions.data ?? []).find(item => item.id === values.registryId)
      const payload = {
        ...values,
        accessScope: registry?.scope === 'global' ? 'personal' : values.accessScope,
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
      accessScope: credential.accessScope,
      registryId: credential.registryId,
      name: credential.name,
      username: credential.username,
      password: '',
      token: '',
      scope: credential.scope,
      repositoryTemplate: credential.repositoryTemplate,
      tagTemplate: credential.tagTemplate,
    })
    setCredentialDialogOpen(true)
  }

  const selectedRegistry = registryOptionItems.find(registry => registry.id === credentialRegistryFilterId)
  const visibleCredentials: CredentialWithRegistry[] = credentialRegistryFilterId
    ? (credentials.data?.items ?? []).map(credential => ({ ...credential, registryName: selectedRegistry?.name ?? '' }))
    : (allCredentials.data ?? [])

  const selectImageRepository = (repository: RegistryRepositoryItem) => {
    imageForm.setValue('repository', repository.name, { shouldDirty: true, shouldValidate: true })
    imageForm.setValue('tag', '', { shouldDirty: true, shouldValidate: true })
    setImageRepositorySearch(repository.name)
    setImageRepositoryResultsOpen(false)
  }

  return (
    <div className="grid gap-6">
      <ContentTabs
        tabs={[
          { value: 'registries', label: t('registriesPage.registriesTab') },
          { value: 'credentials', label: t('registriesPage.credentialsTab') },
          { value: 'images', label: t('registriesPage.imagesTab') },
        ]}
        tools={(
          <>
            {activeTab === 'registries' && (
              <Button
                onClick={() => {
                  setEditingRegistry(null)
                  registryForm.reset(registryDefaults)
                  setRegistryDialogOpen(true)
                }}
              >
                <Plus size={16} />
                {t('registriesPage.createRegistry')}
              </Button>
            )}
            {activeTab === 'credentials' && (
              <div className="flex flex-nowrap items-center justify-end gap-2">
                <Select
                  className="h-9"
                  containerClassName="w-40 shrink-0"
                  value={credentialRegistryFilterId}
                  aria-label={t('registriesPage.selectRegistryTitle')}
                  onChange={(event) => {
                    setCredentialRegistryFilterId(event.target.value)
                    setCredentialPage(1)
                  }}
                >
                  <option value="">{t('registriesPage.allRegistries')}</option>
                  {(registryOptions.data ?? registryItems).map(registry => (
                    <option key={registry.id} value={registry.id}>{registry.name}</option>
                  ))}
                </Select>
                <Button
                  className="shrink-0 whitespace-nowrap"
                  onClick={() => {
                    setEditingCredential(null)
                    credentialForm.reset({ ...credentialDefaults, registryId: credentialRegistryFilterId })
                    credentialForm.setValue('registryId', credentialRegistryFilterId, { shouldValidate: true })
                    credentialForm.setValue('accessScope', 'personal', { shouldValidate: true })
                    setCredentialDialogOpen(true)
                  }}
                >
                  <KeyRound size={16} />
                  {t('registriesPage.createCredentialTitle')}
                </Button>
              </div>
            )}
            {activeTab === 'images' && (
              <Button
                onClick={() => {
                  imageForm.setValue('registryId', credentialRegistryFilterId, { shouldValidate: true })
                  setImageDialogOpen(true)
                }}
              >
                <Container size={16} />
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
            items={registryItems}
            isError={registries.isError}
            projectMap={projectMap}
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
            items={visibleCredentials}
            registryFilterId={credentialRegistryFilterId}
            pagination={credentialRegistryFilterId
              ? {
                  data: credentials.data
                    ? {
                        ...credentials.data,
                        items: credentials.data.items.map(credential => ({ ...credential, registryName: selectedRegistry?.name ?? '' })),
                      }
                    : undefined,
                  page: credentialPage,
                  pageSize: credentialPageSize,
                  onPageChange: setCredentialPage,
                  onPageSizeChange: (nextPageSize) => {
                    setCredentialPageSize(nextPageSize)
                    setCredentialPage(1)
                  },
                }
              : {
                  data: undefined,
                  page: credentialPage,
                  pageSize: credentialPageSize,
                  onPageChange: setCredentialPage,
                  onPageSizeChange: setCredentialPageSize,
                }}
            onDelete={setCredentialToDelete}
            onEdit={beginEditCredential}
          />
        </TabsContent>

        <TabsContent value="images">
          <ImagesPanel
            images={images.data?.items ?? []}
            registries={registryOptions.data ?? registryItems}
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
          />
        </TabsContent>

      </ContentTabs>

      <RegistryDialog
        open={registryDialogOpen}
        editingRegistry={editingRegistry}
        form={registryForm}
        pending={saveRegistry.isPending}
        projects={projects.data ?? []}
        onSubmit={values => saveRegistry.mutate(values)}
        onOpenChange={(open) => {
          setRegistryDialogOpen(open)
          if (!open)
            setEditingRegistry(null)
        }}
      />

      <CredentialDialog
        open={credentialDialogOpen}
        editingCredential={editingCredential}
        form={credentialForm}
        pending={saveCredential.isPending}
        registries={registryOptions.data ?? registryItems}
        defaultRegistryId={credentialRegistryFilterId}
        onOpenChange={(open) => {
          setCredentialDialogOpen(open)
          if (!open)
            setEditingCredential(null)
        }}
        onSubmit={values => saveCredential.mutate(values)}
      />

      <ImageDialog
        open={imageDialogOpen}
        form={imageForm}
        pending={createImage.isPending}
        registries={registryOptions.data ?? registryItems}
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
    </div>
  )
}
