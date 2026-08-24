import type { AssistantModelInput, ModelRuntime } from "../model-runtime.js"
import type { ModelToolCall } from "../provider/provider.js"
import { createId } from "../id.js"
import { redact } from "../redaction.js"
import { agentMetrics, errorDiagnostic, internalSpanOptions, isExpectedCancellation, recordAIContent, stableErrorCode, telemetryLog, withSpan } from "../telemetry.js"
import { contentError } from "./tool-results.js"
import type { RunStreamBus, RunStreamSession } from "../run-stream-bus.js"

export class StreamModelFinalizationError extends Error {
  override readonly name = "StreamModelFinalizationError"
  constructor(
    cause: unknown,
    readonly finalizeTerminal: (to: "completed" | "failed" | "canceled" | "interrupted", errorCode?: string, conversationTitle?: string) => Promise<void>,
  ) {
    super(stableErrorCode(cause), { cause })
  }
}

// 单次模型流式调用：消费 provider 事件流，把 reasoning/正文/tool call
// 投影到时间线，并记录首 token、用量与终态遥测。
export async function streamModel(
  streamBus: RunStreamBus,
  modelRuntime: ModelRuntime,
  runId: string,
  turnId: string,
  input: AssistantModelInput,
  signal: AbortSignal,
  expectedRunVersion?: number,
): Promise<AssistantModelInput & {
  finalizeTerminal?: (to: "completed" | "failed" | "canceled" | "interrupted", errorCode?: string, conversationTitle?: string) => Promise<void>
}> {
  const startedAt = performance.now()
  let outcome = "success"
  let stream: RunStreamSession | undefined
  return withSpan("agent.response.process", internalSpanOptions({
    "luna.run.id": runId,
    "luna.turn.id": turnId,
  }), async span => {
    const reasoningItemId = createId("aiitm")
    const messageItemId = createId("aiitm")
    let reasoningSummary = ""
    let answer = ""
    let toolCalls: ModelToolCall[] = []
    let firstOutputRecorded = false
    let reasoningTimelineIndex: number | undefined
    let messageTimelineIndex: number | undefined
    try {
      stream = await streamBus.open(runId, turnId, expectedRunVersion)
      await stream.appendEvent("model.started", {})
      for await (const event of modelRuntime.stream(input, signal)) {
        if (!firstOutputRecorded && ["reasoning_summary_delta", "message_delta", "tool_call_delta"].includes(event.type)) {
          firstOutputRecorded = true
          const outputType = event.type === "reasoning_summary_delta"
            ? "reasoning"
            : event.type === "message_delta" ? "message" : "tool_call"
          agentMetrics.modelFirstTokenDuration.record((performance.now() - startedAt) / 1000, { output_type: outputType })
          span.addEvent("luna.agent.first_output", { "luna.agent.output.type": outputType })
        }
        if (event.type === "context.compacted") {
          // 本轮发生了实际上下文压缩，向时间线写入一条系统提示，
          // 让前端推送一个轻量 badge 告知用户历史已被摘要。
          await stream.appendItemWithEvent({
            id: createId("aiitm"), runId, turnId, type: "system_notice", status: "completed",
            content: redact({
              notice: "context_compacted",
              summarizedThroughTurnIndex: event.summarizedThroughTurnIndex,
              sourceTurnCount: event.sourceTurnCount,
              trigger: event.trigger,
              ...(event.priorPromptTokens !== undefined ? { priorPromptTokens: event.priorPromptTokens } : {}),
            }),
          }, "context.compacted", redact({
            summarizedThroughTurnIndex: event.summarizedThroughTurnIndex,
            sourceTurnCount: event.sourceTurnCount,
            trigger: event.trigger,
            ...(event.priorPromptTokens !== undefined ? { priorPromptTokens: event.priorPromptTokens } : {}),
          }))
          continue
        }
        if (event.type === "reasoning_summary_delta" && event.delta) {
          reasoningSummary += event.delta
          if (reasoningTimelineIndex === undefined) {
            const { item } = await stream.appendItemWithEvent({
              id: reasoningItemId, runId, turnId, type: "reasoning_summary", status: "streaming",
              content: redact({ summary: reasoningSummary, display: "summary" }),
            }, "thinking.started", redact({
              itemId: reasoningItemId,
              summary: event.delta,
              display: "summary",
            }))
            reasoningTimelineIndex = item.timelineIndex
          }
          else {
            await stream.updateItemWithEvent(
              reasoningItemId,
              "streaming",
              redact({ summary: reasoningSummary, display: "summary" }),
              "thinking.delta",
              redact({ itemId: reasoningItemId, delta: event.delta, display: "summary", timelineIndex: reasoningTimelineIndex }),
            )
          }
        }
        if (event.type === "message_delta" && event.delta) {
          answer += event.delta
          if (messageTimelineIndex === undefined) {
            const { item } = await stream.appendItemWithEvent({
              id: messageItemId, runId, turnId, type: "assistant_message", status: "streaming",
              content: redact({ parts: [{ type: "text", text: answer }] }),
            }, "content.delta", redact({
              itemId: messageItemId,
              contentPartId: `${messageItemId}:0`,
              partIndex: 0,
              delta: event.delta,
            }))
            messageTimelineIndex = item.timelineIndex
          }
          else {
            await stream.updateItemWithEvent(
              messageItemId,
              "streaming",
              redact({ parts: [{ type: "text", text: answer }] }),
              "content.delta",
              redact({
                itemId: messageItemId,
                contentPartId: `${messageItemId}:0`,
                partIndex: 0,
                delta: event.delta,
                timelineIndex: messageTimelineIndex,
              }),
            )
          }
        }
        if (event.type === "completed") {
          toolCalls = event.toolCalls ?? []
          span.setAttribute("luna.tool_call.count", toolCalls.length)
          if (event.usage.status === "reported") {
            agentMetrics.modelTokens.add(event.usage.value.promptTokens, { direction: "input" })
            agentMetrics.modelTokens.add(event.usage.value.completionTokens, { direction: "output" })
          }
          const usage = event.reconciliationRequired
            ? { status: "reconciliation_required" as const, reason: event.usage.status === "unavailable" ? event.usage.reason : "hold_deficit" }
            : event.usage.status === "reported"
              ? {
                  status: "reported" as const,
                  promptTokens: event.usage.value.promptTokens,
                  completionTokens: event.usage.value.completionTokens,
                  totalTokens: event.usage.value.totalTokens,
                  ...(event.usage.value.cachedPromptTokens !== undefined ? { cachedPromptTokens: event.usage.value.cachedPromptTokens } : {}),
                  ...(event.usage.value.cacheWritePromptTokens !== undefined ? { cacheWritePromptTokens: event.usage.value.cacheWritePromptTokens } : {}),
                  ...(event.usage.value.reasoningCompletionTokens !== undefined ? { reasoningCompletionTokens: event.usage.value.reasoningCompletionTokens } : {}),
                }
              : event.usage
          await stream.appendEvent("model.completed", {
            usage,
            ...(input.model ? { modelId: input.model.id, maxContextTokensSnapshot: input.model.maxContextTokens } : {}),
            ...(event.creditHoldId ? { creditHoldId: event.creditHoldId } : {}),
            ...(event.providerRequestId ? { providerRequestId: event.providerRequestId } : {}),
            ...(event.responseId ? { responseId: event.responseId } : {}),
            ...(event.responseModel ? { responseModel: event.responseModel } : {}),
            ...(event.finishReason ? { finishReason: event.finishReason } : {}),
          })
        }
      }
      if (reasoningSummary) {
        await stream.updateItemWithEvent(
          reasoningItemId,
          "completed",
          redact({ summary: reasoningSummary, display: "summary" }),
          "thinking.completed",
          { itemId: reasoningItemId, display: "summary", timelineIndex: reasoningTimelineIndex },
        )
      }
      if (answer) {
        await stream.updateItemWithEvent(
          messageItemId,
          "completed",
          redact({ parts: [{ type: "text", text: answer }] }),
          "message.completed",
          { itemId: messageItemId, contentPartId: `${messageItemId}:0`, partIndex: 0, timelineIndex: messageTimelineIndex },
        )
      }
      agentMetrics.modelRequests.add(1, { operation: "stream", outcome })
      agentMetrics.modelSteps.add(1, { outcome })
      agentMetrics.modelDuration.record((performance.now() - startedAt) / 1000, { operation: "stream", outcome })
      telemetryLog("agent.model.completed", "info", { "luna.run.id": runId, "tool_call.count": toolCalls.length })
      const finalSession = toolCalls.length === 0 && answer.trim().length > 0 ? stream : undefined
      if (!finalSession) await stream.commit("completed")
      return {
        ...input, reasoningSummary, answer, toolCalls,
        ...(finalSession
          ? { finalizeTerminal: (to: "completed" | "failed" | "canceled" | "interrupted", errorCode?: string, conversationTitle?: string) => finalSession.commitTerminal(to, errorCode, conversationTitle) }
          : {}),
      }
    }
    catch (error) {
      if (stream) {
        throw new StreamModelFinalizationError(error, (to, errorCode, conversationTitle) => stream!.commitTerminal(to, errorCode, conversationTitle))
      }
      recordAIContent(span, "luna.gen_ai.content.error", "luna.gen_ai.response.error_body", contentError(error))
      throw error
    }
  }).catch((error: unknown) => {
    const sourceError = error instanceof StreamModelFinalizationError ? error.cause : error
    const canceled = isExpectedCancellation(sourceError)
    outcome = canceled ? "canceled" : stableErrorCode(sourceError)
    agentMetrics.modelRequests.add(1, { operation: "stream", outcome })
    agentMetrics.modelSteps.add(1, { outcome })
    agentMetrics.modelDuration.record((performance.now() - startedAt) / 1000, { operation: "stream", outcome })
    telemetryLog(canceled ? "agent.model.canceled" : "agent.model.failed", canceled ? "info" : "error", {
      "luna.run.id": runId,
	  "operation": "agent.model.stream",
	  "outcome": canceled ? "cancelled" : "failed",
      ...(canceled ? {} : errorDiagnostic(sourceError, outcome)),
    })
    throw error
  })
}
