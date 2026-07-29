import i18next from 'i18next'
import { z } from 'zod'

export const aiSettingsSchema = z.object({
  enabled: z.boolean(),
  providerType: z.enum(['', 'openai-compatible']),
  baseUrl: z.union([z.literal(''), z.url().refine(value => value.startsWith('https://'))]),
  apiKey: z.string(),
  defaultModel: z.string(),
  fallbackModel: z.string(),
  modelPricing: z.string().refine((value) => {
    try {
      return Array.isArray(JSON.parse(value))
    }
    catch {
      return false
    }
  }, { message: i18next.t('settings.ai.modelPricingInvalid') }),
  accessMode: z.enum(['admins', 'all_authenticated', 'allowlist']),
  userIds: z.string(),
  projectIds: z.string(),
  userConcurrentRuns: z.number().int().min(1).max(10),
  userDailyTokens: z.number().int().min(1000).max(10_000_000),
  projectConcurrentRuns: z.number().int().min(1).max(50),
  runMaxToolCalls: z.number().int().min(1).max(100),
  platformDailyCostHard: z.number().min(0),
  platformDailyCostSoft: z.number().min(0),
  conversationDays: z.number().int().refine(value => value === 0 || (value >= 7 && value <= 365)),
  runEventDays: z.number().int().refine(value => value === 0 || (value >= 1 && value <= 90)),
  checkpointDays: z.number().int().min(1).max(30),
}).superRefine((value, context) => {
  if (value.enabled && (!value.providerType || !value.baseUrl || !value.defaultModel.trim()))
    context.addIssue({ code: 'custom', path: ['providerType'], message: i18next.t('settings.ai.providerRequired') })
  if (value.accessMode === 'all_authenticated' && value.platformDailyCostHard <= 0)
    context.addIssue({ code: 'custom', path: ['platformDailyCostHard'], message: i18next.t('settings.ai.hardLimitRequired') })
})

export type AISettingsFormValues = z.infer<typeof aiSettingsSchema>

function listValue(value: string) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}

export function aiSettingsPayload(values: AISettingsFormValues) {
  const payload: Record<string, unknown> = {
    'ai.assistant.enabled': values.enabled,
    'ai.provider.type': values.providerType,
    'ai.provider.base_url': values.baseUrl.trim(),
    'ai.provider.default_model': values.defaultModel.trim(),
    'ai.provider.fallback_model': values.fallbackModel.trim(),
    'ai.provider.model_pricing': JSON.parse(values.modelPricing),
    'ai.access.mode': values.accessMode,
    'ai.access.user_ids': listValue(values.userIds),
    'ai.access.project_ids': listValue(values.projectIds),
    'ai.quota.user_concurrent_runs': values.userConcurrentRuns,
    'ai.quota.user_daily_tokens': values.userDailyTokens,
    'ai.quota.project_concurrent_runs': values.projectConcurrentRuns,
    'ai.quota.run_max_tool_calls': values.runMaxToolCalls,
    'ai.quota.platform_daily_cost_hard': values.platformDailyCostHard,
    'ai.quota.platform_daily_cost_soft': values.platformDailyCostSoft,
    'ai.retention.conversation_days': values.conversationDays,
    'ai.retention.run_event_days': values.runEventDays,
    'ai.retention.checkpoint_days': values.checkpointDays,
  }
  if (values.apiKey.trim())
    payload['ai.provider.api_key'] = values.apiKey.trim()
  return payload
}
