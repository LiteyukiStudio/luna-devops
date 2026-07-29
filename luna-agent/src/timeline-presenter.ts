import type { Repository } from "./persistence/repository.js"
import type { RunEvent, TimelineItem } from "./domain.js"
import { normalizeEventSequence } from "./event-sequence.js"

export async function presentTimeline(repository: Repository, ownerUserId: string, conversationId: string) {
  const snapshot = await repository.getTimeline(ownerUserId, conversationId)
  if (!snapshot) return undefined
  const eventCursors = await Promise.all(snapshot.turns.flatMap(turn => turn.run ? [turn.run] : []).map(async run => {
    const events = await repository.getEvents(ownerUserId, run.id, 0)
    return { runId: run.id, after: events.at(-1) ? normalizeEventSequence(events.at(-1)!.sequence) : 0 }
  }))
  return {
    conversation: {
      id: snapshot.conversation.id,
      title: snapshot.conversation.title,
      status: snapshot.conversation.status,
    },
    turns: snapshot.turns.map(turn => ({
      id: turn.id,
      turnIndex: turn.turnIndex,
      status: turn.status,
      input: {
        id: `${turn.id}:input`,
        type: "user_message" as const,
        parts: [{ id: `${turn.id}:input:0`, partIndex: 0, type: "text" as const, text: turn.input }],
      },
      ...(turn.run ? {
        selectedRun: {
          id: turn.run.id,
          runIndex: turn.run.runIndex,
          status: turn.run.status === "expired" ? "failed" as const : turn.run.status,
          expectedVersion: turn.run.rowVersion,
          items: turn.items.filter(item => item.type !== "user_message").map(presentItem),
        },
      } : {}),
    })),
    eventCursors,
  }
}

export async function presentEvent(repository: Repository, ownerUserId: string, event: RunEvent) {
  const run = await repository.getRun(ownerUserId, event.runId)
  if (!run) return undefined
  return {
    version: 1 as const,
    eventId: event.id,
    eventSequence: normalizeEventSequence(event.sequence),
    type: event.type,
    conversationId: run.conversationId,
    turnId: run.turnId,
    runId: run.id,
    ...(stringValue(event.data.itemId) ? { itemId: stringValue(event.data.itemId) } : {}),
    ...(stringValue(event.data.contentPartId) ? { contentPartId: stringValue(event.data.contentPartId) } : {}),
    ...(stringValue(event.data.toolCallId) ? { toolCallId: stringValue(event.data.toolCallId) } : {}),
    occurredAt: event.createdAt,
    payload: event.data,
  }
}

function presentItem(item: TimelineItem) {
  const text = extractText(item)
  const base = {
    id: item.id,
    timelineIndex: item.timelineIndex,
    type: mapType(item.type),
    status: item.status,
    parts: text === undefined ? [] : [{ id: `${item.id}:0`, partIndex: 0, type: "text" as const, text }],
  }
  if (item.type !== "tool_call" && item.type !== "tool_result") {
    return {
      ...base,
      ...(item.type === "reasoning_summary" ? { display: item.content.display === "progress" ? "progress" as const : "summary" as const } : {}),
    }
  }
  const toolCallId = stringValue(item.content.toolCallId) ?? item.id
  const status = toolStatus(stringValue(item.content.status), item.status)
  const errorCode = stringValue(item.content.errorCode)
  return {
    ...base,
    toolCall: {
      id: toolCallId,
      operationId: stringValue(item.content.operationId) ?? "unknown",
      callIndex: item.timelineIndex,
      status,
      arguments: objectValue(item.content.arguments),
      ...(item.content.result !== undefined ? { result: presentToolResult(item.content.result) } : {}),
      ...(errorCode ? { titleKey: errorCode } : {}),
    },
  }
}

function mapType(type: TimelineItem["type"]) {
  if (type === "user_message") return "assistant_message" as const
  return type
}
function extractText(item: TimelineItem): string | undefined {
  if (item.type === "reasoning_summary") return stringValue(item.content.summary)
  const parts = Array.isArray(item.content.parts) ? item.content.parts : []
  return parts.map(part => objectValue(part)).map(part => stringValue(part.text)).filter((value): value is string => Boolean(value)).join("\n") || undefined
}
function presentToolResult(value: unknown) {
  const object = objectValue(value)
  return {
    summaryKey: stringValue(object.summaryKey) ?? "ai.tool.result.completed",
    ...(object.summaryParams && typeof object.summaryParams === "object" ? { summaryParams: object.summaryParams as Record<string, string | number | boolean> } : {}),
    presentation: { component: "key_value" as const, version: 1 as const },
  }
}
function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {}
}
function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined
}
function toolStatus(value: string | undefined, itemStatus: TimelineItem["status"]) {
  const allowed = ["proposed", "awaiting_approval", "awaiting_mfa", "running", "succeeded", "failed", "canceled", "skipped"] as const
  return allowed.find(status => status === value) ?? (itemStatus === "failed" ? "failed" as const : "succeeded" as const)
}
