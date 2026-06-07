import type { AuthAdmissionPolicy, AuthProvider } from '@/api/client'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Save, ShieldCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api/client'
import { ContentTabs } from '@/components/common/content-tabs'
import { EditActionButton } from '@/components/common/edit-action-button'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { MotionItem, MotionList } from '@/components/common/motion'
import { StatusBadge } from '@/components/common/status-badge'
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
  const policy = useQuery({ queryKey: ['auth-admission-policy'], queryFn: api.getAuthAdmissionPolicy })
  const providerForm = useForm<ProviderForm>({ resolver: zodResolver(providerSchema), mode: 'onChange', defaultValues: providerDefaults })
  const policyForm = useForm<PolicyForm>({
    resolver: zodResolver(policySchema),
    mode: 'onChange',
    defaultValues: {
      allowLocalLogin: true,
      allowOidcLogin: true,
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
          <Card>
            {providers.isError && <ErrorState title={t('authProvidersPage.loadFailedTitle')} description={t('common.platformAdminPermissionRequired')} />}
            <MotionList className="grid gap-3">
              {(providers.data ?? []).map(provider => (
                <MotionItem key={provider.id}>
                  <div className="grid gap-2 rounded-md border border-border bg-background p-3 transition duration-150 hover:border-primary hover:shadow-sm">
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        <p className="font-medium">{provider.name}</p>
                        <p className="truncate text-sm text-muted-foreground">{provider.issuerUrl}</p>
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        {provider.isDefault && <StatusBadge>{t('common.default')}</StatusBadge>}
                        <StatusBadge>{provider.enabled ? t('common.enabled') : t('common.disabled')}</StatusBadge>
                        <EditActionButton
                          aria-label={t('edit')}
                          type="button"
                          label={t('edit')}
                          onClick={() => {
                            setEditingProvider(provider)
                            setProviderDialogOpen(true)
                          }}
                        />
                      </div>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {provider.groupClaim}
                      {' '}
                      /
                      {' '}
                      {provider.scopes}
                    </p>
                  </div>
                </MotionItem>
              ))}
            </MotionList>
          </Card>
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
              <div className="grid grid-cols-3 gap-3">
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
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" {...providerForm.register('enabled')} />
                {t('authProvidersPage.enabled')}
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" {...providerForm.register('isDefault')} />
                {t('authProvidersPage.defaultProvider')}
              </label>
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
                <label className="flex items-center gap-2 text-sm">
                  <input type="checkbox" {...policyForm.register('allowLocalLogin')} />
                  {t('authProvidersPage.allowLocalLogin')}
                </label>
                <label className="flex items-center gap-2 text-sm">
                  <input type="checkbox" {...policyForm.register('allowOidcLogin')} />
                  {t('authProvidersPage.allowOidcLogin')}
                </label>
              </div>
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
