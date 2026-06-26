import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { Box, ShieldPlus } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useDocumentTitle } from '@/app/document-title'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { FormField as Field } from '@/components/common/form-field'
import { PageMotion } from '@/components/common/motion'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import i18next from '@/i18n'

const schema = z.object({
  email: z.string().email(i18next.t('common.validEmailRequired')),
  name: z.string().min(1, i18next.t('bootstrap.nameRequired')),
  password: z.string().min(8, i18next.t('usersPage.passwordMin')),
})

type BootstrapForm = z.infer<typeof schema>

export function BootstrapPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const session = useSession()
  const configs = usePublicConfig()
  const status = useQuery({ queryKey: ['bootstrap-status'], queryFn: api.getBootstrapStatus })
  const form = useForm<BootstrapForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      email: '',
      name: 'Platform Admin',
      password: '',
    },
  })
  useDocumentTitle(t('bootstrap.title'))

  useEffect(() => {
    if (status.data?.initialized)
      navigate('/login', { replace: true })
  }, [navigate, status.data?.initialized])

  const handleInitialize = form.handleSubmit((values) => {
    session.initializeAdmin({ ...values, language: i18n.language === 'en-US' ? 'en-US' : 'zh-CN' })
      .then(() => {
        toast.success(t('bootstrap.success'))
        navigate('/projects')
      })
      .catch(error => toast.error(error.message))
  })

  return (
    <div className="grid min-h-screen place-items-center bg-background px-4 text-foreground">
      <PageMotion className="w-full max-w-md">
        <Card>
          <div className="mb-6 flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-md bg-primary text-primary-foreground">
              {configs['site.logoUrl']
                ? <img alt="" className="size-7 rounded-sm object-contain" src={configs['site.logoUrl']} />
                : <Box size={20} />}
            </span>
            <div>
              <h1 className="text-lg font-semibold">
                {t('bootstrap.title')}
                {' '}
                {configs['site.title'] || 'Liteyuki DevOps'}
              </h1>
              <p className="text-sm text-muted-foreground">{t('bootstrap.description')}</p>
            </div>
          </div>

          <form className="grid gap-3" onSubmit={handleInitialize}>
            <Field error={form.formState.errors.email?.message} hint={t('bootstrap.emailHint')} label={t('bootstrap.email')} required>
              <Input {...form.register('email')} aria-invalid={Boolean(form.formState.errors.email)} autoComplete="email" />
            </Field>
            <Field error={form.formState.errors.name?.message} hint={t('bootstrap.nameHint')} label={t('bootstrap.name')} required>
              <Input {...form.register('name')} aria-invalid={Boolean(form.formState.errors.name)} autoComplete="name" />
            </Field>
            <Field error={form.formState.errors.password?.message} hint={t('bootstrap.passwordHint')} label={t('loginPage.password')} required>
              <Input {...form.register('password')} aria-invalid={Boolean(form.formState.errors.password)} autoComplete="new-password" type="password" />
            </Field>
            <Button disabled={session.isLoggingIn || status.isLoading || !form.formState.isValid} type="submit">
              <ShieldPlus size={16} />
              {t('bootstrap.create')}
            </Button>
            <p className="text-xs text-muted-foreground">
              {t('bootstrap.note')}
            </p>
          </form>
        </Card>
      </PageMotion>
    </div>
  )
}
