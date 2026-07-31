import { afterEach, describe, expect, it, vi } from 'vitest'
import { createAIEventSource } from './stream'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('aI assistant event stream', () => {
  it('forces stream mode and preserves the recovery cursor', () => {
    const constructor = vi.fn()
    class MockEventSource {
      readyState = 0
      addEventListener = vi.fn()
      close = vi.fn()

      constructor(url: string | URL, options?: EventSourceInit) {
        constructor(url, options)
      }
    }
    vi.stubGlobal('EventSource', MockEventSource)

    createAIEventSource('/api/v1/ai/runs/run-1/events?stream=false', 42, 'https://luna.example')

    expect(constructor).toHaveBeenCalledWith(
      'https://luna.example/api/v1/ai/runs/run-1/events?stream=true&after=42',
      { withCredentials: true },
    )
  })
})
