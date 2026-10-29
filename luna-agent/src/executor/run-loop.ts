import type { Span } from "@opentelemetry/api"
import type { ClaimedRun, RunCheckpointState, RunToolVerification } from "../domain.js"
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
import { checkpointState, resumePlan } from "./checkpoint.js"
import { modelDecision } from "./decision.js"
import { RunFinalizer, type TerminalCommit } from "./finalize.js"
import { decisionAction, resumeAction, toolBatchAction } from "./graph.js"
import { StreamModelFinalizationError, streamModel } from "./streaming.js"
import { stableError } from "./tool-results.js"
import { ToolStepInterruption, ToolStepRunner, type ToolStepState } from "./tool-step.js"
import { ToolVerificationRunner } from "./verification.js"

const protocolRepairInstruction = "系统指令：上一步的结束原因表明需要调用工具，但没有任何可解析的工具调用。请直接调用所需工具，或给出最终答复。"

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
  private readonly verification: ToolVerificationRunner

  constructor(
    private readonly repository: Repository,
    private readonly modelRuntime: ModelRuntime,
    private readonly tools: ToolOrchestrator,
    private readonly streamBus: RunStreamBus,
    verificationPollIntervalMs = 1_500,
  ) {
    this.toolStep = new ToolStepRunner(repository, modelRuntime, tools)
    this.finalizer = new RunFinalizer(repository, streamBus, modelRuntime)
    this.verification = new ToolVerificationRunner(tools, verificationPollIntervalMs)
  }

  async execute(input: RunLoopInput): Promise<string> {
    const { run, runtimeSettings, providerConfig, signal, span } = input
    let outcome = "completed"
    let pendingTerminal: TerminalCommit | undefined
    let checkpointRevision = 0
    let modelStep = 0
    let protocolRepairCount = 0
    let toolState: ToolStepState | undefined
    let pendingVerifications: RunToolVerification[] = []
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
      pendingVerifications = [...checkpoint?.state.pendingVerifications ?? []]
      toolState = await this.toolStep.initialState(run, executionInput, checkpoint)
      const currentState = (extra: Omit<Parameters<typeof checkpointState>[0], "loopGuard" | "protocolRepairCount" | "argumentRepairAttempts"> = {}) => checkpointState({
        loopGuard: this.tools.runLoopState(run.id),
        protocolRepairCount,
        argumentRepairAttempts: toolState?.providerArgumentRepairAttempts ?? new Map(),
        pendingVerifications,
        completionBlockers: toolState?.completionBlockers ?? [],
        ...extra,
      })
      const writeCheckpoint = async (phase: "model" | "tool_execution" | "verification" | "awaiting_input" | "awaiting_approval" | "waiting_external" | "finalizing", step: number, state: RunCheckpointState) => {
        checkpointRevision = await this.saveCheckpoint(run.id, checkpointRevision, phase, step, state)
      }
      const settleVerifications = async () => {
        while (pendingVerifications.length > 0) {
          const current = pendingVerifications[0]!
          await writeCheckpoint("verification", modelStep, currentState())
          const execution = await this.verification.verify(current, signal)
          if (execution.kind === "pending") {
            const waiting = { ...current, attempts: current.attempts + 1 }
            pendingVerifications = [waiting, ...pendingVerifications.slice(1)]
            if (waiting.attempts === 1) {
              await this.repository.appendEvent(run.id, "run.waiting_external", {
                operationId: current.operationId,
                externalOperationId: current.externalOperationId,
              })
              telemetryLog("agent.run.waiting_external", "info", {
                "luna.run.id": run.id,
                "tool.name": current.operationId,
                "luna.verification.family": current.family,
              })
            }
            await writeCheckpoint("waiting_external", modelStep, currentState())
            span.addEvent("run.waiting_external", {
              "luna.verification.operation": current.operationId,
              "luna.verification.attempt": waiting.attempts,
            })
            await this.verification.wait(waiting.attempts, signal)
            continue
          }
          await this.toolStep.recordVerification(toolState!, current, execution, runtimeSettings)
          if (execution.kind === "failed") throw new Error(execution.errorCode)
          pendingVerifications = pendingVerifications.slice(1)
          if (pendingVerifications.length > 0)
            await writeCheckpoint("verification", modelStep, currentState())
        }
      }
      let awaitingToolCallId = resume.type === "waiting_approval" ? resume.toolCallId : undefined
      if (resume.type === "settle_tools" || resume.type === "waiting_approval") {
        try {
          const pending = await this.toolStep.recoverInterruptedBatch(run.id, toolState, runtimeSettings)
          awaitingToolCallId = pending.awaitingToolCallId ?? awaitingToolCallId
          pendingVerifications.push(...pending.verifications)
        }
        catch (error) {
          if (error instanceof Error && error.message.startsWith("ai.")) throw error
          throw new Error("ai.tool_persistence_failed", { cause: error })
        }
      }
      if (resume.type === "verify" || pendingVerifications.length > 0)
        await settleVerifications()
      if (executionInput.pageContext.__lunaDirectToolAction === true) {
        const directTool = executionInput.toolInteractions.find(item =>
          item.type === "tool_call"
          && item.content.inputMode !== "verification"
          && typeof item.content.toolCallId === "string")
        if (!directTool) throw new Error("ai.direct_tool_not_ready")
        const toolCallId = String(directTool.content.toolCallId)
        const directCall = await this.tools.inspect(toolCallId)
        const directOutcome = await this.tools.outcome(toolCallId)
        this.toolStep.trackOutcome(toolState, directCall, directOutcome)
        if (directOutcome.kind === "failed") throw new Error(directOutcome.errorCode)
        if (pendingVerifications.length === 0 && directCall.status === "succeeded") {
          const verification = await this.verification.verificationForToolCall(toolCallId)
          if (verification) {
            pendingVerifications.push(verification)
            await settleVerifications()
          }
        }
        this.toolStep.assertCompletionAllowed(toolState)
        const directStatus = directCall.status === "failed" || directCall.status === "rejected" || directCall.status === "canceled"
          ? "failed"
          : "completed"
        await writeCheckpoint("finalizing", modelStep, currentState())
        try {
          outcome = await this.finalizer.completeDirectTool(
            run,
            directStatus,
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
        const settled = await this.resumeToolCall(awaitingToolCallId)
        if (!settled) {
          outcome = "waiting_approval"
          telemetryLog("agent.run.waiting_approval", "info", { "luna.run.id": run.id })
          await writeCheckpoint("awaiting_approval", modelStep, currentState({ pendingToolCallId: awaitingToolCallId }))
          await this.repository.updateRun(run.id, "running", "waiting_approval")
          span.setAttribute("luna.run.outcome", outcome)
          return outcome
        }
      }
      let finalAnswer = ""
      let completed = false
      let finalResponseMissing = false
      let finalFinishReason: string | undefined
      const recoveredFinal = resume.type === "finalize"
      const recoveredFinalOutput = resume.type === "finalize" ? resume.finalOutput : undefined
      if (resume.type === "finalize") {
        if (resume.finalOutput !== undefined) finalAnswer = resume.finalOutput
        completed = true
      }
      for (let step = modelStep; !completed && step < runtimeSettings.maxModelSteps; step += 1) {
        modelStep = step
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
          reasoningSummary: "",
          answer: "",
          toolCalls: [],
          continuationMessages: toolState.continuationMessages,
          loadedOperationIds: [...toolState.selectedOperationIds],
          toolCatalogDigest: input.toolCatalogDigest,
          runtimeSettings,
          providerConfig,
          ...(executionInput.model ? { model: executionInput.model } : {}),
        }, signal, run.rowVersion)
        pendingTerminal = result.finalizeTerminal
        finalAnswer = result.answer
        finalResponseMissing = false
        const action = decisionAction(modelDecision(result), {
          protocolRepairCount,
          cardRepairExhausted: toolState.cardRepairExhausted,
        }, "model")
        if (action.type === "fail") throw new Error(action.errorCode)
        if (action.type === "repair") {
          protocolRepairCount += 1
          if (result.answer.trim()) toolState.continuationMessages.push({ role: "assistant", content: result.answer })
          toolState.continuationMessages.push({ role: "system", content: protocolRepairInstruction })
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
          calls,
          signal,
        }, toolState)
        const batchAction = toolBatchAction(batch, "tool_execution")
        if (batchAction.type === "verify") {
          if (batch.kind !== "verification") throw new Error("ai.run_graph_illegal_transition")
          pendingVerifications.push(...batch.verifications)
          await settleVerifications()
          continue
        }
        if (batchAction.type === "waiting_approval") {
          outcome = "waiting_approval"
          await writeCheckpoint("awaiting_approval", modelStep, currentState({ pendingToolCallId: batchAction.toolCallId }))
          await this.repository.updateRun(run.id, "running", "waiting_approval")
          span.setAttribute("luna.run.outcome", outcome)
          return outcome
        }
        if (batchAction.type === "complete") {
          completed = true
          break
        }
      }
      if (!completed) throw new Error(finalResponseMissing ? "ai.final_response_missing" : "ai.limit_exceeded")
      this.toolStep.assertCompletionAllowed(toolState)
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
    catch (error) {
      if (error instanceof StreamModelFinalizationError) pendingTerminal = error.finalizeTerminal
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
        if (error instanceof ToolStepInterruption)
          pendingVerifications = mergeVerifications(pendingVerifications, error.pendingVerifications)
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
            completionBlockers: toolState?.completionBlockers ?? [],
            pendingVerifications,
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

  private async resumeToolCall(toolCallId: string): Promise<boolean> {
    const call = await this.tools.inspect(toolCallId)
    if (!call) throw new Error("ai.tool_call_not_found")
    if (call.status === "awaiting_approval") return false
    if (call.status === "proposed" || call.status === "running") throw new Error("ai.tool_outcome_unknown")
    return true
  }

  private streamModel(runId: string, turnId: string, input: Parameters<typeof streamModel>[4], signal: AbortSignal, expectedRunVersion?: number) {
    return streamModel(this.streamBus, this.modelRuntime, runId, turnId, input, signal, expectedRunVersion)
  }
}

function mergeVerifications(current: RunToolVerification[], pending: RunToolVerification[]): RunToolVerification[] {
  const merged = new Map(current.map(item => [item.sourceToolCallId, item]))
  for (const item of pending) merged.set(item.sourceToolCallId, item)
  return [...merged.values()]
}
