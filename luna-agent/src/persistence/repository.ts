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
  RunExecutionSnapshot,
  RunEvent,
  TimelineItem,
  TimelineMutation,
  TimelinePage,
} from "../domain.js"
import type { OfficialModelUsage, UsageUnavailableReason } from "../provider/provider.js"

export class RunStateConflictError extends Error {
  override readonly name = "RunStateConflictError"

  constructor(
    readonly runId: string,
    readonly expectedStatus: Run["status"],
    readonly targetStatus: Run["status"],
    readonly actualStatus?: Run["status"],
  ) {
    super("ai.run_state_conflict")
  }
}

export type ConversationListOptions = {
  search?: string
  sortOrder?: "asc" | "desc"
}

export type TimelinePageOptions = {
  beforeTurnIndex?: number
  limit?: number
}

export type ModelCallOperation = "assistant" | "summary" | "title"
export type ModelCreditHold = { id: string, attempt: number, maxOutputTokens: number }
export type ModelAttemptMetadata = {
  providerRequestId?: string
  responseId?: string
  responseModel?: string
  finishReason?: string
  failureStage?: string
}
export type RepositoryReadiness = { database: boolean, schema: boolean }
export type RunToolSelection = {
  selectedOperationIds: string[]
  alreadySelectedOperationIds: string[]
  evictedOperationIds: string[]
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
  deleteConversation(ownerUserId: string, conversationId: string): Promise<boolean>
  createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn>
  getRun(ownerUserId: string, runId: string): Promise<Run | undefined>
  getRunToolState(runId: string): Promise<Pick<Run, "toolCatalogDigest" | "selectedOperationIds"> | undefined>
  getRunStreamPosition(runId: string): Promise<RunStreamPosition | undefined>
  persistRunStreamBatch(batch: RunStreamBatch): Promise<void>
  touchRunSelectedOperations(runId: string, operationIds: string[], limit: number): Promise<RunToolSelection>
  listActiveToolCatalogDigests(): Promise<string[]>
  cancelRun(ownerUserId: string, runId: string): Promise<Run | undefined>
  claimNextQueuedRun(executionSnapshot?: RunExecutionSnapshot): Promise<ClaimedRun | undefined>
  listRunningRuns(): Promise<Array<Pick<Run, "id" | "rowVersion">>>
  interruptOrphanedRun(runId: string, expectedRunVersion: number): Promise<boolean>
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
    model?: AIModelSnapshot
  } | undefined>
  createModelCreditHold(input: {
    id: string
    runId: string
    ownerUserId: string
    operation: ModelCallOperation
    requestedOutputTokens: number
    leaseSeconds: number
  }): Promise<ModelCreditHold>
  recordReportedModelUsage(
    creditHoldId: string,
    usage: OfficialModelUsage,
    metadata: ModelAttemptMetadata & { callType: "stream" | "complete" },
  ): Promise<{ reconciliationRequired: boolean }>
  markModelUsageUnavailable(
    creditHoldId: string,
    reason: UsageUnavailableReason | "request_outcome_unknown",
    metadata: ModelAttemptMetadata,
  ): Promise<void>
  releaseModelCreditHold(creditHoldId: string): Promise<void>
  getLatestReportedModelUsage(conversationId: string): Promise<{
    modelId: string
    promptTokens: number
    maxContextTokensSnapshot: number
  } | undefined>
  getConversationSummary(conversationId: string): Promise<ConversationSummary | undefined>
  saveConversationSummary(summary: Omit<ConversationSummary, "createdAt" | "updatedAt">): Promise<ConversationSummary>
  listConversationHistory(conversationId: string, afterTurnIndex: number, beforeTurnIndex: number, limit: number): Promise<ConversationHistoryEntry[]>
  appendRunInput(runId: string, text: string): Promise<void>
  updateRun(runId: string, from: Run["status"], to: Run["status"], fields?: Partial<Run>): Promise<Run>
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
  finalizeStreamingItems(runId: string, status: Exclude<TimelineItem["status"], "streaming">): Promise<void>
  appendEvent(runId: string, type: string, data: Record<string, unknown>): Promise<RunEvent>
  getEvents(ownerUserId: string, runId: string, after: number): Promise<RunEvent[]>
  getTimeline(ownerUserId: string, conversationId: string, options?: TimelinePageOptions): Promise<TimelinePage | undefined>
}
