import type { UseFormReturn } from 'react-hook-form'
import type { CredentialForm, ImageForm, RegistryForm } from './registry-form-model'
import type { ArtifactRegistry, Project, RegistryCredential, RegistryRepositoryItem, RegistryTagItem } from '@/api'
import { KeyRound, PackagePlus, Plus, Save, Search } from 'lucide-react'
import { useId, useState } from 'react'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogBody, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { credentialDefaults, registryDefaults } from './registry-form-model'

interface RegistryDialogProps {
  canManageGlobal?: boolean
  open: boolean
  editingRegistry: ArtifactRegistry | null
  form: UseFormReturn<RegistryForm>
  projects: Project[]
  projectsError?: boolean
  projectsLoading?: boolean
  projectsRetrying?: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onRetryProjects?: () => void
  onSubmit: (values: RegistryForm) => void
}

export function RegistryDialog({
  canManageGlobal = false,
  open,
  editingRegistry,
  form,
  projects,
  projectsError = false,
  projectsLoading = false,
  projectsRetrying = false,
  pending,
  onOpenChange,
  onRetryProjects,
  onSubmit,
}: RegistryDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset({ ...registryDefaults, scope: canManageGlobal ? 'global' : 'user' })
      }}
    >
      <DialogContent className="max-w-2xl" scrollable>
        <DialogHeader>
          <DialogTitle>{editingRegistry ? t('registriesPage.editRegistryTitle') : t('registriesPage.createRegistryTitle')}</DialogTitle>
          <DialogDescription>{t('registriesPage.description')}</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 flex-1 flex-col overflow-hidden" onSubmit={form.handleSubmit(onSubmit)}>
          <DialogBody className="grid gap-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field error={form.formState.errors.name?.message} hint={t('registriesPage.registryNameHint')} label={t('registriesPage.name')} required>
                <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('registriesPage.registryNamePlaceholder')} />
              </Field>
              <Field error={form.formState.errors.provider?.message} hint={t('registriesPage.providerHint')} label={t('registriesPage.provider')} required>
                {fieldControl => (
                  <Controller
                    control={form.control}
                    name="provider"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange}>
                        <SelectTrigger {...fieldControl} className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="harbor">{t('registriesPage.providerHarbor')}</SelectItem>
                          <SelectItem value="dockerhub">{t('registriesPage.providerDockerHub')}</SelectItem>
                          <SelectItem value="gitea-registry">{t('registriesPage.providerGiteaRegistry')}</SelectItem>
                          <SelectItem value="generic-oci">{t('registriesPage.providerGenericOCI')}</SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  />
                )}
              </Field>
            </div>
            <Field error={form.formState.errors.endpoint?.message} hint={t('registriesPage.endpointHint')} label={t('registriesPage.endpoint')} required>
              <Input {...form.register('endpoint')} aria-invalid={Boolean(form.formState.errors.endpoint)} placeholder={t('registriesPage.endpointPlaceholder')} />
            </Field>
            <Field error={form.formState.errors.scope?.message} hint={t('registriesPage.registryScopeHint')} label={t('registriesPage.scope')} required>
              {fieldControl => (
                <Controller
                  control={form.control}
                  name="scope"
                  render={({ field }) => (
                    <Select
                      value={field.value}
                      onValueChange={(next) => {
                        if (next === field.value)
                          return
                        field.onChange(next)
                        if (next !== 'project')
                          form.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
                      }}
                    >
                      <SelectTrigger {...fieldControl} className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {canManageGlobal && <SelectItem value="global">{t('registriesPage.scopeGlobal')}</SelectItem>}
                        <SelectItem value="project">{t('registriesPage.scopeProject')}</SelectItem>
                        <SelectItem value="user">{t('registriesPage.scopeUser')}</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              )}
            </Field>
            {form.watch('scope') === 'project' && (
              <Field error={form.formState.errors.projectIds?.message} hint={t('registriesPage.ownerProjectHint')} label={t('registriesPage.ownerProject')}>
                {fieldControl => (
                  <div className="grid gap-1.5">
                    <ProjectSpaceMultiSelect
                      {...fieldControl}
                      error={projectsError}
                      loading={projectsLoading}
                      projects={projects}
                      value={form.watch('projectIds')}
                      onChange={value => form.setValue('projectIds', value, { shouldDirty: true, shouldValidate: true })}
                    />
                    {projectsError && onRetryProjects && (
                      <div className="flex items-center justify-between gap-2 text-xs text-danger" role="alert">
                        <span>{t('common.unavailable')}</span>
                        <Button disabled={projectsRetrying} size="sm" type="button" variant="outline" onClick={onRetryProjects}>
                          {t('common.retry')}
                        </Button>
                      </div>
                    )}
                  </div>
                )}
              </Field>
            )}
            <ProgressiveSection
              summary={[form.watch('capabilitiesText').trim() || t('registriesPage.noCapabilities'), form.watch('isDefault') ? t('common.default') : null].filter(Boolean).join(' · ')}
              title={t('registriesPage.advancedSettings')}
            >
              <Field error={form.formState.errors.capabilitiesText?.message} hint={t('registriesPage.capabilitiesHint')} label={t('registriesPage.capabilities')}>
                <Input {...form.register('capabilitiesText')} aria-invalid={Boolean(form.formState.errors.capabilitiesText)} />
              </Field>
              <Controller
                control={form.control}
                name="isDefault"
                render={({ field }) => (
                  <ControlledCheckboxField field={field}>
                    {t('registriesPage.setAsDefault')}
                  </ControlledCheckboxField>
                )}
              />
            </ProgressiveSection>
          </DialogBody>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">{t('common.cancel')}</Button>
            </DialogClose>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              {editingRegistry ? <Save size={16} /> : <Plus size={16} />}
              {editingRegistry ? t('registriesPage.saveRegistry') : t('registriesPage.createRegistry')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface CredentialDialogProps {
  canManageGlobal?: boolean
  open: boolean
  editingCredential: RegistryCredential | null
  form: UseFormReturn<CredentialForm>
  registries: ArtifactRegistry[]
  registriesError?: boolean
  registriesLoading?: boolean
  registriesRetrying?: boolean
  projects: Project[]
  projectsError?: boolean
  projectsLoading?: boolean
  projectsRetrying?: boolean
  defaultRegistryId: string
  pending: boolean
  onOpenChange: (open: boolean) => void
  onRetryProjects?: () => void
  onRetryRegistries?: () => void
  onSubmit: (values: CredentialForm) => void
}

export function CredentialDialog({
  canManageGlobal = false,
  editingCredential,
  open,
  form,
  registries,
  registriesError = false,
  registriesLoading = false,
  registriesRetrying = false,
  projects,
  projectsError = false,
  projectsLoading = false,
  projectsRetrying = false,
  defaultRegistryId,
  pending,
  onOpenChange,
  onRetryProjects,
  onRetryRegistries,
  onSubmit,
}: CredentialDialogProps) {
  const { t } = useTranslation()
  const credentialRegistry = registries.find(registry => registry.id === form.watch('registryId'))
  const allowedProjects = credentialRegistry?.scope === 'project'
    ? projects.filter(project => credentialRegistry.projectIds.includes(project.id))
    : projects
  const secretProvided = (form.watch('password') ?? '').trim() !== '' || (form.watch('token') ?? '').trim() !== ''
  const secretRequired = !editingCredential
  const repositoryTemplate = form.watch('repositoryTemplate') || credentialDefaults.repositoryTemplate
  const tagTemplate = form.watch('tagTemplate') || credentialDefaults.tagTemplate
  const credentialScope = form.watch('scope')
  const credentialScopeSummary = t(`registriesPage.scope${credentialScope.charAt(0).toUpperCase()}${credentialScope.slice(1)}`)

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset({ ...credentialDefaults, registryId: defaultRegistryId })
      }}
    >
      <DialogContent scrollable>
        <DialogHeader>
          <DialogTitle>{editingCredential ? t('registriesPage.editCredentialTitle') : t('registriesPage.createCredentialTitle')}</DialogTitle>
          <DialogDescription>{editingCredential ? t('registriesPage.editCredentialDescription') : t('registriesPage.credentialRegistryHint')}</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 flex-1 flex-col overflow-hidden" onSubmit={form.handleSubmit(onSubmit)}>
          <DialogBody className="grid gap-3">
            <Field error={form.formState.errors.registryId?.message} hint={t('registriesPage.credentialRegistryHint')} label={t('registries')} required>
              {fieldControl => (
                <div className="grid gap-1.5">
                  <Controller
                    control={form.control}
                    name="registryId"
                    render={({ field }) => (
                      <Select
                        disabled={Boolean(editingCredential) || registriesLoading || registriesError}
                        value={!registriesLoading && !registriesError && registries.some(registry => registry.id === field.value) ? field.value : ''}
                        onValueChange={(next) => {
                          if (!next || next === field.value)
                            return
                          field.onChange(next)
                          form.setValue('scope', 'user', { shouldDirty: true, shouldValidate: true })
                          form.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
                        }}
                      >
                        <SelectTrigger {...fieldControl} className="w-full">
                          <SelectValue
                            placeholder={
                              registriesLoading
                                ? t('common.loading')
                                : registriesError
                                  ? t('common.unavailable')
                                  : registries.length === 0
                                    ? t('common.noOptions')
                                    : t('registriesPage.selectRegistry')
                            }
                          />
                        </SelectTrigger>
                        <SelectContent>
                          {registriesLoading
                            ? <SelectItem disabled value="__loading__">{t('common.loading')}</SelectItem>
                            : registriesError
                              ? <SelectItem disabled value="__error__">{t('common.unavailable')}</SelectItem>
                              : registries.length === 0
                                ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                                : registries.map(registry => (
                                    <SelectItem key={registry.id} value={registry.id}>{registry.name}</SelectItem>
                                  ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                  {registriesError && onRetryRegistries && (
                    <div className="flex items-center justify-between gap-2 text-xs text-danger" role="alert">
                      <span>{t('common.unavailable')}</span>
                      <Button disabled={registriesRetrying} size="sm" type="button" variant="outline" onClick={onRetryRegistries}>
                        {t('common.retry')}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field error={form.formState.errors.name?.message} hint={t('registriesPage.credentialNameHint')} label={t('registriesPage.name')} required>
                <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} />
              </Field>
              <Field error={form.formState.errors.usage?.message} hint={t('registriesPage.credentialScopeHint')} label={t('registriesPage.usage')} required>
                {fieldControl => (
                  <Controller
                    control={form.control}
                    name="usage"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange}>
                        <SelectTrigger {...fieldControl} className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="push-pull">{t('registriesPage.credentialScopePushPull')}</SelectItem>
                          <SelectItem value="push">{t('registriesPage.credentialScopePush')}</SelectItem>
                          <SelectItem value="pull">{t('registriesPage.credentialScopePull')}</SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  />
                )}
              </Field>
            </div>
            <Field error={form.formState.errors.username?.message} hint={t('registriesPage.usernameHint')} label={t('registriesPage.username')}>
              <Input {...form.register('username')} aria-invalid={Boolean(form.formState.errors.username)} autoComplete="off" />
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field error={form.formState.errors.password?.message} hint={editingCredential ? t('registriesPage.passwordEditHint') : t('registriesPage.passwordHint')} label={t('registriesPage.password')}>
                <Input
                  {...form.register('password')}
                  aria-invalid={Boolean(form.formState.errors.password)}
                  autoComplete="off"
                  placeholder={editingCredential?.passwordSet ? t('common.secretSetPlaceholder') : undefined}
                  type="password"
                />
              </Field>
              <Field error={form.formState.errors.token?.message} hint={editingCredential ? t('registriesPage.tokenEditHint') : t('registriesPage.tokenHint')} label={t('registriesPage.token')}>
                <Input
                  {...form.register('token')}
                  aria-invalid={Boolean(form.formState.errors.token)}
                  autoComplete="off"
                  placeholder={editingCredential?.tokenSet ? t('common.secretSetPlaceholder') : undefined}
                  type="password"
                />
              </Field>
            </div>
            <ProgressiveSection
              description={t('registriesPage.credentialAccessScopeHint')}
              summary={credentialScopeSummary}
              title={t('registriesPage.credentialAccessScope')}
            >
              <Field error={form.formState.errors.scope?.message} hint={t('registriesPage.credentialAccessScopeHint')} label={t('registriesPage.credentialAccessScope')} required>
                {fieldControl => (
                  <Controller
                    control={form.control}
                    name="scope"
                    render={({ field }) => (
                      <Select
                        value={field.value}
                        onValueChange={(next) => {
                          if (next === field.value)
                            return
                          field.onChange(next)
                          if (next !== 'project')
                            form.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
                        }}
                      >
                        <SelectTrigger {...fieldControl} className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {canManageGlobal && credentialRegistry?.scope === 'global' && <SelectItem value="global">{t('registriesPage.scopeGlobal')}</SelectItem>}
                          {credentialRegistry?.scope !== 'user' && <SelectItem value="project">{t('registriesPage.scopeProject')}</SelectItem>}
                          <SelectItem value="user">{t('registriesPage.scopeUser')}</SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  />
                )}
              </Field>
              {credentialScope === 'project' && (
                <Field error={form.formState.errors.projectIds?.message} hint={t('registriesPage.ownerProjectHint')} label={t('registriesPage.ownerProject')} required>
                  {fieldControl => (
                    <div className="grid gap-1.5">
                      <ProjectSpaceMultiSelect
                        {...fieldControl}
                        error={projectsError}
                        loading={projectsLoading}
                        projects={allowedProjects}
                        value={form.watch('projectIds')}
                        onChange={value => form.setValue('projectIds', value, { shouldDirty: true, shouldValidate: true })}
                      />
                      {projectsError && onRetryProjects && (
                        <div className="flex items-center justify-between gap-2 text-xs text-danger" role="alert">
                          <span>{t('common.unavailable')}</span>
                          <Button disabled={projectsRetrying} size="sm" type="button" variant="outline" onClick={onRetryProjects}>
                            {t('common.retry')}
                          </Button>
                        </div>
                      )}
                    </div>
                  )}
                </Field>
              )}
            </ProgressiveSection>
            <ProgressiveSection
              summary={t('registriesPage.credentialNamingRulesSummary', { repositoryTemplate, tagTemplate })}
              title={t('registriesPage.credentialNamingRulesTitle')}
            >
              <div className="grid gap-3 sm:grid-cols-2">
                <Field error={form.formState.errors.repositoryTemplate?.message} hint={t('registriesPage.repositoryTemplateHint')} label={t('registriesPage.repositoryTemplate')} required>
                  <Input {...form.register('repositoryTemplate')} aria-invalid={Boolean(form.formState.errors.repositoryTemplate)} placeholder={t('registriesPage.repositoryTemplatePlaceholder')} />
                </Field>
                <Field error={form.formState.errors.tagTemplate?.message} hint={t('registriesPage.tagTemplateHint')} label={t('registriesPage.tagTemplate')} required>
                  <Input {...form.register('tagTemplate')} aria-invalid={Boolean(form.formState.errors.tagTemplate)} placeholder={t('registriesPage.tagTemplatePlaceholder')} />
                </Field>
              </div>
            </ProgressiveSection>
          </DialogBody>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">{t('common.cancel')}</Button>
            </DialogClose>
            <Button disabled={pending || !form.formState.isValid || (secretRequired && !secretProvided)} type="submit">
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
  registriesError?: boolean
  registriesLoading?: boolean
  registriesRetrying?: boolean
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
  onRetryRegistries?: () => void
  onRepositorySearchChange: (value: string) => void
  onRepositoryResultsOpenChange: (open: boolean) => void
  onSelectRepository: (repository: RegistryRepositoryItem) => void
  onSubmit: (values: ImageForm) => void
}

export function ImageDialog({
  open,
  form,
  registries,
  registriesError = false,
  registriesLoading = false,
  registriesRetrying = false,
  repositorySearch,
  repositoryResultsOpen,
  repositoryResults,
  tagResults,
  pending,
  onOpenChange,
  onRetryRegistries,
  onRepositorySearchChange,
  onRepositoryResultsOpenChange,
  onSelectRepository,
  onSubmit,
}: ImageDialogProps) {
  const { t } = useTranslation()
  const listboxId = useId()
  const [activeIndex, setActiveIndex] = useState(-1)
  const registryId = form.watch('registryId')
  const showRepositoryResults = Boolean(registryId && repositoryResultsOpen)
  const safeActiveIndex = activeIndex >= 0 && activeIndex < repositoryResults.items.length ? activeIndex : -1
  const activeOptionId = safeActiveIndex >= 0 ? `${listboxId}-opt-${safeActiveIndex}` : undefined

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen) {
          setActiveIndex(-1)
          form.reset()
        }
      }}
    >
      <DialogContent scrollable>
        <DialogHeader>
          <DialogTitle>{t('registriesPage.recordImageTitle')}</DialogTitle>
          <DialogDescription>{t('registriesPage.imageRegistryHint')}</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 flex-1 flex-col overflow-hidden" onSubmit={form.handleSubmit(onSubmit)}>
          <DialogBody className="grid gap-3">
            <Field error={form.formState.errors.registryId?.message} hint={t('registriesPage.imageRegistryHint')} label={t('registries')} required>
              {fieldControl => (
                <div className="grid gap-1.5">
                  <Controller
                    control={form.control}
                    name="registryId"
                    render={({ field }) => (
                      <Select
                        disabled={registriesLoading || registriesError}
                        value={!registriesLoading && !registriesError && registries.some(registry => registry.id === field.value) ? field.value : ''}
                        onValueChange={(next) => {
                          if (!next || next === field.value)
                            return
                          field.onChange(next)
                          form.setValue('repository', '', { shouldDirty: true, shouldValidate: true })
                          form.setValue('tag', '', { shouldDirty: true, shouldValidate: true })
                          onRepositorySearchChange('')
                          onRepositoryResultsOpenChange(false)
                          setActiveIndex(-1)
                        }}
                      >
                        <SelectTrigger {...fieldControl} className="w-full">
                          <SelectValue
                            placeholder={
                              registriesLoading
                                ? t('common.loading')
                                : registriesError
                                  ? t('common.unavailable')
                                  : registries.length === 0
                                    ? t('common.noOptions')
                                    : t('registriesPage.selectRegistry')
                            }
                          />
                        </SelectTrigger>
                        <SelectContent>
                          {registriesLoading
                            ? <SelectItem disabled value="__loading__">{t('common.loading')}</SelectItem>
                            : registriesError
                              ? <SelectItem disabled value="__error__">{t('common.unavailable')}</SelectItem>
                              : registries.length === 0
                                ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                                : registries.map(registry => (
                                    <SelectItem key={registry.id} value={registry.id}>{registry.name}</SelectItem>
                                  ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                  {registriesError && onRetryRegistries && (
                    <div className="flex items-center justify-between gap-2 text-xs text-danger" role="alert">
                      <span>{t('common.unavailable')}</span>
                      <Button disabled={registriesRetrying} size="sm" type="button" variant="outline" onClick={onRetryRegistries}>
                        {t('common.retry')}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </Field>
            <Field error={form.formState.errors.repository?.message} hint={t('registriesPage.repositoryHint')} label={t('registriesPage.repository')} required>
              {fieldControl => (
                <div className="flex gap-2">
                  <Input
                    {...form.register('repository', {
                      onChange: (event) => {
                        const nextValue = event.target.value
                        onRepositorySearchChange(nextValue)
                        onRepositoryResultsOpenChange(nextValue.trim().length >= 2)
                        setActiveIndex(-1)
                      },
                    })}
                    {...fieldControl}
                    aria-activedescendant={activeOptionId}
                    aria-autocomplete="list"
                    aria-controls={showRepositoryResults ? listboxId : undefined}
                    aria-expanded={showRepositoryResults}
                    placeholder={t('registriesPage.repositoryPlaceholder')}
                    role="combobox"
                    onFocus={() => onRepositoryResultsOpenChange(Boolean(registryId && repositorySearch.trim().length >= 2))}
                    onKeyDown={(event) => {
                      if (!showRepositoryResults)
                        return
                      if (event.key === 'Escape') {
                        event.preventDefault()
                        onRepositoryResultsOpenChange(false)
                        setActiveIndex(-1)
                        return
                      }
                      const total = repositoryResults.items.length
                      if (total === 0)
                        return
                      if (event.key === 'ArrowDown') {
                        event.preventDefault()
                        setActiveIndex(current => (current + 1) % total)
                      }
                      else if (event.key === 'ArrowUp') {
                        event.preventDefault()
                        setActiveIndex(current => (current <= 0 ? total - 1 : current - 1))
                      }
                      else if (event.key === 'Enter' && safeActiveIndex >= 0) {
                        event.preventDefault()
                        const chosen = repositoryResults.items[safeActiveIndex]
                        if (chosen) {
                          onSelectRepository(chosen)
                          setActiveIndex(-1)
                        }
                      }
                    }}
                  />
                  <Button
                    disabled={!registryId || repositorySearch.trim().length < 2 || repositoryResults.isFetching}
                    type="button"
                    variant="outline"
                    onClick={() => {
                      onRepositoryResultsOpenChange(true)
                      repositoryResults.refetch()
                    }}
                  >
                    <Search size={16} />
                    {t('registriesPage.searchImages')}
                  </Button>
                </div>
              )}
            </Field>
            {showRepositoryResults && (
              <div
                aria-label={t('registriesPage.repository')}
                className="grid max-h-52 gap-1 overflow-y-auto rounded-control border border-border p-1"
                id={listboxId}
                role="listbox"
              >
                {repositoryResults.isFetching && repositoryResults.items.length === 0
                  ? <p className="px-3 py-2 text-sm text-muted-foreground">{t('registriesPage.searchingImages')}</p>
                  : repositoryResults.isError
                    ? (
                        <ErrorState
                          description={t('registriesPage.repositorySearchFailedDescription')}
                          retrying={repositoryResults.isFetching}
                          title={t('registriesPage.repositorySearchFailedTitle')}
                          onRetry={() => repositoryResults.refetch()}
                        />
                      )
                    : repositoryResults.isSuccess && repositoryResults.items.length === 0
                      ? (
                          <EmptyState description={t('registriesPage.noRepositoryResultsDescription')} title={t('registriesPage.noRepositoryResultsTitle')} variant="plain" />
                        )
                      : (
                          <>
                            {repositoryResults.isFetching && (
                              <p className="px-3 py-2 text-sm text-muted-foreground">{t('registriesPage.searchingImages')}</p>
                            )}
                            {repositoryResults.items.map((repository, index) => (
                              <div
                                key={repository.name}
                                aria-selected={safeActiveIndex === index}
                                className={cn(
                                  'cursor-pointer rounded-control px-3 py-2 text-left transition-colors hover:bg-muted',
                                  safeActiveIndex === index && 'bg-accent text-accent-foreground',
                                )}
                                id={`${listboxId}-opt-${index}`}
                                role="option"
                                tabIndex={-1}
                                onClick={() => {
                                  onSelectRepository(repository)
                                  setActiveIndex(-1)
                                }}
                                onMouseDown={event => event.preventDefault()}
                              >
                                <span className="block text-sm font-medium">{repository.name}</span>
                                {repository.description && <span className="block truncate text-xs text-muted-foreground">{repository.description}</span>}
                              </div>
                            ))}
                          </>
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
          </DialogBody>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">{t('common.cancel')}</Button>
            </DialogClose>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              <PackagePlus size={16} />
              {t('registriesPage.recordImage')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
