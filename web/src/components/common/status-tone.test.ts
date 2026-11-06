import { describe, expect, it } from 'vitest'
import { isInProgressStatus } from './status-tone'

describe('isInProgressStatus', () => {
  it('treats queued and running work as in progress', () => {
    for (const status of ['running', 'queued', 'pending', 'progressing', 'streaming', 'connecting', 'in_progress', ' Running '])
      expect(isInProgressStatus(status)).toBe(true)
  })

  it('treats terminal and idle states as settled', () => {
    for (const status of ['succeeded', 'failed', 'ready', 'canceled', 'disabled', '', null, undefined])
      expect(isInProgressStatus(status)).toBe(false)
  })
})
