import type { AuthAdmissionPolicy, AuthProvider } from '@/api'
import type { DataListColumn } from '@/components/common/data-list'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Plus, Save, ShieldCheck } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ContentTabs } from '@/components/common/content-tabs'
import { DataList } from '@/components/common/data-list'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

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
  defaultRole: z.enum(['platform_admin', 'user']),
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
    defaultValues: {
      allowLocalLogin: true,
      allowOidcLogin: true,
      requireVerifiedOidcEmail: true,
      allowedEmailDomains: '',
      allowedOidcGroups: '',
      invitedEmails: '',
      defaultRole: 'user',
    },
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
    policyForm.reset({
      allowLocalLogin: policy.data.allowLocalLogin,
      allowOidcLogin: policy.data.allowOidcLogin,
      requireVerifiedOidcEmail: policy.data.requireVerifiedOidcEmail,
      allowedEmailDomains: (policy.data.allowedEmailDomains ?? []).join(', '),
      allowedOidcGroups: (policy.data.allowedOidcGroups ?? []).join(', '),
      invitedEmails: (policy.data.invitedEmails ?? []).join(', '),
      defaultRole: policy.data.defaultRole,
    })
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
    },
    onError: error => toast.error(error.message),
  })

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
      className: 'min-w-64',
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
      className: 'min-w-40',
      render: provider => <span className="font-mono text-xs text-muted-foreground">{provider.groupClaim}</span>,
    },
    {
      key: 'scopes',
      header: t('authProvidersPage.scopes'),
      className: 'min-w-56',
      render: provider => <span className="font-mono text-xs text-muted-foreground">{provider.scopes}</span>,
    },
    {
      key: 'status',
      header: t('common.status'),
      className: 'w-52',
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
      className: 'w-32 whitespace-nowrap text-right',
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
    <div className="grid gap-6">
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
          {providers.isError
            ? <Card className="p-4"><ErrorState title={t('authProvidersPage.loadFailedTitle')} description={t('common.platformAdminPermissionRequired')} /></Card>
            : (
                <DataList
                  columns={providerColumns}
                  emptyTitle={t('authProvidersPage.providersTab')}
                  emptyDescription={t('authProvidersPage.description')}
                  items={providers.data ?? []}
                  rowKey={provider => provider.id}
                />
              )}
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
              <div className="grid gap-2 rounded-md border border-border bg-muted/40 p-3">
                <p className="text-sm font-medium">{t('authProvidersPage.callbackUrl')}</p>
                {oidcCallbackConfig.data?.configured
                  ? (
                      <div className="flex min-w-0 items-center gap-2">
                        <code className="min-w-0 flex-1 truncate rounded bg-background px-2 py-1.5 text-xs text-muted-foreground">
                          {oidcCallbackConfig.data.callbackUrl}
                        </code>
                        <Button aria-label={t('authProvidersPage.copyCallbackUrl')} size="icon" type="button" variant="outline" onClick={copyOIDCCallbackURL}>
                          <Copy size={14} />
                        </Button>
                      </div>
                    )
                  : <p className="text-sm text-danger">{t('authProvidersPage.callbackUrlMissing')}</p>}
                <p className="text-xs text-muted-foreground">{t('authProvidersPage.callbackUrlHint')}</p>
              </div>
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
                  placeholder={editingProvider?.clientSecretSet ? t('authProvidersPage.secretSetPlaceholder') : t('authProvidersPage.secretPlaceholder')}
                  type="password"
                />
              </Field>
              <Field error={providerForm.formState.errors.scopes?.message} hint={t('authProvidersPage.scopesHint')} label={t('authProvidersPage.scopes')} required>
                <Input {...providerForm.register('scopes')} aria-invalid={Boolean(providerForm.formState.errors.scopes)} />
              </Field>
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
              <CheckboxField {...providerForm.register('enabled')}>
                {t('authProvidersPage.enabled')}
              </CheckboxField>
              <CheckboxField {...providerForm.register('isDefault')}>
                {t('authProvidersPage.defaultProvider')}
              </CheckboxField>
              <DialogFooter>
                <Button disabled={saveProvider.isPending || !providerForm.formState.isValid} type="submit">
                  <Save size={16} />
                  {editingProvider ? t('authProvidersPage.save') : t('authProvidersPage.create')}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        <TabsContent value="policy">
          <Card>
            <form className="grid gap-3" onSubmit={policyForm.handleSubmit(values => savePolicy.mutate(values))}>
              <h2 className="text-base font-semibold">{t('authProvidersPage.admissionPolicy')}</h2>
              {policy.isError && <ErrorState title={t('authProvidersPage.policyLoadFailedTitle')} description={t('common.platformAdminPermissionRequired')} />}
              <div className="grid gap-3 md:grid-cols-2">
                <CheckboxField {...policyForm.register('allowLocalLogin')}>
                  {t('authProvidersPage.allowLocalLogin')}
                </CheckboxField>
                <CheckboxField {...policyForm.register('allowOidcLogin')}>
                  {t('authProvidersPage.allowOidcLogin')}
                </CheckboxField>
                <CheckboxField {...policyForm.register('requireVerifiedOidcEmail')}>
                  {t('authProvidersPage.requireVerifiedOidcEmail')}
                </CheckboxField>
              </div>
              <p className="text-sm text-muted-foreground">{t('authProvidersPage.requireVerifiedOidcEmailHint')}</p>
              <Field error={policyForm.formState.errors.allowedEmailDomains?.message} hint={t('authProvidersPage.allowedEmailDomainsHint')} label={t('authProvidersPage.allowedEmailDomains')}>
                <Textarea {...policyForm.register('allowedEmailDomains')} aria-invalid={Boolean(policyForm.formState.errors.allowedEmailDomains)} placeholder={t('authProvidersPage.allowedEmailDomainsPlaceholder')} />
              </Field>
              <Field error={policyForm.formState.errors.allowedOidcGroups?.message} hint={t('authProvidersPage.allowedOidcGroupsHint')} label={t('authProvidersPage.allowedOidcGroups')}>
                <Textarea {...policyForm.register('allowedOidcGroups')} aria-invalid={Boolean(policyForm.formState.errors.allowedOidcGroups)} placeholder={t('authProvidersPage.allowedOidcGroupsPlaceholder')} />
              </Field>
              <Field error={policyForm.formState.errors.invitedEmails?.message} hint={t('authProvidersPage.invitedEmailsHint')} label={t('authProvidersPage.invitedEmails')}>
                <Textarea {...policyForm.register('invitedEmails')} aria-invalid={Boolean(policyForm.formState.errors.invitedEmails)} placeholder={t('authProvidersPage.invitedEmailsPlaceholder')} />
              </Field>
              <Field error={policyForm.formState.errors.defaultRole?.message} hint={t('authProvidersPage.defaultRoleHint')} label={t('authProvidersPage.defaultRole')} required>
                <Select {...policyForm.register('defaultRole')} aria-invalid={Boolean(policyForm.formState.errors.defaultRole)}>
                  <option value="user">{t('usersPage.normalUser')}</option>
                  <option value="platform_admin">{t('usersPage.platformAdmin')}</option>
                </Select>
              </Field>
              <Button disabled={savePolicy.isPending || !policyForm.formState.isValid} type="submit">
                <ShieldCheck size={16} />
                {t('authProvidersPage.savePolicy')}
              </Button>
            </form>
          </Card>
        </TabsContent>
      </ContentTabs>
    </div>
  )
}

function splitText(value: string) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}
