import { describe, expect, it } from 'vitest'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

const validValues = {
  enabled: false,
  baseUrl: 'https://api.example.com/v1',
  apiKey: '',
  apiKeyConfigured: true,
  model: 'model-1',
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
    })
  })

  it('requires all three model settings before enabling the assistant', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, enabled: true, apiKeyConfigured: false }).success).toBe(false)
  })
})
