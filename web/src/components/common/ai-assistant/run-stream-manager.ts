import type { AIEvent } from '@/api'
import { useEffect, useRef, useState } from 'react'
import { createAIEventSource } from './stream'

export const AI_EVENT_TYPES = [
  'run.started',
  'run.running',
  'run.queued',
  'run.waiting_approval',
  'run.waiting_mfa',
  'run.waiting_input',
  'run.input_required',
  'model.started',
  'content.delta',
  'message.completed',
  'thinking.started',
  'thinking.delta',
  'thinking.completed',
  'item.finalized',
  'tool.started',
  'tool.progress',
  'tool.completed',
  'tool.failed',
  'approval.required',
  'approval.resolved',
  'mfa.required',
  'mfa.resolved',
  'ui.action',
  'model.completed',
  'context.compacted',
  'run.failed',
  'run.completed',
  'run.canceled',
] as const

const terminalRunEvents = new Set(['run.completed', 'run.failed', 'run.canceled'])

export interface AIRunStreamSubscription {
  conversationId: string
  eventsUrl: string
  runId: string
  after: number
}

interface AIRunStreamCallbacks {
  onCapabilitiesChanged?: () => void
  onEvent: (event: AIEvent) => void
  onMalformedEvent?: (subscription: AIRunStreamSubscription) => Promise<void> | void
  onSubscriptionsChange?: (subscriptions: AIRunStreamSubscription[]) => void
}

type EventSourceFactory = (url: string, after: number) => EventSource

interface ActiveStream {
  source: EventSource
  subscription: AIRunStreamSubscription
}

function parseAIEvent(data: string): AIEvent {
  const value = JSON.parse(data) as unknown
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('ai_invalid_stream_event')
  const event = value as Partial<AIEvent>
  if (event.version !== 2
    || typeof event.eventId !== 'string'
    || typeof event.eventSequence !== 'number'
    || !Number.isSafeInteger(event.eventSequence)
    || event.eventSequence < 1
    || typeof event.type !== 'string'
    || typeof event.conversationId !== 'string'
    || typeof event.turnId !== 'string'
    || typeof event.runId !== 'string'
    || typeof event.occurredAt !== 'string'
    || !event.payload
    || typeof event.payload !== 'object'
    || Array.isArray(event.payload)) {
    throw new Error('ai_invalid_stream_event')
  }
  return event as AIEvent
}

export class AIRunStreamManager {
  private readonly streams = new Map<string, ActiveStream>()
  private readonly recoveringRuns = new Set<string>()
  private readonly createSource: EventSourceFactory
  private readonly callbacks: AIRunStreamCallbacks
  private enabled: boolean

  constructor(
    createSource: EventSourceFactory,
    callbacks: AIRunStreamCallbacks,
    enabled = true,
  ) {
    this.createSource = createSource
    this.callbacks = callbacks
    this.enabled = enabled
  }

  setEnabled(enabled: boolean) {
    this.enabled = enabled
    if (!enabled)
      this.closeAll()
  }

  subscriptions() {
    return [...this.streams.values()].map(({ subscription }) => subscription)
  }

  connect = (subscription: AIRunStreamSubscription) => {
    if (!this.enabled || this.streams.has(subscription.runId) || this.recoveringRuns.has(subscription.runId))
      return

    const source = this.createSource(subscription.eventsUrl, subscription.after)
    const receive = (rawEvent: Event) => {
      if (this.streams.get(subscription.runId)?.source !== source)
        return
      try {
        const event = parseAIEvent((rawEvent as MessageEvent<string>).data)
        this.callbacks.onEvent(event)
        if (terminalRunEvents.has(event.type))
          this.disconnect(event.runId)
      }
      catch {
        this.recoveringRuns.add(subscription.runId)
        this.disconnect(subscription.runId)
        void Promise.resolve()
          .then(() => this.callbacks.onMalformedEvent?.(subscription))
          .catch(() => undefined)
          .finally(() => {
            this.recoveringRuns.delete(subscription.runId)
            if (this.enabled)
              this.notify()
          })
      }
    }
    source.onmessage = receive
    AI_EVENT_TYPES.forEach(type => source.addEventListener(type, receive))
    source.addEventListener('ai.capabilities_changed', () => this.callbacks.onCapabilitiesChanged?.())
    this.streams.set(subscription.runId, { source, subscription })
    this.notify()
  }

  syncConversation = (conversationId: string, subscriptions: AIRunStreamSubscription[]) => {
    const desiredRunIds = new Set(subscriptions.map(subscription => subscription.runId))
    for (const [runId, stream] of this.streams) {
      if (stream.subscription.conversationId === conversationId && !desiredRunIds.has(runId))
        this.disconnect(runId)
    }
    subscriptions.forEach(this.connect)
  }

  disconnect = (runId: string) => {
    const stream = this.streams.get(runId)
    if (!stream)
      return
    stream.source.close()
    this.streams.delete(runId)
    this.notify()
  }

  closeAll = () => {
    if (this.streams.size === 0 && this.recoveringRuns.size === 0)
      return
    this.streams.forEach(({ source }) => source.close())
    this.streams.clear()
    this.recoveringRuns.clear()
    this.notify()
  }

  private notify() {
    this.callbacks.onSubscriptionsChange?.(this.subscriptions())
  }
}

interface UseAIRunStreamManagerOptions extends Omit<AIRunStreamCallbacks, 'onSubscriptionsChange'> {
  enabled: boolean
}

export function useAIRunStreamManager(options: UseAIRunStreamManagerOptions) {
  const callbacksRef = useRef(options)
  callbacksRef.current = options
  const [subscriptions, setSubscriptions] = useState<AIRunStreamSubscription[]>([])
  const managerRef = useRef<AIRunStreamManager | null>(null)
  if (!managerRef.current) {
    managerRef.current = new AIRunStreamManager(
      createAIEventSource,
      {
        onEvent: event => callbacksRef.current.onEvent(event),
        onCapabilitiesChanged: () => callbacksRef.current.onCapabilitiesChanged?.(),
        onMalformedEvent: subscription => callbacksRef.current.onMalformedEvent?.(subscription),
        onSubscriptionsChange: setSubscriptions,
      },
      options.enabled,
    )
  }
  const manager = managerRef.current

  useEffect(() => {
    manager.setEnabled(options.enabled)
  }, [manager, options.enabled])
  useEffect(() => () => manager.setEnabled(false), [manager])

  return {
    connect: manager.connect,
    subscriptions,
    syncConversation: manager.syncConversation,
  }
}
