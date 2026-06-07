import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import i18next from 'i18next'
import { LogIn } from 'lucide-react'
import { useEffect } from 'react'
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
    const errorCode = searchParams.get('auth_error')
    if (errorCode)
      toast.error(authErrorMessage(errorCode, t))
  }, [searchParams, t])

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
        <Card className="overflow-hidden p-0">
          <div className="grid lg:min-h-[620px] lg:grid-cols-[1.08fr_0.92fr]">
            <div className="relative hidden overflow-hidden bg-muted lg:block">
              <img
                alt=""
                className="absolute inset-0 size-full object-cover"
                src="/brand/mascot-liteyuki-devops.png"
              />
              <div className="absolute inset-0 bg-gradient-to-r from-background/5 via-transparent to-background/20" />
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
  const messages: Record<string, string> = {
    oidc_state_invalid: t('loginPage.oidcStateInvalid'),
    oidc_group_denied: t('loginPage.oidcGroupDenied'),
    oidc_email_required: t('loginPage.oidcEmailRequired'),
    oidc_admission_denied: t('loginPage.oidcAdmissionDenied'),
    oidc_provider_disabled: t('loginPage.oidcProviderDisabled'),
    oidc_bind_failed: t('loginPage.oidcBindFailed'),
    auth_forbidden: t('loginPage.authForbidden'),
  }
  return messages[code] ?? t('loginPage.oidcFallback')
}
