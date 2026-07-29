import { describe, expect, it } from 'vitest'
import { aiSettingsPayload, aiSettingsSchema } from './ai-assistant-settings'

const validValues = {
  enabled: false,
  providerType: 'openai-compatible' as const,
  baseUrl: 'https://api.example.com/v1',
  apiKey: '',
  defaultModel: 'model-1',
  fallbackModel: '',
  modelPricing: '[]',
  accessMode: 'admins' as const,
  userIds: '',
  projectIds: '',
  userConcurrentRuns: 2,
  userDailyTokens: 200000,
  projectConcurrentRuns: 5,
  runMaxToolCalls: 20,
  platformDailyCostHard: 10,
  platformDailyCostSoft: 5,
  conversationDays: 90,
  runEventDays: 30,
  checkpointDays: 7,
}

describe('aI assistant admin settings', () => {
  it('requires an HTTPS Provider base URL', () => {
    expect(aiSettingsSchema.safeParse({ ...validValues, baseUrl: 'http://api.example.com' }).success).toBe(false)
  })

  it('omits a blank secret and normalizes allowlists', () => {
    const payload = aiSettingsPayload({ ...validValues, userIds: 'usr_1\nusr_2' })
    expect(payload).not.toHaveProperty('ai.provider.api_key')
    expect(payload['ai.access.user_ids']).toEqual(['usr_1', 'usr_2'])
  })
})
