import type { StatusIndicatorState } from './status-tone'
import { describe, expect, it } from 'vitest'
import { statusIndicatorStateFor, statusIndicatorTone } from './status-tone'

describe('statusIndicatorStateFor', () => {
  it('separates waiting work from work that is actively running', () => {
    for (const status of ['queued', 'pending', ' Pending '])
      expect(statusIndicatorStateFor(status)).toBe('queued')
    for (const status of ['running', 'progressing', 'streaming', 'connecting', 'in_progress', 'container-creating', 'deleting', ' Running '])
      expect(statusIndicatorStateFor(status)).toBe('running')
  })

  it('maps settled states through the shared status tones', () => {
    expect(statusIndicatorStateFor('succeeded')).toBe('success')
    expect(statusIndicatorStateFor('ready')).toBe('success')
    expect(statusIndicatorStateFor('failed')).toBe('failure')
    expect(statusIndicatorStateFor('timeout')).toBe('failure')
    expect(statusIndicatorStateFor('degraded')).toBe('warning')
    expect(statusIndicatorStateFor('canceled')).toBe('canceled')
    expect(statusIndicatorStateFor('cancelled')).toBe('canceled')
    expect(statusIndicatorStateFor('disabled')).toBe('neutral')
    expect(statusIndicatorStateFor('info')).toBe('info')
    expect(statusIndicatorStateFor('')).toBe('info')
    expect(statusIndicatorStateFor(undefined)).toBe('info')
  })
})

describe('statusIndicatorTone', () => {
  it('colors every indicator state with a semantic status tone', () => {
    const tones: Record<StatusIndicatorState, string> = {
      canceled: 'neutral',
      failure: 'danger',
      info: 'info',
      neutral: 'neutral',
      queued: 'warning',
      running: 'warning',
      success: 'success',
      warning: 'warning',
    }
    for (const [state, tone] of Object.entries(tones))
      expect(statusIndicatorTone(state as StatusIndicatorState)).toBe(tone)
  })
})
