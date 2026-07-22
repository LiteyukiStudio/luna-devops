import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Link2, Save, Unlink } from 'lucide-react'
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
import { ContentTabs } from '@/components/common/content-tabs'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { MotionItem, MotionList } from '@/components/common/motion'
import { StatusValueBadge } from '@/components/common/status-badge'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { AccessTokensPanel } from '@/pages/access-tokens/AccessTokensPage'
import { AccountMFAPanel } from './account-mfa-panel'
import { OAuthApplicationsPanel, OAuthGrantsPanel } from './account-oauth-panels'
import { AccountPasswordPanel } from './account-password-panel'
import { BrandColorPresetField } from './brand-color-preset-field'

const profileSchema = z.object({
  name: z.string().min(1, i18next.t('accountPage.profileNameRequired')),
  avatarUrl: z.string().optional(),
  language: z.enum(['zh-CN', 'en-US']),
  brandColorPreset: z.union([z.literal(''), z.enum(brandColorPresets)]),
})

type ProfileForm = z.infer<typeof profileSchema>

export function AccountPage() {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState('profile')

  const activeContent = (() => {
    switch (activeTab) {
      case 'security':
        return (
          <div className="grid gap-4">
            <AccountPasswordPanel />
            <AccountMFAPanel />
            <IdentityBindingsPanel />
          </div>
        )
      case 'tokens':
        return <AccessTokensPanel />
      case 'oauth-applications':
        return <OAuthApplicationsPanel />
      case 'oauth-grants':
        return <OAuthGrantsPanel />
      default:
        return <ProfilePanel />
    }
  })()

  return (
    <ContentTabs
      tabs={[
        { value: 'profile', label: t('accountPage.profileTab') },
        { value: 'security', label: t('accountPage.securityTab') },
        { value: 'tokens', label: t('accountPage.tokensTab') },
        { value: 'oauth-applications', label: t('oauthApps.applicationsTab') },
        { value: 'oauth-grants', label: t('oauthApps.grantsTab') },
      ]}
      value={activeTab}
      onValueChange={setActiveTab}
    >
      <TabsContent value={activeTab}>
        <motion.div
          key={activeTab}
          animate={{ opacity: 1, y: 0 }}
          initial={{ opacity: 0, y: 6 }}
          transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
        >
          {activeContent}
        </motion.div>
      </TabsContent>
    </ContentTabs>
  )
}

function ProfilePanel() {
  const { t } = useTranslation()
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

  return (
    <Card className="grid gap-5">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <UserAvatar className="size-16 text-xl" user={previewUser} />
        <div className="min-w-0">
          <h2 className="truncate text-base font-semibold">{user?.name}</h2>
          <p className="truncate text-sm text-muted-foreground">{user?.email}</p>
        </div>
      </div>

      <form className="grid gap-3" onSubmit={form.handleSubmit(values => saveProfile.mutate({ ...values, avatarUrl: values.avatarUrl ?? '' }))}>
        <Field error={form.formState.errors.name?.message} hint={t('accountPage.profileNameHint')} label={t('accountPage.profileName')} required>
          <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} placeholder={t('accountPage.profileNamePlaceholder')} />
        </Field>
        <Field error={form.formState.errors.avatarUrl?.message} hint={t('accountPage.avatarUrlHint')} label={t('accountPage.avatarUrl')}>
          <Input {...form.register('avatarUrl')} aria-invalid={Boolean(form.formState.errors.avatarUrl)} placeholder={t('accountPage.avatarUrlPlaceholder')} />
        </Field>
        <Field error={form.formState.errors.language?.message} hint={t('accountPage.languageHint')} label={t('language')} required>
          <Select {...form.register('language')} aria-invalid={Boolean(form.formState.errors.language)}>
            <option value="zh-CN">{t('languages.zhCN')}</option>
            <option value="en-US">{t('languages.enUS')}</option>
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
        <div className="flex justify-end">
          <Button disabled={saveProfile.isPending || !form.formState.isValid || !form.formState.isDirty} type="submit">
            <Save size={16} />
            {t('accountPage.saveProfile')}
          </Button>
        </div>
      </form>
    </Card>
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

  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <h2 className="mb-4 text-base font-semibold">{t('settings.boundIdentities')}</h2>
        {identities.isError && <ErrorState title={t('settings.identityLoadFailedTitle')} description={t('settings.identityLoadFailedDescription')} />}
        {identities.data?.length === 0 && <EmptyState description={t('settings.noIdentitiesDescription')} title={t('settings.noIdentitiesTitle')} variant="plain" />}
        <MotionList className="grid gap-3">
          {(identities.data ?? []).map(identity => (
            <MotionItem key={identity.id}>
              <div className="flex items-center justify-between gap-4 rounded-md border border-border p-3 transition duration-150 hover:border-primary hover:shadow-sm">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <p className="font-medium">{identity.providerName}</p>
                    <StatusValueBadge value={identity.emailVerified ? 'verified' : 'unverified'} />
                  </div>
                  <p className="truncate text-sm text-muted-foreground">{identity.email || identity.username || identity.subject}</p>
                </div>
                <Button
                  aria-label={t('settings.unbindIdentity')}
                  disabled={unbind.isPending}
                  variant="ghost"
                  onClick={() => unbind.mutate(identity.id)}
                >
                  <Unlink size={16} />
                </Button>
              </div>
            </MotionItem>
          ))}
        </MotionList>
      </Card>

      <Card>
        <h2 className="mb-4 text-base font-semibold">{t('settings.bindProviderTitle')}</h2>
        {providers.isError && <ErrorState title={t('settings.providerLoadFailedTitle')} description={t('settings.providerLoadFailedDescription')} />}
        <div className="grid gap-2">
          {(providers.data ?? []).map(provider => (
            <Button
              key={provider.id}
              type="button"
              variant="secondary"
              onClick={() => {
                window.location.href = oidcStartUrl(provider.id, 'bind', '/settings/account')
              }}
            >
              <Link2 size={16} />
              {t('settings.bindProvider', { provider: provider.name })}
            </Button>
          ))}
        </div>
      </Card>
    </div>
  )
}
