import { describe, expect, it } from 'vitest'
import { parseResultVisibility, withResultVisibility } from '@/lib/result-visibility'

describe('result visibility URL contract', () => {
  it('defaults unknown or omitted values to related', () => {
    expect(parseResultVisibility(undefined)).toBe('related')
    expect(parseResultVisibility(null)).toBe('related')
    expect(parseResultVisibility('mine')).toBe('related')
  })

  it('preserves an explicit all value', () => {
    expect(parseResultVisibility('all')).toBe('all')
  })

  it('adds visibility without losing filters or hashes', () => {
    expect(withResultVisibility('/events?categories=build#details', 'all'))
      .toBe('/events?categories=build&visibility=all#details')
  })
})
