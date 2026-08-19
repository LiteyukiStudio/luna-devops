import type { AssistantModelInput, ModelRuntime } from "../model-runtime.js"
import type { Repository } from "../persistence/repository.js"
import type { ModelToolCall } from "../provider/provider.js"
import { createId } from "../id.js"
import { redact } from "../redaction.js"
import { agentMetrics, internalSpanOptions, isExpectedCancellation, recordAIContent, stableErrorCode, telemetryLog, withSpan } from "../telemetry.js"
import { contentError } from "./tool-results.js"

// 单次模型流式调用：消费 provider 事件流，把 reasoning/正文/tool call
// 投影到时间线，并记录首 token、用量与终态遥测。
export async function streamModel(
  repository: Repository,
  modelRuntime: ModelRuntime,
  runId: string,
  turnId: string,
  input: AssistantModelInput,
  signal: AbortSignal,
): Promise<AssistantModelInput> {
  const startedAt = performance.now()
  let outcome = "success"
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
      await repository.appendEvent(runId, "model.started", {})
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
          await repository.appendItemWithEvent({
            id: createId("aiitm"), runId, turnId, type: "system_notice", status: "completed",
            content: redact({
              notice: "context_compacted",
              summarizedThroughTurnIndex: event.summarizedThroughTurnIndex,
              estimatedInputTokens: event.estimatedInputTokens,
            }),
          }, "context.compacted", redact({
            summarizedThroughTurnIndex: event.summarizedThroughTurnIndex,
            estimatedInputTokens: event.estimatedInputTokens,
          }))
          continue
        }
        if (event.type === "reasoning_summary_delta" && event.delta) {
          reasoningSummary += event.delta
          if (reasoningTimelineIndex === undefined) {
            const { item } = await repository.appendItemWithEvent({
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
            await repository.updateItemWithEvent(
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
            const { item } = await repository.appendItemWithEvent({
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
            await repository.updateItemWithEvent(
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
          agentMetrics.modelTokens.add(event.usage.inputTokens, { direction: "input" })
          agentMetrics.modelTokens.add(event.usage.outputTokens, { direction: "output" })
          await repository.appendEvent(runId, "model.completed", {
            usage: {
              inputTokens: event.usage.inputTokens,
              outputTokens: event.usage.outputTokens,
              cachedInputTokens: event.usage.cachedInputTokens ?? 0,
              cachedOutputTokens: event.usage.cachedOutputTokens ?? 0,
            },
            ...(event.reservationId ? { reservationId: event.reservationId } : {}),
          })
        }
      }
      if (reasoningSummary) {
        await repository.updateItemWithEvent(
          reasoningItemId,
          "completed",
          redact({ summary: reasoningSummary, display: "summary" }),
          "thinking.completed",
          { itemId: reasoningItemId, display: "summary", timelineIndex: reasoningTimelineIndex },
        )
      }
      if (answer) {
        await repository.updateItemWithEvent(
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
      return { ...input, reasoningSummary, answer, toolCalls }
    }
    catch (error) {
      recordAIContent(span, "luna.gen_ai.content.error", "luna.gen_ai.response.error_body", contentError(error))
      throw error
    }
  }).catch((error: unknown) => {
    const canceled = isExpectedCancellation(error)
    outcome = canceled ? "canceled" : stableErrorCode(error)
    agentMetrics.modelRequests.add(1, { operation: "stream", outcome })
    agentMetrics.modelSteps.add(1, { outcome })
    agentMetrics.modelDuration.record((performance.now() - startedAt) / 1000, { operation: "stream", outcome })
    telemetryLog(canceled ? "agent.model.canceled" : "agent.model.failed", canceled ? "info" : "error", {
      "luna.run.id": runId,
      "error.type": error instanceof Error ? error.name : "UnknownError",
      ...(canceled ? {} : { "error.code": outcome }),
    })
    throw error
  })
}

