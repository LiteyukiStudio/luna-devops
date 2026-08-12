import { beforeEach, describe, expect, it, vi } from 'vitest'
import { agentObservabilityRangeStorageKey, defaultAgentObservabilityRange, readAgentObservabilityRange, writeAgentObservabilityRange } from './agent-observability-range-preference'

describe('agent observability range preference', () => {
  beforeEach(() => window.localStorage.clear())

  it('persists and restores the last valid range per user', () => {
    writeAgentObservabilityRange('admin-1', '30d')
    writeAgentObservabilityRange('admin-2', '7d')

    expect(readAgentObservabilityRange('admin-1')).toBe('30d')
    expect(readAgentObservabilityRange('admin-2')).toBe('7d')
    expect(window.localStorage.getItem(agentObservabilityRangeStorageKey('admin-1'))).toBe('30d')
  })

  it('falls back to the default for absent, invalid, or inaccessible storage', () => {
    window.localStorage.setItem(agentObservabilityRangeStorageKey('admin-1'), 'forever')
    const inaccessible = {
      getItem: vi.fn(() => {
        throw new Error('blocked')
      }),
      setItem: vi.fn(() => {
        throw new Error('blocked')
      }),
    } as unknown as Storage

    expect(readAgentObservabilityRange('missing')).toBe(defaultAgentObservabilityRange)
    expect(readAgentObservabilityRange('admin-1')).toBe(defaultAgentObservabilityRange)
    expect(readAgentObservabilityRange('admin-1', inaccessible)).toBe(defaultAgentObservabilityRange)
    expect(() => writeAgentObservabilityRange('admin-1', '1y', inaccessible)).not.toThrow()
  })
})
