import { createClient } from "redis"
import { context, propagation, trace, type Link } from "@opentelemetry/api"
import type { RunEvent, TimelineItem } from "./domain.js"
import { createId } from "./id.js"
import { RunStateConflictError, type Repository, type RunStreamBatch, type RunStreamPosition } from "./persistence/repository.js"
import { agentMetrics, errorDiagnostic, internalSpanOptions, telemetryLog, withSpan } from "./telemetry.js"

// 最长 Run 为 2 小时；额外保留 1 小时供浏览器断线重附着。
const streamTTLSeconds = 10_800
const maximumEventsPerRun = 131_072
const maximumEventBytes = 256 * 1024
// Run 生命周期累计发布上限；MAXLEN 淘汰旧事件也不返还额度，避免工具循环绕过预算。
const maximumRunBytes = 64 * 1024 * 1024
const maximumGlobalStreamBytes = 512 * 1024 * 1024
const keyPrefix = "luna:agent:run-stream:v1"
const ownerLeaseTTLSeconds = 20
const terminalGraceTTLSeconds = 300
const terminalPersistenceMaximumAttempts = 3
const terminalPersistenceInitialDelayMs = 50
type AgentRedisClient = ReturnType<typeof createClient>

const initializeScript = `
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local baseline = tonumber(ARGV[1])
if current < baseline then redis.call('SET', KEYS[1], baseline) end
redis.call('EXPIRE', KEYS[1], ARGV[2])
redis.call('EXPIRE', KEYS[2], ARGV[2])
redis.call('EXPIRE', KEYS[3], ARGV[2])
redis.call('EXPIRE', KEYS[4], ARGV[2])
return math.max(current, baseline)
`

const publishScript = `
local size = tonumber(ARGV[2])
local owner = redis.call('GET', KEYS[8])
local owner_separator = owner and string.find(owner, '|', 1, true) or nil
local owner_generation = owner_separator and tonumber(string.sub(owner, 1, owner_separator - 1)) or nil
if not owner_generation or owner_generation ~= tonumber(ARGV[12]) then
  return redis.error_reply('ai.owner_lease_lost')
end
local function cleanup_expired_batch()
  local expired = redis.call('ZRANGEBYSCORE', KEYS[6], '-inf', ARGV[8], 'LIMIT', 0, 128)
  for _, run_id in ipairs(expired) do
    local expired_bytes = tonumber(redis.call('HGET', KEYS[5], run_id) or '0')
    if expired_bytes > 0 then
      local before = tonumber(redis.call('GET', KEYS[7]) or '0')
      redis.call('SET', KEYS[7], math.max(0, before - expired_bytes))
    end
    redis.call('HDEL', KEYS[5], run_id)
    redis.call('ZREM', KEYS[6], run_id)
  end
  return #expired
end
cleanup_expired_batch()
local total = tonumber(redis.call('GET', KEYS[3]) or '0')
local global_total = tonumber(redis.call('GET', KEYS[7]) or '0')
local event_count = tonumber(redis.call('GET', KEYS[4]) or '0')
if size > tonumber(ARGV[4]) or total + size > tonumber(ARGV[5]) or event_count + 1 > tonumber(ARGV[3]) then
  return redis.error_reply('ai.stream_buffer_overflow')
end
while global_total + size > tonumber(ARGV[10]) do
  if cleanup_expired_batch() == 0 then break end
  global_total = tonumber(redis.call('GET', KEYS[7]) or '0')
end
if global_total + size > tonumber(ARGV[10]) then
  return redis.error_reply('ai.stream_global_buffer_overflow')
end
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local minimum = tonumber(ARGV[11])
if current < minimum then redis.call('SET', KEYS[1], minimum) end
local sequence = redis.call('INCR', KEYS[1])
redis.call('INCRBY', KEYS[3], size)
redis.call('INCR', KEYS[4])
redis.call('HINCRBY', KEYS[5], ARGV[7], size)
local retained = redis.call('INCRBY', KEYS[7], size)
redis.call('ZADD', KEYS[6], ARGV[9], ARGV[7])
redis.call('XADD', KEYS[2], 'MAXLEN', '=', ARGV[3], tostring(sequence) .. '-0', 'event', ARGV[1])
redis.call('EXPIRE', KEYS[1], ARGV[6])
redis.call('EXPIRE', KEYS[2], ARGV[6])
redis.call('EXPIRE', KEYS[3], ARGV[6])
redis.call('EXPIRE', KEYS[4], ARGV[6])
return { sequence, retained }
`

const compareExpireScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('EXPIRE', KEYS[1], ARGV[2])
return 1
`
const compareDeleteScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
return redis.call('DEL', KEYS[1])
`
const acquireOwnerScript = `
local current = redis.call('GET', KEYS[1])
local generation = tonumber(ARGV[1])
if not current then
  redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
  return 1
end
local separator = string.find(current, '|', 1, true)
local current_generation = separator and tonumber(string.sub(current, 1, separator - 1)) or nil
if current_generation and current_generation < generation then
  redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
  return 1
end
return 0
`

export class RedisRunStreamBus {
  private readonly client: AgentRedisClient
  private connecting: Promise<void> | undefined

  constructor(redisURL: string, private readonly repository: Repository) {
    this.client = createClient({
      url: redisURL,
      socket: { connectTimeout: 2_000, reconnectStrategy: false },
    })
    this.client.on("error", error => telemetryLog("agent.stream.redis_failed", "warn", {
      operation: "agent.stream.redis",
      outcome: "failed",
      ...errorDiagnostic(error, "ai.stream_transport_unavailable"),
    }))
  }

  async connect(): Promise<void> {
    if (this.client.isReady) return
    this.connecting ??= withSpan("agent.stream.transport.connect", internalSpanOptions(), async () => {
      if (!this.client.isOpen) await this.client.connect()
      await this.client.ping()
    }).finally(() => { this.connecting = undefined })
    await this.connecting
  }

  async health(): Promise<boolean> {
    try {
      await this.connect()
      await this.client.ping()
      return true
    }
    catch { return false }
  }

  async close(): Promise<void> {
    if (this.client.isOpen) await this.client.close()
  }

  async open(runId: string, turnId: string, expectedRunVersion?: number): Promise<RunStreamSession> {
    const position = await this.repository.getRunStreamPosition(runId)
    if (!position) throw new Error("ai.run_not_found")
    const keys = streamKeys(runId)
    try {
      await withSpan("agent.stream.open", internalSpanOptions(), async () => {
        await this.client.eval(initializeScript, {
          keys: [keys.sequence, keys.stream, keys.emittedBytes, keys.eventCount],
          arguments: [String(position.nextEventSequence - 1), String(streamTTLSeconds)],
        })
      })
    }
    catch (error) {
      throw new Error("ai.stream_transport_unavailable", { cause: error })
    }
    return new RunStreamSession(this, this.repository, runId, turnId, position, expectedRunVersion ?? position.runVersion)
  }

  async publish(runId: string, event: Omit<RunEvent, "sequence">, minimumSequence = 0, expectedRunVersion?: number): Promise<RunEvent> {
    const keys = streamKeys(runId)
    const traceContext: Record<string, string> = {}
    propagation.inject(context.active(), traceContext)
    const encoded = JSON.stringify({ event, traceContext })
    const size = Buffer.byteLength(encoded)
    const startedAt = performance.now()
    let sequence: number
    try {
      const now = Date.now()
      const result = await withSpan("agent.stream.publish", internalSpanOptions(), () => this.client.eval(publishScript, {
          keys: [keys.sequence, keys.stream, keys.emittedBytes, keys.eventCount, globalKeys.bytesByRun, globalKeys.expiry, globalKeys.totalBytes, keys.ownerLease],
          arguments: [encoded, String(size), String(maximumEventsPerRun), String(maximumEventBytes), String(maximumRunBytes), String(streamTTLSeconds), runId, String(now), String(now + streamTTLSeconds * 1_000), String(maximumGlobalStreamBytes), String(minimumSequence), String(expectedRunVersion ?? 0)],
        }))
      const values = result as Array<number | string>
      sequence = Number(values[0])
      agentMetrics.streamRetainedBytes.record(Number(values[1]), { outcome: "published" })
      if (!Number.isSafeInteger(sequence) || sequence < 1) throw new Error("ai.stream_sequence_invalid")
    }
    catch (error) {
      const ownerLeaseLost = error instanceof Error && error.message.includes("ai.owner_lease_lost")
      const globalOverflow = error instanceof Error && error.message.includes("ai.stream_global_buffer_overflow")
      const message = ownerLeaseLost
        ? "ai.owner_lease_lost"
        : globalOverflow
        ? "ai.stream_global_buffer_overflow"
        : error instanceof Error && error.message.includes("ai.stream_buffer_overflow")
          ? "ai.stream_buffer_overflow"
          : "ai.stream_transport_unavailable"
      agentMetrics.streamEvents.add(1, { outcome: globalOverflow ? "global_overflow" : "failed" })
      agentMetrics.streamTransportDuration.record((performance.now() - startedAt) / 1000, { operation: "publish", outcome: "failed" })
      throw new Error(message, { cause: error })
    }
    agentMetrics.streamEvents.add(1, { outcome: "published" })
    agentMetrics.streamBufferBytes.record(size, { outcome: "published" })
    agentMetrics.streamTransportDuration.record((performance.now() - startedAt) / 1000, { operation: "publish", outcome: "succeeded" })
    return { ...event, sequence }
  }

  async read(runId: string, after: number, count = 256): Promise<RunEvent[]> {
    const startedAt = performance.now()
    try {
      const events = await withSpan("agent.stream.replay", internalSpanOptions(), async () => {
        const rows = await this.client.xRange(streamKeys(runId).stream, `(${after}-0`, "+", { COUNT: count })
        await recordConsumeLinks(rows.map(row => row.message.event))
        return rows.flatMap(row => decodeStreamRow(row.id, row.message.event))
      })
      agentMetrics.streamTransportDuration.record((performance.now() - startedAt) / 1000, { operation: "replay", outcome: "succeeded" })
      return events
    }
    catch (error) {
      agentMetrics.streamTransportDuration.record((performance.now() - startedAt) / 1000, { operation: "replay", outcome: "failed" })
      throw error
    }
  }

  async openReader(runId: string): Promise<RunStreamReader> {
    const reader = this.client.duplicate()
    await withSpan("agent.stream.reader.open", internalSpanOptions(), () => reader.connect())
    agentMetrics.activeStreamReaders.add(1)
    return new RedisRunStreamReader(reader, streamKeys(runId).stream)
  }

  async requestCancellation(runId: string): Promise<void> {
    const keys = streamKeys(runId)
    const traceContext: Record<string, string> = {}
    propagation.inject(context.active(), traceContext)
    await withSpan("agent.stream.cancel.request", internalSpanOptions(), async () => {
      await this.client.del(keys.cancelAcknowledged)
      await this.client.xAdd(keys.cancelStream, "*", {
        type: "cancel",
        traceparent: traceContext.traceparent ?? "",
        tracestate: traceContext.tracestate ?? "",
      }, {
        TRIM: { strategy: "MAXLEN", strategyModifier: "=", threshold: 8 },
      })
      await this.client.expire(keys.cancelStream, streamTTLSeconds)
    })
    agentMetrics.streamCancellations.add(1, { operation: "request", outcome: "succeeded" })
  }

  async waitForCancellation(runId: string, _after: number, signal: AbortSignal): Promise<void> {
    void _after
    const reader = this.client.duplicate()
    await withSpan("agent.stream.cancel.reader.open", internalSpanOptions(), () => reader.connect())
    let cursor = "0-0"
    const abort = () => reader.destroy()
    signal.addEventListener("abort", abort, { once: true })
    try {
      while (!signal.aborted) {
        const groups = await withSpan("agent.stream.cancel.wait", internalSpanOptions(), () => reader.xRead({ key: streamKeys(runId).cancelStream, id: cursor }, {
            COUNT: 8, BLOCK: 1_000,
        })) as unknown as Array<{ messages: Array<{ id: string, message: Record<string, string> }> }> | null
        for (const message of groups?.flatMap(group => group.messages) ?? []) {
          cursor = message.id
          await recordControlConsumeLink(message.message)
          return
        }
      }
    }
    catch (error) {
      if (!signal.aborted) throw error
    }
    finally {
      signal.removeEventListener("abort", abort)
      if (reader.isOpen) reader.destroy()
    }
  }

  async acknowledgeCancellation(runId: string): Promise<void> {
    await withSpan("agent.stream.cancel.acknowledge", internalSpanOptions(), () => this.client.set(streamKeys(runId).cancelAcknowledged, "1", { EX: 60 }))
    agentMetrics.streamCancellations.add(1, { operation: "acknowledge", outcome: "succeeded" })
  }

  async waitForCancellationAcknowledgement(runId: string, timeoutMs = 2_000): Promise<boolean> {
    return withSpan("agent.stream.cancel.acknowledgement.wait", internalSpanOptions(), async () => {
      const deadline = Date.now() + timeoutMs
      while (Date.now() < deadline) {
        if (await this.client.exists(streamKeys(runId).cancelAcknowledged)) return true
        await new Promise(resolve => setTimeout(resolve, 50))
      }
      return false
    })
  }

  async acquireOwnership(runId: string, generation: number, ownerId: string): Promise<RunOwnerLease | undefined> {
    const token = `${generation}|${Buffer.from(ownerId).toString("base64url")}|${createId("lease")}`
    const key = streamKeys(runId).ownerLease
    const acquired = Number(await withSpan("agent.stream.owner.acquire", internalSpanOptions(), () => this.client.eval(acquireOwnerScript, {
      keys: [key], arguments: [String(generation), token, String(ownerLeaseTTLSeconds)],
    }))) === 1
    agentMetrics.ownerLeases.add(1, { operation: "acquire", outcome: acquired ? "succeeded" : "contended" })
    if (!acquired) return undefined
    return {
      renew: async () => {
        const renewed = Number(await withSpan("agent.stream.owner.renew", internalSpanOptions(), () => this.client.eval(compareExpireScript, {
            keys: [key], arguments: [token, String(ownerLeaseTTLSeconds)],
          }))) === 1
        agentMetrics.ownerLeases.add(1, { operation: "renew", outcome: renewed ? "succeeded" : "fenced" })
        return renewed
      },
      release: async () => {
        await withSpan("agent.stream.owner.release", internalSpanOptions(), () => this.client.eval(compareDeleteScript, { keys: [key], arguments: [token] }))
      },
    }
  }

  async fenceExpiredOwnership(runId: string, generation: number, reconcilerId: string): Promise<boolean> {
    const token = `reconciler:${reconcilerId}:${generation}`
    const fenced = (await withSpan("agent.stream.owner.fence", internalSpanOptions(), () => this.client.set(streamKeys(runId).ownerLease, token, { NX: true, EX: ownerLeaseTTLSeconds }))) === "OK"
    agentMetrics.ownerLeases.add(1, { operation: "fence", outcome: fenced ? "succeeded" : "active" })
    return fenced
  }

  async cleanup(runId: string): Promise<void> {
    const keys = Object.values(streamKeys(runId))
    await withSpan("agent.stream.cleanup", internalSpanOptions(), async () => {
      const transaction = this.client.multi()
      for (const key of keys) transaction.expire(key, terminalGraceTTLSeconds)
      transaction.zAdd(globalKeys.expiry, { score: Date.now() + terminalGraceTTLSeconds * 1_000, value: runId })
      await transaction.exec()
    })
    agentMetrics.streamCleanups.add(1, { outcome: "succeeded" })
  }

  async getHighWatermark(runId: string): Promise<number> {
    const value = await withSpan("agent.stream.high_watermark.read", internalSpanOptions(), () => this.client.get(streamKeys(runId).sequence))
    const sequence = Number(value ?? 0)
    if (!Number.isSafeInteger(sequence) || sequence < 0) throw new Error("ai.stream_sequence_invalid")
    return sequence
  }
}

export interface RunStreamBus {
  health(): Promise<boolean>
  open(runId: string, turnId: string, expectedRunVersion?: number): Promise<RunStreamSession>
  read(runId: string, after: number, count?: number): Promise<RunEvent[]>
  openReader(runId: string): Promise<RunStreamReader>
  requestCancellation(runId: string): Promise<void>
  waitForCancellation(runId: string, after: number, signal: AbortSignal): Promise<void>
  acknowledgeCancellation(runId: string): Promise<void>
  waitForCancellationAcknowledgement(runId: string, timeoutMs?: number): Promise<boolean>
  acquireOwnership(runId: string, generation: number, ownerId: string): Promise<RunOwnerLease | undefined>
  fenceExpiredOwnership(runId: string, generation: number, reconcilerId: string): Promise<boolean>
  cleanup(runId: string): Promise<void>
  getHighWatermark(runId: string): Promise<number>
}

export interface RunOwnerLease {
  renew(): Promise<boolean>
  release(): Promise<void>
}

export interface RunStreamReader {
  wait(after: number, signal: AbortSignal): Promise<RunEvent[]>
  close(): Promise<void>
}

interface RunStreamTransport {
  publish(runId: string, event: Omit<RunEvent, "sequence">, minimumSequence?: number, expectedRunVersion?: number): Promise<RunEvent>
  cleanup(runId: string): Promise<void>
}

/** 测试与显式单实例开发模式使用；生产多副本必须使用 RedisRunStreamBus。 */
export class InMemoryRunStreamBus implements RunStreamBus, RunStreamTransport {
  private readonly events = new Map<string, RunEvent[]>()
  private readonly sequences = new Map<string, number>()
  private readonly waiters = new Map<string, Set<() => void>>()
  private readonly cancellationAcknowledged = new Set<string>()
  private readonly cancellationRequested = new Set<string>()
  private readonly ownerLeases = new Map<string, { token: string, generation: number, expiresAt: number }>()

  constructor(private readonly repository: Repository) {}

  async health(): Promise<boolean> { return true }

  async open(runId: string, turnId: string, expectedRunVersion?: number): Promise<RunStreamSession> {
    const position = await this.repository.getRunStreamPosition(runId)
    if (!position) throw new Error("ai.run_not_found")
    this.sequences.set(runId, Math.max(this.sequences.get(runId) ?? 0, position.nextEventSequence - 1))
    return new RunStreamSession(this, this.repository, runId, turnId, position, expectedRunVersion ?? position.runVersion)
  }

  async publish(runId: string, event: Omit<RunEvent, "sequence">, minimumSequence = 0, expectedRunVersion?: number): Promise<RunEvent> {
    const lease = this.ownerLeases.get(runId)
    if (lease && (lease.expiresAt <= Date.now() || lease.generation !== expectedRunVersion || lease.token.startsWith("reconciler:"))) {
      throw new Error("ai.owner_lease_lost")
    }
    const value = { ...event, sequence: Math.max(this.sequences.get(runId) ?? 0, minimumSequence) + 1 }
    this.sequences.set(runId, value.sequence)
    const events = this.events.get(runId) ?? []
    events.push(value)
    if (events.length > maximumEventsPerRun) events.splice(0, events.length - maximumEventsPerRun)
    this.events.set(runId, events)
    for (const notify of this.waiters.get(runId) ?? []) notify()
    return value
  }

  async read(runId: string, after: number, count = 256): Promise<RunEvent[]> {
    return (this.events.get(runId) ?? []).filter(event => event.sequence > after).slice(0, count)
  }

  async openReader(runId: string): Promise<RunStreamReader> {
    let closed = false
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
            resolve()
          }
          const timer = setTimeout(finish, 1_000)
          signal.addEventListener("abort", finish, { once: true })
          waiters.add(finish)
          this.waiters.set(runId, waiters)
        })
        return this.read(runId, after)
      },
      close: async () => { closed = true },
    }
  }

  async requestCancellation(runId: string): Promise<void> {
    this.cancellationAcknowledged.delete(runId)
    this.cancellationRequested.add(runId)
    for (const notify of this.waiters.get(`${runId}:cancel`) ?? []) notify()
  }

  async waitForCancellation(runId: string, after: number, signal: AbortSignal): Promise<void> {
    void after
    if (this.cancellationRequested.has(runId) || signal.aborted) return
    await new Promise<void>((resolve) => {
      const key = `${runId}:cancel`
      const waiters = this.waiters.get(key) ?? new Set<() => void>()
      const finish = () => {
        signal.removeEventListener("abort", finish)
        waiters.delete(finish)
        resolve()
      }
      signal.addEventListener("abort", finish, { once: true })
      waiters.add(finish)
      this.waiters.set(key, waiters)
    })
  }

  async acknowledgeCancellation(runId: string): Promise<void> { this.cancellationAcknowledged.add(runId) }
  async waitForCancellationAcknowledgement(runId: string, timeoutMs = 2_000): Promise<boolean> {
    const deadline = Date.now() + timeoutMs
    while (Date.now() < deadline) {
      if (this.cancellationAcknowledged.has(runId)) return true
      await new Promise(resolve => setTimeout(resolve, 10))
    }
    return false
  }

  async acquireOwnership(runId: string, generation: number, ownerId: string): Promise<RunOwnerLease | undefined> {
    const current = this.ownerLeases.get(runId)
    if (current && current.expiresAt > Date.now() && current.generation >= generation) return undefined
    const token = `${generation}|${Buffer.from(ownerId).toString("base64url")}|${createId("lease")}`
    const renew = async () => {
      if (this.ownerLeases.get(runId)?.token !== token) return false
      this.ownerLeases.set(runId, { token, generation, expiresAt: Date.now() + ownerLeaseTTLSeconds * 1_000 })
      return true
    }
    this.ownerLeases.set(runId, { token, generation, expiresAt: Date.now() + ownerLeaseTTLSeconds * 1_000 })
    return {
      renew,
      release: async () => { if (this.ownerLeases.get(runId)?.token === token) this.ownerLeases.delete(runId) },
    }
  }

  async fenceExpiredOwnership(runId: string, generation: number, reconcilerId: string): Promise<boolean> {
    const current = this.ownerLeases.get(runId)
    if (current && current.expiresAt > Date.now()) return false
    this.ownerLeases.set(runId, {
      token: `reconciler:${reconcilerId}:${generation}`,
      generation,
      expiresAt: Date.now() + ownerLeaseTTLSeconds * 1_000,
    })
    return true
  }

  async cleanup(runId: string): Promise<void> {
    const timer = setTimeout(() => {
      this.events.delete(runId)
      this.sequences.delete(runId)
      this.cancellationAcknowledged.delete(runId)
      this.cancellationRequested.delete(runId)
      this.ownerLeases.delete(runId)
    }, terminalGraceTTLSeconds * 1_000)
    timer.unref()
  }
  async getHighWatermark(runId: string): Promise<number> { return this.sequences.get(runId) ?? 0 }
}

class RedisRunStreamReader implements RunStreamReader {
  private closed = false
  constructor(private readonly client: AgentRedisClient, private readonly key: string) {}

  async wait(after: number, signal: AbortSignal): Promise<RunEvent[]> {
    if (signal.aborted) return []
    const abort = () => this.client.destroy()
    signal.addEventListener("abort", abort, { once: true })
    try {
      return await withSpan("agent.stream.reader.wait", internalSpanOptions(), async () => {
        const groups = await this.client.xRead({ key: this.key, id: `${after}-0` }, { COUNT: 256, BLOCK: 1_000 }) as unknown as Array<{
          messages: Array<{ id: string, message: Record<string, string> }>
        }> | null
        if (!groups) return []
        await recordConsumeLinks(groups.flatMap(group => group.messages.map(row => row.message.event)))
        return groups.flatMap(group => group.messages.flatMap(row => decodeStreamRow(row.id, row.message.event)))
      })
    }
    catch (error) {
      if (signal.aborted) return []
      throw new Error("ai.stream_transport_unavailable", { cause: error })
    }
    finally {
      signal.removeEventListener("abort", abort)
    }
  }

  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true
    if (this.client.isOpen) this.client.destroy()
    agentMetrics.activeStreamReaders.add(-1)
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
        ...errorDiagnostic(error, "ai.stream_transport_unavailable"),
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

function streamKeys(runId: string) {
  return {
    sequence: `${keyPrefix}:${runId}:sequence`,
    stream: `${keyPrefix}:${runId}:events`,
    emittedBytes: `${keyPrefix}:${runId}:emitted-bytes`,
    eventCount: `${keyPrefix}:${runId}:event-count`,
    cancelAcknowledged: `${keyPrefix}:${runId}:cancel-acknowledged`,
    cancelStream: `${keyPrefix}:${runId}:control`,
    ownerLease: `${keyPrefix}:${runId}:owner-lease`,
  }
}

const globalKeys = {
  bytesByRun: `${keyPrefix}:global:bytes-by-run`,
  expiry: `${keyPrefix}:global:expiry`,
  totalBytes: `${keyPrefix}:global:total-bytes`,
}

function decodeStreamRow(id: string, encoded: unknown): RunEvent[] {
  if (typeof encoded !== "string") throw new Error("ai.stream_event_invalid")
  try {
    const parsed = JSON.parse(encoded) as { event?: Omit<RunEvent, "sequence"> } | Omit<RunEvent, "sequence">
    const event = "event" in parsed && parsed.event ? parsed.event : parsed as Omit<RunEvent, "sequence">
    const sequence = Number(id.split("-", 1)[0])
    if (!Number.isSafeInteger(sequence) || !event || typeof event.id !== "string" || typeof event.type !== "string") {
      throw new Error("ai.stream_event_invalid")
    }
    return [{ ...event, sequence }]
  }
  catch (error) {
    if (error instanceof Error && error.message === "ai.stream_event_invalid") throw error
    throw new Error("ai.stream_event_invalid", { cause: error })
  }
}

async function recordConsumeLinks(encodedEvents: unknown[]): Promise<void> {
  const links: Link[] = []
  for (const encoded of encodedEvents) {
    if (typeof encoded !== "string") continue
    try {
      const parsed = JSON.parse(encoded) as { traceContext?: Record<string, string> }
      if (!parsed.traceContext) continue
      const spanContext = trace.getSpanContext(propagation.extract(context.active(), parsed.traceContext))
      if (spanContext) links.push({ context: spanContext })
    }
    catch { /* malformed rows are rejected by decodeStreamRow; never log their contents. */ }
  }
  if (links.length) await withSpan("agent.stream.consume", { ...internalSpanOptions(), links: links.slice(0, 256) }, async () => undefined)
}

async function recordControlConsumeLink(carrier: Record<string, string>): Promise<void> {
  if (!carrier.traceparent) return
  const spanContext = trace.getSpanContext(propagation.extract(context.active(), {
    traceparent: carrier.traceparent,
    ...(carrier.tracestate ? { tracestate: carrier.tracestate } : {}),
  }))
  if (spanContext) await withSpan("agent.stream.cancel.consume", {
    ...internalSpanOptions(),
    links: [{ context: spanContext }],
  }, async () => undefined)
}
