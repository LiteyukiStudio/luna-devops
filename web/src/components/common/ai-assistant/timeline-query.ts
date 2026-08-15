import type { InfiniteData } from '@tanstack/react-query'
import type { AIRunStreamSubscription } from './run-stream-manager'
import type { AIAssistantState } from './state'
import type { AIEvent, AITimeline, AITimelineTurn } from '@/api'
import {
  addOptimisticTurn,
  emptyAIAssistantState,
  isValidAITimeline,
  mergeTimelineSnapshot,
  reduceAIEvent,
  stateFromTimeline,
} from './state'

export const AI_TIMELINE_PAGE_SIZE = 30

export interface AITimelineQueryData {
  snapshot?: AITimeline
  state: AIAssistantState
}

export type AITimelinePageParam = string | null
export type AITimelineInfiniteData = InfiniteData<AITimelineQueryData, AITimelinePageParam>

export function aiTimelineQueryKey(conversationId: string | undefined) {
  return ['ai', 'timeline', conversationId] as const
}

export function timelineQueryDataFromSnapshot(snapshot: AITimeline): AITimelineQueryData {
  if (!isValidAITimeline(snapshot))
    throw new Error('ai_invalid_timeline')
  return { snapshot, state: stateFromTimeline(snapshot) }
}

export function olderTimelinePageParam(page: AITimelineQueryData): AITimelinePageParam | undefined {
  return page.snapshot?.pageInfo.hasOlder ? page.snapshot.pageInfo.olderCursor : undefined
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

export function mergeTimelineInfiniteSnapshot(
  current: AITimelineInfiniteData | undefined,
  incoming: AITimelineInfiniteData,
): AITimelineInfiniteData {
  if (!current)
    return incoming
  const merged = {
    pageParams: incoming.pageParams,
    pages: incoming.pages.map((page, index) => {
      const pageParam = incoming.pageParams[index]
      const currentIndex = current.pageParams.findIndex(candidate => candidate === pageParam)
      return mergeTimelineQuerySnapshot(currentIndex >= 0 ? current.pages[currentIndex] : undefined, page)
    }),
  }
  return preserveDisplacedLatestTurns(current, merged)
}

export function mergeLatestTimelineSnapshot(
  current: AITimelineInfiniteData | undefined,
  incoming: AITimelineQueryData,
): AITimelineInfiniteData {
  if (!current)
    return { pages: [incoming], pageParams: [null] }
  const latestIndex = Math.max(0, current.pageParams.findIndex(pageParam => pageParam === null))
  const merged = {
    pageParams: current.pageParams,
    pages: current.pages.map((page, index) => index === latestIndex ? mergeTimelineQuerySnapshot(page, incoming) : page),
  }
  return preserveDisplacedLatestTurns(current, merged)
}

/**
 * Once older pages are loaded, their cursor points before the oldest loaded
 * turn. A growing latest window can therefore push a turn into the gap between
 * that cursor and the refreshed window. Move only that displaced prefix into
 * the adjacent cached history page. Before history is loaded, retain it in the
 * latest page and keep the old cursor so the first older fetch remains gapless.
 * Deleting the conversation still drops the whole query.
 */
function preserveDisplacedLatestTurns(current: AITimelineInfiniteData, incoming: AITimelineInfiniteData): AITimelineInfiniteData {
  const currentLatestIndex = current.pageParams.findIndex(pageParam => pageParam === null)
  const incomingLatestIndex = incoming.pageParams.findIndex(pageParam => pageParam === null)
  const olderIndex = incomingLatestIndex - 1
  const currentLatest = current.pages[currentLatestIndex]?.snapshot
  const incomingLatest = incoming.pages[incomingLatestIndex]?.snapshot
  if (!currentLatest || !incomingLatest)
    return incoming
  const incomingIds = new Set(incomingLatest.turns.map(turn => turn.id))
  const incomingFirstIndex = incomingLatest.turns[0]?.turnIndex ?? Number.MAX_SAFE_INTEGER
  const displaced = currentLatest.turns.filter(turn => turn.turnIndex < incomingFirstIndex && !incomingIds.has(turn.id))
  if (displaced.length === 0)
    return incoming
  const older = incoming.pages[olderIndex]
  if (!older?.snapshot) {
    const turns = new Map(incomingLatest.turns.map(turn => [turn.id, turn]))
    displaced.forEach(turn => turns.set(turn.id, turn))
    const cursors = new Map(currentLatest.eventCursors.map(cursor => [cursor.runId, cursor.after]))
    incomingLatest.eventCursors.forEach(cursor => cursors.set(cursor.runId, Math.max(cursors.get(cursor.runId) ?? 0, cursor.after)))
    const retained = timelineQueryDataFromSnapshot({
      ...incomingLatest,
      turns: [...turns.values()].sort((left, right) => left.turnIndex - right.turnIndex),
      eventCursors: [...cursors].map(([runId, after]) => ({ runId, after })),
      pageInfo: currentLatest.pageInfo,
    })
    return {
      pageParams: incoming.pageParams,
      pages: incoming.pages.map((page, index) => index === incomingLatestIndex
        ? mergeTimelineQuerySnapshot(current.pages[currentLatestIndex], retained)
        : page),
    }
  }
  const turns = new Map(older.snapshot.turns.map(turn => [turn.id, turn]))
  displaced.forEach(turn => turns.set(turn.id, turn))
  const relocated = timelineQueryDataFromSnapshot({
    ...older.snapshot,
    turns: [...turns.values()].sort((left, right) => left.turnIndex - right.turnIndex),
  })
  return {
    pageParams: incoming.pageParams,
    pages: incoming.pages.map((page, index) => index === olderIndex ? mergeTimelineQuerySnapshot(older, relocated) : page),
  }
}

function combinedSnapshot(pages: AITimelineQueryData[]): AITimeline | undefined {
  const snapshots = pages.flatMap(page => page.snapshot ? [page.snapshot] : [])
  if (snapshots.length === 0)
    return undefined
  const turnsById = new Map<string, AITimelineTurn>()
  const eventCursors = new Map<string, number>()
  for (const snapshot of snapshots) {
    snapshot.turns.forEach(turn => turnsById.set(turn.id, turn))
    snapshot.eventCursors.forEach(cursor => eventCursors.set(cursor.runId, Math.max(eventCursors.get(cursor.runId) ?? 0, cursor.after)))
  }
  const oldest = snapshots.reduce((candidate, snapshot) => {
    const candidateIndex = candidate.turns[0]?.turnIndex ?? Number.MAX_SAFE_INTEGER
    const snapshotIndex = snapshot.turns[0]?.turnIndex ?? Number.MAX_SAFE_INTEGER
    return snapshotIndex < candidateIndex ? snapshot : candidate
  })
  const latest = snapshots.reduce((candidate, snapshot) => {
    const candidateIndex = candidate.turns.at(-1)?.turnIndex ?? -1
    const snapshotIndex = snapshot.turns.at(-1)?.turnIndex ?? -1
    return snapshotIndex > candidateIndex ? snapshot : candidate
  })
  return {
    conversation: latest.conversation,
    turns: [...turnsById.values()].sort((left, right) => left.turnIndex - right.turnIndex),
    eventCursors: [...eventCursors].map(([runId, after]) => ({ runId, after })),
    pageInfo: oldest.pageInfo,
  }
}

function combinedState(pages: AITimelineQueryData[], snapshot: AITimeline): AIAssistantState {
  const authoritative = stateFromTimeline(snapshot)
  const blocks = new Map(authoritative.blocks.map(block => [block.id, block]))
  const itemRevisions = { ...authoritative.itemRevisions }
  const selectedBlockRevisions = { ...authoritative.itemRevisions }
  const seenEventIds = new Set<string>()
  const desyncedRunIds = new Set<string>()
  const lastEventSequences = { ...authoritative.lastEventSequences }
  const runStatuses = { ...authoritative.runStatuses }
  const runExpectedVersions = { ...authoritative.runExpectedVersions }
  const turnIndexes = { ...authoritative.turnIndexes }
  const desyncRecoverySequences: Record<string, number> = {}

  for (const page of pages) {
    page.state.seenEventIds.forEach(eventId => seenEventIds.add(eventId))
    page.state.desyncedRunIds.forEach(runId => desyncedRunIds.add(runId))
    Object.assign(runStatuses, page.state.runStatuses)
    Object.assign(runExpectedVersions, page.state.runExpectedVersions)
    Object.assign(turnIndexes, page.state.turnIndexes)
    for (const [runId, sequence] of Object.entries(page.state.lastEventSequences))
      lastEventSequences[runId] = Math.max(lastEventSequences[runId] ?? 0, sequence)
    for (const [runId, sequence] of Object.entries(page.state.desyncRecoverySequences))
      desyncRecoverySequences[runId] = Math.max(desyncRecoverySequences[runId] ?? 0, sequence)
    for (const [itemId, revision] of Object.entries(page.state.itemRevisions))
      itemRevisions[itemId] = Math.max(itemRevisions[itemId] ?? 0, revision)
    for (const block of page.state.blocks) {
      const existing = blocks.get(block.id)
      const revision = page.state.itemRevisions[block.id] ?? 0
      if (!existing || revision > (selectedBlockRevisions[block.id] ?? 0)) {
        blocks.set(block.id, block)
        selectedBlockRevisions[block.id] = revision
      }
    }
  }
  return {
    blocks: [...blocks.values()].sort((left, right) => left.index - right.index),
    seenEventIds,
    runStatuses,
    runExpectedVersions,
    lastEventSequences,
    turnIndexes,
    itemRevisions,
    desyncedRunIds,
    desyncRecoverySequences,
  }
}

export function timelineQueryDataFromInfinite(data: AITimelineInfiniteData | undefined): AITimelineQueryData | undefined {
  if (!data)
    return undefined
  const snapshot = combinedSnapshot(data.pages)
  if (!snapshot)
    return { state: emptyAIAssistantState }
  return { snapshot, state: combinedState(data.pages, snapshot) }
}

export function applyTimelineInfiniteEvent(
  current: AITimelineInfiniteData | undefined,
  event: AIEvent,
): AITimelineInfiniteData | undefined {
  if (!current)
    return current
  let targetIndex = current.pages.findIndex(page => page.state.turnIndexes[event.turnId] !== undefined)
  if (targetIndex < 0)
    targetIndex = Math.max(0, current.pages.length - 1)
  const currentPage = current.pages[targetIndex]
  const nextPage = applyTimelineQueryEvent(currentPage, event)
  if (nextPage === currentPage)
    return current
  return {
    pageParams: current.pageParams,
    pages: current.pages.map((page, index) => index === targetIndex ? nextPage : page),
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
  current: AITimelineInfiniteData | undefined,
  input: { turnId: string, turnIndex: number, runId: string, text: string },
): AITimelineInfiniteData {
  const initial = current ?? { pages: [{ state: emptyAIAssistantState }], pageParams: [null] }
  let targetIndex = initial.pageParams.findIndex(pageParam => pageParam === null)
  if (targetIndex < 0)
    targetIndex = Math.max(0, initial.pages.length - 1)
  return {
    pageParams: initial.pageParams,
    pages: initial.pages.map((page, index) => index === targetIndex
      ? { snapshot: page.snapshot, state: addOptimisticTurn(page.state, input) }
      : page),
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
