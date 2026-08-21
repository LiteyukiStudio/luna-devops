import { hashCanonicalJSON } from "../canonical-json.js"
import { genAIToolCallObject, genAIToolSpanAttributes } from "../genai-semconv.js"
import { createId } from "../id.js"
import type { Repository } from "../persistence/repository.js"
import { redact } from "../redaction.js"
import { agentMetrics, internalSpanOptions, recordAIContent, stableErrorCode, telemetryLog, withSpan } from "../telemetry.js"
import { ToolArgumentsInvalidError, requiredInputFields, validateToolArguments } from "./argument-validator.js"
import type { ToolCatalog } from "./catalog.js"
import type { LunaApiToolClient, ToolExecutionResult } from "./luna-api-client.js"
import { InMemoryLoopGuard, type LoopGuard, type ToolLoopSnapshot } from "./loop-guard.js"
import { ToolPolicy } from "./policy.js"

export type ToolCallStatus = "proposed" | "awaiting_approval" | "running" | "succeeded" | "failed" | "rejected" | "canceled" | "skipped"
export type ToolCallRecord = {
  id: string
  runId: string
  operationId: string
  status: ToolCallStatus
  arguments: Record<string, unknown>
  /** 仅用于 Run 内参数归一化计数及历史数据兼容，不再作为审批凭据。 */
  argumentsHash: string
  attempt: number
  /** 仅用于工具状态的原子更新，不再由审批 API 暴露。 */
  rowVersion: number
  approvalDecision?: "approve" | "approve_always"
  inputMode?: "model" | "direct"
  result?: unknown
  errorCode?: string
}
export type ToolEvent = { type: string, toolCallId: string, data: Record<string, unknown> }
export type ApprovalDecision = "reject" | "approve" | "approve_always"

export interface ToolCallStore {
  insert(value: ToolCallRecord): Promise<void>
  get(id: string): Promise<ToolCallRecord | undefined>
  update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>): Promise<ToolCallRecord>
  emit(event: ToolEvent): Promise<void>
}

export class ToolInterruption extends Error {
  constructor(readonly state: "waiting_input", readonly fields: string[]) {
    super("ai.input_required")
  }
}

export class SensitiveInputRejected extends Error {
  constructor(readonly operationId: string) {
    super("ai.sensitive_input_requires_user_form")
  }
}

type ApprovalRepository = Pick<Repository, "hasToolApprovalExemption" | "grantToolApprovalExemption">

export class ToolOrchestrator {
  private readonly approvalRepository: ApprovalRepository | undefined
  private readonly loopGuard: LoopGuard

  constructor(
    private readonly catalog: ToolCatalog,
    private readonly client: LunaApiToolClient,
    private readonly store: ToolCallStore,
    private readonly policy = new ToolPolicy(),
    approvalRepository?: ApprovalRepository,
    loopGuard: LoopGuard = new InMemoryLoopGuard(),
  ) {
    this.approvalRepository = approvalRepository
    this.loopGuard = loopGuard
  }

  async propose(input: { runId: string, operationId: string, arguments: unknown, inputMode?: "model" | "direct" }): Promise<ToolCallRecord> {
    const operation = this.catalog.get(input.operationId)
    const argumentsHash = safeArgumentsHash(input.arguments)
    this.loopGuard.beforePropose({ runId: input.runId, operationId: input.operationId, argumentsHash })
    let args: Record<string, unknown>
    try {
      args = validateToolArguments(operation.inputSchema, input.arguments)
    }
    catch (error) {
      if (!(error instanceof ToolArgumentsInvalidError)) throw error
      const fields = requiredInputFields(error)
      if (fields) throw new ToolInterruption("waiting_input", fields)
      return this.rejectInvalidArguments(input, argumentsHash, error)
    }
    const inputMode = input.inputMode ?? "model"
    if (inputMode !== "direct" && hasSensitiveInput(operation.inputSchema, args))
      throw new SensitiveInputRejected(operation.operationId)

    const record: ToolCallRecord = {
      id: createId("aitool"),
      runId: input.runId,
      operationId: input.operationId,
      status: "proposed",
      arguments: args,
      argumentsHash,
      attempt: 1,
      rowVersion: 1,
      inputMode,
    }
    await this.store.insert(record)
    telemetryLog("agent.tool.proposed", "info", { "luna.run.id": input.runId, "tool.name": input.operationId })
    await this.store.emit({
      type: "tool.started",
      toolCallId: record.id,
      data: { operationId: record.operationId, parameterSummary: redact(record.arguments) },
    })
    const exempt = await this.approvalRepository?.hasToolApprovalExemption(record.runId, record.operationId) ?? false
    return this.advance(record, { approved: false, exempt })
  }

  async resolveApproval(id: string, decision: ApprovalDecision): Promise<ToolCallRecord> {
    const call = await this.requireAwaitingApproval(id)
    agentMetrics.approvals.add(1, { decision, tool: call.operationId })
    telemetryLog("agent.approval.resolved", "info", {
      "luna.run.id": call.runId,
      "tool.name": call.operationId,
      decision,
    })
    if (decision === "approve_always") {
      await this.approvalRepository?.grantToolApprovalExemption(call.runId, call.operationId, call.id)
    }
    await this.store.emit({
      type: "approval.resolved",
      toolCallId: call.id,
      data: { decision, operationId: call.operationId, parameterSummary: redact(call.arguments) },
    })
    if (decision === "reject") {
      return this.transition(call, "rejected", {
        errorCode: "ai.tool_rejected",
        result: { code: "ai.tool_rejected", retryable: false },
      }, "tool_call.rejected", { decision })
    }
    const approved = await this.store.update(call.id, call.status, {
      approvalDecision: decision,
      rowVersion: call.rowVersion + 1,
    })
    return this.advance(approved, { approved: true, exempt: decision === "approve_always" })
  }

  async inspect(id: string): Promise<ToolCallRecord> {
    return this.require(id)
  }

  async retryFailed(id: string): Promise<ToolCallRecord> {
    const previous = await this.require(id)
    if (previous.status !== "failed") throw new Error("ai.tool_call_not_retryable")
    this.loopGuard.beforePropose({ runId: previous.runId, operationId: previous.operationId, argumentsHash: previous.argumentsHash })
    const retry: ToolCallRecord = {
      id: createId("aitool"),
      runId: previous.runId,
      operationId: previous.operationId,
      status: "proposed",
      arguments: previous.arguments,
      argumentsHash: previous.argumentsHash,
      attempt: previous.attempt + 1,
      rowVersion: 1,
      ...(previous.inputMode ? { inputMode: previous.inputMode } : {}),
    }
    await this.store.insert(retry)
    await this.store.emit({
      type: "tool.started",
      toolCallId: retry.id,
      data: {
        operationId: retry.operationId,
        parameterSummary: redact(retry.arguments),
        previousToolCallId: previous.id,
        attempt: retry.attempt,
      },
    })
    const exempt = await this.approvalRepository?.hasToolApprovalExemption(retry.runId, retry.operationId) ?? false
    return this.advance(retry, { approved: false, exempt })
  }

  setRunMaxToolCalls(limit: number): void {
    this.loopGuard.setMaxToolCalls(limit)
  }

  toolLoopSnapshot(runId: string): ToolLoopSnapshot {
    return this.loopGuard.snapshot(runId)
  }

  clearRunLoopState(runId: string): void {
    this.loopGuard.clearRun(runId)
  }

  private async advance(call: ToolCallRecord, state: { approved: boolean, exempt: boolean }): Promise<ToolCallRecord> {
    const operation = this.catalog.get(call.operationId)
    const decision = this.policy.evaluate(operation, state)
    if (decision.action === "wait_approval")
      return this.transition(call, "awaiting_approval", {}, "approval.required", {
        operationId: call.operationId,
        parameterSummary: redact(call.arguments),
      })
    if (state.exempt && !call.approvalDecision) {
      call = await this.store.update(call.id, call.status, {
        approvalDecision: "approve_always",
        rowVersion: call.rowVersion + 1,
      })
    }
    return this.execute(call, state.approved || state.exempt)
  }

  private async execute(call: ToolCallRecord, approvalGranted: boolean): Promise<ToolCallRecord> {
    const startedAt = performance.now()
    let outcome = "succeeded"
    const operation = this.catalog.get(call.operationId)
    this.loopGuard.beforeExecute({ runId: call.runId, operationId: call.operationId, argumentsHash: call.argumentsHash })
    try {
      return await withSpan(`execute_tool ${call.operationId}`, internalSpanOptions({
        ...genAIToolSpanAttributes({
          name: call.operationId,
          callId: call.id,
          ...(operation.summary ? { description: operation.summary } : {}),
        }),
        "luna.run.id": call.runId,
        "luna.tool_call.id": call.id,
        "luna.tool.approval_granted": approvalGranted,
      }), async span => {
        recordAIContent(span, "luna.gen_ai.tool.content.input", "gen_ai.tool.call.arguments", genAIToolCallObject(call.arguments), {
          "gen_ai.tool.name": call.operationId,
          "luna.tool_call.id": call.id,
        })
        const running = await this.transition(call, "running", {}, "tool_call.running")
        let result: ToolExecutionResult
        try {
          result = await this.client.execute({
            runId: call.runId,
            toolCallId: call.id,
            operation,
            arguments: call.arguments,
          })
        }
        catch (error) {
          const errorCode = stableErrorCode(error)
          recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", {
            error: { type: error instanceof Error ? error.name : "UnknownError", code: errorCode },
          }, { "gen_ai.tool.name": call.operationId, "luna.tool_call.id": call.id })
          const failed = await this.fail(running, errorCode, {
            code: errorCode,
            retryable: false,
          }, {
            durationMs: Math.max(0, Math.round(performance.now() - startedAt)),
            traceId: span.spanContext().traceId,
          })
          outcome = failed.status
          telemetryLog("agent.tool.failed", "warn", {
            "luna.run.id": call.runId,
            "luna.tool_call.id": call.id,
            "tool.name": call.operationId,
            "error.code": errorCode,
          })
          return failed
        }
        span.setAttribute("http.response.status_code", result.status)
        recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", genAIToolCallObject({
          status: result.status,
          body: result.body,
          requestId: result.requestId,
        }), { "gen_ai.tool.name": call.operationId, "luna.tool_call.id": call.id })
        const finished = await this.finish(running, result, {
          durationMs: Math.max(0, Math.round(performance.now() - startedAt)),
          traceId: span.spanContext().traceId,
        })
        outcome = finished.status
        telemetryLog(finished.status === "succeeded" ? "agent.tool.completed" : "agent.tool.failed", finished.status === "succeeded" ? "info" : "warn", {
          "luna.run.id": call.runId,
          "luna.tool_call.id": call.id,
          "tool.name": call.operationId,
          ...(finished.errorCode ? { "error.code": finished.errorCode } : {}),
        })
        return finished
      })
    }
    catch (error) {
      outcome = stableErrorCode(error)
      telemetryLog("agent.tool.failed", "error", {
        "luna.run.id": call.runId,
        "luna.tool_call.id": call.id,
        "tool.name": call.operationId,
        "error.type": error instanceof Error ? error.name : "UnknownError",
        "error.code": outcome,
      })
      throw error
    }
    finally {
      const attributes = { tool: call.operationId, outcome }
      agentMetrics.toolCalls.add(1, attributes)
      agentMetrics.toolDuration.record((performance.now() - startedAt) / 1000, attributes)
    }
  }

  private async finish(call: ToolCallRecord, result: ToolExecutionResult, diagnostics: { durationMs: number, traceId: string }): Promise<ToolCallRecord> {
    const code = extractCode(result.body)
    const storedResult = redact(withRequestId(result.body, result.requestId))
    if (result.status === 428 && code === "ai.approval_required")
      return this.transition(call, "awaiting_approval", {}, "approval.required", {
        operationId: call.operationId,
        parameterSummary: redact(call.arguments),
      })
    if (result.status === 401 || result.status === 403)
      return this.fail(call, code ?? "ai.tool_forbidden", storedResult, diagnostics)
    if (result.status < 200 || result.status >= 300)
      return this.fail(call, code ?? "ai.tool_failed", storedResult, diagnostics)
    return this.transition(call, "succeeded", { result: storedResult }, "tool_call.succeeded", diagnostics)
  }

  private fail(call: ToolCallRecord, errorCode: string, storedResult: unknown, diagnostics: { durationMs: number, traceId: string }): Promise<ToolCallRecord> {
    return this.transition(call, "failed", { errorCode, result: storedResult }, "tool_call.failed", diagnostics)
  }

  private async rejectInvalidArguments(input: { runId: string, operationId: string, inputMode?: "model" | "direct" }, argumentsHash: string, error: ToolArgumentsInvalidError): Promise<ToolCallRecord> {
    const result = error.toJSON()
    const record: ToolCallRecord = {
      id: createId("aitool"),
      runId: input.runId,
      operationId: input.operationId,
      status: "proposed",
      arguments: {},
      argumentsHash,
      attempt: 1,
      rowVersion: 1,
      errorCode: error.code,
      result,
      ...(input.inputMode ? { inputMode: input.inputMode } : {}),
    }
    await this.store.insert(record)
    telemetryLog("agent.tool.arguments_invalid", "warn", {
      "luna.run.id": input.runId,
      "tool.name": input.operationId,
      "error.code": error.code,
    })
    await this.store.emit({
      type: "tool.started",
      toolCallId: record.id,
      data: { operationId: record.operationId, parameterSummary: {} },
    })
    const failed = await this.transition(record, "failed", { errorCode: error.code, result }, "tool_call.failed")
    agentMetrics.toolCalls.add(1, { tool: input.operationId, outcome: error.code })
    return failed
  }

  private async transition(call: ToolCallRecord, status: ToolCallStatus, patch: Partial<ToolCallRecord>, event: string, eventData: Record<string, unknown> = {}) {
    const next = await this.store.update(call.id, call.status, { ...patch, status, rowVersion: call.rowVersion + 1 })
    await this.store.emit({
      type: event,
      toolCallId: call.id,
      data: redact({ status, result: next.result, errorCode: next.errorCode, ...eventData }),
    })
    return next
  }

  private async require(id: string): Promise<ToolCallRecord> {
    const call = await this.store.get(id)
    if (!call) throw new Error("ai.tool_call_not_found")
    return call
  }

  private async requireAwaitingApproval(id: string): Promise<ToolCallRecord> {
    const call = await this.require(id)
    if (call.status !== "awaiting_approval") throw new Error("ai.approval_not_pending")
    return call
  }
}

function hasSensitiveInput(schema: Record<string, unknown>, value: unknown): boolean {
  if (schema.writeOnly === true || schema["x-luna-sensitive"] === true)
    return value !== undefined && value !== null && (!Array.isArray(value) || value.length > 0) && (typeof value !== "string" || value.length > 0)
  if (value && typeof value === "object" && !Array.isArray(value) && schema.properties && typeof schema.properties === "object") {
    const properties = schema.properties as Record<string, Record<string, unknown>>
    return Object.entries(value as Record<string, unknown>).some(([key, item]) => properties[key] && hasSensitiveInput(properties[key], item))
  }
  if (Array.isArray(value) && schema.items && typeof schema.items === "object")
    return value.some(item => hasSensitiveInput(schema.items as Record<string, unknown>, item))
  return false
}

function extractCode(body: unknown): string | undefined {
  if (!body || typeof body !== "object") return undefined
  const object = body as Record<string, unknown>
  return typeof object.code === "string"
    ? object.code
    : typeof (object.error as Record<string, unknown> | undefined)?.code === "string"
      ? (object.error as { code: string }).code
      : undefined
}

function withRequestId(body: unknown, requestId: string | undefined): unknown {
  if (!requestId || !body || typeof body !== "object" || Array.isArray(body)) return body
  const object = body as Record<string, unknown>
  return typeof object.requestId === "string" ? body : { ...object, requestId }
}

function safeArgumentsHash(value: unknown): string {
  try {
    return hashCanonicalJSON(value)
  }
  catch {
    return hashCanonicalJSON({ invalidArgumentsType: typeof value })
  }
}

export class MemoryToolCallStore implements ToolCallStore {
  readonly records = new Map<string, ToolCallRecord>()
  readonly events: ToolEvent[] = []
  async insert(value: ToolCallRecord) { this.records.set(value.id, value) }
  async get(id: string) { return this.records.get(id) }
  async update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>) {
    const value = this.records.get(id)
    if (!value || value.status !== expected) throw new Error("ai.tool_call_state_conflict")
    const next = { ...value, ...patch }
    this.records.set(id, next)
    return next
  }
  async emit(event: ToolEvent) { this.events.push(event) }
}

export class ProjectingToolCallStore implements ToolCallStore {
  constructor(private readonly inner: ToolCallStore, private readonly repository: Repository) {}
  insert(value: ToolCallRecord) { return this.inner.insert(value) }
  get(id: string) { return this.inner.get(id) }
  update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>) { return this.inner.update(id, expected, patch) }
  async emit(event: ToolEvent) {
    await this.inner.emit(event)
    const call = await this.inner.get(event.toolCallId)
    if (!call) return
    const execution = await this.repository.getExecutionInput(call.runId)
    if (!execution) return
    const itemId = `${call.id}:item`
    const content = redact({ ...toolCallContent(call), ...toolDiagnostics(event.data) })
    const publicType = publicToolEventType(event.type)
    const eventData = { itemId, toolCallId: call.id, ...event.data }
    if (event.type === "tool.started") {
      await this.repository.appendItemWithEvent({ id: itemId, runId: call.runId, turnId: execution.turnId, type: "tool_call", status: "streaming", content }, publicType, eventData)
    }
    else if (isToolTerminalEvent(event.type)) {
      await this.repository.completeToolItemWithEvent(itemId, toolItemStatus(event.type), content, {
        id: `${call.id}:result`,
        runId: call.runId,
        turnId: execution.turnId,
        type: "tool_result",
        status: event.type === "tool_call.failed" ? "failed" : "completed",
        content: redact({ relatedItemId: itemId, result: call.result, errorCode: call.errorCode }),
      }, publicType, eventData)
    }
    else {
      await this.repository.updateItemWithEvent(itemId, toolItemStatus(event.type), content, publicType, eventData)
    }
  }
}

function toolCallContent(call: ToolCallRecord) {
  return {
    toolCallId: call.id,
    operationId: call.operationId,
    status: call.status,
    arguments: call.arguments,
    result: call.result,
    errorCode: call.errorCode,
  }
}

function toolDiagnostics(data: Record<string, unknown>) {
  return {
    ...(typeof data.durationMs === "number" && Number.isFinite(data.durationMs) ? { durationMs: Math.max(0, Math.round(data.durationMs)) } : {}),
    ...(typeof data.traceId === "string" && /^(?!0{32}$)[a-f0-9]{32}$/i.test(data.traceId) ? { traceId: data.traceId } : {}),
  }
}

function isToolTerminalEvent(type: string) {
  return type === "tool_call.succeeded" || type === "tool_call.failed" || type === "tool_call.rejected"
}

function toolItemStatus(type: string): "streaming" | "completed" | "failed" {
  if (type === "tool_call.failed") return "failed"
  return isToolTerminalEvent(type) ? "completed" : "streaming"
}

function publicToolEventType(type: string) {
  if (type === "tool_call.running") return "tool.progress"
  if (type === "tool_call.succeeded") return "tool.completed"
  if (type === "tool_call.failed") return "tool.failed"
  if (type === "tool_call.rejected") return "tool.rejected"
  return type
}
