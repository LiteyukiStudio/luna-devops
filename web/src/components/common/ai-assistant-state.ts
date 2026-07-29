import type { AIEvent, AIMessagePart, AITimeline, AIToolDisplayResult, AIToolStatus, AIUIAction } from '@/api'

export type AIBlock
  = | { id: string, turnId: string, index: number, type: 'thinking', status: string, display: 'summary' | 'progress', text: string }
    | { id: string, turnId: string, index: number, type: 'message', role: 'user' | 'assistant', status: string, text: string }
    | { id: string, turnId: string, runId: string, index: number, type: 'tool_call', toolCallId: string, operationId: string, titleKey?: string, status: AIToolStatus, arguments: Record<string, unknown>, result?: AIToolDisplayResult, uiActions: AIUIAction[], durationMs?: number, argumentsHash?: string, expectedVersion?: number, mfaPurpose?: string }

export interface AIAssistantState {
  blocks: AIBlock[]
  seenEventIds: Set<string>
  runStatuses: Record<string, string>
  runExpectedVersions: Record<string, number>
  lastEventSequences: Record<string, number>
}

export function isValidAITimeline(value: unknown): value is AITimeline {
  if (!value || typeof value !== 'object')
    return false
  const candidate = value as Partial<AITimeline>
  if (!candidate.conversation || typeof candidate.conversation.id !== 'string' || !Array.isArray(candidate.turns) || !Array.isArray(candidate.eventCursors))
    return false
  return candidate.turns.every(turn =>
    Boolean(turn)
    && typeof turn.id === 'string'
    && typeof turn.turnIndex === 'number'
    && Boolean(turn.input)
    && Array.isArray(turn.input.parts)
    && turn.input.parts.every(part => Boolean(part) && typeof part.id === 'string' && typeof part.partIndex === 'number')
    && (!turn.selectedRun || (typeof turn.selectedRun.id === 'string'
      && Array.isArray(turn.selectedRun.items)
      && turn.selectedRun.items.every(item => Boolean(item) && typeof item.id === 'string' && typeof item.timelineIndex === 'number' && Array.isArray(item.parts)))),
  ) && candidate.eventCursors.every(cursor => Boolean(cursor) && typeof cursor.runId === 'string' && typeof cursor.after === 'number')
}

function textFromParts(parts: AIMessagePart[]) {
  return [...parts].sort((a, b) => a.partIndex - b.partIndex).map(part => part.text ?? '').join('')
}

export function stateFromTimeline(timeline: AITimeline): AIAssistantState {
  const blocks: AIBlock[] = []
  for (const turn of [...timeline.turns].sort((a, b) => a.turnIndex - b.turnIndex)) {
    blocks.push({
      id: turn.input.id,
      turnId: turn.id,
      index: turn.turnIndex * 10000 - 1,
      type: 'message',
      role: 'user',
      status: 'completed',
      text: textFromParts(turn.input.parts),
    })
    const results = new Map(turn.selectedRun?.items.filter(item => item.type === 'tool_result' && item.relatedItemId).map(item => [item.relatedItemId!, item]) ?? [])
    for (const item of [...(turn.selectedRun?.items ?? [])].sort((a, b) => a.timelineIndex - b.timelineIndex)) {
      if (item.type === 'assistant_message') {
        blocks.push({ id: item.id, turnId: turn.id, index: turn.turnIndex * 10000 + item.timelineIndex, type: 'message', role: 'assistant', status: item.status, text: textFromParts(item.parts) })
      }
      else if (item.type === 'reasoning_summary' || item.type === 'progress') {
        blocks.push({ id: item.id, turnId: turn.id, index: turn.turnIndex * 10000 + item.timelineIndex, type: 'thinking', status: item.status, display: item.type === 'progress' ? 'progress' : 'summary', text: textFromParts(item.parts) })
      }
      else if (item.type === 'tool_call' && item.toolCall) {
        const resultItem = results.get(item.id)
        const structuredResult = resultItem?.parts.find(part => part.type === 'structured_data')?.data as AIToolDisplayResult | undefined
        blocks.push({
          id: item.id,
          turnId: turn.id,
          runId: turn.selectedRun?.id ?? '',
          index: turn.turnIndex * 10000 + item.timelineIndex,
          type: 'tool_call',
          toolCallId: item.toolCall.id,
          operationId: item.toolCall.operationId,
          titleKey: item.toolCall.titleKey,
          status: item.toolCall.status ?? (item.status === 'completed' ? 'succeeded' : 'running'),
          arguments: item.toolCall.arguments ?? {},
          result: item.toolCall.result ?? structuredResult,
          uiActions: item.toolCall.uiActions ?? [],
          durationMs: item.toolCall.durationMs,
          argumentsHash: item.toolCall.argumentsHash,
          expectedVersion: item.toolCall.expectedVersion ?? turn.selectedRun?.expectedVersion,
          mfaPurpose: item.toolCall.mfaPurpose,
        })
      }
    }
  }
  return {
    blocks: blocks.sort((a, b) => a.index - b.index),
    seenEventIds: new Set(),
    runStatuses: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun ? [[turn.selectedRun.id, turn.selectedRun.status]] : [])),
    runExpectedVersions: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun?.expectedVersion === undefined ? [] : [[turn.selectedRun.id, turn.selectedRun.expectedVersion]])),
    lastEventSequences: Object.fromEntries(timeline.eventCursors.map(cursor => [cursor.runId, cursor.after])),
  }
}

function stringPayload(payload: Record<string, unknown>, key: string) {
  return typeof payload[key] === 'string' ? payload[key] : ''
}

function updateBlock(state: AIAssistantState, id: string, update: (block: AIBlock) => AIBlock, create?: () => AIBlock) {
  const blockIndex = state.blocks.findIndex(block => block.id === id || (block.type === 'tool_call' && block.toolCallId === id))
  if (blockIndex < 0)
    return create ? [...state.blocks, create()].sort((a, b) => a.index - b.index) : state.blocks
  return state.blocks.map((block, index) => index === blockIndex ? update(block) : block)
}

export function reduceAIEvent(state: AIAssistantState, event: AIEvent): AIAssistantState {
  if (event.version !== 1 || !event.eventId || state.seenEventIds.has(event.eventId))
    return state
  if (event.eventSequence <= (state.lastEventSequences[event.runId] ?? 0))
    return state

  const next = {
    ...state,
    seenEventIds: new Set(state.seenEventIds).add(event.eventId),
    lastEventSequences: { ...state.lastEventSequences, [event.runId]: event.eventSequence },
  }
  const itemId = event.itemId ?? `${event.type}:${event.runId}`
  const eventIndex = Number(event.payload.timelineIndex ?? event.eventSequence)

  if (event.type === 'run.started' || event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.canceled') {
    const status = event.type === 'run.started' ? 'running' : event.type.slice(4)
    return {
      ...next,
      runStatuses: { ...state.runStatuses, [event.runId]: status },
      runExpectedVersions: typeof event.payload.expectedVersion === 'number'
        ? { ...state.runExpectedVersions, [event.runId]: event.payload.expectedVersion }
        : state.runExpectedVersions,
    }
  }
  if (event.type === 'content.delta') {
    const delta = stringPayload(event.payload, 'delta') || stringPayload(event.payload, 'text')
    return {
      ...next,
      blocks: updateBlock(state, itemId, block => block.type === 'message' ? { ...block, text: block.text + delta, status: 'streaming' } : block, () => ({
        id: itemId,
        turnId: event.turnId,
        index: eventIndex,
        type: 'message',
        role: 'assistant',
        status: 'streaming',
        text: delta,
      })),
    }
  }
  if (event.type === 'approval.required' || event.type === 'approval.resolved' || event.type === 'mfa.required' || event.type === 'mfa.resolved') {
    const toolCallId = event.toolCallId ?? itemId
    const required = event.type.endsWith('.required')
    const status: AIToolStatus = event.type.startsWith('approval')
      ? (required ? 'awaiting_approval' : 'running')
      : (required ? 'awaiting_mfa' : 'running')
    return {
      ...next,
      runStatuses: {
        ...state.runStatuses,
        [event.runId]: required ? (event.type.startsWith('approval') ? 'waiting_approval' : 'waiting_mfa') : 'running',
      },
      runExpectedVersions: typeof event.payload.expectedVersion === 'number'
        ? { ...state.runExpectedVersions, [event.runId]: event.payload.expectedVersion }
        : state.runExpectedVersions,
      blocks: updateBlock(state, toolCallId, block => block.type === 'tool_call'
        ? {
            ...block,
            status,
            argumentsHash: stringPayload(event.payload, 'argumentsHash') || block.argumentsHash,
            expectedVersion: typeof event.payload.expectedVersion === 'number' ? event.payload.expectedVersion : block.expectedVersion,
            mfaPurpose: stringPayload(event.payload, 'purpose') || block.mfaPurpose,
          }
        : block),
    }
  }
  if (event.type === 'thinking.started' || event.type === 'thinking.delta' || event.type === 'thinking.completed') {
    const delta = stringPayload(event.payload, 'delta') || stringPayload(event.payload, 'summary')
    const display = event.payload.display === 'summary' ? 'summary' : 'progress'
    return {
      ...next,
      blocks: updateBlock(state, itemId, block => block.type === 'thinking'
        ? { ...block, text: event.type === 'thinking.started' ? delta : block.text + delta, status: event.type === 'thinking.completed' ? 'completed' : 'streaming', display }
        : block, () => ({
        id: itemId,
        turnId: event.turnId,
        index: eventIndex,
        type: 'thinking',
        status: event.type === 'thinking.completed' ? 'completed' : 'streaming',
        display,
        text: delta,
      })),
    }
  }
  if (event.type === 'tool.started' || event.type === 'tool.progress' || event.type === 'tool.completed' || event.type === 'tool.failed') {
    const toolCallId = event.toolCallId ?? itemId
    const status: AIToolStatus = event.type === 'tool.completed' ? 'succeeded' : event.type === 'tool.failed' ? 'failed' : 'running'
    return {
      ...next,
      blocks: updateBlock(state, toolCallId, block => block.type === 'tool_call'
        ? {
            ...block,
            status,
            result: event.payload.result as AIToolDisplayResult | undefined ?? block.result,
            uiActions: event.payload.uiActions as AIUIAction[] | undefined ?? block.uiActions,
            durationMs: typeof event.payload.durationMs === 'number' ? event.payload.durationMs : block.durationMs,
          }
        : block, () => ({
        id: itemId,
        turnId: event.turnId,
        runId: event.runId,
        index: eventIndex,
        type: 'tool_call',
        toolCallId,
        operationId: stringPayload(event.payload, 'operationId'),
        titleKey: stringPayload(event.payload, 'titleKey') || undefined,
        status,
        arguments: typeof event.payload.arguments === 'object' && event.payload.arguments ? event.payload.arguments as Record<string, unknown> : {},
        result: event.payload.result as AIToolDisplayResult | undefined,
        uiActions: event.payload.uiActions as AIUIAction[] | undefined ?? [],
      })),
    }
  }
  return next
}

export const emptyAIAssistantState: AIAssistantState = {
  blocks: [],
  seenEventIds: new Set(),
  runStatuses: {},
  runExpectedVersions: {},
  lastEventSequences: {},
}
