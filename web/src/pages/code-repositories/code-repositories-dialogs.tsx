import type { UseFormReturn } from 'react-hook-form'
import type { CredentialForm, ProviderForm } from './code-repositories-form-model'
import type { GitAccount, GitProvider, Project } from '@/api'
import { Info, Plus, Save } from 'lucide-react'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogBody, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { credentialDefaults } from './code-repositories-form-model'
import { CredentialOAuthGuide, GitProviderIcon, OAuthAppGuide } from './code-repositories-panels'
import { gitProviderGuide, normalizeGitBaseUrl } from './code-repositories-utils'

interface ProviderDialogProps {
  open: boolean
  editingProvider: GitProvider | null
  form: UseFormReturn<ProviderForm>
  projects: Project[]
  projectsError?: boolean
  projectsLoading?: boolean
  projectsRetrying?: boolean
  hasAnotherGithubProvider: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onRetryProjects?: () => void
  onSubmit: (values: ProviderForm) => void
}

export function ProviderDialog({
  open,
  editingProvider,
  form,
  projects,
  projectsError = false,
  projectsLoading = false,
  projectsRetrying = false,
  hasAnotherGithubProvider,
  pending,
  onOpenChange,
  onRetryProjects,
  onSubmit,
}: ProviderDialogProps) {
  const { t } = useTranslation()
  const providerType = form.watch('type')
  const providerBaseUrl = form.watch('baseUrl')
  const providerName = form.watch('name')
  const providerAuthType = form.watch('authType')
  const providerScope = form.watch('scope')
  const isGithubProvider = providerType === 'github'
  const providerGuide = gitProviderGuide(providerType, providerBaseUrl, providerName)

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
    >
      <DialogContent className="sm:max-w-2xl" scrollable>
        <DialogHeader>
          <DialogTitle>{editingProvider ? t('codeRepositoriesView.editProvider') : t('codeRepositoriesView.createProvider')}</DialogTitle>
          <DialogDescription>{t('codeRepositoriesView.providerDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 flex-1 flex-col overflow-hidden" onSubmit={form.handleSubmit(onSubmit)}>
          <DialogBody className="grid gap-4">
            <section className="grid gap-3">
              <h3 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
                {t('codeRepositoriesView.providerBasicSectionTitle')}
                <HelpTip label={t('codeRepositoriesView.providerBasicSectionTitle')}>{t('codeRepositoriesView.providerBasicSectionDescription')}</HelpTip>
              </h3>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field error={form.formState.errors.name?.message} hint={t('codeRepositoriesView.providerNameHint')} label={t('codeRepositoriesView.name')} required><Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('codeRepositoriesView.providerNamePlaceholder')} /></Field>
                <Field error={form.formState.errors.type?.message} hint={t('codeRepositoriesView.providerTypeHint')} label={t('codeRepositoriesView.type')} required>
                  {fieldControl => (
                    <div className="flex gap-2">
                      <GitProviderIcon baseUrl={providerBaseUrl} className="flex size-9 shrink-0 items-center justify-center rounded-control border border-border bg-surface-subtle p-1.5" type={providerType} />
                      <Controller
                        control={form.control}
                        name="type"
                        render={({ field }) => (
                          <Select
                            value={field.value}
                            onValueChange={(next) => {
                              if (next === field.value)
                                return
                              field.onChange(next)
                              if (next === 'github') {
                                form.setValue('baseUrl', normalizeGitBaseUrl('github'), { shouldDirty: true, shouldValidate: true })
                                form.setValue('scope', 'global', { shouldDirty: true, shouldValidate: true })
                                form.setValue('ownerRef', '', { shouldDirty: true, shouldValidate: true })
                                form.setValue('projectIds', [], { shouldDirty: true, shouldValidate: true })
                              }
                            }}
                          >
                            <SelectTrigger {...fieldControl} className="w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem disabled={hasAnotherGithubProvider && (!editingProvider || editingProvider.type !== 'github')} value="github">
                                {t('codeRepositoriesView.github')}
                              </SelectItem>
                              <SelectItem value="gitea">{t('codeRepositoriesView.gitea')}</SelectItem>
                              <SelectItem value="gitlab">{t('codeRepositoriesView.gitlab')}</SelectItem>
                            </SelectContent>
                          </Select>
                        )}
                      />
                    </div>
                  )}
                </Field>
              </div>
              {hasAnotherGithubProvider && !editingProvider?.type && (
                <Alert>
                  <Info />
                  <AlertDescription>{t('codeRepositoriesView.githubProviderOnlyOne')}</AlertDescription>
                </Alert>
              )}
              <Field error={form.formState.errors.baseUrl?.message} hint={t('codeRepositoriesView.baseUrlHint')} label={t('codeRepositoriesView.baseUrl')}>
                <Input
                  {...form.register('baseUrl')}
                  aria-invalid={Boolean(form.formState.errors.baseUrl)}
                  disabled={isGithubProvider}
                  placeholder={t('codeRepositoriesView.baseUrlPlaceholder')}
                />
              </Field>
            </section>
            <ProgressiveSection
              description={t('codeRepositoriesView.providerAdvancedSectionDescription')}
              summary={t('codeRepositoriesView.providerAdvancedSectionSummary', {
                authType: providerAuthType === 'oauth' ? t('codeRepositoriesView.oauth') : t('codeRepositoriesView.pat'),
                scope: t(`codeRepositoriesView.scope${providerScope.charAt(0).toUpperCase()}${providerScope.slice(1)}`),
                secretState: editingProvider?.clientSecretSet ? t('codeRepositoriesView.secretSet') : t('codeRepositoriesView.secretNotSet'),
              })}
              title={t('codeRepositoriesView.providerAdvancedSectionTitle')}
            >
              <Field error={form.formState.errors.scope?.message} hint={t('codeRepositoriesView.scopeHint')} label={t('codeRepositoriesView.scope')} required>
                {fieldControl => (
                  <Controller
                    control={form.control}
                    name="scope"
                    render={({ field }) => (
                      <Select
                        disabled={isGithubProvider}
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
                          <SelectItem value="global">{t('codeRepositoriesView.scopeGlobal')}</SelectItem>
                          <SelectItem value="project">{t('codeRepositoriesView.scopeProject')}</SelectItem>
                          <SelectItem value="user">{t('codeRepositoriesView.scopeUser')}</SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  />
                )}
              </Field>
              {providerScope === 'project' && (
                <Field error={form.formState.errors.projectIds?.message} hint={t('codeRepositoriesView.ownerProjectHint')} label={t('codeRepositoriesView.ownerProject')} required>
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
              <div className="grid gap-3 sm:grid-cols-2">
                <Field error={form.formState.errors.authType?.message} hint={t('codeRepositoriesView.authTypeHint')} label={t('codeRepositoriesView.authType')} required>
                  {fieldControl => (
                    <Controller
                      control={form.control}
                      name="authType"
                      render={({ field }) => (
                        <Select value={field.value} onValueChange={field.onChange}>
                          <SelectTrigger {...fieldControl} className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="oauth">{t('codeRepositoriesView.oauth')}</SelectItem>
                            <SelectItem value="pat">{t('codeRepositoriesView.pat')}</SelectItem>
                          </SelectContent>
                        </Select>
                      )}
                    />
                  )}
                </Field>
                <Field error={form.formState.errors.clientId?.message} hint={t('codeRepositoriesView.clientIdHint')} label={t('codeRepositoriesView.clientId')}><Input {...form.register('clientId')} aria-invalid={Boolean(form.formState.errors.clientId)} /></Field>
              </div>
              <Field error={form.formState.errors.clientSecret?.message} hint={t('codeRepositoriesView.clientSecretHint')} label={t('codeRepositoriesView.clientSecret')}>
                <Input
                  {...form.register('clientSecret')}
                  aria-invalid={Boolean(form.formState.errors.clientSecret)}
                  autoComplete="off"
                  placeholder={editingProvider?.clientSecretSet ? t('common.secretSetPlaceholder') : t('codeRepositoriesView.clientSecretPlaceholder')}
                  type="password"
                />
              </Field>
              {providerAuthType === 'oauth' && <OAuthAppGuide guide={providerGuide} />}
            </ProgressiveSection>
          </DialogBody>
          <DialogFooter>
            <DialogClose asChild>
              <Button disabled={pending} type="button" variant="outline">{t('common.cancel')}</Button>
            </DialogClose>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              {editingProvider ? <Save size={16} /> : <Plus size={16} />}
              {editingProvider ? t('codeRepositoriesView.saveProvider') : t('codeRepositoriesView.createProvider')}
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
  editingCredential: GitAccount | null
  form: UseFormReturn<CredentialForm>
  projects: Project[]
  projectsError?: boolean
  projectsLoading?: boolean
  projectsRetrying?: boolean
  providers: GitProvider[]
  providersError?: boolean
  providersLoading?: boolean
  providersRetrying?: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onRetryProjects?: () => void
  onRetryProviders?: () => void
  onSubmit: (values: CredentialForm) => void
}

export function CredentialDialog({
  canManageGlobal = false,
  open,
  editingCredential,
  form,
  projects,
  projectsError = false,
  projectsLoading = false,
  projectsRetrying = false,
  providers,
  providersError = false,
  providersLoading = false,
  providersRetrying = false,
  pending,
  onOpenChange,
  onRetryProjects,
  onRetryProviders,
  onSubmit,
}: CredentialDialogProps) {
  const { t } = useTranslation()
  const selectedProvider = providers.find(provider => provider.id === form.watch('providerId'))
  const credentialScope = form.watch('scope')
  const allowedProjects = selectedProvider?.scope === 'project'
    ? projects.filter(project => selectedProvider.projectIds.includes(project.id))
    : projects

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset(credentialDefaults)
      }}
    >
      <DialogContent scrollable>
        <DialogHeader>
          <DialogTitle>{editingCredential ? t('codeRepositoriesView.editCredential') : t('codeRepositoriesView.createCredential')}</DialogTitle>
          <DialogDescription>{t(editingCredential ? 'codeRepositoriesView.editCredentialDialogDescription' : 'codeRepositoriesView.credentialDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="flex min-h-0 flex-1 flex-col overflow-hidden" onSubmit={form.handleSubmit(onSubmit)}>
          <DialogBody className="grid gap-3">
            <Field error={form.formState.errors.providerId?.message} label={t('codeRepositoriesView.provider')} required>
              {fieldControl => (
                <div className="grid gap-1.5">
                  <Controller
                    control={form.control}
                    name="providerId"
                    render={({ field }) => (
                      <Select
                        disabled={Boolean(editingCredential) || providersLoading || providersError}
                        value={!providersLoading && !providersError && providers.some(provider => provider.id === field.value) ? field.value : ''}
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
                              providersLoading
                                ? t('common.loading')
                                : providersError
                                  ? t('common.unavailable')
                                  : providers.length === 0
                                    ? t('common.noOptions')
                                    : t('codeRepositoriesView.selectProvider')
                            }
                          />
                        </SelectTrigger>
                        <SelectContent>
                          {providersLoading
                            ? <SelectItem disabled value="__loading__">{t('common.loading')}</SelectItem>
                            : providersError
                              ? <SelectItem disabled value="__error__">{t('common.unavailable')}</SelectItem>
                              : providers.length === 0
                                ? <SelectItem disabled value="__empty__">{t('common.noOptions')}</SelectItem>
                                : providers.map(provider => (
                                    <SelectItem key={provider.id} value={provider.id}>{provider.name}</SelectItem>
                                  ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                  {providersError && onRetryProviders && (
                    <div className="flex items-center justify-between gap-2 text-xs text-danger" role="alert">
                      <span>{t('common.unavailable')}</span>
                      <Button disabled={providersRetrying} size="sm" type="button" variant="outline" onClick={onRetryProviders}>
                        {t('common.retry')}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </Field>
            {selectedProvider && <CredentialOAuthGuide provider={selectedProvider} />}
            <Field error={form.formState.errors.scope?.message} hint={t('codeRepositoriesView.scopeHint')} label={t('codeRepositoriesView.scope')} required>
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
                        {canManageGlobal && selectedProvider?.scope === 'global' && <SelectItem value="global">{t('codeRepositoriesView.scopeGlobal')}</SelectItem>}
                        {selectedProvider?.scope !== 'user' && <SelectItem value="project">{t('codeRepositoriesView.scopeProject')}</SelectItem>}
                        <SelectItem value="user">{t('codeRepositoriesView.scopeUser')}</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              )}
            </Field>
            {credentialScope === 'project' && (
              <Field error={form.formState.errors.projectIds?.message} hint={t('codeRepositoriesView.ownerProjectHint')} label={t('codeRepositoriesView.ownerProject')} required>
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
            <Field error={form.formState.errors.username?.message} hint={t('codeRepositoriesView.usernameHint')} label={t('codeRepositoriesView.username')} required><Input {...form.register('username')} aria-invalid={Boolean(form.formState.errors.username)} autoComplete="off" placeholder={t('codeRepositoriesView.usernamePlaceholder')} /></Field>
            <Field error={form.formState.errors.accessToken?.message} hint={t(editingCredential ? 'codeRepositoriesView.accessTokenEditHint' : 'codeRepositoriesView.accessTokenHint')} label={t('codeRepositoriesView.accessToken')}>
              <Input
                {...form.register('accessToken')}
                aria-invalid={Boolean(form.formState.errors.accessToken)}
                autoComplete="off"
                placeholder={editingCredential?.accessTokenSet ? t('common.secretSetPlaceholder') : undefined}
                type="password"
              />
            </Field>
            <Field error={form.formState.errors.scopesText?.message} hint={t('codeRepositoriesView.scopesHint')} label={t('codeRepositoriesView.scopes')}>
              <Input {...form.register('scopesText')} aria-invalid={Boolean(form.formState.errors.scopesText)} />
            </Field>
          </DialogBody>
          <DialogFooter>
            <DialogClose asChild>
              <Button disabled={pending} type="button" variant="outline">{t('common.cancel')}</Button>
            </DialogClose>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              {editingCredential ? <Save size={16} /> : <Plus size={16} />}
              {editingCredential ? t('codeRepositoriesView.saveCredential') : t('codeRepositoriesView.createCredential')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
