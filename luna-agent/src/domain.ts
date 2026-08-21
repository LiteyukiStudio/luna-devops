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
  cachedOutputCreditsPerMillion: string
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
  clientInstanceId?: string
  createdAt: string
  startedAt?: string
  completedAt?: string
  errorCode?: string
  model?: AIModelSnapshot
  /** 当前 Run 最近一次主回答模型调用的输入 token 数，用于展示上下文占用。 */
  latestInputTokens?: number
}

export type TimelineItem = {
  id: string
  runId: string
  turnId: string
  timelineIndex: number
  revision: number
  type: "user_message" | "reasoning_summary" | "assistant_message" | "tool_call" | "tool_result" | "system_notice"
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

export type TimelinePage = {
  conversation: Conversation
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
  clientInstanceId?: string
  modelId?: string
  modelSnapshot?: AIModelSnapshot
}

export type CreatedTurn = { turn: Turn, run: Run }

export type UIActionStatus = "pending" | "succeeded" | "failed" | "expired"

export type UIActionDelivery = {
  id: string
  runId: string
  toolCallId: string
  clientInstanceId: string
  action: Record<string, unknown>
  status: UIActionStatus
  attempts: number
  expiresAt: string
  acknowledgedAt?: string
  actualPath?: string
  errorCode?: string
  createdAt: string
  updatedAt: string
}

export type UIActionAcknowledgement = {
  status: "succeeded" | "failed"
  actualPath?: string
  errorCode?: string
}
