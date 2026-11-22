import type { AuthAdmissionPolicy, AuthProvider } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, KeyRound, Plus, Save } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, ApiError } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { FloatingFormSaveBar } from '@/components/common/floating-form-save-bar'
import { FormField as Field } from '@/components/common/form-field'
import { HelpTip } from '@/components/common/help-tip'
import { SettingsSkeleton } from '@/components/common/loading-states'
import { PageShell } from '@/components/common/page-shell'
import { ProgressiveSection } from '@/components/common/progressive-section'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { PLATFORM_ROLES, PlatformRole } from '@/lib/roles'

const providerSchema = z.object({
  name: z.string().min(1),
  enabled: z.boolean(),
  issuerUrl: z.string().url(),
  clientId: z.string().min(1),
  clientSecret: z.string(),
  scopes: z.string().min(1),
  groupClaim: z.string().min(1),
  emailClaim: z.string().min(1),
  usernameClaim: z.string().min(1),
  isDefault: z.boolean(),
})

const policySchema = z.object({
  allowLocalLogin: z.boolean(),
  allowOidcLogin: z.boolean(),
  requireVerifiedOidcEmail: z.boolean(),
  allowedEmailDomains: z.string(),
  allowedOidcGroups: z.string(),
  invitedEmails: z.string(),
  defaultRole: z.enum(PLATFORM_ROLES),
})

type ProviderForm = z.infer<typeof providerSchema>
type PolicyForm = z.infer<typeof policySchema>

const providerDefaults: ProviderForm = {
  name: '',
  enabled: true,
  issuerUrl: '',
  clientId: '',
  clientSecret: '',
  scopes: 'openid profile email',
  groupClaim: 'groups',
  emailClaim: 'email',
  usernameClaim: 'preferred_username',
  isDefault: false,
}

const policyDefaults: PolicyForm = {
  allowLocalLogin: true,
  allowOidcLogin: true,
  requireVerifiedOidcEmail: true,
  allowedEmailDomains: '',
  allowedOidcGroups: '',
  invitedEmails: '',
  defaultRole: PlatformRole.User,
}

export function AuthProvidersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editingProvider, setEditingProvider] = useState<AuthProvider | null>(null)
  const [providerDialogOpen, setProviderDialogOpen] = useState(false)
  const [activeTab, setActiveTab] = useState('providers')
  const providers = useQuery({ queryKey: ['auth-providers', 'admin'], queryFn: () => api.listAuthProviders(true) })
  const oidcCallbackConfig = useQuery({ queryKey: ['auth-oidc-callback-config'], queryFn: api.getOIDCCallbackConfig })
  const policy = useQuery({ queryKey: ['auth-admission-policy'], queryFn: api.getAuthAdmissionPolicy })
  const providerForm = useForm<ProviderForm>({ resolver: zodResolver(providerSchema), mode: 'onChange', defaultValues: providerDefaults })
  const policyForm = useForm<PolicyForm>({
    resolver: zodResolver(policySchema),
    mode: 'onChange',
    defaultValues: policyDefaults,
  })

  useEffect(() => {
    if (!editingProvider) {
      providerForm.reset(providerDefaults)
      return
    }
    providerForm.reset({
      name: editingProvider.name,
      enabled: editingProvider.enabled,
      issuerUrl: editingProvider.issuerUrl,
      clientId: editingProvider.clientId,
      clientSecret: '',
      scopes: editingProvider.scopes,
      groupClaim: editingProvider.groupClaim,
      emailClaim: editingProvider.emailClaim,
      usernameClaim: editingProvider.usernameClaim,
      isDefault: editingProvider.isDefault,
    })
  }, [editingProvider, providerForm])

  useEffect(() => {
    if (!policy.data)
      return
    policyForm.reset(policyToFormValues(policy.data))
  }, [policy.data, policyForm])

  const saveProvider = useMutation({
    mutationFn: (values: ProviderForm) => {
      const payload = { ...values, type: 'oidc' as const }
      if (editingProvider)
        return api.updateAuthProvider(editingProvider.id, payload)
      return api.createAuthProvider(payload)
    },
    onSuccess: () => {
      toast.success(editingProvider ? t('authProvidersPage.updated') : t('authProvidersPage.created'))
      setEditingProvider(null)
      setProviderDialogOpen(false)
      providerForm.reset(providerDefaults)
      queryClient.invalidateQueries({ queryKey: ['auth-providers'] })
    },
    onError: error => toast.error(error.message),
  })

  const savePolicy = useMutation({
    mutationFn: (values: PolicyForm) => api.updateAuthAdmissionPolicy({
      allowLocalLogin: values.allowLocalLogin,
      allowOidcLogin: values.allowOidcLogin,
      requireVerifiedOidcEmail: values.requireVerifiedOidcEmail,
      allowedEmailDomains: splitText(values.allowedEmailDomains),
      allowedOidcGroups: splitText(values.allowedOidcGroups),
      invitedEmails: splitText(values.invitedEmails),
      defaultRole: values.defaultRole,
    }),
    onSuccess: (result: AuthAdmissionPolicy) => {
      toast.success(t('authProvidersPage.policySaved'))
      queryClient.setQueryData(['auth-admission-policy'], result)
      policyForm.reset(policyToFormValues(result))
    },
    onError: error => toast.error(error.message),
  })

  const claimValues = providerForm.watch(['groupClaim', 'emailClaim', 'usernameClaim'])
  const copyOIDCCallbackURL = () => {
    const value = oidcCallbackConfig.data?.callbackUrl
    if (!value)
      return
    navigator.clipboard.writeText(value)
    toast.success(t('common.copied'))
  }
  const providerColumns = useMemo<DataListColumn<AuthProvider>[]>(() => [
    {
      key: 'name',
      header: t('common.name'),
      width: 'primary',
      render: provider => (
        <div className="min-w-0">
          <p className="truncate font-medium">{provider.name}</p>
          <p className="truncate text-sm text-muted-foreground">{provider.issuerUrl}</p>
        </div>
      ),
    },
    {
      key: 'groupClaim',
      header: t('authProvidersPage.groupClaim'),
      mobile: 'hidden',
      width: 'compact',
      render: provider => <span className="font-mono text-xs text-muted-foreground">{provider.groupClaim}</span>,
    },
    {
      key: 'scopes',
      header: t('authProvidersPage.scopes'),
      mobile: 'hidden',
      width: 'secondary',
      render: provider => <span className="font-mono text-xs text-muted-foreground">{provider.scopes}</span>,
    },
    {
      key: 'status',
      header: t('common.status'),
      width: 'status',
      render: provider => (
        <div className="flex flex-wrap items-center gap-2">
          {provider.isDefault && <StatusBadge>{t('common.default')}</StatusBadge>}
          <StatusValueBadge value={provider.enabled ? 'enabled' : 'disabled'} />
        </div>
      ),
    },
    {
      key: 'actions',
      header: t('common.actions'),
      className: 'text-right whitespace-nowrap',
      sticky: 'right',
      width: 'actions',
      render: provider => (
        <EditActionButton
          aria-label={t('edit')}
          type="button"
          label={t('edit')}
          onClick={() => {
            setEditingProvider(provider)
            setProviderDialogOpen(true)
          }}
        />
      ),
    },
  ], [t])

  return (
    <PageShell>
      <ContentTabs
        tabs={[
          { value: 'providers', label: t('authProvidersPage.providersTab') },
          { value: 'policy', label: t('authProvidersPage.policyTab') },
        ]}
        tools={activeTab === 'providers'
          ? (
              <Button
                onClick={() => {
                  setEditingProvider(null)
                  providerForm.reset(providerDefaults)
                  setProviderDialogOpen(true)
                }}
              >
                <Plus size={16} />
                {t('authProvidersPage.createTitle')}
              </Button>
            )
          : undefined}
        value={activeTab}
        onValueChange={setActiveTab}
      >
        <TabsContent value="providers">
          <DataList
            columns={providerColumns}
            emptyDescription={t('authProvidersPage.description')}
            emptyIcon={<KeyRound className="size-5" />}
            emptyTitle={t('authProvidersPage.emptyTitle')}
            error={providers.isError
              ? {
                  description: adminLoadErrorDescription(providers.error, t),
                  onRetry: () => void providers.refetch(),
                  retrying: providers.isFetching,
                  title: t('authProvidersPage.loadFailedTitle'),
                }
              : undefined}
            items={providers.data ?? []}
            loading={providers.isPending}
            rowKey={provider => provider.id}
          />
        </TabsContent>

        <Dialog
          open={providerDialogOpen}
          onOpenChange={(open) => {
            setProviderDialogOpen(open)
            if (!open) {
              setEditingProvider(null)
              providerForm.reset(providerDefaults)
            }
          }}
        >
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>{editingProvider ? t('authProvidersPage.editTitle') : t('authProvidersPage.createTitle')}</DialogTitle>
              <DialogDescription>{t('authProvidersPage.description')}</DialogDescription>
            </DialogHeader>
            <form className="grid gap-3" onSubmit={providerForm.handleSubmit(values => saveProvider.mutate(values))}>
              <Surface className="grid gap-2 p-3" variant="inset">
                <p className="flex items-center gap-1.5 text-sm font-medium">
                  {t('authProvidersPage.callbackUrl')}
                  <HelpTip label={t('authProvidersPage.callbackUrl')}>{t('authProvidersPage.callbackUrlHint')}</HelpTip>
                </p>
                {oidcCallbackConfig.isPending
                  ? (
                      <div aria-label={t('common.loading')} role="status">
                        <Skeleton className="h-8 w-full rounded-control" />
                      </div>
                    )
                  : oidcCallbackConfig.isError
                    ? (
                        <ErrorState
                          retrying={oidcCallbackConfig.isFetching}
                          title={t('errors.request.failed')}
                          onRetry={() => void oidcCallbackConfig.refetch()}
                        />
                      )
                    : oidcCallbackConfig.data.configured
                      ? (
                          <div className="flex min-w-0 items-center gap-2">
                            <code className="min-w-0 flex-1 truncate rounded-control border border-border bg-surface-raised px-2 py-1.5 font-mono text-xs text-muted-foreground">
                              {oidcCallbackConfig.data.callbackUrl}
                            </code>
                            <Button aria-label={t('authProvidersPage.copyCallbackUrl')} size="icon" type="button" variant="outline" onClick={copyOIDCCallbackURL}>
                              <Copy size={14} />
                            </Button>
                          </div>
                        )
                      : <p className="text-sm text-danger">{t('authProvidersPage.callbackUrlMissing')}</p>}
              </Surface>
              <Field error={providerForm.formState.errors.name?.message} hint={t('authProvidersPage.nameHint')} label={t('authProvidersPage.name')} required>
                <Input {...providerForm.register('name')} aria-invalid={Boolean(providerForm.formState.errors.name)} placeholder={t('authProvidersPage.namePlaceholder')} />
              </Field>
              <Field error={providerForm.formState.errors.issuerUrl?.message} hint={t('authProvidersPage.issuerUrlHint')} label={t('authProvidersPage.issuerUrl')} required>
                <Input {...providerForm.register('issuerUrl')} aria-invalid={Boolean(providerForm.formState.errors.issuerUrl)} placeholder={t('authProvidersPage.issuerUrlPlaceholder')} />
              </Field>
              <Field error={providerForm.formState.errors.clientId?.message} hint={t('authProvidersPage.clientIdHint')} label={t('authProvidersPage.clientId')} required>
                <Input {...providerForm.register('clientId')} aria-invalid={Boolean(providerForm.formState.errors.clientId)} />
              </Field>
              <Field error={providerForm.formState.errors.clientSecret?.message} hint={t('authProvidersPage.clientSecretHint')} label={t('authProvidersPage.clientSecret')}>
                <Input
                  {...providerForm.register('clientSecret')}
                  aria-invalid={Boolean(providerForm.formState.errors.clientSecret)}
                  autoComplete="off"
                  placeholder={editingProvider?.clientSecretSet ? t('common.secretSetPlaceholder') : t('authProvidersPage.secretPlaceholder')}
                  type="password"
                />
              </Field>
              <Field error={providerForm.formState.errors.scopes?.message} hint={t('authProvidersPage.scopesHint')} label={t('authProvidersPage.scopes')} required>
                <Input {...providerForm.register('scopes')} aria-invalid={Boolean(providerForm.formState.errors.scopes)} />
              </Field>
              <ProgressiveSection
                summary={claimValues.join(' · ')}
                title={t('authProvidersPage.claimMapping')}
              >
                <div className="grid gap-3 md:grid-cols-3">
                  <Field error={providerForm.formState.errors.groupClaim?.message} hint={t('authProvidersPage.groupClaimHint')} label={t('authProvidersPage.groupClaim')} required>
                    <Input {...providerForm.register('groupClaim')} aria-invalid={Boolean(providerForm.formState.errors.groupClaim)} />
                  </Field>
                  <Field error={providerForm.formState.errors.emailClaim?.message} hint={t('authProvidersPage.emailClaimHint')} label={t('authProvidersPage.emailClaim')} required>
                    <Input {...providerForm.register('emailClaim')} aria-invalid={Boolean(providerForm.formState.errors.emailClaim)} />
                  </Field>
                  <Field error={providerForm.formState.errors.usernameClaim?.message} hint={t('authProvidersPage.usernameClaimHint')} label={t('authProvidersPage.usernameClaim')} required>
                    <Input {...providerForm.register('usernameClaim')} aria-invalid={Boolean(providerForm.formState.errors.usernameClaim)} />
                  </Field>
                </div>
              </ProgressiveSection>
              <div className="flex flex-wrap gap-x-6 gap-y-3">
                <Controller
                  control={providerForm.control}
                  name="enabled"
                  render={({ field }) => (
                    <ControlledCheckboxField field={field}>
                      {t('authProvidersPage.enabled')}
                    </ControlledCheckboxField>
                  )}
                />
                <Controller
                  control={providerForm.control}
                  name="isDefault"
                  render={({ field }) => (
                    <ControlledCheckboxField field={field}>
                      {t('authProvidersPage.defaultProvider')}
                    </ControlledCheckboxField>
                  )}
                />
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setProviderDialogOpen(false)}>{t('common.cancel')}</Button>
                <Button disabled={saveProvider.isPending || !providerForm.formState.isValid} type="submit">
                  <Save size={16} />
                  {editingProvider ? t('authProvidersPage.save') : t('authProvidersPage.create')}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        <TabsContent value="policy">
          {policy.isPending
            ? <SettingsSkeleton />
            : policy.isError
              ? (
                  <ErrorState
                    description={adminLoadErrorDescription(policy.error, t)}
                    retrying={policy.isFetching}
                    title={t('authProvidersPage.policyLoadFailedTitle')}
                    onRetry={() => void policy.refetch()}
                  />
                )
              : (
                  <form className="grid max-w-3xl gap-5" onSubmit={policyForm.handleSubmit(values => savePolicy.mutate(values))}>
                    <Surface className="grid gap-5 p-6" variant="bordered">
                      <div className="grid gap-3 @xl/workspace:grid-cols-2">
                        <Controller
                          control={policyForm.control}
                          name="allowLocalLogin"
                          render={({ field }) => (
                            <ControlledCheckboxField field={field}>
                              {t('authProvidersPage.allowLocalLogin')}
                            </ControlledCheckboxField>
                          )}
                        />
                        <Controller
                          control={policyForm.control}
                          name="allowOidcLogin"
                          render={({ field }) => (
                            <ControlledCheckboxField field={field}>
                              {t('authProvidersPage.allowOidcLogin')}
                            </ControlledCheckboxField>
                          )}
                        />
                        <Controller
                          control={policyForm.control}
                          name="requireVerifiedOidcEmail"
                          render={({ field }) => (
                            <ControlledCheckboxField
                              className="@xl/workspace:col-span-2"
                              description={t('authProvidersPage.requireVerifiedOidcEmailHint')}
                              field={field}
                            >
                              {t('authProvidersPage.requireVerifiedOidcEmail')}
                            </ControlledCheckboxField>
                          )}
                        />
                      </div>
                      <div className="grid gap-4 @xl/workspace:grid-cols-2">
                        <Field error={policyForm.formState.errors.allowedEmailDomains?.message} hint={t('authProvidersPage.allowedEmailDomainsHint')} label={t('authProvidersPage.allowedEmailDomains')}>
                          <Textarea {...policyForm.register('allowedEmailDomains')} aria-invalid={Boolean(policyForm.formState.errors.allowedEmailDomains)} placeholder={t('authProvidersPage.allowedEmailDomainsPlaceholder')} rows={2} />
                        </Field>
                        <Field error={policyForm.formState.errors.allowedOidcGroups?.message} hint={t('authProvidersPage.allowedOidcGroupsHint')} label={t('authProvidersPage.allowedOidcGroups')}>
                          <Textarea {...policyForm.register('allowedOidcGroups')} aria-invalid={Boolean(policyForm.formState.errors.allowedOidcGroups)} placeholder={t('authProvidersPage.allowedOidcGroupsPlaceholder')} rows={2} />
                        </Field>
                      </div>
                      <Field error={policyForm.formState.errors.invitedEmails?.message} hint={t('authProvidersPage.invitedEmailsHint')} label={t('authProvidersPage.invitedEmails')}>
                        <Textarea {...policyForm.register('invitedEmails')} aria-invalid={Boolean(policyForm.formState.errors.invitedEmails)} placeholder={t('authProvidersPage.invitedEmailsPlaceholder')} rows={2} />
                      </Field>
                      <Field error={policyForm.formState.errors.defaultRole?.message} hint={t('authProvidersPage.defaultRoleHint')} label={t('authProvidersPage.defaultRole')} required>
                        {fieldControl => (
                          <Controller
                            control={policyForm.control}
                            name="defaultRole"
                            render={({ field }) => (
                              <Select name={field.name} value={field.value} onValueChange={field.onChange}>
                                <SelectTrigger {...fieldControl} ref={field.ref} className="w-full" onBlur={field.onBlur}>
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  <SelectItem value={PlatformRole.User}>{t('usersPage.normalUser')}</SelectItem>
                                  <SelectItem value={PlatformRole.Admin}>{t('usersPage.platformAdmin')}</SelectItem>
                                </SelectContent>
                              </Select>
                            )}
                          />
                        )}
                      </Field>
                    </Surface>
                    <FloatingFormSaveBar
                      open={policyForm.formState.isDirty}
                      pending={savePolicy.isPending}
                      saveDisabled={!policyForm.formState.isValid}
                      saveLabel={t('authProvidersPage.savePolicy')}
                      tone={policyForm.formState.isValid ? 'primary' : 'danger'}
                      onDiscard={() => policyForm.reset(policy.data ? policyToFormValues(policy.data) : policyDefaults)}
                    />
                  </form>
                )}
        </TabsContent>
      </ContentTabs>
    </PageShell>
  )
}

function policyToFormValues(policy: AuthAdmissionPolicy): PolicyForm {
  return {
    allowLocalLogin: policy.allowLocalLogin,
    allowOidcLogin: policy.allowOidcLogin,
    requireVerifiedOidcEmail: policy.requireVerifiedOidcEmail,
    allowedEmailDomains: (policy.allowedEmailDomains ?? []).join(', '),
    allowedOidcGroups: (policy.allowedOidcGroups ?? []).join(', '),
    invitedEmails: (policy.invitedEmails ?? []).join(', '),
    defaultRole: policy.defaultRole,
  }
}

function splitText(value: string) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}

function adminLoadErrorDescription(error: unknown, t: (key: string) => string) {
  if (error instanceof ApiError && error.status === 403)
    return t('common.platformAdminPermissionRequired')
  return error instanceof Error ? error.message : undefined
}
