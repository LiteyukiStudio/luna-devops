import { describe, expect, it } from 'vitest'
import {
  PENDING_UI_ACTIONS_POLL_INTERVAL_MS,
  PENDING_UI_ACTIONS_RECOVERY_INTERVAL_MS,
  pendingUIActionsPollInterval,
} from './pending-ui-actions-query'

describe('pending AI UI action polling', () => {
  it('uses the normal interval while Agent is available', () => {
    expect(pendingUIActionsPollInterval({ items: [] })).toBe(PENDING_UI_ACTIONS_POLL_INTERVAL_MS)
  })

  it('backs off while Agent is temporarily unavailable', () => {
    expect(pendingUIActionsPollInterval({ items: [], agentAvailable: false, retryAfterSeconds: 45 })).toBe(45_000)
  })

  it('continues low-frequency recovery after an unexpected request error', () => {
    expect(pendingUIActionsPollInterval(undefined, true)).toBe(PENDING_UI_ACTIONS_RECOVERY_INTERVAL_MS)
  })

  it('clamps invalid recovery hints to conservative bounds', () => {
    expect(pendingUIActionsPollInterval({ items: [], agentAvailable: false, retryAfterSeconds: 1 })).toBe(5_000)
    expect(pendingUIActionsPollInterval({ items: [], agentAvailable: false, retryAfterSeconds: 3_600 })).toBe(300_000)
    expect(pendingUIActionsPollInterval({ items: [], agentAvailable: false })).toBe(PENDING_UI_ACTIONS_RECOVERY_INTERVAL_MS)
  })
})
