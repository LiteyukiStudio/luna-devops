export const runStatuses = [
  "queued", "running", "waiting_approval", "waiting_mfa", "waiting_input",
  "completed", "failed", "canceled", "expired",
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

export type Conversation = {
  id: string
  ownerUserId: string
  projectId?: string
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
  createdAt: string
}

export type ConversationHistoryEntry = {
  turnIndex: number
  user: string
  assistant: string
}

export type Run = {
  id: string
  conversationId: string
  turnId: string
  runIndex: number
  status: RunStatus
  rowVersion: number
  graphVersion: "assistant-v1"
  promptVersion: PromptVersion
  toolCatalogDigest: string
  pageContext: Record<string, unknown>
  traceContext?: Record<string, string>
  clientInstanceId?: string
  createdAt: string
  startedAt?: string
  completedAt?: string
  errorCode?: string
}

export type TimelineItem = {
  id: string
  runId: string
  turnId: string
  timelineIndex: number
  revision: number
  type: "user_message" | "reasoning_summary" | "assistant_message" | "tool_call" | "tool_result"
  status: "streaming" | "completed" | "failed"
  content: Record<string, unknown>
  createdAt: string
}

export type TimelineMutation = {
  item: TimelineItem
  event: RunEvent
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
  runActorGrantCiphertext?: string
  toolCatalogDigest?: string
  clientInstanceId?: string
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
