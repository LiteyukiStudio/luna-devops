import { describe, expect, it } from 'vitest'
import { shouldDisplayAIOptions } from './options-visibility'

describe('ai assistant options visibility', () => {
  it('hides suggestions behind the mobile conversation list', () => {
    expect(shouldDisplayAIOptions(false, true)).toBe(false)
    expect(shouldDisplayAIOptions(false, false)).toBe(true)
    expect(shouldDisplayAIOptions(true, true)).toBe(true)
  })
})
