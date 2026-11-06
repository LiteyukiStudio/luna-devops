import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { ChevronDown, Link2, Plus, Unlink } from 'lucide-react'
import { motion } from 'motion/react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, oidcStartUrl } from '@/api'
import { brandColorPresets } from '@/app/brand-theme'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { useTheme } from '@/app/theme-context'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { PageChromeTools } from '@/components/common/page-chrome'
import { PageShell } from '@/components/common/page-shell'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { BrandColorPresetField } from '@/pages/settings/appearance/brand-color-preset-field'
import { SettingsTabSaveButton } from '@/pages/settings/settings-tab-save-button'
import { AccessTokensPanel } from './access-tokens-panel'
import { AccountNotificationsPanel } from './account-notifications-panel'
import { OAuthApplicationsPanel, OAuthGrantsPanel } from './account-oauth-panels'
import { AccountPasswordPanel } from './account-password-panel'

const profileSchema = z.object({
  name: z.string().min(1, { error: () => i18next.t('accountPage.profileNameRequired') }),
  avatarUrl: z.string().optional(),
  language: z.enum(['zh-CN', 'zh-TW', 'en-US', 'ja-JP', 'ko-KR']),
  brandColorPreset: z.union([z.literal(''), z.enum(brandColorPresets)]),
})

type ProfileForm = z.infer<typeof profileSchema>
type AccountCreateDialog = 'access-token' | 'oauth-application' | null

export function AccountPage() {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState('profile')
  const [createDialog, setCreateDialog] = useState<AccountCreateDialog>(null)

  const activeContent = (() => {
    switch (activeTab) {
      case 'security':
        return (
          <div className="grid gap-4">
            <AccountPasswordPanel />
            <IdentityBindingsPanel />
          </div>
        )
      case 'tokens':
        return (
          <AccessTokensPanel
            createDialogOpen={createDialog === 'access-token'}
            onCreateDialogOpenChange={open => setCreateDialog(open ? 'access-token' : null)}
          />
        )
      case 'notifications':
        return <AccountNotificationsPanel />
      case 'oauth-applications':
        return (
          <OAuthApplicationsPanel
            createDialogOpen={createDialog === 'oauth-application'}
            onCreateDialogOpenChange={open => setCreateDialog(open ? 'oauth-application' : null)}
          />
        )
      case 'oauth-grants':
        return <OAuthGrantsPanel />
      default:
        return <ProfilePanel />
    }
  })()
  const activeTools = activeTab === 'tokens'
    ? (
        <Button type="button" onClick={() => setCreateDialog('access-token')}>
          <Plus size={16} />
          {t('accessTokens.createTitle')}
        </Button>
      )
    : activeTab === 'oauth-applications'
      ? (
          <Button type="button" onClick={() => setCreateDialog('oauth-application')}>
            <Plus size={16} />
            {t('oauthApps.createApplication')}
          </Button>
        )
      : null

  return (
    <PageShell spacing="compact" width="settings">
      <ContentTabs
        headerClassName="flex justify-start"
        tabs={[
          { value: 'profile', label: t('accountPage.profileTab') },
          { value: 'security', label: t('accountPage.securityTab') },
          { value: 'notifications', label: t('accountPage.notificationsTab') },
          { value: 'tokens', label: t('accountPage.tokensTab') },
          { value: 'oauth-applications', label: t('oauthApps.applicationsTab') },
          { value: 'oauth-grants', label: t('oauthApps.grantsTab') },
        ]}
        tools={activeTools}
        value={activeTab}
        onValueChange={(nextTab) => {
          setCreateDialog(null)
          setActiveTab(nextTab)
        }}
      >
        <TabsContent value={activeTab}>
          <motion.div
            key={activeTab}
            animate={{ opacity: 1, y: 0 }}
            className={activeTab === 'profile' || activeTab === 'security' ? 'w-full max-w-3xl' : undefined}
            initial={{ opacity: 0, y: 6 }}
            transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
          >
            {activeContent}
          </motion.div>
        </TabsContent>
      </ContentTabs>
    </PageShell>
  )
}

function ProfilePanel() {
  const { t } = useTranslation()
  const { mode, setMode } = useTheme()
  const { updateProfile, user } = useSession()
  const configs = usePublicConfig()
  const form = useForm<ProfileForm>({
    resolver: zodResolver(profileSchema),
    mode: 'onChange',
    defaultValues: {
      avatarUrl: user?.avatarUrl ?? '',
      brandColorPreset: user?.brandColorPreset ?? '',
      language: user?.language ?? 'zh-CN',
      name: user?.name ?? '',
    },
  })

  useEffect(() => {
    if (!user)
      return
    form.reset({
      avatarUrl: user.avatarUrl ?? '',
      brandColorPreset: user.brandColorPreset ?? '',
      language: user.language,
      name: user.name,
    })
  }, [form, user])

  const saveProfile = useMutation({
    mutationFn: updateProfile,
    onSuccess: () => toast.success(t('accountPage.profileSaved')),
    onError: error => toast.error(error.message),
  })

  const avatarUrl = form.watch('avatarUrl')
  const name = form.watch('name')
  const previewUser = {
    avatarUrl: avatarUrl ?? '',
    email: user?.email ?? '',
    name: name || user?.name || '',
  }

  const submitProfile = form.handleSubmit(values => saveProfile.mutate({ ...values, avatarUrl: values.avatarUrl ?? '' }))

  return (
    <Surface className="grid gap-5 p-6" variant="bordered">
      <PageChromeTools>
        <SettingsTabSaveButton
          disabled={!form.formState.isValid || !form.formState.isDirty}
          label={t('accountPage.saveProfile')}
          pending={saveProfile.isPending}
          type="button"
          onClick={() => void submitProfile()}
        />
      </PageChromeTools>
      <div className="flex flex-col gap-4 border-b border-border pb-5 @lg/workspace:flex-row @lg/workspace:items-center">
        <UserAvatar className="size-16 text-xl" user={previewUser} />
        <div className="min-w-0">
          <h2 className="truncate text-base font-semibold">{user?.name}</h2>
          <p className="truncate text-sm text-muted-foreground">{user?.email}</p>
        </div>
      </div>

      <form className="grid gap-4" onSubmit={submitProfile}>
        <Field error={form.formState.errors.name?.message} hint={t('accountPage.profileNameHint')} label={t('accountPage.profileName')} required>
          <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('accountPage.profileNamePlaceholder')} />
        </Field>
        <Field error={form.formState.errors.avatarUrl?.message} hint={t('accountPage.avatarUrlHint')} label={t('accountPage.avatarUrl')}>
          <Input {...form.register('avatarUrl')} aria-invalid={Boolean(form.formState.errors.avatarUrl)} placeholder={t('accountPage.avatarUrlPlaceholder')} />
        </Field>
        <Field error={form.formState.errors.language?.message} hint={t('accountPage.languageHint')} label={t('language')} required>
          <Select {...form.register('language')} aria-invalid={Boolean(form.formState.errors.language)}>
            <option value="zh-CN">{t('languages.zhCN')}</option>
            <option value="zh-TW">{t('languages.zhTW')}</option>
            <option value="en-US">{t('languages.enUS')}</option>
            <option value="ja-JP">{t('languages.jaJP')}</option>
            <option value="ko-KR">{t('languages.koKR')}</option>
          </Select>
        </Field>
        <Field hint={t('accountPage.appearanceModeHint')} label={t('theme.mode')}>
          <Select value={mode} onChange={event => setMode(event.target.value as typeof mode)}>
            <option value="system">{t('theme.system')}</option>
            <option value="light">{t('theme.light')}</option>
            <option value="dark">{t('theme.dark')}</option>
          </Select>
        </Field>
        <Field error={form.formState.errors.brandColorPreset?.message} hint={t('accountPage.brandColorHint')} label={t('accountPage.brandColor')}>
          <BrandColorPresetField
            ariaLabel={t('accountPage.brandColor')}
            inheritedPreset={configs['site.brandColorPreset']}
            inheritLabel={t('accountPage.followPlatformBrandColor')}
            value={form.watch('brandColorPreset')}
            onValueChange={nextValue => form.setValue('brandColorPreset', nextValue, { shouldDirty: true, shouldValidate: true })}
          />
        </Field>
      </form>
    </Surface>
  )
}

function IdentityBindingsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const providers = useQuery({ queryKey: ['auth-providers'], queryFn: () => api.listAuthProviders(false) })
  const identities = useQuery({ queryKey: ['external-identities'], queryFn: api.listMyExternalIdentities })
  const unbind = useMutation({
    mutationFn: api.unbindMyExternalIdentity,
    onSuccess: () => {
      toast.success(t('settings.identityUnbound'))
      queryClient.invalidateQueries({ queryKey: ['external-identities'] })
    },
    onError: error => toast.error(error.message),
  })

  const providerItems = providers.data ?? []

  return (
    <Section
      title={t('settings.boundIdentities')}
      tools={providerItems.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" type="button" variant="outline">
              <Link2 className="size-4" />
              {t('settings.bindProviderTitle')}
              <ChevronDown className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {providerItems.map(provider => (
              <DropdownMenuItem
                key={provider.id}
                onSelect={() => {
                  window.location.href = oidcStartUrl(provider.id, 'bind', '/settings/account')
                }}
              >
                {t('settings.bindProvider', { provider: provider.name })}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      variant="bordered"
    >
      {providers.isError && <ErrorState title={t('settings.providerLoadFailedTitle')} description={t('settings.providerLoadFailedDescription')} />}
      {identities.isError
        ? <ErrorState title={t('settings.identityLoadFailedTitle')} description={t('settings.identityLoadFailedDescription')} />
        : identities.data?.length === 0
          ? <EmptyState description={t('settings.noIdentitiesDescription')} icon={<Link2 className="size-5" />} title={t('settings.noIdentitiesTitle')} variant="plain" />
          : (
              <ul className="divide-y divide-border">
                {(identities.data ?? []).map(identity => (
                  <li key={identity.id} className="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="truncate font-medium">{identity.providerName}</p>
                        <StatusValueBadge value={identity.emailVerified ? 'verified' : 'unverified'} />
                      </div>
                      <p className="truncate text-sm text-muted-foreground">{identity.email || identity.username || identity.subject}</p>
                    </div>
                    <ConfirmDialog
                      confirmText={t('settings.unbindIdentity')}
                      description={t('settings.unbindIdentityDescription', { provider: identity.providerName })}
                      pending={unbind.isPending}
                      title={t('settings.unbindIdentity')}
                      onConfirm={() => unbind.mutateAsync(identity.id).then(() => undefined)}
                    >
                      <Button aria-label={t('settings.unbindIdentity')} size="icon" variant="ghost">
                        <Unlink className="size-4" />
                      </Button>
                    </ConfirmDialog>
                  </li>
                ))}
              </ul>
            )}
    </Section>
  )
}
