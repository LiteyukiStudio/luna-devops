import type { Repository } from "./persistence/repository.js"
import type { RunEvent, TimelineItem } from "./domain.js"
import { normalizeEventSequence } from "./event-sequence.js"
import type { optionUIActions } from "./tools/ui-options.js"

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
      titleSource: snapshot.conversation.titleSource,
      status: snapshot.conversation.status,
    },
    turns: snapshot.turns.map(turn => ({
      id: turn.id,
      turnIndex: turn.turnIndex,
      status: turn.status,
      input: {
        id: `${turn.id}:input`,
        type: "user_message" as const,
        createdAt: turn.createdAt,
        parts: [{ id: `${turn.id}:input:0`, partIndex: 0, type: "text" as const, text: turn.input }],
      },
      ...(turn.run ? {
        selectedRun: {
          id: turn.run.id,
          runIndex: turn.run.runIndex,
          status: turn.run.status === "expired" ? "failed" as const : turn.run.status,
          expectedVersion: turn.run.rowVersion,
          ...(turn.run.errorCode ? { errorCode: turn.run.errorCode } : {}),
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
  const eventItem = timelineItemValue(event.data.item)
  const payload = presentEventPayload(Object.fromEntries(Object.entries(event.data).filter(([key]) => key !== "item")))
  return {
    version: 2 as const,
    eventId: event.id,
    eventSequence: normalizeEventSequence(event.sequence),
    type: event.type,
    conversationId: run.conversationId,
    turnId: run.turnId,
    runId: run.id,
    ...(stringValue(payload.itemId) ? { itemId: stringValue(payload.itemId) } : {}),
    ...(stringValue(payload.contentPartId) ? { contentPartId: stringValue(payload.contentPartId) } : {}),
    ...(stringValue(payload.toolCallId) ? { toolCallId: stringValue(payload.toolCallId) } : {}),
    ...(eventItem ? { item: presentItem(eventItem) } : {}),
    occurredAt: event.createdAt,
    payload,
  }
}

function presentEventPayload(data: Record<string, unknown>) {
  if (data.result === undefined)
    return data
  return {
    ...data,
    result: presentToolResult(data.result, stringValue(data.errorCode)),
  }
}

function presentItem(item: TimelineItem) {
  const text = extractText(item)
  const base = {
    id: item.id,
    timelineIndex: item.timelineIndex,
    revision: item.revision,
    type: mapType(item.type),
    status: item.status,
    createdAt: item.createdAt,
    parts: text === undefined ? [] : [{ id: `${item.id}:0`, partIndex: 0, type: "text" as const, text }],
    ...(stringValue(item.content.relatedItemId) ? { relatedItemId: stringValue(item.content.relatedItemId) } : {}),
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
  const titleKey = stringValue(item.content.titleKey)
  const argumentsHash = stringValue(item.content.argumentsHash)
  const mfaPurpose = stringValue(item.content.mfaPurpose)
  const traceId = stringValue(item.content.traceId)
  return {
    ...base,
    toolCall: {
      id: toolCallId,
      operationId: stringValue(item.content.operationId) ?? "unknown",
      callIndex: item.timelineIndex,
      status,
      arguments: objectValue(item.content.arguments),
      ...(item.content.result !== undefined ? { result: presentToolResult(item.content.result, errorCode) } : {}),
      ...(Array.isArray(objectValue(item.content.result).uiActions)
        ? { uiActions: objectValue(item.content.result).uiActions as ReturnType<typeof optionUIActions> }
        : {}),
      ...(argumentsHash ? { argumentsHash } : {}),
      ...(typeof item.content.expectedVersion === "number" ? { expectedVersion: item.content.expectedVersion } : {}),
      ...(mfaPurpose ? { mfaPurpose } : {}),
      ...(typeof item.content.durationMs === "number" ? { durationMs: item.content.durationMs } : {}),
      ...(traceId ? { traceId } : {}),
      ...(titleKey ? { titleKey } : {}),
      ...(errorCode ? { errorCode } : {}),
    },
  }
}

function timelineItemValue(value: unknown): TimelineItem | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined
  const item = value as Partial<TimelineItem>
  return typeof item.id === "string"
    && typeof item.runId === "string"
    && typeof item.turnId === "string"
    && typeof item.timelineIndex === "number"
    && typeof item.revision === "number"
    && typeof item.type === "string"
    && typeof item.status === "string"
    && Boolean(item.content)
    && typeof item.createdAt === "string"
    ? item as TimelineItem
    : undefined
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
function presentToolResult(value: unknown, errorCode?: string) {
  const object = objectValue(value)
  const data = toolDisplayData(object)
  const issues = toolDisplayIssues(object.issues)
  const displayErrorCode = errorCode ?? stringValue(object.code)
  return {
    summaryKey: stringValue(object.summaryKey) ?? "ai.tool.result.completed",
    ...(object.summaryParams && typeof object.summaryParams === "object" ? { summaryParams: object.summaryParams as Record<string, string | number | boolean> } : {}),
    ...(stringValue(object.requestId) ? { requestId: stringValue(object.requestId) } : {}),
    ...(displayErrorCode ? { errorCode: displayErrorCode } : {}),
    ...(stringValue(object.error) ? { errorMessage: stringValue(object.error) } : {}),
    ...(data !== undefined ? { data } : {}),
    ...(issues.length ? { issues } : {}),
    ...(stringValue(object.title) ? { fields: [
      { labelKey: "aiAssistant.options.title", value: stringValue(object.title)! },
      ...(stringValue(object.description) ? [{ labelKey: "aiAssistant.options.description", value: stringValue(object.description)! }] : []),
    ] } : {}),
    presentation: { component: "key_value" as const, version: 1 as const },
  }
}

const sensitiveDisplayKey = /authorization|cookie|password|secret|token|credential|api[-_]?key|kubeconfig/i
const resultMetadataKeys = new Set(["summaryKey", "summaryParams", "requestId", "code", "error", "detail", "issues", "title", "description", "uiActions"])

function toolDisplayData(object: Record<string, unknown>): unknown {
  const candidate = object.result !== undefined
    ? object.result
    : Object.fromEntries(Object.entries(object).filter(([key]) => !resultMetadataKeys.has(key)))
  if (candidate && typeof candidate === "object" && !Array.isArray(candidate) && Object.keys(candidate).length === 0)
    return undefined
  return sanitizeDisplayValue(candidate, 0)
}

function sanitizeDisplayValue(value: unknown, depth: number): unknown {
  if (value === null || typeof value === "number" || typeof value === "boolean") return value
  if (typeof value === "string") return value.slice(0, 2_000)
  if (depth >= 6) return "[TRUNCATED]"
  if (Array.isArray(value)) return value.slice(0, 50).map(item => sanitizeDisplayValue(item, depth + 1))
  if (!value || typeof value !== "object") return undefined
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .filter(([key]) => !sensitiveDisplayKey.test(key))
      .slice(0, 50)
      .map(([key, item]) => [key, sanitizeDisplayValue(item, depth + 1)]),
  )
}

function toolDisplayIssues(value: unknown) {
  if (!Array.isArray(value)) return []
  return value.slice(0, 20).map((issue) => {
    const object = objectValue(issue)
    return {
      code: stringValue(object.code) ?? "invalid",
      path: stringValue(object.path) ?? "",
      message: (stringValue(object.message) ?? "Invalid value").slice(0, 500),
    }
  })
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
