import type {
  ClaimedRun,
  Conversation,
  ConversationSummary,
  Run,
  RunExecutionSnapshot,
  RunEvent,
  TimelineItem,
} from "../../domain.js"
import { normalizeEventSequence } from "../../event-sequence.js"
import type {
  ConversationRow,
  ConversationSummaryRow,
  RunEventRow,
  RunRow,
  TimelineItemRow,
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
    ...(row.modelId ? { modelId: row.modelId } : {}),
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
    ownerUserId: row.ownerUserId,
    conversationId: row.conversationId,
    turnId: row.turnId,
    runIndex: row.runIndex,
    status: row.status,
    rowVersion: row.rowVersion,
    promptVersion: row.promptVersion,
    toolCatalogDigest: row.toolCatalogDigest,
    selectedOperationIds: row.selectedOperationIds,
    pageContext: row.pageContext,
    ...(Object.keys(row.traceContext ?? {}).length ? { traceContext: row.traceContext } : {}),
    createdAt: row.createdAt.toISOString(),
    ...(row.startedAt ? { startedAt: row.startedAt.toISOString() } : {}),
    ...(row.completedAt ? { completedAt: row.completedAt.toISOString() } : {}),
    ...(row.errorCode ? { errorCode: row.errorCode } : {}),
    ...(row.modelId && row.modelName ? {
      model: {
        id: row.modelId,
        name: row.modelName,
        maxContextTokens: row.maxContextTokens ?? 524_288,
        maxOutputTokens: row.maxOutputTokens ?? 65_536,
        inputCreditsPerMillion: row.inputCreditsPerMillion ?? "0",
        outputCreditsPerMillion: row.outputCreditsPerMillion ?? "0",
        cachedInputCreditsPerMillion: row.cachedInputCreditsPerMillion ?? "0",
      },
    } : {}),
  }
}

export function mapClaimedRun(row: RunRow, executionSnapshot?: RunExecutionSnapshot): ClaimedRun {
  return {
    ...mapRun(row),
    ...(executionSnapshot ? { executionSnapshot } : {}),
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

export function timelineContentText(content: Record<string, unknown>): string {
  if (!Array.isArray(content.parts)) return ""
  return content.parts.map((part) => {
    if (!part || typeof part !== "object") return ""
    return typeof (part as Record<string, unknown>).text === "string"
      ? String((part as Record<string, unknown>).text)
      : ""
  }).join("")
}
