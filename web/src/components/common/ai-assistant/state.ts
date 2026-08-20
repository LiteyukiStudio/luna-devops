import type { AIToolVisibility } from '@luna-devops/ai-interaction-card-contract'
import type { AIEvent, AIMessagePart, AITimeline, AITimelineItem, AIToolDisplayResult, AIToolStatus, AIUIAction } from '@/api'

export type AIBlock
  = | { id: string, turnId: string, index: number, type: 'thinking', status: string, display: 'summary' | 'progress', text: string }
    | { id: string, turnId: string, index: number, type: 'message', role: 'user' | 'assistant', status: string, text: string, createdAt: string }
    | { id: string, turnId: string, runId: string, index: number, type: 'run_status', status: 'failed' | 'canceled', errorCode?: string }
    | { id: string, turnId: string, index: number, type: 'context_compacted', status: string }
    | { id: string, turnId: string, runId: string, index: number, type: 'tool_call', toolCallId: string, operationId: string, visibility: AIToolVisibility, titleKey?: string, errorCode?: string, status: AIToolStatus, arguments: Record<string, unknown>, result?: AIToolDisplayResult, uiActions: AIUIAction[], durationMs?: number, traceId?: string, argumentsHash?: string, expectedVersion?: number, mfaPurpose?: string }

export interface AIRunUsage {
  /** 最近一次主回答模型调用由 Provider 返回的输入 Token 数。 */
  latestInputTokens?: number
  /** 当前 Run 累计消耗的输入与输出 Token 数。 */
  usedTokens?: number
  /** 当前 Run 的累计 Token 预算上限。 */
  tokenBudget?: number
}

export interface AIAssistantState {
  blocks: AIBlock[]
  seenEventIds: Set<string>
  runStatuses: Record<string, string>
  runExpectedVersions: Record<string, number>
  lastEventSequences: Record<string, number>
  turnIndexes: Record<string, number>
  itemRevisions: Record<string, number>
  desyncedRunIds: Set<string>
  desyncRecoverySequences: Record<string, number>
  /** 用量必须按 Run 隔离，避免新一轮继承上一轮的上下文或累计预算。 */
  runUsage: Record<string, AIRunUsage>
}

const TURN_ORDER_STRIDE = 1_000_000

function blockIndex(turnIndex: number, itemIndex: number): number {
  return turnIndex * TURN_ORDER_STRIDE + itemIndex
}

function isOptionalTokenCount(value: unknown) {
  return value === undefined || (typeof value === 'number' && Number.isSafeInteger(value) && value >= 0)
}

function isValidRunBudget(value: unknown) {
  if (value === undefined)
    return true
  if (!value || typeof value !== 'object' || Array.isArray(value))
    return false
  const budget = value as Record<string, unknown>
  return typeof budget.totalTokens === 'number'
    && Number.isSafeInteger(budget.totalTokens)
    && budget.totalTokens > 0
    && typeof budget.totalCredits === 'string'
}

export function isValidAITimeline(value: unknown): value is AITimeline {
  if (!value || typeof value !== 'object')
    return false
  const candidate = value as Partial<AITimeline>
  if (!candidate.conversation || typeof candidate.conversation.id !== 'string' || !Array.isArray(candidate.turns) || !Array.isArray(candidate.eventCursors)
    || !candidate.pageInfo || typeof candidate.pageInfo.hasOlder !== 'boolean'
    || (candidate.pageInfo.olderCursor !== undefined && typeof candidate.pageInfo.olderCursor !== 'string')) {
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
      && isOptionalTokenCount(turn.selectedRun.usedTokens)
      && isOptionalTokenCount(turn.selectedRun.latestInputTokens)
      && isValidRunBudget(turn.selectedRun.budget)
      && Array.isArray(turn.selectedRun.items)
      && turn.selectedRun.items.every(item => Boolean(item) && typeof item.id === 'string' && typeof item.timelineIndex === 'number' && typeof item.revision === 'number' && typeof item.createdAt === 'string' && Array.isArray(item.parts)))),
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
    const results = new Map(turn.selectedRun?.items.filter(item => item.type === 'tool_result' && item.relatedItemId).map(item => [item.relatedItemId!, item]) ?? [])
    for (const item of [...(turn.selectedRun?.items ?? [])].sort((a, b) => a.timelineIndex - b.timelineIndex)) {
      if (item.type === 'assistant_message') {
        blocks.push({ id: item.id, turnId: turn.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'message', role: 'assistant', status: item.status, text: textFromParts(item.parts), createdAt: item.createdAt })
      }
      else if (item.type === 'reasoning_summary' || item.type === 'progress') {
        blocks.push({ id: item.id, turnId: turn.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'thinking', status: item.status, display: item.type === 'progress' ? 'progress' : 'summary', text: textFromParts(item.parts) })
      }
      else if (item.type === 'system_notice' && item.notice === 'context_compacted') {
        blocks.push({ id: item.id, turnId: turn.id, index: blockIndex(turn.turnIndex, item.timelineIndex), type: 'context_compacted', status: item.status })
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
          visibility: item.toolCall.visibility ?? 'normal',
          titleKey: item.toolCall.titleKey,
          errorCode: item.toolCall.errorCode,
          status: item.toolCall.status ?? (item.status === 'completed' ? 'succeeded' : 'running'),
          arguments: item.toolCall.arguments ?? {},
          result: normalizeToolResult(item.toolCall.result ?? structuredResult),
          uiActions: item.toolCall.uiActions ?? [],
          durationMs: item.toolCall.durationMs,
          traceId: item.toolCall.traceId,
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
    itemRevisions: Object.fromEntries(timeline.turns.flatMap(turn => turn.selectedRun?.items.map(item => [item.id, item.revision]) ?? [])),
    desyncedRunIds: new Set(),
    desyncRecoverySequences: {},
    runUsage: Object.fromEntries(timeline.turns.flatMap((turn) => {
      const run = turn.selectedRun
      if (!run)
        return []
      return [[run.id, {
        ...(run.latestInputTokens !== undefined ? { latestInputTokens: run.latestInputTokens } : {}),
        ...(run.usedTokens !== undefined ? { usedTokens: run.usedTokens } : {}),
        ...(run.budget ? { tokenBudget: run.budget.totalTokens } : {}),
      }]]
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
    const snapshotUsage = snapshot[runId] ?? {}
    const currentUsage = current[runId] ?? {}
    const currentAhead = (currentSequences[runId] ?? 0) > (snapshotSequences[runId] ?? 0)
    const preferred = currentAhead ? currentUsage : snapshotUsage
    const fallback = currentAhead ? snapshotUsage : currentUsage
    const usedTokens = Math.max(snapshotUsage.usedTokens ?? 0, currentUsage.usedTokens ?? 0)
    return [runId, {
      ...(preferred.latestInputTokens !== undefined
        ? { latestInputTokens: preferred.latestInputTokens }
        : fallback.latestInputTokens !== undefined ? { latestInputTokens: fallback.latestInputTokens } : {}),
      ...(snapshotUsage.usedTokens !== undefined || currentUsage.usedTokens !== undefined ? { usedTokens } : {}),
      ...(preferred.tokenBudget !== undefined
        ? { tokenBudget: preferred.tokenBudget }
        : fallback.tokenBudget !== undefined ? { tokenBudget: fallback.tokenBudget } : {}),
    }]
  }))
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
    return (current.itemRevisions[block.id] ?? 0) > (snapshot.itemRevisions[block.id] ?? 0) ? live : block
  })
  current.blocks
    .filter(block => block.type === 'message'
      && block.role === 'user'
      && (current.itemRevisions[block.id] ?? 0) === 0
      && !snapshotIds.has(block.id))
    .forEach(block => blocks.push(block))
  return {
    ...snapshot,
    blocks: blocks.sort((a, b) => a.index - b.index),
    seenEventIds: current.seenEventIds,
    lastEventSequences: Object.fromEntries([...new Set([...Object.keys(snapshot.lastEventSequences), ...Object.keys(current.lastEventSequences)])]
      .map(runId => [runId, Math.max(snapshot.lastEventSequences[runId] ?? 0, current.lastEventSequences[runId] ?? 0)])),
    runStatuses: mergeRunStatuses(snapshot.runStatuses, current.runStatuses),
    runExpectedVersions: { ...snapshot.runExpectedVersions, ...current.runExpectedVersions },
    turnIndexes: { ...snapshot.turnIndexes, ...current.turnIndexes },
    itemRevisions: Object.fromEntries([...new Set([...Object.keys(current.itemRevisions), ...Object.keys(snapshot.itemRevisions)])]
      .map(itemId => [itemId, Math.max(current.itemRevisions[itemId] ?? 0, snapshot.itemRevisions[itemId] ?? 0)])),
    runUsage: mergeRunUsage(snapshot.runUsage, current.runUsage, snapshot.lastEventSequences, current.lastEventSequences),
    desyncedRunIds: new Set([...current.desyncedRunIds].filter(runId =>
      (snapshot.lastEventSequences[runId] ?? 0) < (current.desyncRecoverySequences[runId] ?? Number.MAX_SAFE_INTEGER))),
    desyncRecoverySequences: Object.fromEntries(Object.entries(current.desyncRecoverySequences).filter(([runId, sequence]) =>
      (snapshot.lastEventSequences[runId] ?? 0) < sequence)),
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
      createdAt: new Date().toISOString(),
    }].sort((a, b) => a.index - b.index),
    runStatuses: { ...state.runStatuses, [input.runId]: 'queued' },
    runUsage: { ...state.runUsage, [input.runId]: { usedTokens: 0 } },
    turnIndexes: { ...state.turnIndexes, [input.turnId]: input.turnIndex },
    itemRevisions: { ...state.itemRevisions, [`${input.turnId}:input`]: 0 },
  }
}

function stringPayload(payload: Record<string, unknown>, key: string) {
  return typeof payload[key] === 'string' ? payload[key] : ''
}

// budgetTotalTokens extracts the total token budget from a run.started payload.
function budgetTotalTokens(payload: Record<string, unknown>): number | undefined {
  const budget = payload.budget
  if (!budget || typeof budget !== 'object' || Array.isArray(budget))
    return undefined
  const total = (budget as Record<string, unknown>).totalTokens
  return typeof total === 'number' && Number.isFinite(total) && total > 0 ? total : undefined
}

function updateBlock(state: AIAssistantState, id: string, update: (block: AIBlock) => AIBlock, create?: () => AIBlock) {
  const blockIndex = state.blocks.findIndex(block => block.id === id || (block.type === 'tool_call' && block.toolCallId === id))
  if (blockIndex < 0)
    return create ? [...state.blocks, create()].sort((a, b) => a.index - b.index) : state.blocks
  return state.blocks.map((block, index) => index === blockIndex ? update(block) : block).sort((a, b) => a.index - b.index)
}

function blockFromTimelineItem(item: AITimelineItem, turnId: string, runId: string, turnIndex: number, expectedVersion?: number): AIBlock | undefined {
  const index = blockIndex(turnIndex, item.timelineIndex)
  if (item.type === 'assistant_message') {
    return {
      id: item.id,
      turnId,
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
    return { id: item.id, turnId, index, type: 'context_compacted', status: item.status }
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
    result: normalizeToolResult(item.toolCall.result),
    uiActions: item.toolCall.uiActions ?? [],
    durationMs: item.toolCall.durationMs,
    traceId: item.toolCall.traceId,
    argumentsHash: item.toolCall.argumentsHash,
    expectedVersion: item.toolCall.expectedVersion ?? expectedVersion,
    mfaPurpose: item.toolCall.mfaPurpose,
  }
}

function applyAuthoritativeItem(state: AIAssistantState, event: AIEvent, turnIndex: number): AIAssistantState {
  const item = event.item
  if (!item || item.revision <= (state.itemRevisions[item.id] ?? 0))
    return state
  const block = blockFromTimelineItem(item, event.turnId, event.runId, turnIndex, state.runExpectedVersions[event.runId])
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

export function reduceAIEvent(state: AIAssistantState, event: AIEvent): AIAssistantState {
  if (event.version !== 2 || !event.eventId || state.seenEventIds.has(event.eventId))
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
  const next = applyAuthoritativeItem({
    ...state,
    seenEventIds: new Set(state.seenEventIds).add(event.eventId),
    lastEventSequences: { ...state.lastEventSequences, [event.runId]: event.eventSequence },
    turnIndexes: { ...state.turnIndexes, [event.turnId]: turnIndex },
  }, event, turnIndex)

  if (event.type === 'run.started' || event.type === 'run.running' || event.type === 'run.queued' || event.type === 'run.waiting_approval' || event.type === 'run.waiting_mfa' || event.type === 'run.waiting_input' || event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.canceled') {
    const status = event.type === 'run.started' || event.type === 'run.running' ? 'running' : event.type.slice(4)
    const tokenBudget = budgetTotalTokens(event.payload)
    const terminalBlock = status === 'failed' || status === 'canceled'
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
      runUsage: {
        ...next.runUsage,
        [event.runId]: {
          ...(next.runUsage[event.runId] ?? { usedTokens: 0 }),
          ...(tokenBudget !== undefined ? { tokenBudget } : {}),
        },
      },
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
    const inputTokens = usageRecord?.inputTokens
    const outputTokens = usageRecord?.outputTokens
    const hasInput = typeof inputTokens === 'number' && Number.isFinite(inputTokens)
    const hasOutput = typeof outputTokens === 'number' && Number.isFinite(outputTokens)
    if (!hasInput && !hasOutput)
      return next
    const delta = (hasInput ? inputTokens : 0) + (hasOutput ? outputTokens : 0)
    const currentUsage = next.runUsage[event.runId] ?? {}
    return {
      ...next,
      runUsage: {
        ...next.runUsage,
        [event.runId]: {
          ...currentUsage,
          ...(hasInput ? { latestInputTokens: inputTokens } : {}),
          usedTokens: (currentUsage.usedTokens ?? 0) + delta,
        },
      },
    }
  }
  if (event.type === 'approval.required' || event.type === 'approval.resolved' || event.type === 'mfa.required' || event.type === 'mfa.resolved') {
    const required = event.type.endsWith('.required')
    return {
      ...next,
      runStatuses: {
        ...next.runStatuses,
        [event.runId]: required ? (event.type.startsWith('approval') ? 'waiting_approval' : 'waiting_mfa') : 'running',
      },
      runExpectedVersions: typeof event.payload.expectedVersion === 'number'
        ? { ...next.runExpectedVersions, [event.runId]: event.payload.expectedVersion }
        : next.runExpectedVersions,
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
    ...(typeof result.errorCode === 'string' ? { errorCode: result.errorCode } : {}),
    ...(typeof result.errorMessage === 'string' ? { errorMessage: result.errorMessage } : {}),
    ...(typeof result.generationId === 'string' ? { generationId: result.generationId } : {}),
    ...(typeof result.attempt === 'number' ? { attempt: result.attempt } : {}),
    ...(typeof result.maxAttempts === 'number' ? { maxAttempts: result.maxAttempts } : {}),
    ...('data' in result ? { data: result.data } : {}),
    ...(Array.isArray(result.issues)
      ? {
          issues: result.issues
            .filter(issue => issue && typeof issue === 'object' && !Array.isArray(issue))
            .map((issue) => {
              const candidate = issue as Record<string, unknown>
              return {
                code: typeof candidate.code === 'string' ? candidate.code : 'invalid',
                path: typeof candidate.path === 'string' ? candidate.path : '',
                message: typeof candidate.message === 'string' ? candidate.message : '',
                ...(typeof candidate.expected === 'string' ? { expected: candidate.expected } : {}),
                ...(typeof candidate.received === 'string' ? { received: candidate.received } : {}),
              }
            }),
        }
      : {}),
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
  itemRevisions: {},
  desyncedRunIds: new Set(),
  desyncRecoverySequences: {},
  runUsage: {},
}
