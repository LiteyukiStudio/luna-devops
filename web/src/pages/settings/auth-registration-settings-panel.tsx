import type { AuthRegistrationSettings } from '@/api/types'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Save } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { CheckboxField } from '@/components/common/checkbox-field'
import { ErrorState } from '@/components/common/error-state'
import { FormActions } from '@/components/common/form-actions'
import { FormField as Field } from '@/components/common/form-field'
import { Section } from '@/components/common/section'
import { Surface } from '@/components/common/surface'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'

const schema = z.object({
  allowEmailRegistration: z.boolean(),
  allowOidcRegistration: z.boolean(),
  allowExternalIdentityPassword: z.boolean(),
  smtpHost: z.string(),
  smtpPort: z.number().int().min(1).max(65535),
  smtpSecurity: z.enum(['none', 'starttls', 'tls']),
  smtpUsername: z.string(),
  smtpPassword: z.string(),
  smtpFromAddress: z.string(),
  smtpFromName: z.string(),
}).superRefine((value, context) => {
  if (!value.allowEmailRegistration)
    return
  if (!value.smtpHost.trim())
    context.addIssue({ code: 'custom', path: ['smtpHost'], message: i18next.t('settings.registration.smtpHostRequired') })
  if (!z.string().email().safeParse(value.smtpFromAddress.trim()).success)
    context.addIssue({ code: 'custom', path: ['smtpFromAddress'], message: i18next.t('common.validEmailRequired') })
})

type FormValues = z.infer<typeof schema>

const defaultValues: FormValues = {
  allowEmailRegistration: false,
  allowOidcRegistration: true,
  allowExternalIdentityPassword: false,
  smtpHost: '',
  smtpPort: 587,
  smtpSecurity: 'starttls',
  smtpUsername: '',
  smtpPassword: '',
  smtpFromAddress: '',
  smtpFromName: 'Luna DevOps',
}

export function AuthRegistrationSettingsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const settings = useQuery({ queryKey: ['auth-registration-settings'], queryFn: api.getAuthRegistrationSettings })
  const form = useForm<FormValues>({ resolver: zodResolver(schema), mode: 'onChange', defaultValues })

  useEffect(() => {
    if (settings.data)
      form.reset(settingsToForm(settings.data))
  }, [form, settings.data])

  const save = useMutation({
    mutationFn: api.updateAuthRegistrationSettings,
    onSuccess: (result) => {
      queryClient.setQueryData(['auth-registration-settings'], result)
      queryClient.invalidateQueries({ queryKey: ['auth-registration-status'] })
      form.reset(settingsToForm(result))
      toast.success(t('settings.registration.saved'))
    },
    onError: error => toast.error(error.message),
  })

  if (settings.isError)
    return <ErrorState title={t('settings.registration.loadFailedTitle')} description={t('settings.registration.loadFailedDescription')} />

  const emailEnabled = form.watch('allowEmailRegistration')
  const smtpPasswordSet = settings.data?.smtpPasswordSet ?? false

  return (
    <div className="grid max-w-3xl gap-4">
      <Surface className="grid gap-4 rounded-xl p-6" variant="bordered">
        <div className="grid gap-3">
          <CheckboxField description={t('settings.registration.emailRegistrationDescription')} {...form.register('allowEmailRegistration')}>
            {t('settings.registration.emailRegistration')}
          </CheckboxField>
          <CheckboxField description={t('settings.registration.oidcRegistrationDescription')} {...form.register('allowOidcRegistration')}>
            {t('settings.registration.oidcRegistration')}
          </CheckboxField>
          <CheckboxField description={t('settings.registration.externalPasswordDescription')} {...form.register('allowExternalIdentityPassword')}>
            {t('settings.registration.externalPassword')}
          </CheckboxField>
        </div>
      </Surface>

      <Section className="rounded-xl" title={t('settings.registration.smtpTitle')} variant="bordered">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field error={form.formState.errors.smtpHost?.message} label={t('settings.registration.smtpHost')} required={emailEnabled}>
            <Input {...form.register('smtpHost')} aria-invalid={Boolean(form.formState.errors.smtpHost)} placeholder="smtp.example.com" />
          </Field>
          <Field error={form.formState.errors.smtpPort?.message} label={t('settings.registration.smtpPort')} required>
            <Input {...form.register('smtpPort', { valueAsNumber: true })} aria-invalid={Boolean(form.formState.errors.smtpPort)} inputMode="numeric" type="number" />
          </Field>
          <Field label={t('settings.registration.smtpSecurity')} required>
            <NativeSelect {...form.register('smtpSecurity')}>
              <option value="starttls">STARTTLS</option>
              <option value="tls">TLS</option>
              <option value="none">{t('settings.registration.smtpSecurityNone')}</option>
            </NativeSelect>
          </Field>
          <Field label={t('settings.registration.smtpUsername')}>
            <Input {...form.register('smtpUsername')} autoComplete="username" />
          </Field>
          <Field hint={t('settings.registration.smtpPasswordHint')} label={t('settings.registration.smtpPassword')}>
            <Input
              {...form.register('smtpPassword')}
              autoComplete="new-password"
              placeholder={smtpPasswordSet ? t('common.secretSetPlaceholder') : undefined}
              type="password"
            />
          </Field>
          <Field error={form.formState.errors.smtpFromAddress?.message} label={t('settings.registration.smtpFromAddress')} required={emailEnabled}>
            <Input {...form.register('smtpFromAddress')} aria-invalid={Boolean(form.formState.errors.smtpFromAddress)} type="email" />
          </Field>
          <Field label={t('settings.registration.smtpFromName')}>
            <Input {...form.register('smtpFromName')} />
          </Field>
        </div>
      </Section>

      <FormActions separated={false}>
        <Button
          disabled={save.isPending || settings.isLoading || !form.formState.isValid || !form.formState.isDirty}
          type="button"
          onClick={() => void form.handleSubmit(values => save.mutate(values))()}
        >
          <Save className="size-4" />
          {t('settings.registration.save')}
        </Button>
      </FormActions>
    </div>
  )
}

function settingsToForm(settings: AuthRegistrationSettings): FormValues {
  return {
    allowEmailRegistration: settings.allowEmailRegistration,
    allowOidcRegistration: settings.allowOidcRegistration,
    allowExternalIdentityPassword: settings.allowExternalIdentityPassword,
    smtpHost: settings.smtpHost,
    smtpPort: settings.smtpPort,
    smtpSecurity: settings.smtpSecurity,
    smtpUsername: settings.smtpUsername,
    smtpPassword: '',
    smtpFromAddress: settings.smtpFromAddress,
    smtpFromName: settings.smtpFromName,
  }
}
