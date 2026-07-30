import i18next from 'i18next'
import { z } from 'zod'

export const aiSettingsSchema = z.object({
  enabled: z.boolean(),
  baseUrl: z.union([z.literal(''), z.url().refine(value => value.startsWith('https://'))]),
  apiKey: z.string(),
  apiKeyConfigured: z.boolean(),
  model: z.string(),
}).superRefine((value, context) => {
  if (!value.enabled)
    return
  if (!value.baseUrl)
    context.addIssue({ code: 'custom', path: ['baseUrl'], message: i18next.t('settings.ai.baseUrlRequired') })
  if (!value.model.trim())
    context.addIssue({ code: 'custom', path: ['model'], message: i18next.t('settings.ai.modelRequired') })
  if (!value.apiKey.trim() && !value.apiKeyConfigured)
    context.addIssue({ code: 'custom', path: ['apiKey'], message: i18next.t('settings.ai.apiKeyRequired') })
})

export type AISettingsFormValues = z.infer<typeof aiSettingsSchema>

export function aiSettingsPayload(values: AISettingsFormValues) {
  const payload: Record<string, unknown> = {
    'ai.assistant.enabled': values.enabled,
    'ai.provider.base_url': values.baseUrl.trim(),
    'ai.provider.default_model': values.model.trim(),
  }
  if (values.apiKey.trim())
    payload['ai.provider.api_key'] = values.apiKey.trim()
  return payload
}
