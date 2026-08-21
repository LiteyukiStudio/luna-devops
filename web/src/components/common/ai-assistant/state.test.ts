import type { AIEvent, AITimeline, AITimelineItem } from '@/api'
import { describe, expect, it } from 'vitest'
import { addOptimisticTurn, emptyAIAssistantState, isValidAITimeline, mergeTimelineSnapshot, reduceAIEvent, stateFromTimeline } from './state'

function event(overrides: Partial<AIEvent>): AIEvent {
  return {
    version: 2,
    eventId: 'event-1',
    eventSequence: 1,
    type: 'content.delta',
    conversationId: 'conversation-1',
    turnId: 'turn-1',
    runId: 'run-1',
    itemId: 'item-1',
    occurredAt: '2026-07-28T00:00:00Z',
    payload: { delta: 'hello' },
    item: messageItem(),
    ...overrides,
  }
}

function messageItem(overrides: Partial<AITimelineItem> = {}): AITimelineItem {
  return {
    id: 'item-1',
    timelineIndex: 0,
    revision: 1,
    createdAt: '2026-07-28T00:00:00Z',
    type: 'assistant_message',
    status: 'streaming',
    parts: [{ id: 'item-1:0', partIndex: 0, type: 'text', text: 'hello' }],
    ...overrides,
  }
}

function toolItem(overrides: Partial<AITimelineItem> = {}): AITimelineItem {
  return {
    id: 'tool-item',
    timelineIndex: 0,
    revision: 1,
    createdAt: '2026-07-28T00:00:00Z',
    type: 'tool_call',
    status: 'streaming',
    parts: [],
    toolCall: { id: 'tool-1', operationId: 'listApplications', callIndex: 0, status: 'running', arguments: {} },
    ...overrides,
  }
}

describe('aI assistant state', () => {
  it('rejects incomplete timeline snapshots fail closed', () => {
    expect(isValidAITimeline({ conversation: { id: 'c' }, turns: [] })).toBe(false)
    expect(isValidAITimeline({ conversation: { id: 'c' }, turns: [], eventCursors: [], pageInfo: { hasOlder: false } })).toBe(true)
  })

  it('merges streaming deltas and ignores duplicate or stale events', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({}))
    const duplicate = reduceAIEvent(first, event({}))
    const second = reduceAIEvent(duplicate, event({
      eventId: 'event-2',
      eventSequence: 2,
      payload: { delta: ' world' },
      item: messageItem({ revision: 2, parts: [{ id: 'item-1:0', partIndex: 0, type: 'text', text: 'hello world' }] }),
    }))
    expect(duplicate).toBe(first)
    expect(second.blocks[0]).toMatchObject({ text: 'hello world' })
  })

  it('keeps a newly streamed turn after older turns by its stable turn and timeline indexes', () => {
    const older = addOptimisticTurn(emptyAIAssistantState, {
      turnId: 'turn-older',
      turnIndex: 3,
      runId: 'run-older',
      text: 'older',
    })
    const optimistic = addOptimisticTurn(older, {
      turnId: 'turn-new',
      turnIndex: 4,
      runId: 'run-new',
      text: 'new',
    })
    const streamed = reduceAIEvent(optimistic, event({
      eventId: 'new-delta',
      eventSequence: 1,
      turnId: 'turn-new',
      runId: 'run-new',
      itemId: 'new-answer',
      payload: { delta: 'answer', timelineIndex: 0 },
      item: messageItem({ id: 'new-answer', parts: [{ id: 'new-answer:0', partIndex: 0, type: 'text', text: 'answer' }] }),
    }))
    expect(streamed.blocks.map(block => block.id)).toEqual([
      'turn-older:input',
      'turn-new:input',
      'new-answer',
    ])
  })

  it('does not regress a newer live item revision when a delayed timeline snapshot arrives', () => {
    const live = reduceAIEvent(addOptimisticTurn(emptyAIAssistantState, {
      turnId: 'turn',
      turnIndex: 0,
      runId: 'run',
      text: 'question',
    }), event({
      itemId: 'answer',
      payload: { delta: 'streamed answer', timelineIndex: 0 },
      item: messageItem({ id: 'answer', revision: 2, parts: [{ id: 'answer:0', partIndex: 0, type: 'text', text: 'streamed answer' }] }),
    }))
    const snapshot: AITimeline = {
      pageInfo: { hasOlder: false },
      conversation: { id: 'conversation-1', title: 't', titleSource: 'assistant', status: 'active' },
      eventCursors: [{ runId: 'run', after: 0 }],
      turns: [{
        id: 'turn',
        turnIndex: 0,
        status: 'running',
        input: { id: 'turn:input', type: 'user_message', createdAt: '2026-07-28T00:00:00Z', parts: [{ id: 'input', partIndex: 0, type: 'text', text: 'question' }] },
        selectedRun: {
          id: 'run',
          runIndex: 0,
          status: 'running',
          items: [{ id: 'answer', timelineIndex: 0, revision: 1, createdAt: '2026-07-28T00:00:01Z', type: 'assistant_message', status: 'streaming', parts: [{ id: 'answer:0', partIndex: 0, type: 'text', text: 'streamed' }] }],
        },
      }],
    }
    expect(mergeTimelineSnapshot(live, snapshot).blocks.at(-1)).toMatchObject({ text: 'streamed answer' })
  })

  it('maps a started run to the active running state', () => {
    const state = reduceAIEvent(emptyAIAssistantState, event({ type: 'run.started', item: undefined }))
    expect(state.runStatuses['run-1']).toBe('running')
  })

  it('renders a stable failure notice from a terminal SSE event', () => {
    const state = reduceAIEvent(emptyAIAssistantState, event({
      type: 'run.failed',
      item: undefined,
      payload: { errorCode: 'ai.provider_quota_exhausted' },
    }))
    expect(state.runStatuses['run-1']).toBe('failed')
    expect(state.blocks[0]).toMatchObject({
      id: 'run-1:status',
      type: 'run_status',
      status: 'failed',
      errorCode: 'ai.provider_quota_exhausted',
    })
  })

  it('renders and preserves an interrupted terminal state', () => {
    const state = reduceAIEvent(emptyAIAssistantState, event({
      type: 'run.interrupted',
      item: undefined,
      payload: {},
    }))
    expect(state.runStatuses['run-1']).toBe('interrupted')
    expect(state.blocks[0]).toMatchObject({
      id: 'run-1:status',
      type: 'run_status',
      status: 'interrupted',
    })
  })

  it('restores a failed run notice from a timeline snapshot', () => {
    const timeline: AITimeline = {
      pageInfo: { hasOlder: false },
      conversation: { id: 'c', title: 't', titleSource: 'assistant', status: 'active' },
      eventCursors: [],
      turns: [{
        id: 'turn',
        turnIndex: 0,
        status: 'failed',
        input: { id: 'input', type: 'user_message', createdAt: '2026-07-28T00:00:00Z', parts: [{ id: 'p', partIndex: 0, type: 'text', text: 'check' }] },
        selectedRun: {
          id: 'run',
          runIndex: 0,
          status: 'failed',
          errorCode: 'ai.provider_unavailable',
          items: [],
        },
      }],
    }
    expect(stateFromTimeline(timeline).blocks.at(-1)).toMatchObject({
      type: 'run_status',
      status: 'failed',
      errorCode: 'ai.provider_unavailable',
    })
  })

  it('projects tool result into its tool call without a duplicate block', () => {
    const timeline: AITimeline = {
      pageInfo: { hasOlder: false },
      conversation: { id: 'c', title: 't', titleSource: 'assistant', status: 'active' },
      eventCursors: [],
      turns: [{
        id: 'turn',
        turnIndex: 0,
        status: 'completed',
        input: { id: 'input', type: 'user_message', createdAt: '2026-07-28T00:00:00Z', parts: [{ id: 'p', partIndex: 0, type: 'text', text: 'check' }] },
        selectedRun: {
          id: 'run',
          runIndex: 0,
          status: 'completed',
          items: [
            { id: 'call', timelineIndex: 0, revision: 1, createdAt: '2026-07-28T00:00:01Z', type: 'tool_call', status: 'completed', parts: [], toolCall: { id: 'tool', operationId: 'runtime.read', callIndex: 0 } },
            { id: 'result', timelineIndex: 1, revision: 1, createdAt: '2026-07-28T00:00:02Z', type: 'tool_result', status: 'completed', relatedItemId: 'call', parts: [{ id: 'r', partIndex: 0, type: 'structured_data', data: { summaryKey: 'aiAssistant.resultAvailable' } }] },
          ],
        },
      }],
    }
    const projected = stateFromTimeline(timeline)
    expect(projected.blocks).toHaveLength(2)
    expect(projected.blocks[1]).toMatchObject({ type: 'tool_call', result: { summaryKey: 'aiAssistant.resultAvailable' } })
  })

  it('projects a dangerous tool confirmation without the removed hash and row-version protocol', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({
      type: 'tool.started',
      toolCallId: 'tool-1',
      payload: { operationId: 'restartDeploymentTarget', arguments: { targetId: 'target-1' } },
      item: toolItem({
        toolCall: { id: 'tool-1', operationId: 'restartDeploymentTarget', callIndex: 0, status: 'running', arguments: { targetId: 'target-1' } },
      }),
    }))
    const approval = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'approval.required',
      toolCallId: 'tool-1',
      payload: {},
      item: toolItem({
        revision: 2,
        toolCall: { id: 'tool-1', operationId: 'restartDeploymentTarget', callIndex: 0, status: 'awaiting_approval', arguments: { targetId: 'target-1' } },
      }),
    }))
    expect(approval.blocks[0]).toMatchObject({
      type: 'tool_call',
      status: 'awaiting_approval',
      arguments: { targetId: 'target-1' },
    })
    expect(approval.runStatuses['run-1']).toBe('waiting_approval')
    expect(approval.runExpectedVersions['run-1']).toBeUndefined()
  })

  it('replaces a running card creation block with the completed card in place', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({
      type: 'tool.started',
      toolCallId: 'card-generation',
      payload: {
        operationId: 'create_interaction_cards',
        arguments: { schemaVersion: 1, generationId: 'database-list', title: '正在整理数据库候选' },
        timelineIndex: 2,
      },
      item: toolItem({
        id: 'card-item',
        timelineIndex: 2,
        toolCall: { id: 'card-generation', operationId: 'create_interaction_cards', callIndex: 2, status: 'running', arguments: { schemaVersion: 1, generationId: 'database-list', title: '正在整理数据库候选' } },
      }),
    }))
    const completed = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'tool.completed',
      toolCallId: 'card-generation',
      payload: {
        operationId: 'create_interaction_cards',
        arguments: {
          schemaVersion: 1,
          generationId: 'database-list',
          title: '数据库候选',
          template: 'candidates',
          cards: [],
        },
        timelineIndex: 2,
      },
      item: toolItem({
        id: 'card-item',
        timelineIndex: 2,
        revision: 2,
        status: 'completed',
        toolCall: {
          id: 'card-generation',
          operationId: 'create_interaction_cards',
          callIndex: 2,
          status: 'succeeded',
          arguments: { schemaVersion: 1, generationId: 'database-list', title: '数据库候选', template: 'candidates', cards: [] },
        },
      }),
    }))

    expect(completed.blocks).toHaveLength(1)
    expect(completed.blocks[0]).toMatchObject({
      type: 'tool_call',
      operationId: 'create_interaction_cards',
      status: 'succeeded',
      arguments: expect.objectContaining({ generationId: 'database-list', title: '数据库候选' }),
    })
  })

  it('retains a failed tool error code and request id from the live event', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({
      type: 'tool.started',
      toolCallId: 'tool-1',
      payload: { operationId: 'listApplications', arguments: { projectId: 'prj_1' } },
      item: toolItem({ toolCall: { id: 'tool-1', operationId: 'listApplications', callIndex: 0, status: 'running', arguments: { projectId: 'prj_1' } } }),
    }))
    const failed = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'tool.failed',
      toolCallId: 'tool-1',
      payload: {
        errorCode: 'ai.tool_storage_unavailable',
        result: {
          code: 'ai.tool_storage_unavailable',
          requestId: 'req_tool_failure',
        },
      },
      item: toolItem({
        revision: 2,
        status: 'failed',
        toolCall: {
          id: 'tool-1',
          operationId: 'listApplications',
          callIndex: 0,
          status: 'failed',
          arguments: { projectId: 'prj_1' },
          durationMs: 234,
          traceId: '717690e2661f8337d53fcd3295591b4b',
          errorCode: 'ai.tool_storage_unavailable',
          result: { summaryKey: 'ai.tool.result.failed', errorCode: 'ai.tool_storage_unavailable', requestId: 'req_tool_failure' },
        },
      }),
    }))

    expect(failed.blocks[0]).toMatchObject({
      type: 'tool_call',
      status: 'failed',
      errorCode: 'ai.tool_storage_unavailable',
      durationMs: 234,
      traceId: '717690e2661f8337d53fcd3295591b4b',
      result: {
        requestId: 'req_tool_failure',
      },
    })
  })

  it('keeps message, tool and follow-up message in authoritative item order during streaming', () => {
    const firstMessage = reduceAIEvent(emptyAIAssistantState, event({
      item: messageItem({ id: 'message-1', timelineIndex: 0, revision: 1, parts: [{ id: 'message-1:0', partIndex: 0, type: 'text', text: '先检查资源。' }] }),
    }))
    const completedMessage = reduceAIEvent(firstMessage, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'message.completed',
      payload: {},
      item: messageItem({ id: 'message-1', timelineIndex: 0, revision: 2, status: 'completed', parts: [{ id: 'message-1:0', partIndex: 0, type: 'text', text: '先检查资源。' }] }),
    }))
    const tool = reduceAIEvent(completedMessage, event({
      eventId: 'event-3',
      eventSequence: 3,
      type: 'tool.completed',
      payload: {},
      item: toolItem({ id: 'tool-item', timelineIndex: 1, status: 'completed', toolCall: { id: 'tool-1', operationId: 'listProjects', callIndex: 1, status: 'succeeded', arguments: {} } }),
    }))
    const followUp = reduceAIEvent(tool, event({
      eventId: 'event-4',
      eventSequence: 4,
      itemId: 'message-2',
      payload: { delta: '接下来检查部署。' },
      item: messageItem({ id: 'message-2', timelineIndex: 2, revision: 1, parts: [{ id: 'message-2:0', partIndex: 0, type: 'text', text: '接下来检查部署。' }] }),
    }))

    expect(followUp.blocks.map(block => block.id)).toEqual(['message-1', 'tool-item', 'message-2'])
  })

  it('stops projection and requests a snapshot when an event sequence has a gap', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({}))
    const gap = reduceAIEvent(first, event({
      eventId: 'event-3',
      eventSequence: 3,
      item: messageItem({ revision: 3, parts: [{ id: 'item-1:0', partIndex: 0, type: 'text', text: 'must not apply before recovery' }] }),
    }))

    expect(gap.blocks[0]).toMatchObject({ text: 'hello' })
    expect(gap.desyncedRunIds.has('run-1')).toBe(true)
    expect(gap.lastEventSequences['run-1']).toBe(1)
  })

  it('records provider-reported input tokens from model.completed usage', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({ eventId: 'event-1', eventSequence: 1, type: 'run.started', item: undefined, payload: {} }))
    const completed = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { inputTokens: 25600, outputTokens: 512, cachedInputTokens: 0, cachedOutputTokens: 0 } },
    }))
    expect(completed.runUsage['run-1']?.latestInputTokens).toBe(25600)

    const withoutUsage = reduceAIEvent(completed, event({
      eventId: 'event-3',
      eventSequence: 3,
      type: 'model.completed',
      item: undefined,
      payload: {},
    }))
    expect(withoutUsage.runUsage['run-1']?.latestInputTokens).toBe(25600)
  })

  it('accumulates run token usage across model.completed events', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({ eventId: 'event-1', eventSequence: 1, type: 'run.started', item: undefined, payload: {} }))
    const first = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { inputTokens: 1000, outputTokens: 500 } },
    }))
    expect(first.runUsage['run-1']?.usedTokens).toBe(1500)
    const second = reduceAIEvent(first, event({
      eventId: 'event-3',
      eventSequence: 3,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { inputTokens: 200, outputTokens: 300 } },
    }))
    expect(second.runUsage['run-1']?.usedTokens).toBe(2000)
  })

  it('captures the token budget snapshot from run.started', () => {
    const state = reduceAIEvent(emptyAIAssistantState, event({
      eventId: 'event-1',
      eventSequence: 1,
      type: 'run.started',
      item: undefined,
      payload: { budget: { totalTokens: 2_000_000, totalCredits: '10000' } },
    }))
    expect(state.runUsage['run-1']?.tokenBudget).toBe(2_000_000)
  })

  it('isolates token usage between runs', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({
      eventId: 'event-1',
      eventSequence: 1,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { inputTokens: 1000, outputTokens: 200 } },
    }))
    const nextRun = reduceAIEvent(first, event({
      eventId: 'event-next-1',
      eventSequence: 1,
      runId: 'run-2',
      turnId: 'turn-2',
      type: 'run.started',
      item: undefined,
      payload: { budget: { totalTokens: 500_000, totalCredits: '1000' } },
    }))

    expect(nextRun.runUsage['run-1']).toMatchObject({ latestInputTokens: 1000, usedTokens: 1200 })
    expect(nextRun.runUsage['run-2']).toEqual({ usedTokens: 0, tokenBudget: 500_000 })
  })
})
