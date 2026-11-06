import { describe, expect, it } from 'vitest'
import { latestByDate } from './application-facts'

describe('application facts', () => {
  it('picks the newest item by date', () => {
    const items = [
      { createdAt: '2026-10-01T00:00:00Z', id: 'older' },
      { createdAt: '2026-10-03T00:00:00Z', id: 'newest' },
      { createdAt: '2026-10-02T00:00:00Z', id: 'newer' },
    ]
    expect(latestByDate(items, item => item.createdAt)?.id).toBe('newest')
    expect(latestByDate([], () => undefined)).toBeUndefined()
  })
})
