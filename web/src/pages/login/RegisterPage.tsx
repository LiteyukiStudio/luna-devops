import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import i18next from 'i18next'
import { ArrowLeft, MailCheck, UserPlus } from 'lucide-react'
import { useRef } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useDocumentTitle } from '@/app/document-title'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ErrorState } from '@/components/common/error-state'
import { FormField as Field } from '@/components/common/form-field'
import { PageMotion } from '@/components/common/motion'
import { OneTimeCodeInput } from '@/components/common/one-time-code-input'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

const schema = z.object({
  email: z.string().email(i18next.t('common.validEmailRequired')),
  name: z.string().min(1, i18next.t('accountPage.profileNameRequired')),
  password: z.string().min(8, i18next.t('usersPage.passwordMin')),
  confirmPassword: z.string(),
  code: z.string().regex(/^\d{6}$/, i18next.t('loginPage.registration.codeRequired')),
  rememberMe: z.boolean(),
}).refine(value => value.password === value.confirmPassword, {
  path: ['confirmPassword'],
  message: i18next.t('loginPage.registration.passwordMismatch'),
})

type RegistrationForm = z.infer<typeof schema>

export function RegisterPage() {
  const { i18n, t } = useTranslation()
  const navigate = useNavigate()
  const session = useSession()
  const configs = usePublicConfig()
  const codeInputRef = useRef<HTMLInputElement>(null)
  const status = useQuery({ queryKey: ['auth-registration-status'], queryFn: api.getAuthRegistrationStatus })
  const form = useForm<RegistrationForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { email: '', name: '', password: '', confirmPassword: '', code: '', rememberMe: false },
  })
  useDocumentTitle(t('loginPage.registration.title'))

  const requestCode = useMutation({
    mutationFn: (email: string) => api.requestEmailRegistrationCode({ email, language: i18n.resolvedLanguage === 'en-US' ? 'en-US' : 'zh-CN' }),
    onSuccess: () => {
      form.setValue('code', '')
      form.clearErrors('code')
      requestAnimationFrame(() => codeInputRef.current?.focus())
      toast.success(t('loginPage.registration.codeSent'))
    },
    onError: error => toast.error(error.message),
  })
  const challengeId = requestCode.data?.challengeId ?? ''

  const register = useMutation({
    mutationFn: (values: RegistrationForm) => api.completeEmailRegistration({
      challengeId,
      code: values.code,
      email: values.email,
      name: values.name,
      password: values.password,
      language: i18n.resolvedLanguage === 'en-US' ? 'en-US' : 'zh-CN',
      rememberMe: values.rememberMe,
    }),
    onSuccess: async () => {
      await session.refreshUser()
      toast.success(t('loginPage.registration.success'))
      navigate('/projects', { replace: true })
    },
    onError: error => toast.error(error.message),
  })

  if (status.isLoading)
    return <div className="min-h-screen bg-background" />
  if (status.isError)
    return <div className="grid min-h-screen place-items-center p-4"><ErrorState title={t('loginPage.registration.statusFailedTitle')} description={t('loginPage.registration.statusFailedDescription')} /></div>
  if (!status.data?.emailRegistrationEnabled)
    return <Navigate to="/login" replace />

  return (
    <div className="grid min-h-screen place-items-center bg-background px-4 py-8 text-foreground">
      <PageMotion className="w-full max-w-lg">
        <Card className="grid gap-5 p-6 sm:p-8">
          <div className="flex items-center gap-3">
            <img alt="" className="size-11 rounded-xl object-contain" src={configs['site.logoUrl'] || '/luna-devops-logo.svg'} />
            <div className="min-w-0">
              <h1 className="text-lg font-semibold">{t('loginPage.registration.title')}</h1>
              <p className="text-sm text-muted-foreground">{t('loginPage.registration.description')}</p>
            </div>
          </div>

          <form className="grid gap-3" onSubmit={form.handleSubmit(values => register.mutate(values))}>
            <Field error={form.formState.errors.email?.message} label={t('loginPage.email')} required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} autoComplete="email" type="email" />
            </Field>
            <Field error={form.formState.errors.name?.message} label={t('accountPage.profileName')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} autoComplete="name" />
            </Field>
            <Field error={form.formState.errors.password?.message} label={t('loginPage.password')} required>
              <Input {...form.register('password')} aria-invalid={Boolean(form.formState.errors.password)} autoComplete="new-password" type="password" />
            </Field>
            <Field error={form.formState.errors.confirmPassword?.message} label={t('loginPage.registration.confirmPassword')} required>
              <Input {...form.register('confirmPassword')} aria-invalid={Boolean(form.formState.errors.confirmPassword)} autoComplete="new-password" type="password" />
            </Field>
            <Field error={form.formState.errors.code?.message} label={t('loginPage.registration.code')} required>
              <div className="flex flex-col items-start gap-2 sm:flex-row sm:items-center">
                <Controller
                  control={form.control}
                  name="code"
                  render={({ field }) => (
                    <OneTimeCodeInput
                      {...field}
                      ref={codeInputRef}
                      aria-label={t('loginPage.registration.code')}
                      invalid={Boolean(form.formState.errors.code)}
                      name="one-time-code"
                    />
                  )}
                />
                <Button
                  className="w-full sm:w-auto"
                  disabled={requestCode.isPending}
                  type="button"
                  variant="secondary"
                  onClick={async () => {
                    if (await form.trigger('email'))
                      requestCode.mutate(form.getValues('email'))
                  }}
                >
                  <MailCheck className="size-4" />
                  {requestCode.data ? t('loginPage.registration.resendCode') : t('loginPage.registration.sendCode')}
                </Button>
              </div>
            </Field>
            <CheckboxField {...form.register('rememberMe')}>{t('loginPage.rememberMe')}</CheckboxField>
            <Button disabled={!challengeId || register.isPending || !form.formState.isValid} type="submit">
              <UserPlus className="size-4" />
              {t('loginPage.registration.submit')}
            </Button>
          </form>

          <Button asChild variant="ghost">
            <Link to="/login">
              <ArrowLeft className="size-4" />
              {t('loginPage.registration.backToLogin')}
            </Link>
          </Button>
        </Card>
      </PageMotion>
    </div>
  )
}
