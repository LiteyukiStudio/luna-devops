import Fastify, { type FastifyBaseLogger, type FastifyInstance, LogController } from "fastify"
import { z } from "zod"
import type { RequestVerifier } from "./auth.js"
import type { Config } from "./config.js"
import type { AIModelSnapshot, ActorContext, Run, RunEvent } from "./domain.js"
import { RunStateConflictError, type Repository } from "./persistence/repository.js"
import type { RemoteConfigSnapshot } from "./provider/config-client.js"
import { createConfiguredProvider } from "./provider/managed.js"
import { redact } from "./redaction.js"
import type { ToolOrchestrator } from "./tools/orchestrator.js"
import { presentEventForRun, presentTimeline } from "./timeline-presenter.js"
import type { RunStreamBus } from "./run-stream-bus.js"
import { RunStreamHubManager } from "./run-stream-hub.js"
import { maximumRequestBodyBytes, maximumTurnInputBytes, utf8ByteLength } from "./input-limits.js"
import { agentLogger, agentMetrics, captureTraceContext, errorDiagnostic, internalSpanOptions, stableErrorCode as telemetryErrorCode, telemetryLog, withSpan } from "./telemetry.js"

declare module "fastify" {
  interface FastifyRequest { actor: ActorContext }
}

const id = z.string().min(5).max(64)
const page = z.coerce.number().int().min(1).default(1)
const pageSize = z.coerce.number().int().min(1).max(100).default(20)
const timelineLimit = z.coerce.number().int().min(1).max(100).default(30)
const inputText = z.string().trim().min(1).refine(
  value => utf8ByteLength(value) <= maximumTurnInputBytes,
  { message: "input exceeds UTF-8 byte limit" },
)
const turnInput = z.object({
  parts: z.array(z.object({ type: z.literal("text"), text: inputText })).min(1).max(10),
}).refine(
  value => utf8ByteLength(value.parts.map(part => part.text).join("\n")) <= maximumTurnInputBytes,
  { message: "input exceeds UTF-8 byte limit", path: ["parts"] },
)
const aiModelSnapshot = z.object({
  id: id,
  name: z.string().trim().min(1).max(200),
  maxContextTokens: z.number().int().min(4096).max(2097152),
  maxOutputTokens: z.number().int().min(256).max(262144),
  inputCreditsPerMillion: z.string(),
  outputCreditsPerMillion: z.string(),
  cachedInputCreditsPerMillion: z.string(),
}).refine(snapshot => snapshot.maxOutputTokens < snapshot.maxContextTokens, { path: ["maxOutputTokens"] }) satisfies z.ZodType<AIModelSnapshot>
export function buildServer(input: {
  config: Config
  repository: Repository
  requestVerifier: RequestVerifier
  tools?: ToolOrchestrator
  remoteConfig?: RemoteConfigSnapshot
  cancelRun?: (runId: string) => boolean | Promise<boolean>
  toolCatalogDigest?: string | (() => string)
  streamBus?: RunStreamBus
  streamHubLimits?: { perRun: number, perInstance: number, pendingEvents?: number, pendingBytes?: number }
}): FastifyInstance {
  const toolCatalogDigest = () => typeof input.toolCatalogDigest === "function" ? input.toolCatalogDigest() : input.toolCatalogDigest
  const app = Fastify({
    ...(input.config.NODE_ENV === "test" ? { logger: false as const } : { loggerInstance: agentLogger() as unknown as FastifyBaseLogger }),
    bodyLimit: maximumRequestBodyBytes,
    logController: new LogController({ disableRequestLogging: true }),
    requestIdHeader: "x-request-id",
  })
  const streamHubs = new RunStreamHubManager(input.repository, input.streamBus, input.streamHubLimits)
  app.addHook("onClose", async () => streamHubs.close())

  app.get("/internal/health/live", { logLevel: "silent" }, async () => ({ status: "ok" }))
  app.get("/internal/health/ready", { logLevel: "silent" }, async (_request, reply) => {
    const persistence = await input.repository.readiness()
    const remoteConfig = input.remoteConfig?.current()
    const providerConfigAvailable = !input.remoteConfig || remoteConfig !== undefined
    const providerConfigured = input.remoteConfig ? remoteConfig?.provider.configured === true : true
    if (!persistence.database || !persistence.schema || !providerConfigAvailable) {
      const errorCode = !persistence.database
        ? "ai.persistence_unavailable"
        : !persistence.schema
          ? "ai.database_schema_mismatch"
          : "ai.provider_config_unavailable"
      telemetryLog("agent.readiness.failed", "warn", {
        "operation": "agent.readiness",
        "outcome": "failed",
        "error.code": errorCode,
        "error.type": "AgentReadinessError",
        "error.message": errorCode,
      })
      return reply.code(503).send({
        status: "not_ready",
        checks: { ...persistence, providerConfigAvailable, providerConfigured },
        errorCode,
      })
    }
    return { status: "ready", checks: { ...persistence, providerConfigAvailable, providerConfigured } }
  })
  app.register(async secured => {
    secured.addHook("preHandler", async request => {
      request.actor = await input.requestVerifier.verify(request.headers)
    })

    secured.post("/internal/v1/provider/test", async (_request, reply) => {
      if (!input.remoteConfig) return reply.code(503).send(errorBody("ai.provider_config_unavailable", _request.id))
      reply.header("cache-control", "no-store")
      const config = input.remoteConfig.current()
      if (!config) return reply.code(503).send(errorBody("ai.provider_config_unavailable", _request.id))
      const selectedModel = config.provider.models[0]
      if (!config.provider.configured || !selectedModel) return reply.code(409).send({ status: "not_configured", configVersion: config.version, capabilities: {} })
      const provider = createConfiguredProvider(config, selectedModel.name)
      const health = await provider.health()
      return {
        status: health.ok ? "available" : "unavailable",
        configVersion: config.version,
        capabilities: provider.capabilities(),
        ...(health.requestId ? { providerRequestId: health.requestId } : {}),
      }
    })

    secured.get("/internal/v1/conversations", async request => {
      const query = z.object({
        page,
        pageSize,
        search: z.string().trim().max(120).optional(),
        sortBy: z.literal("updatedAt").default("updatedAt"),
        sortOrder: z.enum(["asc", "desc"]).default("desc"),
      }).parse(request.query)
      const result = await input.repository.listConversations(request.actor.userId, query.page, query.pageSize, {
        ...(query.search ? { search: query.search } : {}),
        sortOrder: query.sortOrder,
      })
      return { ...result, page: query.page, pageSize: query.pageSize, sortBy: query.sortBy, sortOrder: query.sortOrder, totalPages: Math.ceil(result.total / query.pageSize) }
    })
    secured.post("/internal/v1/conversations", async (request, reply) => {
      const body = z.object({
        projectId: id.optional(),
        modelId: id,
        title: z.string().trim().min(1).max(120).default("新会话"),
      }).parse(request.body)
      if (body.title === "新会话") {
        const existing = await input.repository.findEmptyConversation(request.actor.userId, body.projectId)
        if (existing) {
          const updated = await input.repository.updateConversation(request.actor.userId, existing.id, { modelId: body.modelId })
          return reply.code(200).send(updated)
        }
      }
      const value = await input.repository.createConversation(
        request.actor.userId,
        body.title,
        body.projectId,
        body.title === "新会话" ? "default" : "user",
        body.modelId,
      )
      return reply.code(201).send(value)
    })
    secured.get("/internal/v1/conversations/:conversationId", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const value = await input.repository.getConversation(request.actor.userId, conversationId)
      return value ?? reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
    })
    secured.patch("/internal/v1/conversations/:conversationId", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const body = z.object({
        title: z.string().trim().min(1).max(120).optional(),
        modelId: id.optional(),
      }).refine(value => value.title !== undefined || value.modelId !== undefined).parse(request.body)
      const value = await input.repository.updateConversation(request.actor.userId, conversationId, {
        ...(body.title !== undefined ? { title: body.title } : {}),
        ...(body.modelId !== undefined ? { modelId: body.modelId } : {}),
      })
      return value ?? reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
    })
    secured.delete("/internal/v1/conversations/:conversationId", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const deleted = await input.repository.deleteConversation(request.actor.userId, conversationId)
      return deleted ? reply.code(204).send() : reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
    })
    secured.get("/internal/v1/conversations/:conversationId/timeline", async (request, reply) => {
      reply.header("cache-control", "no-store")
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const query = z.object({
        before: z.string().min(1).max(512).optional(),
        limit: timelineLimit,
      }).parse(request.query)
      const value = await presentTimeline(input.repository, request.actor.userId, conversationId, {
        ...(query.before ? { before: query.before } : {}),
        limit: query.limit,
      })
      return value ?? reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
    })
    secured.post("/internal/v1/conversations/:conversationId/turns", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const key = request.headers["idempotency-key"]
      if (typeof key !== "string" || key.length < 8 || key.length > 128) return reply.code(400).send(errorBody("idempotency_key_required", request.id))
      const body = z.object({
        input: turnInput,
        modelId: id,
        modelSnapshot: aiModelSnapshot.optional(),
        pageContext: z.record(z.string(), z.unknown()).default({}),
        runId: id.optional(),
      }).parse(request.body)
      const catalogDigest = toolCatalogDigest()
      const created = await withSpan("agent.turn.accept", internalSpanOptions({
        "luna.operation.name": "create_turn",
        "gen_ai.conversation.id": conversationId,
      }), async span => {
        const value = await input.repository.createTurn(request.actor.userId, {
          conversationId, input: body.input.parts.map(part => part.text).join("\n"), pageContext: redact(body.pageContext),
          traceContext: captureTraceContext(request.headers),
          actorSessionId: request.actor.sessionId,
          idempotencyKey: key, ...(body.runId ? { preallocatedRunId: body.runId } : {}),
          ...(catalogDigest ? { toolCatalogDigest: catalogDigest } : {}),
          modelId: body.modelId,
          ...(body.modelSnapshot ? { modelSnapshot: body.modelSnapshot } : {}),
        })
        span.setAttribute("luna.turn.id", value.turn.id)
        span.setAttribute("luna.run.id", value.run.id)
        return value
      })
      return reply.code(202).send({ turnId: created.turn.id, turnIndex: created.turn.turnIndex, runId: created.run.id, state: created.run.status, eventsUrl: `/api/v1/ai/runs/${created.run.id}/events` })
    })
    secured.post("/internal/v1/conversations/:conversationId/tool-actions", async (request, reply) => {
      if (!input.tools) return reply.code(503).send(errorBody("ai.tool_not_available", request.id))
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const key = request.headers["idempotency-key"]
      if (typeof key !== "string" || key.length < 8 || key.length > 128) return reply.code(400).send(errorBody("idempotency_key_required", request.id))
      const body = z.object({
        operationId: z.string().regex(/^[a-zA-Z][a-zA-Z0-9_.-]{2,100}$/),
        arguments: z.record(z.string(), z.unknown()),
        message: inputText,
        runId: id,
      }).parse(request.body)
      if (body.runId !== request.actor.runId) return reply.code(409).send(errorBody("ai.run_state_conflict", request.id))
      const catalogDigest = toolCatalogDigest()
      const created = await withSpan("agent.tool_action.accept", internalSpanOptions({
        "luna.operation.name": "create_tool_action",
        "gen_ai.conversation.id": conversationId,
      }), async span => {
        const value = await input.repository.createTurn(request.actor.userId, {
          conversationId,
          input: body.message,
          pageContext: { __lunaDirectToolAction: true },
          traceContext: captureTraceContext(request.headers),
          actorSessionId: request.actor.sessionId,
          idempotencyKey: key,
          preallocatedRunId: body.runId,
          ...(catalogDigest ? { toolCatalogDigest: catalogDigest } : {}),
        })
        span.setAttribute("luna.turn.id", value.turn.id)
        span.setAttribute("luna.run.id", value.run.id)
        return value
      })
      try {
        await input.repository.touchRunSelectedOperations(created.run.id, [body.operationId], 16)
        const call = await input.tools.propose({ runId: created.run.id, operationId: body.operationId, arguments: body.arguments, inputMode: "direct" })
		if (call.status === "awaiting_approval")
		  await input.repository.updateRun(created.run.id, "queued", "waiting_approval")
		else
          await input.repository.updateRun(created.run.id, "queued", call.status === "failed" ? "failed" : "completed", { completedAt: new Date().toISOString() })
        const run = await input.repository.getRun(request.actor.userId, created.run.id)
        return reply.code(202).send({ turnId: created.turn.id, turnIndex: created.turn.turnIndex, runId: created.run.id, state: run?.status ?? "completed", toolCallId: call.id, eventsUrl: `/api/v1/ai/runs/${created.run.id}/events` })
      } catch (error) {
        try { await input.repository.updateRun(created.run.id, "queued", "failed", { completedAt: new Date().toISOString(), errorCode: telemetryErrorCode(error) }) } catch { /* the tool may already have transitioned the run */ }
        throw error
      }
    })
    secured.get("/internal/v1/runs/:runId", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      const value = await input.repository.getRun(request.actor.userId, runId)
      return value ? presentRun(value) : reply.code(404).send(errorBody("ai.run_not_found", request.id))
    })
    secured.post("/internal/v1/runs/:runId/cancel", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      const current = await input.repository.getRun(request.actor.userId, runId)
      if (!current) return reply.code(404).send(errorBody("ai.run_not_found", request.id))
      const wasTerminal = isTerminalRun(current)
      const value = wasTerminal
        ? current
        : await input.repository.cancelRun(request.actor.userId, runId)
      if (!value) return reply.code(404).send(errorBody("ai.run_not_found", request.id))
      if (!wasTerminal && value.status === "canceled") {
        try { await input.cancelRun?.(runId) } catch { /* 权威 canceled 已持久化，本地中止仅为 best effort。 */ }
      }
      if (isTerminalRun(value)) await input.streamBus?.cleanup(runId).catch(() => undefined)
      return reply.code(202).send(presentRun(value))
    })
    secured.post("/internal/v1/runs/:runId/approvals/:toolCallId/decision", async (request, reply) => {
      if (!input.tools) return reply.code(503).send(errorBody("ai.tool_not_available", request.id))
      const { runId, toolCallId } = z.object({ runId: id, toolCallId: id }).parse(request.params)
      const body = z.object({
		decision: z.enum(["reject", "approve"]),
	  }).parse(request.body)
      const run = await input.repository.getRun(request.actor.userId, runId)
      if (!run || run.status !== "waiting_approval") return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      const selected = await input.tools.inspect(toolCallId)
      if (selected.runId !== runId) return reply.code(404).send(errorBody("ai.tool_call_not_found", request.id))
      if (body.decision === "reject") {
        try {
          await input.tools.resolveApproval(toolCallId, "reject")
          await input.repository.updateRun(runId, "waiting_approval", "queued")
          return reply.code(202).send({ runId, state: "queued" })
        }
        catch (error) {
          const latest = await input.repository.getRun(request.actor.userId, runId)
          if (!latest || latest.status !== "waiting_approval")
            return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
          throw error
        }
      }
      // 批准后的平台调用必须在权威 Run 处于 running 时执行，Go 执行身份
      // 中间件会据此拒绝过期、并发或跨 Run 的 ToolCall。running 不会被
      // queued claim 领取，因此同步执行期间不存在第二个 Executor 竞争。
      try {
        await input.repository.updateRun(runId, "waiting_approval", "running")
      }
      catch (error) {
        if (error instanceof RunStateConflictError)
          return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
        throw error
      }
      try {
        const resolved = await input.tools.resolveApproval(toolCallId, body.decision)
        if (resolved.status === "awaiting_approval") {
          await input.repository.updateRun(runId, "running", "waiting_approval")
          const waiting = await input.repository.getRun(request.actor.userId, runId)
          return waiting ? presentRun(waiting) : reply.code(404).send(errorBody("ai.run_not_found", request.id))
        }
        await input.repository.updateRun(runId, "running", "queued")
        return reply.code(202).send({ runId, state: "queued" })
      }
      catch (error) {
        const latest = await input.repository.getRun(request.actor.userId, runId)
        if (!latest || latest.status !== "running")
          return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
        try {
          await input.repository.updateRun(runId, "running", "queued")
        }
        catch {
          const afterConflict = await input.repository.getRun(request.actor.userId, runId)
          if (!afterConflict || afterConflict.status !== "running")
            return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
          throw error
        }
        throw error
      }
	})
    secured.post("/internal/v1/runs/:runId/input", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      const body = z.object({ text: inputText, expectedVersion: z.number().int() }).parse(request.body)
      const run = await input.repository.getRun(request.actor.userId, runId)
      if (!run || run.status !== "waiting_input" || run.rowVersion !== body.expectedVersion) return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      await input.repository.appendRunInput(runId, body.text)
      await input.repository.appendEvent(runId, "run.input_received", { length: body.text.length })
      await input.repository.updateRun(runId, "waiting_input", "queued")
      return reply.code(202).send({ runId, state: "queued" })
    })
    secured.get("/internal/v1/runs/:runId/events", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      let run = await input.repository.getRun(request.actor.userId, runId)
      if (!run) return reply.code(404).send(errorBody("ai.run_not_found", request.id))
      const query = z.object({ after: z.coerce.number().int().min(0).optional(), stream: z.enum(["true", "false"]).optional() }).parse(request.query)
      const headerCursor = typeof request.headers["last-event-id"] === "string" ? Number(request.headers["last-event-id"]) : 0
      const after = Math.max(query.after ?? 0, Number.isSafeInteger(headerCursor) && headerCursor >= 0 ? headerCursor : 0)
      const acceptsEventStream = request.headers.accept?.split(",").some(value => value.trim().split(";")[0] === "text/event-stream") ?? false
      const streamRequested = acceptsEventStream || query.stream === "true"
      const subscription = streamRequested && !isTerminalRun(run)
        ? await streamHubs.subscribe(runId, request.actor.userId, after)
        : undefined
      try {
      let durableEvents: RunEvent[]
      try { durableEvents = await input.repository.getEvents(request.actor.userId, runId, after) }
      catch (error) {
        subscription?.close()
        throw error
      }
      let liveEvents: typeof durableEvents = []
      let liveReplayFailed = false
      try { liveEvents = input.streamBus ? await input.streamBus.read(runId, after) : [] }
      catch (error) {
        liveReplayFailed = true
        telemetryLog("agent.stream.replay_failed", "warn", {
          operation: "agent.stream.replay", outcome: "failed",
          ...errorDiagnostic(error, "ai.stream_transport_unavailable"),
        })
      }
      const events = mergeEvents(durableEvents, liveEvents)
      const presentedEvents = events.filter(isPublicStreamEvent).map(event => presentEventForRun(run!, event))
      if (!streamRequested) return { items: presentedEvents, cursor: events.at(-1)?.sequence ?? after }
      if (liveReplayFailed && !isTerminalRun(run)) {
        subscription?.close()
        return reply.code(503).send(errorBody("ai.stream_transport_unavailable", request.id))
      }
      reply.raw.setHeader("content-type", "text/event-stream")
      reply.raw.setHeader("cache-control", "no-cache, no-store")
      reply.raw.setHeader("x-accel-buffering", "no")
      reply.hijack()
      let cursor = after
      let closed = false
      const streamAbort = new AbortController()
      request.raw.once("close", () => { closed = true; streamAbort.abort() })
      const push = async (batch: typeof events) => {
        for (const event of batch) {
          if (isPublicStreamEvent(event)) {
            const presented = presentEventForRun(run!, event)
            if (!await writeSSE(reply.raw, sse(event.type, event.sequence, presented), streamAbort.signal)) return
          }
          cursor = event.sequence
        }
      }
      await push(events)
      subscription?.advance(cursor)
      let heartbeatAt = Date.now()
      try {
        while (!closed) {
          if (!subscription) break
          const update = await subscription.next(streamAbort.signal)
          if (update.error) {
            telemetryLog("agent.stream.read_failed", "warn", {
              operation: "agent.stream.read", outcome: "failed",
              ...errorDiagnostic(update.error, "ai.stream_transport_unavailable"),
            })
            if (!isTerminalRun(run)) {
              reply.raw.destroy(new Error("ai.stream_transport_unavailable"))
            }
            break
          }
          if (update.run) run = update.run
          await push(update.events)
          subscription.advance(cursor)
          if (update.terminal && update.events.length === 0) break
          if (Date.now() - heartbeatAt >= 15_000) {
            if (!await writeSSE(reply.raw, sseHeartbeat(runId, run.conversationId), streamAbort.signal)) break
            heartbeatAt = Date.now()
          }
        }
      }
      finally { subscription?.close() }
      if (!closed) reply.raw.end()
      return reply
      }
      finally { subscription?.close() }
    })
  })

  app.setErrorHandler((error, request, reply) => {
    const normalized = error instanceof Error ? error : new Error("ai.internal_error")
    const code = normalized instanceof z.ZodError ? "invalid_request" : stableCode(normalized.message)
    const status = code === "ai.unauthorized"
      ? 401
      : code.endsWith("_not_found")
        ? 404
        : code === "ai.stream_subscriber_limit"
          ? 429
          : code === "ai.stream_transport_unavailable"
            ? 503
            : code === "idempotency_conflict"
              ? 409
              : code === "ai.internal_error"
                ? 500
                : 400
    telemetryLog("agent.http.request_failed", status >= 500 ? "error" : "warn", {
      "operation": "agent.http.request",
      "outcome": status >= 500 ? "failed" : "rejected",
      "http.request.method": request.method,
      "http.route": request.routeOptions.url,
      "http.response.status_code": status,
      "request_id": request.id,
      ...(normalized instanceof z.ZodError
        ? errorDiagnostic(new Error("request schema validation failed"), code)
        : errorDiagnostic(normalized, telemetryErrorCode(normalized))),
    })
    void reply.code(status).send(errorBody(code, request.id))
  })
  return app
}

function presentRun(run: Run): Omit<Run, "traceContext"> {
  const publicRun = { ...run }
  delete publicRun.traceContext
  return publicRun
}

function errorBody(code: string, requestId: string) { return { error: { code, requestId } } }
function stableCode(message: string) { return /^(ai\.|idempotency_)[a-z0-9_.]+$/.test(message) ? message : "ai.internal_error" }
function sse(type: string, idValue: number, data: unknown) { return `id: ${idValue}\nevent: ${type}\ndata: ${JSON.stringify(data)}\n\n` }
function sseHeartbeat(runId: string, conversationId: string) {
  return `event: stream.heartbeat\ndata: ${JSON.stringify({ version: 1, type: "stream.heartbeat", runId, conversationId, occurredAt: new Date().toISOString() })}\n\n`
}
function mergeEvents(...groups: RunEvent[][]) {
  const unique = new Map<string, RunEvent>()
  for (const event of groups.flat()) unique.set(event.id, event)
  return [...unique.values()].sort((left, right) => left.sequence - right.sequence)
}
function isPublicStreamEvent(event: RunEvent) { return event.type !== "run.cancel_requested" }
function isTerminalRun(run: Run | undefined) {
  return Boolean(run && ["completed", "failed", "canceled", "expired", "interrupted"].includes(run.status))
}
const sseDrainDeadlineMs = 10_000
export async function writeSSE(stream: NodeJS.WritableStream, chunk: string, signal: AbortSignal): Promise<boolean> {
  if (signal.aborted) return false
  if (stream.write(chunk)) return true
  const startedAt = performance.now()
  return new Promise(resolve => {
    let settled = false
    const aborted = () => finish(false, "aborted")
    const drained = () => finish(true, "drained")
    const closed = () => finish(false, "closed")
    const failed = () => finish(false, "failed")
    const deadline = setTimeout(() => {
      finish(false, "timeout")
      const destroy = (stream as NodeJS.WritableStream & { destroy?: () => void }).destroy
      if (typeof destroy === "function") destroy.call(stream)
    }, sseDrainDeadlineMs)
    deadline.unref?.()
    const cleanup = () => {
      clearTimeout(deadline)
      signal.removeEventListener("abort", aborted)
      stream.removeListener("drain", drained)
      stream.removeListener("close", closed)
      stream.removeListener("error", failed)
    }
    const finish = (value: boolean, outcome: "aborted" | "drained" | "closed" | "failed" | "timeout") => {
      if (settled) return
      settled = true
      cleanup()
      agentMetrics.sseBackpressureDuration.record((performance.now() - startedAt) / 1000, {
        outcome,
      })
      resolve(value)
    }
    signal.addEventListener("abort", aborted, { once: true })
    stream.once("drain", drained)
    stream.once("close", closed)
    stream.once("error", failed)
  })
}
