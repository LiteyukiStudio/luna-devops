import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { ChevronDown, Link2, Plus, Unlink } from 'lucide-react'
import { motion } from 'motion/react'
import { useEffect, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
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
import { FloatingFormSaveBar } from '@/components/common/floating-form-save-bar'
import { FormField as Field } from '@/components/common/form-field'
import { PageShell } from '@/components/common/page-shell'
import { Section } from '@/components/common/section'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Surface } from '@/components/common/surface'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { TabsContent } from '@/components/ui/tabs'
import { BrandColorPresetField } from '@/pages/settings/appearance/brand-color-preset-field'
import { AccessTokensPanel } from './access-tokens-panel'
import { AccountMcpPanel } from './account-mcp-panel'
import { AccountNotificationsPanel } from './account-notifications-panel'
import { OAuthApplicationsPanel, OAuthGrantsPanel } from './account-oauth-panels'
import { AccountPasswordPanel } from './account-password-panel'

const ACCOUNT_TAB_VALUES = [
  'profile',
  'security',
  'notifications',
  'tokens',
  'mcp',
  'oauth-applications',
  'oauth-grants',
] as const

type AccountTabValue = typeof ACCOUNT_TAB_VALUES[number]

function isAccountTabValue(value: string | null | undefined): value is AccountTabValue {
  return Boolean(value && (ACCOUNT_TAB_VALUES as readonly string[]).includes(value))
}

function resolveAccountTabFromLocation(): AccountTabValue {
  const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const hashTab = hashParams.get('tab')
  if (isAccountTabValue(hashTab))
    return hashTab

  const searchParams = new URLSearchParams(window.location.search)
  const queryTab = searchParams.get('tab')
  if (isAccountTabValue(queryTab))
    return queryTab

  return 'profile'
}

function writeAccountTabToLocation(nextTab: AccountTabValue) {
  const searchParams = new URLSearchParams(window.location.search)
  searchParams.delete('tab')
  const nextSearch = searchParams.toString()

  const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  hashParams.set('tab', nextTab)
  const nextHash = hashParams.toString()

  const nextUrl = `${window.location.pathname}${nextSearch ? `?${nextSearch}` : ''}#${nextHash}`
  window.history.replaceState(null, '', nextUrl)
}

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
  const [activeTab, setActiveTab] = useState<AccountTabValue>(() => resolveAccountTabFromLocation())
  const [createDialog, setCreateDialog] = useState<AccountCreateDialog>(null)

  useEffect(() => {
    const syncTabFromLocation = () => {
      const nextTab = resolveAccountTabFromLocation()
      setActiveTab(currentTab => (currentTab === nextTab ? currentTab : nextTab))
    }
    window.addEventListener('hashchange', syncTabFromLocation)
    window.addEventListener('popstate', syncTabFromLocation)
    return () => {
      window.removeEventListener('hashchange', syncTabFromLocation)
      window.removeEventListener('popstate', syncTabFromLocation)
    }
  }, [])

  const selectTab = (nextTab: string) => {
    if (!isAccountTabValue(nextTab))
      return
    setCreateDialog(null)
    setActiveTab(nextTab)
    writeAccountTabToLocation(nextTab)
  }

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
            onOpenMcpGuide={() => selectTab('mcp')}
          />
        )
      case 'mcp':
        return <AccountMcpPanel onNavigateTab={selectTab} />
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
          { value: 'mcp', label: t('accountPage.mcpTab') },
          { value: 'oauth-applications', label: t('oauthApps.applicationsTab') },
          { value: 'oauth-grants', label: t('oauthApps.grantsTab') },
        ]}
        tools={activeTools}
        value={activeTab}
        onValueChange={selectTab}
      >
        <TabsContent value={activeTab}>
          <motion.div
            key={activeTab}
            animate={{ opacity: 1, y: 0 }}
            className={
              activeTab === 'profile' || activeTab === 'security'
                ? 'w-full max-w-3xl'
                : activeTab === 'mcp'
                  ? 'w-full max-w-4xl'
                  : undefined
            }
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
    defaultValues: userToProfileForm(user),
  })

  useEffect(() => {
    if (!user)
      return
    form.reset(userToProfileForm(user))
  }, [form, user])

  const saveProfile = useMutation({
    mutationFn: (values: ProfileForm & { avatarUrl: string }) => updateProfile(values),
    onSuccess: (_result, values) => {
      form.reset({
        avatarUrl: values.avatarUrl ?? '',
        brandColorPreset: values.brandColorPreset ?? '',
        language: values.language,
        name: values.name,
      })
      toast.success(t('accountPage.profileSaved'))
    },
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
    <form className="grid gap-5" onSubmit={submitProfile}>
      <Surface className="grid gap-5 p-6" variant="bordered">
        <div className="flex flex-col gap-4 border-b border-border pb-5 @lg/workspace:flex-row @lg/workspace:items-center">
          <UserAvatar className="size-16 text-xl" user={previewUser} />
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold">{user?.name}</h2>
            <p className="truncate text-sm text-muted-foreground">{user?.email}</p>
          </div>
        </div>

        <div className="grid gap-4">
          <Field error={form.formState.errors.name?.message} hint={t('accountPage.profileNameHint')} label={t('accountPage.profileName')} required>
            <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('accountPage.profileNamePlaceholder')} />
          </Field>
          <Field error={form.formState.errors.avatarUrl?.message} hint={t('accountPage.avatarUrlHint')} label={t('accountPage.avatarUrl')}>
            <Input {...form.register('avatarUrl')} aria-invalid={Boolean(form.formState.errors.avatarUrl)} placeholder={t('accountPage.avatarUrlPlaceholder')} />
          </Field>
          <Field error={form.formState.errors.language?.message} hint={t('accountPage.languageHint')} label={t('language')} required>
            {fieldControl => (
              <Controller
                control={form.control}
                name="language"
                render={({ field }) => (
                  <Select name={field.name} value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger {...fieldControl} ref={field.ref} className="w-full" onBlur={field.onBlur}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="zh-CN">{t('languages.zhCN')}</SelectItem>
                      <SelectItem value="zh-TW">{t('languages.zhTW')}</SelectItem>
                      <SelectItem value="en-US">{t('languages.enUS')}</SelectItem>
                      <SelectItem value="ja-JP">{t('languages.jaJP')}</SelectItem>
                      <SelectItem value="ko-KR">{t('languages.koKR')}</SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
            )}
          </Field>
          <Field hint={t('accountPage.appearanceModeHint')} label={t('theme.mode')}>
            {fieldControl => (
              <Select
                value={mode}
                onValueChange={(next) => {
                  if (next === 'system' || next === 'light' || next === 'dark')
                    setMode(next)
                }}
              >
                <SelectTrigger {...fieldControl} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="system">{t('theme.system')}</SelectItem>
                  <SelectItem value="light">{t('theme.light')}</SelectItem>
                  <SelectItem value="dark">{t('theme.dark')}</SelectItem>
                </SelectContent>
              </Select>
            )}
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
        </div>
      </Surface>
      <FloatingFormSaveBar
        open={form.formState.isDirty}
        pending={saveProfile.isPending}
        saveDisabled={!form.formState.isValid}
        saveLabel={t('accountPage.saveProfile')}
        tone={form.formState.isValid ? 'primary' : 'danger'}
        onDiscard={() => form.reset(userToProfileForm(user))}
      />
    </form>
  )
}

function userToProfileForm(user: ReturnType<typeof useSession>['user']): ProfileForm {
  return {
    avatarUrl: user?.avatarUrl ?? '',
    brandColorPreset: (user?.brandColorPreset as ProfileForm['brandColorPreset'] | undefined) ?? '',
    language: (user?.language as ProfileForm['language'] | undefined) ?? 'zh-CN',
    name: user?.name ?? '',
  }
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
      tools={providers.isPending
        ? <Skeleton className="h-8 w-28 rounded-control" />
        : providerItems.length > 0 && (
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
      {identities.isPending
        ? (
            <div aria-label={t('common.loading')} className="divide-y divide-border" role="status">
              {Array.from({ length: 2 }, (_, index) => (
                <div key={index} className="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
                  <div className="grid min-w-0 flex-1 gap-1.5">
                    <div className="flex items-center gap-2">
                      <Skeleton className="h-4 w-28" />
                      <Skeleton className="h-5 w-16 rounded-control" />
                    </div>
                    <Skeleton className="h-3.5 w-44 max-w-full" />
                  </div>
                  <Skeleton className="size-9 shrink-0 rounded-control" />
                </div>
              ))}
            </div>
          )
        : identities.isError
          ? (
              <ErrorState
                description={t('settings.identityLoadFailedDescription')}
                retrying={identities.isFetching || providers.isFetching}
                title={t('settings.identityLoadFailedTitle')}
                onRetry={() => {
                  void identities.refetch()
                  if (providers.isError)
                    void providers.refetch()
                }}
              />
            )
          : identities.data.length === 0
            ? providers.isError
              ? (
                  <ErrorState
                    description={t('settings.providerLoadFailedDescription')}
                    retrying={providers.isFetching}
                    title={t('settings.providerLoadFailedTitle')}
                    onRetry={() => void providers.refetch()}
                  />
                )
              : <EmptyState description={t('settings.noIdentitiesDescription')} icon={<Link2 className="size-5" />} title={t('settings.noIdentitiesTitle')} variant="plain" />
            : (
                <div className="grid gap-4">
                  {providers.isError && (
                    <ErrorState
                      description={t('settings.providerLoadFailedDescription')}
                      retrying={providers.isFetching}
                      title={t('settings.providerLoadFailedTitle')}
                      onRetry={() => void providers.refetch()}
                    />
                  )}
                  <ul className="divide-y divide-border">
                    {identities.data.map(identity => (
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
                </div>
              )}
    </Section>
  )
}
