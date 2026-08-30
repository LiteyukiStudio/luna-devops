import { describe, expect, it } from 'vitest'
import { buildRunIdFromHash } from './application-build-navigation'

describe('build run deep-link navigation', () => {
  it('reads the build run identifier from the application tab hash', () => {
    expect(buildRunIdFromHash('#tab=builds&buildRunId=bldr_123')).toBe('bldr_123')
  })

  it('returns an empty identifier for a regular builds tab link', () => {
    expect(buildRunIdFromHash('#tab=builds')).toBe('')
  })
})
