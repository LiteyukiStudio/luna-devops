import type { AIEvent, AIMessagePart, AITimeline, AIToolDisplayResult, AIToolStatus, AIUIAction } from '@/api'

export type AIBlock
  = | { id: string, turnId: string, index: number, type: 'thinking', status: string, display: 'summary' | 'progress', text: string }
    | { id: string, turnId: string, index: number, type: 'message', role: 'user' | 'assistant', status: string, text: string }
    | { id: string, turnId: string, runId: string, index: number, type: 'run_status', status: 'failed' | 'canceled', errorCode?: string }
    | { id: string, turnId: string, runId: string, index: number, type: 'tool_call', toolCallId: string, operationId: string, titleKey?: string, status: AIToolStatus, arguments: Record<string, unknown>, result?: AIToolDisplayResult, uiActions: AIUIAction[], durationMs?: number, argumentsHash?: string, expectedVersion?: number, mfaPurpose?: string }

export interface AIAssistantState {
  blocks: AIBlock[]
  seenEventIds: Set<string>
  runStatuses: Record<string, string>
  runExpectedVersions: Record<string, number>
  lastEventSequences: Record<string, number>
  turnIndexes: Record<string, number>
}

const TURN_ORDER_STRIDE = 1_000_000

function blockIndex(turnIndex: number, itemIndex: number): number {
  return turnIndex * TURN_ORDER_STRIDE + itemIndex
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
      index: blockIndex(turn.turnIndex, -1),
      type: 'message',
      role: 'user',
      status: 'completed',
      text: textFromParts(turn.input.parts),
    })
    const results = new Map(turn.selectedRun?.items.filter(item => item.type === 'tool_result' && item.relatedItemId).map(item => [item.relatedItemId!, item]) ?? [])
    for (const item of [...(turn.selectedRun?.items ?? [])].sort((a, b) => a.timelineIndex - b.timelineIndex)) {
      if (item.type === 'assistant_message') {
        blocks.push({ id: item.id, turnId: turn.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'message', role: 'assistant', status: item.status, text: textFromParts(item.parts) })
      }
      else if (item.type === 'reasoning_summary' || item.type === 'progress') {
        blocks.push({ id: item.id, turnId: turn.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'thinking', status: item.status, display: item.type === 'progress' ? 'progress' : 'summary', text: textFromParts(item.parts) })
      }
      else if (item.type === 'tool_call' && item.toolCall) {
        const resultItem = results.get(item.id)
        const structuredResult = resultItem?.parts.find(part => part.type === 'structured_data')?.data as AIToolDisplayResult | undefined
        blocks.push({
          id: item.id,
          turnId: turn.id,
          runId: turn.selectedRun?.id ?? '',
          index: blockIndex(turn.turnIndex, item.timelineIndex),
          type: 'tool_call',
          toolCallId: item.toolCall.id,
          operationId: item.toolCall.operationId,
          titleKey: item.toolCall.titleKey,
          status: item.toolCall.status ?? (item.status === 'completed' ? 'succeeded' : 'running'),
          arguments: item.toolCall.arguments ?? {},
          result: normalizeToolResult(item.toolCall.result ?? structuredResult),
          uiActions: item.toolCall.uiActions ?? [],
          durationMs: item.toolCall.durationMs,
          argumentsHash: item.toolCall.argumentsHash,
          expectedVersion: item.toolCall.expectedVersion ?? turn.selectedRun?.expectedVersion,
          mfaPurpose: item.toolCall.mfaPurpose,
        })
      }
    }
    if (turn.selectedRun?.status === 'failed' || turn.selectedRun?.status === 'canceled') {
      blocks.push({
        id: `${turn.selectedRun.id}:status`,
        turnId: turn.id,
        runId: turn.selectedRun.id,
        index: blockIndex(turn.turnIndex, TURN_ORDER_STRIDE - 1),
        type: 'run_status',
        status: turn.selectedRun.status,
        errorCode: turn.selectedRun.errorCode,
      })
    }
  }
  return {
    blocks: blocks.sort((a, b) => a.index - b.index),
    seenEventIds: new Set(),
    runStatuses: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun ? [[turn.selectedRun.id, turn.selectedRun.status]] : [])),
    runExpectedVersions: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun?.expectedVersion === undefined ? [] : [[turn.selectedRun.id, turn.selectedRun.expectedVersion]])),
    lastEventSequences: Object.fromEntries(timeline.eventCursors.map(cursor => [cursor.runId, cursor.after])),
    turnIndexes: Object.fromEntries(timeline.turns.map(turn => [turn.id, turn.turnIndex])),
  }
}

export function mergeTimelineSnapshot(current: AIAssistantState | undefined, timeline: AITimeline): AIAssistantState {
  const snapshot = stateFromTimeline(timeline)
  if (!current)
    return snapshot
  const snapshotIds = new Set(snapshot.blocks.map(block => block.id))
  const currentBlocks = new Map(current.blocks.map(block => [block.id, block]))
  const blocks = snapshot.blocks.map((block) => {
    const live = currentBlocks.get(block.id)
    if (!live || live.type !== block.type)
      return block
    if ((block.type === 'message' && live.type === 'message') || (block.type === 'thinking' && live.type === 'thinking')) {
      return live.text.length > block.text.length ? { ...live, index: block.index } : block
    }
    if (block.type === 'tool_call' && live.type === 'tool_call' && live.status !== 'succeeded' && live.status !== 'failed')
      return { ...live, index: block.index }
    return block
  })
  current.blocks.filter(block => block.status === 'streaming' && !snapshotIds.has(block.id)).forEach(block => blocks.push(block))
  return {
    ...snapshot,
    blocks: blocks.sort((a, b) => a.index - b.index),
    seenEventIds: current.seenEventIds,
    lastEventSequences: Object.fromEntries([...new Set([...Object.keys(snapshot.lastEventSequences), ...Object.keys(current.lastEventSequences)])]
      .map(runId => [runId, Math.max(snapshot.lastEventSequences[runId] ?? 0, current.lastEventSequences[runId] ?? 0)])),
    runStatuses: mergeRunStatuses(snapshot.runStatuses, current.runStatuses),
    runExpectedVersions: { ...snapshot.runExpectedVersions, ...current.runExpectedVersions },
    turnIndexes: { ...snapshot.turnIndexes, ...current.turnIndexes },
  }
}

function mergeRunStatuses(snapshot: Record<string, string>, current: Record<string, string>) {
  const terminal = new Set(['completed', 'failed', 'canceled'])
  return Object.fromEntries([...new Set([...Object.keys(snapshot), ...Object.keys(current)])].map((runId) => {
    const currentStatus = current[runId]
    const snapshotStatus = snapshot[runId]
    return [runId, terminal.has(currentStatus) ? currentStatus : snapshotStatus ?? currentStatus]
  }))
}

export function addOptimisticTurn(state: AIAssistantState, input: { turnId: string, turnIndex: number, runId: string, text: string }): AIAssistantState {
  if (state.turnIndexes[input.turnId] !== undefined)
    return state
  return {
    ...state,
    blocks: [...state.blocks, {
      id: `${input.turnId}:input`,
      turnId: input.turnId,
      index: blockIndex(input.turnIndex, -1),
      type: 'message' as const,
      role: 'user' as const,
      status: 'completed',
      text: input.text,
    }].sort((a, b) => a.index - b.index),
    runStatuses: { ...state.runStatuses, [input.runId]: 'queued' },
    turnIndexes: { ...state.turnIndexes, [input.turnId]: input.turnIndex },
  }
}

function stringPayload(payload: Record<string, unknown>, key: string) {
  return typeof payload[key] === 'string' ? payload[key] : ''
}

function updateBlock(state: AIAssistantState, id: string, update: (block: AIBlock) => AIBlock, create?: () => AIBlock) {
  const blockIndex = state.blocks.findIndex(block => block.id === id || (block.type === 'tool_call' && block.toolCallId === id))
  if (blockIndex < 0)
    return create ? [...state.blocks, create()].sort((a, b) => a.index - b.index) : state.blocks
  return state.blocks.map((block, index) => index === blockIndex ? update(block) : block).sort((a, b) => a.index - b.index)
}

export function reduceAIEvent(state: AIAssistantState, event: AIEvent): AIAssistantState {
  if (event.version !== 1 || !event.eventId || state.seenEventIds.has(event.eventId))
    return state
  if (event.eventSequence <= (state.lastEventSequences[event.runId] ?? 0))
    return state

  const turnIndex = state.turnIndexes[event.turnId] ?? Math.max(-1, ...Object.values(state.turnIndexes)) + 1
  const next = {
    ...state,
    seenEventIds: new Set(state.seenEventIds).add(event.eventId),
    lastEventSequences: { ...state.lastEventSequences, [event.runId]: event.eventSequence },
    turnIndexes: { ...state.turnIndexes, [event.turnId]: turnIndex },
  }
  const itemId = event.itemId ?? `${event.type}:${event.runId}`
  const timelineIndex = typeof event.payload.timelineIndex === 'number'
    ? event.payload.timelineIndex
    : TURN_ORDER_STRIDE / 2 + event.eventSequence
  const eventIndex = blockIndex(turnIndex, timelineIndex)

  if (event.type === 'run.started' || event.type === 'run.queued' || event.type === 'run.waiting_approval' || event.type === 'run.waiting_mfa' || event.type === 'run.waiting_input' || event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.canceled') {
    const status = event.type === 'run.started' ? 'running' : event.type.slice(4)
    const terminalBlock = status === 'failed' || status === 'canceled'
      ? updateBlock(state, `${event.runId}:status`, block => block.type === 'run_status'
          ? { ...block, status, errorCode: stringPayload(event.payload, 'errorCode') || block.errorCode }
          : block, () => ({
          id: `${event.runId}:status`,
          turnId: event.turnId,
          runId: event.runId,
          index: blockIndex(turnIndex, TURN_ORDER_STRIDE - 1),
          type: 'run_status',
          status,
          errorCode: stringPayload(event.payload, 'errorCode') || undefined,
        }))
      : state.blocks
    return {
      ...next,
      blocks: terminalBlock,
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
      blocks: updateBlock(state, itemId, block => block.type === 'message' ? { ...block, index: eventIndex, text: block.text + delta, status: 'streaming' } : block, () => ({
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
  if (event.type === 'content.completed' || event.type === 'message.completed' || event.type === 'item.completed') {
    return {
      ...next,
      blocks: updateBlock(state, itemId, block => block.type === 'tool_call' || block.type === 'run_status' ? block : { ...block, index: eventIndex, status: 'completed' }),
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
            index: eventIndex,
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
        ? { ...block, index: eventIndex, text: event.type === 'thinking.started' ? delta : block.text + delta, status: event.type === 'thinking.completed' ? 'completed' : 'streaming', display }
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
    const status: AIToolStatus = event.type === 'tool.completed'
      ? event.payload.status === 'skipped' ? 'skipped' : 'succeeded'
      : event.type === 'tool.failed' ? 'failed' : 'running'
    return {
      ...next,
      blocks: updateBlock(state, toolCallId, block => block.type === 'tool_call'
        ? {
            ...block,
            index: eventIndex,
            status,
            titleKey: stringPayload(event.payload, 'errorCode') || block.titleKey,
            result: normalizeToolResult(event.payload.result) ?? block.result,
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
        status,
        arguments: typeof event.payload.arguments === 'object' && event.payload.arguments ? event.payload.arguments as Record<string, unknown> : {},
        titleKey: stringPayload(event.payload, 'errorCode') || stringPayload(event.payload, 'titleKey') || undefined,
        result: normalizeToolResult(event.payload.result),
        uiActions: event.payload.uiActions as AIUIAction[] | undefined ?? [],
      })),
    }
  }
  return next
}

function normalizeToolResult(value: unknown): AIToolDisplayResult | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    return undefined
  const result = value as Record<string, unknown>
  return {
    summaryKey: typeof result.summaryKey === 'string' ? result.summaryKey : 'ai.tool.result.completed',
    ...(result.summaryParams && typeof result.summaryParams === 'object' ? { summaryParams: result.summaryParams as Record<string, string | number | boolean> } : {}),
    ...(typeof result.requestId === 'string' ? { requestId: result.requestId } : {}),
    ...(Array.isArray(result.fields) ? { fields: result.fields as AIToolDisplayResult['fields'] } : {}),
    ...(result.presentation && typeof result.presentation === 'object' ? { presentation: result.presentation as AIToolDisplayResult['presentation'] } : {}),
  }
}

export const emptyAIAssistantState: AIAssistantState = {
  blocks: [],
  seenEventIds: new Set(),
  runStatuses: {},
  runExpectedVersions: {},
  lastEventSequences: {},
  turnIndexes: {},
}
