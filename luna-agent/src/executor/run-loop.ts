import type { DocsKnowledge } from "../knowledge/docs-knowledge.js"
import type { Span } from "@opentelemetry/api"
import type { ClaimedRun, RunCheckpointState } from "../domain.js"
import { genAIInputMessages } from "../genai-semconv.js"
import type { ModelRuntime } from "../model-runtime.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import type { RemoteProviderConfig } from "../provider/config-client.js"
import { redact } from "../redaction.js"
import type { RunStreamBus } from "../run-stream-bus.js"
import { TerminalPersistenceExhaustedError } from "../run-stream-bus.js"
import type { RuntimeSettings } from "../runtime-settings.js"
import { recordAIContent, recordSpanError, stableErrorCode, telemetryLog } from "../telemetry.js"
import { ToolInterruption, type ToolOrchestrator } from "../tools/orchestrator.js"
import type { TaskStatusClient } from "../tools/wait-for-task.js"
import { checkpointState, resumePlan } from "./checkpoint.js"
import { modelDecision } from "./decision.js"
import { RunFinalizer, type TerminalCommit } from "./finalize.js"
import { decisionAction, resumeAction, toolBatchAction } from "./graph.js"
import { StreamModelFinalizationError, streamModel } from "./streaming.js"
import { reasoningByToolCallItemId } from "./resume.js"
import { stableError, toolResultMessage } from "./tool-results.js"
import { ToolStepRunner, type RunExecutionInput, type ToolBatchResult, type ToolStepState } from "./tool-step.js"

const protocolRepairInstruction = "[系统协议提示] 上一步的结束原因表明需要调用工具，但没有任何可解析的工具调用。请直接调用所需工具，或给出最终答复。"
const gracefulFinalSummaryInstruction = "[系统收尾提示] 已达到单轮工具探索步数上限，本步不再提供任何工具。请立即基于目前已获取的真实结果，向用户诚实总结：1) 已确认的事实与已完成的操作；2) 尚未完成的部分及原因；3) 建议用户的下一步操作。严禁编造未执行的工具结果。"

export type RunLoopInput = {
  run: ClaimedRun
  runtimeSettings: RuntimeSettings
  providerConfig: RemoteProviderConfig
  toolCatalogDigest: string
  signal: AbortSignal
  span: Span
}

export class RunLoop {
  private readonly toolStep: ToolStepRunner
  private readonly finalizer: RunFinalizer

  constructor(
    private readonly repository: Repository,
    private readonly modelRuntime: ModelRuntime,
    private readonly tools: ToolOrchestrator,
    private readonly streamBus: RunStreamBus,
    docsKnowledge?: DocsKnowledge,
    taskStatusClient?: TaskStatusClient,
  ) {
    this.toolStep = new ToolStepRunner(repository, modelRuntime, tools, docsKnowledge, taskStatusClient)
    this.finalizer = new RunFinalizer(repository, streamBus, modelRuntime)
  }

  async execute(input: RunLoopInput): Promise<string> {
    const { run, runtimeSettings, providerConfig, signal, span } = input
    let outcome = "completed"
    let pendingTerminal: TerminalCommit | undefined
    let pendingFlushStream: ((fallbackStatus?: "completed" | "failed") => Promise<void>) | undefined
    let checkpointRevision = 0
    let modelStep = 0
    let protocolRepairCount = 0
    let toolState: ToolStepState | undefined
    try {
      telemetryLog("agent.run.started", "info", { "luna.run.id": run.id })
      await this.repository.appendEvent(run.id, "run.started", { state: "running" })
      const executionInput = await this.repository.getExecutionInput(run.id)
      if (!executionInput) throw new Error("ai.turn_not_found")
      recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", genAIInputMessages([
        { role: "user", content: executionInput.input },
      ]))
      const checkpoint = await this.repository.loadRunCheckpoint(run.id)
      const resume = resumeAction(
        checkpoint ? resumePlan(checkpoint) : { kind: "model" },
        checkpoint?.phase ?? "model",
      )
      this.tools.restoreRunLoopState(run.id, checkpoint?.state.loopGuard, checkpoint?.toolCallCount ?? 0)
      checkpointRevision = checkpoint?.revision ?? 0
      modelStep = checkpoint?.modelStep ?? 0
      protocolRepairCount = checkpoint?.state.protocolRepairCount ?? 0
      toolState = await this.toolStep.initialState(run, executionInput, checkpoint)
      const currentState = (extra: Omit<Parameters<typeof checkpointState>[0], "loopGuard" | "protocolRepairCount" | "argumentRepairAttempts" | "cardGeneration" | "cardRepairExhausted"> = {}) => checkpointState({
        loopGuard: this.tools.runLoopState(run.id),
        protocolRepairCount,
        argumentRepairAttempts: toolState?.providerArgumentRepairAttempts ?? new Map(),
        ...(toolState?.cardGeneration ? { cardGeneration: toolState.cardGeneration } : {}),
        ...(toolState?.cardRepairExhausted ? { cardRepairExhausted: true } : {}),
        ...extra,
      })
      const writeCheckpoint = async (
        phase: "model" | "tool_execution" | "verification" | "awaiting_input" | "awaiting_approval" | "waiting_external" | "finalizing",
        step: number,
        state: RunCheckpointState,
      ) => {
        checkpointRevision = await this.saveCheckpoint(run.id, checkpointRevision, phase, step, state)
      }

      if (checkpoint?.phase === "waiting_external" && checkpoint.state.waitingTask) {
        signal.throwIfAborted()
        const waitingTask = checkpoint.state.waitingTask
        const wakeAt = new Date().toISOString()
        outcome = "waiting_external"
        await this.repository.updateRun(run.id, "running", "waiting_external", {
          wakeAt,
          eventData: {
            projectId: waitingTask.projectId,
            taskType: waitingTask.taskType,
            taskId: waitingTask.taskId,
            timeoutSeconds: waitingTask.timeoutSeconds,
            deadlineAt: waitingTask.deadlineAt,
          },
        })
        span.setAttribute("luna.run.outcome", outcome)
        return outcome
      }

      let awaitingToolCallId = resume.type === "waiting_approval" ? resume.toolCallId : undefined
      if (resume.type === "settle_tools" || resume.type === "waiting_approval") {
        try {
          const pending = await this.toolStep.recoverInterruptedBatch(run.id, toolState)
          awaitingToolCallId = pending.awaitingToolCallId ?? awaitingToolCallId
        }
        catch (error) {
          if (error instanceof Error && error.message.startsWith("ai.")) throw error
          throw new Error("ai.tool_persistence_failed", { cause: error })
        }
      }

      if (executionInput.pageContext.__lunaDirectToolAction === true) {
        const directFromStore = (await this.tools.listByRun(run.id)).find(call => call.inputMode === "direct")
        const directToolItem = executionInput.toolInteractions.find(item =>
          item.type === "tool_call"
          && item.content.inputMode !== "verification"
          && typeof item.content.toolCallId === "string")
        const toolCallId = directFromStore?.id ?? (directToolItem ? String(directToolItem.content.toolCallId) : undefined)
        if (!toolCallId) throw new Error("ai.direct_tool_not_ready")
        let directCall = await this.tools.inspect(toolCallId)
        if (directCall.status === "proposed") {
          directCall = await this.tools.executePendingCall(directCall.id, signal)
        }
        if (directCall.status === "awaiting_approval") {
          signal.throwIfAborted()
          outcome = "waiting_approval"
          telemetryLog("agent.run.waiting_approval", "info", { "luna.run.id": run.id, "tool.name": directCall.operationId })
          await writeCheckpoint("awaiting_approval", modelStep, currentState({ pendingToolCallId: directCall.id }))
          signal.throwIfAborted()
          await this.repository.updateRun(run.id, "running", "waiting_approval")
          span.setAttribute("luna.run.outcome", outcome)
          return outcome
        }
        if (directCall.status === "rejected") {
          await writeCheckpoint("finalizing", modelStep, currentState())
          try {
            outcome = await this.finalizer.completeDirectTool(run, "completed", directCall.operationId)
          }
          catch (error) {
            return await this.handleTerminalPersistenceFailure(run, span, error)
          }
          span.setAttribute("luna.run.outcome", outcome)
          return outcome
        }
        const directOutcome = await this.tools.outcome(directCall.id)
        if (directOutcome.kind === "failed" || directCall.status === "failed" || directCall.status === "canceled") {
          throw new Error(directOutcome.kind === "failed" ? directOutcome.errorCode : (directCall.errorCode ?? "ai.tool_execution_failed"))
        }
        await writeCheckpoint("finalizing", modelStep, currentState())
        try {
          outcome = await this.finalizer.completeDirectTool(
            run,
            "completed",
            directCall.operationId,
          )
        }
        catch (error) {
          return await this.handleTerminalPersistenceFailure(run, span, error)
        }
        span.setAttribute("luna.run.outcome", outcome)
        return outcome
      }

      if (awaitingToolCallId) {
        const settled = await this.resumeToolCall(awaitingToolCallId, toolState, executionInput, signal)
        if (!settled) {
          signal.throwIfAborted()
          outcome = "waiting_approval"
          telemetryLog("agent.run.waiting_approval", "info", { "luna.run.id": run.id })
          await writeCheckpoint("awaiting_approval", modelStep, currentState({
            pendingToolCallId: awaitingToolCallId,
            ...(checkpoint?.state.pendingBatchId ? { pendingBatchId: checkpoint.state.pendingBatchId } : {}),
            ...(checkpoint?.state.pendingBatchCalls?.length ? { pendingBatchCalls: checkpoint.state.pendingBatchCalls } : {}),
          }))
          signal.throwIfAborted()
          await this.repository.updateRun(run.id, "running", "waiting_approval")
          span.setAttribute("luna.run.outcome", outcome)
          return outcome
        }
      }

      const handlePausedBatch = async (batch: ToolBatchResult): Promise<string | undefined> => {
        if (batch.kind === "waiting_approval") {
          signal.throwIfAborted()
          outcome = "waiting_approval"
          await writeCheckpoint("awaiting_approval", modelStep, currentState({
            pendingToolCallId: batch.toolCallId,
            ...(batch.batchId ? { pendingBatchId: batch.batchId } : {}),
            ...(batch.remainingCalls?.length ? { pendingBatchCalls: batch.remainingCalls } : {}),
          }))
          signal.throwIfAborted()
          await this.repository.updateRun(run.id, "running", "waiting_approval")
          span.setAttribute("luna.run.outcome", outcome)
          return outcome
        }
        if (batch.kind === "waiting_external") {
          signal.throwIfAborted()
          outcome = "waiting_external"
          telemetryLog("agent.run.waiting_external", "info", {
            "luna.run.id": run.id,
            "tool.name": "wait_for_task",
            "luna.task.type": batch.waitingTask.taskType,
          })
          await writeCheckpoint("waiting_external", modelStep, currentState({
            waitingTask: batch.waitingTask,
            ...(batch.batchId ? { pendingBatchId: batch.batchId } : {}),
            ...(batch.remainingCalls?.length ? { pendingBatchCalls: batch.remainingCalls } : {}),
          }))
          signal.throwIfAborted()
          await this.repository.updateRun(run.id, "running", "waiting_external", {
            wakeAt: batch.wakeAt,
            eventData: {
              projectId: batch.waitingTask.projectId,
              taskType: batch.waitingTask.taskType,
              taskId: batch.waitingTask.taskId,
              timeoutSeconds: batch.waitingTask.timeoutSeconds,
              deadlineAt: batch.waitingTask.deadlineAt,
            },
          })
          span.setAttribute("luna.run.outcome", outcome)
          return outcome
        }
        return undefined
      }

      let finalAnswer = ""
      let completed = false
      let finalResponseMissing = false
      let finalFinishReason: string | undefined
      let lastTotalTokens = executionInput.lastTotalTokens
      const recoveredFinal = resume.type === "finalize"
      const recoveredFinalOutput = resume.type === "finalize" ? resume.finalOutput : undefined
      if (resume.type === "finalize") {
        if (resume.finalOutput !== undefined) finalAnswer = resume.finalOutput
        completed = true
      }

      if (!completed && checkpoint?.state.pendingBatchCalls?.length) {
        const settledCallIds = new Set(
          toolState.continuationMessages.flatMap(message => message.role === "tool" ? [message.toolCallId] : []),
        )
        const remainingPendingCalls = checkpoint.state.pendingBatchCalls.filter(call => !settledCallIds.has(call.id))
        if (remainingPendingCalls.length > 0) {
          await writeCheckpoint("tool_execution", modelStep, currentState({
            ...(checkpoint.state.pendingBatchId ? { pendingBatchId: checkpoint.state.pendingBatchId } : {}),
            pendingBatchCalls: remainingPendingCalls,
          }))
          const resumedBatch = await this.toolStep.execute({
            run,
            executionInput,
            runtimeSettings,
            answer: "",
            reasoning: "",
            calls: remainingPendingCalls.map(call => ({
              id: call.id,
              operationId: call.operationId,
              arguments: call.arguments,
              ...(call.argumentError ? { argumentError: call.argumentError } : {}),
            })),
            ...(checkpoint.state.pendingBatchId ? { batchId: checkpoint.state.pendingBatchId } : {}),
            appendToExistingBatch: true,
            signal,
          }, toolState)
          const resumedAction = toolBatchAction(resumedBatch, "tool_execution")
          const pausedOutcome = await handlePausedBatch(resumedBatch)
          if (pausedOutcome) return pausedOutcome
          if (resumedAction.type === "complete") {
            completed = true
          }
        }
      }

      let finalSummaryAttempted = false
      const runGracefulFinalSummaryStep = async (stepIndex: number): Promise<void> => {
        finalSummaryAttempted = true
        modelStep = stepIndex
        if (executionInput.model) {
          await this.repository.assertPositiveAvailableBalance(run.ownerUserId)
        }
        toolState!.continuationMessages.push({
          role: "user",
          content: gracefulFinalSummaryInstruction,
        })
        await writeCheckpoint("model", stepIndex, currentState())
        const summaryResult = await this.streamModel(run.id, run.turnId, {
          runId: run.id,
          ownerUserId: run.ownerUserId,
          conversationId: executionInput.conversationId,
          input: executionInput.input,
          pageContext: executionInput.pageContext,
          history: executionInput.history,
          conversation: toolState!.conversation,
          promptVersion: run.promptVersion,
          reasoning: "",
          answer: "",
          toolCalls: [],
          continuationMessages: toolState!.continuationMessages,
          loadedOperationIds: [],
          disableTools: true,
          toolCatalogDigest: input.toolCatalogDigest,
          runtimeSettings,
          providerConfig,
          ...(lastTotalTokens !== undefined ? { lastTotalTokens } : {}),
          ...(executionInput.model ? { model: executionInput.model } : {}),
        }, signal, run.rowVersion)
        lastTotalTokens = summaryResult.lastTotalTokens
        pendingTerminal = summaryResult.finalizeTerminal
        pendingFlushStream = summaryResult.flushStream
        finalAnswer = summaryResult.answer
        finalResponseMissing = false
        const summaryAction = decisionAction(modelDecision(summaryResult), {
          protocolRepairCount,
          cardRepairExhausted: toolState!.cardRepairExhausted,
        }, "model")
        if (summaryAction.type === "fail") throw new Error(summaryAction.errorCode)
        if (summaryAction.type === "finalize") {
          finalFinishReason = summaryAction.finishReason
          completed = true
          return
        }
        if (summaryResult.answer.trim().length > 0) {
          finalFinishReason = summaryResult.finishReason
          completed = true
          return
        }
        throw new Error("ai.final_response_missing")
      }

      for (let step = modelStep; !completed && step < runtimeSettings.maxModelSteps; step += 1) {
        const hasPriorToolStep = toolState.continuationMessages.some(message => message.role === "tool")
        const isFinalSummaryStep = runtimeSettings.maxModelSteps >= 2
          && step === runtimeSettings.maxModelSteps - 1
          && !finalResponseMissing
          && hasPriorToolStep
        if (isFinalSummaryStep) {
          await runGracefulFinalSummaryStep(step)
          break
        }
        modelStep = step
        if (executionInput.model) {
          await this.repository.assertPositiveAvailableBalance(run.ownerUserId)
        }
        await writeCheckpoint("model", step, currentState())
        const result = await this.streamModel(run.id, run.turnId, {
          runId: run.id,
          ownerUserId: run.ownerUserId,
          conversationId: executionInput.conversationId,
          input: executionInput.input,
          pageContext: executionInput.pageContext,
          history: executionInput.history,
          conversation: toolState.conversation,
          promptVersion: run.promptVersion,
          reasoning: "",
          answer: "",
          toolCalls: [],
          continuationMessages: toolState.continuationMessages,
          loadedOperationIds: [...toolState.selectedOperationIds],
          toolCatalogDigest: input.toolCatalogDigest,
          runtimeSettings,
          providerConfig,
          ...(lastTotalTokens !== undefined ? { lastTotalTokens } : {}),
          ...(executionInput.model ? { model: executionInput.model } : {}),
        }, signal, run.rowVersion)
        lastTotalTokens = result.lastTotalTokens
        pendingTerminal = result.finalizeTerminal
        pendingFlushStream = result.flushStream
        finalAnswer = result.answer
        finalResponseMissing = false
        const action = decisionAction(modelDecision(result), {
          protocolRepairCount,
          cardRepairExhausted: toolState.cardRepairExhausted,
        }, "model")
        if (action.type === "fail") throw new Error(action.errorCode)
        if (action.type === "repair") {
          protocolRepairCount += 1
          toolState.continuationMessages.push({
            role: "assistant",
            content: result.answer.trim() || "（未输出有效工具调用）",
            ...(result.reasoning ? { reasoning: result.reasoning } : {}),
          })
          toolState.continuationMessages.push({ role: "user", content: protocolRepairInstruction })
          continue
        }
        if (action.type === "retry") {
          finalResponseMissing = true
          continue
        }
        if (action.type === "finalize") {
          finalFinishReason = action.finishReason
          completed = true
          break
        }
        const batchId = `batch_${step}`
        const calls = action.calls.map((call, index) => ({
          ...call,
          id: call.id ?? `call_${step}_${index}`,
        }))
        modelStep = step + 1
        await writeCheckpoint("tool_execution", modelStep, currentState())
        const batch = await this.toolStep.execute({
          run,
          executionInput,
          runtimeSettings,
          answer: result.answer,
          reasoning: result.reasoning,
          calls,
          batchId,
          signal,
        }, toolState)
        const batchAction = toolBatchAction(batch, "tool_execution")
        const pausedOutcome = await handlePausedBatch(batch)
        if (pausedOutcome) return pausedOutcome
        if (batchAction.type === "complete") {
          completed = true
          break
        }
      }
      if (!completed && !finalSummaryAttempted && !finalResponseMissing && toolState.continuationMessages.some(message => message.role === "tool")) {
        await runGracefulFinalSummaryStep(modelStep)
      }
      if (!completed) throw new Error(finalResponseMissing ? "ai.final_response_missing" : "ai.limit_exceeded")
      const finalOutputForCheckpoint = pendingTerminal ? redact(finalAnswer) : recoveredFinalOutput
      await writeCheckpoint("finalizing", modelStep, currentState(
        finalOutputForCheckpoint === undefined ? {} : { finalOutput: finalOutputForCheckpoint },
      ))
      try {
        outcome = await this.finalizer.complete({
          run,
          executionInput,
          finalAnswer,
          ...(finalFinishReason === undefined ? {} : { finalFinishReason }),
          ...(pendingTerminal ? { pendingTerminal } : {}),
          recoveredFinal,
          assistantRenamed: toolState.assistantRenamed,
          providerConfig,
          signal,
          span,
        })
      }
      catch (error) {
        return await this.handleTerminalPersistenceFailure(run, span, error)
      }
      span.setAttribute("luna.run.outcome", outcome)
      return outcome
    }
    catch (caughtError) {
      const error = caughtError instanceof ToolInterruption && signal.aborted
        ? (signal.reason instanceof Error ? signal.reason : new Error("ai.run_canceled"))
        : caughtError
      if (error instanceof StreamModelFinalizationError) {
        pendingTerminal = error.finalizeTerminal
        pendingFlushStream = error.flushStream
      }
      if (error instanceof TerminalPersistenceExhaustedError) {
        return await this.handleTerminalPersistenceFailure(run, span, error)
      }
      const message = error instanceof Error ? error.message : "ai.run_failed"
      const errorCode = stableErrorCode(error)
      const canceled = errorCode === "ai.run_canceled"
        || (error instanceof RunStateConflictError && error.actualStatus === "canceled")
      if (toolState?.cardGeneration) {
        const cardError = canceled ? "ai.run_canceled" : errorCode === "ai.agent_stopping" ? "ai.agent_stopping" : stableError(message)
        try {
          if (pendingFlushStream) {
            await pendingFlushStream("failed")
            pendingFlushStream = undefined
            pendingTerminal = undefined
          }
          await this.toolStep.failCard(run.id, toolState, cardError)
        }
        catch (persistenceError) {
          return await this.handleTerminalPersistenceFailure(run, span, persistenceError)
        }
      }
      if (canceled) {
        if (error instanceof RunStateConflictError) {
          span.setAttribute("luna.run.expected_status", error.expectedStatus)
          span.setAttribute("luna.run.actual_status", error.actualStatus ?? "missing")
          span.setAttribute("luna.run.target_status", error.targetStatus)
        }
        try {
          outcome = await this.finalizer.cancel(
            run,
            error,
            pendingTerminal,
            error instanceof RunStateConflictError ? "durable_state" : "abort_signal",
          )
        }
        catch (persistenceError) {
          return await this.handleTerminalPersistenceFailure(run, span, persistenceError)
        }
        span.setAttribute("luna.run.outcome", outcome)
        if (outcome === "failed") recordSpanError(span, error)
        return outcome
      }
      if (errorCode === "ai.agent_stopping") {
        try {
          outcome = await this.finalizer.interrupt(run, pendingTerminal)
        }
        catch (persistenceError) {
          return await this.handleTerminalPersistenceFailure(run, span, persistenceError)
        }
        span.setAttribute("luna.run.outcome", outcome)
        if (outcome === "failed") recordSpanError(span, error)
        return outcome
      }
      if (error instanceof ToolInterruption && error.state === "waiting_input") {
        outcome = "waiting_input"
        telemetryLog("agent.run.waiting_input", "info", {
          "luna.run.id": run.id,
          "operation": "agent.run",
          "outcome": outcome,
        })
        try {
          checkpointRevision = await this.saveCheckpoint(run.id, checkpointRevision, "awaiting_input", modelStep, checkpointState({
            loopGuard: this.tools.runLoopState(run.id),
            protocolRepairCount,
            argumentRepairAttempts: toolState?.providerArgumentRepairAttempts ?? new Map(),
            ...(toolState?.cardGeneration ? { cardGeneration: toolState.cardGeneration } : {}),
            ...(toolState?.cardRepairExhausted ? { cardRepairExhausted: true } : {}),
          }))
          await this.repository.appendEvent(run.id, "run.input_required", { fields: error.fields })
          await this.repository.updateRun(run.id, "running", "waiting_input")
        }
        catch (persistenceError) {
          return await this.handleTerminalPersistenceFailure(run, span, persistenceError)
        }
        span.setAttribute("luna.run.outcome", outcome)
        return outcome
      }
      outcome = error instanceof ToolInterruption ? error.state : stableErrorCode(error)
      if (error instanceof RunStateConflictError) {
        span.setAttribute("luna.run.expected_status", error.expectedStatus)
        span.setAttribute("luna.run.actual_status", error.actualStatus ?? "missing")
        span.setAttribute("luna.run.target_status", error.targetStatus)
      }
      try {
        outcome = await this.finalizer.fail(run, error, pendingTerminal)
      }
      catch (persistenceError) {
        return await this.handleTerminalPersistenceFailure(run, span, persistenceError)
      }
      span.setAttribute("luna.run.outcome", outcome)
      if (outcome === "failed") recordSpanError(span, error)
      return outcome
    }
  }

  async failBeforeExecution(run: ClaimedRun, error: unknown, span: Span): Promise<string> {
    return await this.handleTerminalPersistenceFailure(run, span, error)
  }

  private async handleTerminalPersistenceFailure(run: ClaimedRun, span: Span, error: unknown): Promise<string> {
    const outcome = "terminal_persistence_exhausted"
    span.setAttribute("luna.run.outcome", outcome)
    recordSpanError(span, error)
    await this.finalizer.terminalPersistenceExhausted(run, error)
    return outcome
  }

  private async saveCheckpoint(
    runId: string,
    expectedRevision: number,
    phase: "model" | "tool_execution" | "verification" | "awaiting_input" | "awaiting_approval" | "waiting_external" | "finalizing",
    modelStep: number,
    state: RunCheckpointState,
  ): Promise<number> {
    const saved = await this.repository.saveRunCheckpoint(runId, {
      expectedRevision,
      phase,
      modelStep,
      toolCallCount: this.tools.toolLoopSnapshot(runId).executed,
      state,
    })
    return saved.revision
  }

  private async resumeToolCall(
    toolCallId: string,
    toolState: ToolStepState,
    executionInput: RunExecutionInput,
    signal: AbortSignal,
  ): Promise<boolean> {
    let call = await this.tools.inspect(toolCallId)
    if (!call) throw new Error("ai.tool_call_not_found")
    if (call.status === "proposed" && call.approvalDecision === "approve") {
      call = await this.tools.executePendingCall(toolCallId, signal)
      if (call.status !== "awaiting_approval") {
        const itemId = `${call.id}:item`
        const projected = executionInput.toolInteractions.find(item => item.itemId === itemId)
        const projectedModelCallId = typeof projected?.content.modelCallId === "string" && projected.content.modelCallId
          ? projected.content.modelCallId
          : undefined
        const effectiveCallId = call.modelCallId ?? projectedModelCallId ?? call.id
        const alreadyInMessages = toolState.continuationMessages.some(
          message => message.role === "tool" && (message.toolCallId === effectiveCallId || message.toolCallId === call.id),
        )
        if (!alreadyInMessages) {
          const outcome = await this.tools.outcome(call.id)
          const reasoning = reasoningByToolCallItemId(executionInput.toolInteractions).get(itemId)
          const toolCall = { id: effectiveCallId, operationId: call.operationId, arguments: call.arguments }
          toolState.continuationMessages.push({
            role: "assistant",
            content: "",
            toolCalls: [toolCall],
            ...(reasoning ? { reasoning } : {}),
          })
          toolState.continuationMessages.push(toolResultMessage(toolCall, {
            status: outcome.kind === "failed" ? "failed" : call.status,
            ...(call.result !== undefined ? { result: call.result } : {}),
            ...(outcome.kind === "failed" ? { errorCode: outcome.errorCode } : call.errorCode ? { errorCode: call.errorCode } : {}),
          }))
        }
      }
    }
    if (call.status === "awaiting_approval") return false
    if (call.status === "proposed" || call.status === "running") throw new Error("ai.tool_outcome_unknown")
    return true
  }

  private streamModel(runId: string, turnId: string, input: Parameters<typeof streamModel>[4], signal: AbortSignal, expectedRunVersion?: number) {
    return streamModel(this.streamBus, this.modelRuntime, runId, turnId, input, signal, expectedRunVersion)
  }
}
