import type { AIOptionVisual, AIToolVisibility } from '@luna-devops/ai-interaction-card-contract'
import type { components } from './generated/openapi.js'

type AISchemas = components['schemas']
type AITimelineItemTransport = AISchemas['AITimelineItem']
type AITimelineTurnTransport = AISchemas['AITimelineTurn']
type AISelectedRunTransport = NonNullable<AITimelineTurnTransport['selectedRun']>

export type AIAssistantAccess = AISchemas['AIAssistantAccess']
export type AICapabilities = Omit<AIAssistantAccess, 'maxInputBytes'> & { maxInputBytes: number }
export type AIModelOption = AISchemas['AIModelOption']
export type AIModelConfig = AISchemas['AIModelConfig']
export type AIConversation = AISchemas['AIConversation']
export type AIConversationPage = AISchemas['AIConversationPage']
export type AIMessagePart = AISchemas['AIMessagePart']
export type AIRunStatus = AISelectedRunTransport['status']
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
    | { version: 1, id?: string, repeatable?: boolean, activation?: 'manual', type: 'send_message', label?: string, description?: string, tone?: 'default' | 'primary' | 'danger', visual?: AIOptionVisual, payload: { message: string } }
    | { version: 1, id?: string, repeatable?: boolean, activation?: 'manual', type: 'request_tool', label?: string, description?: string, tone?: 'default' | 'primary' | 'danger', visual?: AIOptionVisual, payload: { operationId: string, arguments?: Record<string, unknown>, message: string } }

export type AITimelineItem = Omit<AITimelineItemTransport, 'toolCall'> & {
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

export type AITimelineTurn = Omit<AITimelineTurnTransport, 'selectedRun'> & {
  selectedRun?: Omit<AISelectedRunTransport, 'items'> & {
    items: AITimelineItem[]
  }
}

export type AIContextUsage = AISchemas['AIContextUsage']
export type AITimeline = Omit<AISchemas['AITimelinePage'], 'turns'> & {
  turns: AITimelineTurn[]
}

export type AIEvent = Omit<AISchemas['AIEvent'], 'item' | 'payload'> & {
  item?: AITimelineItem
  payload: Record<string, unknown>
}

export interface AITurnCreated {
  turnId: string
  turnIndex: number
  runId: string
  state: AIRunStatus
  eventsUrl: string
}
