import type { AIEvent } from '@/api'
import { describe, expect, it, vi } from 'vitest'
import { AIRunStreamManager } from './run-stream-manager'

class MockEventSource extends EventTarget {
  onmessage: ((this: EventSource, event: MessageEvent) => unknown) | null = null
  close = vi.fn()

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
    expect(onSubscriptionsChange).toHaveBeenLastCalledWith([])
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
