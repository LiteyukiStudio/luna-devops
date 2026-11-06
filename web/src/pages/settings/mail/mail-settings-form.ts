import type { MailSettings } from '@/api'
import i18next from 'i18next'
import { z } from 'zod'

export const mailSettingsSchema = z.object({
  host: z.string().trim().min(1, { error: () => i18next.t('settings.mail.hostRequired') }),
  port: z.number().int().min(1).max(65535),
  security: z.enum(['none', 'starttls', 'tls']),
  username: z.string(),
  password: z.string(),
  fromAddress: z.string().trim().email({ error: () => i18next.t('common.validEmailRequired') }),
  fromName: z.string(),
  personalEmailCooldownSeconds: z.number()
    .int({ error: () => i18next.t('settings.mail.personalEmailCooldownInvalid') })
    .min(0, { error: () => i18next.t('settings.mail.personalEmailCooldownInvalid') })
    .max(3600, { error: () => i18next.t('settings.mail.personalEmailCooldownInvalid') }),
})

export type MailSettingsForm = z.infer<typeof mailSettingsSchema>

export const defaultMailSettingsFormValues: MailSettingsForm = {
  host: '',
  port: 587,
  security: 'starttls',
  username: '',
  password: '',
  fromAddress: '',
  fromName: 'Luna DevOps',
  personalEmailCooldownSeconds: 60,
}

export function mailSettingsToForm(settings: MailSettings): MailSettingsForm {
  return {
    host: settings.host,
    port: settings.port,
    security: settings.security,
    username: settings.username,
    password: '',
    fromAddress: settings.fromAddress,
    fromName: settings.fromName,
    personalEmailCooldownSeconds: settings.personalEmailCooldownSeconds,
  }
}
