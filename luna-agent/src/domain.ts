import type { RuntimeSettings } from "./runtime-settings.js"

export const runStatuses = [
	"queued", "running", "waiting_approval", "waiting_input", "waiting_external",
  "completed", "failed", "canceled", "expired", "interrupted",
] as const
export type RunStatus = typeof runStatuses[number]

export type ActorContext = {
  userId: string
  sessionId: string
  platformRole?: string
  projectId?: string
  locale: string
  issuedAt: number
  expiresAt: number
  requestId: string
  runId?: string
}

export type ApprovalPreviewChange = {
  field: string
  before?: unknown
  after?: unknown
  action?: string
}

export type ApprovalPreview = {
  summary: string
  targetResource?: string
  targetId?: string
  projectId?: string
  changes?: ApprovalPreviewChange[]
  sensitiveKeys?: string[]
  impact?: string[]
  warnings?: string[]
}

export type ConversationTitleSource = "default" | "assistant" | "user"
export const conversationApprovalModes = ["strict", "default", "auto", "full_access"] as const
export type ConversationApprovalMode = typeof conversationApprovalModes[number]
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
  approvalMode: ConversationApprovalMode
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
  /** 该轮模型推理原文（已脱敏）；需要回传思维链的 Provider 方言据此继续推理。 */
  reasoning?: string
  pageContext?: Record<string, unknown>
  toolInteractions?: Array<Record<string, unknown>>
}

export type ConversationToolInteraction = {
  itemId: string
  type: "tool_call" | "tool_result" | "reasoning_summary" | "progress" | "user_message"
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
  wakeAt?: string
  errorCode?: string
  model?: AIModelSnapshot
  /** 当前 Run 最近一次主回答模型调用的官方 prompt_tokens 快照。 */
  latestPromptTokens?: number
  latestUsageModelId?: string
  latestUsageMaxContextTokensSnapshot?: number
}

/** 仅供 Executor claim 使用；不得通过 Run 查询或 API 返回。严禁包含 providerConfig / apiKey / toolCatalog。 */
export type RunExecutionSnapshot = {
  runtimeSettings: RuntimeSettings
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

export type RunPendingToolCall = {
  id: string
  operationId: string
  arguments?: Record<string, unknown>
  argumentsCiphertext?: string
  argumentError?: {
    code: "invalid_json"
    message: string
  }
}

export type RunWaitingTaskState = {
  itemId: string
  toolCallId: string
  projectId: string
  taskType: "build_run" | "release"
  taskId: string
  timeoutSeconds: number
  startedAt: string
  deadlineAt: string
  pollCount: number
}

export type RunCardGenerationState = {
  operationId: "present_card" | "request_input" | "request_choice"
  itemId: string
  toolCallId: string
  modelCallId?: string
  batchId?: string
  timelineIndex: number
  generationId: string
  placeholderArguments: {
    schemaVersion: 1
    generationId: string
    title?: string
    description?: string
    placement: "inline" | "turn_end"
  }
  attempt: number
  maxAttempts: number
  status: "streaming" | "completed" | "failed"
  issues?: Array<{
    code: string
    path: string
    message: string
    expected?: string
  }>
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
  /** 当前活跃的交互卡片占位与修复进度。 */
  cardGeneration?: RunCardGenerationState
  /** 当前回合跨卡片工具累计修复失败次数。 */
  cardRepairAttempts?: number
  /** 当前回合是否已耗尽交互卡片修复次数（仅阻断继续调用卡片工具，不杀死非卡片工具）。 */
  cardRepairExhausted?: boolean
  /** phase=awaiting_approval 时等待审批的持久化 tool call id。 */
  pendingToolCallId?: string
  /** phase=waiting_external 时持久化挂起的后台任务等待状态。 */
  waitingTask?: RunWaitingTaskState
  /** 同批次工具调用在暂停点之后的剩余未执行队列与批次标识。 */
  pendingBatchId?: string
  pendingBatchCalls?: RunPendingToolCall[]
  /** phase=finalizing 时已决定但尚未提交的最终答复；终态批次只存在于流式会话内存中。 */
  finalOutput?: string
}

export type RunToolVerification = {
  runId: string
  sourceToolCallId: string
  sourceOperationId: string
  operationId: string
  family: "build_run" | "release" | "gateway_route" | "application" | "deployment_target" | "project_order" | "billing_rate_rules" | "resource"
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
    collectionPath?: string
    targetField: string
    statusField?: string
    pendingStatuses?: string[]
    successStatuses?: string[]
    failureStatuses?: string[]
  }
}

/** Executor 对 checkpoint 的一次推进；expectedRevision=0 表示首次创建。 */
export type RunCheckpointAdvance = {
  expectedRevision: number
  expectedRunVersion?: number
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
  directToolAction?: {
    operationId: string
    arguments: Record<string, unknown>
  }
  traceContext?: Record<string, string>
  idempotencyKey: string
  preallocatedRunId?: string
  actorSessionId?: string
  toolCatalogDigest?: string
  modelId?: string
  modelSnapshot?: AIModelSnapshot
}

export type CreatedTurn = { turn: Turn, run: Run, replayed?: boolean }
