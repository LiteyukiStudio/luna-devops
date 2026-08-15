import type { AIRunStreamSubscription } from './run-stream-manager'
import type { AIAssistantState } from './state'
import type { AIEvent, AITimeline } from '@/api'
import {
  addOptimisticTurn,
  emptyAIAssistantState,
  isValidAITimeline,
  mergeTimelineSnapshot,
  reduceAIEvent,
  stateFromTimeline,
} from './state'

export interface AITimelineQueryData {
  snapshot?: AITimeline
  state: AIAssistantState
}

export function aiTimelineQueryKey(conversationId: string | undefined) {
  return ['ai', 'timeline', conversationId] as const
}

export function timelineQueryDataFromSnapshot(snapshot: AITimeline): AITimelineQueryData {
  if (!isValidAITimeline(snapshot))
    throw new Error('ai_invalid_timeline')
  return { snapshot, state: stateFromTimeline(snapshot) }
}

export function mergeTimelineQuerySnapshot(
  current: AITimelineQueryData | undefined,
  incoming: AITimelineQueryData,
): AITimelineQueryData {
  if (!incoming.snapshot)
    return incoming
  return {
    snapshot: incoming.snapshot,
    state: mergeTimelineSnapshot(current?.state, incoming.snapshot),
  }
}

export function applyTimelineQueryEvent(
  current: AITimelineQueryData | undefined,
  event: AIEvent,
): AITimelineQueryData {
  const state = current?.state ?? emptyAIAssistantState
  const nextState = reduceAIEvent(state, event)
  if (current && nextState === state)
    return current
  return { snapshot: current?.snapshot, state: nextState }
}

export function addOptimisticTimelineTurn(
  current: AITimelineQueryData | undefined,
  input: { turnId: string, turnIndex: number, runId: string, text: string },
): AITimelineQueryData {
  return {
    snapshot: current?.snapshot,
    state: addOptimisticTurn(current?.state ?? emptyAIAssistantState, input),
  }
}

export function activeRunStreamSubscriptions(
  data: AITimelineQueryData | undefined,
  conversationId = data?.snapshot?.conversation.id,
  currentSubscriptions: AIRunStreamSubscription[] = [],
): AIRunStreamSubscription[] {
  if (!conversationId)
    return []
  const cursors = new Map(data?.snapshot?.eventCursors.map(cursor => [cursor.runId, cursor.after]) ?? [])
  const snapshotRunIds = new Set(data?.snapshot?.turns.flatMap(turn => turn.selectedRun ? [turn.selectedRun.id] : []) ?? [])
  const authoritative = data?.snapshot?.turns.flatMap((turn) => {
    const run = turn.selectedRun
    if (!run || ['completed', 'failed', 'canceled'].includes(data.state.runStatuses[run.id] ?? run.status))
      return []
    return [{
      conversationId,
      eventsUrl: `/api/v1/ai/runs/${encodeURIComponent(run.id)}/events`,
      runId: run.id,
      after: Math.max(cursors.get(run.id) ?? 0, data.state.lastEventSequences[run.id] ?? 0),
    }]
  }) ?? []
  const optimistic = currentSubscriptions.filter(subscription =>
    subscription.conversationId === conversationId
    && !snapshotRunIds.has(subscription.runId)
    && !['completed', 'failed', 'canceled'].includes(data?.state.runStatuses[subscription.runId] ?? 'queued'))
  return [...authoritative, ...optimistic]
}

export async function recoverTimelineOnce(
  recoveries: Set<string>,
  conversationId: string,
  recover: () => Promise<unknown>,
) {
  if (recoveries.has(conversationId))
    return
  recoveries.add(conversationId)
  try {
    await recover()
  }
  finally {
    recoveries.delete(conversationId)
  }
}
