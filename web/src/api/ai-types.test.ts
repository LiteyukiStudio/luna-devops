import { describe, expect, it } from 'vitest'
import { isUsableAICapabilities } from './ai-types'

describe('aI capabilities fail-closed validation', () => {
  it('rejects an incomplete available response', () => {
    expect(isUsableAICapabilities({ available: true })).toBe(false)
  })

  it('accepts a complete available response', () => {
    expect(isUsableAICapabilities({
      available: true,
      reasonCode: null,
      features: { streaming: true, approvals: true, stepUpMFA: true, uiActions: true, longTermMemory: false },
      limits: { maxInputBytes: 32768, maxConcurrentRuns: 2 },
    })).toBe(true)
  })
})
