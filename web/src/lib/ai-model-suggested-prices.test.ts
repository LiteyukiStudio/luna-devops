import { describe, expect, it } from 'vitest'
import { findSuggestedModelPreset, listSuggestedModelPresets } from './ai-model-suggested-prices'

describe('findSuggestedModelPreset', () => {
  it('returns prices together with capability limits', () => {
    expect(findSuggestedModelPreset('deepseek-chat')).toEqual({
      displayName: 'deepseek-chat',
      prices: { input: '28', output: '42', cachedInput: '2.8' },
      maxContextTokens: 131072,
      maxOutputTokens: 8192,
    })
  })

  it('uses the canonical display name for dated variants', () => {
    expect(findSuggestedModelPreset('gpt-5.2-2025-12-11')?.displayName).toBe('gpt-5.2')
  })

  it('keeps capability limits within the form bounds', () => {
    for (const preset of listSuggestedModelPresets()) {
      expect(preset.maxContextTokens).toBeGreaterThanOrEqual(4096)
      expect(preset.maxContextTokens).toBeLessThanOrEqual(2097152)
      expect(preset.maxOutputTokens).toBeGreaterThanOrEqual(256)
      expect(preset.maxOutputTokens).toBeLessThanOrEqual(262144)
      expect(preset.maxOutputTokens).toBeLessThan(preset.maxContextTokens)
    }
  })

  it('returns unique display names for the selectable preset list', () => {
    const names = listSuggestedModelPresets().map(preset => preset.displayName)
    expect(new Set(names).size).toBe(names.length)
  })
})
