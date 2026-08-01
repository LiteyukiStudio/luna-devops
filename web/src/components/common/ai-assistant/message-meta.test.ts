import { describe, expect, it } from 'vitest'
import { formatMessageTime } from './message-time'

describe('ai message timestamp', () => {
  const now = new Date(2026, 7, 1, 15, 30)

  it('shows only the time for messages sent today', () => {
    expect(formatMessageTime(new Date(2026, 7, 1, 13, 11).toISOString(), 'zh-CN', now).label).toBe('13:11')
  })

  it('adds the date for messages sent earlier in the same year', () => {
    const label = formatMessageTime(new Date(2026, 6, 31, 13, 11).toISOString(), 'zh-CN', now).label

    expect(label).toContain('07/31')
    expect(label).not.toContain('2026')
  })

  it('adds the year for messages sent in another year', () => {
    const label = formatMessageTime(new Date(2025, 6, 31, 13, 11).toISOString(), 'zh-CN', now).label

    expect(label).toContain('2025')
    expect(label).toContain('07/31')
  })
})
