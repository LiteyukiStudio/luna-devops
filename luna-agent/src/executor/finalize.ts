import type { Span } from "@opentelemetry/api"
import { createId } from "../id.js"
import type { ClaimedRun } from "../domain.js"
import type { ModelRuntime } from "../model-runtime.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import type { RemoteProviderConfig } from "../provider/config-client.js"
import { genAIOutputMessages } from "../genai-semconv.js"
import { redact } from "../redaction.js"
import type { RunStreamBus } from "../run-stream-bus.js"
import {
  agentMetrics,
  errorDiagnostic,
  recordAIContent,
  stableErrorCode,
  telemetryLog,
} from "../telemetry.js"
import type { RunExecutionInput } from "./tool-step.js"
import { stableError } from "./tool-results.js"

const titleTimeoutMs = 4_000

export type TerminalCommit = (
  to: "completed" | "failed" | "canceled" | "interrupted",
  errorCode?: string,
  conversationTitle?: string,
) => Promise<void>

export type CompleteRunInput = {
  run: ClaimedRun
  executionInput: RunExecutionInput
  finalAnswer: string
  finalFinishReason?: string
  pendingTerminal?: TerminalCommit
  recoveredFinal: boolean
  assistantRenamed: boolean
  providerConfig?: RemoteProviderConfig
  signal: AbortSignal
  span: Span
}

export class RunFinalizer {
  constructor(
    private readonly repository: Repository,
    private readonly streamBus: RunStreamBus,
    private readonly modelRuntime: ModelRuntime,
  ) {}

  async complete(input: CompleteRunInput): Promise<ClaimedRun["status"]> {
    const { run, executionInput, finalAnswer, pendingTerminal, recoveredFinal, assistantRenamed } = input
    recordAIContent(input.span, "luna.gen_ai.content.output", "gen_ai.output.messages", genAIOutputMessages({
      text: finalAnswer,
      finishReason: input.finalFinishReason ?? "stop",
    }))
    input.span.setAttribute("gen_ai.response.finish_reasons", [input.finalFinishReason ?? "stop"])
    const result = await this.commitTerminal(async () => {
      await this.repository.finalizeStreamingItems(run.id, "completed")
      if (pendingTerminal) {
        await pendingTerminal("completed")
      }
      else if (recoveredFinal && finalAnswer.trim()) {
        await this.commitRecoveredFinalMessage(run, finalAnswer)
      }
      else {
        await this.repository.updateRun(run.id, "running", "completed", { completedAt: new Date().toISOString() })
      }
      return { status: "completed", committed: true }
    })
    if (!result.committed) {
      telemetryLog("agent.run.terminal_race", "info", {
        "luna.run.id": run.id,
        "luna.run.expected_status": "completed",
        "luna.run.actual_status": result.status,
      })
      return result.status
    }
    telemetryLog("agent.run.completed", "info", { "luna.run.id": run.id })
    try {
      await this.generateTitleAfterCompletion(run, executionInput, finalAnswer, assistantRenamed, input.providerConfig, input.signal)
    }
    finally {
      await this.appendPostProcessCompleted(run.id)
    }
    return "completed"
  }

  async completeDirectTool(
    run: ClaimedRun,
    status: "completed" | "failed",
    toolName: string,
  ): Promise<ClaimedRun["status"]> {
    const result = await this.commitTerminal(async () => {
      await this.repository.updateRun(run.id, "running", status, { completedAt: new Date().toISOString() })
      return { status, committed: true }
    })
    if (!result.committed) {
      telemetryLog("agent.run.terminal_race", "info", {
        "luna.run.id": run.id,
        "luna.run.expected_status": status,
        "luna.run.actual_status": result.status,
      })
      return result.status
    }
    telemetryLog("agent.direct_tool.completed", "info", {
      "luna.run.id": run.id,
      "tool.name": toolName,
      "tool.outcome": status,
    })
    if (status === "completed") await this.appendPostProcessCompleted(run.id)
    return status
  }

  async cancel(
    run: ClaimedRun,
    error: unknown,
    pendingTerminal?: TerminalCommit,
    source: "abort_signal" | "durable_state" = "abort_signal",
  ): Promise<ClaimedRun["status"]> {
    const errorCode = stableErrorCode(error)
    const result = await this.commitTerminal(async () => {
      await this.repository.finalizeStreamingItems(run.id, "failed")
      if (pendingTerminal) {
        await pendingTerminal("canceled", errorCode)
        return { status: "canceled", committed: true }
      }
      else {
        const canceled = await this.repository.cancelRun(run.ownerUserId, run.id)
        if (canceled) return { status: "canceled", committed: true }
        return { status: await this.requireConfirmedTerminal(run), committed: false }
      }
    })
    if (result.status === "canceled") {
      telemetryLog("agent.run.canceled", "info", {
        "luna.run.id": run.id,
        "luna.run.cancel_source": source,
      })
    }
    else {
      telemetryLog("agent.run.terminal_race", "info", {
        "luna.run.id": run.id,
        "luna.run.expected_status": "canceled",
        "luna.run.actual_status": result.status,
      })
    }
    return result.status
  }

  async interrupt(run: ClaimedRun, pendingTerminal?: TerminalCommit): Promise<ClaimedRun["status"]> {
    const errorCode = "ai.agent_stopping"
    const result = await this.commitTerminal(async () => {
      await this.repository.finalizeStreamingItems(run.id, "failed")
      if (pendingTerminal) {
        await pendingTerminal("interrupted", errorCode)
      }
      else {
        await this.repository.updateRun(run.id, "running", "interrupted", {
          completedAt: new Date().toISOString(),
          errorCode,
        })
      }
      return { status: "interrupted", committed: true }
    })
    if (result.status === "interrupted") {
      telemetryLog("agent.run.interrupted", "info", { "luna.run.id": run.id })
    }
    else {
      telemetryLog("agent.run.terminal_race", "info", {
        "luna.run.id": run.id,
        "luna.run.expected_status": "interrupted",
        "luna.run.actual_status": result.status,
      })
    }
    return result.status
  }

  async fail(run: ClaimedRun, error: unknown, pendingTerminal?: TerminalCommit): Promise<ClaimedRun["status"]> {
    const message = error instanceof Error ? error.message : "ai.run_failed"
    const errorCode = stableError(message)
    const result = await this.commitTerminal(async () => {
      await this.repository.finalizeStreamingItems(run.id, "failed")
      if (pendingTerminal) {
        await pendingTerminal("failed", errorCode)
      }
      else {
        await this.repository.updateRun(run.id, "running", "failed", {
          completedAt: new Date().toISOString(),
          errorCode,
        })
      }
      return { status: "failed", committed: true }
    })
    if (result.status === "failed") {
      telemetryLog("agent.run.failed", "error", {
        "luna.run.id": run.id,
        "operation": "agent.run",
        "outcome": "failed",
        ...errorDiagnostic(error, stableErrorCode(error)),
      })
    }
    else {
      telemetryLog("agent.run.terminal_race", "info", {
        "luna.run.id": run.id,
        "luna.run.expected_status": "failed",
        "luna.run.actual_status": result.status,
      })
    }
    return result.status
  }

  async terminalPersistenceExhausted(run: ClaimedRun, error: unknown): Promise<void> {
    let interrupted = await this.repository.interruptOrphanedRun(run.id, run.rowVersion).catch(() => false)
    if (!interrupted) {
      const current = await this.repository.getRun(run.ownerUserId, run.id).catch(() => undefined)
      if (current?.status === "running")
        interrupted = await this.repository.interruptOrphanedRun(run.id, current.rowVersion).catch(() => false)
    }
    if (interrupted) await this.cleanupTerminalStream(run.id)
    telemetryLog("agent.run.terminal_persistence_exhausted", "error", {
      operation: "agent.run.terminal_persistence",
      outcome: "exhausted",
      "luna.run.interrupted": interrupted,
      ...errorDiagnostic(error, stableErrorCode(error)),
    })
  }

  private async generateTitleAfterCompletion(
    run: ClaimedRun,
    executionInput: RunExecutionInput,
    finalAnswer: string,
    assistantRenamed: boolean,
    providerConfig?: RemoteProviderConfig,
    signal?: AbortSignal,
  ): Promise<void> {
    if (executionInput.conversation.titleSource !== "default" || assistantRenamed) return
    try {
      const titleSignal = signal
        ? AbortSignal.any([signal, AbortSignal.timeout(titleTimeoutMs)])
        : AbortSignal.timeout(titleTimeoutMs)
      const title = await this.modelRuntime.generateConversationTitle(
        executionInput.input,
        finalAnswer,
        { runId: run.id, ownerUserId: run.ownerUserId },
        titleSignal,
        executionInput.model,
        providerConfig,
      )
      if (title) await this.repository.renameConversationByAssistant(run.conversationId, title, run.id)
    }
    catch (error) {
      telemetryLog("agent.conversation.title.failed", "warn", {
        operation: "agent.conversation.title",
        outcome: "failed",
        "luna.run.id": run.id,
        ...errorDiagnostic(error, stableErrorCode(error)),
      })
    }
  }

  private async appendPostProcessCompleted(runId: string): Promise<void> {
    try {
      await this.repository.appendEvent(runId, "run.postprocess.completed", {})
    }
    catch (error) {
      telemetryLog("agent.run.postprocess_marker_failed", "warn", {
        operation: "agent.run.postprocess_marker",
        outcome: "failed",
        "luna.run.id": runId,
        ...errorDiagnostic(error, "ai.persistence_unavailable"),
      })
    }
  }

  private async commitTerminal(
    operation: () => Promise<{ status: ClaimedRun["status"], committed: boolean }>,
  ): Promise<{ status: ClaimedRun["status"], committed: boolean }> {
    try {
      return await operation()
    }
    catch (error) {
      if (!isConfirmedTerminalRace(error)) throw error
      return { status: error.actualStatus, committed: false }
    }
  }

  private async requireConfirmedTerminal(run: ClaimedRun): Promise<ClaimedRun["status"]> {
    const current = await this.repository.getRun(run.ownerUserId, run.id)
    if (!current) throw new Error("ai.run_terminal_state_unknown")
    if (!isTerminalStatus(current.status)) throw new Error("ai.run_terminal_conflict")
    return current.status
  }

  private async commitRecoveredFinalMessage(run: ClaimedRun, finalOutput: string): Promise<void> {
    const session = await this.streamBus.open(run.id, run.turnId, run.rowVersion)
    const content = redact({ parts: [{ type: "text", text: finalOutput }] })
    const itemId = createId("aiitm")
    const { item } = await session.appendItemWithEvent({
      id: itemId, runId: run.id, turnId: run.turnId, type: "assistant_message", status: "streaming", content,
    }, "content.delta", redact({ itemId, contentPartId: `${itemId}:0`, partIndex: 0, delta: finalOutput }))
    await session.updateItemWithEvent(itemId, "completed", content, "message.completed", redact({
      itemId, contentPartId: `${itemId}:0`, partIndex: 0, timelineIndex: item.timelineIndex,
    }))
    await session.commitTerminal("completed")
  }

  private async cleanupTerminalStream(runId: string): Promise<void> {
    try {
      await this.streamBus.cleanup(runId)
    }
    catch (error) {
      agentMetrics.streamCleanups.add(1, { outcome: "failed" })
      telemetryLog("agent.stream.cleanup_failed", "warn", {
        operation: "agent.stream.cleanup",
        outcome: "failed",
        ...errorDiagnostic(error, "ai.stream_cleanup_failed"),
      })
    }
  }
}

function isConfirmedTerminalRace(error: unknown): error is RunStateConflictError & { actualStatus: ClaimedRun["status"] } {
  return error instanceof RunStateConflictError
    && error.actualStatus !== undefined
    && isTerminalStatus(error.actualStatus)
}

function isTerminalStatus(status: string): boolean {
  return ["completed", "failed", "canceled", "expired", "interrupted"].includes(status)
}
