import { describe, expect, it } from 'vitest'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

const validValues = {
  enabled: false,
  accessMode: 'all_authenticated' as const,
  baseUrl: 'https://api.example.com/v1',
  apiKey: '',
  apiKeyConfigured: true,
  webProxyEnabled: false,
  webProxyPool: '',
  webProxyPoolConfigured: false,
  providerTimeoutSeconds: 30,
  maxRequestRetries: 5,
  runTimeoutSeconds: 300,
  agentConcurrentRuns: 2,
  contextInputKTokens: 256,
  contextMaxUncompressedTurnCount: 32,
  contextMaxCompressionTurnsPerCompile: 128,
  contextSummaryInputKTokens: 32,
  contextSummaryMaxOutputTokens: 3000,
  modelMaxOutputTokens: 8192,
  runMaxModelSteps: 64,
  runMaxInputKBytes: 64,
  runNavigateActionTtlSeconds: 120,
  toolsMaxCardRepairAttempts: 5,
  observabilityEnabled: false,
  prometheusUrl: '',
  prometheusToken: '',
  prometheusTokenConfigured: false,
  lokiUrl: '',
  lokiTenantId: '',
  lokiToken: '',
  lokiTokenConfigured: false,
  tempoUrl: '',
  tempoTenantId: '',
  tempoToken: '',
  tempoTokenConfigured: false,
}

describe('aI assistant admin settings', () => {
  it('requires an HTTPS Provider base URL', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, baseUrl: 'http://api.example.com' }).success).toBe(false)
  })

  it('omits a blank secret while preserving the configured key', () => {
    const payload = aiSettingsPayload(validValues)
    expect(payload).not.toHaveProperty('ai.provider.api_key')
    expect(payload).toEqual({
      'ai.assistant.enabled': false,
      'ai.access.mode': 'all_authenticated',
      'ai.provider.base_url': 'https://api.example.com/v1',
      'ai.web.proxy_enabled': false,
      'ai.runtime.provider_timeout_seconds': 30,
      'ai.runtime.max_request_retries': 5,
      'ai.runtime.run_timeout_seconds': 300,
      'ai.runtime.agent_concurrent_runs': 2,
      'ai.runtime.context_input_k_tokens': 256,
      'ai.context.max_uncompressed_turn_count': 32,
      'ai.context.max_compression_turns_per_compile': 128,
      'ai.context.summary_input_k_tokens': 32,
      'ai.context.summary_max_output_tokens': 3000,
      'ai.model.max_output_tokens': 8192,
      'ai.run.max_model_steps': 64,
      'ai.run.max_input_k_bytes': 64,
      'ai.run.navigate_action_ttl_seconds': 120,
      'ai.tools.max_card_repair_attempts': 5,
      'ai.observability.enabled': false,
      'ai.observability.prometheus_url': '',
      'ai.observability.loki_url': '',
      'ai.observability.loki_tenant_id': '',
      'ai.observability.tempo_url': '',
      'ai.observability.tempo_tenant_id': '',
    })
  })

  it('supports restricting assistant access to platform administrators', () => {
    const restricted = { ...validValues, accessMode: 'admins' as const }
    expect(aiSettingsSchema.safeParse(restricted).success).toBe(true)
    expect(aiSettingsPayload(restricted)['ai.access.mode']).toBe('admins')
  })

  it('requires the Provider API key before enabling the assistant', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, enabled: true, apiKeyConfigured: false }).success).toBe(false)
  })

  it('requires all three query URLs before enabling Agent observability', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, observabilityEnabled: true }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({
      ...validValues,
      observabilityEnabled: true,
      prometheusUrl: 'http://prometheus:9090',
      lokiUrl: 'http://loki:3100',
      tempoUrl: 'http://tempo:3200',
    }).success).toBe(true)
  })

  it('rejects unsafe runtime settings', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, runTimeoutSeconds: 10 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, agentConcurrentRuns: 101 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, contextInputKTokens: 32 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, contextInputKTokens: 2049 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, contextInputKTokens: 2048 }).success).toBe(true)
  })

  it('rejects unsafe advanced settings', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, modelMaxOutputTokens: 100 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, runMaxModelSteps: 0 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, runMaxInputKBytes: 8193 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, runNavigateActionTtlSeconds: 5 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, toolsMaxCardRepairAttempts: 0 }).success).toBe(false)
  })

  it('accepts tuned advanced settings within the platform contract', () => {
    const tuned = {
      ...validValues,
      contextMaxUncompressedTurnCount: 48,
      contextMaxCompressionTurnsPerCompile: 160,
      contextSummaryInputKTokens: 48,
      contextSummaryMaxOutputTokens: 4000,
      modelMaxOutputTokens: 12000,
      runMaxModelSteps: 96,
      runMaxInputKBytes: 96,
      runNavigateActionTtlSeconds: 240,
      toolsMaxCardRepairAttempts: 8,
    }
    const parsed = aiSettingsSchema.safeParse(tuned)
    expect(parsed.success).toBe(true)
    if (parsed.success) {
      expect(aiSettingsPayload(parsed.data)['ai.model.max_output_tokens']).toBe(12000)
      expect(aiSettingsPayload(parsed.data)['ai.tools.max_card_repair_attempts']).toBe(8)
    }
  })

  it('accepts authenticated HTTP proxy URLs and keeps configured pools write-only', () => {
    const withProxy = {
      ...validValues,
      webProxyEnabled: true,
      webProxyPool: 'http://user:password@proxy.example.com:888',
    }
    expect(aiSettingsSchema.safeParse(withProxy).success).toBe(true)
    expect(aiSettingsPayload(withProxy)['ai.web.proxy_pool']).toBe(withProxy.webProxyPool)

    const unchanged = { ...withProxy, webProxyPool: '', webProxyPoolConfigured: true }
    expect(aiSettingsSchema.safeParse(unchanged).success).toBe(true)
    expect(aiSettingsPayload(unchanged)).not.toHaveProperty('ai.web.proxy_pool')
  })

  it('rejects unsupported proxy schemes and proxy URL paths', () => {
    for (const webProxyPool of ['socks5://proxy.example.com:1080', 'http://proxy.example.com:888/path']) {
      expect(aiSettingsSchema.safeParse({ ...validValues, webProxyEnabled: true, webProxyPool }).success).toBe(false)
    }
  })
})
