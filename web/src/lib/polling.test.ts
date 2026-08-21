import { describe, expect, it } from 'vitest'
import { IDLE_STATUS_REFETCH_INTERVAL_MS, statusRefetchInterval, WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from './polling'

describe('statusRefetchInterval', () => {
  it('keeps active workflows responsive', () => {
    expect(statusRefetchInterval(true)).toBe(WORKFLOW_STATUS_REFETCH_INTERVAL_MS)
  })

  it('backs off idle observations', () => {
    expect(statusRefetchInterval(false)).toBe(IDLE_STATUS_REFETCH_INTERVAL_MS)
  })
})
