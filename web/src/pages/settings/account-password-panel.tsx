import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import i18next from 'i18next'
import { KeyRound, Save } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { FormActions } from '@/components/common/form-actions'
import { FormField as Field } from '@/components/common/form-field'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

const schema = z.object({
  currentPassword: z.string(),
  newPassword: z.string().min(8, i18next.t('usersPage.passwordMin')),
  confirmPassword: z.string(),
}).refine(value => value.newPassword === value.confirmPassword, {
  path: ['confirmPassword'],
  message: i18next.t('accountPage.password.mismatch'),
})

type PasswordForm = z.infer<typeof schema>

export function AccountPasswordPanel() {
  const { t } = useTranslation()
  const { user } = useSession()
  const registration = useQuery({ queryKey: ['auth-registration-status'], queryFn: api.getAuthRegistrationStatus })
  const form = useForm<PasswordForm>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: { currentPassword: '', newPassword: '', confirmPassword: '' },
  })
  const save = useMutation({
    mutationFn: (values: PasswordForm) => api.updateMyPassword({ currentPassword: values.currentPassword, newPassword: values.newPassword }),
    onSuccess: () => {
      toast.success(t('accountPage.password.saved'))
      window.location.assign('/login')
    },
    onError: error => toast.error(error.message),
  })
  const canSetPassword = Boolean(user?.passwordSet || registration.data?.externalIdentityPasswordEnabled)

  return (
    <Card className="grid gap-4">
      <div className="flex items-center gap-2">
        <KeyRound className="size-4 text-muted-foreground" />
        <h2 className="text-base font-semibold">{user?.passwordSet ? t('accountPage.password.changeTitle') : t('accountPage.password.setTitle')}</h2>
      </div>
      {!canSetPassword
        ? (
            <Alert>
              <KeyRound />
              <AlertTitle>{t('accountPage.password.disabledTitle')}</AlertTitle>
              <AlertDescription>{t('accountPage.password.disabledDescription')}</AlertDescription>
            </Alert>
          )
        : (
            <form className="grid gap-3" onSubmit={form.handleSubmit(values => save.mutate(values))}>
              {user?.passwordSet && (
                <Field error={form.formState.errors.currentPassword?.message} label={t('accountPage.password.current')} required>
                  <Input {...form.register('currentPassword', { required: t('accountPage.password.currentRequired') })} autoComplete="current-password" type="password" />
                </Field>
              )}
              <Field error={form.formState.errors.newPassword?.message} label={t('accountPage.password.next')} required>
                <Input {...form.register('newPassword')} autoComplete="new-password" type="password" />
              </Field>
              <Field error={form.formState.errors.confirmPassword?.message} label={t('accountPage.password.confirm')} required>
                <Input {...form.register('confirmPassword')} autoComplete="new-password" type="password" />
              </Field>
              <p className="text-xs text-muted-foreground">{t('accountPage.password.sessionNotice')}</p>
              <FormActions>
                <Button disabled={save.isPending || !form.formState.isValid} type="submit">
                  <Save className="size-4" />
                  {t('accountPage.password.save')}
                </Button>
              </FormActions>
            </form>
          )}
    </Card>
  )
}
