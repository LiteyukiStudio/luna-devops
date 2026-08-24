import type { AIEvent } from '@/api'
import { describe, expect, it, vi } from 'vitest'
import { AIRunStreamManager } from './run-stream-manager'

class MockEventSource extends EventTarget {
  onmessage: ((this: EventSource, event: MessageEvent) => unknown) | null = null
  readyState = 0
  close = vi.fn()

  open() {
    this.readyState = 1
    this.dispatchEvent(new Event('open'))
  }

  fail({ closed = false } = {}) {
    this.readyState = closed ? 2 : 0
    this.dispatchEvent(new Event('error'))
  }

  heartbeat() {
    this.dispatchEvent(new MessageEvent('stream.heartbeat', { data: JSON.stringify({
      version: 1,
      type: 'stream.heartbeat',
      runId: 'run-1',
      conversationId: 'conversation-1',
      occurredAt: '2026-08-15T00:00:00Z',
    }) }))
  }

  emit(type: string, payload: unknown) {
    this.emitRaw(type, JSON.stringify(payload))
  }

  emitRaw(type: string, data: string) {
    const event = new MessageEvent(type, { data })
    if (type === 'message')
      this.onmessage?.call(this as unknown as EventSource, event)
    else
      this.dispatchEvent(event)
  }
}

function runEvent(type = 'run.running'): AIEvent {
  return {
    version: 2,
    eventId: `event-${type}`,
    eventSequence: 1,
    type,
    conversationId: 'conversation-1',
    turnId: 'turn-1',
    runId: 'run-1',
    occurredAt: '2026-08-15T00:00:00Z',
    payload: {},
  }
}

describe('run stream manager', () => {
  it('owns at most one EventSource for each run', () => {
    const sources: MockEventSource[] = []
    const factory = vi.fn(() => {
      const source = new MockEventSource()
      sources.push(source)
      return source as unknown as EventSource
    })
    const manager = new AIRunStreamManager(factory, { onEvent: vi.fn() })
    const subscription = { conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 }

    manager.connect(subscription)
    manager.connect({ ...subscription, after: 8 })
    manager.syncConversation('conversation-1', [{ ...subscription, after: 12 }])

    expect(factory).toHaveBeenCalledTimes(1)
    expect(sources).toHaveLength(1)
    expect(manager.subscriptions()).toEqual([subscription])
  })

  it('keeps other conversations connected and removes only stale runs in the synchronized conversation', () => {
    const sources: MockEventSource[] = []
    const manager = new AIRunStreamManager(() => {
      const source = new MockEventSource()
      sources.push(source)
      return source as unknown as EventSource
    }, { onEvent: vi.fn() })
    manager.connect({ conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 })
    manager.connect({ conversationId: 'conversation-2', eventsUrl: '/events/run-2', runId: 'run-2', after: 0 })

    manager.syncConversation('conversation-1', [])

    expect(sources[0]!.close).toHaveBeenCalledOnce()
    expect(sources[1]!.close).not.toHaveBeenCalled()
    expect(manager.subscriptions().map(item => item.runId)).toEqual(['run-2'])
  })

  it('forwards terminal events before closing and publishing the remaining subscriptions', () => {
    const source = new MockEventSource()
    const onEvent = vi.fn()
    const onSubscriptionsChange = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      { onEvent, onSubscriptionsChange },
    )
    manager.connect({ conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 })

    source.emit('run.completed', runEvent('run.completed'))

    expect(onEvent).toHaveBeenCalledWith(runEvent('run.completed'))
    expect(source.close).toHaveBeenCalledOnce()
    expect(manager.subscriptions()).toEqual([])
    expect(manager.streamStates()).toEqual([expect.objectContaining({ runId: 'run-1', status: 'terminal' })])
    expect(onSubscriptionsChange).toHaveBeenLastCalledWith([])
  })

  it('publishes connection freshness without sending heartbeats through the business reducer', () => {
    vi.useFakeTimers()
    let now = 1_000
    const source = new MockEventSource()
    const onEvent = vi.fn()
    const onStatesChange = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      { onEvent, onStatesChange },
      true,
      { now: () => now, outputIdleAfterMs: 2_000, stalledAfterMs: 35_000 },
    )
    manager.connect({ conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 })

    expect(manager.streamStates()[0]).toMatchObject({ status: 'connecting' })
    now = 1_100
    source.open()
    expect(manager.streamStates()[0]).toMatchObject({ connectedAt: 1_100, status: 'open' })

    now = 1_200
    source.emit('content.delta', runEvent('content.delta'))
    expect(manager.streamStates()[0]).toMatchObject({ after: 1, lastEventAt: 1_200, status: 'streaming' })
    vi.advanceTimersByTime(2_000)
    expect(manager.streamStates()[0]).toMatchObject({ status: 'open' })

    now = 16_200
    source.heartbeat()
    expect(manager.streamStates()[0]).toMatchObject({ lastHeartbeatAt: 16_200, status: 'open' })
    expect(onEvent).toHaveBeenCalledTimes(1)

    vi.advanceTimersByTime(35_000)
    expect(manager.streamStates()[0]).toMatchObject({ status: 'stalled' })
    expect(onStatesChange).toHaveBeenCalled()
    manager.closeAll()
    vi.useRealTimers()
  })

  it('does not publish a new subscription snapshot for a no-op conversation sync', () => {
    const source = new MockEventSource()
    const onSubscriptionsChange = vi.fn()
    const onStatesChange = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      { onEvent: vi.fn(), onStatesChange, onSubscriptionsChange },
    )
    const subscription = { conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 }
    manager.connect(subscription)
    onSubscriptionsChange.mockClear()
    onStatesChange.mockClear()

    manager.syncConversation('conversation-1', [subscription])

    expect(onSubscriptionsChange).not.toHaveBeenCalled()
    expect(onStatesChange).not.toHaveBeenCalled()
    manager.closeAll()
  })

  it('distinguishes native reconnecting from a closed stream and restores the latest cursor', () => {
    const sources: MockEventSource[] = []
    const factory = vi.fn((_url: string, _after: number) => {
      const source = new MockEventSource()
      sources.push(source)
      return source as unknown as EventSource
    })
    const manager = new AIRunStreamManager(factory, { onEvent: vi.fn() })
    manager.connect({ conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 4 })
    sources[0]!.open()
    sources[0]!.emit('content.delta', { ...runEvent('content.delta'), eventSequence: 5 })
    sources[0]!.fail()
    expect(manager.streamStates()[0]).toMatchObject({ after: 5, status: 'reconnecting' })

    manager.reconnect('run-1')

    expect(sources[0]!.close).toHaveBeenCalledOnce()
    expect(factory).toHaveBeenLastCalledWith('/events/run-1', 5)
    expect(manager.streamStates()[0]).toMatchObject({ after: 5, status: 'connecting' })
    sources[1]!.fail({ closed: true })
    expect(manager.streamStates()[0]).toMatchObject({ status: 'stalled' })
    manager.closeAll()
  })

  it('does not commit a sequence that the reducer rejected and rebuilds from the authoritative cursor', async () => {
    const sources: MockEventSource[] = []
    const factory = vi.fn((_url: string, _after: number) => {
      const source = new MockEventSource()
      sources.push(source)
      return source as unknown as EventSource
    })
    const onSequenceGap = vi.fn(async () => ({ after: 1, terminal: false }))
    let acceptedSequence = 1
    const manager = new AIRunStreamManager(factory, {
      onEvent: (event) => {
        if (event.eventSequence !== acceptedSequence + 1)
          return { accepted: false, desynced: true }
        acceptedSequence = event.eventSequence
        return { accepted: true, desynced: false }
      },
      onSequenceGap,
    })
    manager.connect({ conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 1 })

    sources[0]!.emit('content.delta', { ...runEvent('content.delta'), eventSequence: 3 })

    expect(sources[0]!.close).toHaveBeenCalledOnce()
    expect(manager.streamStates()[0]).toMatchObject({ after: 1, status: 'reconnecting' })
    await vi.waitFor(() => expect(sources).toHaveLength(2))
    expect(onSequenceGap).toHaveBeenCalledWith(
      expect.objectContaining({ after: 1, runId: 'run-1' }),
      expect.objectContaining({ eventSequence: 3 }),
    )
    expect(factory).toHaveBeenLastCalledWith('/events/run-1', 1)
    expect(manager.streamStates()[0]).toMatchObject({ after: 1, status: 'connecting' })

    sources[1]!.emit('run.running', { ...runEvent('run.running'), eventSequence: 2 })
    sources[1]!.emit('run.completed', { ...runEvent('run.completed'), eventSequence: 3 })
    expect(manager.streamStates()[0]).toMatchObject({ after: 3, status: 'terminal' })
    expect(manager.subscriptions()).toEqual([])
    manager.closeAll()
  })

  it('keeps retrying from the last accepted cursor when live replay was lost and converges on an authoritative terminal snapshot', async () => {
    const sources: MockEventSource[] = []
    const recoveries = [
      { after: 1, terminal: false },
      { after: 3, terminal: true },
    ]
    const manager = new AIRunStreamManager(
      () => {
        const source = new MockEventSource()
        sources.push(source)
        return source as unknown as EventSource
      },
      {
        onEvent: () => ({ accepted: false, desynced: true }),
        onSequenceGap: vi.fn(async () => recoveries.shift()),
      },
    )
    manager.connect({ conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 1 })

    sources[0]!.emit('content.delta', { ...runEvent('content.delta'), eventSequence: 3 })
    await vi.waitFor(() => expect(sources).toHaveLength(2))
    expect(manager.streamStates()[0]).toMatchObject({ after: 1, status: 'connecting' })

    sources[1]!.emit('run.completed', { ...runEvent('run.completed'), eventSequence: 3 })
    await vi.waitFor(() => expect(manager.streamStates()[0]).toMatchObject({ after: 3, status: 'terminal' }))

    expect(sources).toHaveLength(2)
    expect(manager.subscriptions()).toEqual([])
    manager.closeAll()
  })

  it('forwards input receipt and closes an interrupted run', () => {
    const source = new MockEventSource()
    const onEvent = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      { onEvent },
    )
    manager.connect({ conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 })

    source.emit('run.input_received', runEvent('run.input_received'))
    expect(onEvent).toHaveBeenLastCalledWith(runEvent('run.input_received'))
    expect(source.close).not.toHaveBeenCalled()

    source.emit('run.interrupted', runEvent('run.interrupted'))
    expect(onEvent).toHaveBeenLastCalledWith(runEvent('run.interrupted'))
    expect(source.close).toHaveBeenCalledOnce()
  })

  it('reports a malformed event once and disconnects its run', async () => {
    const source = new MockEventSource()
    const onMalformedEvent = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      { onEvent: vi.fn(), onMalformedEvent },
    )
    const subscription = { conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 }
    manager.connect(subscription)

    source.emitRaw('message', '{')
    source.emitRaw('message', '{')

    expect(source.close).toHaveBeenCalledOnce()
    await vi.waitFor(() => expect(onMalformedEvent).toHaveBeenCalledOnce())
    expect(onMalformedEvent).toHaveBeenCalledWith(subscription)
  })

  it('recovers from a syntactically valid event that violates the stream contract', async () => {
    const source = new MockEventSource()
    const onMalformedEvent = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      { onEvent: vi.fn(), onMalformedEvent },
    )
    const subscription = { conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 }
    manager.connect(subscription)

    source.emit('run.running', { version: 2, type: 'run.running' })

    expect(source.close).toHaveBeenCalledOnce()
    await vi.waitFor(() => expect(onMalformedEvent).toHaveBeenCalledOnce())
    expect(manager.subscriptions()).toEqual([])
  })

  it('does not advance the cursor when the Timeline reducer rejects a malformed payload', async () => {
    const source = new MockEventSource()
    const onMalformedEvent = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      {
        onEvent: () => {
          throw new Error('ai_invalid_stream_event_payload')
        },
        onMalformedEvent,
      },
    )
    const subscription = { conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 7 }
    manager.connect(subscription)

    source.emit('content.delta', { ...runEvent('content.delta'), eventSequence: 8 })

    expect(source.close).toHaveBeenCalledOnce()
    expect(manager.subscriptions()).toEqual([])
    await vi.waitFor(() => expect(onMalformedEvent).toHaveBeenCalledWith(subscription))
  })

  it.each([
    ['run', { runId: 'run-other' }],
    ['conversation', { conversationId: 'conversation-other' }],
  ])('rejects a frame bound to a different %s before it reaches Timeline state', async (_boundary, mismatch) => {
    const source = new MockEventSource()
    const onEvent = vi.fn()
    const onMalformedEvent = vi.fn()
    const manager = new AIRunStreamManager(
      () => source as unknown as EventSource,
      { onEvent, onMalformedEvent },
    )
    const subscription = { conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 }
    manager.connect(subscription)

    source.emit('run.completed', { ...runEvent('run.completed'), ...mismatch })

    expect(onEvent).not.toHaveBeenCalled()
    expect(source.close).toHaveBeenCalledOnce()
    expect(manager.subscriptions()).toEqual([])
    expect(manager.streamStates()).toEqual([])
    await vi.waitFor(() => expect(onMalformedEvent).toHaveBeenCalledOnce())
    expect(onMalformedEvent).toHaveBeenCalledWith(subscription)
  })

  it('does not reconnect a malformed stream until authoritative recovery settles', async () => {
    const sources: MockEventSource[] = []
    let finishRecovery: (() => void) | undefined
    const manager = new AIRunStreamManager(
      () => {
        const source = new MockEventSource()
        sources.push(source)
        return source as unknown as EventSource
      },
      {
        onEvent: vi.fn(),
        onMalformedEvent: () => new Promise<void>((resolve) => {
          finishRecovery = resolve
        }),
      },
    )
    const subscription = { conversationId: 'conversation-1', eventsUrl: '/events/run-1', runId: 'run-1', after: 0 }
    manager.connect(subscription)
    sources[0]!.emitRaw('message', '{')
    await vi.waitFor(() => expect(finishRecovery).toBeTypeOf('function'))

    manager.connect(subscription)
    expect(sources).toHaveLength(1)

    finishRecovery?.()
    await vi.waitFor(() => {
      manager.connect(subscription)
      expect(sources).toHaveLength(2)
    })
  })
})
