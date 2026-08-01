import type { AIAssistantState } from './state'
import type { AIEvent, AITimeline } from '@/api'
import { addOptimisticTurn, emptyAIAssistantState, mergeTimelineSnapshot, reduceAIEvent } from './state'

export const AI_EVENT_TYPES = [
  'run.started',
  'run.running',
  'run.queued',
  'run.waiting_approval',
  'run.waiting_mfa',
  'run.waiting_input',
  'run.input_required',
  'model.started',
  'content.delta',
  'message.completed',
  'thinking.started',
  'thinking.delta',
  'thinking.completed',
  'item.finalized',
  'tool.started',
  'tool.progress',
  'tool.completed',
  'tool.failed',
  'approval.required',
  'approval.resolved',
  'mfa.required',
  'mfa.resolved',
  'ui.action',
  'model.completed',
  'run.failed',
  'run.completed',
  'run.canceled',
] as const

export interface LiveSubscription {
  conversationId: string
  eventsUrl: string
  runId: string
}

type SessionStateAction
  = { type: 'snapshot', conversationId: string, timeline: AITimeline }
    | { type: 'event', event: AIEvent }
    | { type: 'optimistic_turn', conversationId: string, turnId: string, turnIndex: number, runId: string, text: string }

export function sessionStateReducer(state: Record<string, AIAssistantState>, action: SessionStateAction) {
  if (action.type === 'snapshot')
    return { ...state, [action.conversationId]: mergeTimelineSnapshot(state[action.conversationId], action.timeline) }
  if (action.type === 'optimistic_turn') {
    return {
      ...state,
      [action.conversationId]: addOptimisticTurn(state[action.conversationId] ?? emptyAIAssistantState, action),
    }
  }
  return {
    ...state,
    [action.event.conversationId]: reduceAIEvent(state[action.event.conversationId] ?? emptyAIAssistantState, action.event),
  }
}
