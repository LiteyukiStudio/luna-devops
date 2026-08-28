import type { MailSettings } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { Save, Send } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ErrorState } from '@/components/common/error-state'
import { FormActions } from '@/components/common/form-actions'
import { FormField as Field } from '@/components/common/form-field'
import { Section } from '@/components/common/section'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'

const mailSettingsSchema = z.object({
  host: z.string().trim().min(1, i18next.t('settings.mail.hostRequired')),
  port: z.number().int().min(1).max(65535),
  security: z.enum(['none', 'starttls', 'tls']),
  username: z.string(),
  password: z.string(),
  fromAddress: z.string().trim().email(i18next.t('common.validEmailRequired')),
  fromName: z.string(),
})

type MailSettingsForm = z.infer<typeof mailSettingsSchema>

const defaultValues: MailSettingsForm = {
  host: '',
  port: 587,
  security: 'starttls',
  username: '',
  password: '',
  fromAddress: '',
  fromName: 'Luna DevOps',
}

export function MailSettingsPanel() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [testRecipient, setTestRecipient] = useState('')
  const settings = useQuery({ queryKey: ['mail-settings'], queryFn: api.getMailSettings })
  const form = useForm<MailSettingsForm>({
    defaultValues,
    mode: 'onChange',
    resolver: zodResolver(mailSettingsSchema),
  })

  useEffect(() => {
    if (settings.data)
      form.reset(mailSettingsToForm(settings.data))
  }, [form, settings.data])

  const save = useMutation({
    mutationFn: (values: MailSettingsForm) => api.updateMailSettings({
      ...values,
      password: values.password.trim() || undefined,
    }),
    onSuccess: (result) => {
      queryClient.setQueryData(['mail-settings'], result)
      form.reset(mailSettingsToForm(result))
      toast.success(t('settings.mail.saved'))
    },
    onError: error => toast.error(error.message),
  })
  const test = useMutation({
    mutationFn: api.testMailSettings,
    onSuccess: () => toast.success(t('settings.mail.testSent')),
    onError: error => toast.error(error.message),
  })

  if (settings.isError)
    return <ErrorState title={t('settings.mail.loadFailedTitle')} description={t('settings.mail.loadFailedDescription')} />

  const passwordSet = settings.data?.passwordSet ?? false
  const testRecipientValid = z.string().email().safeParse(testRecipient.trim()).success

  return (
    <Section
      className="max-w-3xl"
      description={t('settings.mail.description')}
      title={t('settings.mail.title')}
      variant="bordered"
    >
      <form className="grid gap-4" onSubmit={form.handleSubmit(values => save.mutate(values))}>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field error={form.formState.errors.host?.message} label={t('settings.mail.host')} required>
            <Input {...form.register('host')} aria-invalid={Boolean(form.formState.errors.host)} placeholder="smtp.example.com" />
          </Field>
          <Field error={form.formState.errors.port?.message} label={t('settings.mail.port')} required>
            <Input {...form.register('port', { valueAsNumber: true })} aria-invalid={Boolean(form.formState.errors.port)} inputMode="numeric" max={65535} min={1} type="number" />
          </Field>
          <Field label={t('settings.mail.security')} required>
            <NativeSelect {...form.register('security')}>
              <option value="starttls">{t('settings.mail.securityStartTLS')}</option>
              <option value="tls">{t('settings.mail.securityTLS')}</option>
              <option value="none">{t('settings.mail.securityNone')}</option>
            </NativeSelect>
          </Field>
          <Field label={t('settings.mail.username')}>
            <Input {...form.register('username')} autoComplete="off" />
          </Field>
          <Field hint={t('settings.mail.passwordHint')} label={t('settings.mail.password')}>
            <Input
              {...form.register('password')}
              autoComplete="new-password"
              placeholder={passwordSet ? t('common.secretSetPlaceholder') : undefined}
              type="password"
            />
          </Field>
          <Field error={form.formState.errors.fromAddress?.message} label={t('settings.mail.fromAddress')} required>
            <Input {...form.register('fromAddress')} aria-invalid={Boolean(form.formState.errors.fromAddress)} type="email" />
          </Field>
          <Field label={t('settings.mail.fromName')}>
            <Input {...form.register('fromName')} />
          </Field>
        </div>

        <div className="grid gap-2 border-t border-border pt-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
          <Field hint={t('settings.mail.testRecipientHint')} label={t('settings.mail.testRecipient')}>
            <Input value={testRecipient} type="email" onChange={event => setTestRecipient(event.target.value)} />
          </Field>
        </div>

        <FormActions separated={false}>
          <Button
            disabled={!testRecipientValid || test.isPending || settings.isLoading || form.formState.isDirty || !settings.data?.host}
            type="button"
            variant="outline"
            onClick={() => test.mutate(testRecipient.trim())}
          >
            <Send className="size-4" />
            {t('settings.mail.sendTest')}
          </Button>
          <Button disabled={settings.isLoading || save.isPending || !form.formState.isValid || !form.formState.isDirty} type="submit">
            <Save className="size-4" />
            {t('settings.mail.save')}
          </Button>
        </FormActions>
      </form>
    </Section>
  )
}

function mailSettingsToForm(settings: MailSettings): MailSettingsForm {
  return {
    host: settings.host,
    port: settings.port,
    security: settings.security,
    username: settings.username,
    password: '',
    fromAddress: settings.fromAddress,
    fromName: settings.fromName,
  }
}
