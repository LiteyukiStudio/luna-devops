import { describe, expect, it } from 'vitest'
import { aiConversationModelKey, resolveAIConversationModel } from './model-selection'

const models = [
  { id: 'aimod_fast', name: 'Fast', maxContextTokens: 128_000, maxOutputTokens: 16_000 },
  { id: 'aimod_deep', name: 'Deep', maxContextTokens: 256_000, maxOutputTokens: 32_000 },
]

describe('ai conversation model selection', () => {
  it('keeps user selections isolated by conversation', () => {
    const overrides = {
      [aiConversationModelKey('aicnv_a')]: 'aimod_deep',
      [aiConversationModelKey('aicnv_b')]: 'aimod_fast',
    }

    expect(resolveAIConversationModel(models, 'aicnv_a', 'aimod_fast', overrides)?.id).toBe('aimod_deep')
    expect(resolveAIConversationModel(models, 'aicnv_b', 'aimod_deep', overrides)?.id).toBe('aimod_fast')
  })

  it('uses the persisted conversation preference and safely falls back for legacy conversations', () => {
    expect(resolveAIConversationModel(models, 'aicnv_saved', 'aimod_deep', {})?.id).toBe('aimod_deep')
    expect(resolveAIConversationModel(models, 'aicnv_legacy', undefined, {})?.id).toBe('aimod_fast')
  })
})
