import { describe, expect, it } from 'vitest'
import { ApiError } from '@/api'
import i18next from '@/i18n'
import { aiModelFormSchema, modelFormValues, modelSaveErrorMessage } from './ai-model-management-utils'

const validModel = {
  id: 'aimod_test',
  name: 'test-model',
  maxContextTokens: 4096,
  maxOutputTokens: 512,
  inputCreditsPerMillion: '1.25',
  outputCreditsPerMillion: '2.5',
  cachedInputCreditsPerMillion: '0.5',
  enabled: true,
  createdAt: '2026-08-17T00:00:00Z',
  updatedAt: '2026-08-17T00:00:00Z',
}

describe('ai model management contract', () => {
  it('validates capability ranges and requires output below context', () => {
    expect(aiModelFormSchema.safeParse(validModel).success).toBe(true)
    expect(aiModelFormSchema.safeParse({ ...validModel, maxContextTokens: 4095 }).success).toBe(false)
    expect(aiModelFormSchema.safeParse({ ...validModel, maxOutputTokens: 4096 }).success).toBe(false)
    expect(aiModelFormSchema.safeParse({ ...validModel, maxOutputTokens: 262145 }).success).toBe(false)
  })

  it('roundtrips capability and official prompt/completion price fields when editing', () => {
    expect(modelFormValues(validModel)).toEqual({
      name: validModel.name,
      maxContextTokens: validModel.maxContextTokens,
      maxOutputTokens: validModel.maxOutputTokens,
      inputCreditsPerMillion: validModel.inputCreditsPerMillion,
      outputCreditsPerMillion: validModel.outputCreditsPerMillion,
      cachedInputCreditsPerMillion: validModel.cachedInputCreditsPerMillion,
      enabled: true,
    })
  })

  it.each([
    ['ai.model_context_limit_invalid', 'settings.ai.models.errors.contextLimit'],
    ['ai.model_output_limit_invalid', 'settings.ai.models.errors.outputLimit'],
  ])('maps %s to a stable localized management error', (code, key) => {
    const error = new ApiError('raw backend error', { code, path: '/ai/models', status: 422 })
    expect(modelSaveErrorMessage(error, 'fallback', value => value)).toBe(key)
    expect(i18next.exists(key)).toBe(true)
  })
})
