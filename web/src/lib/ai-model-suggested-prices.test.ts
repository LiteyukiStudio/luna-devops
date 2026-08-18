import { describe, expect, it } from 'vitest'
import { findSuggestedModelPrice } from './ai-model-suggested-prices'

describe('findSuggestedModelPrice', () => {
  it('matches exact model names case-insensitively', () => {
    expect(findSuggestedModelPrice('GPT-4o')).toEqual({ input: '250', output: '1000', cachedInput: '125', cachedOutput: '0' })
    expect(findSuggestedModelPrice(' deepseek-chat ')).toEqual({ input: '28', output: '42', cachedInput: '2.8', cachedOutput: '0' })
  })

  it('matches dated variants through prefixes', () => {
    expect(findSuggestedModelPrice('gpt-5.2-2025-12-11')?.input).toBe('175')
    expect(findSuggestedModelPrice('claude-opus-4-5-20251101')?.input).toBe('500')
  })

  it('prefers exact matches over prefixes', () => {
    expect(findSuggestedModelPrice('claude-sonnet-4')?.input).toBe('300')
  })

  it('returns undefined for unknown or empty names', () => {
    expect(findSuggestedModelPrice('my-fine-tuned-model')).toBeUndefined()
    expect(findSuggestedModelPrice('   ')).toBeUndefined()
  })
})
