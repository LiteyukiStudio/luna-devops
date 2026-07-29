import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAIEventSource } from './ai-assistant-stream'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('aI assistant event stream', () => {
  it('forces stream mode and preserves the recovery cursor', () => {
    const eventSource = vi.fn()
    vi.stubGlobal('EventSource', eventSource)

    createAIEventSource('/api/v1/ai/runs/run-1/events?stream=false', 42, 'https://luna.example')

    expect(eventSource).toHaveBeenCalledWith(
      'https://luna.example/api/v1/ai/runs/run-1/events?stream=true&after=42',
      { withCredentials: true },
    )
  })
})
