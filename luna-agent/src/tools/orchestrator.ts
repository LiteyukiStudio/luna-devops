import { hashCanonicalJSON } from "../canonical-json.js"
import { genAIToolCallObject, genAIToolSpanAttributes } from "../genai-semconv.js"
import { createId } from "../id.js"
import { redact, redactSensitivePaths } from "../redaction.js"
import { agentMetrics, errorDiagnostic, internalSpanOptions, recordAIContent, stableErrorCode, telemetryLog, withSpan } from "../telemetry.js"
import { ToolArgumentsInvalidError, validateToolArguments } from "./argument-validator.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import { activeRunningRunVersionFence } from "../persistence/run-fence.js"
import type { ToolCatalog } from "./catalog.js"
import { UnknownToolOutcomeError, type LunaApiToolClient, type ToolExecutionResult } from "./luna-api-client.js"
import { InMemoryLoopGuard, type LoopGuard, type LoopGuardState, type ToolLoopSnapshot } from "./loop-guard.js"
import { toolEffect, toolOutcome, type ToolOutcome } from "./outcome.js"
import { ToolPolicy } from "./policy.js"

export type ToolCallStatus = "proposed" | "awaiting_approval" | "running" | "succeeded" | "failed" | "rejected" | "canceled" | "skipped"
export type ToolCallRecord = {
  id: string
  idempotencyKey: string
  verificationForToolCallId?: string
  modelCallId?: string
  batchId?: string
  runId: string
  operationId: string
  status: ToolCallStatus
  arguments: Record<string, unknown>
  /** 仅用于 Run 内参数归一化计数及历史数据兼容，不再作为审批凭据。 */
  argumentsHash: string
  attempt: number
  executionCount: number
  /** 仅用于工具状态的原子更新，不再由审批 API 暴露。 */
  rowVersion: number
  approvalDecision?: "approve"
  inputMode?: "model" | "direct" | "verification"
  result?: unknown
  errorCode?: string
}
export type ToolEvent = { type: string, toolCallId: string, data: Record<string, unknown> }
export type ApprovalDecision = "reject" | "approve"

export interface ToolCallStore {
  insert(value: ToolCallRecord, expectedRunVersion?: number): Promise<void>
  get(id: string): Promise<ToolCallRecord | undefined>
  /** Run 的工具执行事实，按创建顺序返回；恢复时用它核对批次是否已落地。 */
  listByRun(runId: string): Promise<ToolCallRecord[]>
  update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>, expectedRunVersion?: number): Promise<ToolCallRecord>
  emit(event: ToolEvent, expectedRunVersion?: number): Promise<void>
}

export class ToolInterruption extends Error {
  constructor(readonly state: "waiting_input", readonly fields: string[]) {
    super("ai.input_required")
  }
}

type ToolCatalogResolver = ToolCatalog | ((runId: string) => ToolCatalog | Promise<ToolCatalog>)

export class ToolOrchestrator {
  private readonly loopGuard: LoopGuard
  private readonly executionControllers = new Map<string, Set<AbortController>>()
  private readonly directActionLocks = new Map<string, Promise<void>>()

  constructor(
    private readonly catalogResolver: ToolCatalogResolver,
    private readonly client: LunaApiToolClient,
    private readonly store: ToolCallStore,
    private readonly policy = new ToolPolicy(),
    loopGuard: LoopGuard = new InMemoryLoopGuard(),
  ) {
    this.loopGuard = loopGuard
  }

  async propose(input: {
    runId: string
    operationId: string
    arguments: unknown
    inputMode?: "model" | "direct" | "verification"
    verificationForToolCallId?: string
    modelCallId?: string
    batchId?: string
  }, signal?: AbortSignal): Promise<ToolCallRecord> {
    const catalog = await this.catalogForRun(input.runId)
    const operation = catalog.get(input.operationId)
    const argumentsHash = safeArgumentsHash(input.arguments)
    const inputMode = input.inputMode ?? "model"
    const effect = toolEffect(operation)
    const sideEffect = inputMode !== "verification" && (effect === "write" || effect === "async")
    if (sideEffect && !operation.verification?.length)
      throw new Error("ai.tool_not_available")
    const history = sideEffect
      ? (await this.store.listByRun(input.runId)).filter(call =>
          call.inputMode !== "verification"
          && !(call.status === "skipped" && call.executionCount === 0)
          && isSideEffectOperation(catalog, call.operationId))
      : []
    const previous = history.at(-1)
    const idempotencyKey = previous?.operationId === input.operationId && previous.argumentsHash === argumentsHash
      ? previous.idempotencyKey
      : logicalIdempotencyKey(
          input.runId,
          input.operationId,
          argumentsHash,
          new Set(history
            .filter(call => call.operationId === input.operationId && call.argumentsHash === argumentsHash)
            .map(call => call.idempotencyKey)).size + 1,
        )
    const existingCalls = sideEffect
      ? history.filter(call => call.idempotencyKey === idempotencyKey)
      : []
    const reusable = existingCalls.find(call =>
      (call.status === "proposed" && call.executionCount > 0) || call.status === "running" || call.status === "awaiting_approval" || call.status === "succeeded")
    if (reusable) return reusable
    const attempt = existingCalls.reduce((maximum, call) => Math.max(maximum, call.attempt), 0) + 1
    if (inputMode !== "verification")
      this.loopGuard.beforePropose({ runId: input.runId, operationId: input.operationId, argumentsHash })
    let args: Record<string, unknown>
    try {
      args = validateToolArguments(operation.inputSchema, input.arguments)
    }
    catch (error) {
      if (!(error instanceof ToolArgumentsInvalidError)) throw error
      return this.rejectInvalidArguments(input, argumentsHash, idempotencyKey, attempt, error, signal)
    }
    const id = createId("aitool")
    const record: ToolCallRecord = {
      id,
      idempotencyKey,
      ...(input.verificationForToolCallId ? { verificationForToolCallId: input.verificationForToolCallId } : {}),
      ...(input.modelCallId ? { modelCallId: input.modelCallId } : {}),
      ...(input.batchId ? { batchId: input.batchId } : {}),
      runId: input.runId,
      operationId: input.operationId,
      status: "proposed",
      arguments: args,
      argumentsHash,
      attempt,
      rowVersion: 1,
      executionCount: 0,
      inputMode,
    }
    signal?.throwIfAborted()
    await this.store.insert(record)
    telemetryLog("agent.tool.proposed", "info", { "luna.run.id": input.runId, "tool.name": input.operationId })
    await this.store.emit({
      type: "tool.started",
      toolCallId: record.id,
      data: {
        operationId: record.operationId,
        parameterSummary: redact(record.arguments),
        ...(record.modelCallId ? { modelCallId: record.modelCallId } : {}),
        ...(record.batchId ? { batchId: record.batchId } : {}),
      },
    })
    return this.advance(record, false, signal)
  }

  async isCallableOperation(runId: string, operationId: string): Promise<boolean> {
    const catalog = await this.catalogForRun(runId)
    try {
      const operation = catalog.get(operationId)
      if (operation.verificationOnly) return false
      const effect = toolEffect(operation)
      if ((effect === "write" || effect === "async") && !operation.verification?.length) return false
      return true
    }
    catch {
      return false
    }
  }

  async isReadOrDiscoveryOperation(runId: string, operationId: string): Promise<boolean> {
    const catalog = await this.catalogForRun(runId)
    try {
      const effect = toolEffect(catalog.get(operationId))
      return effect === "read" || effect === "discovery"
    }
    catch {
      return false
    }
  }

  operationCallCount(runId: string, operationId: string): number {
    return this.loopGuard.operationCallCount(runId, operationId)
  }

  listByRun(runId: string): Promise<ToolCallRecord[]> {
    return this.store.listByRun(runId)
  }

  /**
   * 仅校验并持久化直接工具动作（HTTP 接收阶段不执行外部平台工具请求）。
   * - 若静态策略需审批：写入 awaiting_approval 并发出 approval.required 事件；
   * - 若参数不合法：写入 failed；
   * - 否则保留 proposed，交由后台 RunExecutor 在 running 态独占执行。
   */
  async prepareDirectAction(input: {
    runId: string
    operationId: string
    arguments: unknown
  }): Promise<ToolCallRecord> {
    const previousLock = this.directActionLocks.get(input.runId) ?? Promise.resolve()
    let releaseLock!: () => void
    const currentLock = new Promise<void>((resolve) => { releaseLock = resolve })
    const chained = previousLock.then(() => currentLock, () => currentLock)
    this.directActionLocks.set(input.runId, chained)
    await previousLock.catch(() => undefined)
    try {
      const existingDirect = (await this.store.listByRun(input.runId)).find(call => call.inputMode === "direct")
      if (existingDirect) return existingDirect
      const catalog = await this.catalogForRun(input.runId)
      const operation = catalog.get(input.operationId)
      const effect = toolEffect(operation)
      if ((effect === "write" || effect === "async") && !operation.verification?.length) {
        throw new Error("ai.tool_not_available")
      }
      const argumentsHash = safeArgumentsHash(input.arguments)
      const idempotencyKey = logicalIdempotencyKey(input.runId, input.operationId, argumentsHash, 1)
      let args: Record<string, unknown>
      try {
        args = validateToolArguments(operation.inputSchema, input.arguments)
      }
      catch (error) {
        if (!(error instanceof ToolArgumentsInvalidError)) throw error
        return this.rejectInvalidArguments(
          { runId: input.runId, operationId: input.operationId, inputMode: "direct" },
          argumentsHash,
          idempotencyKey,
          1,
          error,
        )
      }
      const decision = this.policy.evaluate(operation, false)
      const requiresApproval = decision.action === "wait_approval"
      const id = createId("aitool")
      const record: ToolCallRecord = {
        id,
        idempotencyKey,
        runId: input.runId,
        operationId: input.operationId,
        status: requiresApproval ? "awaiting_approval" : "proposed",
        arguments: args,
        argumentsHash,
        attempt: 1,
        rowVersion: 1,
        executionCount: 0,
        inputMode: "direct",
      }
      await this.store.insert(record)
      telemetryLog("agent.tool.proposed", "info", { "luna.run.id": input.runId, "tool.name": input.operationId })
      await this.store.emit({
        type: "tool.started",
        toolCallId: record.id,
        data: {
          operationId: record.operationId,
          parameterSummary: redact(record.arguments),
        },
      })
      if (requiresApproval) {
        await this.store.emit({
          type: "approval.required",
          toolCallId: record.id,
          data: redact({
            status: "awaiting_approval",
            operationId: record.operationId,
            parameterSummary: redact(record.arguments),
          }),
        })
      }
      return record
    }
    finally {
      releaseLock()
      if (this.directActionLocks.get(input.runId) === chained) {
        this.directActionLocks.delete(input.runId)
      }
    }
  }

  /**
   * 原子持久化用户审批决定，不在此处执行外部平台工具 HTTP 调用。
   * 重复提交相同决定幂等返回，后续执行统一由 RunExecutor 接管。
   */
  async recordApprovalDecision(
    id: string,
    decision: ApprovalDecision,
  ): Promise<{ call: ToolCallRecord, alreadyDecided: boolean }> {
    const call = await this.require(id)
    if (decision === "approve" && call.approvalDecision === "approve" && call.status !== "canceled") {
      return { call, alreadyDecided: true }
    }
    if (decision === "reject" && call.status === "rejected") {
      return { call, alreadyDecided: true }
    }
    if (call.status !== "awaiting_approval") {
      throw new Error("ai.approval_not_pending")
    }
    await this.recoverToolBatch(call.runId)
    agentMetrics.approvals.add(1, { decision, tool: call.operationId })
    telemetryLog("agent.approval.resolved", "info", {
      "luna.run.id": call.runId,
      "tool.name": call.operationId,
      decision,
    })
    if (decision === "reject") {
      await this.store.emit({
        type: "approval.resolved",
        toolCallId: call.id,
        data: { decision, operationId: call.operationId, parameterSummary: redact(call.arguments) },
      })
      const rejected = await this.transition(call, "rejected", {
        errorCode: "ai.tool_rejected",
        result: { code: "ai.tool_rejected", retryable: false },
      }, "tool_call.rejected", { decision })
      return { call: rejected, alreadyDecided: false }
    }
    const approved = await this.store.update(call.id, "awaiting_approval", {
      status: "proposed",
      approvalDecision: "approve",
      rowVersion: call.rowVersion + 1,
    })
    await this.store.emit({
      type: "approval.resolved",
      toolCallId: call.id,
      data: { decision, operationId: call.operationId, parameterSummary: redact(call.arguments) },
    })
    return { call: approved, alreadyDecided: false }
  }

  /**
   * 仅供 RunExecutor 在 Run 处于 running 状态时执行待处理的 direct 或已批准工具调用。
   */
  async executePendingCall(id: string, signal?: AbortSignal): Promise<ToolCallRecord> {
    const call = await this.require(id)
    if (call.status !== "proposed") return call
    if (call.inputMode === "direct" && call.executionCount === 0) {
      this.loopGuard.beforePropose({
        runId: call.runId,
        operationId: call.operationId,
        argumentsHash: call.argumentsHash,
      })
    }
    return this.advance(call, call.approvalDecision === "approve", signal)
  }

  async resolveApproval(id: string, decision: ApprovalDecision, signal?: AbortSignal): Promise<ToolCallRecord> {
    const { call } = await this.recordApprovalDecision(id, decision)
    if (decision === "reject") return call
    return this.executePendingCall(call.id, signal)
  }

  async inspect(id: string): Promise<ToolCallRecord> {
    return this.require(id)
  }

  async outcome(id: string): Promise<ToolOutcome> {
    const call = await this.require(id)
    const catalog = await this.catalogForRun(call.runId)
    const operation = catalog.get(call.operationId)
    return toolOutcome(operation, call, (operationId) => {
      try {
        return catalog.get(operationId)
      }
      catch {
        return undefined
      }
    })
  }

  /**
   * 恢复崩溃批次：读取 Run 的工具执行事实，并把已落地的调用计入循环保护计数，
   * 使重启后 Run 的工具预算不放宽。事实表是权威结果，Timeline 投影缺失不影响计数。
   */
  async recoverToolBatch(runId: string): Promise<ToolCallRecord[]> {
    const rawCalls = await this.store.listByRun(runId)
    const calls: ToolCallRecord[] = []
    for (const call of rawCalls) {
      if (call.status === "proposed" && call.executionCount === 0 && call.inputMode !== "direct" && call.approvalDecision !== "approve") {
        const skipped = await this.store.update(call.id, "proposed", {
          status: "skipped",
          errorCode: "ai.agent_restarted",
          rowVersion: call.rowVersion + 1,
        }).catch(() => call)
        calls.push(skipped)
        continue
      }
      calls.push(call)
    }
    const modelCalls = calls.filter(call => call.inputMode !== "verification" && !(call.status === "skipped" && call.executionCount === 0))
    this.loopGuard.raiseFromSettledCalls(runId, {
      proposed: modelCalls.length,
      executed: modelCalls.reduce((count, call) => count + call.executionCount, 0),
      occurrences: modelCalls.map(call => ({ operationId: call.operationId, argumentsHash: call.argumentsHash })),
      nonRetryableCalls: modelCalls
        .filter(call => isExplicitlyNonRetryable(call.result))
        .map(call => ({ operationId: call.operationId, argumentsHash: call.argumentsHash })),
    })
    return calls
  }

  async retryFailed(id: string, signal?: AbortSignal): Promise<ToolCallRecord> {
    const previous = await this.require(id)
    if (previous.status !== "failed") throw new Error("ai.tool_call_not_retryable")
    this.loopGuard.beforePropose({ runId: previous.runId, operationId: previous.operationId, argumentsHash: previous.argumentsHash })
    const retry: ToolCallRecord = {
      id: createId("aitool"),
      idempotencyKey: previous.idempotencyKey,
      ...(previous.verificationForToolCallId ? { verificationForToolCallId: previous.verificationForToolCallId } : {}),
      runId: previous.runId,
      operationId: previous.operationId,
      status: "proposed",
      arguments: previous.arguments,
      argumentsHash: previous.argumentsHash,
      attempt: previous.attempt + 1,
      rowVersion: 1,
      executionCount: 0,
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
    return this.advance(retry, false, signal)
  }

  cancelRun(runId: string, reason: unknown = new Error("ai.run_canceled")): boolean {
    const controllers = this.executionControllers.get(runId)
    if (!controllers?.size) return false
    for (const controller of controllers) controller.abort(reason)
    return true
  }

  cancelAll(reason: unknown = new Error("ai.agent_stopping")): number {
    let canceled = 0
    for (const controllers of this.executionControllers.values()) {
      for (const controller of controllers) {
        if (controller.signal.aborted) continue
        controller.abort(reason)
        canceled += 1
      }
    }
    return canceled
  }

  setRunMaxToolCalls(limit: number): void {
    this.loopGuard.setMaxToolCalls(limit)
  }

  setRunMaxToolCallsForRun(runId: string, limit: number): void {
    this.loopGuard.setRunMaxToolCalls(runId, limit)
  }

  toolLoopSnapshot(runId: string): ToolLoopSnapshot {
    return this.loopGuard.snapshot(runId)
  }

  /** Run 内循环保护计数，供 Executor 写入 checkpoint。 */
  runLoopState(runId: string): LoopGuardState | undefined {
    return this.loopGuard.exportRun(runId)
  }

  /** 从 checkpoint 恢复循环保护计数；重置进程内存不会放宽调用保护。 */
  restoreRunLoopState(runId: string, state: LoopGuardState | undefined, executed: number): void {
    this.loopGuard.restoreRun(runId, state, executed)
  }

  clearRunLoopState(runId: string): void {
    this.loopGuard.clearRun(runId)
  }

  private async advance(call: ToolCallRecord, approved: boolean, externalSignal?: AbortSignal): Promise<ToolCallRecord> {
    const execution = this.beginExecution(call.runId, externalSignal)
    try {
      const operation = (await this.catalogForRun(call.runId)).get(call.operationId)
      const decision = this.policy.evaluate(operation, approved)
      if (decision.action === "wait_approval")
        return this.transition(call, "awaiting_approval", {}, "approval.required", {
          operationId: call.operationId,
          parameterSummary: redact(call.arguments),
        })
      const result = await this.execute(call, approved, operation, execution.signal)
      return result
    }
    finally {
      execution.release()
    }
  }

  private async execute(call: ToolCallRecord, approvalGranted: boolean, operation: ReturnType<ToolCatalog["get"]>, signal: AbortSignal): Promise<ToolCallRecord> {
    const startedAt = performance.now()
    let outcome = "succeeded"
    if (call.inputMode !== "verification")
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
        recordAIContent(span, "luna.gen_ai.tool.content.input", "gen_ai.tool.call.arguments", genAIToolCallObject(
          redactSensitivePaths(call.arguments, operation.sensitivePaths),
        ), {
          "gen_ai.tool.name": call.operationId,
          "luna.tool_call.id": call.id,
        })
        const running = await this.transition(call, "running", { executionCount: call.executionCount + 1 }, "tool_call.running")
        let result: ToolExecutionResult
        try {
          signal.throwIfAborted()
          result = await this.client.execute({
            runId: call.runId,
            toolCallId: call.id,
            idempotencyKey: call.idempotencyKey,
            operation,
            arguments: call.arguments,
            signal,
          })
        }
        catch (error) {
          if (error instanceof RunStateConflictError) throw error
          if (
            stableErrorCode(error) === "ai.run_lease_lost"
            || (signal.aborted && stableErrorCode(signal.reason) === "ai.run_lease_lost")
          ) {
            outcome = "abandoned"
            throw error instanceof Error && stableErrorCode(error) === "ai.run_lease_lost"
              ? error
              : new Error("ai.run_lease_lost", { cause: error })
          }
          let durablyCanceled: ToolCallRecord | undefined
          if (signal.aborted) {
            try {
              const current = await this.store.get(call.id)
              if (current?.status === "canceled") durablyCanceled = current
            }
            catch { /* 下方保留原始执行错误，数据库异常由 Run 终态回读处理。 */ }
          }
          if (durablyCanceled) {
            outcome = "canceled"
            recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", {
              error: { type: "AbortError", code: durablyCanceled.errorCode ?? "ai.run_canceled" },
            }, { "gen_ai.tool.name": call.operationId, "luna.tool_call.id": call.id })
            telemetryLog("agent.tool.canceled", "info", {
              "luna.run.id": call.runId,
              "luna.tool_call.id": call.id,
              "tool.name": call.operationId,
              "operation": "agent.tool.execute",
              "outcome": "canceled",
            })
            return durablyCanceled
          }
          const errorCode = stableErrorCode(error)
          const unknownOutcome = error instanceof UnknownToolOutcomeError
          const retryable = unknownOutcome && operation.retryOnUnknown === true
          recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", {
            error: { type: error instanceof Error ? error.name : "UnknownError", code: errorCode },
          }, { "gen_ai.tool.name": call.operationId, "luna.tool_call.id": call.id })
          const failed = await this.fail(running, errorCode, {
            code: errorCode,
            retryable,
            ...(unknownOutcome ? { outcome: "unknown" } : {}),
          }, {
            durationMs: Math.max(0, Math.round(performance.now() - startedAt)),
            traceId: span.spanContext().traceId,
          })
          outcome = failed.status
          telemetryLog("agent.tool.failed", "warn", {
            "luna.run.id": call.runId,
            "luna.tool_call.id": call.id,
            "tool.name": call.operationId,
			"operation": "agent.tool.execute",
			"outcome": "failed",
			...errorDiagnostic(error, errorCode),
          })
          return failed
        }
        if (signal.aborted && stableErrorCode(signal.reason) === "ai.run_lease_lost") {
          outcome = "abandoned"
          throw new Error("ai.run_lease_lost")
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
        }, operation)
        outcome = finished.status
        telemetryLog(finished.status === "succeeded" ? "agent.tool.completed" : "agent.tool.failed", finished.status === "succeeded" ? "info" : "warn", {
          "luna.run.id": call.runId,
          "luna.tool_call.id": call.id,
          "tool.name": call.operationId,
		  "operation": "agent.tool.execute",
		  "outcome": finished.status === "succeeded" ? "succeeded" : "failed",
          ...(finished.errorCode
            ? {
                "error.code": finished.errorCode,
                "error.type": "AgentToolResultError",
                "error.message": finished.errorCode,
              }
            : {}),
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
		"operation": "agent.tool.execute",
		"outcome": "failed",
		...errorDiagnostic(error, outcome),
      })
      throw error
    }
    finally {
      const attributes = { tool: call.operationId, outcome }
      agentMetrics.toolCalls.add(1, attributes)
      agentMetrics.toolDuration.record((performance.now() - startedAt) / 1000, attributes)
    }
  }

  private async finish(call: ToolCallRecord, result: ToolExecutionResult, diagnostics: { durationMs: number, traceId: string }, operation: ReturnType<ToolCatalog["get"]>): Promise<ToolCallRecord> {
    const code = extractCode(result.body)
    const storedResult = redact(withRequestId(result.body, result.requestId))
    if (result.status === 428 && code === "ai.approval_required")
      return this.transition(call, "awaiting_approval", {}, "approval.required", {
        operationId: call.operationId,
        parameterSummary: redact(call.arguments),
      })
    if (result.status === 401 || result.status === 403)
      return this.fail(call, code ?? "ai.tool_forbidden", storedResult, diagnostics)
    if (result.status < 200 || result.status >= 300) {
      const errorStoredResult = storedResult && typeof storedResult === "object" && !Array.isArray(storedResult)
        ? { ...(storedResult as Record<string, unknown>), httpStatus: result.status }
        : { ...(storedResult === undefined ? {} : { body: storedResult }), httpStatus: result.status }
      const failed = await this.fail(call, code ?? "ai.tool_failed", errorStoredResult, diagnostics)
      if (isExplicitlyNonRetryable(result.body)) {
        this.loopGuard.blockNonRetryable({ runId: call.runId, operationId: call.operationId, argumentsHash: call.argumentsHash })
      }
      return failed
    }
    if (toolEffect(operation) === "execute") {
      const outcome = toolOutcome(operation, { ...call, status: "succeeded", result: storedResult })
      if (outcome.kind === "failed") return this.fail(call, outcome.errorCode, storedResult, diagnostics)
    }
    return this.transition(call, "succeeded", { result: storedResult }, "tool_call.succeeded", diagnostics)
  }

  private fail(call: ToolCallRecord, errorCode: string, storedResult: unknown, diagnostics: { durationMs: number, traceId: string }): Promise<ToolCallRecord> {
    return this.transition(call, "failed", { errorCode, result: storedResult }, "tool_call.failed", diagnostics)
  }

  private async rejectInvalidArguments(
    input: { runId: string, operationId: string, inputMode?: "model" | "direct" | "verification", modelCallId?: string, batchId?: string },
    argumentsHash: string,
    idempotencyKey: string,
    attempt: number,
    error: ToolArgumentsInvalidError,
    signal?: AbortSignal,
  ): Promise<ToolCallRecord> {
    const result = error.toJSON()
    const id = createId("aitool")
    const record: ToolCallRecord = {
      id,
      idempotencyKey,
      ...(input.modelCallId ? { modelCallId: input.modelCallId } : {}),
      ...(input.batchId ? { batchId: input.batchId } : {}),
      runId: input.runId,
      operationId: input.operationId,
      status: "failed",
      arguments: {},
      argumentsHash,
      attempt,
      rowVersion: 1,
      executionCount: 0,
      errorCode: error.code,
      result,
      ...(input.inputMode ? { inputMode: input.inputMode } : {}),
    }
    signal?.throwIfAborted()
    await this.store.insert(record)
    telemetryLog("agent.tool.arguments_invalid", "warn", {
      "luna.run.id": input.runId,
      "tool.name": input.operationId,
      "operation": "agent.tool.validate_arguments",
      "outcome": "rejected",
      "error.code": error.code,
      "error.type": error.name,
      "error.message": error.code,
    })
    await this.store.emit({
      type: "tool.started",
      toolCallId: record.id,
      data: {
        operationId: record.operationId,
        parameterSummary: {},
        ...(record.modelCallId ? { modelCallId: record.modelCallId } : {}),
        ...(record.batchId ? { batchId: record.batchId } : {}),
      },
    })
    await this.store.emit({
      type: "tool_call.failed",
      toolCallId: record.id,
      data: redact({
        status: "failed",
        result,
        errorCode: error.code,
        ...(record.modelCallId ? { modelCallId: record.modelCallId } : {}),
        ...(record.batchId ? { batchId: record.batchId } : {}),
      }),
    })
    agentMetrics.toolCalls.add(1, { tool: input.operationId, outcome: error.code })
    return record
  }

  private async transition(call: ToolCallRecord, status: ToolCallStatus, patch: Partial<ToolCallRecord>, event: string, eventData: Record<string, unknown> = {}) {
    const next = await this.store.update(call.id, call.status, { ...patch, status, rowVersion: call.rowVersion + 1 })
    await this.store.emit({
      type: event,
      toolCallId: call.id,
      data: redact({
        status,
        result: next.result,
        errorCode: next.errorCode,
        ...(call.modelCallId ? { modelCallId: call.modelCallId } : {}),
        ...(call.batchId ? { batchId: call.batchId } : {}),
        ...eventData,
      }),
    })
    return next
  }

  private async require(id: string): Promise<ToolCallRecord> {
    const call = await this.store.get(id)
    if (!call) throw new Error("ai.tool_call_not_found")
    return call
  }

  private catalogForRun(runId: string): ToolCatalog | Promise<ToolCatalog> {
    return typeof this.catalogResolver === "function" ? this.catalogResolver(runId) : this.catalogResolver
  }

  private beginExecution(runId: string, externalSignal?: AbortSignal): { signal: AbortSignal, release: () => void } {
    const controller = new AbortController()
    const controllers = this.executionControllers.get(runId) ?? new Set<AbortController>()
    controllers.add(controller)
    this.executionControllers.set(runId, controllers)
    const signal = externalSignal
      ? AbortSignal.any([controller.signal, externalSignal])
      : controller.signal
    return {
      signal,
      release: () => {
        controllers.delete(controller)
        if (controllers.size === 0) this.executionControllers.delete(runId)
      },
    }
  }
}

function extractCode(body: unknown): string | undefined {
  if (!body || typeof body !== "object") return undefined
  const object = body as Record<string, unknown>
  return typeof object.code === "string" ? object.code : undefined
}

function isExplicitlyNonRetryable(body: unknown): boolean {
  if (!body || typeof body !== "object" || Array.isArray(body)) return false
  const value = body as Record<string, unknown>
  return value.retryable === false
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

function logicalIdempotencyKey(runId: string, operationId: string, argumentsHash: string, occurrence: number): string {
  return `aiidem_${hashCanonicalJSON({ runId, operationId, argumentsHash, occurrence })}`
}

function isSideEffectOperation(catalog: ToolCatalog, operationId: string): boolean {
  const operation = catalog.get(operationId)
  const effect = toolEffect(operation)
  return effect === "write" || effect === "async"
}

export class MemoryToolCallStore implements ToolCallStore {
  readonly records = new Map<string, ToolCallRecord>()
  readonly events: ToolEvent[] = []
  async insert(value: ToolCallRecord) { this.records.set(value.id, value) }
  async get(id: string) { return this.records.get(id) }
  async listByRun(runId: string) { return [...this.records.values()].filter(record => record.runId === runId) }
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
  async insert(value: ToolCallRecord, expectedRunVersion?: number) {
    const effective = expectedRunVersion ?? activeRunningRunVersionFence(value.runId)
    if (effective !== undefined) await this.repository.assertRunningRunVersion(value.runId, effective)
    return this.inner.insert(value, effective)
  }
  get(id: string) { return this.inner.get(id) }
  listByRun(runId: string) { return this.inner.listByRun(runId) }
  async update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>, expectedRunVersion?: number) {
    const existing = await this.inner.get(id)
    const effective = expectedRunVersion ?? (existing ? activeRunningRunVersionFence(existing.runId) : undefined)
    if (existing && effective !== undefined) await this.repository.assertRunningRunVersion(existing.runId, effective)
    return this.inner.update(id, expected, patch, effective)
  }
  async emit(event: ToolEvent, expectedRunVersion?: number) {
    const call = await this.inner.get(event.toolCallId)
    if (!call) return
    const effective = expectedRunVersion ?? activeRunningRunVersionFence(call.runId)
    if (effective !== undefined) await this.repository.assertRunningRunVersion(call.runId, effective)
    await this.inner.emit(event, effective)
    const execution = await this.repository.getExecutionInput(call.runId)
    if (!execution) return
    const itemId = `${call.id}:item`
    const content = redact({
      ...toolCallContent(call),
      ...(typeof event.data.modelCallId === "string" ? { modelCallId: event.data.modelCallId } : {}),
      ...(typeof event.data.batchId === "string" ? { batchId: event.data.batchId } : {}),
      ...toolDiagnostics(event.data),
    })
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
    ...(call.modelCallId ? { modelCallId: call.modelCallId } : {}),
    ...(call.batchId ? { batchId: call.batchId } : {}),
    operationId: call.operationId,
    status: call.status,
    inputMode: call.inputMode ?? "model",
    ...(call.approvalDecision ? { approvalDecision: call.approvalDecision } : {}),
    ...(call.verificationForToolCallId ? { verificationForToolCallId: call.verificationForToolCallId } : {}),
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
