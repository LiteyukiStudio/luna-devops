import type {
  Conversation,
  ConversationSummary,
  Run,
  RunEvent,
  TimelineItem,
  UIActionDelivery,
} from "../../domain.js"
import { normalizeEventSequence } from "../../event-sequence.js"
import type {
  ConversationRow,
  ConversationSummaryRow,
  RunEventRow,
  RunRow,
  TimelineItemRow,
  UIActionRow,
} from "../schema/index.js"

/** 行到领域模型的统一映射，禁止在 Repository 各方法中散落转换逻辑 */

export function mapConversation(row: ConversationRow): Conversation {
  return {
    id: row.id,
    ownerUserId: row.ownerUserId,
    title: row.title,
    titleSource: row.titleSource,
    status: row.status,
    createdAt: row.createdAt.toISOString(),
    updatedAt: row.updatedAt.toISOString(),
    ...(row.projectId ? { projectId: row.projectId } : {}),
  }
}

export function mapConversationSummary(row: ConversationSummaryRow): ConversationSummary {
  return {
    conversationId: row.conversationId,
    coveredThroughTurnIndex: row.coveredThroughTurnIndex,
    compressionVersion: row.compressionVersion,
    sourceTurnCount: row.sourceTurnCount,
    content: row.content,
    createdAt: row.createdAt.toISOString(),
    updatedAt: row.updatedAt.toISOString(),
  }
}

export function mapRun(row: RunRow): Run {
  return {
    id: row.id,
    conversationId: row.conversationId,
    turnId: row.turnId,
    runIndex: row.runIndex,
    status: row.status,
    rowVersion: row.rowVersion,
    graphVersion: row.graphVersion,
    promptVersion: row.promptVersion,
    toolCatalogDigest: row.toolCatalogDigest,
    pageContext: row.pageContext,
    ...(Object.keys(row.traceContext ?? {}).length ? { traceContext: row.traceContext } : {}),
    createdAt: row.createdAt.toISOString(),
    ...(row.clientInstanceId ? { clientInstanceId: row.clientInstanceId } : {}),
    ...(row.startedAt ? { startedAt: row.startedAt.toISOString() } : {}),
    ...(row.completedAt ? { completedAt: row.completedAt.toISOString() } : {}),
    ...(row.errorCode ? { errorCode: row.errorCode } : {}),
  }
}

export function mapTimelineItem(row: TimelineItemRow): TimelineItem {
  return {
    id: row.id,
    runId: row.runId,
    turnId: row.turnId,
    timelineIndex: row.timelineIndex,
    revision: row.revision,
    type: row.type,
    status: row.status,
    content: row.content,
    createdAt: row.createdAt.toISOString(),
  }
}

export function mapRunEvent(row: RunEventRow): RunEvent {
  return {
    id: row.id,
    runId: row.runId,
    sequence: normalizeEventSequence(row.eventSequence),
    type: row.type,
    data: row.data,
    createdAt: row.createdAt.toISOString(),
  }
}

export function mapUIAction(row: UIActionRow): UIActionDelivery {
  return {
    id: row.id,
    runId: row.runId,
    toolCallId: row.toolCallId,
    clientInstanceId: row.clientInstanceId,
    action: row.action,
    status: row.status,
    attempts: row.attempts,
    expiresAt: row.expiresAt.toISOString(),
    ...(row.acknowledgedAt ? { acknowledgedAt: row.acknowledgedAt.toISOString() } : {}),
    ...(row.actualPath ? { actualPath: row.actualPath } : {}),
    ...(row.errorCode ? { errorCode: row.errorCode } : {}),
    createdAt: row.createdAt.toISOString(),
    updatedAt: row.updatedAt.toISOString(),
  }
}

export function timelineContentText(content: Record<string, unknown>): string {
  if (!Array.isArray(content.parts)) return ""
  return content.parts.map((part) => {
    if (!part || typeof part !== "object") return ""
    return typeof (part as Record<string, unknown>).text === "string"
      ? String((part as Record<string, unknown>).text)
      : ""
  }).join("")
}
