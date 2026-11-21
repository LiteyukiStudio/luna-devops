import Fastify, { type FastifyBaseLogger, type FastifyInstance, LogController } from "fastify"
import { z } from "zod"
import type { RequestVerifier } from "./auth.js"
import type { Config } from "./config.js"
import type { AIModelSnapshot, ActorContext, Run, RunEvent } from "./domain.js"
import { conversationApprovalModes } from "./domain.js"
import { RunStateConflictError, type Repository } from "./persistence/repository.js"
import type { RemoteConfigSource } from "./provider/config-client.js"
import { createConfiguredProvider, providerDialect } from "./provider/managed.js"
import { redact } from "./redaction.js"
import type { ToolOrchestrator } from "./tools/orchestrator.js"
import { presentEventForRun, presentTimeline } from "./timeline-presenter.js"
import type { RunStreamBus } from "./run-stream-bus.js"
import { RunStreamHubManager } from "./run-stream-hub.js"
import { maximumRequestBodyBytes, maximumTurnInputBytes, utf8ByteLength } from "./input-limits.js"
import { createId } from "./id.js"
import { type HeadlessInferenceRunner, headlessInferenceInputSchema } from "./headless-inference.js"
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
export type BuildServerDependencies = {
  config: Config
  repository: Repository
  requestVerifier: RequestVerifier
  tools: ToolOrchestrator
  remoteConfig: Pick<RemoteConfigSource, "current">
  cancelRun: (runId: string) => boolean | Promise<boolean>
  cancelExecutingRun: (runId: string) => Promise<boolean>
  notifyRunQueued?: () => void
  toolCatalogDigest: (platformRole?: string) => string
  streamBus: RunStreamBus
  streamHubLimits?: { perRun: number, perInstance: number, pendingEvents?: number, pendingBytes?: number }
  headlessInference?: HeadlessInferenceRunner
}

export function buildServer(input: BuildServerDependencies): FastifyInstance {
  const app = Fastify({
    ...(input.config.NODE_ENV === "test" ? { logger: false as const } : { loggerInstance: agentLogger() as unknown as FastifyBaseLogger }),
    bodyLimit: maximumRequestBodyBytes,
    logController: new LogController({ disableRequestLogging: true }),
    requestIdHeader: "x-request-id",
  })
  const streamHubs = new RunStreamHubManager(input.repository, input.streamBus, input.streamHubLimits)
  const toolActionLocks = new Map<string, Promise<void>>()
  const withToolActionLock = async <T>(lockKey: string, work: () => Promise<T>): Promise<T> => {
    const previous = toolActionLocks.get(lockKey) ?? Promise.resolve()
    let release!: () => void
    const current = new Promise<void>((resolve) => { release = resolve })
    const chained = previous.then(() => current, () => current)
    toolActionLocks.set(lockKey, chained)
    await previous.catch(() => undefined)
    try {
      return await work()
    }
    finally {
      release()
      if (toolActionLocks.get(lockKey) === chained) {
        toolActionLocks.delete(lockKey)
      }
    }
  }
  app.addHook("onClose", async () => streamHubs.close())

  app.get("/internal/health/live", { logLevel: "silent" }, async (_request, reply) => {
    reply.header("cache-control", "no-store")
    return { status: "ok" }
  })
  app.get("/internal/health/ready", { logLevel: "silent" }, async (_request, reply) => {
    reply.header("cache-control", "no-store")
    const persistence = await input.repository.readiness()
    const remoteConfig = input.remoteConfig.current()
    const providerConfigAvailable = remoteConfig !== undefined
    const providerConfigured = remoteConfig?.provider.configured === true
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
    secured.addHook("preHandler", async (request, reply) => {
      reply.header("cache-control", "no-store")
      request.actor = await input.requestVerifier.verify(request.headers)
    })

    secured.post("/internal/v1/provider/test", async (_request, reply) => {
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
        providerDialect: providerDialect(config, selectedModel.name),
        capabilities: provider.capabilities(),
        ...(health.requestId ? { providerRequestId: health.requestId } : {}),
      }
    })

    secured.post("/internal/v1/inference", async (request, reply) => {
      reply.header("cache-control", "no-store")
      if (!input.headlessInference) {
        return reply.code(503).send(errorBody("ai.not_configured", request.id))
      }
      const body = headlessInferenceInputSchema.parse(request.body)
      const abort = new AbortController()
      request.raw.on("close", () => {
        if (!reply.sent) abort.abort(new Error("ai.run_canceled"))
      })
      return input.headlessInference.run(request.actor, body, abort.signal)
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
    const approvalModeSchema = z.enum(conversationApprovalModes)
    secured.post("/internal/v1/conversations", async (request, reply) => {
      const body = z.object({
        projectId: id.optional(),
        modelId: id,
        title: z.string().trim().min(1).max(120).default("新会话"),
        approvalMode: approvalModeSchema.default("default"),
      }).parse(request.body)
      if (body.title === "新会话") {
        const existing = await input.repository.findEmptyConversation(request.actor.userId, body.projectId)
        if (existing) {
          const updated = await input.repository.updateConversation(request.actor.userId, existing.id, {
            modelId: body.modelId,
            approvalMode: body.approvalMode,
          })
          return reply.code(200).send(updated)
        }
      }
      const value = await input.repository.createConversation(
        request.actor.userId,
        body.title,
        body.projectId,
        body.title === "新会话" ? "default" : "user",
        body.modelId,
        body.approvalMode,
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
        approvalMode: approvalModeSchema.optional(),
      }).refine(value => value.title !== undefined || value.modelId !== undefined || value.approvalMode !== undefined).parse(request.body)
      const value = await input.repository.updateConversation(request.actor.userId, conversationId, {
        ...(body.title !== undefined ? { title: body.title } : {}),
        ...(body.modelId !== undefined ? { modelId: body.modelId } : {}),
        ...(body.approvalMode !== undefined ? { approvalMode: body.approvalMode } : {}),
      })
      if (!value) return reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
      if (body.approvalMode === "auto" || body.approvalMode === "full_access") {
        const activeRuns = await input.repository.listActiveConversationRuns(request.actor.userId, conversationId)
        for (const activeRun of activeRuns) {
          if (activeRun.status !== "waiting_approval") continue
          const runCalls = await input.tools.listByRun(activeRun.id)
          let wokePending = false
          for (const candidate of runCalls) {
            if (candidate.status !== "awaiting_approval") continue
            const { autoApproved } = await input.tools.autoApprovePendingForMode(candidate.id, body.approvalMode).catch(() => ({ autoApproved: false }))
            if (autoApproved) wokePending = true
          }
          if (wokePending) {
            await input.repository.updateRun(activeRun.id, "waiting_approval", "queued").then(() => {
              input.notifyRunQueued?.()
            }).catch((updateError) => {
              if (!(updateError instanceof RunStateConflictError)) throw updateError
            })
          }
        }
      }
      return value
    })
    secured.delete("/internal/v1/conversations/:conversationId", async (request, reply) => {
      const { conversationId } = z.object({ conversationId: id }).parse(request.params)
      const existing = await input.repository.getConversation(request.actor.userId, conversationId)
      if (!existing) return reply.code(404).send(errorBody("ai.conversation_not_found", request.id))
      const activeRuns = await input.repository.listActiveConversationRuns(request.actor.userId, conversationId)
      for (const activeRun of activeRuns) {
        let executorOwned = false
        try { executorOwned = await input.cancelExecutingRun(activeRun.id) } catch { /* 本地执行器未持有或中止失败时回退持久层取消 */ }
        if (!executorOwned) {
          const canceled = await input.repository.cancelRun(request.actor.userId, activeRun.id)
          if (canceled?.status === "canceled") {
            try { await input.cancelRun(activeRun.id) } catch { /* best effort */ }
          }
        }
        await input.streamBus.cleanup(activeRun.id).catch(() => undefined)
      }
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
      const catalogDigest = input.toolCatalogDigest(request.actor.platformRole)
      const created = await withSpan("agent.turn.accept", internalSpanOptions({
        "luna.operation.name": "create_turn",
        "gen_ai.conversation.id": conversationId,
      }), async span => {
        const value = await input.repository.createTurn(request.actor.userId, {
          conversationId, input: body.input.parts.map(part => part.text).join("\n"), pageContext: redact(body.pageContext),
          traceContext: captureTraceContext(request.headers),
          actorSessionId: request.actor.sessionId,
          idempotencyKey: key, ...(body.runId ? { preallocatedRunId: body.runId } : {}),
          toolCatalogDigest: catalogDigest,
          modelId: body.modelId,
          ...(body.modelSnapshot ? { modelSnapshot: body.modelSnapshot } : {}),
        })
        span.setAttribute("luna.turn.id", value.turn.id)
        span.setAttribute("luna.run.id", value.run.id)
        return value
      })
      if (created.run.status === "queued") input.notifyRunQueued?.()
      return reply.code(202).send({ turnId: created.turn.id, turnIndex: created.turn.turnIndex, runId: created.run.id, state: created.run.status, eventsUrl: `/api/v1/ai/runs/${created.run.id}/events` })
    })
    secured.post("/internal/v1/conversations/:conversationId/tool-actions", async (request, reply) => {
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
      const catalogDigest = input.toolCatalogDigest(request.actor.platformRole)
      const responsePayload = await withToolActionLock(`${request.actor.userId}:${key}`, async () => {
        const created = await withSpan("agent.tool_action.accept", internalSpanOptions({
          "luna.operation.name": "create_tool_action",
          "gen_ai.conversation.id": conversationId,
        }), async span => {
          const value = await input.repository.createTurn(request.actor.userId, {
            conversationId,
            input: body.message,
            pageContext: { __lunaDirectToolAction: true },
            directToolAction: {
              operationId: body.operationId,
              arguments: body.arguments,
            },
            traceContext: captureTraceContext(request.headers),
            actorSessionId: request.actor.sessionId,
            idempotencyKey: key,
            preallocatedRunId: body.runId,
            toolCatalogDigest: catalogDigest,
          })
          span.setAttribute("luna.turn.id", value.turn.id)
          span.setAttribute("luna.run.id", value.run.id)
          return value
        })
        const existingDirect = created.replayed
          ? (await input.tools.listByRun(created.run.id)).find(candidate => candidate.inputMode === "direct")
          : undefined
        if (created.replayed && existingDirect && (created.run.status !== "queued" || existingDirect.status === "proposed")) {
          const latestRun = await input.repository.getRun(request.actor.userId, created.run.id)
          if (latestRun?.status === "queued" && existingDirect.status === "proposed") {
            input.notifyRunQueued?.()
          }
          return {
            turnId: created.turn.id,
            turnIndex: created.turn.turnIndex,
            runId: created.run.id,
            state: latestRun?.status ?? created.run.status,
            toolCallId: existingDirect.id,
            eventsUrl: `/api/v1/ai/runs/${created.run.id}/events`,
          }
        }
        try {
          if (!existingDirect) {
            await input.repository.touchRunSelectedOperations(created.run.id, [body.operationId], 16)
          }
          const call = existingDirect ?? await input.tools.prepareDirectAction({
            runId: created.run.id,
            operationId: body.operationId,
            arguments: body.arguments,
          })
          if (created.run.status === "queued") {
            if (call.status === "awaiting_approval") {
              const existingCheckpoint = await input.repository.loadRunCheckpoint(created.run.id)
              if (!existingCheckpoint) {
                await input.repository.saveRunCheckpoint(created.run.id, {
                  expectedRevision: 0,
                  phase: "awaiting_approval",
                  modelStep: 0,
                  toolCallCount: 0,
                  state: { pendingToolCallId: call.id },
                })
              }
              await input.repository.updateRun(created.run.id, "queued", "waiting_approval").catch((updateError) => {
                if (!(updateError instanceof RunStateConflictError)) throw updateError
              })
            }
            else if (call.status === "failed") {
              await input.repository.updateRun(created.run.id, "queued", "failed", {
                completedAt: new Date().toISOString(),
                errorCode: call.errorCode ?? "ai.tool_arguments_invalid",
              }).catch((updateError) => {
                if (!(updateError instanceof RunStateConflictError)) throw updateError
              })
            }
            else {
              input.notifyRunQueued?.()
            }
          }
          const run = await input.repository.getRun(request.actor.userId, created.run.id)
          return {
            turnId: created.turn.id,
            turnIndex: created.turn.turnIndex,
            runId: created.run.id,
            state: run?.status ?? created.run.status,
            toolCallId: call.id,
            eventsUrl: `/api/v1/ai/runs/${created.run.id}/events`,
          }
        }
        catch (error) {
          if (!created.replayed) {
            const directCall = (await input.tools.listByRun(created.run.id).catch(() => [])).find(candidate => candidate.inputMode === "direct")
            if (!directCall || directCall.status === "failed") {
              try {
                await input.repository.updateRun(created.run.id, "queued", "failed", {
                  completedAt: new Date().toISOString(),
                  errorCode: telemetryErrorCode(error),
                })
              }
              catch { /* Run 状态可能已流转，保留原始业务错误供上层错误处理转换。 */ }
            }
          }
          throw error
        }
      })
      return reply.code(202).send(responsePayload)
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
      let value: Run | undefined = current
      if (!isTerminalRun(current)) {
        // 本地执行器持有 Run 时由它通过同一流式会话提交 canceled，事件序号与已推送的实时事件连续；
        // 在这里直接写库会复用实时事件已占用的序号，客户端会把终态事件当作重复丢弃。
        let executorOwned = false
        try { executorOwned = await input.cancelExecutingRun(runId) } catch { /* 本地中止失败时退回直接取消。 */ }
        if (executorOwned) value = await input.repository.getRun(request.actor.userId, runId)
        if (!executorOwned || (value && !isTerminalRun(value))) {
          value = await input.repository.cancelRun(request.actor.userId, runId)
          if (value?.status === "canceled") {
            try { await input.cancelRun(runId) } catch { /* 权威 canceled 已持久化，本地中止仅为 best effort。 */ }
          }
        }
      }
      if (!value) return reply.code(404).send(errorBody("ai.run_not_found", request.id))
      if (isTerminalRun(value)) await input.streamBus.cleanup(runId).catch(() => undefined)
      return reply.code(202).send(presentRun(value))
    })
    secured.post("/internal/v1/runs/:runId/approvals/:toolCallId/decision", async (request, reply) => {
      const { runId, toolCallId } = z.object({ runId: id, toolCallId: id }).parse(request.params)
      const body = z.object({
        decision: z.enum(["reject", "approve", "approve_and_remember"]),
      }).parse(request.body)
      const run = await input.repository.getRun(request.actor.userId, runId)
      if (!run) return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      let selected: Awaited<ReturnType<typeof input.tools.inspect>>
      try {
        selected = await input.tools.inspect(toolCallId)
      }
      catch {
        return reply.code(404).send(errorBody("ai.tool_call_not_found", request.id))
      }
      if (selected.runId !== runId) return reply.code(404).send(errorBody("ai.tool_call_not_found", request.id))
      const isMatchingDecisionRecorded = (call: Awaited<ReturnType<typeof input.tools.inspect>>) =>
        call.status !== "awaiting_approval"
        && call.status !== "canceled"
        && (
          (body.decision === "approve" && (call.approvalDecision === "approve" || call.approvalDecision === "approve_and_remember"))
          || (body.decision === "approve_and_remember" && call.approvalDecision === "approve_and_remember")
          || (body.decision === "reject" && call.status === "rejected")
        )
      const ensureQueuedAfterDecision = async (fallbackStatus: Run["status"]): Promise<Run["status"]> => {
        const current = await input.repository.getRun(request.actor.userId, runId)
        const currentStatus = current?.status ?? fallbackStatus
        if (currentStatus !== "waiting_approval") return currentStatus
        try {
          await input.repository.updateRun(runId, "waiting_approval", "queued")
          input.notifyRunQueued?.()
          return "queued"
        }
        catch (updateError) {
          if (updateError instanceof RunStateConflictError) {
            const afterConflict = await input.repository.getRun(request.actor.userId, runId)
            return afterConflict?.status ?? updateError.actualStatus ?? currentStatus
          }
          throw updateError
        }
      }
      if (isMatchingDecisionRecorded(selected)) {
        const nextState = await ensureQueuedAfterDecision(run.status)
        return reply.code(202).send({ runId, state: nextState })
      }
      if (run.status !== "waiting_approval" || selected.status !== "awaiting_approval") {
        return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      }
      try {
        const { alreadyDecided } = await input.tools.recordApprovalDecision(toolCallId, body.decision)
        if (alreadyDecided) {
          const nextState = await ensureQueuedAfterDecision(run.status)
          return reply.code(202).send({ runId, state: nextState })
        }
        const nextState = await ensureQueuedAfterDecision("waiting_approval")
        return reply.code(202).send({ runId, state: nextState })
      }
      catch (error) {
        if (error instanceof Error && error.message === "ai.approval_remember_not_allowed") {
          return reply.code(409).send(errorBody("ai.approval_remember_not_allowed", request.id))
        }
        if (
          error instanceof RunStateConflictError
          || (error instanceof Error && (error.message === "ai.approval_not_pending" || error.message === "ai.tool_call_state_conflict"))
        ) {
          const latest = await input.repository.getRun(request.actor.userId, runId)
          const latestCall = await input.tools.inspect(toolCallId).catch(() => undefined)
          if (latest && latestCall && isMatchingDecisionRecorded(latestCall)) {
            const nextState = await ensureQueuedAfterDecision(latest.status)
            return reply.code(202).send({ runId, state: nextState })
          }
          return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
        }
        throw error
      }
    })
    secured.post("/internal/v1/runs/:runId/input", async (request, reply) => {
      const { runId } = z.object({ runId: id }).parse(request.params)
      const body = z.object({ text: inputText, expectedVersion: z.number().int() }).parse(request.body)
      const run = await input.repository.getRun(request.actor.userId, runId)
      if (!run || run.status !== "waiting_input" || run.rowVersion !== body.expectedVersion) return reply.code(409).send(errorBody("ai.run_not_resumable", request.id))
      const item = await input.repository.appendRunInput(runId, body.text)
      await input.repository.appendEvent(runId, "run.input_received", { length: body.text.length, item })
      await input.repository.updateRun(runId, "waiting_input", "queued")
      input.notifyRunQueued?.()
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
      const durableEvents = await input.repository.getEvents(request.actor.userId, runId, after)
      let liveEvents: typeof durableEvents = []
      let liveReplayFailed = false
      try { liveEvents = await input.streamBus.read(runId, after) }
      catch (error) {
        liveReplayFailed = true
        telemetryLog("agent.stream.replay_failed", "warn", {
          operation: "agent.stream.replay", outcome: "failed",
          ...errorDiagnostic(error, "ai.stream_transport_unavailable"),
        })
      }
      let events = mergeEvents(durableEvents, liveEvents)
      const presentedEvents = events.filter(isPublicStreamEvent).map(event => presentEventForRun(run!, event))
      if (!streamRequested) return { items: presentedEvents, cursor: events.at(-1)?.sequence ?? after }
      if (liveReplayFailed && !isTerminalRun(run)) {
        return reply.code(503).send(errorBody("ai.stream_transport_unavailable", request.id))
      }
      const replayCursor = events.at(-1)?.sequence ?? after
      const subscription = await streamHubs.subscribe(runId, request.actor.userId, replayCursor)
      try {
        const catchupDurable = await input.repository.getEvents(request.actor.userId, runId, replayCursor)
        let catchupLive: typeof durableEvents = []
        try { catchupLive = await input.streamBus.read(runId, replayCursor) }
        catch (error) {
          telemetryLog("agent.stream.replay_failed", "warn", {
            operation: "agent.stream.replay", outcome: "failed",
            ...errorDiagnostic(error, "ai.stream_transport_unavailable"),
          })
          if (!isTerminalRun(run)) {
            subscription.close()
            return reply.code(503).send(errorBody("ai.stream_transport_unavailable", request.id))
          }
        }
        events = mergeEvents(events, catchupDurable, catchupLive)
        reply.raw.setHeader("content-type", "text/event-stream")
        reply.raw.setHeader("cache-control", "no-cache, no-store, no-transform")
        reply.raw.setHeader("x-accel-buffering", "no")
        reply.hijack()
        const flushHeaders = (reply.raw as NodeJS.WritableStream & { flushHeaders?: () => void }).flushHeaders
        if (typeof flushHeaders === "function") flushHeaders.call(reply.raw)
        let cursor = after
        let closed = false
        let emittedPublicFrame = false
        const streamAbort = new AbortController()
        request.raw.once("close", () => { closed = true; streamAbort.abort() })
        const push = async (batch: typeof events) => {
          for (const event of batch) {
            if (isPublicStreamEvent(event)) {
              const presented = presentEventForRun(run!, event)
              if (!await writeSSE(reply.raw, sse(event.type, event.sequence, presented), streamAbort.signal)) return
              emittedPublicFrame = true
            }
            cursor = event.sequence
          }
        }
        await push(events)
        subscription.advance(cursor)
        let heartbeatAt = Date.now()
        if (!closed && !emittedPublicFrame && !isTerminalRun(run)) {
          await writeSSE(reply.raw, sseHeartbeat(runId, run.conversationId), streamAbort.signal)
          heartbeatAt = Date.now()
        }
        try {
          while (!closed) {
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
        finally { subscription.close() }
        if (!closed) {
          const completedSequence = Math.max(1, cursor)
          await writeSSE(reply.raw, sse("run.stream.completed", completedSequence, {
            version: 2,
            eventId: createId("aievt"),
            eventSequence: completedSequence,
            type: "run.stream.completed",
            conversationId: run.conversationId,
            turnId: run.turnId,
            runId: run.id,
            occurredAt: new Date().toISOString(),
            payload: {},
          }), streamAbort.signal)
          reply.raw.end()
        }
        return reply
      }
      finally { subscription.close() }
    })
  })

  app.setErrorHandler((error, request, reply) => {
    const normalized = error instanceof Error ? error : new Error("ai.internal_error")
    if (normalized.name === "AbortError" || normalized.message === "ai.run_canceled") {
      telemetryLog("agent.http.request_aborted", "warn", {
        "operation": "agent.http.request",
        "outcome": "cancelled",
        "http.request.method": request.method,
        "http.route": request.routeOptions.url,
        "http.response.status_code": 499,
        "request_id": request.id,
      })
      void reply.header("cache-control", "no-store").code(499).send(errorBody("ai.run_canceled", request.id))
      return
    }
    const rawCode = normalized instanceof z.ZodError ? "invalid_request" : stableCode(normalized.message)
    const code = rawCode === "ai.provider_context_length_exceeded" ? "ai.model_context_insufficient" : rawCode
    const status = code === "ai.unauthorized" || code === "ai.actor_context_invalid" || code === "ai.actor_context_expired"
      ? 401
      : code === "ai.project_scope_mismatch" || code === "ai.user_not_allowed" || code === "ai.tool_permission_denied"
        ? 403
        : code.endsWith("_not_found")
          ? 404
          : code === "ai.wallet_balance_insufficient"
            ? 422
            : code === "ai.provider_usage_unavailable"
              || code === "ai.structured_output_invalid"
              || code === "ai.tool_outcome_unknown"
              || code === "ai.provider_auth_failed"
              || code === "ai.provider_quota_exhausted"
              || code === "ai.provider_request_failed"
              || code === "ai.provider_request_invalid"
              || code === "ai.provider_response_invalid"
              || code === "ai.provider_stream_invalid"
              || code === "ai.provider_empty_stream"
              ? 502
              : code === "ai.stream_subscriber_limit" || code === "ai.provider_rate_limited"
                ? 429
                : code === "ai.stream_transport_unavailable" || code === "ai.provider_unavailable" || code === "ai.provider_config_unavailable" || code === "ai.persistence_unavailable"
                  ? 503
                  : code === "ai.provider_timeout"
                    ? 504
                    : code === "idempotency_conflict" || code === "ai.run_state_conflict" || code === "ai.not_configured" || code === "ai.model_not_available" || code === "ai.approval_remember_not_allowed"
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
    void reply.header("cache-control", "no-store").code(status).send(errorBody(code, request.id))
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
function isPublicStreamEvent(event: RunEvent) {
  return event.type !== "run.cancel_requested" && event.type !== "run.postprocess.completed"
}
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
