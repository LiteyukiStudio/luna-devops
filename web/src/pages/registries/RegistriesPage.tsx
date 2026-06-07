import type { ArtifactRegistry, BuilderAgent, BuildProvider, ContainerImage, RegistryCredential, RegistryRepositoryItem } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, Container, Cpu, KeyRound, Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api/client'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { EditActionButton } from '@/components/common/edit-action-button'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { MotionItem, MotionList } from '@/components/common/motion'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import i18next from '@/i18n'

const registrySchema = z.object({
  name: z.string().min(1, i18next.t('registriesPage.registryNameRequired')),
  provider: z.enum(['harbor', 'dockerhub', 'gitea-registry']),
  endpoint: z.string().url(i18next.t('registriesPage.validUrlRequired')),
  scope: z.enum(['global', 'project', 'user']),
  ownerRef: z.string(),
  isDefault: z.boolean(),
  capabilitiesText: z.string(),
})

const credentialSchema = z.object({
  registryId: z.string().min(1, i18next.t('registriesPage.registryRequired')),
  name: z.string().min(1, i18next.t('registriesPage.credentialNameRequired')),
  username: z.string(),
  password: z.string(),
  token: z.string(),
  scope: z.enum(['push-pull', 'push', 'pull']),
  accessScope: z.enum(['personal', 'registry']),
}).refine(values => values.password.trim() !== '' || values.token.trim() !== '', {
  message: i18next.t('registriesPage.passwordOrTokenRequired'),
  path: ['password'],
})

const imageSchema = z.object({
  projectId: z.string(),
  applicationId: z.string(),
  registryId: z.string().min(1, i18next.t('registriesPage.registryRequired')),
  repository: z.string().min(1, i18next.t('registriesPage.repositoryRequired')),
  tag: z.string(),
  digest: z.string(),
  sourceCommit: z.string(),
  buildRunId: z.string(),
  sourceType: z.enum(['manual-image', 'build']),
  scanStatus: z.enum(['unknown', 'pending', 'scanning', 'passed', 'failed']),
})

const buildProviderSchema = z.object({
  name: z.string().min(1, i18next.t('buildsPage.providerNameRequired')),
  type: z.enum(['platform']),
  scope: z.enum(['global', 'project', 'user']),
  ownerRef: z.string(),
  config: z.string(),
  enabled: z.boolean(),
})

type RegistryForm = z.infer<typeof registrySchema>
type CredentialForm = z.infer<typeof credentialSchema>
type ImageForm = z.infer<typeof imageSchema>
type BuildProviderForm = z.infer<typeof buildProviderSchema>
type CredentialWithRegistry = RegistryCredential & { registryName: string }

const registryDefaults: RegistryForm = {
  name: '',
  provider: 'harbor',
  endpoint: '',
  scope: 'global',
  ownerRef: '',
  isDefault: false,
  capabilitiesText: 'push,pull,tags,digest',
}
const buildProviderDefaults: BuildProviderForm = { config: '{}', enabled: true, name: '', ownerRef: '', scope: 'global', type: 'platform' }

export function RegistriesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editingRegistry, setEditingRegistry] = useState<ArtifactRegistry | null>(null)
  const [registryToDelete, setRegistryToDelete] = useState<ArtifactRegistry | null>(null)
  const [credentialToDelete, setCredentialToDelete] = useState<RegistryCredential | null>(null)
  const [selectedRegistryId, setSelectedRegistryId] = useState('')
  const [activeTab, setActiveTab] = useState('registries')
  const [registryDialogOpen, setRegistryDialogOpen] = useState(false)
  const [credentialDialogOpen, setCredentialDialogOpen] = useState(false)
  const [imageDialogOpen, setImageDialogOpen] = useState(false)
  const [buildProviderDialogOpen, setBuildProviderDialogOpen] = useState(false)
  const [editingBuildProvider, setEditingBuildProvider] = useState<BuildProvider | null>(null)
  const [buildProviderToDelete, setBuildProviderToDelete] = useState<BuildProvider | null>(null)
  const [builderAgentToDelete, setBuilderAgentToDelete] = useState<BuilderAgent | null>(null)
  const [imageRepositorySearch, setImageRepositorySearch] = useState('')
  const [imageRepositoryResultsOpen, setImageRepositoryResultsOpen] = useState(false)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const registries = useQuery({ queryKey: ['registries'], queryFn: () => api.listRegistries() })
  const images = useQuery({ queryKey: ['container-images'], queryFn: () => api.listContainerImages() })
  const buildProviders = useQuery({ queryKey: ['build-providers'], queryFn: () => api.listBuildProviders() })
  const builderAgents = useQuery({ queryKey: ['builder-agents'], queryFn: () => api.listBuilderAgents() })
  const projectMap = useMemo(() => Object.fromEntries((projects.data ?? []).map(project => [project.id, project])), [projects.data])
  const allCredentials = useQuery({
    queryKey: ['registry-credentials', 'all', (registries.data ?? []).map(registry => registry.id).join(',')],
    queryFn: async () => {
      const results = await Promise.all((registries.data ?? []).map(async (registry) => {
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
    enabled: registries.isSuccess,
  })
  const credentials = useQuery({
    queryKey: ['registry-credentials', selectedRegistryId],
    queryFn: () => api.listRegistryCredentials(selectedRegistryId),
    enabled: Boolean(selectedRegistryId),
  })

  const registryForm = useForm<RegistryForm>({
    resolver: zodResolver(registrySchema),
    mode: 'onChange',
    defaultValues: registryDefaults,
  })
  const credentialForm = useForm<CredentialForm>({
    resolver: zodResolver(credentialSchema),
    mode: 'onChange',
    defaultValues: { accessScope: 'personal', registryId: '', name: 'default', username: '', password: '', token: '', scope: 'push-pull' },
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
  const buildProviderForm = useForm<BuildProviderForm>({
    resolver: zodResolver(buildProviderSchema),
    mode: 'onChange',
    defaultValues: buildProviderDefaults,
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
        ownerRef: values.scope === 'project' ? values.ownerRef : '',
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

  const createCredential = useMutation({
    mutationFn: (values: CredentialForm) => {
      const registry = (registries.data ?? []).find(item => item.id === values.registryId)
      return api.createRegistryCredential(values.registryId, {
        ...values,
        accessScope: registry?.scope === 'global' ? 'personal' : values.accessScope,
      })
    },
    onSuccess: (_, values) => {
      toast.success(t('registriesPage.credentialSaved'))
      setCredentialDialogOpen(false)
      setSelectedRegistryId(values.registryId)
      credentialForm.reset({ accessScope: 'personal', registryId: values.registryId, name: 'default', username: '', password: '', token: '', scope: 'push-pull' })
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
  const saveBuildProvider = useMutation({
    mutationFn: (values: BuildProviderForm) => {
      const payload = {
        ...values,
        ownerRef: values.scope === 'project' ? values.ownerRef : '',
      }
      if (editingBuildProvider)
        return api.updateBuildProvider(editingBuildProvider.id, payload)
      return api.createBuildProvider(payload)
    },
    onSuccess: () => {
      toast.success(editingBuildProvider ? t('buildsPage.providerUpdated') : t('buildsPage.providerCreated'))
      setBuildProviderDialogOpen(false)
      setEditingBuildProvider(null)
      buildProviderForm.reset(buildProviderDefaults)
      queryClient.invalidateQueries({ queryKey: ['build-providers'] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteBuildProvider = useMutation({
    mutationFn: api.deleteBuildProvider,
    onSuccess: () => {
      toast.success(t('buildsPage.providerDeleted'))
      setBuildProviderToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['build-providers'] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteBuilderAgent = useMutation({
    mutationFn: api.deleteBuilderAgent,
    onSuccess: () => {
      toast.success(t('registriesPage.builderDeleted'))
      setBuilderAgentToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['builder-agents'] })
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
      isDefault: registry.isDefault,
      capabilitiesText: registry.capabilities.join(', '),
    })
    setRegistryDialogOpen(true)
  }

  const beginEditBuildProvider = (provider: BuildProvider) => {
    setEditingBuildProvider(provider)
    buildProviderForm.reset({
      config: provider.config,
      enabled: provider.enabled,
      name: provider.name,
      ownerRef: provider.ownerRef,
      scope: provider.scope,
      type: provider.type,
    })
    setBuildProviderDialogOpen(true)
  }

  const selectedRegistry = (registries.data ?? []).find(registry => registry.id === selectedRegistryId)
  const credentialRegistry = (registries.data ?? []).find(registry => registry.id === credentialForm.watch('registryId'))
  const credentialRegistryIsGlobal = credentialRegistry?.scope === 'global'
  const visibleCredentials: CredentialWithRegistry[] = selectedRegistryId
    ? (credentials.data ?? []).map(credential => ({ ...credential, registryName: selectedRegistry?.name ?? '' }))
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
          { value: 'build-providers', label: t('registriesPage.buildPlatformsTab') },
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
              <Button
                onClick={() => {
                  credentialForm.setValue('registryId', selectedRegistryId, { shouldValidate: true })
                  credentialForm.setValue('accessScope', 'personal', { shouldValidate: true })
                  setCredentialDialogOpen(true)
                }}
              >
                <KeyRound size={16} />
                {t('registriesPage.createCredentialTitle')}
              </Button>
            )}
            {activeTab === 'images' && (
              <Button
                onClick={() => {
                  imageForm.setValue('registryId', selectedRegistryId, { shouldValidate: true })
                  setImageDialogOpen(true)
                }}
              >
                <Container size={16} />
                {t('registriesPage.recordImage')}
              </Button>
            )}
            {activeTab === 'build-providers' && (
              <Button
                onClick={() => {
                  setEditingBuildProvider(null)
                  buildProviderForm.reset(buildProviderDefaults)
                  setBuildProviderDialogOpen(true)
                }}
              >
                <Cpu size={16} />
                {t('buildsPage.createProvider')}
              </Button>
            )}
          </>
        )}
        value={activeTab}
        onValueChange={setActiveTab}
      >

        <TabsContent value="registries">
          <Card>
            {registries.isError && <ErrorState title={t('registriesPage.loadFailedTitle')} description={t('registriesPage.loadFailedDescription')} />}
            <MotionList className="grid gap-3">
              {(registries.data ?? []).map(registry => (
                <MotionItem key={registry.id}>
                  <RegistryRow
                    registry={registry}
                    testing={testRegistry.isPending}
                    onDelete={() => setRegistryToDelete(registry)}
                    onEdit={() => beginEdit(registry)}
                    onSelect={() => {
                      setSelectedRegistryId(registry.id)
                      setActiveTab('credentials')
                    }}
                    onTest={() => testRegistry.mutate(registry.id)}
                  />
                </MotionItem>
              ))}
              {registries.data?.length === 0 && <EmptyState title={t('registriesPage.emptyTitle')} description={t('registriesPage.emptyDescription')} />}
            </MotionList>
          </Card>
        </TabsContent>

        <TabsContent value="credentials">
          <Card className="grid gap-4">
            <Field hint={t('registriesPage.selectRegistryDescription')} label={t('registriesPage.selectRegistryTitle')}>
              <Select value={selectedRegistryId} onChange={event => setSelectedRegistryId(event.target.value)}>
                <option value="">{t('registriesPage.allRegistries')}</option>
                {(registries.data ?? []).map(registry => (
                  <option key={registry.id} value={registry.id}>{registry.name}</option>
                ))}
              </Select>
            </Field>
            <MotionList className="grid gap-3">
              {visibleCredentials.map(credential => (
                <MotionItem key={credential.id}>
                  <CredentialRow credential={credential} onDelete={() => setCredentialToDelete(credential)} />
                </MotionItem>
              ))}
              {visibleCredentials.length === 0 && <EmptyState title={t('registriesPage.noCredentialsTitle')} description={t('registriesPage.noCredentialsDescription')} />}
            </MotionList>
          </Card>
        </TabsContent>

        <TabsContent value="images">
          <Card>
            <MotionList className="grid gap-3">
              {(images.data ?? []).map(image => (
                <MotionItem key={image.id}>
                  <ImageRow image={image} registry={registries.data?.find(registry => registry.id === image.registryId)} />
                </MotionItem>
              ))}
              {images.data?.length === 0 && <EmptyState title={t('registriesPage.noImagesTitle')} description={t('registriesPage.noImagesDescription')} />}
            </MotionList>
          </Card>
        </TabsContent>

        <TabsContent value="build-providers">
          <Card className="grid gap-4">
            <div>
              <h2 className="text-base font-semibold">{t('registriesPage.builderAgentsTitle')}</h2>
              <p className="text-sm text-muted-foreground">{t('registriesPage.builderAgentsDescription')}</p>
            </div>
            <MotionList className="grid gap-3">
              {(builderAgents.data?.items ?? []).map(builder => (
                <MotionItem key={builder.id}>
                  <BuilderAgentRow builder={builder} onDelete={() => setBuilderAgentToDelete(builder)} />
                </MotionItem>
              ))}
              {builderAgents.data?.items.length === 0 && <EmptyState title={t('registriesPage.emptyBuilderAgentsTitle')} description={t('registriesPage.emptyBuilderAgentsDescription')} />}
            </MotionList>
            <div className="border-t border-border pt-4">
              <h2 className="text-base font-semibold">{t('registriesPage.buildProviderConfigsTitle')}</h2>
              <p className="text-sm text-muted-foreground">{t('registriesPage.buildProviderConfigsDescription')}</p>
            </div>
            <MotionList className="grid gap-3">
              {(buildProviders.data ?? []).map(provider => (
                <MotionItem key={provider.id}>
                  <BuildProviderRow
                    provider={provider}
                    projectName={provider.scope === 'project' ? projectMap[provider.ownerRef]?.name : undefined}
                    onDelete={() => setBuildProviderToDelete(provider)}
                    onEdit={() => beginEditBuildProvider(provider)}
                  />
                </MotionItem>
              ))}
              {buildProviders.data?.length === 0 && <EmptyState title={t('buildsPage.emptyProviders')} description={t('registriesPage.emptyBuildPlatformsDescription')} />}
            </MotionList>
          </Card>
        </TabsContent>
      </ContentTabs>

      <Dialog
        open={registryDialogOpen}
        onOpenChange={(open) => {
          setRegistryDialogOpen(open)
          if (!open) {
            setEditingRegistry(null)
            registryForm.reset(registryDefaults)
          }
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingRegistry ? t('registriesPage.editRegistryTitle') : t('registriesPage.createRegistryTitle')}</DialogTitle>
            <DialogDescription>{t('registriesPage.description')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={registryForm.handleSubmit(values => saveRegistry.mutate(values))}>
            <div className="grid grid-cols-2 gap-3">
              <Field error={registryForm.formState.errors.name?.message} hint={t('registriesPage.registryNameHint')} label={t('registriesPage.name')} required>
                <Input {...registryForm.register('name')} aria-invalid={Boolean(registryForm.formState.errors.name)} placeholder={t('registriesPage.registryNamePlaceholder')} />
              </Field>
              <Field error={registryForm.formState.errors.provider?.message} hint={t('registriesPage.providerHint')} label={t('registriesPage.provider')} required>
                <Select {...registryForm.register('provider')} aria-invalid={Boolean(registryForm.formState.errors.provider)}>
                  <option value="harbor">{t('registriesPage.providerHarbor')}</option>
                  <option value="dockerhub">{t('registriesPage.providerDockerHub')}</option>
                  <option value="gitea-registry">{t('registriesPage.providerGiteaRegistry')}</option>
                </Select>
              </Field>
            </div>
            <Field error={registryForm.formState.errors.endpoint?.message} hint={t('registriesPage.endpointHint')} label={t('registriesPage.endpoint')} required>
              <Input {...registryForm.register('endpoint')} aria-invalid={Boolean(registryForm.formState.errors.endpoint)} placeholder={t('registriesPage.endpointPlaceholder')} />
            </Field>
            <Field error={registryForm.formState.errors.scope?.message} hint={t('registriesPage.registryScopeHint')} label={t('registriesPage.scope')} required>
              <Select {...registryForm.register('scope')} aria-invalid={Boolean(registryForm.formState.errors.scope)}>
                <option value="global">{t('registriesPage.scopeGlobal')}</option>
                <option value="project">{t('registriesPage.scopeProject')}</option>
                <option value="user">{t('registriesPage.scopeUser')}</option>
              </Select>
            </Field>
            {registryForm.watch('scope') === 'project' && (
              <Field error={registryForm.formState.errors.ownerRef?.message} hint={t('registriesPage.ownerProjectHint')} label={t('registriesPage.ownerProject')}>
                <Select {...registryForm.register('ownerRef')} aria-invalid={Boolean(registryForm.formState.errors.ownerRef)}>
                  <option value="">{t('registriesPage.selectProject')}</option>
                  {(projects.data ?? []).map(project => (
                    <option key={project.id} value={project.id}>{project.name}</option>
                  ))}
                </Select>
              </Field>
            )}
            <Field error={registryForm.formState.errors.capabilitiesText?.message} hint={t('registriesPage.capabilitiesHint')} label={t('registriesPage.capabilities')}>
              <Input {...registryForm.register('capabilitiesText')} aria-invalid={Boolean(registryForm.formState.errors.capabilitiesText)} />
            </Field>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" {...registryForm.register('isDefault')} />
              {t('registriesPage.setAsDefault')}
            </label>
            <DialogFooter>
              <Button disabled={saveRegistry.isPending || !registryForm.formState.isValid} type="submit">
                <Plus size={16} />
                {editingRegistry ? t('registriesPage.saveRegistry') : t('registriesPage.createRegistry')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={credentialDialogOpen}
        onOpenChange={(open) => {
          setCredentialDialogOpen(open)
          if (!open)
            credentialForm.reset({ accessScope: 'personal', registryId: selectedRegistryId, name: 'default', username: '', password: '', token: '', scope: 'push-pull' })
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('registriesPage.createCredentialTitle')}</DialogTitle>
            <DialogDescription>{t('registriesPage.credentialRegistryHint')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={credentialForm.handleSubmit(values => createCredential.mutate(values))}>
            <Field error={credentialForm.formState.errors.registryId?.message} hint={t('registriesPage.credentialRegistryHint')} label={t('registries')} required>
              <Select
                {...credentialForm.register('registryId')}
                aria-invalid={Boolean(credentialForm.formState.errors.registryId)}
                onChange={(event) => {
                  credentialForm.setValue('registryId', event.target.value, { shouldValidate: true })
                  const registry = (registries.data ?? []).find(item => item.id === event.target.value)
                  if (registry?.scope === 'global')
                    credentialForm.setValue('accessScope', 'personal', { shouldValidate: true })
                  setSelectedRegistryId(event.target.value)
                }}
              >
                <option value="">{t('registriesPage.selectRegistry')}</option>
                {(registries.data ?? []).map(registry => (
                  <option key={registry.id} value={registry.id}>{registry.name}</option>
                ))}
              </Select>
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field error={credentialForm.formState.errors.name?.message} hint={t('registriesPage.credentialNameHint')} label={t('registriesPage.name')} required>
                <Input {...credentialForm.register('name')} aria-invalid={Boolean(credentialForm.formState.errors.name)} />
              </Field>
              <Field error={credentialForm.formState.errors.scope?.message} hint={t('registriesPage.credentialScopeHint')} label={t('registriesPage.usage')} required>
                <Select {...credentialForm.register('scope')} aria-invalid={Boolean(credentialForm.formState.errors.scope)}>
                  <option value="push-pull">{t('registriesPage.credentialScopePushPull')}</option>
                  <option value="push">{t('registriesPage.credentialScopePush')}</option>
                  <option value="pull">{t('registriesPage.credentialScopePull')}</option>
                </Select>
              </Field>
            </div>
            <Field error={credentialForm.formState.errors.accessScope?.message} hint={credentialRegistryIsGlobal ? t('registriesPage.credentialAccessScopeGlobalHint') : t('registriesPage.credentialAccessScopeHint')} label={t('registriesPage.credentialAccessScope')} required>
              <Select {...credentialForm.register('accessScope')} aria-invalid={Boolean(credentialForm.formState.errors.accessScope)} disabled={credentialRegistryIsGlobal}>
                <option value="personal">{t('registriesPage.credentialAccessScopePersonal')}</option>
                {!credentialRegistryIsGlobal && <option value="registry">{t('registriesPage.credentialAccessScopeRegistry')}</option>}
              </Select>
            </Field>
            <Field error={credentialForm.formState.errors.username?.message} hint={t('registriesPage.usernameHint')} label={t('registriesPage.username')}>
              <Input {...credentialForm.register('username')} aria-invalid={Boolean(credentialForm.formState.errors.username)} />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field error={credentialForm.formState.errors.password?.message} hint={t('registriesPage.passwordHint')} label={t('registriesPage.password')}>
                <Input {...credentialForm.register('password')} aria-invalid={Boolean(credentialForm.formState.errors.password)} type="password" />
              </Field>
              <Field error={credentialForm.formState.errors.token?.message} hint={t('registriesPage.tokenHint')} label={t('registriesPage.token')}>
                <Input {...credentialForm.register('token')} aria-invalid={Boolean(credentialForm.formState.errors.token)} type="password" />
              </Field>
            </div>
            <DialogFooter>
              <Button disabled={createCredential.isPending || !credentialForm.formState.isValid} type="submit">
                <KeyRound size={16} />
                {t('registriesPage.saveCredential')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={imageDialogOpen}
        onOpenChange={(open) => {
          setImageDialogOpen(open)
          if (!open)
            imageForm.reset()
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('registriesPage.recordImageTitle')}</DialogTitle>
            <DialogDescription>{t('registriesPage.imageRegistryHint')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={imageForm.handleSubmit(values => createImage.mutate(values))}>
            <Field error={imageForm.formState.errors.registryId?.message} hint={t('registriesPage.imageRegistryHint')} label={t('registries')} required>
              <Select
                {...imageForm.register('registryId', {
                  onChange: () => {
                    imageForm.setValue('repository', '', { shouldDirty: true, shouldValidate: true })
                    imageForm.setValue('tag', '', { shouldDirty: true, shouldValidate: true })
                    setImageRepositorySearch('')
                    setImageRepositoryResultsOpen(false)
                  },
                })}
                aria-invalid={Boolean(imageForm.formState.errors.registryId)}
              >
                <option value="">{t('registriesPage.selectRegistry')}</option>
                {(registries.data ?? []).map(registry => (
                  <option key={registry.id} value={registry.id}>{registry.name}</option>
                ))}
              </Select>
            </Field>
            <Field error={imageForm.formState.errors.repository?.message} hint={t('registriesPage.repositoryHint')} label={t('registriesPage.repository')} required>
              <div className="flex gap-2">
                <Input
                  {...imageForm.register('repository')}
                  aria-invalid={Boolean(imageForm.formState.errors.repository)}
                  placeholder={t('registriesPage.repositoryPlaceholder')}
                  value={imageRepositorySearch}
                  onChange={(event) => {
                    setImageRepositorySearch(event.target.value)
                    imageForm.setValue('repository', event.target.value, { shouldDirty: true, shouldValidate: true })
                    setImageRepositoryResultsOpen(event.target.value.trim().length >= 2)
                  }}
                  onFocus={() => setImageRepositoryResultsOpen(Boolean(imageRegistryId && imageRepositorySearch.trim().length >= 2))}
                />
                <Button
                  disabled={!imageRegistryId || imageRepositorySearch.trim().length < 2 || imageRepositoryResults.isFetching}
                  type="button"
                  variant="secondary"
                  onClick={() => {
                    setImageRepositoryResultsOpen(true)
                    imageRepositoryResults.refetch()
                  }}
                >
                  <Search size={16} />
                  {t('registriesPage.searchImages')}
                </Button>
              </div>
            </Field>
            {imageRegistryId && imageRepositoryResultsOpen && (
              <div className="grid max-h-52 gap-2 overflow-y-auto rounded-md border border-border p-2">
                {imageRepositoryResults.isFetching && (
                  <p className="px-3 py-2 text-sm text-muted-foreground">{t('registriesPage.searchingImages')}</p>
                )}
                {(imageRepositoryResults.data?.items ?? []).map(repository => (
                  <button
                    key={repository.name}
                    className="rounded-md px-3 py-2 text-left hover:bg-muted"
                    type="button"
                    onClick={() => selectImageRepository(repository)}
                  >
                    <span className="block text-sm font-medium">{repository.name}</span>
                    {repository.description && <span className="block truncate text-xs text-muted-foreground">{repository.description}</span>}
                  </button>
                ))}
                {imageRepositoryResults.isSuccess && imageRepositoryResults.data.items.length === 0 && (
                  <EmptyState title={t('registriesPage.noRepositoryResultsTitle')} description={t('registriesPage.noRepositoryResultsDescription')} />
                )}
                {imageRepositoryResults.isError && (
                  <EmptyState title={t('registriesPage.repositorySearchFailedTitle')} description={t('registriesPage.repositorySearchFailedDescription')} />
                )}
              </div>
            )}
            <div className="grid grid-cols-2 gap-3">
              <Field error={imageForm.formState.errors.tag?.message} hint={t('registriesPage.tagHint')} label={t('registriesPage.tag')}>
                <Input
                  {...imageForm.register('tag')}
                  aria-invalid={Boolean(imageForm.formState.errors.tag)}
                  list="registry-image-tag-options"
                  placeholder={imageTags.isFetching ? t('registriesPage.loadingTags') : t('registriesPage.tagPlaceholder')}
                />
                <datalist id="registry-image-tag-options">
                  {(imageTags.data?.items ?? []).map(tag => (
                    <option key={tag.name} value={tag.name}>{tag.digest}</option>
                  ))}
                </datalist>
              </Field>
              <Field error={imageForm.formState.errors.digest?.message} hint={t('registriesPage.digestHint')} label={t('registriesPage.digest')}>
                <Input {...imageForm.register('digest')} aria-invalid={Boolean(imageForm.formState.errors.digest)} />
              </Field>
            </div>
            <DialogFooter>
              <Button disabled={createImage.isPending || !imageForm.formState.isValid} type="submit">
                <Container size={16} />
                {t('registriesPage.recordImage')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={buildProviderDialogOpen}
        onOpenChange={(open) => {
          setBuildProviderDialogOpen(open)
          if (!open) {
            setEditingBuildProvider(null)
            buildProviderForm.reset(buildProviderDefaults)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editingBuildProvider ? t('buildsPage.editProvider') : t('buildsPage.createProvider')}</DialogTitle>
            <DialogDescription>{t('registriesPage.buildPlatformDialogDescription')}</DialogDescription>
          </DialogHeader>
          <form className="grid gap-3" onSubmit={buildProviderForm.handleSubmit(values => saveBuildProvider.mutate(values))}>
            <div className="grid grid-cols-2 gap-3">
              <Field error={buildProviderForm.formState.errors.name?.message} hint={t('registriesPage.buildPlatformNameHint')} label={t('common.name')} required>
                <Input {...buildProviderForm.register('name')} aria-invalid={Boolean(buildProviderForm.formState.errors.name)} />
              </Field>
              <Field error={buildProviderForm.formState.errors.type?.message} hint={t('registriesPage.buildPlatformTypeHint')} label={t('common.type')} required>
                <Select {...buildProviderForm.register('type')} aria-invalid={Boolean(buildProviderForm.formState.errors.type)}>
                  <option value="platform">{t('buildsPage.typePlatform')}</option>
                </Select>
              </Field>
            </div>
            <Field error={buildProviderForm.formState.errors.scope?.message} hint={t('registriesPage.buildPlatformScopeHint')} label={t('registriesPage.scope')} required>
              <Select {...buildProviderForm.register('scope')} aria-invalid={Boolean(buildProviderForm.formState.errors.scope)}>
                <option value="global">{t('registriesPage.scopeGlobal')}</option>
                <option value="project">{t('registriesPage.scopeProject')}</option>
                <option value="user">{t('registriesPage.scopeUser')}</option>
              </Select>
            </Field>
            {buildProviderForm.watch('scope') === 'project' && (
              <Field error={buildProviderForm.formState.errors.ownerRef?.message} hint={t('registriesPage.ownerProjectHint')} label={t('registriesPage.ownerProject')}>
                <Select {...buildProviderForm.register('ownerRef')} aria-invalid={Boolean(buildProviderForm.formState.errors.ownerRef)}>
                  <option value="">{t('registriesPage.selectProject')}</option>
                  {(projects.data ?? []).map(project => (
                    <option key={project.id} value={project.id}>{project.name}</option>
                  ))}
                </Select>
              </Field>
            )}
            <Field error={buildProviderForm.formState.errors.config?.message} hint={t('registriesPage.buildPlatformConfigHint')} label={t('buildsPage.config')}>
              <Input {...buildProviderForm.register('config')} aria-invalid={Boolean(buildProviderForm.formState.errors.config)} />
            </Field>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" {...buildProviderForm.register('enabled')} />
              {t('common.enabled')}
            </label>
            <DialogFooter>
              <Button disabled={saveBuildProvider.isPending || !buildProviderForm.formState.isValid} type="submit">
                <Cpu size={16} />
                {editingBuildProvider ? t('common.save') : t('buildsPage.createProvider')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

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
      <ConfirmDialog
        confirmText={t('common.delete')}
        description={t('buildsPage.deleteProviderDescription')}
        open={Boolean(buildProviderToDelete)}
        pending={deleteBuildProvider.isPending}
        title={t('buildsPage.deleteProviderTitle')}
        onConfirm={() => buildProviderToDelete && deleteBuildProvider.mutate(buildProviderToDelete.id)}
        onOpenChange={open => !open && setBuildProviderToDelete(null)}
      />
      <ConfirmDialog
        confirmText={t('common.delete')}
        description={t('registriesPage.deleteBuilderDescription', { name: builderAgentToDelete?.name ?? '' })}
        open={Boolean(builderAgentToDelete)}
        pending={deleteBuilderAgent.isPending}
        title={t('registriesPage.deleteBuilderTitle')}
        onConfirm={() => builderAgentToDelete && deleteBuilderAgent.mutate(builderAgentToDelete.id)}
        onOpenChange={open => !open && setBuilderAgentToDelete(null)}
      />
    </div>
  )
}

function RegistryRow({
  registry,
  testing,
  onDelete,
  onEdit,
  onSelect,
  onTest,
}: {
  registry: ArtifactRegistry
  testing: boolean
  onDelete: () => void
  onEdit: () => void
  onSelect: () => void
  onTest: () => void
}) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-3 rounded-md border border-border bg-background p-3">
      <button className="grid gap-1 text-left" type="button" onClick={onSelect}>
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="truncate font-medium">{registry.name}</p>
            <p className="truncate text-sm text-muted-foreground">{registry.endpoint}</p>
          </div>
          <div className="flex shrink-0 gap-2">
            {registry.isDefault && <StatusBadge>{t('common.default')}</StatusBadge>}
            <StatusBadge>{registry.scope}</StatusBadge>
            <StatusBadge>{registry.provider}</StatusBadge>
          </div>
        </div>
        <p className="text-xs text-muted-foreground">
          {registry.capabilities.join(', ') || t('registriesPage.noCapabilities')}
        </p>
      </button>
      <div className="flex flex-wrap gap-2">
        <EditActionButton type="button" label={t('edit')} onClick={onEdit} />
        <Button disabled={testing} type="button" variant="secondary" onClick={onTest}>
          <RefreshCw size={16} />
          {t('registriesPage.test')}
        </Button>
        <Button aria-label={t('registriesPage.deleteRegistryAria')} type="button" variant="ghost" onClick={onDelete}>
          <Trash2 size={16} />
        </Button>
      </div>
    </div>
  )
}

function CredentialRow({ credential, onDelete }: { credential: CredentialWithRegistry, onDelete: () => void }) {
  const { t } = useTranslation()

  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-background p-3">
      <div className="min-w-0">
        <p className="truncate font-medium">{credential.name}</p>
        <p className="truncate text-sm text-muted-foreground">
          {credential.registryName}
          {' · '}
          {credential.username || t('registriesPage.tokenOnly')}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <StatusBadge>{credential.scope}</StatusBadge>
        <StatusBadge>{credential.accessScope === 'registry' ? t('registriesPage.credentialAccessScopeRegistry') : t('registriesPage.credentialAccessScopePersonal')}</StatusBadge>
        {credential.passwordSet && <StatusBadge>{t('registriesPage.passwordSet')}</StatusBadge>}
        {credential.tokenSet && <StatusBadge>{t('registriesPage.tokenSet')}</StatusBadge>}
        <Button aria-label={t('registriesPage.deleteCredentialAria')} variant="ghost" onClick={onDelete}>
          <Trash2 size={16} />
        </Button>
      </div>
    </div>
  )
}

function ImageRow({ image, registry }: { image: ContainerImage, registry?: ArtifactRegistry }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-background p-3">
      <div className="min-w-0">
        <p className="truncate font-medium">{image.imageRef}</p>
        <p className="truncate text-sm text-muted-foreground">{registry?.name ?? image.registryId}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <StatusBadge>{image.sourceType}</StatusBadge>
        <StatusValueBadge value={image.scanStatus} />
        {image.digest && <CheckCircle2 className="text-primary" size={16} />}
      </div>
    </div>
  )
}

function BuilderAgentRow({ builder, onDelete }: { builder: BuilderAgent, onDelete: () => void }) {
  const { t } = useTranslation()
  const labels = splitText(builder.labels)
  const scopes = splitText(builder.scopes)
  const concurrency = `${builder.currentConcurrency}/${builder.maxConcurrency}`
  const heartbeat = builder.lastHeartbeatAt
    ? new Date(builder.lastHeartbeatAt).toLocaleString()
    : t('common.none')

  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-background p-3">
      <div className="min-w-0">
        <p className="truncate font-medium">{builder.name}</p>
        <p className="truncate text-sm text-muted-foreground">
          {builder.id}
          {' · '}
          {builder.executor}
          {' · '}
          {t('registriesPage.builderConcurrency', { value: concurrency })}
          {' · '}
          {t('registriesPage.builderHeartbeat', { value: heartbeat })}
        </p>
      </div>
      <div className="flex shrink-0 flex-wrap justify-end gap-2">
        <StatusValueBadge value={builder.status} />
        {(scopes.length > 0 ? scopes : [t('registriesPage.builderScopeGlobalDefault')]).map(scope => (
          <StatusBadge key={scope}>{scope}</StatusBadge>
        ))}
        {labels.map(label => (
          <StatusBadge key={label}>{label}</StatusBadge>
        ))}
        <Button aria-label={t('registriesPage.deleteBuilderAria')} variant="ghost" onClick={onDelete}>
          <Trash2 size={16} />
        </Button>
      </div>
    </div>
  )
}

function BuildProviderRow({
  onDelete,
  onEdit,
  projectName,
  provider,
}: {
  onDelete: () => void
  onEdit: () => void
  projectName?: string
  provider: BuildProvider
}) {
  const { t } = useTranslation()
  const scopeText = provider.scope === 'project'
    ? projectName ?? provider.ownerRef
    : provider.scope === 'user' ? t('registriesPage.scopeUser') : t('registriesPage.scopeGlobal')

  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-background p-3">
      <div className="min-w-0">
        <p className="truncate font-medium">{provider.name}</p>
        <p className="truncate text-sm text-muted-foreground">{provider.config || t('common.noDescription')}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <StatusBadge>{t('buildsPage.typePlatform')}</StatusBadge>
        <StatusBadge>{scopeText}</StatusBadge>
        <StatusValueBadge value={provider.enabled ? 'enabled' : 'disabled'} />
        <EditActionButton type="button" label={t('common.edit')} onClick={onEdit} />
        <Button aria-label={t('registriesPage.deleteBuildPlatformAria')} variant="ghost" onClick={onDelete}>
          <Trash2 size={16} />
        </Button>
      </div>
    </div>
  )
}

function splitText(value: string) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}
