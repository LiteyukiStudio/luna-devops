import type { AIOptionVisual, AIToolVisibility } from '@luna-devops/ai-interaction-card-contract'

export interface AICapabilities {
  enabled: boolean
  maxInputBytes: number
}

export interface AIModelOption {
  id: string
  name: string
  maxContextTokens: number
  maxOutputTokens: number
}

export interface AIModelConfig extends AIModelOption {
  inputCreditsPerMillion: string
  outputCreditsPerMillion: string
  cachedInputCreditsPerMillion: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface AIConversation {
  id: string
  title: string
  titleSource: 'default' | 'assistant' | 'user'
  status: string
  projectId?: string
  modelId?: string
  createdAt: string
  updatedAt: string
}

export interface AIPaginatedResponse<T> {
  items: T[]
  page: number
  pageSize: number
  sortBy: string
  sortOrder: 'asc' | 'desc'
  total: number
  totalPages: number
}

export interface AIMessagePart {
  id: string
  partIndex: number
  type: 'text' | 'structured_data'
  text?: string
  data?: Record<string, unknown>
}

export type AIRunStatus = 'queued' | 'running' | 'waiting_approval' | 'waiting_input' | 'completed' | 'failed' | 'canceled' | 'interrupted'
export type AIToolStatus = 'proposed' | 'awaiting_approval' | 'running' | 'succeeded' | 'failed' | 'rejected' | 'canceled' | 'skipped'

export interface AIToolDisplayResult {
  summaryKey: string
  summaryParams?: Record<string, string | number | boolean>
  requestId?: string
  errorCode?: string
  errorMessage?: string
  generationId?: string
  attempt?: number
  maxAttempts?: number
  data?: unknown
  issues?: Array<{
    code: string
    path: string
    message: string
    expected?: string
    received?: string
  }>
  fields?: Array<{
    labelKey: string
    value: string | number | boolean | null
    tone?: 'neutral' | 'success' | 'warning' | 'danger'
  }>
  presentation?: {
    component: 'key_value' | 'status_list' | 'resource_link' | 'log_excerpt'
    version: 1
  }
}

export type AIUIAction
  = | { version: 1, id?: string, repeatable?: boolean, activation?: 'manual', type: 'navigate', label?: string, description?: string, tone?: 'default' | 'primary' | 'danger', visual?: AIOptionVisual, payload: { routeName: string, params?: Record<string, string>, query?: Record<string, string> } }
    | { version: 1, type: 'select_tab', payload: { tabId: string } }
    | { version: 1, type: 'set_filters', payload: { targetId: string, values: Record<string, string> } }
    | { version: 1, type: 'refresh_query', payload: { queryKeyId: string } }
    | { version: 1, type: 'highlight', payload: { resourceId: string } }
    | { version: 1, id?: string, repeatable?: boolean, activation?: 'manual', type: 'send_message', label?: string, description?: string, tone?: 'default' | 'primary' | 'danger', visual?: AIOptionVisual, payload: { message: string } }
    | { version: 1, id?: string, repeatable?: boolean, activation?: 'manual', type: 'request_tool', label?: string, description?: string, tone?: 'default' | 'primary' | 'danger', visual?: AIOptionVisual, payload: { operationId: string, arguments?: Record<string, unknown>, message: string } }

export interface AITimelineItem {
  id: string
  timelineIndex: number
  revision: number
  createdAt: string
  type: 'user_message' | 'reasoning_summary' | 'progress' | 'assistant_message' | 'tool_call' | 'tool_result' | 'system_notice'
  status: string
  relatedItemId?: string
  parts: AIMessagePart[]
  display?: 'summary' | 'progress'
  notice?: string
  toolCall?: {
    id: string
    operationId: string
    visibility?: AIToolVisibility
    titleKey?: string
    errorCode?: string
    callIndex: number
    status?: AIToolStatus
    arguments?: Record<string, unknown>
    result?: AIToolDisplayResult
    uiActions?: AIUIAction[]
    durationMs?: number
    traceId?: string
  }
}

export interface AITimelineTurn {
  id: string
  turnIndex: number
  status: string
  input: {
    id: string
    type: 'user_message'
    createdAt: string
    parts: AIMessagePart[]
  }
  selectedRun?: {
    id: string
    runIndex: number
    status: AIRunStatus
    expectedVersion?: number
    errorCode?: string
    /** 当前 Run 最近一次主回答模型调用的官方 prompt_tokens。 */
    latestPromptTokens?: number
    latestUsageModelId?: string
    latestUsageMaxContextTokensSnapshot?: number
    items: AITimelineItem[]
  }
}

export interface AIContextUsage {
  status: 'reported'
  runId: string
  modelId: string
  /** 最近一次主回答完成后的官方 total_tokens。 */
  usedTokens: number
  maxContextTokensSnapshot: number
  recordedAt: string
}

export interface AITimeline {
  conversation: Pick<AIConversation, 'id' | 'title' | 'titleSource' | 'status' | 'modelId'>
  /** 会话最近一次已确认的上下文大小；新 Run 启动时不会清零。 */
  contextUsage?: AIContextUsage
  turns: AITimelineTurn[]
  eventCursors: Array<{ runId: string, after: number }>
  pageInfo: {
    olderCursor?: string
    hasOlder: boolean
  }
}

export type AIProviderUsage
  = | {
    status: 'reported'
    inputTokens: number
    outputTokens: number
    totalTokens: number
    cacheReadInputTokens?: number | null
    cacheWriteInputTokens?: number | null
    reasoningOutputTokens?: number | null
  }
  | { status: 'unavailable', reason: 'missing_usage' | 'invalid_usage' | 'stream_ended_without_usage' }
  | { status: 'reconciliation_required', reason: 'missing_usage' | 'invalid_usage' | 'stream_ended_without_usage' | 'request_outcome_unknown' | 'hold_deficit' }

export interface AIModelCompletedPayload {
  usage: AIProviderUsage
  modelId?: string
  maxContextTokensSnapshot?: number
  creditHoldId?: string
  providerRequestId?: string
  responseId?: string
  responseModel?: string
  finishReason?: string
}

export interface AIEvent {
  version: 2
  eventId: string
  eventSequence: number
  type: string
  conversationId: string
  turnId: string
  runId: string
  itemId?: string
  contentPartId?: string
  toolCallId?: string
  item?: AITimelineItem
  occurredAt: string
  payload: Record<string, unknown>
}

export interface AITurnCreated {
  turnId: string
  turnIndex: number
  runId: string
  state: AIRunStatus
  eventsUrl: string
}
