import { describe, expect, it } from 'vitest'
import { statusRefetchInterval } from './polling'

describe('statusRefetchInterval', () => {
  it.each([
    { active: true, expected: 2_000 },
    { active: false, expected: 30_000 },
  ])('uses $expected ms when active is $active', ({ active, expected }) => {
    expect(statusRefetchInterval(active)).toBe(expected)
  })
})
