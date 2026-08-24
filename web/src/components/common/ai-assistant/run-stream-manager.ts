import type { AIEvent } from '@/api'
import { useEffect, useRef, useState } from 'react'
import { createAIEventSource } from './stream'

export const AI_EVENT_TYPES = [
  'run.started',
  'run.running',
  'run.queued',
  'run.waiting_approval',
  'run.waiting_input',
  'run.input_required',
  'run.input_received',
  'conversation.title.updated',
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
  'ui.action',
  'model.completed',
  'context.compacted',
  'run.failed',
  'run.completed',
  'run.canceled',
  'run.interrupted',
] as const

const terminalRunEvents = new Set(['run.completed', 'run.failed', 'run.canceled', 'run.interrupted'])

export type AIRunStreamStatus = 'connecting' | 'open' | 'streaming' | 'stalled' | 'reconnecting' | 'terminal'

export interface AIRunStreamState extends AIRunStreamSubscription {
  status: AIRunStreamStatus
  connectedAt?: number
  lastEventAt?: number
  lastHeartbeatAt?: number
}

export interface AIRunStreamSubscription {
  conversationId: string
  eventsUrl: string
  runId: string
  after: number
}

interface AIRunStreamCallbacks {
  onCapabilitiesChanged?: () => void
  onEvent: (event: AIEvent) => AIRunStreamEventResult | void
  onMalformedEvent?: (subscription: AIRunStreamSubscription) => Promise<void> | void
  onSequenceGap?: (subscription: AIRunStreamSubscription, event: AIEvent) => Promise<AIRunStreamRecovery | undefined>
  onStatesChange?: (states: AIRunStreamState[]) => void
  onSubscriptionsChange?: (subscriptions: AIRunStreamSubscription[]) => void
}

export interface AIRunStreamEventResult {
  accepted: boolean
  desynced: boolean
}

export interface AIRunStreamRecovery {
  after: number
  terminal: boolean
}

type EventSourceFactory = (url: string, after: number) => EventSource

interface ActiveStream {
  source: EventSource
  state: AIRunStreamState
  outputIdleTimer?: ReturnType<typeof setTimeout>
  stalledTimer?: ReturnType<typeof setTimeout>
}

interface AIRunStreamManagerOptions {
  now?: () => number
  outputIdleAfterMs?: number
  stalledAfterMs?: number
}

const defaultOutputIdleAfterMs = 2_000
// Agent 默认每 15 秒发送一次心跳；预留超过两个心跳周期的恢复窗口。
const defaultStalledAfterMs = 35_000

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
  private readonly states = new Map<string, AIRunStreamState>()
  private readonly recoveringRuns = new Set<string>()
  private readonly createSource: EventSourceFactory
  private readonly callbacks: AIRunStreamCallbacks
  private readonly now: () => number
  private readonly outputIdleAfterMs: number
  private readonly stalledAfterMs: number
  private enabled: boolean

  constructor(
    createSource: EventSourceFactory,
    callbacks: AIRunStreamCallbacks,
    enabled = true,
    options: AIRunStreamManagerOptions = {},
  ) {
    this.createSource = createSource
    this.callbacks = callbacks
    this.enabled = enabled
    this.now = options.now ?? Date.now
    this.outputIdleAfterMs = options.outputIdleAfterMs ?? defaultOutputIdleAfterMs
    this.stalledAfterMs = options.stalledAfterMs ?? defaultStalledAfterMs
  }

  setEnabled(enabled: boolean) {
    this.enabled = enabled
    if (!enabled)
      this.closeAll()
  }

  subscriptions() {
    return [...this.streams.values()].map(({ state }) => this.subscriptionFromState(state))
  }

  streamStates() {
    return [...this.states.values()]
  }

  connect = (subscription: AIRunStreamSubscription) => {
    if (!this.enabled || this.streams.has(subscription.runId) || this.recoveringRuns.has(subscription.runId))
      return

    const source = this.createSource(subscription.eventsUrl, subscription.after)
    const state: AIRunStreamState = { ...subscription, status: 'connecting' }
    const stream: ActiveStream = { source, state }
    this.streams.set(subscription.runId, stream)
    this.states.set(subscription.runId, state)
    this.scheduleStalled(stream)

    source.addEventListener('open', () => {
      if (!this.isCurrent(subscription.runId, source))
        return
      this.updateState(stream, {
        connectedAt: this.now(),
        status: 'open',
      })
      this.scheduleStalled(stream)
    })
    source.addEventListener('error', () => {
      if (!this.isCurrent(subscription.runId, source))
        return
      this.updateState(stream, {
        status: source.readyState === 2 ? 'stalled' : 'reconnecting',
      })
      this.scheduleStalled(stream)
    })
    // SSE 注释不会透传给 EventSource。Agent 改为显式心跳事件后，此处即可
    // 区分“Provider 暂无输出”和“连接已经失活”，心跳不进入业务 reducer。
    source.addEventListener('stream.heartbeat', () => {
      if (!this.isCurrent(subscription.runId, source))
        return
      this.updateState(stream, {
        lastHeartbeatAt: this.now(),
        status: stream.state.status === 'streaming' ? 'streaming' : 'open',
      })
      this.scheduleStalled(stream)
    })
    const receive = (rawEvent: Event) => {
      if (!this.isCurrent(subscription.runId, source))
        return
      try {
        const event = parseAIEvent((rawEvent as MessageEvent<string>).data)
        if (event.runId !== subscription.runId || event.conversationId !== subscription.conversationId)
          throw new Error('ai_stream_subscription_mismatch')
        const receivedAt = this.now()
        const result = this.callbacks.onEvent(event)
        const accepted = result?.accepted ?? true
        this.updateState(stream, {
          after: accepted ? Math.max(stream.state.after, event.eventSequence) : stream.state.after,
          lastEventAt: receivedAt,
          status: terminalRunEvents.has(event.type) && accepted ? 'terminal' : 'streaming',
        })
        if (result?.desynced) {
          this.recoverSequenceGap(stream, event)
          return
        }
        if (terminalRunEvents.has(event.type) && accepted) {
          this.finish(event.runId)
          return
        }
        this.scheduleOutputIdle(stream)
        this.scheduleStalled(stream)
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
    this.notify()
  }

  syncConversation = (conversationId: string, subscriptions: AIRunStreamSubscription[]) => {
    const desiredRunIds = new Set(subscriptions.map(subscription => subscription.runId))
    let removedState = false
    for (const [runId, stream] of this.streams) {
      if (stream.state.conversationId === conversationId && !desiredRunIds.has(runId))
        this.disconnect(runId)
    }
    for (const [runId, state] of this.states) {
      if (state.conversationId === conversationId && !desiredRunIds.has(runId)) {
        this.states.delete(runId)
        removedState = true
      }
    }
    subscriptions.forEach(this.connect)
    if (removedState)
      this.notify()
  }

  reconnect = (runId: string) => {
    const stream = this.streams.get(runId)
    if (!stream
      || !['reconnecting', 'stalled'].includes(stream.state.status)
      || this.recoveringRuns.has(runId)) {
      return
    }
    const subscription = this.subscriptionFromState(stream.state)
    this.disconnect(runId)
    this.connect(subscription)
  }

  disconnect = (runId: string) => {
    const stream = this.streams.get(runId)
    if (!stream)
      return
    stream.source.close()
    this.clearTimers(stream)
    this.streams.delete(runId)
    this.states.delete(runId)
    this.notify()
  }

  closeAll = () => {
    if (this.streams.size === 0 && this.states.size === 0 && this.recoveringRuns.size === 0)
      return
    this.streams.forEach((stream) => {
      stream.source.close()
      this.clearTimers(stream)
    })
    this.streams.clear()
    this.states.clear()
    this.recoveringRuns.clear()
    this.notify()
  }

  private finish(runId: string) {
    const stream = this.streams.get(runId)
    if (!stream)
      return
    stream.source.close()
    this.clearTimers(stream)
    this.streams.delete(runId)
    this.states.set(runId, { ...stream.state, status: 'terminal' })
    this.notify()
  }

  private recoverSequenceGap(stream: ActiveStream, event: AIEvent) {
    const runId = stream.state.runId
    if (this.recoveringRuns.has(runId))
      return
    this.recoveringRuns.add(runId)
    stream.source.close()
    this.clearTimers(stream)
    this.streams.delete(runId)
    this.states.set(runId, { ...stream.state, status: 'reconnecting' })
    this.notify()
    const acceptedAfter = stream.state.after
    const subscription = this.subscriptionFromState(stream.state)
    void Promise.resolve()
      .then(() => this.callbacks.onSequenceGap?.(subscription, event))
      .catch(() => undefined)
      .then((recovery) => {
        this.recoveringRuns.delete(runId)
        if (!this.enabled || !this.states.has(runId))
          return
        if (recovery?.terminal) {
          this.states.set(runId, {
            ...stream.state,
            after: Math.max(acceptedAfter, recovery.after),
            status: 'terminal',
          })
          this.notify()
          return
        }
        this.connect({
          ...subscription,
          after: Math.max(acceptedAfter, recovery?.after ?? 0),
        })
      })
      .finally(() => {
        this.recoveringRuns.delete(runId)
        if (this.enabled && this.states.has(runId) && !this.streams.has(runId) && this.states.get(runId)?.status !== 'terminal') {
          this.connect({ ...subscription, after: acceptedAfter })
        }
        this.notify()
      })
  }

  private isCurrent(runId: string, source: EventSource) {
    return this.streams.get(runId)?.source === source
  }

  private subscriptionFromState(state: AIRunStreamState): AIRunStreamSubscription {
    return {
      conversationId: state.conversationId,
      eventsUrl: state.eventsUrl,
      runId: state.runId,
      after: state.after,
    }
  }

  private updateState(stream: ActiveStream, patch: Partial<AIRunStreamState>) {
    stream.state = { ...stream.state, ...patch }
    this.states.set(stream.state.runId, stream.state)
    this.notify()
  }

  private scheduleOutputIdle(stream: ActiveStream) {
    if (stream.outputIdleTimer)
      clearTimeout(stream.outputIdleTimer)
    stream.outputIdleTimer = setTimeout(() => {
      if (!this.isCurrent(stream.state.runId, stream.source) || stream.state.status !== 'streaming')
        return
      this.updateState(stream, { status: 'open' })
    }, this.outputIdleAfterMs)
  }

  private scheduleStalled(stream: ActiveStream) {
    if (stream.stalledTimer)
      clearTimeout(stream.stalledTimer)
    stream.stalledTimer = setTimeout(() => {
      if (!this.isCurrent(stream.state.runId, stream.source) || stream.state.status === 'terminal')
        return
      this.updateState(stream, { status: 'stalled' })
    }, this.stalledAfterMs)
  }

  private clearTimers(stream: ActiveStream) {
    if (stream.outputIdleTimer)
      clearTimeout(stream.outputIdleTimer)
    if (stream.stalledTimer)
      clearTimeout(stream.stalledTimer)
  }

  private notify() {
    this.callbacks.onSubscriptionsChange?.(this.subscriptions())
    this.callbacks.onStatesChange?.(this.streamStates())
  }
}

interface UseAIRunStreamManagerOptions extends Omit<AIRunStreamCallbacks, 'onStatesChange' | 'onSubscriptionsChange'> {
  enabled: boolean
}

export function useAIRunStreamManager(options: UseAIRunStreamManagerOptions) {
  const callbacksRef = useRef(options)
  callbacksRef.current = options
  const [subscriptions, setSubscriptions] = useState<AIRunStreamSubscription[]>([])
  const [streamStates, setStreamStates] = useState<AIRunStreamState[]>([])
  const managerRef = useRef<AIRunStreamManager | null>(null)
  if (!managerRef.current) {
    managerRef.current = new AIRunStreamManager(
      createAIEventSource,
      {
        onEvent: event => callbacksRef.current.onEvent(event),
        onCapabilitiesChanged: () => callbacksRef.current.onCapabilitiesChanged?.(),
        onMalformedEvent: subscription => callbacksRef.current.onMalformedEvent?.(subscription),
        onSequenceGap: async (subscription, event) => callbacksRef.current.onSequenceGap?.(subscription, event),
        onStatesChange: setStreamStates,
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
    reconnect: manager.reconnect,
    streamStates,
    subscriptions,
    syncConversation: manager.syncConversation,
  }
}
