export interface AICapabilities {
  available: boolean
  reasonCode: string | null
  features: {
    streaming: boolean
    approvals: boolean
    stepUpMFA: boolean
    uiActions: boolean
    longTermMemory: boolean
  }
  limits: {
    maxInputBytes: number
    maxConcurrentRuns: number
  }
}

export function isUsableAICapabilities(value: unknown): value is AICapabilities {
  if (!value || typeof value !== 'object')
    return false
  const candidate = value as Partial<AICapabilities>
  return candidate.available === true
    && typeof candidate.features?.streaming === 'boolean'
    && typeof candidate.features?.approvals === 'boolean'
    && typeof candidate.features?.stepUpMFA === 'boolean'
    && typeof candidate.features?.uiActions === 'boolean'
    && typeof candidate.limits?.maxInputBytes === 'number'
    && candidate.limits.maxInputBytes > 0
    && typeof candidate.limits?.maxConcurrentRuns === 'number'
    && candidate.limits.maxConcurrentRuns > 0
}

export interface AIConversation {
  id: string
  title: string
  status: string
  projectId?: string
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

export type AIRunStatus = 'queued' | 'running' | 'waiting_approval' | 'waiting_mfa' | 'waiting_input' | 'completed' | 'failed' | 'canceled'
export type AIToolStatus = 'proposed' | 'awaiting_approval' | 'awaiting_mfa' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'skipped'

export interface AIToolDisplayResult {
  summaryKey: string
  summaryParams?: Record<string, string | number | boolean>
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
  = | { version: 1, type: 'navigate', payload: { routeName: string, params?: Record<string, string>, query?: Record<string, string> } }
    | { version: 1, type: 'select_tab', payload: { tabId: string } }
    | { version: 1, type: 'set_filters', payload: { targetId: string, values: Record<string, string> } }
    | { version: 1, type: 'refresh_query', payload: { queryKeyId: string } }
    | { version: 1, type: 'highlight', payload: { resourceId: string } }

export interface AITimelineItem {
  id: string
  timelineIndex: number
  type: 'reasoning_summary' | 'progress' | 'assistant_message' | 'tool_call' | 'tool_result'
  status: string
  relatedItemId?: string
  parts: AIMessagePart[]
  display?: 'summary' | 'progress'
  toolCall?: {
    id: string
    operationId: string
    titleKey?: string
    callIndex: number
    status?: AIToolStatus
    arguments?: Record<string, unknown>
    result?: AIToolDisplayResult
    uiActions?: AIUIAction[]
    durationMs?: number
    argumentsHash?: string
    expectedVersion?: number
    mfaPurpose?: string
  }
}

export interface AITimelineTurn {
  id: string
  turnIndex: number
  status: string
  input: {
    id: string
    type: 'user_message'
    parts: AIMessagePart[]
  }
  selectedRun?: {
    id: string
    runIndex: number
    status: AIRunStatus
    expectedVersion?: number
    items: AITimelineItem[]
  }
}

export interface AITimeline {
  conversation: Pick<AIConversation, 'id' | 'title' | 'status'>
  turns: AITimelineTurn[]
  eventCursors: Array<{ runId: string, after: number }>
}

export interface AIEvent {
  version: number
  eventId: string
  eventSequence: number
  type: string
  conversationId: string
  turnId: string
  runId: string
  itemId?: string
  contentPartId?: string
  toolCallId?: string
  occurredAt: string
  payload: Record<string, unknown>
}

export interface AITurnCreated {
  turnId: string
  runId: string
  state: AIRunStatus
  eventsUrl: string
}

export interface AIMFAResumePayload {
  stepUpAssertionId: string
  expectedVersion: number
}
