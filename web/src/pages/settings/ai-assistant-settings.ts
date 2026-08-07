import i18next from 'i18next'
import { z } from 'zod'

const observabilityUrl = z.union([z.literal(''), z.url().refine((value) => {
  const parsed = new URL(value)
  return ['http:', 'https:'].includes(parsed.protocol) && !parsed.username && !parsed.password && !parsed.search && !parsed.hash
}, { message: i18next.t('settings.ai.observabilityUrlInvalid') })])

function boundedInt(min: number, max: number, messageKey: string) {
  const message = i18next.t(messageKey, { min, max })
  return z.number({ message }).int({ message }).min(min, { message }).max(max, { message })
}

function boundedRatio(min: number, max: number, messageKey: string) {
  const message = i18next.t(messageKey, { min, max })
  return z.number({ message }).min(min, { message }).max(max, { message })
}

export const aiSettingsSchema = z.object({
  enabled: z.boolean(),
  accessMode: z.enum(['all_authenticated', 'admins']),
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
    .max(100, { message: i18next.t('settings.ai.agentConcurrentRunsInvalid') }),
  contextInputKTokens: z.number({ message: i18next.t('settings.ai.contextInputBudgetInvalid') })
    .int({ message: i18next.t('settings.ai.contextInputBudgetInvalid') })
    .min(64, { message: i18next.t('settings.ai.contextInputBudgetInvalid') })
    .max(1024, { message: i18next.t('settings.ai.contextInputBudgetInvalid') }),
  // 高级设置：上下文与压缩
  contextCompressionTriggerRatio: boundedRatio(0.5, 0.95, 'settings.ai.advancedNumberInvalid'),
  contextCompressionTargetRatio: boundedRatio(0.1, 0.8, 'settings.ai.advancedNumberInvalid'),
  contextRecentTurnCount: boundedInt(1, 16, 'settings.ai.advancedNumberInvalid'),
  contextMaxRecentTurnCount: boundedInt(2, 32, 'settings.ai.advancedNumberInvalid'),
  contextMaxUncompressedTurnCount: boundedInt(4, 200, 'settings.ai.advancedNumberInvalid'),
  contextMaxCompressionTurnsPerCompile: boundedInt(8, 500, 'settings.ai.advancedNumberInvalid'),
  contextSummaryInputKTokens: boundedInt(4, 128, 'settings.ai.advancedNumberInvalid'),
  contextSummaryMaxOutputTokens: boundedInt(200, 8000, 'settings.ai.advancedNumberInvalid'),
  contextHistoricalToolKTokens: boundedInt(1, 64, 'settings.ai.advancedNumberInvalid'),
  // 高级设置：模型与执行
  modelMaxOutputTokens: boundedInt(256, 16384, 'settings.ai.advancedNumberInvalid'),
  runMaxModelSteps: boundedInt(1, 200, 'settings.ai.advancedNumberInvalid'),
  runMaxInputKBytes: boundedInt(8, 1024, 'settings.ai.advancedNumberInvalid'),
  runNavigateActionTtlSeconds: boundedInt(10, 600, 'settings.ai.advancedNumberInvalid'),
  // 高级设置：工具结果与卡片
  toolsResultPayloadKBytes: boundedInt(4, 512, 'settings.ai.advancedNumberInvalid'),
  toolsMaxCardRepairAttempts: boundedInt(1, 10, 'settings.ai.advancedNumberInvalid'),
  observabilityEnabled: z.boolean(),
  prometheusUrl: observabilityUrl,
  prometheusToken: z.string(),
  prometheusTokenConfigured: z.boolean(),
  lokiUrl: observabilityUrl,
  lokiTenantId: z.string(),
  lokiToken: z.string(),
  lokiTokenConfigured: z.boolean(),
  tempoUrl: observabilityUrl,
  tempoTenantId: z.string(),
  tempoToken: z.string(),
  tempoTokenConfigured: z.boolean(),
}).superRefine((value, context) => {
  if (value.enabled) {
    if (!value.baseUrl)
      context.addIssue({ code: 'custom', path: ['baseUrl'], message: i18next.t('settings.ai.baseUrlRequired') })
    if (!value.model.trim())
      context.addIssue({ code: 'custom', path: ['model'], message: i18next.t('settings.ai.modelRequired') })
    if (!value.apiKey.trim() && !value.apiKeyConfigured)
      context.addIssue({ code: 'custom', path: ['apiKey'], message: i18next.t('settings.ai.apiKeyRequired') })
  }
  if (value.webProxyEnabled && !value.webProxyPool.trim() && !value.webProxyPoolConfigured)
    context.addIssue({ code: 'custom', path: ['webProxyPool'], message: i18next.t('settings.ai.webProxyPoolRequired') })
  if (value.observabilityEnabled) {
    for (const field of ['prometheusUrl', 'lokiUrl', 'tempoUrl'] as const) {
      if (!value[field])
        context.addIssue({ code: 'custom', path: [field], message: i18next.t('settings.ai.observabilityUrlRequired') })
    }
  }
  if (value.contextCompressionTriggerRatio <= value.contextCompressionTargetRatio)
    context.addIssue({ code: 'custom', path: ['contextCompressionTriggerRatio'], message: i18next.t('settings.ai.contextRatioInvalid') })
  if (value.contextRecentTurnCount > value.contextMaxRecentTurnCount)
    context.addIssue({ code: 'custom', path: ['contextRecentTurnCount'], message: i18next.t('settings.ai.contextRecentTurnInvalid') })
})

export type AISettingsFormValues = z.infer<typeof aiSettingsSchema>

export function aiSettingsPayload(values: AISettingsFormValues) {
  const payload: Record<string, unknown> = {
    'ai.assistant.enabled': values.enabled,
    'ai.access.mode': values.accessMode,
    'ai.provider.base_url': values.baseUrl.trim(),
    'ai.provider.default_model': values.model.trim(),
    'ai.web.proxy_enabled': values.webProxyEnabled,
    'ai.runtime.provider_timeout_seconds': values.providerTimeoutSeconds,
    'ai.runtime.run_timeout_seconds': values.runTimeoutSeconds,
    'ai.runtime.agent_concurrent_runs': values.agentConcurrentRuns,
    'ai.runtime.context_input_k_tokens': values.contextInputKTokens,
    'ai.context.compression_trigger_ratio': values.contextCompressionTriggerRatio,
    'ai.context.compression_target_ratio': values.contextCompressionTargetRatio,
    'ai.context.recent_turn_count': values.contextRecentTurnCount,
    'ai.context.max_recent_turn_count': values.contextMaxRecentTurnCount,
    'ai.context.max_uncompressed_turn_count': values.contextMaxUncompressedTurnCount,
    'ai.context.max_compression_turns_per_compile': values.contextMaxCompressionTurnsPerCompile,
    'ai.context.summary_input_k_tokens': values.contextSummaryInputKTokens,
    'ai.context.summary_max_output_tokens': values.contextSummaryMaxOutputTokens,
    'ai.context.historical_tool_k_tokens': values.contextHistoricalToolKTokens,
    'ai.model.max_output_tokens': values.modelMaxOutputTokens,
    'ai.run.max_model_steps': values.runMaxModelSteps,
    'ai.run.max_input_k_bytes': values.runMaxInputKBytes,
    'ai.run.navigate_action_ttl_seconds': values.runNavigateActionTtlSeconds,
    'ai.tools.result_payload_k_bytes': values.toolsResultPayloadKBytes,
    'ai.tools.max_card_repair_attempts': values.toolsMaxCardRepairAttempts,
    'ai.observability.enabled': values.observabilityEnabled,
    'ai.observability.prometheus_url': values.prometheusUrl.trim(),
    'ai.observability.loki_url': values.lokiUrl.trim(),
    'ai.observability.loki_tenant_id': values.lokiTenantId.trim(),
    'ai.observability.tempo_url': values.tempoUrl.trim(),
    'ai.observability.tempo_tenant_id': values.tempoTenantId.trim(),
  }
  if (values.apiKey.trim())
    payload['ai.provider.api_key'] = values.apiKey.trim()
  if (values.webProxyPool.trim())
    payload['ai.web.proxy_pool'] = values.webProxyPool.trim()
  if (values.prometheusToken.trim())
    payload['ai.observability.prometheus_token'] = values.prometheusToken.trim()
  if (values.lokiToken.trim())
    payload['ai.observability.loki_token'] = values.lokiToken.trim()
  if (values.tempoToken.trim())
    payload['ai.observability.tempo_token'] = values.tempoToken.trim()
  return payload
}
