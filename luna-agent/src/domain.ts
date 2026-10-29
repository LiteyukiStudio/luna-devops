import type { RemoteProviderConfig } from "./provider/config-client.js"
import type { RuntimeSettings } from "./runtime-settings.js"

export const runStatuses = [
	"queued", "running", "waiting_approval", "waiting_input",
  "completed", "failed", "canceled", "expired", "interrupted",
] as const
export type RunStatus = typeof runStatuses[number]

export type ActorContext = {
  userId: string
  sessionId: string
  projectId?: string
  locale: string
  issuedAt: number
  expiresAt: number
  requestId: string
  runId?: string
}

export type ConversationTitleSource = "default" | "assistant" | "user"
export type PromptVersion = "system-v4"

export type AIModelSnapshot = {
  id: string
  name: string
  maxContextTokens: number
  maxOutputTokens: number
  inputCreditsPerMillion: string
  outputCreditsPerMillion: string
  cachedInputCreditsPerMillion: string
}

export type Conversation = {
  id: string
  ownerUserId: string
  projectId?: string
  modelId?: string
  title: string
  titleSource: ConversationTitleSource
  status: "active"
  createdAt: string
  updatedAt: string
}

export type Turn = {
  id: string
  conversationId: string
  turnIndex: number
  status: RunStatus
  input: string
  selectedRunId: string
  modelId?: string
  createdAt: string
}

export type ConversationHistoryEntry = {
  turnIndex: number
  user: string
  assistant: string
  pageContext?: Record<string, unknown>
  toolInteractions?: Array<Record<string, unknown>>
}

export type ConversationToolInteraction = {
  itemId: string
  type: "tool_call" | "tool_result"
  status: TimelineItem["status"]
  content: Record<string, unknown>
}

export const conversationSummaryVersion = 1 as const

export type ConversationSummaryContent = {
  userGoals: string[]
  constraints: string[]
  confirmedResources: Array<{ type: string, name?: string, id?: string }>
  completedActions: string[]
  failures: string[]
  pendingWork: string[]
  durableFacts: string[]
  /** 原样保留的最近助手回复原文（不被摘要改写），用于保持模型自我一致性。 */
  recentAssistantMessages?: string[]
}

export type ConversationSummary = {
  conversationId: string
  coveredThroughTurnIndex: number
  compressionVersion: typeof conversationSummaryVersion
  sourceTurnCount: number
  content: ConversationSummaryContent
  createdAt: string
  updatedAt: string
}

export type Run = {
  id: string
  ownerUserId: string
  actorSessionId?: string
  conversationId: string
  turnId: string
  runIndex: number
  status: RunStatus
  rowVersion: number
  promptVersion: PromptVersion
  toolCatalogDigest: string
  /** Run 内已选平台工具的 LRU 顺序（最旧在前）；不包含 Schema。 */
  selectedOperationIds: string[]
  pageContext: Record<string, unknown>
  traceContext?: Record<string, string>
  createdAt: string
  startedAt?: string
  completedAt?: string
  errorCode?: string
  model?: AIModelSnapshot
  /** 当前 Run 最近一次主回答模型调用的官方 prompt_tokens 快照。 */
  latestPromptTokens?: number
  latestUsageModelId?: string
  latestUsageMaxContextTokensSnapshot?: number
}

/** 仅供 Executor claim 使用；不得通过 Run 查询或 API 返回。 */
export type RunExecutionSnapshot = {
  runtimeSettings: RuntimeSettings
  providerConfig?: RemoteProviderConfig
  toolCatalogDigest?: string
}

export type ClaimedRun = Run & { executionSnapshot?: RunExecutionSnapshot }

export const runCheckpointPhases = [
  "model", "tool_execution", "verification", "awaiting_input", "awaiting_approval", "waiting_external", "finalizing",
] as const
export type RunCheckpointPhase = typeof runCheckpointPhases[number]

/**
 * Executor 必须在进程退出后恢复的 Run 控制流状态。
 * 阶段、模型步数和剩余预算以 checkpoint 为准；Timeline 只作为用户可见投影，
 * 不得用其中的条目反推控制流位置。
 */
export type RunCheckpoint = {
  runId: string
  revision: number
  phase: RunCheckpointPhase
  /** 下一步模型调用的索引；恢复时从该步继续，不重置预算。 */
  modelStep: number
  /** Run 内已执行的工具调用数；循环保护的 executed 计数由该列承载。 */
  toolCallCount: number
  /** 启动恢复次数；由启动对账写入，Executor 不修改。 */
  recoveries: number
  state: RunCheckpointState
  createdAt: string
  updatedAt: string
}

export type RunCheckpointState = {
  /** 循环保护计数；executed 由 RunCheckpoint.toolCallCount 承载，避免两处事实。 */
  loopGuard?: {
    proposed: number
    occurrences: Array<{ operationId: string, argumentsHash: string, count: number }>
    nonRetryableCalls: Array<{ operationId: string, argumentsHash: string }>
  }
  /** 模型协议违例的有界修复次数。 */
  protocolRepairCount?: number
  /** 平台工具参数修复次数，按 operationId 计数。 */
  argumentRepairAttempts?: Record<string, number>
  /** phase=awaiting_approval 时等待审批的持久化 tool call id。 */
  pendingToolCallId?: string
  /** phase=finalizing 时已决定但尚未提交的最终答复；终态批次只存在于流式会话内存中。 */
  finalOutput?: string
  /** phase=verification/waiting_external 时尚未完成的权威回读。 */
  pendingVerifications?: RunToolVerification[]
  /** 已发生但尚未被成功权威回读解决的副作用或工具失败。 */
  completionBlockers?: RunCompletionBlocker[]
}

export type RunToolVerification = {
  runId: string
  sourceToolCallId: string
  sourceOperationId: string
  operationId: string
  family: "build_run" | "release" | "gateway_route" | "application" | "deployment_target" | "runtime_secrets" | "runtime_config_secrets" | "project_order" | "billing_rate_rules" | "resource"
  expectation: "present" | "absent" | "canceled"
  arguments: Record<string, unknown>
  externalOperationId: string
  attempts: number
  requiredKeys?: string[]
  absentKeys?: string[]
  expected?: unknown
  resource?: {
    singleton: boolean
    collection: boolean
    targetField: string
    statusField?: string
    pendingStatuses?: string[]
    successStatuses?: string[]
    failureStatuses?: string[]
  }
}

export type RunCompletionBlocker = {
  toolCallId: string
  idempotencyKey: string
  operationId: string
  reason: "verification_required" | "tool_failed"
  errorCode?: string
}

/** Executor 对 checkpoint 的一次推进；expectedRevision=0 表示首次创建。 */
export type RunCheckpointAdvance = {
  expectedRevision: number
  phase: RunCheckpointPhase
  modelStep: number
  toolCallCount: number
  state: RunCheckpointState
}

export type TimelineItem = {
  id: string
  runId: string
  turnId: string
  timelineIndex: number
  revision: number
  type: "user_message" | "reasoning_summary" | "progress" | "assistant_message" | "tool_call" | "tool_result" | "system_notice"
  status: "streaming" | "completed" | "failed"
  content: Record<string, unknown>
  createdAt: string
}

export type TimelineMutation = {
  item: TimelineItem
  event: RunEvent
}

export type TimelineTurn = {
  id: string
  turnIndex: number
  status: RunStatus
  input: string
  createdAt: string
  run?: Run
  items: TimelineItem[]
}

export type ContextUsageSnapshot = {
  status: "reported"
  runId: string
  modelId: string
  usedTokens: number
  maxContextTokensSnapshot: number
  recordedAt: string
}

export type TimelinePage = {
  conversation: Conversation
  contextUsage?: ContextUsageSnapshot
  turns: TimelineTurn[]
  eventCursors: Array<{ runId: string, after: number }>
  pageInfo: {
    hasOlder: boolean
    oldestTurnIndex?: number
  }
}

export type RunEvent = {
  id: string
  runId: string
  sequence: number
  type: string
  data: Record<string, unknown>
  createdAt: string
}

export type CreateTurn = {
  conversationId: string
  input: string
  pageContext: Record<string, unknown>
  traceContext?: Record<string, string>
  idempotencyKey: string
  preallocatedRunId?: string
  actorSessionId?: string
  toolCatalogDigest?: string
  modelId?: string
  modelSnapshot?: AIModelSnapshot
}

export type CreatedTurn = { turn: Turn, run: Run }
