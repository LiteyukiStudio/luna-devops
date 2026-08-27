import type { RunEvent, TimelineItem } from "./domain.js"
import { createId } from "./id.js"
import { RunStateConflictError, type Repository, type RunStreamBatch, type RunStreamPosition } from "./persistence/repository.js"
import { agentMetrics, errorDiagnostic, internalSpanOptions, telemetryLog, withSpan } from "./telemetry.js"

const maximumEventsPerRun = 131_072
const maximumEventBytes = 256 * 1024
const maximumRunBytes = 64 * 1024 * 1024
const maximumProcessBytes = 512 * 1024 * 1024
const terminalGraceMs = 5 * 60 * 1_000
const terminalPersistenceMaximumAttempts = 3
const terminalPersistenceInitialDelayMs = 50

export interface RunStreamBus {
  open(runId: string, turnId: string, expectedRunVersion?: number): Promise<RunStreamSession>
  read(runId: string, after: number, count?: number): Promise<RunEvent[]>
  openReader(runId: string): Promise<RunStreamReader>
  cleanup(runId: string): Promise<void>
}

export interface RunStreamReader {
  wait(after: number, signal: AbortSignal): Promise<RunEvent[]>
  close(): Promise<void>
}

interface RunStreamTransport {
  publish(runId: string, event: Omit<RunEvent, "sequence">, minimumSequence?: number, expectedRunVersion?: number): Promise<RunEvent>
  cleanup(runId: string): Promise<void>
}

/**
 * 单 Agent 副本的短生命周期实时流。
 * PostgreSQL 仍是时间线与可恢复事件的事实源；此缓冲只降低当前副本内 SSE 的延迟。
 */
export class InMemoryRunStreamBus implements RunStreamBus, RunStreamTransport {
  private readonly events = new Map<string, RunEvent[]>()
  private readonly sequences = new Map<string, number>()
  private readonly emittedBytes = new Map<string, number>()
  private readonly eventCounts = new Map<string, number>()
  private readonly waiters = new Map<string, Set<() => void>>()
  private readonly cleanupTimers = new Map<string, ReturnType<typeof setTimeout>>()
  private totalBytes = 0

  constructor(private readonly repository: Repository) {}

  async open(runId: string, turnId: string, expectedRunVersion?: number): Promise<RunStreamSession> {
    const position = await this.repository.getRunStreamPosition(runId)
    if (!position) throw new Error("ai.run_not_found")
    const cleanupTimer = this.cleanupTimers.get(runId)
    if (cleanupTimer) {
      clearTimeout(cleanupTimer)
      this.cleanupTimers.delete(runId)
    }
    this.sequences.set(runId, Math.max(this.sequences.get(runId) ?? 0, position.nextEventSequence - 1))
    return new RunStreamSession(this, this.repository, runId, turnId, position, expectedRunVersion ?? position.runVersion)
  }

  async publish(
    runId: string,
    event: Omit<RunEvent, "sequence">,
    minimumSequence = 0,
    expectedRunVersion?: number,
  ): Promise<RunEvent> {
    void expectedRunVersion
    const startedAt = performance.now()
    const size = Buffer.byteLength(JSON.stringify(event))
    const total = this.emittedBytes.get(runId) ?? 0
    const count = this.eventCounts.get(runId) ?? 0
    if (size > maximumEventBytes || total + size > maximumRunBytes || this.totalBytes + size > maximumProcessBytes || count + 1 > maximumEventsPerRun) {
      agentMetrics.streamEvents.add(1, { outcome: "failed" })
      agentMetrics.streamTransportDuration.record((performance.now() - startedAt) / 1000, { operation: "publish", outcome: "failed" })
      throw new Error("ai.stream_buffer_overflow")
    }

    const value: RunEvent = {
      ...event,
      sequence: Math.max(this.sequences.get(runId) ?? 0, minimumSequence) + 1,
    }
    this.sequences.set(runId, value.sequence)
    this.emittedBytes.set(runId, total + size)
    this.totalBytes += size
    this.eventCounts.set(runId, count + 1)
    const events = this.events.get(runId) ?? []
    events.push(value)
    this.events.set(runId, events)
    for (const notify of this.waiters.get(runId) ?? []) notify()

    agentMetrics.streamEvents.add(1, { outcome: "published" })
    agentMetrics.streamBufferBytes.record(size, { outcome: "published" })
    agentMetrics.streamRetainedBytes.record(total + size, { outcome: "published" })
    agentMetrics.streamTransportDuration.record((performance.now() - startedAt) / 1000, { operation: "publish", outcome: "succeeded" })
    return value
  }

  async read(runId: string, after: number, count = 256): Promise<RunEvent[]> {
    return withSpan("agent.stream.replay", internalSpanOptions(), async () => (
      this.events.get(runId) ?? []
    ).filter(event => event.sequence > after).slice(0, count))
  }

  async openReader(runId: string): Promise<RunStreamReader> {
    let closed = false
    let waiting: (() => void) | undefined
    agentMetrics.activeStreamReaders.add(1)
    return {
      wait: async (after, signal) => {
        const available = await this.read(runId, after)
        if (available.length || closed || signal.aborted) return available
        await new Promise<void>((resolve) => {
          const waiters = this.waiters.get(runId) ?? new Set<() => void>()
          const finish = () => {
            clearTimeout(timer)
            signal.removeEventListener("abort", finish)
            waiters.delete(finish)
            if (waiting === finish) waiting = undefined
            resolve()
          }
          const timer = setTimeout(finish, 1_000)
          signal.addEventListener("abort", finish, { once: true })
          waiters.add(finish)
          this.waiters.set(runId, waiters)
          waiting = finish
        })
        return this.read(runId, after)
      },
      close: async () => {
        if (closed) return
        closed = true
        waiting?.()
        agentMetrics.activeStreamReaders.add(-1)
      },
    }
  }

  async cleanup(runId: string): Promise<void> {
    const previous = this.cleanupTimers.get(runId)
    if (previous) clearTimeout(previous)
    const timer = setTimeout(() => {
      this.events.delete(runId)
      this.sequences.delete(runId)
      this.totalBytes = Math.max(0, this.totalBytes - (this.emittedBytes.get(runId) ?? 0))
      this.emittedBytes.delete(runId)
      this.eventCounts.delete(runId)
      this.waiters.delete(runId)
      this.cleanupTimers.delete(runId)
    }, terminalGraceMs)
    timer.unref()
    this.cleanupTimers.set(runId, timer)
    agentMetrics.streamCleanups.add(1, { outcome: "succeeded" })
  }
}

export class RunStreamSession {
  private readonly items = new Map<string, TimelineItem>()
  private readonly durableEvents: RunEvent[] = []
  private committed = false
  private lastSequence: number
  private terminalIntent: NonNullable<RunStreamBatch["terminal"]> | undefined

  constructor(
    private readonly bus: RunStreamTransport,
    private readonly repository: Repository,
    readonly runId: string,
    private readonly turnId: string,
    private readonly expected: RunStreamPosition,
    private readonly expectedRunVersion: number,
  ) {
    this.lastSequence = expected.nextEventSequence - 1
  }

  async appendEvent(type: string, data: Record<string, unknown>): Promise<RunEvent> {
    this.assertOpen()
    const createdAt = new Date().toISOString()
    const event = await this.bus.publish(this.runId, {
      id: createId("aievt"), runId: this.runId, type, data: structuredClone(data), createdAt,
    }, this.lastSequence, this.expectedRunVersion)
    this.lastSequence = event.sequence
    if (durableEventTypes.has(event.type)) this.durableEvents.push(event)
    return event
  }

  async appendItemWithEvent(
    value: Omit<TimelineItem, "timelineIndex" | "revision" | "createdAt">,
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    this.assertOpen()
    const item: TimelineItem = {
      ...structuredClone(value),
      timelineIndex: this.expected.nextItemPosition + this.items.size,
      revision: 1,
      createdAt: new Date().toISOString(),
    }
    this.items.set(item.id, item)
    const event = await this.appendEvent(eventType, liveItemEventTypes.has(eventType)
      ? { ...eventData, itemId: item.id, timelineIndex: item.timelineIndex, createdAt: item.createdAt }
      : { ...eventData, item: structuredClone(item) })
    return { item, event }
  }

  async updateItemWithEvent(
    itemId: string,
    status: TimelineItem["status"],
    content: Record<string, unknown>,
    eventType: string,
    eventData: Record<string, unknown> = {},
  ) {
    this.assertOpen()
    const current = this.items.get(itemId)
    if (!current) throw new Error("ai.item_not_found")
    const item: TimelineItem = { ...current, status, content: structuredClone(content), revision: current.revision + 1 }
    this.items.set(itemId, item)
    const event = await this.appendEvent(eventType, liveItemEventTypes.has(eventType)
      ? { ...eventData, itemId: item.id, timelineIndex: item.timelineIndex }
      : { ...eventData, item: structuredClone(item) })
    return { item, event }
  }

  async commit(fallbackStatus: Exclude<TimelineItem["status"], "streaming"> = "completed"): Promise<void> {
    await this.persist(fallbackStatus)
  }

  async commitTerminal(
    to: "completed" | "failed" | "canceled" | "interrupted",
    errorCode?: string,
    conversationTitle?: string,
  ): Promise<void> {
    const requestedIntent: NonNullable<RunStreamBatch["terminal"]> = {
      from: "running", to, completedAt: new Date().toISOString(),
      ...(errorCode ? { errorCode } : {}),
      ...(conversationTitle ? { conversationTitle } : {}),
    }
    if (!this.terminalIntent) this.terminalIntent = requestedIntent
    else if (this.terminalIntent.to !== to) throw new Error("ai.terminal_intent_conflict")
    await this.persistWithRetry(to === "completed" ? "completed" : "failed", this.terminalIntent)
    try { await this.bus.cleanup(this.runId) }
    catch (error) {
      agentMetrics.streamCleanups.add(1, { outcome: "failed" })
      telemetryLog("agent.stream.cleanup_failed", "warn", {
        operation: "agent.stream.cleanup", outcome: "failed",
        ...errorDiagnostic(error, "ai.stream_cleanup_failed"),
      })
    }
  }

  private async persistWithRetry(
    fallbackStatus: Exclude<TimelineItem["status"], "streaming">,
    terminal: NonNullable<RunStreamBatch["terminal"]>,
  ): Promise<void> {
    let lastError: unknown
    for (let attempt = 1; attempt <= terminalPersistenceMaximumAttempts; attempt += 1) {
      try {
        await this.persist(fallbackStatus, terminal)
        agentMetrics.streamTerminalPersistence.add(1, { operation: "persist", outcome: attempt === 1 ? "succeeded" : "retried" })
        return
      }
      catch (error) {
        lastError = error
        if (!isRetryableTerminalPersistenceError(error)) throw error
        agentMetrics.streamTerminalPersistence.add(1, {
          operation: "persist",
          outcome: attempt === terminalPersistenceMaximumAttempts ? "exhausted" : "retryable_failed",
        })
        telemetryLog("agent.stream.terminal_persistence_failed", attempt === terminalPersistenceMaximumAttempts ? "error" : "warn", {
          operation: "agent.stream.terminal_persistence",
          outcome: attempt === terminalPersistenceMaximumAttempts ? "exhausted" : "retrying",
          attempt,
          terminal_state: terminal.to,
          ...errorDiagnostic(error, "ai.terminal_persistence_failed"),
        })
        if (attempt < terminalPersistenceMaximumAttempts) {
          await new Promise(resolve => setTimeout(resolve, terminalPersistenceInitialDelayMs * 2 ** (attempt - 1)))
        }
      }
    }
    throw new TerminalPersistenceExhaustedError(terminal.to, lastError)
  }

  private async persist(
    fallbackStatus: Exclude<TimelineItem["status"], "streaming">,
    terminal?: NonNullable<RunStreamBatch["terminal"]>,
  ): Promise<void> {
    if (this.committed) return
    for (const [id, item] of this.items) {
      if (item.status === "streaming") this.items.set(id, { ...item, status: fallbackStatus, revision: item.revision + 1 })
    }
    await this.repository.persistRunStreamBatch({
      runId: this.runId,
      expected: this.expected,
      items: [...this.items.values()],
      events: this.durableEvents,
      eventHighWatermark: this.lastSequence,
      expectedRunVersion: this.expectedRunVersion,
      ...(terminal ? { terminal } : {}),
    })
    this.committed = true
  }

  private assertOpen(): void {
    if (this.committed) throw new Error("ai.stream_session_closed")
  }
}

export class TerminalPersistenceExhaustedError extends Error {
  constructor(readonly terminalState: NonNullable<RunStreamBatch["terminal"]>["to"], cause: unknown) {
    super("ai.terminal_persistence_failed", { cause })
    this.name = "TerminalPersistenceExhaustedError"
  }
}

function isRetryableTerminalPersistenceError(error: unknown): boolean {
  if (error instanceof RunStateConflictError) return false
  if (!(error instanceof Error)) return true
  return !new Set([
    "ai.run_not_found",
    "ai.stream_batch_partial_conflict",
    "ai.stream_item_position_conflict",
    "ai.stream_sequence_conflict",
    "ai.terminal_intent_conflict",
  ]).has(error.message)
}

const durableEventTypes = new Set([
  "context.compacted", "thinking.completed", "message.completed", "model.completed",
])
const liveItemEventTypes = new Set(["content.delta", "thinking.started", "thinking.delta"])
