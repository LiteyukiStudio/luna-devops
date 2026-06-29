import type { UseFormReturn } from 'react-hook-form'
import type { CredentialForm, ImageForm, RegistryForm } from './registry-form-model'
import type { ArtifactRegistry, Project, RegistryRepositoryItem, RegistryTagItem } from '@/api'
import { Container, KeyRound, Plus, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CheckboxField } from '@/components/common/checkbox-field'
import { EmptyState } from '@/components/common/empty-state'
import { FormField as Field } from '@/components/common/form-field'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { credentialDefaults, registryDefaults } from './registry-form-model'

interface RegistryDialogProps {
  open: boolean
  editingRegistry: ArtifactRegistry | null
  form: UseFormReturn<RegistryForm>
  projects: Project[]
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (values: RegistryForm) => void
}

export function RegistryDialog({ open, editingRegistry, form, projects, pending, onOpenChange, onSubmit }: RegistryDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset(registryDefaults)
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{editingRegistry ? t('registriesPage.editRegistryTitle') : t('registriesPage.createRegistryTitle')}</DialogTitle>
          <DialogDescription>{t('registriesPage.description')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.name?.message} hint={t('registriesPage.registryNameHint')} label={t('registriesPage.name')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('registriesPage.registryNamePlaceholder')} />
            </Field>
            <Field error={form.formState.errors.provider?.message} hint={t('registriesPage.providerHint')} label={t('registriesPage.provider')} required>
              <Select {...form.register('provider')} aria-invalid={Boolean(form.formState.errors.provider)}>
                <option value="harbor">{t('registriesPage.providerHarbor')}</option>
                <option value="dockerhub">{t('registriesPage.providerDockerHub')}</option>
                <option value="gitea-registry">{t('registriesPage.providerGiteaRegistry')}</option>
              </Select>
            </Field>
          </div>
          <Field error={form.formState.errors.endpoint?.message} hint={t('registriesPage.endpointHint')} label={t('registriesPage.endpoint')} required>
            <Input {...form.register('endpoint')} aria-invalid={Boolean(form.formState.errors.endpoint)} placeholder={t('registriesPage.endpointPlaceholder')} />
          </Field>
          <Field error={form.formState.errors.scope?.message} hint={t('registriesPage.registryScopeHint')} label={t('registriesPage.scope')} required>
            <Select {...form.register('scope')} aria-invalid={Boolean(form.formState.errors.scope)}>
              <option value="global">{t('registriesPage.scopeGlobal')}</option>
              <option value="project">{t('registriesPage.scopeProject')}</option>
              <option value="user">{t('registriesPage.scopeUser')}</option>
            </Select>
          </Field>
          {form.watch('scope') === 'project' && (
            <Field error={form.formState.errors.projectIds?.message} hint={t('registriesPage.ownerProjectHint')} label={t('registriesPage.ownerProject')}>
              <ProjectSpaceMultiSelect
                projects={projects}
                value={form.watch('projectIds')}
                onChange={value => form.setValue('projectIds', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
          )}
          <Field error={form.formState.errors.capabilitiesText?.message} hint={t('registriesPage.capabilitiesHint')} label={t('registriesPage.capabilities')}>
            <Input {...form.register('capabilitiesText')} aria-invalid={Boolean(form.formState.errors.capabilitiesText)} />
          </Field>
          <CheckboxField {...form.register('isDefault')}>
            {t('registriesPage.setAsDefault')}
          </CheckboxField>
          <DialogFooter>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              <Plus size={16} />
              {editingRegistry ? t('registriesPage.saveRegistry') : t('registriesPage.createRegistry')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface CredentialDialogProps {
  open: boolean
  form: UseFormReturn<CredentialForm>
  registries: ArtifactRegistry[]
  selectedRegistryId: string
  pending: boolean
  onOpenChange: (open: boolean) => void
  onRegistryChange: (registryId: string) => void
  onSubmit: (values: CredentialForm) => void
}

export function CredentialDialog({ open, form, registries, selectedRegistryId, pending, onOpenChange, onRegistryChange, onSubmit }: CredentialDialogProps) {
  const { t } = useTranslation()
  const credentialRegistry = registries.find(registry => registry.id === form.watch('registryId'))
  const credentialRegistryIsGlobal = credentialRegistry?.scope === 'global'

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset({ ...credentialDefaults, registryId: selectedRegistryId })
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('registriesPage.createCredentialTitle')}</DialogTitle>
          <DialogDescription>{t('registriesPage.credentialRegistryHint')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <Field error={form.formState.errors.registryId?.message} hint={t('registriesPage.credentialRegistryHint')} label={t('registries')} required>
            <Select
              {...form.register('registryId')}
              aria-invalid={Boolean(form.formState.errors.registryId)}
              onChange={(event) => {
                form.setValue('registryId', event.target.value, { shouldValidate: true })
                const registry = registries.find(item => item.id === event.target.value)
                if (registry?.scope === 'global')
                  form.setValue('accessScope', 'personal', { shouldValidate: true })
                onRegistryChange(event.target.value)
              }}
            >
              <option value="">{t('registriesPage.selectRegistry')}</option>
              {registries.map(registry => (
                <option key={registry.id} value={registry.id}>{registry.name}</option>
              ))}
            </Select>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.name?.message} hint={t('registriesPage.credentialNameHint')} label={t('registriesPage.name')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} />
            </Field>
            <Field error={form.formState.errors.scope?.message} hint={t('registriesPage.credentialScopeHint')} label={t('registriesPage.usage')} required>
              <Select {...form.register('scope')} aria-invalid={Boolean(form.formState.errors.scope)}>
                <option value="push-pull">{t('registriesPage.credentialScopePushPull')}</option>
                <option value="push">{t('registriesPage.credentialScopePush')}</option>
                <option value="pull">{t('registriesPage.credentialScopePull')}</option>
              </Select>
            </Field>
          </div>
          <Field error={form.formState.errors.accessScope?.message} hint={credentialRegistryIsGlobal ? t('registriesPage.credentialAccessScopeGlobalHint') : t('registriesPage.credentialAccessScopeHint')} label={t('registriesPage.credentialAccessScope')} required>
            <Select {...form.register('accessScope')} aria-invalid={Boolean(form.formState.errors.accessScope)} disabled={credentialRegistryIsGlobal}>
              <option value="personal">{t('registriesPage.credentialAccessScopePersonal')}</option>
              {!credentialRegistryIsGlobal && <option value="registry">{t('registriesPage.credentialAccessScopeRegistry')}</option>}
            </Select>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.repositoryTemplate?.message} hint={t('registriesPage.repositoryTemplateHint')} label={t('registriesPage.repositoryTemplate')} required>
              <Input {...form.register('repositoryTemplate')} aria-invalid={Boolean(form.formState.errors.repositoryTemplate)} placeholder={t('registriesPage.repositoryTemplatePlaceholder')} />
            </Field>
            <Field error={form.formState.errors.tagTemplate?.message} hint={t('registriesPage.tagTemplateHint')} label={t('registriesPage.tagTemplate')} required>
              <Input {...form.register('tagTemplate')} aria-invalid={Boolean(form.formState.errors.tagTemplate)} placeholder={t('registriesPage.tagTemplatePlaceholder')} />
            </Field>
          </div>
          <Field error={form.formState.errors.username?.message} hint={t('registriesPage.usernameHint')} label={t('registriesPage.username')}>
            <Input {...form.register('username')} aria-invalid={Boolean(form.formState.errors.username)} />
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.password?.message} hint={t('registriesPage.passwordHint')} label={t('registriesPage.password')}>
              <Input {...form.register('password')} aria-invalid={Boolean(form.formState.errors.password)} type="password" />
            </Field>
            <Field error={form.formState.errors.token?.message} hint={t('registriesPage.tokenHint')} label={t('registriesPage.token')}>
              <Input {...form.register('token')} aria-invalid={Boolean(form.formState.errors.token)} type="password" />
            </Field>
          </div>
          <DialogFooter>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              <KeyRound size={16} />
              {t('registriesPage.saveCredential')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface ImageDialogProps {
  open: boolean
  form: UseFormReturn<ImageForm>
  registries: ArtifactRegistry[]
  repositorySearch: string
  repositoryResultsOpen: boolean
  repositoryResults: {
    items: RegistryRepositoryItem[]
    isFetching: boolean
    isSuccess: boolean
    isError: boolean
    refetch: () => void
  }
  tagResults: {
    items: RegistryTagItem[]
    isFetching: boolean
  }
  pending: boolean
  onOpenChange: (open: boolean) => void
  onRepositorySearchChange: (value: string) => void
  onRepositoryResultsOpenChange: (open: boolean) => void
  onSelectRepository: (repository: RegistryRepositoryItem) => void
  onSubmit: (values: ImageForm) => void
}

export function ImageDialog({
  open,
  form,
  registries,
  repositorySearch,
  repositoryResultsOpen,
  repositoryResults,
  tagResults,
  pending,
  onOpenChange,
  onRepositorySearchChange,
  onRepositoryResultsOpenChange,
  onSelectRepository,
  onSubmit,
}: ImageDialogProps) {
  const { t } = useTranslation()
  const registryId = form.watch('registryId')

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('registriesPage.recordImageTitle')}</DialogTitle>
          <DialogDescription>{t('registriesPage.imageRegistryHint')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <Field error={form.formState.errors.registryId?.message} hint={t('registriesPage.imageRegistryHint')} label={t('registries')} required>
            <Select
              {...form.register('registryId', {
                onChange: () => {
                  form.setValue('repository', '', { shouldDirty: true, shouldValidate: true })
                  form.setValue('tag', '', { shouldDirty: true, shouldValidate: true })
                  onRepositorySearchChange('')
                  onRepositoryResultsOpenChange(false)
                },
              })}
              aria-invalid={Boolean(form.formState.errors.registryId)}
            >
              <option value="">{t('registriesPage.selectRegistry')}</option>
              {registries.map(registry => (
                <option key={registry.id} value={registry.id}>{registry.name}</option>
              ))}
            </Select>
          </Field>
          <Field error={form.formState.errors.repository?.message} hint={t('registriesPage.repositoryHint')} label={t('registriesPage.repository')} required>
            <div className="flex gap-2">
              <Input
                {...form.register('repository')}
                aria-invalid={Boolean(form.formState.errors.repository)}
                placeholder={t('registriesPage.repositoryPlaceholder')}
                value={repositorySearch}
                onChange={(event) => {
                  onRepositorySearchChange(event.target.value)
                  form.setValue('repository', event.target.value, { shouldDirty: true, shouldValidate: true })
                  onRepositoryResultsOpenChange(event.target.value.trim().length >= 2)
                }}
                onFocus={() => onRepositoryResultsOpenChange(Boolean(registryId && repositorySearch.trim().length >= 2))}
              />
              <Button
                disabled={!registryId || repositorySearch.trim().length < 2 || repositoryResults.isFetching}
                type="button"
                variant="secondary"
                onClick={() => {
                  onRepositoryResultsOpenChange(true)
                  repositoryResults.refetch()
                }}
              >
                <Search size={16} />
                {t('registriesPage.searchImages')}
              </Button>
            </div>
          </Field>
          {registryId && repositoryResultsOpen && (
            <div className="grid max-h-52 gap-2 overflow-y-auto rounded-md border border-border p-2">
              {repositoryResults.isFetching && (
                <p className="px-3 py-2 text-sm text-muted-foreground">{t('registriesPage.searchingImages')}</p>
              )}
              {repositoryResults.items.map(repository => (
                <button
                  key={repository.name}
                  className="rounded-md px-3 py-2 text-left hover:bg-muted"
                  type="button"
                  onClick={() => onSelectRepository(repository)}
                >
                  <span className="block text-sm font-medium">{repository.name}</span>
                  {repository.description && <span className="block truncate text-xs text-muted-foreground">{repository.description}</span>}
                </button>
              ))}
              {repositoryResults.isSuccess && repositoryResults.items.length === 0 && (
                <EmptyState description={t('registriesPage.noRepositoryResultsDescription')} title={t('registriesPage.noRepositoryResultsTitle')} variant="plain" />
              )}
              {repositoryResults.isError && (
                <EmptyState description={t('registriesPage.repositorySearchFailedDescription')} title={t('registriesPage.repositorySearchFailedTitle')} variant="plain" />
              )}
            </div>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.tag?.message} hint={t('registriesPage.tagHint')} label={t('registriesPage.tag')}>
              <Input
                {...form.register('tag')}
                aria-invalid={Boolean(form.formState.errors.tag)}
                list="registry-image-tag-options"
                placeholder={tagResults.isFetching ? t('registriesPage.loadingTags') : t('registriesPage.tagPlaceholder')}
              />
              <datalist id="registry-image-tag-options">
                {tagResults.items.map(tag => (
                  <option key={tag.name} value={tag.name}>{tag.digest}</option>
                ))}
              </datalist>
            </Field>
            <Field error={form.formState.errors.digest?.message} hint={t('registriesPage.digestHint')} label={t('registriesPage.digest')}>
              <Input {...form.register('digest')} aria-invalid={Boolean(form.formState.errors.digest)} />
            </Field>
          </div>
          <DialogFooter>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              <Container size={16} />
              {t('registriesPage.recordImage')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
