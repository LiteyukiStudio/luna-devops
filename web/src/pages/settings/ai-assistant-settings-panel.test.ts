import { describe, expect, it } from 'vitest'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

const validValues = {
  enabled: false,
  accessMode: 'all_authenticated' as const,
  baseUrl: 'https://api.example.com/v1',
  apiKey: '',
  apiKeyConfigured: true,
  providerCompatibility: 'auto' as const,
  promptCacheKeyMode: 'auto' as const,
  channelAffinityEnabled: true,
  webProxyEnabled: false,
  webProxyPool: '',
  webProxyPoolConfigured: false,
  providerTimeoutSeconds: 30,
  maxRequestRetries: 5,
  runTimeoutSeconds: 300,
  agentConcurrentRuns: 2,
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
      'ai.provider.compatibility': 'auto',
      'ai.provider.prompt_cache_key_mode': 'auto',
      'ai.provider.channel_affinity_enabled': true,
      'ai.web.proxy_enabled': false,
      'ai.runtime.provider_timeout_seconds': 30,
      'ai.runtime.max_request_retries': 5,
      'ai.runtime.run_timeout_seconds': 300,
      'ai.runtime.agent_concurrent_runs': 2,
      'ai.observability.enabled': false,
      'ai.observability.prometheus_url': '',
      'ai.observability.loki_url': '',
      'ai.observability.loki_tenant_id': '',
      'ai.observability.tempo_url': '',
      'ai.observability.tempo_tenant_id': '',
    })
  })

  it('publishes the channel-affinity switch as an explicit boolean', () => {
    expect(aiSettingsPayload(validValues)['ai.provider.channel_affinity_enabled']).toBe(true)
    expect(aiSettingsPayload({ ...validValues, channelAffinityEnabled: false })['ai.provider.channel_affinity_enabled']).toBe(false)
  })

  it('publishes explicit Provider compatibility and prompt cache key policies', () => {
    const payload = aiSettingsPayload({
      ...validValues,
      providerCompatibility: 'deepseek',
      promptCacheKeyMode: 'disabled',
    })
    expect(payload['ai.provider.compatibility']).toBe('deepseek')
    expect(payload['ai.provider.prompt_cache_key_mode']).toBe('disabled')
    expect(aiSettingsSchema.safeParse({ ...validValues, providerCompatibility: 'unknown' }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, promptCacheKeyMode: 'unknown' }).success).toBe(false)
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
