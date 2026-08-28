import type { MailSettings } from '@/api'
import { describe, expect, it } from 'vitest'
import { mailSettingsSchema, mailSettingsToForm } from './mail-settings-form'

const validForm = {
  host: 'smtp.example.com',
  port: 587,
  security: 'starttls' as const,
  username: 'mailer',
  password: '',
  fromAddress: 'noreply@example.com',
  fromName: 'Luna DevOps',
  personalEmailCooldownSeconds: 60,
}

describe('mail settings form contract', () => {
  it('accepts the documented personal email cooldown boundaries', () => {
    expect(mailSettingsSchema.safeParse({ ...validForm, personalEmailCooldownSeconds: 0 }).success).toBe(true)
    expect(mailSettingsSchema.safeParse({ ...validForm, personalEmailCooldownSeconds: 3600 }).success).toBe(true)
  })

  it('rejects cooldowns outside the backend range', () => {
    expect(mailSettingsSchema.safeParse({ ...validForm, personalEmailCooldownSeconds: -1 }).success).toBe(false)
    expect(mailSettingsSchema.safeParse({ ...validForm, personalEmailCooldownSeconds: 3601 }).success).toBe(false)
    expect(mailSettingsSchema.safeParse({ ...validForm, personalEmailCooldownSeconds: 1.5 }).success).toBe(false)
  })

  it('preserves the authoritative cooldown when loading the form', () => {
    const settings: MailSettings = {
      host: 'smtp.example.com',
      port: 587,
      security: 'starttls',
      username: 'mailer',
      passwordSet: true,
      fromAddress: 'noreply@example.com',
      fromName: 'Luna DevOps',
      personalEmailCooldownSeconds: 300,
    }

    expect(mailSettingsToForm(settings).personalEmailCooldownSeconds).toBe(300)
  })
})
