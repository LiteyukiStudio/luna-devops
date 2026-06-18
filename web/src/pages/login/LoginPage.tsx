import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import i18next from 'i18next'
import { LogIn, TriangleAlert } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api, oidcStartUrl } from '@/api/client'
import { useDocumentTitle } from '@/app/document-title'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { FormField as Field } from '@/components/common/form-field'
import { PageMotion } from '@/components/common/motion'
import { UserAvatar } from '@/components/common/user-avatar'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

const schema = z.object({
  email: z.string().email(i18next.t('common.validEmailRequired')),
  password: z.string().min(1, i18next.t('loginPage.passwordRequired')),
})

type LoginForm = z.infer<typeof schema>

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const session = useSession()
  const configs = usePublicConfig()
  const redirectTo = safeRedirect(searchParams.get('redirect'))
  const authErrorCode = searchParams.get('auth_error')
  const authError = useMemo(() => authErrorCode ? authErrorMessage(authErrorCode, t) : null, [authErrorCode, t])
  const status = useQuery({ queryKey: ['bootstrap-status'], queryFn: api.getBootstrapStatus })
  const providers = useQuery({ queryKey: ['auth-providers'], queryFn: () => api.listAuthProviders(false) })
  const form = useForm<LoginForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      email: '',
      password: '',
    },
  })
  useDocumentTitle(t('login'))

  useEffect(() => {
    if (session.user)
      navigate(redirectTo, { replace: true })
  }, [navigate, redirectTo, session.user])

  useEffect(() => {
    if (status.data && !status.data.initialized)
      navigate('/bootstrap', { replace: true })
  }, [navigate, status.data])

  useEffect(() => {
    if (authError)
      toast.error(authError.title, { id: `auth-error-${authErrorCode}`, description: authError.description })
  }, [authError, authErrorCode])

  const handleLogin = form.handleSubmit((values) => {
    session.login(values)
      .then(() => {
        toast.success(t('loginPage.success'))
        navigate(redirectTo)
      })
      .catch(error => toast.error(error.message))
  })

  function prefillRecentUser(email: string) {
    form.setValue('email', email, { shouldDirty: true, shouldTouch: true, shouldValidate: true })
    form.setValue('password', '', { shouldDirty: true, shouldTouch: true, shouldValidate: true })
  }

  function selectRecentUser(user: { email: string, id: string }) {
    session.resumeLogin(user.id)
      .then(() => {
        toast.success(t('loginPage.success'))
        navigate(redirectTo)
      })
      .catch((error) => {
        prefillRecentUser(user.email)
        toast.error(error.message)
      })
  }

  return (
    <div className="grid min-h-screen place-items-center bg-background px-4 py-8 text-foreground">
      <PageMotion className="w-full max-w-5xl">
        <Card className="relative overflow-visible p-0">
          <div className="grid lg:min-h-[620px] lg:grid-cols-[1.08fr_0.92fr]">
            <div className="relative hidden overflow-visible rounded-l-[inherit] bg-gradient-to-br from-[#eef5ff] via-[#f8fbff] to-[#e8fbf7] lg:block">
              <div className="absolute inset-0 rounded-l-[inherit] bg-[linear-gradient(135deg,rgba(47,123,244,0.12)_0,transparent_36%),linear-gradient(45deg,rgba(34,199,169,0.12)_0,transparent_42%)]" />
              <img
                alt=""
                className="pointer-events-none absolute bottom-0 left-[51.5%] z-10 h-[112%] w-auto max-w-none -translate-x-1/2 select-none object-contain object-bottom drop-shadow-[0_28px_42px_rgba(47,123,244,0.22)]"
                src="/brand/mascot-liteyuki-catgirl-login-alpha.png"
              />
              <div className="absolute inset-0 z-20 rounded-l-[inherit] bg-gradient-to-r from-background/10 via-transparent to-background/30" />
            </div>
            <div className="flex min-w-0 items-center justify-center p-6 sm:p-8 lg:p-10">
              <div className="w-full max-w-sm">
                <div className="mb-6 flex items-center gap-3">
                  <img
                    alt=""
                    className="size-11 shrink-0 rounded-xl object-contain"
                    src={configs['site.logoUrl'] || '/liteyuki-logo.svg'}
                  />
                  <div className="min-w-0">
                    <h1 className="truncate text-lg font-semibold">{configs['site.title'] || t('appName')}</h1>
                    <p className="text-sm text-muted-foreground">{configs['site.loginSubtitle'] || t('loginPage.subtitle')}</p>
                  </div>
                </div>
                {authError && (
                  <Alert className="mb-4" variant="destructive">
                    <TriangleAlert />
                    <AlertTitle>{authError.title}</AlertTitle>
                    <AlertDescription>{authError.description}</AlertDescription>
                  </Alert>
                )}
                <form className="grid gap-3" onSubmit={handleLogin}>
                  {session.recentLoginUsers.length > 0 && (
                    <div className="grid gap-2">
                      <p className="text-xs font-medium text-muted-foreground">{t('loginPage.recentAccounts')}</p>
                      <div className="flex items-center gap-2">
                        {session.recentLoginUsers.map(user => (
                          <Tooltip key={user.id}>
                            <TooltipTrigger asChild>
                              <button
                                aria-label={t('loginPage.selectRecentAccount', { email: user.email, name: user.name })}
                                className="group flex size-10 items-center justify-center overflow-hidden rounded-full border border-border bg-muted text-sm font-semibold text-muted-foreground transition hover:border-primary hover:text-primary focus-visible:border-primary focus-visible:ring-3 focus-visible:ring-primary/30 focus-visible:outline-none"
                                type="button"
                                onClick={() => selectRecentUser(user)}
                              >
                                <UserAvatar className="size-full" user={user} />
                              </button>
                            </TooltipTrigger>
                            <TooltipContent side="top">
                              <div className="grid gap-0.5">
                                <span className="font-medium">{user.name || t('common.noDescription')}</span>
                                <span className="text-background/80">{user.email}</span>
                              </div>
                            </TooltipContent>
                          </Tooltip>
                        ))}
                      </div>
                    </div>
                  )}
                  <Field error={form.formState.errors.email?.message} hint={t('loginPage.emailHint')} label={t('loginPage.email')} required>
                    <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} autoComplete="email" />
                  </Field>
                  <Field error={form.formState.errors.password?.message} hint={t('loginPage.passwordHint')} label={t('loginPage.password')} required>
                    <Input {...form.register('password')} aria-invalid={Boolean(form.formState.errors.password)} autoComplete="current-password" type="password" />
                  </Field>
                  <Button disabled={session.isLoggingIn || !form.formState.isValid} type="submit">
                    <LogIn size={16} />
                    {t('login')}
                  </Button>
                  {status.data?.mode === 'development' && status.data.devLoginEnabled && status.data.devLoginHint && (
                    <p className="text-xs text-muted-foreground">
                      {t('loginPage.devAccount')}
                      {status.data.devLoginHint.email}
                      {' '}
                      /
                      {' '}
                      {status.data.devLoginHint.password}
                    </p>
                  )}
                </form>
                {(providers.data ?? []).length > 0 && (
                  <div className="mt-5 grid gap-2 border-t border-border pt-5">
                    {(providers.data ?? []).map(provider => (
                      <Button
                        key={provider.id}
                        type="button"
                        variant="secondary"
                        onClick={() => {
                          window.location.href = oidcStartUrl(provider.id, 'login')
                        }}
                      >
                        <LogIn size={16} />
                        {t('loginPage.useProviderLogin', { provider: provider.name })}
                      </Button>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        </Card>
      </PageMotion>
    </div>
  )
}

function safeRedirect(value: string | null) {
  if (!value || !value.startsWith('/') || value.startsWith('//'))
    return '/projects'
  if (value === '/login' || value.startsWith('/login?') || value === '/bootstrap' || value.startsWith('/bootstrap?'))
    return '/projects'
  return value
}

function authErrorMessage(code: string, t: ReturnType<typeof useTranslation>['t']) {
  const messages: Record<string, { description: string, title: string }> = {
    oidc_callback_invalid: { title: t('loginPage.oidcCallbackInvalidTitle'), description: t('loginPage.oidcCallbackInvalid') },
    oidc_state_invalid: { title: t('loginPage.oidcStateInvalidTitle'), description: t('loginPage.oidcStateInvalid') },
    oidc_group_denied: { title: t('loginPage.oidcGroupDeniedTitle'), description: t('loginPage.oidcGroupDenied') },
    oidc_email_required: { title: t('loginPage.oidcEmailRequiredTitle'), description: t('loginPage.oidcEmailRequired') },
    oidc_admission_denied: { title: t('loginPage.oidcAdmissionDeniedTitle'), description: t('loginPage.oidcAdmissionDenied') },
    oidc_provider_disabled: { title: t('loginPage.oidcProviderDisabledTitle'), description: t('loginPage.oidcProviderDisabled') },
    oidc_disabled: { title: t('loginPage.oidcDisabledTitle'), description: t('loginPage.oidcDisabled') },
    oidc_discovery_failed: { title: t('loginPage.oidcDiscoveryFailedTitle'), description: t('loginPage.oidcDiscoveryFailed') },
    oidc_code_invalid: { title: t('loginPage.oidcCodeInvalidTitle'), description: t('loginPage.oidcCodeInvalid') },
    oidc_token_invalid: { title: t('loginPage.oidcTokenInvalidTitle'), description: t('loginPage.oidcTokenInvalid') },
    oidc_bind_failed: { title: t('loginPage.oidcBindFailedTitle'), description: t('loginPage.oidcBindFailed') },
    oidc_login_failed: { title: t('loginPage.oidcFallbackTitle'), description: t('loginPage.oidcFallback') },
    auth_forbidden: { title: t('loginPage.authForbiddenTitle'), description: t('loginPage.authForbidden') },
  }
  return messages[code] ?? { title: t('loginPage.oidcFallbackTitle'), description: t('loginPage.oidcFallback') }
}
