import type { AIToolVisibility } from '@luna-devops/ai-interaction-card-contract'
import type { AIContextUsage, AIEvent, AIMessagePart, AITimeline, AITimelineItem, AIToolDisplayResult, AIToolStatus, AIUIAction } from '@/api'
import { z } from 'zod'

export type AIBlock
  = | { id: string, turnId: string, runId?: string, index: number, type: 'thinking', status: string, display: 'summary' | 'progress', text: string }
    | { id: string, turnId: string, runId?: string, index: number, type: 'message', role: 'user' | 'assistant', status: string, text: string, createdAt: string }
    | { id: string, turnId: string, runId: string, index: number, type: 'run_status', status: 'failed' | 'canceled' | 'interrupted', errorCode?: string }
    | { id: string, turnId: string, runId?: string, index: number, type: 'context_compacted', status: string }
    | { id: string, turnId: string, runId: string, index: number, type: 'tool_call', toolCallId: string, operationId: string, visibility: AIToolVisibility, titleKey?: string, errorCode?: string, status: AIToolStatus, arguments: Record<string, unknown>, result?: AIToolDisplayResult, uiActions: AIUIAction[], durationMs?: number, traceId?: string }

export interface AIRunUsage {
  /** 只有 reported 状态允许参与上下文比例展示。 */
  status: 'reported' | 'unavailable' | 'reconciliation_required'
  promptTokens?: number
  modelId?: string
  maxContextTokensSnapshot?: number
}

export interface AIAssistantState {
  blocks: AIBlock[]
  runStatuses: Record<string, string>
  runExpectedVersions: Record<string, number>
  lastEventSequences: Record<string, number>
  turnIndexes: Record<string, number>
  itemRevisions: Record<string, number>
  desyncedRunIds: Set<string>
  desyncRecoverySequences: Record<string, number>
  /** 上下文用量必须按 Run 隔离，避免新一轮继承上一轮的输入量。 */
  runUsage: Record<string, AIRunUsage>
  /** 会话最近一次已确认的上下文大小；活动 Run 创建时保持不变。 */
  contextUsage?: AIContextUsage
}

const TURN_ORDER_STRIDE = 1_000_000

const aiToolDisplayResultSchema = z.object({
  summaryKey: z.string(),
  summaryParams: z.record(z.string(), z.union([z.string(), z.number(), z.boolean()])).optional(),
  requestId: z.string().optional(),
  errorCode: z.string().optional(),
  errorMessage: z.string().optional(),
  generationId: z.string().optional(),
  attempt: z.number().optional(),
  maxAttempts: z.number().optional(),
  data: z.unknown().optional(),
  issues: z.array(z.object({
    code: z.string(),
    path: z.string(),
    message: z.string(),
    expected: z.string().optional(),
    received: z.string().optional(),
  })).optional(),
  fields: z.array(z.object({
    labelKey: z.string(),
    value: z.union([z.string(), z.number(), z.boolean(), z.null()]),
    tone: z.enum(['neutral', 'success', 'warning', 'danger']).optional(),
  })).optional(),
  presentation: z.object({
    component: z.enum(['key_value', 'status_list', 'resource_link', 'log_excerpt']),
    version: z.literal(1),
  }).optional(),
})

function blockIndex(turnIndex: number, itemIndex: number): number {
  return turnIndex * TURN_ORDER_STRIDE + itemIndex
}

function isOptionalTokenCount(value: unknown) {
  return value === undefined || (typeof value === 'number' && Number.isSafeInteger(value) && value >= 0)
}

export function isValidAITimelineItem(value: unknown): value is AITimelineItem {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    return false
  const item = value as Partial<AITimelineItem>
  if (typeof item.id !== 'string' || typeof item.timelineIndex !== 'number' || typeof item.revision !== 'number' || typeof item.createdAt !== 'string' || !Array.isArray(item.parts))
    return false
  if (item.type !== 'tool_call')
    return true
  const toolCall = item.toolCall
  return Boolean(toolCall)
    && typeof toolCall?.id === 'string'
    && typeof toolCall.operationId === 'string'
    && typeof toolCall.callIndex === 'number'
    && (toolCall.result === undefined || aiToolDisplayResultSchema.safeParse(toolCall.result).success)
}

function hasCompleteReportedUsage(run: NonNullable<AITimeline['turns'][number]['selectedRun']>) {
  const values = [run.latestPromptTokens, run.latestUsageModelId, run.latestUsageMaxContextTokensSnapshot]
  return values.every(value => value !== undefined)
    && isOptionalTokenCount(run.latestPromptTokens)
    && typeof run.latestUsageModelId === 'string'
    && run.latestUsageModelId.length > 0
    && typeof run.latestUsageMaxContextTokensSnapshot === 'number'
    && Number.isSafeInteger(run.latestUsageMaxContextTokensSnapshot)
    && run.latestUsageMaxContextTokensSnapshot > 0
}

function isValidContextUsage(value: unknown): value is AIContextUsage {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    return false
  const usage = value as Partial<AIContextUsage>
  return usage.status === 'reported'
    && typeof usage.runId === 'string'
    && usage.runId.length > 0
    && typeof usage.modelId === 'string'
    && usage.modelId.length > 0
    && typeof usage.usedTokens === 'number'
    && Number.isSafeInteger(usage.usedTokens)
    && usage.usedTokens >= 0
    && typeof usage.maxContextTokensSnapshot === 'number'
    && Number.isSafeInteger(usage.maxContextTokensSnapshot)
    && usage.maxContextTokensSnapshot > 0
    && typeof usage.recordedAt === 'string'
    && Number.isFinite(Date.parse(usage.recordedAt))
}

export function latestContextUsage(left: AIContextUsage | undefined, right: AIContextUsage | undefined) {
  if (!left)
    return right
  if (!right)
    return left
  return Date.parse(right.recordedAt) >= Date.parse(left.recordedAt) ? right : left
}

export function isValidAITimeline(value: unknown): value is AITimeline {
  if (!value || typeof value !== 'object')
    return false
  const candidate = value as Partial<AITimeline>
  if (!candidate.conversation || typeof candidate.conversation.id !== 'string' || !Array.isArray(candidate.turns) || !Array.isArray(candidate.eventCursors)
    || !candidate.pageInfo || typeof candidate.pageInfo.hasOlder !== 'boolean'
    || (candidate.pageInfo.olderCursor !== undefined && typeof candidate.pageInfo.olderCursor !== 'string')
    || (candidate.contextUsage !== undefined && !isValidContextUsage(candidate.contextUsage))) {
    return false
  }
  return candidate.turns.every(turn =>
    Boolean(turn)
    && typeof turn.id === 'string'
    && typeof turn.turnIndex === 'number'
    && Boolean(turn.input)
    && typeof turn.input.createdAt === 'string'
    && Array.isArray(turn.input.parts)
    && turn.input.parts.every(part => Boolean(part) && typeof part.id === 'string' && typeof part.partIndex === 'number')
    && (!turn.selectedRun || (typeof turn.selectedRun.id === 'string'
      && (hasCompleteReportedUsage(turn.selectedRun)
        || (turn.selectedRun.latestPromptTokens === undefined
          && turn.selectedRun.latestUsageModelId === undefined
          && turn.selectedRun.latestUsageMaxContextTokensSnapshot === undefined))
        && Array.isArray(turn.selectedRun.items)
        && turn.selectedRun.items.every(isValidAITimelineItem))),
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
      createdAt: turn.input.createdAt,
    })
    for (const item of [...(turn.selectedRun?.items ?? [])].sort((a, b) => a.timelineIndex - b.timelineIndex)) {
      if (item.type === 'assistant_message') {
        blocks.push({ id: item.id, turnId: turn.id, runId: turn.selectedRun?.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'message', role: 'assistant', status: item.status, text: textFromParts(item.parts), createdAt: item.createdAt })
      }
      else if (item.type === 'reasoning_summary' || item.type === 'progress') {
        blocks.push({ id: item.id, turnId: turn.id, runId: turn.selectedRun?.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'thinking', status: item.status, display: item.type === 'progress' ? 'progress' : 'summary', text: textFromParts(item.parts) })
      }
      else if (item.type === 'system_notice' && item.notice === 'context_compacted') {
        blocks.push({ id: item.id, turnId: turn.id, runId: turn.selectedRun?.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'context_compacted', status: item.status })
      }
      else if (item.type === 'tool_call' && item.toolCall) {
        blocks.push({
          id: item.id,
          turnId: turn.id,
          runId: turn.selectedRun?.id ?? '',
          index: blockIndex(turn.turnIndex, item.timelineIndex),
          type: 'tool_call',
          toolCallId: item.toolCall.id,
          operationId: item.toolCall.operationId,
          visibility: item.toolCall.visibility ?? 'normal',
          titleKey: item.toolCall.titleKey,
          errorCode: item.toolCall.errorCode,
          status: item.toolCall.status ?? (item.status === 'completed' ? 'succeeded' : 'running'),
          arguments: item.toolCall.arguments ?? {},
          result: item.toolCall.result,
          uiActions: item.toolCall.uiActions ?? [],
          durationMs: item.toolCall.durationMs,
          traceId: item.toolCall.traceId,
        })
      }
    }
    if (turn.selectedRun?.status === 'failed' || turn.selectedRun?.status === 'canceled' || turn.selectedRun?.status === 'interrupted') {
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
    runStatuses: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun ? [[turn.selectedRun.id, turn.selectedRun.status]] : [])),
    runExpectedVersions: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun?.expectedVersion === undefined ? [] : [[turn.selectedRun.id, turn.selectedRun.expectedVersion]])),
    lastEventSequences: Object.fromEntries(timeline.eventCursors.map(cursor => [cursor.runId, cursor.after])),
    turnIndexes: Object.fromEntries(timeline.turns.map(turn => [turn.id, turn.turnIndex])),
    itemRevisions: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun?.items.map(item => [item.id, item.revision]) ?? [])),
    desyncedRunIds: new Set(),
    desyncRecoverySequences: {},
    contextUsage: timeline.contextUsage,
    runUsage: Object.fromEntries(timeline.turns.flatMap((turn) => {
      const run = turn.selectedRun
      if (!run)
        return []
      return [[run.id, hasCompleteReportedUsage(run)
        ? {
            status: 'reported' as const,
            promptTokens: run.latestPromptTokens,
            modelId: run.latestUsageModelId,
            maxContextTokensSnapshot: run.latestUsageMaxContextTokensSnapshot,
          }
        : { status: 'unavailable' as const }]]
    })),
  }
}

function mergeRunUsage(
  snapshot: Record<string, AIRunUsage>,
  current: Record<string, AIRunUsage>,
  snapshotSequences: Record<string, number>,
  currentSequences: Record<string, number>,
) {
  return Object.fromEntries([...new Set([...Object.keys(snapshot), ...Object.keys(current)])].map((runId) => {
    const snapshotUsage = snapshot[runId]
    const currentUsage = current[runId]
    const currentAhead = (currentSequences[runId] ?? 0) > (snapshotSequences[runId] ?? 0)
    return [runId, (currentAhead ? currentUsage : snapshotUsage) ?? currentUsage ?? snapshotUsage ?? { status: 'unavailable' }]
  }))
}

export function mergeTimelineSnapshot(current: AIAssistantState | undefined, timeline: AITimeline): AIAssistantState {
  const snapshot = stateFromTimeline(timeline)
  if (!current)
    return snapshot
  const activeRunByTurn = new Map(timeline.turns.flatMap((turn) => {
    const run = turn.selectedRun
    if (!run || isTerminalRunStatus(run.status) || isTerminalRunStatus(current.runStatuses[run.id]))
      return []
    return [[turn.id, run.id] as const]
  }))
  const belongsToActiveRun = (block: AIBlock) =>
    block.runId !== undefined && activeRunByTurn.get(block.turnId) === block.runId
  const snapshotIds = new Set(snapshot.blocks.map(block => block.id))
  const currentBlocks = new Map(current.blocks.map(block => [block.id, block]))
  const retainedCurrentIds = new Set<string>()
  const blocks = snapshot.blocks.map((block) => {
    const live = currentBlocks.get(block.id)
    if (!live || live.type !== block.type)
      return block
    if (belongsToActiveRun(live)
      && (current.itemRevisions[block.id] ?? 0) > (snapshot.itemRevisions[block.id] ?? 0)) {
      retainedCurrentIds.add(block.id)
      return live
    }
    return block
  })
  current.blocks
    .filter(block => block.type === 'message'
      && block.role === 'user'
      && (current.itemRevisions[block.id] ?? 0) === 0
      && !snapshotIds.has(block.id))
    .forEach((block) => {
      retainedCurrentIds.add(block.id)
      blocks.push(block)
    })
  current.blocks
    .filter(block => !snapshotIds.has(block.id) && belongsToActiveRun(block))
    .forEach((block) => {
      retainedCurrentIds.add(block.id)
      blocks.push(block)
    })
  const itemRevisionIds = new Set([...Object.keys(snapshot.itemRevisions), ...retainedCurrentIds])
  return {
    ...snapshot,
    blocks: blocks.sort((a, b) => a.index - b.index),
    lastEventSequences: Object.fromEntries([...new Set([...Object.keys(snapshot.lastEventSequences), ...Object.keys(current.lastEventSequences)])]
      .map(runId => [runId, Math.max(snapshot.lastEventSequences[runId] ?? 0, current.lastEventSequences[runId] ?? 0)])),
    runStatuses: mergeRunStatuses(snapshot.runStatuses, current.runStatuses),
    runExpectedVersions: { ...snapshot.runExpectedVersions, ...current.runExpectedVersions },
    turnIndexes: { ...snapshot.turnIndexes, ...current.turnIndexes },
    itemRevisions: Object.fromEntries([...itemRevisionIds]
      .map(itemId => [itemId, Math.max(current.itemRevisions[itemId] ?? 0, snapshot.itemRevisions[itemId] ?? 0)])),
    runUsage: mergeRunUsage(snapshot.runUsage, current.runUsage, snapshot.lastEventSequences, current.lastEventSequences),
    contextUsage: latestContextUsage(current.contextUsage, snapshot.contextUsage),
    desyncedRunIds: new Set([...current.desyncedRunIds].filter(runId =>
      (snapshot.lastEventSequences[runId] ?? 0) < (current.desyncRecoverySequences[runId] ?? Number.MAX_SAFE_INTEGER))),
    desyncRecoverySequences: Object.fromEntries(Object.entries(current.desyncRecoverySequences).filter(([runId, sequence]) =>
      (snapshot.lastEventSequences[runId] ?? 0) < sequence)),
  }
}

function mergeRunStatuses(snapshot: Record<string, string>, current: Record<string, string>) {
  return Object.fromEntries([...new Set([...Object.keys(snapshot), ...Object.keys(current)])].map((runId) => {
    const currentStatus = current[runId]
    const snapshotStatus = snapshot[runId]
    return [runId, isTerminalRunStatus(currentStatus) ? currentStatus : snapshotStatus ?? currentStatus]
  }))
}

function isTerminalRunStatus(status: string | undefined) {
  return status === 'completed' || status === 'failed' || status === 'canceled' || status === 'interrupted'
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
      createdAt: new Date().toISOString(),
    }].sort((a, b) => a.index - b.index),
    runStatuses: { ...state.runStatuses, [input.runId]: 'queued' },
    runUsage: { ...state.runUsage, [input.runId]: { status: 'unavailable' } },
    turnIndexes: { ...state.turnIndexes, [input.turnId]: input.turnIndex },
    itemRevisions: { ...state.itemRevisions, [`${input.turnId}:input`]: 0 },
  }
}

function stringPayload(payload: Record<string, unknown>, key: string) {
  return typeof payload[key] === 'string' ? payload[key] : ''
}

function integerPayload(payload: Record<string, unknown>, key: string) {
  const value = payload[key]
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : undefined
}

function assertPayloadString(payload: Record<string, unknown>, key: string) {
  const value = stringPayload(payload, key)
  if (!value)
    throw new Error('ai_invalid_stream_event_payload')
  return value
}

function assertLiveItemIdentity(event: AIEvent) {
  const itemId = assertPayloadString(event.payload, 'itemId')
  const timelineIndex = integerPayload(event.payload, 'timelineIndex')
  if (event.item !== undefined || event.itemId !== itemId || timelineIndex === undefined)
    throw new Error('ai_invalid_stream_event_payload')
  return { itemId, timelineIndex }
}

function updateBlock(state: AIAssistantState, id: string, update: (block: AIBlock) => AIBlock, create?: () => AIBlock) {
  const blockIndex = state.blocks.findIndex(block => block.id === id || (block.type === 'tool_call' && block.toolCallId === id))
  if (blockIndex < 0)
    return create ? [...state.blocks, create()].sort((a, b) => a.index - b.index) : state.blocks
  return state.blocks.map((block, index) => index === blockIndex ? update(block) : block).sort((a, b) => a.index - b.index)
}

function blockFromTimelineItem(item: AITimelineItem, turnId: string, runId: string, turnIndex: number): AIBlock | undefined {
  const index = blockIndex(turnIndex, item.timelineIndex)
  if (item.type === 'user_message') {
    return {
      id: item.id,
      turnId,
      index: blockIndex(turnIndex, -1),
      type: 'message',
      role: 'user',
      status: item.status,
      text: textFromParts(item.parts),
      createdAt: item.createdAt,
    }
  }
  if (item.type === 'assistant_message') {
    return {
      id: item.id,
      turnId,
      runId,
      index,
      type: 'message',
      role: 'assistant',
      status: item.status,
      text: textFromParts(item.parts),
      createdAt: item.createdAt,
    }
  }
  if (item.type === 'reasoning_summary' || item.type === 'progress') {
    return {
      id: item.id,
      turnId,
      runId,
      index,
      type: 'thinking',
      status: item.status,
      display: item.type === 'progress' ? 'progress' : item.display ?? 'summary',
      text: textFromParts(item.parts),
    }
  }
  if (item.type === 'system_notice') {
    if (item.notice !== 'context_compacted')
      return undefined
    return { id: item.id, turnId, runId, index, type: 'context_compacted', status: item.status }
  }
  if (item.type !== 'tool_call' || !item.toolCall)
    return undefined
  return {
    id: item.id,
    turnId,
    runId,
    index,
    type: 'tool_call',
    toolCallId: item.toolCall.id,
    operationId: item.toolCall.operationId,
    visibility: item.toolCall.visibility ?? 'normal',
    titleKey: item.toolCall.titleKey,
    errorCode: item.toolCall.errorCode,
    status: item.toolCall.status ?? (item.status === 'completed' ? 'succeeded' : item.status === 'failed' ? 'failed' : 'running'),
    arguments: item.toolCall.arguments ?? {},
    result: item.toolCall.result,
    uiActions: item.toolCall.uiActions ?? [],
    durationMs: item.toolCall.durationMs,
    traceId: item.toolCall.traceId,
  }
}

function applyAuthoritativeItem(state: AIAssistantState, event: AIEvent, turnIndex: number): AIAssistantState {
  const item = event.item
  if (!item || item.revision <= (state.itemRevisions[item.id] ?? 0))
    return state
  const block = blockFromTimelineItem(item, event.turnId, event.runId, turnIndex)
  if (!block)
    return { ...state, itemRevisions: { ...state.itemRevisions, [item.id]: item.revision } }
  const existingIndex = state.blocks.findIndex(candidate => candidate.id === item.id)
  const blocks = existingIndex < 0
    ? [...state.blocks, block]
    : state.blocks.map((candidate, index) => index === existingIndex ? block : candidate)
  return {
    ...state,
    blocks: blocks.sort((left, right) => left.index - right.index),
    itemRevisions: { ...state.itemRevisions, [item.id]: item.revision },
  }
}

function applyContentDelta(state: AIAssistantState, event: AIEvent, turnIndex: number): AIAssistantState {
  const { itemId, timelineIndex } = assertLiveItemIdentity(event)
  const contentPartId = assertPayloadString(event.payload, 'contentPartId')
  const partIndex = integerPayload(event.payload, 'partIndex')
  const delta = assertPayloadString(event.payload, 'delta')
  if (event.contentPartId !== contentPartId || partIndex !== 0)
    throw new Error('ai_invalid_stream_event_payload')
  const index = blockIndex(turnIndex, timelineIndex)
  const existing = state.blocks.find(block => block.id === itemId)
  if (existing) {
    if (existing.type !== 'message'
      || existing.role !== 'assistant'
      || existing.turnId !== event.turnId
      || existing.runId !== event.runId
      || existing.index !== index
      || existing.status !== 'streaming') {
      throw new Error('ai_invalid_stream_event_payload')
    }
    return {
      ...state,
      blocks: state.blocks.map(block => block.id === itemId ? { ...existing, text: existing.text + delta } : block),
      itemRevisions: { ...state.itemRevisions, [itemId]: (state.itemRevisions[itemId] ?? 0) + 1 },
    }
  }
  const createdAt = assertPayloadString(event.payload, 'createdAt')
  if (!Number.isFinite(Date.parse(createdAt)))
    throw new Error('ai_invalid_stream_event_payload')
  const block: AIBlock = {
    id: itemId,
    turnId: event.turnId,
    runId: event.runId,
    index,
    type: 'message',
    role: 'assistant',
    status: 'streaming',
    text: delta,
    createdAt,
  }
  return {
    ...state,
    blocks: [...state.blocks, block].sort((left, right) => left.index - right.index),
    itemRevisions: { ...state.itemRevisions, [itemId]: 1 },
  }
}

function applyThinkingStarted(state: AIAssistantState, event: AIEvent, turnIndex: number): AIAssistantState {
  const { itemId, timelineIndex } = assertLiveItemIdentity(event)
  const summary = assertPayloadString(event.payload, 'summary')
  const display = event.payload.display
  const createdAt = assertPayloadString(event.payload, 'createdAt')
  if ((display !== 'summary' && display !== 'progress')
    || !Number.isFinite(Date.parse(createdAt))
    || state.blocks.some(block => block.id === itemId)) {
    throw new Error('ai_invalid_stream_event_payload')
  }
  const block: AIBlock = {
    id: itemId,
    turnId: event.turnId,
    runId: event.runId,
    index: blockIndex(turnIndex, timelineIndex),
    type: 'thinking',
    status: 'streaming',
    display,
    text: summary,
  }
  return {
    ...state,
    blocks: [...state.blocks, block].sort((left, right) => left.index - right.index),
    itemRevisions: { ...state.itemRevisions, [itemId]: 1 },
  }
}

function applyThinkingDelta(state: AIAssistantState, event: AIEvent, turnIndex: number): AIAssistantState {
  const { itemId, timelineIndex } = assertLiveItemIdentity(event)
  const delta = assertPayloadString(event.payload, 'delta')
  const display = event.payload.display
  const index = blockIndex(turnIndex, timelineIndex)
  const existing = state.blocks.find(block => block.id === itemId)
  if ((display !== 'summary' && display !== 'progress')
    || !existing
    || existing.type !== 'thinking'
    || existing.turnId !== event.turnId
    || existing.runId !== event.runId
    || existing.index !== index
    || existing.display !== display
    || existing.status !== 'streaming') {
    throw new Error('ai_invalid_stream_event_payload')
  }
  return {
    ...state,
    blocks: state.blocks.map(block => block.id === itemId ? { ...existing, text: existing.text + delta } : block),
    itemRevisions: { ...state.itemRevisions, [itemId]: (state.itemRevisions[itemId] ?? 0) + 1 },
  }
}

export function reduceAIEvent(state: AIAssistantState, event: AIEvent): AIAssistantState {
  if (event.version !== 2 || !event.eventId)
    return state
  const lastSequence = state.lastEventSequences[event.runId] ?? 0
  if (event.eventSequence <= lastSequence)
    return state
  if (event.eventSequence !== lastSequence + 1) {
    return {
      ...state,
      desyncedRunIds: new Set(state.desyncedRunIds).add(event.runId),
      desyncRecoverySequences: {
        ...state.desyncRecoverySequences,
        [event.runId]: Math.max(state.desyncRecoverySequences[event.runId] ?? 0, event.eventSequence),
      },
    }
  }

  const turnIndex = state.turnIndexes[event.turnId] ?? Math.max(-1, ...Object.values(state.turnIndexes)) + 1
  const desyncedRunIds = new Set(state.desyncedRunIds)
  const desyncRecoverySequences = { ...state.desyncRecoverySequences }
  const recoverySequence = desyncRecoverySequences[event.runId]
  if (recoverySequence !== undefined && event.eventSequence >= recoverySequence) {
    desyncedRunIds.delete(event.runId)
    delete desyncRecoverySequences[event.runId]
  }
  const next = applyAuthoritativeItem({
    ...state,
    desyncedRunIds,
    desyncRecoverySequences,
    lastEventSequences: { ...state.lastEventSequences, [event.runId]: event.eventSequence },
    turnIndexes: { ...state.turnIndexes, [event.turnId]: turnIndex },
  }, event, turnIndex)

  if (event.type === 'content.delta')
    return applyContentDelta(next, event, turnIndex)
  if (event.type === 'thinking.started')
    return applyThinkingStarted(next, event, turnIndex)
  if (event.type === 'thinking.delta')
    return applyThinkingDelta(next, event, turnIndex)

  if (event.type === 'run.started' || event.type === 'run.running' || event.type === 'run.queued' || event.type === 'run.waiting_approval' || event.type === 'run.waiting_input' || event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.canceled' || event.type === 'run.interrupted') {
    const status = event.type === 'run.started' || event.type === 'run.running' ? 'running' : event.type.slice(4)
    const terminalBlock = status === 'failed' || status === 'canceled' || status === 'interrupted'
      ? updateBlock(next, `${event.runId}:status`, block => block.type === 'run_status'
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
      : next.blocks
    return {
      ...next,
      blocks: terminalBlock,
      runStatuses: { ...next.runStatuses, [event.runId]: status },
      runExpectedVersions: typeof event.payload.expectedVersion === 'number'
        ? { ...next.runExpectedVersions, [event.runId]: event.payload.expectedVersion }
        : next.runExpectedVersions,
    }
  }
  if (event.type === 'model.completed') {
    const usage = event.payload.usage
    const usageRecord = usage && typeof usage === 'object' && !Array.isArray(usage)
      ? (usage as Record<string, unknown>)
      : undefined
    const status = usageRecord?.status
    const inputTokens = usageRecord?.inputTokens
    const outputTokens = usageRecord?.outputTokens
    const totalTokens = usageRecord?.totalTokens
    const modelId = event.payload.modelId
    const maxContextTokensSnapshot = event.payload.maxContextTokensSnapshot
    const hasReportedUsage = status === 'reported'
      && typeof inputTokens === 'number'
      && Number.isSafeInteger(inputTokens)
      && inputTokens >= 0
      && typeof outputTokens === 'number'
      && Number.isSafeInteger(outputTokens)
      && outputTokens >= 0
      && typeof totalTokens === 'number'
      && Number.isSafeInteger(totalTokens)
      && totalTokens === inputTokens + outputTokens
      && typeof modelId === 'string'
      && modelId.length > 0
      && typeof maxContextTokensSnapshot === 'number'
      && Number.isSafeInteger(maxContextTokensSnapshot)
      && maxContextTokensSnapshot > 0
    return {
      ...next,
      contextUsage: hasReportedUsage
        ? {
            status: 'reported',
            runId: event.runId,
            modelId,
            usedTokens: totalTokens,
            maxContextTokensSnapshot,
            recordedAt: event.occurredAt,
          }
        : next.contextUsage,
      runUsage: {
        ...next.runUsage,
        [event.runId]: hasReportedUsage
          ? { status: 'reported', promptTokens: inputTokens, modelId, maxContextTokensSnapshot }
          : { status: status === 'reconciliation_required' ? 'reconciliation_required' : 'unavailable' },
      },
    }
  }
  if (event.type === 'approval.required' || event.type === 'approval.resolved') {
    const required = event.type.endsWith('.required')
    return {
      ...next,
      runStatuses: {
        ...next.runStatuses,
        [event.runId]: required ? 'waiting_approval' : 'running',
      },
    }
  }
  return next
}

export const emptyAIAssistantState: AIAssistantState = {
  blocks: [],
  runStatuses: {},
  runExpectedVersions: {},
  lastEventSequences: {},
  turnIndexes: {},
  itemRevisions: {},
  desyncedRunIds: new Set(),
  desyncRecoverySequences: {},
  runUsage: {},
  contextUsage: undefined,
}
