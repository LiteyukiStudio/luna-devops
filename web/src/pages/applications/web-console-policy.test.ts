import { describe, expect, it } from 'vitest'
import { effectiveWebConsoleEnabled, normalizeWebConsoleOverride } from './web-console-policy'

describe('web console policy', () => {
  it('preserves only inherit and further-disable override states', () => {
    expect(normalizeWebConsoleOverride('')).toBeNull()
    expect(normalizeWebConsoleOverride(null)).toBeNull()
    expect(normalizeWebConsoleOverride('true')).toBeNull()
    expect(normalizeWebConsoleOverride(true)).toBeNull()
    expect(normalizeWebConsoleOverride('false')).toBe(false)
    expect(normalizeWebConsoleOverride(false)).toBe(false)
  })

  it('treats the project policy as a hard ceiling', () => {
    expect(effectiveWebConsoleEnabled(true, null)).toBe(true)
    expect(effectiveWebConsoleEnabled(false, null)).toBe(false)
    expect(effectiveWebConsoleEnabled(true, false)).toBe(false)
    expect(effectiveWebConsoleEnabled(false, true)).toBe(false)
    expect(effectiveWebConsoleEnabled(false, false)).toBe(false)
  })
})
