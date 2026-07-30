import { describe, expect, it } from 'vitest'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

const validValues = {
  enabled: false,
  baseUrl: 'https://api.example.com/v1',
  apiKey: '',
  apiKeyConfigured: true,
  model: 'model-1',
  providerTimeoutSeconds: 30,
  runTimeoutSeconds: 300,
  agentConcurrentRuns: 2,
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
      'ai.provider.base_url': 'https://api.example.com/v1',
      'ai.provider.default_model': 'model-1',
      'ai.runtime.provider_timeout_seconds': 30,
      'ai.runtime.run_timeout_seconds': 300,
      'ai.runtime.agent_concurrent_runs': 2,
    })
  })

  it('requires all three model settings before enabling the assistant', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, enabled: true, apiKeyConfigured: false }).success).toBe(false)
  })

  it('rejects unsafe runtime settings', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, runTimeoutSeconds: 10 }).success).toBe(false)
    expect(aiSettingsSchema.safeParse({ ...validValues, agentConcurrentRuns: 100 }).success).toBe(false)
  })
})
