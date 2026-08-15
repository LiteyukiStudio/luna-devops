import Fastify, { type FastifyInstance } from "fastify"
import { z } from "zod"
import type { RequestAuthenticator } from "./auth.js"
import type { Config } from "./config.js"
import type { ActorContext, Run } from "./domain.js"
import type { PayloadCipher } from "./payload-cipher.js"
import type { Repository } from "./persistence/repository.js"
import type { ModelProvider } from "./provider/provider.js"
import type { ProviderConfigClient } from "./provider/config-client.js"
import { OpenAICompatibleProvider } from "./provider/openai-compatible.js"
import { redact } from "./redaction.js"
import type { ToolOrchestrator } from "./tools/orchestrator.js"
import { presentEvent, presentTimeline } from "./timeline-presenter.js"
import { defaultRuntimeSettings } from "./runtime-settings.js"
import { captureTraceContext, internalSpanOptions, stableErrorCode as telemetryErrorCode, telemetryLog, withSpan } from "./telemetry.js"

declare module "fastify" {
  interface FastifyRequest { actor: ActorContext }
}

const id = z.string().min(5).max(64)
const clientInstanceId = z.string().regex(/^[A-Za-z0-9_-]{16,80}$/)
const relativePath = z.string().trim().min(1).max(2048).regex(/^\/(?!\/)/)
const stableErrorCode = z.string().trim().min(3).max(120).regex(/^[a-z][a-z0-9_.-]+$/)
const page = z.coerce.number().int().min(1).default(1)
const pageSize = z.coerce.number().int().min(1).max(100).default(20)

export function buildServer(input: {
  config: Config
  repository: Repository
  authenticator: RequestAuthenticator
  provider: ModelProvider
  grantCipher: PayloadCipher
  tools?: ToolOrchestrator
  providerConfigClient?: ProviderConfigClient
  cancelRun?: (runId: string) => void
  toolCatalogDigest?: string
}): FastifyInstance {
  const app = Fastify({
    logger: input.config.NODE_ENV === "test" ? false : {
      level: "info",
      redact: { paths: ["req.headers.authorization", "req.headers.x-luna-actor-context", "*.apiKey", "*.token", "*.secret"], censor: "[REDACTED]" },
      serializers: {
        req(request) {
          return {
            method: request.method,
            url: String(request.url ?? "").split("?", 1)[0] ?? "",
            host: request.host,
            remoteAddress: request.raw.socket.remoteAddress ?? "",
          }
        },
      },
    },
    bodyLimit: 256 * 1024,
    requestIdHeader: "x-request-id",
  })

  app.get("/internal/health/live", { logLevel: "silent" }, async () => ({ status: "ok" }))
  app.get("/internal/health/ready", { logLevel: "silent" }, async (_request, reply) => {
    const database = await input.repository.health()
    if (!database) {
      telemetryLog("agent.readiness.failed", "warn", { "error.code": "ai.persistence_unavailable" })
      return reply.code(503).send({ status: "not_ready", checks: { database } })
    }
    return { status: "ready", checks: { database, providerConfigured: true } }
  })
  app.get("/internal/v1/health/compatibility", async () => ({
    component: "luna-agent", version: "0.1.0", internalApiVersions: ["v1"],
    aiSchemaMin: 1, aiSchemaMax: 1,
    toolCatalogDigest: input.toolCatalogDigest ?? "sha256:platform-tools-v1", promptVersions: ["system-v4"],
  }))

  app.register(async secured => {
    secured.addHook("preHandler", async request => {
      request.actor = await input.authenticator.verify(request.headers)
    })

    secured.get("/internal/v1/capabilities", async () => {
      const runtime = input.providerConfigClient
        ? await input.providerConfigClient.get().then(config => config.runtime).catch(() => defaultRuntimeSettings)
        : defaultRuntimeSettings
      return {
        mode: input.tools ? "controlled_tools" : "read_only",
        features: {
          conversations: true, streaming: true, cancel: true, uiActions: true,
          approvals: Boolean(input.tools),
          stepUpMFA: Boolean(input.tools),
          longTermMemory: false,
          mfa: Boolean(input.tools),
          toolCalling: Boolean(input.tools),
        },
        limits: {
          maxInputBytes: runtime.maxInputBytes,
          maxConcurrentRuns: runtime.agentConcurrentRuns,
          maxUserConcurrentRuns: runtime.userConcurrentRuns,
          contextInputTokenBudget: runtime.contextInputTokenBudget,
        },
        provider: input.provider.capabilities(),
      }
    })

    secured.get("/internal/v1/provider/health", async () => redact(await input.provider.health()))
    secured.post("/internal/v1/provider/test", async (_request, reply) => {
      if (!input.providerConfigClient) return reply.code(503).send(errorBody("ai.provider_config_unavailable", _request.id))
      reply.header("cache-control", "no-store")
      const config = await input.providerConfigClient.get()
      if (!config.provider.configured) return reply.code(409).send({ status: "not_configured", configVersion: config.version, capabilities: {} })
      const provider = new OpenAICompatibleProvider({
        baseUrl: config.provider.baseUrl, apiKey: config.provider.apiKey,
        model: config.provider.model, timeoutMs: config.runtime.providerTimeoutMs,
        maxRetries: config.runtime.maxRequestRetries,
      })
      const health = await provider.health()
      return {
        status: health.ok ? "available" : "unavailable",
        configVersion: config.version,
        capabilities: provider.capabilities(),
        ...(health.requestId ? { providerRequestId: health.requestId } : {}),
      }
    })

    secured.get("/internal/v1/conversations", async request => {
      const query = z.object({ page, pageSize, sortBy: z.literal("updatedAt").default("updatedAt"), sortOrder: z.enum(["asc", "desc"]).default("desc") }).parse(request.query)
      const result = await input.repository.listConversations(request.actor.userId, query.page, query.pageSize)
      return { ...result, page: query.page, pageSize: query.pageSize, sortBy: query.sortBy, sortOrder: query.sortOrder, totalPages: Math.ceil(result.total / query.pageSize) }
    })
    secured.post("/internal/v1/conversations", async (request, reply) => {
      const body = z.object({ projectId: id.optional(), title: z.string().trim().min(1).max(120).default("新会话") }).parse(request.body)
      if (body.title === "新会话") {
        const existing = await input.repository.findEmptyConversation(request.actor.userId, body.projectId)
        if (existing) return reply.code(200).send(existing)
      }
      const value = await input.repository.createConversation(
        request.actor.userId,
        body.title,
        body.projectId,
        body.title === "新会话" ? "default" : "user",
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
      const { title } = z.object({ title: z.string().trim().min(1).max(120) }).parse(request.body)
      const value = await input.repository.renameConversation(request.actor.userId, conversationId, title)
      return value ?? reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
    })
    secured.delete("/internal/v1/conversations/:conversationId", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const deleted = await input.repository.deleteConversation(request.actor.userId, conversationId)
      return deleted ? reply.code(204).send() : reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
    })
    secured.get("/internal/v1/conversations/:conversationId/timeline", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const value = await presentTimeline(input.repository, request.actor.userId, conversationId)
      return value ?? reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
    })
    secured.post("/internal/v1/conversations/:conversationId/turns", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const key = request.headers["idempotency-key"]
      if (typeof key !== "string" || key.length < 8 || key.length > 128) return reply.code(400).send(errorBody("idempotency_key_required", request.id))
      const body = z.object({
        input: z.object({ parts: z.array(z.object({ type: z.literal("text"), text: z.string().trim().min(1).max(12000) })).min(1).max(10) }),
        pageContext: z.record(z.string(), z.unknown()).default({}),
        clientInstanceId,
        runId: id.optional(),
        runActorGrant: z.string().min(16).max(8192).optional(),
      }).parse(request.body)
      const created = await withSpan("agent.turn.accept", internalSpanOptions({
        "gen_ai.operation.name": "create_turn",
        "gen_ai.conversation.id": conversationId,
      }), async span => {
        const value = await input.repository.createTurn(request.actor.userId, {
          conversationId, input: body.input.parts.map(part => part.text).join("\n"), pageContext: redact(body.pageContext),
          traceContext: captureTraceContext(),
          idempotencyKey: key, ...(body.runId ? { preallocatedRunId: body.runId } : {}),
          ...(body.runActorGrant ? { runActorGrantCiphertext: input.grantCipher.encrypt(body.runActorGrant) } : {}),
          ...(input.toolCatalogDigest ? { toolCatalogDigest: input.toolCatalogDigest } : {}),
          clientInstanceId: body.clientInstanceId,
        })
        span.setAttribute("luna.turn.id", value.turn.id)
        span.setAttribute("luna.run.id", value.run.id)
        return value
      })
      return reply.code(202).send({ turnId: created.turn.id, turnIndex: created.turn.turnIndex, runId: created.run.id, state: created.run.status, eventsUrl: `/api/v1/ai/runs/${created.run.id}/events` })
    })
    secured.get("/internal/v1/ui-actions/pending", async request => {
      const query = z.object({ clientInstanceId }).parse(request.query)
      const items = await input.repository.listPendingUIActions(request.actor.userId, query.clientInstanceId)
      return {
        items: items.map(item => ({
          actionId: item.id,
          runId: item.runId,
          toolCallId: item.toolCallId,
          action: item.action,
          attempts: item.attempts,
          expiresAt: item.expiresAt,
        })),
      }
    })
    secured.post("/internal/v1/ui-actions/:actionId/ack", async (request, reply) => {
      const { actionId } = z.object({ actionId: id }).parse(request.params)
      const body = z.object({
        clientInstanceId,
        status: z.enum(["succeeded", "failed"]),
        actualPath: relativePath.optional(),
        errorCode: stableErrorCode.optional(),
      }).superRefine((value, context) => {
        if (value.status === "succeeded" && !value.actualPath) {
          context.addIssue({ code: "custom", path: ["actualPath"], message: "actualPath is required after successful navigation" })
        }
        if (value.status === "failed" && !value.errorCode) {
          context.addIssue({ code: "custom", path: ["errorCode"], message: "errorCode is required after failed navigation" })
        }
      }).parse(request.body)
      const action = await input.repository.acknowledgeUIAction(request.actor.userId, body.clientInstanceId, actionId, {
        status: body.status,
        ...(body.actualPath ? { actualPath: body.actualPath } : {}),
        ...(body.errorCode ? { errorCode: body.errorCode } : {}),
      })
      if (!action) return reply.code(404).send(errorBody("ai.ui_action_not_found", request.id))
      if (action.status !== body.status) return reply.code(409).send(errorBody("ai.ui_action_not_pending", request.id))
      return reply.code(202).send({
        actionId: action.id,
        status: action.status,
        acknowledgedAt: action.acknowledgedAt,
      })
    })
    secured.get("/internal/v1/runs/:runId", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      const value = await input.repository.getRun(request.actor.userId, runId)
      return value ? presentRun(value) : reply.code(404).send(errorBody("ai.run_not_found", request.id))
    })
    secured.post("/internal/v1/runs/:runId/cancel", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      const value = await input.repository.cancelRun(request.actor.userId, runId)
      if (value?.status === "canceled") {
        try { input.cancelRun?.(runId) } catch {
          // The durable canceled state is authoritative; a local abort is only a latency optimization.
        }
      }
      return value ? presentRun(value) : reply.code(404).send(errorBody("ai.run_not_found", request.id))
    })
    secured.post("/internal/v1/runs/:runId/approvals/:toolCallId/decision", async (request, reply) => {
      if (!input.tools) return reply.code(503).send(errorBody("ai.tool_not_available", request.id))
      const { runId, toolCallId } = z.object({ runId: id, toolCallId: id }).parse(request.params)
      const body = z.object({ decision: z.enum(["approve", "reject", "approve_all"]), argumentsHash: z.string(), expectedVersion: z.number().int(), reason: z.string().max(500).optional() }).parse(request.body)
      const run = await input.repository.getRun(request.actor.userId, runId)
      if (!run || run.status !== "waiting_approval") return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      const selected = await input.tools.inspect(toolCallId)
      if (selected.runId !== runId) return reply.code(404).send(errorBody("ai.tool_call_not_found", request.id))
      if (body.decision === "reject") {
        await input.tools.reject(toolCallId, body.argumentsHash, body.expectedVersion)
        const canceled = await input.repository.cancelRun(request.actor.userId, runId)
        return canceled ? presentRun(canceled) : canceled
      }
      const approved = [await input.tools.approve(toolCallId, body.argumentsHash, body.expectedVersion)]
      if (body.decision === "approve_all") approved.push(...await input.tools.approveAll(runId))
      if (approved.some(call => call.status === "awaiting_mfa")) return presentRun(await input.repository.updateRun(runId, "waiting_approval", "waiting_mfa"))
      await input.repository.updateRun(runId, "waiting_approval", "queued")
      return reply.code(202).send({ runId, state: "queued" })
    })
    secured.post("/internal/v1/runs/:runId/mfa/:toolCallId/resume", async (request, reply) => {
      if (!input.tools) return reply.code(503).send(errorBody("ai.tool_not_available", request.id))
      const { runId, toolCallId } = z.object({ runId: id, toolCallId: id }).parse(request.params)
      const body = z.object({ purpose: z.string().min(1), expectedVersion: z.number().int(), stepUpAssertionId: z.string().min(1) }).parse(request.body)
      const run = await input.repository.getRun(request.actor.userId, runId)
      if (!run || run.status !== "waiting_mfa") return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      await input.tools.resumeMfa(toolCallId, body.purpose, body.expectedVersion, body.stepUpAssertionId)
      await input.repository.updateRun(runId, "waiting_mfa", "queued")
      return reply.code(202).send({ runId, state: "queued" })
    })
    secured.post("/internal/v1/runs/:runId/input", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      const body = z.object({ text: z.string().trim().min(1).max(12000), expectedVersion: z.number().int() }).parse(request.body)
      const run = await input.repository.getRun(request.actor.userId, runId)
      if (!run || run.status !== "waiting_input" || run.rowVersion !== body.expectedVersion) return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      await input.repository.appendRunInput(runId, body.text)
      await input.repository.appendEvent(runId, "run.input_received", { length: body.text.length })
      await input.repository.updateRun(runId, "waiting_input", "queued")
      return reply.code(202).send({ runId, state: "queued" })
    })
    secured.get("/internal/v1/runs/:runId/events", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      if (!await input.repository.getRun(request.actor.userId, runId)) return reply.code(404).send(errorBody("ai.run_not_found", request.id))
      const query = z.object({ after: z.coerce.number().int().min(0).optional(), stream: z.enum(["true", "false"]).optional() }).parse(request.query)
      const headerCursor = typeof request.headers["last-event-id"] === "string" ? Number(request.headers["last-event-id"]) : 0
      const after = Math.max(query.after ?? 0, Number.isSafeInteger(headerCursor) && headerCursor >= 0 ? headerCursor : 0)
      const events = await input.repository.getEvents(request.actor.userId, runId, after)
      const presentedEvents = (await Promise.all(events.map(event => presentEvent(input.repository, request.actor.userId, event)))).filter(event => event !== undefined)
      const acceptsEventStream = request.headers.accept?.split(",").some(value => value.trim().split(";")[0] === "text/event-stream") ?? false
      const streamRequested = acceptsEventStream || query.stream === "true"
      if (!streamRequested) return { items: presentedEvents, cursor: events.at(-1)?.sequence ?? after }
      reply.raw.setHeader("content-type", "text/event-stream")
      reply.raw.setHeader("cache-control", "no-cache, no-store")
      reply.raw.setHeader("x-accel-buffering", "no")
      reply.hijack()
      let cursor = after
      let closed = false
      request.raw.once("close", () => { closed = true })
      const push = async (batch: typeof events) => {
        for (const event of batch) {
          const presented = await presentEvent(input.repository, request.actor.userId, event)
          if (presented) reply.raw.write(sse(event.type, event.sequence, presented))
          cursor = event.sequence
        }
      }
      await push(events)
      let heartbeatAt = Date.now()
      while (!closed) {
        const run = await input.repository.getRun(request.actor.userId, runId)
        const batch = await input.repository.getEvents(request.actor.userId, runId, cursor)
        await push(batch)
        if (run && ["completed", "failed", "canceled", "expired"].includes(run.status) && batch.length === 0) break
        if (Date.now() - heartbeatAt >= 15_000) {
          reply.raw.write(": heartbeat\n\n")
          heartbeatAt = Date.now()
        }
        await delay(100)
      }
      if (!closed) reply.raw.end()
      return reply
    })
  })

  app.setErrorHandler((error, request, reply) => {
    const normalized = error instanceof Error ? error : new Error("ai.internal_error")
    const code = normalized instanceof z.ZodError ? "invalid_request" : stableCode(normalized.message)
    const status = code === "ai.unauthorized" ? 401 : code.endsWith("_not_found") ? 404 : code === "idempotency_conflict" ? 409 : 400
    telemetryLog("agent.http.request_failed", "error", {
      "http.request.method": request.method,
      "http.route": request.routeOptions.url,
      "http.response.status_code": status,
      "request.id": request.id,
      "error.type": normalized.name,
      "error.code": telemetryErrorCode(normalized),
    })
    request.log.warn({ err: { code, name: normalized.name }, requestId: request.id }, "agent request failed")
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
function delay(milliseconds: number) { return new Promise(resolve => setTimeout(resolve, milliseconds)) }
