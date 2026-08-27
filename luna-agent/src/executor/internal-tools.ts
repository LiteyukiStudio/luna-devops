import { createId } from "../id.js"
import { trace } from "@opentelemetry/api"
import type { Repository } from "../persistence/repository.js"
import { redact } from "../redaction.js"
import { renameConversationInput } from "../tools/conversation-title.js"
import { navigateToRouteInput, routeUIAction } from "../tools/ui-route.js"

// 内部工具的副作用处理：这些工具不调用平台 API，
// 只向时间线或会话写入记录。
export class InternalToolHandlers {
  constructor(private readonly repository: Repository) {}

  async recordToolCall(
    runId: string,
    turnId: string,
    operationId: string,
    rawArguments: unknown,
    status: "succeeded" | "failed",
    result?: unknown,
    errorCode?: string,
    toolCallId = createId("aitool"),
    durationMs?: number,
  ) {
    const itemId = createId("aiitm")
    await this.repository.appendItemWithEvent({
      id: itemId,
      runId,
      turnId,
      type: "tool_call",
      status: status === "succeeded" ? "completed" : "failed",
      content: {
        toolCallId,
        operationId,
        status,
        arguments: redact(rawArguments),
        ...(result !== undefined ? { result: redact(result) } : {}),
        ...(errorCode ? { errorCode } : {}),
        ...(typeof durationMs === "number" && Number.isFinite(durationMs) ? { durationMs: Math.max(0, Math.round(durationMs)) } : {}),
        ...(activeTraceId() ? { traceId: activeTraceId() } : {}),
      },
    }, status === "succeeded" ? "tool.completed" : "tool.failed", {
      itemId,
      toolCallId,
      operationId,
      status,
      ...(errorCode ? { errorCode } : {}),
    })
  }

  async navigateToRoute(runId: string, turnId: string, raw: unknown) {
    const input = navigateToRouteInput.parse(raw)
    const itemId = createId("aiitm")
    const toolCallId = createId("aitool")
    const uiActions = [routeUIAction(input)]
    const result = {
      summaryKey: "aiAssistant.tools.navigateToRouteReady",
      uiActions,
    }
    await this.repository.appendItemWithEvent({
      id: itemId, runId, turnId, type: "tool_call", status: "completed",
      content: {
        toolCallId,
        operationId: "navigate_to_route",
        titleKey: "aiAssistant.tools.navigateToRoute",
        status: "succeeded",
        arguments: input,
        result,
      },
    }, "tool.completed", {
      itemId, toolCallId, operationId: "navigate_to_route", titleKey: "aiAssistant.tools.navigateToRoute",
      result, uiActions,
    })
    return result
  }

  async renameConversation(runId: string, turnId: string, conversationId: string, raw: unknown) {
    const input = renameConversationInput.parse(raw)
    const itemId = createId("aiitm")
    const toolCallId = createId("aitool")
    const renamed = await this.repository.renameConversationByAssistant(conversationId, input.title, runId)
    const status = renamed ? "succeeded" : "skipped"
    const result = {
      summaryKey: renamed
        ? "aiAssistant.tools.renameConversationCompleted"
        : "aiAssistant.tools.renameConversationLocked",
      title: input.title,
    }
    await this.repository.appendItemWithEvent({
      id: itemId, runId, turnId, type: "tool_call", status: "completed",
      content: {
        toolCallId,
        operationId: "rename_conversation",
        titleKey: "aiAssistant.tools.renameConversation",
        status,
        arguments: input,
        result,
      },
    }, "tool.completed", {
      itemId, toolCallId, operationId: "rename_conversation", titleKey: "aiAssistant.tools.renameConversation",
      status, result,
    })
    return renamed
  }
}

function activeTraceId(): string | undefined {
  const traceId = trace.getActiveSpan()?.spanContext().traceId
  return traceId && /^(?!0{32}$)[a-f0-9]{32}$/i.test(traceId) ? traceId : undefined
}
