import type {
  Conversation,
  ConversationHistoryEntry,
  ConversationSummary,
  ConversationToolInteraction,
  ConversationTitleSource,
  CreatedTurn,
  CreateTurn,
  AIModelSnapshot,
  ClaimedRun,
  Run,
  RunCheckpoint,
  RunCheckpointAdvance,
  RunExecutionSnapshot,
  RunEvent,
  TimelineItem,
  TimelineMutation,
  TimelinePage,
} from "../domain.js"
import type { OfficialModelUsage } from "../provider/provider.js"

export class RunStateConflictError extends Error {
  override readonly name = "RunStateConflictError"

  constructor(
    readonly runId: string,
    readonly expectedStatus: Run["status"],
    readonly targetStatus: Run["status"],
    readonly actualStatus?: Run["status"],
    readonly actualRowVersion?: number,
  ) {
    super("ai.run_state_conflict")
  }
}

/**
 * checkpoint 的 revision 乐观锁未命中：调用方要么在更旧的 revision 上推进，
 * 要么跳过了中间状态。同一 revision 的重放不算冲突。
 */
export class RunCheckpointConflictError extends Error {
  override readonly name = "RunCheckpointConflictError"

  constructor(
    readonly runId: string,
    readonly expectedRevision: number,
    readonly actualRevision?: number,
  ) {
    super("ai.run_checkpoint_conflict")
  }
}

/** 启动对账结果：requeued 表示可安全续跑，interrupted 表示无法续跑，skipped 表示 Run 已不是 running。 */
export type OrphanedRunRecovery = "requeued" | "interrupted" | "skipped"
export type RunLeaseRenewalOutcome = "renewed" | "canceled" | "lost"
export type InterruptRunOptions = { ignoreActiveLease?: boolean }

export type ConversationListOptions = {
  search?: string
  sortOrder?: "asc" | "desc"
}

export type TimelinePageOptions = {
  beforeTurnIndex?: number
  limit?: number
}

export type ModelCallOperation = "assistant" | "summary" | "title"
export type ModelAttemptMetadata = {
  providerRequestId?: string
  responseId?: string
  responseModel?: string
  finishReason?: string
}
export type RecordReportedModelUsageInput = ModelAttemptMetadata & {
  runId: string
  ownerUserId: string
  operation: ModelCallOperation
  usage: OfficialModelUsage
  callType: "stream" | "complete"
  model?: AIModelSnapshot
}
export type RepositoryReadiness = { database: boolean, schema: boolean }
export type RunToolSelection = {
  selectedOperationIds: string[]
  alreadySelectedOperationIds: string[]
  evictedOperationIds: string[]
}

/**
 * 淘汰策略：当前 Run 内已经实际调用过的平台工具（executedInRun）优先保留，
 * 仅对未实际调用的最早闲置工具按 FIFO/LRU 淘汰；只有当已调用工具数本身超过上限时才淘汰最旧项。
 */
export function selectOperationsWithExecutedProtection(
  current: readonly string[],
  requested: readonly string[],
  executedInRun: ReadonlySet<string>,
  limit: number,
): RunToolSelection {
  const boundedLimit = Math.max(1, Math.min(64, Math.floor(limit)))
  const alreadySelectedOperationIds = requested.filter(operationId => current.includes(operationId))
  const touched = new Set(requested)
  const reordered = [...current.filter(operationId => !touched.has(operationId)), ...requested]
  if (reordered.length <= boundedLimit) {
    return {
      selectedOperationIds: reordered,
      alreadySelectedOperationIds,
      evictedOperationIds: [],
    }
  }
  const excess = reordered.length - boundedLimit
  const evicted = new Set<string>()
  for (const operationId of reordered) {
    if (evicted.size >= excess) break
    if (!executedInRun.has(operationId)) {
      evicted.add(operationId)
    }
  }
  if (evicted.size < excess) {
    for (const operationId of reordered) {
      if (evicted.size >= excess) break
      evicted.add(operationId)
    }
  }
  return {
    selectedOperationIds: reordered.filter(operationId => !evicted.has(operationId)),
    alreadySelectedOperationIds,
    evictedOperationIds: reordered.filter(operationId => evicted.has(operationId)),
  }
}
export type RunStreamPosition = { nextItemPosition: number, nextEventSequence: number, runVersion: number }
export type RunStreamBatch = {
  runId: string
  expected: RunStreamPosition
  items: TimelineItem[]
  events: RunEvent[]
  /** In-memory stream high-watermark; transient deltas below it are intentionally not durable. */
  eventHighWatermark: number
  /** Run-version fence. A superseded execution must never persist late output. */
  expectedRunVersion: number
  terminal?: {
    from: Run["status"]
    to: Extract<Run["status"], "completed" | "failed" | "canceled" | "interrupted">
    completedAt: string
    errorCode?: string
    conversationTitle?: string
  }
}

export type UpdateRunOptions = Partial<Run> & {
  clearWakeAt?: boolean
  expectedRowVersion?: number
  eventData?: Record<string, unknown>
}

export type ResolveWaitingExternalRunInput = {
  runId: string
  expectedCheckpointRevision: number
  nextPhase: RunCheckpoint["phase"]
  nextCheckpointState: RunCheckpoint["state"]
  itemId: string
  toolCallId: string
  itemStatus: Exclude<TimelineItem["status"], "streaming">
  itemContent: Record<string, unknown>
  eventType: "tool.completed" | "tool.failed"
  eventData: Record<string, unknown>
}

export interface Repository {
  health(): Promise<boolean>
  readiness(): Promise<RepositoryReadiness>
  createConversation(ownerUserId: string, title: string, projectId?: string, titleSource?: ConversationTitleSource, modelId?: string): Promise<Conversation>
  findEmptyConversation(ownerUserId: string, projectId?: string): Promise<Conversation | undefined>
  listConversations(ownerUserId: string, page: number, pageSize: number, options?: ConversationListOptions): Promise<{ items: Conversation[], total: number }>
  getConversation(ownerUserId: string, conversationId: string): Promise<Conversation | undefined>
  updateConversation(ownerUserId: string, conversationId: string, input: { title?: string, modelId?: string }): Promise<Conversation | undefined>
  renameConversation(ownerUserId: string, conversationId: string, title: string): Promise<Conversation | undefined>
  renameConversationByAssistant(conversationId: string, title: string, runId?: string): Promise<Conversation | undefined>
  listActiveConversationRuns(ownerUserId: string, conversationId: string): Promise<Run[]>
  deleteConversation(ownerUserId: string, conversationId: string): Promise<boolean>
  assertPositiveAvailableBalance(ownerUserId: string): Promise<void>
  createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn>
  getRun(ownerUserId: string, runId: string): Promise<Run | undefined>
  assertRunningRunVersion(runId: string, expectedRunVersion: number): Promise<void>
  getRunToolState(runId: string): Promise<Pick<Run, "toolCatalogDigest" | "selectedOperationIds"> | undefined>
  getRunStreamPosition(runId: string): Promise<RunStreamPosition | undefined>
  persistRunStreamBatch(batch: RunStreamBatch): Promise<void>
  touchRunSelectedOperations(
    runId: string,
    operationIds: string[],
    limit: number,
    protectedOperationIds?: ReadonlySet<string>,
  ): Promise<RunToolSelection>
  listActiveToolCatalogDigests(): Promise<string[]>
  cancelRun(ownerUserId: string, runId: string, expectedRowVersion?: number): Promise<Run | undefined>
  claimNextQueuedRun(executionSnapshot?: RunExecutionSnapshot, userConcurrentRuns?: number): Promise<ClaimedRun | undefined>
  renewRunLease(runId: string, expectedRowVersion: number, leaseMs: number): Promise<RunLeaseRenewalOutcome>
  updateRunToolCatalogDigest(runId: string, toolCatalogDigest: string, expectedRowVersion?: number): Promise<void>
  claimDueWaitingExternalRuns(limit: number, leaseMs: number, now?: Date): Promise<Run[]>
  rescheduleWaitingExternalRun(runId: string, expectedCheckpointRevision: number, pollCount: number, wakeAt: string): Promise<boolean>
  resolveWaitingExternalRun(input: ResolveWaitingExternalRunInput): Promise<boolean>
  listRunningRuns(now?: Date): Promise<Array<Pick<Run, "id" | "rowVersion">>>
  interruptOrphanedRun(runId: string, expectedRunVersion: number, options?: InterruptRunOptions): Promise<boolean>
  recoverOrphanedRun(runId: string, expectedRunVersion: number, maxRecoveries: number): Promise<OrphanedRunRecovery>
  loadRunCheckpoint(runId: string): Promise<RunCheckpoint | undefined>
  saveRunCheckpoint(runId: string, advance: RunCheckpointAdvance): Promise<RunCheckpoint>
  countActiveUserRuns(userId: string): Promise<number>
  getExecutionInput(runId: string): Promise<{
    conversationId: string
    turnId: string
    turnIndex: number
    input: string
    pageContext: Record<string, unknown>
    toolCatalogDigest: string
    selectedOperationIds: string[]
    toolInteractions: ConversationToolInteraction[]
    history: ConversationHistoryEntry[]
    conversation: Pick<Conversation, "title" | "titleSource">
    lastTotalTokens?: number
    model?: AIModelSnapshot
  } | undefined>
  recordReportedModelUsage(input: RecordReportedModelUsageInput): Promise<{ id: string, attempt: number }>
  getConversationSummary(conversationId: string): Promise<ConversationSummary | undefined>
  saveConversationSummary(summary: Omit<ConversationSummary, "createdAt" | "updatedAt">): Promise<ConversationSummary>
  listConversationHistory(conversationId: string, afterTurnIndex: number, beforeTurnIndex: number, limit: number): Promise<ConversationHistoryEntry[]>
  appendRunInput(runId: string, text: string): Promise<TimelineItem>
  updateRun(runId: string, from: Run["status"], to: Run["status"], fields?: UpdateRunOptions): Promise<Run>
  appendItem(item: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }): Promise<TimelineItem>
  updateItem(itemId: string, status: TimelineItem["status"], content: Record<string, unknown>): Promise<TimelineItem>
  appendItemWithEvent(item: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string }, eventType: string, eventData?: Record<string, unknown>): Promise<TimelineMutation>
  updateItemWithEvent(itemId: string, status: TimelineItem["status"], content: Record<string, unknown>, eventType: string, eventData?: Record<string, unknown>): Promise<TimelineMutation>
  completeToolItemWithEvent(
    itemId: string,
    status: TimelineItem["status"],
    content: Record<string, unknown>,
    resultItem: Omit<TimelineItem, "id" | "timelineIndex" | "revision" | "createdAt"> & { id?: string },
    eventType: string,
    eventData?: Record<string, unknown>,
  ): Promise<TimelineMutation & { resultItem: TimelineItem }>
  finalizeStreamingItems(runId: string, status: Exclude<TimelineItem["status"], "streaming">, expectedRowVersion?: number): Promise<void>
  appendEvent(runId: string, type: string, data: Record<string, unknown>): Promise<RunEvent>
  getEvents(ownerUserId: string, runId: string, after: number): Promise<RunEvent[]>
  getTimeline(ownerUserId: string, conversationId: string, options?: TimelinePageOptions): Promise<TimelinePage | undefined>
}
