import i18next from 'i18next'
import { z } from 'zod'

export const aiSettingsSchema = z.object({
  enabled: z.boolean(),
  baseUrl: z.union([z.literal(''), z.url().refine(value => value.startsWith('https://'))]),
  apiKey: z.string(),
  apiKeyConfigured: z.boolean(),
  model: z.string(),
  webProxyEnabled: z.boolean(),
  webProxyPool: z.string().superRefine((value, context) => {
    const entries = value.split(/\r?\n/).map(item => item.trim()).filter(Boolean)
    if (entries.length > 16) {
      context.addIssue({ code: 'custom', message: i18next.t('settings.ai.webProxyPoolInvalid') })
      return
    }
    for (const entry of entries) {
      try {
        const parsed = new URL(entry)
        if (!['http:', 'https:'].includes(parsed.protocol) || !parsed.hostname || parsed.search || parsed.hash || (parsed.pathname && parsed.pathname !== '/'))
          throw new Error('invalid proxy')
      }
      catch {
        context.addIssue({ code: 'custom', message: i18next.t('settings.ai.webProxyPoolInvalid') })
        return
      }
    }
  }),
  webProxyPoolConfigured: z.boolean(),
  providerTimeoutSeconds: z.number({ message: i18next.t('settings.ai.providerTimeoutInvalid') })
    .int({ message: i18next.t('settings.ai.providerTimeoutInvalid') })
    .min(1, { message: i18next.t('settings.ai.providerTimeoutInvalid') })
    .max(120, { message: i18next.t('settings.ai.providerTimeoutInvalid') }),
  runTimeoutSeconds: z.number({ message: i18next.t('settings.ai.runTimeoutInvalid') })
    .int({ message: i18next.t('settings.ai.runTimeoutInvalid') })
    .min(30, { message: i18next.t('settings.ai.runTimeoutInvalid') })
    .max(900, { message: i18next.t('settings.ai.runTimeoutInvalid') }),
  agentConcurrentRuns: z.number({ message: i18next.t('settings.ai.agentConcurrentRunsInvalid') })
    .int({ message: i18next.t('settings.ai.agentConcurrentRunsInvalid') })
    .min(1, { message: i18next.t('settings.ai.agentConcurrentRunsInvalid') })
    .max(10, { message: i18next.t('settings.ai.agentConcurrentRunsInvalid') }),
}).superRefine((value, context) => {
  if (!value.enabled)
    return
  if (!value.baseUrl)
    context.addIssue({ code: 'custom', path: ['baseUrl'], message: i18next.t('settings.ai.baseUrlRequired') })
  if (!value.model.trim())
    context.addIssue({ code: 'custom', path: ['model'], message: i18next.t('settings.ai.modelRequired') })
  if (!value.apiKey.trim() && !value.apiKeyConfigured)
    context.addIssue({ code: 'custom', path: ['apiKey'], message: i18next.t('settings.ai.apiKeyRequired') })
  if (value.webProxyEnabled && !value.webProxyPool.trim() && !value.webProxyPoolConfigured)
    context.addIssue({ code: 'custom', path: ['webProxyPool'], message: i18next.t('settings.ai.webProxyPoolRequired') })
})

export type AISettingsFormValues = z.infer<typeof aiSettingsSchema>

export function aiSettingsPayload(values: AISettingsFormValues) {
  const payload: Record<string, unknown> = {
    'ai.assistant.enabled': values.enabled,
    'ai.provider.base_url': values.baseUrl.trim(),
    'ai.provider.default_model': values.model.trim(),
    'ai.web.proxy_enabled': values.webProxyEnabled,
    'ai.runtime.provider_timeout_seconds': values.providerTimeoutSeconds,
    'ai.runtime.run_timeout_seconds': values.runTimeoutSeconds,
    'ai.runtime.agent_concurrent_runs': values.agentConcurrentRuns,
  }
  if (values.apiKey.trim())
    payload['ai.provider.api_key'] = values.apiKey.trim()
  if (values.webProxyPool.trim())
    payload['ai.web.proxy_pool'] = values.webProxyPool.trim()
  return payload
}
