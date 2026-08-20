import { hashCanonicalJSON } from "../canonical-json.js"
import { createId } from "../id.js"
import { redact } from "../redaction.js"
import type { Repository } from "../persistence/repository.js"
import { ToolArgumentsInvalidError, requiredInputFields, validateToolArguments } from "./argument-validator.js"
import { InMemoryLoopGuard, type LoopGuard, type ToolLoopSnapshot } from "./loop-guard.js"
import type { ToolCatalog, ToolOperation } from "./catalog.js"
import type { LunaApiToolClient, ToolExecutionResult } from "./luna-api-client.js"
import { genAIToolCallObject, genAIToolSpanAttributes } from "../genai-semconv.js"
import { ToolPolicy } from "./policy.js"
import { agentMetrics, internalSpanOptions, recordAIContent, stableErrorCode, telemetryLog, withSpan } from "../telemetry.js"
import { isRetryableHTTPStatus } from "../retry.js"

export type ToolCallStatus = "proposed" | "awaiting_approval" | "awaiting_mfa" | "running" | "succeeded" | "failed" | "canceled" | "skipped"
export type ToolCallRecord = {
  id: string; runId: string; operationId: string; status: ToolCallStatus; arguments: Record<string, unknown>
  argumentsHash: string; attempt: number; rowVersion: number; approvalExpiresAt?: number; mfaPurpose?: string
  inputMode?: "model" | "direct"; result?: unknown; errorCode?: string
}
export type ToolEvent = { type: string, toolCallId: string, data: Record<string, unknown> }

export interface ToolCallStore {
  insert(value: ToolCallRecord): Promise<void>
  get(id: string): Promise<ToolCallRecord | undefined>
  update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>): Promise<ToolCallRecord>
  emit(event: ToolEvent): Promise<void>
  listAwaitingApproval(runId: string): Promise<ToolCallRecord[]>
}
export interface ToolResultVerifier {
  verify(operationId: string, result: ToolExecutionResult): Promise<{ ok: boolean, code?: string }>
}
export class ContractResultVerifier implements ToolResultVerifier {
  constructor(private readonly catalog: ToolCatalog) {}

  async verify(operationId: string, result: ToolExecutionResult) {
    const verification = this.catalog.get(operationId).contract?.verification
    if (!verification)
      return { ok: false, code: "ai.tool_verification_contract_missing" }
    if (verification.mode === "response") {
      return {
        ok: verification.successCodes.includes(result.status),
        ...(!verification.successCodes.includes(result.status) ? { code: "ai.tool_response_status_unexpected" } : {}),
      }
    }
    return { ok: result.status >= 200 && result.status < 300 }
  }
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

export class ToolOrchestrator {
  constructor(
    private readonly catalog: ToolCatalog,
    private readonly client: LunaApiToolClient,
    private readonly store: ToolCallStore,
    private readonly policy = new ToolPolicy(),
    private readonly verifier: ToolResultVerifier = new ContractResultVerifier(catalog),
    private readonly grantResolver: (runId: string) => Promise<string> = async () => { throw new Error("ai.run_grant_unavailable") },
    private readonly loopGuard: LoopGuard = new InMemoryLoopGuard({
      isAsyncReadbackOperation: operationId => isAsyncReadbackOperation(catalog, operationId),
    }),
    private readonly conversationAuthorizationResolver: (runId: string) => Promise<string | undefined> = async () => undefined,
  ) {}

  async propose(input: { runId: string, operationId: string, arguments: unknown, inputMode?: "model" | "direct" }): Promise<ToolCallRecord> {
    const operation = this.catalog.get(input.operationId)
    const rawArgumentsHash = safeArgumentsHash(input.arguments)
    this.loopGuard.beforePropose({ runId: input.runId, operationId: input.operationId, argumentsHash: rawArgumentsHash })
    let args: Record<string, unknown>
    try {
      args = validateToolArguments(operation.inputSchema, input.arguments)
    }
    catch (error) {
      if (!(error instanceof ToolArgumentsInvalidError)) throw error
      const fields = requiredInputFields(error)
      if (fields) throw new ToolInterruption("waiting_input", fields)
      return this.rejectInvalidArguments(input, rawArgumentsHash, error)
    }
    const inputMode = input.inputMode ?? "model"
    if (inputMode !== "direct" && hasSensitiveInput(operation.inputSchema, args))
      throw new SensitiveInputRejected(operation.operationId)
    const argumentsHash = rawArgumentsHash
    const record: ToolCallRecord = {
      id: createId("aitool"), runId: input.runId, operationId: input.operationId,
      status: "proposed", arguments: args, argumentsHash, attempt: 1, rowVersion: 1, inputMode,
    }
    await this.store.insert(record)
    telemetryLog("agent.tool.proposed", "info", {
      "luna.run.id": input.runId,
      "tool.name": input.operationId,
    })
    await this.store.emit({ type: "tool.started", toolCallId: record.id, data: {
      operationId: record.operationId, arguments: redact(record.arguments), argumentsHash, expectedVersion: record.rowVersion,
    } })
    return this.advance(record, { approved: false })
  }

  async approve(id: string, argumentsHash: string, expectedVersion: number): Promise<ToolCallRecord> {
    const call = await this.require(id)
    this.requireApprovalBinding(call, argumentsHash, expectedVersion)
    agentMetrics.approvals.add(1, { decision: "approve", tool: call.operationId })
    telemetryLog("agent.approval.resolved", "info", { "luna.run.id": call.runId, "tool.name": call.operationId, decision: "approve" })
    await this.store.emit({ type: "approval.resolved", toolCallId: call.id, data: { decision: "approve", argumentsHash, expectedVersion } })
    return this.advance(call, { approved: true })
  }

  async approveConversation(id: string, argumentsHash: string, expectedVersion: number, conversationAuthorizationGrant: string): Promise<ToolCallRecord> {
    const call = await this.require(id)
    this.requireApprovalBinding(call, argumentsHash, expectedVersion)
    if (!conversationAuthorizationGrant) throw new Error("ai.conversation_authorization_invalid")
    agentMetrics.approvals.add(1, { decision: "approve_conversation", tool: call.operationId })
    telemetryLog("agent.approval.resolved", "info", {
      "luna.run.id": call.runId,
      "tool.name": call.operationId,
      decision: "approve_conversation",
    })
    await this.store.emit({ type: "approval.resolved", toolCallId: call.id, data: {
      decision: "approve_conversation", argumentsHash, expectedVersion,
    } })
    return this.execute(call, { approvalGranted: false, conversationAuthorizationGrant })
  }

  async approvePendingWithConversationAuthorization(runId: string, conversationAuthorizationGrant: string): Promise<ToolCallRecord[]> {
    const pending = await this.store.listAwaitingApproval(runId)
    const results: ToolCallRecord[] = []
    for (const call of pending)
      results.push(await this.approveConversation(call.id, call.argumentsHash, call.rowVersion, conversationAuthorizationGrant))
    return results
  }

  async reject(id: string, argumentsHash: string, expectedVersion: number): Promise<ToolCallRecord> {
    const call = await this.require(id)
    this.requireApprovalBinding(call, argumentsHash, expectedVersion)
    agentMetrics.approvals.add(1, { decision: "reject", tool: call.operationId })
    telemetryLog("agent.approval.resolved", "info", { "luna.run.id": call.runId, "tool.name": call.operationId, decision: "reject" })
    return this.transition(call, "canceled", {}, "approval.resolved", { decision: "reject" })
  }

  async inspect(id: string): Promise<ToolCallRecord> {
    return this.require(id)
  }

  async resumeMfa(id: string, purpose: string, expectedVersion: number, stepUpAssertionId: string): Promise<ToolCallRecord> {
    const call = await this.require(id)
    if (call.status !== "awaiting_mfa" || call.mfaPurpose !== purpose || call.rowVersion !== expectedVersion || !stepUpAssertionId) throw new Error("ai.run_not_resumable")
    return this.execute(call, { approvalGranted: true, mfaPurpose: purpose, stepUpAssertionId })
  }

  async retryFailed(id: string): Promise<ToolCallRecord> {
    const previous = await this.require(id)
    if (previous.status !== "failed") throw new Error("ai.tool_call_not_retryable")
    this.loopGuard.beforePropose({
      runId: previous.runId,
      operationId: previous.operationId,
      argumentsHash: previous.argumentsHash,
    })
    const retry: ToolCallRecord = {
      id: createId("aitool"), runId: previous.runId, operationId: previous.operationId,
      status: "proposed", arguments: previous.arguments, argumentsHash: previous.argumentsHash,
      attempt: previous.attempt + 1, rowVersion: 1,
      ...(previous.inputMode ? { inputMode: previous.inputMode } : {}),
    }
    await this.store.insert(retry)
    await this.store.emit({ type: "tool.started", toolCallId: retry.id, data: {
      operationId: retry.operationId, arguments: redact(retry.arguments), previousToolCallId: previous.id,
      attempt: retry.attempt, argumentsHash: retry.argumentsHash, expectedVersion: retry.rowVersion,
    } })
    return this.advance(retry, { approved: false })
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

  seedRunStableResult(input: { runId: string, operationId: string, argumentsHash: string, result: unknown }): void {
    let operation: ToolOperation
    try {
      operation = this.catalog.get(input.operationId)
    }
    catch {
      return
    }
    const contract = operation.contract
    if (!contract?.replaySafe || !["none", "external-read"].includes(contract.sideEffect)) return
    this.loopGuard.seedResult({
      runId: input.runId,
      operationId: input.operationId,
      argumentsHash: input.argumentsHash,
      stableResultHash: safeArgumentsHash(input.result),
    })
  }

  private async advance(call: ToolCallRecord, state: { approved: boolean }): Promise<ToolCallRecord> {
    const operation = this.catalog.get(call.operationId)
    const decision = this.policy.evaluate(operation, state)
    const supportsConversationAuthorization = decision.action === "wait_approval"
      || Boolean(operation.contract?.mfaPurpose ?? operation.stepUpPurpose)
    const conversationAuthorizationGrant = supportsConversationAuthorization
      ? await this.conversationAuthorizationResolver(call.runId)
      : undefined
    if (decision.action === "wait_approval") {
      if (conversationAuthorizationGrant)
        return this.execute(call, { approvalGranted: false, conversationAuthorizationGrant })
      return this.transition(call, "awaiting_approval", { approvalExpiresAt: Date.now() + 30 * 60_000 }, "approval.required")
    }
    return this.execute(call, {
      approvalGranted: state.approved,
      ...(conversationAuthorizationGrant ? { conversationAuthorizationGrant } : {}),
    })
  }

  private async execute(call: ToolCallRecord, authorization: { approvalGranted: boolean, mfaPurpose?: string, stepUpAssertionId?: string, conversationAuthorizationGrant?: string }): Promise<ToolCallRecord> {
    const startedAt = performance.now()
    let outcome = "succeeded"
    const operation = this.catalog.get(call.operationId)
    this.loopGuard.beforeExecute({ runId: call.runId, operationId: call.operationId, argumentsHash: call.argumentsHash })
    try {
      return await withSpan(`execute_tool ${call.operationId}`, internalSpanOptions({
        ...genAIToolSpanAttributes({
          name: call.operationId,
          callId: call.id,
          ...(operation.description ? { description: operation.description } : {}),
        }),
        "luna.run.id": call.runId,
        "luna.tool_call.id": call.id,
        "luna.tool.approval_granted": authorization.approvalGranted,
      }), async span => {
        recordAIContent(span, "luna.gen_ai.tool.content.input", "gen_ai.tool.call.arguments", genAIToolCallObject(call.arguments), {
          "gen_ai.tool.name": call.operationId,
          "luna.tool_call.id": call.id,
        })
        const running = await this.transition(call, "running", {}, "tool_call.running")
        let result: ToolExecutionResult
        try {
          result = await this.client.execute({
            runId: call.runId, toolCallId: call.id, operation, arguments: call.arguments,
            argumentsHash: call.argumentsHash, runActorGrant: await this.grantResolver(call.runId),
            approvalGranted: authorization.approvalGranted,
            inputMode: call.inputMode ?? "model",
            ...(authorization.mfaPurpose ? { mfaPurpose: authorization.mfaPurpose } : {}),
            ...(authorization.stepUpAssertionId ? { stepUpAssertionId: authorization.stepUpAssertionId } : {}),
            ...(authorization.conversationAuthorizationGrant ? { conversationAuthorizationGrant: authorization.conversationAuthorizationGrant } : {}),
          })
        } catch (error) {
          recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", {
            error: {
              type: error instanceof Error ? error.name : "UnknownError",
              code: stableErrorCode(error),
            },
          }, {
            "gen_ai.tool.name": call.operationId,
            "luna.tool_call.id": call.id,
          })
          throw error
        }
        span.setAttribute("http.response.status_code", result.status)
        recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", genAIToolCallObject({
          status: result.status,
          body: result.body,
          requestId: result.requestId,
        }), {
          "gen_ai.tool.name": call.operationId,
          "luna.tool_call.id": call.id,
        })
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
    const operation = this.catalog.get(call.operationId)
    const code = extractCode(result.body)
    const plainResult = withRequestId(result.body, result.requestId)
    let storedResult = redact(plainResult)
    if (result.status === 401 || result.status === 403)
      return this.fail(call, result, code ?? "ai.tool_forbidden", storedResult, diagnostics)
    if (result.status === 428 && code === "mfa_required") {
      const purpose = (result.body as Record<string, unknown>).purpose
      return this.transition(call, "awaiting_mfa", { mfaPurpose: typeof purpose === "string" ? purpose : "" }, "tool_call.awaiting_mfa")
    }
    if (result.status === 428 && code === "ai.approval_required")
      return this.transition(call, "awaiting_approval", { approvalExpiresAt: Date.now() + 30 * 60_000 }, "approval.required")
    if (result.status < 200 || result.status >= 300)
      return this.fail(call, result, code ?? "ai.tool_failed", storedResult, diagnostics)
    const verification = await this.verifier.verify(call.operationId, result)
    if (!verification.ok)
      return this.fail(call, result, verification.code ?? "verification_inconclusive", storedResult, diagnostics)
    if (operation.contract?.verification.mode === "readback" || operation.contract?.verification.mode === "async-readback") {
      let readback: Awaited<ReturnType<ToolOrchestrator["performContractReadback"]>>
      try {
        readback = await this.performContractReadback(call, operation, plainResult)
      }
      catch (error) {
        const errorCode = stableErrorCode(error)
        storedResult = withVerificationEvidence(storedResult, {
          mode: operation.contract.verification.mode,
          operationId: operation.contract.verification.operationId,
          status: "failed",
          errorCode,
        })
        return this.fail(call, { ...result, body: storedResult }, errorCode, storedResult, diagnostics)
      }
      storedResult = withVerificationEvidence(storedResult, readback.evidence)
      if (readback.outcome === "failed") {
        return this.fail(
          call,
          { ...result, body: storedResult },
          readback.errorCode ?? "ai.tool_verification_readback_failed",
          storedResult,
          diagnostics,
        )
      }
    }
    const succeeded = await this.transition(call, "succeeded", { result: storedResult }, "tool_call.succeeded", diagnostics)
    this.loopGuard.recordResult({
      runId: call.runId,
      operationId: call.operationId,
      argumentsHash: call.argumentsHash,
      stableResultHash: safeArgumentsHash(storedResult),
    })
    return succeeded
  }

  private async performContractReadback(
    call: ToolCallRecord,
    operation: ToolOperation,
    writeResult: unknown,
  ): Promise<{
      outcome: "succeeded" | "pending" | "failed"
      errorCode?: string
      evidence: Record<string, unknown>
    }> {
    const verification = operation.contract?.verification
    if (!verification || verification.mode === "response")
      throw new Error("ai.tool_verification_contract_missing")
    const verifier = this.catalog.get(verification.operationId)
    if (verifier.contract?.verification.mode !== "response")
      throw new Error("ai.tool_verifier_contract_invalid")
    // Luna API 的委托执行端点会把真实业务响应放在 result 字段中。
    // 契约里的 JSON Pointer 始终相对业务响应，不能相对传输信封解析。
    const writeBusinessResult = delegatedBusinessResult(writeResult)
    if (jsonPointerValue(writeBusinessResult, verification.idSource) === undefined)
      throw new Error("ai.tool_verification_id_missing")

    const argumentsValue: Record<string, unknown> = {}
    for (const [argument, pointer] of Object.entries(verification.argumentBindings)) {
      const value = jsonPointerValue(writeBusinessResult, pointer)
      if (value === undefined)
        throw new Error("ai.tool_verification_binding_missing")
      argumentsValue[argument] = value
    }

    let readback: ToolCallRecord
    try {
      readback = await this.propose({
        runId: call.runId,
        operationId: verification.operationId,
        arguments: argumentsValue,
        inputMode: "model",
      })
    }
    catch (error) {
      const errorCode = stableErrorCode(error)
      return {
        outcome: "failed",
        errorCode,
        evidence: {
          mode: verification.mode,
          operationId: verification.operationId,
          status: "failed",
          errorCode,
        },
      }
    }
    if (readback.status !== "succeeded") {
      return {
        outcome: "failed",
        errorCode: readback.errorCode ?? "ai.tool_verification_readback_failed",
        evidence: {
          mode: verification.mode,
          operationId: verification.operationId,
          toolCallId: readback.id,
          status: readback.status,
          ...(readback.errorCode ? { errorCode: readback.errorCode } : {}),
        },
      }
    }

    const completion = verification.completion
    if (completion.mode === "readback-success") {
      return {
        outcome: "succeeded",
        evidence: {
          mode: verification.mode,
          operationId: verification.operationId,
          toolCallId: readback.id,
          status: "succeeded",
          result: readback.result,
        },
      }
    }

    const stateValue = jsonPointerValue(delegatedBusinessResult(readback.result), completion.path)
    const state = typeof stateValue === "string" ? stateValue : undefined
    if (!state) {
      return {
        outcome: "failed",
        errorCode: "ai.tool_verification_state_missing",
        evidence: {
          mode: verification.mode,
          operationId: verification.operationId,
          toolCallId: readback.id,
          status: "failed",
          errorCode: "ai.tool_verification_state_missing",
        },
      }
    }
    const evidence = {
      mode: verification.mode,
      operationId: verification.operationId,
      toolCallId: readback.id,
      state,
      result: readback.result,
    }
    if (completion.successStates.includes(state)) return { outcome: "succeeded", evidence: { ...evidence, status: "succeeded" } }
    if (completion.failureStates.includes(state)) {
      return {
        outcome: "failed",
        errorCode: "ai.tool_verification_terminal_failure",
        evidence: { ...evidence, status: "failed", errorCode: "ai.tool_verification_terminal_failure" },
      }
    }
    if (verification.mode === "async-readback" && completion.pendingStates.includes(state)) {
      return { outcome: "pending", evidence: { ...evidence, status: "pending" } }
    }
    return {
      outcome: "failed",
      errorCode: "ai.tool_verification_state_unexpected",
      evidence: { ...evidence, status: "failed", errorCode: "ai.tool_verification_state_unexpected" },
    }
  }

  private async fail(call: ToolCallRecord, result: ToolExecutionResult, errorCode: string, storedResult: unknown, diagnostics: { durationMs: number, traceId: string }): Promise<ToolCallRecord> {
    const failed = await this.transition(call, "failed", { errorCode, result: storedResult }, "tool_call.failed", diagnostics)
    this.loopGuard.recordFailure({
      runId: call.runId,
      operationId: call.operationId,
      argumentsHash: call.argumentsHash,
      errorCode,
      stableResultHash: safeArgumentsHash(storedResult),
      deterministic: !toolFailureRetryable(result),
    })
    return failed
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
      data: { operationId: record.operationId, arguments: {}, argumentsHash, expectedVersion: record.rowVersion },
    })
    const failed = await this.transition(record, "failed", { errorCode: error.code, result }, "tool_call.failed")
    this.loopGuard.recordFailure({
      runId: input.runId,
      operationId: input.operationId,
      argumentsHash,
      errorCode: error.code,
      stableResultHash: safeArgumentsHash(result),
      deterministic: true,
    })
    agentMetrics.toolCalls.add(1, { tool: input.operationId, outcome: error.code })
    return failed
  }

  private async transition(call: ToolCallRecord, status: ToolCallStatus, patch: Partial<ToolCallRecord>, event: string, eventData: Record<string, unknown> = {}) {
    const next = await this.store.update(call.id, call.status, { ...patch, status, rowVersion: call.rowVersion + 1 })
    await this.store.emit({ type: event, toolCallId: call.id, data: redact({
      status,
      result: next.result,
      errorCode: next.errorCode,
      purpose: next.mfaPurpose,
      argumentsHash: next.argumentsHash,
      expectedVersion: next.rowVersion,
      ...eventData,
    }) })
    return next
  }
  private async require(id: string) {
    const call = await this.store.get(id)
    if (!call) throw new Error("ai.tool_call_not_found")
    return call
  }
  private requireApprovalBinding(call: ToolCallRecord, argumentsHash: string, expectedVersion: number) {
    if (call.status !== "awaiting_approval" || call.argumentsHash !== argumentsHash || call.rowVersion !== expectedVersion || (call.approvalExpiresAt ?? 0) < Date.now()) {
      throw new Error("ai.approval_expired")
    }
  }
}

function hasSensitiveInput(schema: Record<string, unknown>, value: unknown): boolean {
  if (schema.writeOnly === true || schema["x-luna-sensitive"] === true)
    return value !== undefined && value !== null && (!Array.isArray(value) || value.length > 0) && (!(typeof value === "string") || value.length > 0)
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
  return typeof object.code === "string" ? object.code : typeof (object.error as Record<string, unknown> | undefined)?.code === "string" ? (object.error as { code: string }).code : undefined
}

function withRequestId(body: unknown, requestId: string | undefined): unknown {
  if (!requestId || !body || typeof body !== "object" || Array.isArray(body)) return body
  const object = body as Record<string, unknown>
  return typeof object.requestId === "string" ? body : { ...object, requestId }
}

function withVerificationEvidence(result: unknown, evidence: Record<string, unknown>): unknown {
  if (result && typeof result === "object" && !Array.isArray(result))
    return { ...(result as Record<string, unknown>), lunaVerification: evidence }
  return { value: result, lunaVerification: evidence }
}

/**
 * 平台工具经委托端点执行时返回稳定传输信封；测试客户端和未来的本地执行器
 * 也允许直接返回业务对象。这里只解一层明确的委托信封，避免把业务对象中名为
 * result 的普通字段误当成传输层。
 */
function delegatedBusinessResult(input: unknown): unknown {
  if (!input || typeof input !== "object" || Array.isArray(input)) return input
  const envelope = input as Record<string, unknown>
  const isDelegationEnvelope = typeof envelope.operationId === "string"
    && typeof envelope.verified === "boolean"
    && Object.hasOwn(envelope, "result")
  return isDelegationEnvelope ? envelope.result : input
}

function jsonPointerValue(input: unknown, pointer: string): unknown {
  if (!pointer.startsWith("/")) return undefined
  let current = input
  for (const rawSegment of pointer.slice(1).split("/")) {
    const segment = rawSegment.replaceAll("~1", "/").replaceAll("~0", "~")
    if (Array.isArray(current)) {
      if (!/^\d+$/.test(segment)) return undefined
      current = current[Number(segment)]
      continue
    }
    if (!current || typeof current !== "object" || !(segment in current)) return undefined
    current = (current as Record<string, unknown>)[segment]
  }
  return current
}

function toolFailureRetryable(result: ToolExecutionResult): boolean {
  if (result.body && typeof result.body === "object") {
    const body = result.body as Record<string, unknown>
    if (typeof body.retryable === "boolean") return body.retryable
    if (body.error && typeof body.error === "object" && typeof (body.error as Record<string, unknown>).retryable === "boolean")
      return (body.error as { retryable: boolean }).retryable
  }
  return isRetryableHTTPStatus(result.status)
}

function safeArgumentsHash(value: unknown): string {
  try {
    return hashCanonicalJSON(value)
  }
  catch {
    return hashCanonicalJSON({ invalidArgumentsType: typeof value })
  }
}

function isAsyncReadbackOperation(catalog: ToolCatalog, operationId: string): boolean {
  return catalog.all().some(operation => operation.contract?.verification.mode === "async-readback"
    && operation.contract.verification.operationId === operationId)
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
  async listAwaitingApproval(runId: string) {
    return [...this.records.values()].filter(item => item.runId === runId && item.status === "awaiting_approval")
  }
}

export class ProjectingToolCallStore implements ToolCallStore {
  constructor(private readonly inner: ToolCallStore, private readonly repository: Repository) {}
  insert(value: ToolCallRecord) { return this.inner.insert(value) }
  get(id: string) { return this.inner.get(id) }
  update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>) { return this.inner.update(id, expected, patch) }
  listAwaitingApproval(runId: string) { return this.inner.listAwaitingApproval(runId) }
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
    await (event.type === "tool.started"
      ? this.repository.appendItemWithEvent({ id: itemId, runId: call.runId, turnId: execution.turnId, type: "tool_call", status: "streaming", content }, publicType, eventData)
      : this.repository.updateItemWithEvent(itemId, toolItemStatus(event.type), content, publicType, eventData))
    if (isToolTerminalEvent(event.type)) {
      await this.repository.appendItem({
        id: `${call.id}:result`, runId: call.runId, turnId: execution.turnId, type: "tool_result",
        status: event.type === "tool_call.failed" ? "failed" : "completed",
        content: redact({ relatedItemId: itemId, result: call.result, errorCode: call.errorCode }),
      })
    }
  }
}

function toolCallContent(call: ToolCallRecord) {
  return {
    toolCallId: call.id, operationId: call.operationId, status: call.status,
    arguments: call.arguments, result: call.result, errorCode: call.errorCode,
    argumentsHash: call.argumentsHash, expectedVersion: call.rowVersion, mfaPurpose: call.mfaPurpose,
  }
}

function toolDiagnostics(data: Record<string, unknown>) {
  return {
    ...(typeof data.durationMs === "number" && Number.isFinite(data.durationMs) ? { durationMs: Math.max(0, Math.round(data.durationMs)) } : {}),
    ...(typeof data.traceId === "string" && /^(?!0{32}$)[a-f0-9]{32}$/i.test(data.traceId) ? { traceId: data.traceId } : {}),
  }
}

function isToolTerminalEvent(type: string) {
  return type === "tool_call.succeeded" || type === "tool_call.failed"
}

function toolItemStatus(type: string): "streaming" | "completed" | "failed" {
  if (type === "tool_call.failed") return "failed"
  return type === "tool_call.succeeded" || type === "approval.resolved" ? "completed" : "streaming"
}

function publicToolEventType(type: string) {
  if (type === "tool_call.running") return "tool.progress"
  if (type === "tool_call.succeeded") return "tool.completed"
  if (type === "tool_call.failed") return "tool.failed"
  if (type === "tool_call.awaiting_mfa") return "mfa.required"
  return type
}
