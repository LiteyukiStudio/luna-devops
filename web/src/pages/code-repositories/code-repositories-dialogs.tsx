import type { UseFormReturn } from 'react-hook-form'
import type { CredentialForm, ProviderForm } from './code-repositories-form-model'
import type { GitProvider, Project } from '@/api'
import { Info, Plus, Save } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { credentialDefaults } from './code-repositories-form-model'
import { CredentialOAuthGuide, GitProviderIcon, OAuthAppGuide } from './code-repositories-panels'
import { gitProviderGuide } from './code-repositories-utils'

interface ProviderDialogProps {
  open: boolean
  editingProvider: GitProvider | null
  form: UseFormReturn<ProviderForm>
  projects: Project[]
  hasAnotherGithubProvider: boolean
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (values: ProviderForm) => void
}

export function ProviderDialog({
  open,
  editingProvider,
  form,
  projects,
  hasAnotherGithubProvider,
  pending,
  onOpenChange,
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
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{editingProvider ? t('codeRepositoriesView.editProvider') : t('codeRepositoriesView.createProvider')}</DialogTitle>
          <DialogDescription>{t('codeRepositoriesView.providerDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.name?.message} hint={t('codeRepositoriesView.providerNameHint')} label={t('codeRepositoriesView.name')} required><Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('codeRepositoriesView.providerNamePlaceholder')} /></Field>
            <Field error={form.formState.errors.type?.message} hint={t('codeRepositoriesView.providerTypeHint')} label={t('codeRepositoriesView.type')} required>
              <div className="flex gap-2">
                <GitProviderIcon baseUrl={providerBaseUrl} type={providerType} />
                <Select {...form.register('type')} aria-invalid={Boolean(form.formState.errors.type)}>
                  <option value="github" disabled={hasAnotherGithubProvider && (!editingProvider || editingProvider.type !== 'github')}>{t('codeRepositoriesView.github')}</option>
                  <option value="gitea">{t('codeRepositoriesView.gitea')}</option>
                  <option value="gitlab">{t('codeRepositoriesView.gitlab')}</option>
                </Select>
              </div>
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
          <Field error={form.formState.errors.scope?.message} hint={t('codeRepositoriesView.scopeHint')} label={t('codeRepositoriesView.scope')} required>
            <Select {...form.register('scope')} aria-invalid={Boolean(form.formState.errors.scope)} disabled={isGithubProvider}>
              <option value="global">{t('codeRepositoriesView.scopeGlobal')}</option>
              <option value="project">{t('codeRepositoriesView.scopeProject')}</option>
              <option value="user">{t('codeRepositoriesView.scopeUser')}</option>
            </Select>
          </Field>
          {providerScope === 'project' && (
            <Field error={form.formState.errors.projectIds?.message} hint={t('codeRepositoriesView.ownerProjectHint')} label={t('codeRepositoriesView.ownerProject')} required>
              <ProjectSpaceMultiSelect
                projects={projects}
                value={form.watch('projectIds')}
                onChange={value => form.setValue('projectIds', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.authType?.message} hint={t('codeRepositoriesView.authTypeHint')} label={t('codeRepositoriesView.authType')} required>
              <Select {...form.register('authType')} aria-invalid={Boolean(form.formState.errors.authType)}>
                <option value="oauth">{t('codeRepositoriesView.oauth')}</option>
                <option value="pat">{t('codeRepositoriesView.pat')}</option>
              </Select>
            </Field>
            <Field error={form.formState.errors.clientId?.message} hint={t('codeRepositoriesView.clientIdHint')} label={t('codeRepositoriesView.clientId')}><Input {...form.register('clientId')} aria-invalid={Boolean(form.formState.errors.clientId)} /></Field>
          </div>
          <Field error={form.formState.errors.clientSecret?.message} hint={t('codeRepositoriesView.clientSecretHint')} label={t('codeRepositoriesView.clientSecret')}>
            <Input
              {...form.register('clientSecret')}
              aria-invalid={Boolean(form.formState.errors.clientSecret)}
              placeholder={editingProvider?.clientSecretSet ? t('codeRepositoriesView.secretSetPlaceholder') : t('codeRepositoriesView.clientSecretPlaceholder')}
              type="password"
            />
          </Field>
          {providerAuthType === 'oauth' && <OAuthAppGuide guide={providerGuide} />}
          <DialogFooter>
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
  open: boolean
  form: UseFormReturn<CredentialForm>
  projects: Project[]
  providers: GitProvider[]
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (values: CredentialForm) => void
}

export function CredentialDialog({ open, form, projects, providers, pending, onOpenChange, onSubmit }: CredentialDialogProps) {
  const { t } = useTranslation()
  const selectedProvider = providers.find(provider => provider.id === form.watch('providerId'))
  const credentialScope = form.watch('scope')

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        onOpenChange(nextOpen)
        if (!nextOpen)
          form.reset(credentialDefaults)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('codeRepositoriesView.createCredential')}</DialogTitle>
          <DialogDescription>{t('codeRepositoriesView.credentialDialogDescription')}</DialogDescription>
        </DialogHeader>
        <form className="grid gap-3" onSubmit={form.handleSubmit(onSubmit)}>
          <Field error={form.formState.errors.providerId?.message} label={t('codeRepositoriesView.provider')} required>
            <Select {...form.register('providerId')} aria-invalid={Boolean(form.formState.errors.providerId)}>
              <option value="">{t('codeRepositoriesView.selectProvider')}</option>
              {providers.map(provider => (
                <option key={provider.id} value={provider.id}>{provider.name}</option>
              ))}
            </Select>
          </Field>
          {selectedProvider && <CredentialOAuthGuide provider={selectedProvider} />}
          <Field error={form.formState.errors.scope?.message} hint={t('codeRepositoriesView.scopeHint')} label={t('codeRepositoriesView.scope')} required>
            <Select {...form.register('scope')} aria-invalid={Boolean(form.formState.errors.scope)}>
              <option value="global">{t('codeRepositoriesView.scopeGlobal')}</option>
              <option value="project">{t('codeRepositoriesView.scopeProject')}</option>
              <option value="user">{t('codeRepositoriesView.scopeUser')}</option>
            </Select>
          </Field>
          {credentialScope === 'project' && (
            <Field error={form.formState.errors.projectIds?.message} hint={t('codeRepositoriesView.ownerProjectHint')} label={t('codeRepositoriesView.ownerProject')} required>
              <ProjectSpaceMultiSelect
                projects={projects}
                value={form.watch('projectIds')}
                onChange={value => form.setValue('projectIds', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.username?.message} hint={t('codeRepositoriesView.usernameHint')} label={t('codeRepositoriesView.username')} required><Input {...form.register('username')} aria-invalid={Boolean(form.formState.errors.username)} placeholder={t('codeRepositoriesView.usernamePlaceholder')} /></Field>
            <Field error={form.formState.errors.accessScope?.message} hint={t('codeRepositoriesView.accessScopeHint')} label={t('codeRepositoriesView.accessScope')} required>
              <Select {...form.register('accessScope')} aria-invalid={Boolean(form.formState.errors.accessScope)}>
                <option value="personal">{t('codeRepositoriesView.personalScope')}</option>
                <option value="provider">{t('codeRepositoriesView.providerScope')}</option>
              </Select>
            </Field>
          </div>
          <Field error={form.formState.errors.accessToken?.message} hint={t('codeRepositoriesView.accessTokenHint')} label={t('codeRepositoriesView.accessToken')}>
            <Input {...form.register('accessToken')} aria-invalid={Boolean(form.formState.errors.accessToken)} type="password" />
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field error={form.formState.errors.scopesText?.message} hint={t('codeRepositoriesView.scopesHint')} label={t('codeRepositoriesView.scopes')}>
              <Input {...form.register('scopesText')} aria-invalid={Boolean(form.formState.errors.scopesText)} />
            </Field>
            <Field error={form.formState.errors.status?.message} label={t('codeRepositoriesView.status')} required>
              <Select {...form.register('status')} aria-invalid={Boolean(form.formState.errors.status)}>
                <option value="connected">{t('common.connected')}</option>
                <option value="expired">{t('common.expired')}</option>
                <option value="revoked">{t('common.revoked')}</option>
              </Select>
            </Field>
          </div>
          <DialogFooter>
            <Button disabled={pending || !form.formState.isValid} type="submit">
              <Plus size={16} />
              {t('codeRepositoriesView.createCredential')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
