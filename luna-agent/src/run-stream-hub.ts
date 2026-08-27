import type { Run, RunEvent } from "./domain.js"
import type { Repository } from "./persistence/repository.js"
import type { RunStreamBus, RunStreamReader } from "./run-stream-bus.js"
import { agentMetrics, errorDiagnostic, telemetryLog } from "./telemetry.js"

const defaultMaximumSubscribersPerRun = 4
const defaultMaximumSubscribersPerInstance = 64
const defaultMaximumPendingEventsPerSubscriber = 256
const defaultMaximumPendingBytesPerSubscriber = 512 * 1024
const durablePollIntervalMs = 1_000

export interface RunStreamHubUpdate {
  events: RunEvent[]
  run?: Run
  terminal: boolean
  error?: Error
}

export interface RunStreamHubSubscription {
  advance(after: number): void
  next(signal: AbortSignal): Promise<RunStreamHubUpdate>
  close(): void
}

/**
 * 单 Agent 副本内的同一 Run 只保留一个实时缓冲读取器和一个 PG 权威观察器。
 * 浏览器连接只订阅本地扇出，慢连接的背压不会阻塞其他订阅者。
 */
export class RunStreamHubManager {
  private readonly hubs = new Map<string, RunStreamHub>()
  private subscriberCount = 0

  constructor(
    private readonly repository: Repository,
    private readonly bus?: RunStreamBus,
    private readonly limits: { perRun: number, perInstance: number, pendingEvents?: number, pendingBytes?: number } = {
      perRun: defaultMaximumSubscribersPerRun,
      perInstance: defaultMaximumSubscribersPerInstance,
    },
  ) {}

  async subscribe(runId: string, ownerUserId: string, after: number): Promise<RunStreamHubSubscription> {
    if (this.subscriberCount >= this.limits.perInstance) {
      agentMetrics.sseSubscriptions.add(1, { outcome: "rejected" })
      throw new Error("ai.stream_subscriber_limit")
    }
    let hub = this.hubs.get(runId)
    let created = false
    if (!hub) {
      created = true
      hub = new RunStreamHub(runId, ownerUserId, after, this.repository, this.bus, () => {
        if (this.hubs.get(runId) === hub) this.hubs.delete(runId)
      })
      this.hubs.set(runId, hub)
    }
    if (hub.size >= this.limits.perRun) {
      if (created) await hub.close()
      agentMetrics.sseSubscriptions.add(1, { outcome: "rejected" })
      throw new Error("ai.stream_subscriber_limit")
    }
    const local = hub.add(after, {
      events: this.limits.pendingEvents ?? defaultMaximumPendingEventsPerSubscriber,
      bytes: this.limits.pendingBytes ?? defaultMaximumPendingBytesPerSubscriber,
    })
    this.subscriberCount += 1
    agentMetrics.activeSseSubscribers.add(1)
    agentMetrics.sseSubscriptions.add(1, { outcome: "accepted" })
    try {
      await hub.ensureStarted()
    }
    catch (error) {
      local.close()
      this.subscriberCount = Math.max(0, this.subscriberCount - 1)
      agentMetrics.activeSseSubscribers.add(-1)
      throw error
    }
    let closed = false
    return {
      advance: cursor => local.advance(cursor),
      next: signal => local.next(signal),
      close: () => {
        if (closed) return
        closed = true
        local.close()
        this.subscriberCount = Math.max(0, this.subscriberCount - 1)
        agentMetrics.activeSseSubscribers.add(-1)
      },
    }
  }

  async close(): Promise<void> {
    const hubs = [...this.hubs.values()]
    this.hubs.clear()
    await Promise.all(hubs.map(hub => hub.close()))
  }
}

class RunStreamHub {
  private readonly subscribers = new Set<LocalSubscription>()
  private readonly abort = new AbortController()
  private reader: RunStreamReader | undefined
  private starting: Promise<void> | undefined
  private loop: Promise<void> | undefined
  private liveCursor: number
  private durableCursor: number
  private stopped = false

  constructor(
    private readonly runId: string,
    private readonly ownerUserId: string,
    after: number,
    private readonly repository: Repository,
    private readonly bus: RunStreamBus | undefined,
    private readonly onStopped: () => void,
  ) {
    this.liveCursor = after
    this.durableCursor = after
  }

  get size(): number { return this.subscribers.size }

  add(after: number, pendingLimits: { events: number, bytes: number }): LocalSubscription {
    const subscription = new LocalSubscription(after, pendingLimits, () => {
      this.subscribers.delete(subscription)
      if (this.subscribers.size === 0) void this.close()
    })
    this.subscribers.add(subscription)
    return subscription
  }

  async ensureStarted(): Promise<void> {
    if (this.stopped) throw new Error("ai.stream_transport_unavailable")
    this.starting ??= (async () => {
      if (this.bus) this.reader = await this.bus.openReader(this.runId)
      this.loop = this.run().catch((error) => {
        const normalized = error instanceof Error ? error : new Error("ai.stream_transport_unavailable")
        telemetryLog("agent.stream.hub_failed", "warn", {
          operation: "agent.stream.hub",
          outcome: "failed",
          ...errorDiagnostic(normalized, "ai.stream_transport_unavailable"),
        })
        this.broadcast([], undefined, normalized, false)
      }).finally(() => this.stopTransport())
    })()
    await this.starting
  }

  private async run(): Promise<void> {
    let durablePollAt = 0
    while (!this.abort.signal.aborted && this.subscribers.size > 0) {
      const live = this.reader
        ? await this.reader.wait(this.liveCursor, this.abort.signal)
        : await delay(durablePollIntervalMs, this.abort.signal).then(() => [] as RunEvent[])
      if (this.abort.signal.aborted) break
      if (live.length) {
        this.liveCursor = Math.max(this.liveCursor, ...live.map(event => event.sequence))
        this.broadcast(live, undefined, undefined, false)
      }
      if (Date.now() - durablePollAt < durablePollIntervalMs) continue
      durablePollAt = Date.now()
      const [run, durable] = await Promise.all([
        this.repository.getRun(this.ownerUserId, this.runId),
        this.repository.getEvents(this.ownerUserId, this.runId, this.durableCursor),
      ])
      if (!run) throw new Error("ai.run_not_found")
      if (durable.length) this.durableCursor = Math.max(this.durableCursor, ...durable.map(event => event.sequence))
      const terminal = isTerminalRun(run)
      this.broadcast(durable, run, undefined, terminal)
      if (terminal) break
    }
  }

  private broadcast(events: RunEvent[], run: Run | undefined, error: Error | undefined, terminal: boolean): void {
    for (const subscription of this.subscribers) subscription.push(events, run, error, terminal)
  }

  async close(): Promise<void> {
    if (this.stopped) return
    this.stopped = true
    this.abort.abort()
    await this.stopTransport()
    for (const subscription of [...this.subscribers]) subscription.finish()
    this.subscribers.clear()
    this.onStopped()
  }

  private async stopTransport(): Promise<void> {
    if (this.stopped && !this.reader) return
    this.stopped = true
    this.abort.abort()
    const reader = this.reader
    this.reader = undefined
    await reader?.close().catch(() => undefined)
    this.onStopped()
  }
}

class LocalSubscription implements RunStreamHubSubscription {
  // 订阅者只保留共享 RunEvent 的有序引用，不再为每条事件维护一份 Map 节点。
  private readonly pending: Array<{ event: RunEvent, bytes: number }> = []
  private pendingBytes = 0
  private cursor: number
  private currentRun: Run | undefined
  private runDirty = false
  private terminal = false
  private error: Error | undefined
  private closed = false
  private wake: (() => void) | undefined

  constructor(
    after: number,
    private readonly pendingLimits: { events: number, bytes: number },
    private readonly onClose: () => void,
  ) { this.cursor = after }

  push(events: RunEvent[], run: Run | undefined, error: Error | undefined, terminal: boolean): void {
    if (this.closed) return
    for (const event of this.error ? [] : events) {
      if (event.sequence <= this.cursor || this.pending.some(pending => pending.event.id === event.id)) continue
      const bytes = Buffer.byteLength(JSON.stringify(event), "utf8")
      if (this.pending.length + 1 > this.pendingLimits.events || this.pendingBytes + bytes > this.pendingLimits.bytes) {
        this.error = new Error("ai.stream_subscriber_slow")
        agentMetrics.sseSubscriptions.add(1, { outcome: "slow" })
        telemetryLog("agent.stream.subscriber_slow", "warn", {
          operation: "agent.stream.subscriber",
          outcome: "rejected",
          "error.code": "ai.stream_subscriber_slow",
        })
        break
      }
      this.pending.push({ event, bytes })
      this.pendingBytes += bytes
    }
    if (run) { this.currentRun = run; this.runDirty = true }
    this.error ??= error
    this.terminal ||= terminal
    this.wake?.()
  }

  advance(after: number): void {
    this.cursor = Math.max(this.cursor, after)
    for (let index = this.pending.length - 1; index >= 0; index -= 1) {
      const pending = this.pending[index]!
      if (pending.event.sequence <= this.cursor) {
        this.pending.splice(index, 1)
        this.pendingBytes = Math.max(0, this.pendingBytes - pending.bytes)
      }
    }
  }

  async next(signal: AbortSignal): Promise<RunStreamHubUpdate> {
    while (!this.closed && !signal.aborted && this.pending.length === 0 && !this.runDirty && !this.terminal && !this.error) {
      await new Promise<void>((resolve) => {
        const wake = () => { cleanup(); resolve() }
        const cleanup = () => { signal.removeEventListener("abort", wake); if (this.wake === wake) this.wake = undefined }
        this.wake = wake
        signal.addEventListener("abort", wake, { once: true })
      })
    }
    const update = {
      events: this.pending.map(value => value.event).sort((left, right) => left.sequence - right.sequence),
      ...(this.runDirty && this.currentRun ? { run: this.currentRun } : {}),
      terminal: this.terminal,
      ...(this.error ? { error: this.error } : {}),
    }
    this.runDirty = false
    return update
  }

  close(): void {
    if (this.closed) return
    this.closed = true
    this.wake?.()
    this.onClose()
  }

  finish(): void {
    this.terminal = true
    this.wake?.()
  }
}

function isTerminalRun(run: Run): boolean {
  return ["completed", "failed", "canceled", "expired", "interrupted"].includes(run.status)
}

function delay(milliseconds: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve()
  return new Promise(resolve => {
    const timer = setTimeout(finish, milliseconds)
    const abort = () => finish()
    function finish() {
      clearTimeout(timer)
      signal.removeEventListener("abort", abort)
      resolve()
    }
    signal.addEventListener("abort", abort, { once: true })
  })
}
