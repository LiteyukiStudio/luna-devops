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
    contentPartId: 'item-1:0',
    occurredAt: '2026-07-28T00:00:00Z',
    payload: {
      itemId: 'item-1',
      contentPartId: 'item-1:0',
      partIndex: 0,
      delta: 'hello',
      timelineIndex: 0,
      createdAt: '2026-07-28T00:00:00Z',
    },
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
    expect(isValidAITimeline({
      conversation: { id: 'c' },
      eventCursors: [],
      pageInfo: { hasOlder: false },
      turns: [{
        id: 'turn',
        turnIndex: 0,
        input: messageItem(),
        selectedRun: {
          id: 'run',
          items: [toolItem({ toolCall: { id: 'tool', operationId: 'runtime.read', callIndex: 0, result: {} as never } })],
        },
      }],
    })).toBe(false)
  })

  it('merges streaming deltas and ignores duplicate or stale events', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({}))
    const duplicate = reduceAIEvent(first, event({}))
    const second = reduceAIEvent(duplicate, event({
      eventId: 'event-2',
      eventSequence: 2,
      payload: { itemId: 'item-1', contentPartId: 'item-1:0', partIndex: 0, delta: ' world', timelineIndex: 0 },
    }))
    expect(duplicate).toBe(first)
    expect(second.blocks[0]).toMatchObject({ text: 'hello world' })
  })

  it('projects real message and thinking delta frames before authoritative completion overwrites them', () => {
    const thinkingStarted = reduceAIEvent(emptyAIAssistantState, event({
      type: 'thinking.started',
      itemId: 'thinking-1',
      payload: { itemId: 'thinking-1', summary: '先分析', display: 'summary', timelineIndex: 0, createdAt: '2026-07-28T00:00:00Z' },
    }))
    const thinkingDelta = reduceAIEvent(thinkingStarted, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'thinking.delta',
      itemId: 'thinking-1',
      payload: { itemId: 'thinking-1', delta: '再检查', display: 'summary', timelineIndex: 0 },
    }))
    const thinkingCompleted = reduceAIEvent(thinkingDelta, event({
      eventId: 'event-3',
      eventSequence: 3,
      type: 'thinking.completed',
      itemId: 'thinking-1',
      payload: { itemId: 'thinking-1', display: 'summary', timelineIndex: 0 },
      item: messageItem({
        id: 'thinking-1',
        timelineIndex: 0,
        revision: 3,
        type: 'reasoning_summary',
        status: 'completed',
        display: 'summary',
        parts: [{ id: 'thinking-1:0', partIndex: 0, type: 'text', text: '权威思考摘要' }],
      }),
    }))
    const messageStarted = reduceAIEvent(thinkingCompleted, event({
      eventId: 'event-4',
      eventSequence: 4,
      itemId: 'message-1',
      contentPartId: 'message-1:0',
      payload: { itemId: 'message-1', contentPartId: 'message-1:0', partIndex: 0, delta: 'Hel', timelineIndex: 1, createdAt: '2026-07-28T00:00:01Z' },
    }))
    const messageDelta = reduceAIEvent(messageStarted, event({
      eventId: 'event-5',
      eventSequence: 5,
      itemId: 'message-1',
      contentPartId: 'message-1:0',
      payload: { itemId: 'message-1', contentPartId: 'message-1:0', partIndex: 0, delta: 'lo', timelineIndex: 1 },
    }))
    const completed = reduceAIEvent(messageDelta, event({
      eventId: 'event-6',
      eventSequence: 6,
      type: 'message.completed',
      itemId: 'message-1',
      contentPartId: 'message-1:0',
      payload: { itemId: 'message-1', contentPartId: 'message-1:0', partIndex: 0, timelineIndex: 1 },
      item: messageItem({
        id: 'message-1',
        timelineIndex: 1,
        revision: 3,
        status: 'completed',
        parts: [{ id: 'message-1:0', partIndex: 0, type: 'text', text: 'Hello!' }],
      }),
    }))

    expect(thinkingDelta.blocks[0]).toMatchObject({ type: 'thinking', status: 'streaming', text: '先分析再检查' })
    expect(messageDelta.blocks.find(block => block.id === 'message-1')).toMatchObject({ status: 'streaming', text: 'Hello' })
    expect(completed.blocks).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'thinking-1', status: 'completed', text: '权威思考摘要' }),
      expect.objectContaining({ id: 'message-1', status: 'completed', text: 'Hello!' }),
    ]))
  })

  it('rebuilds live text when refresh replays from the first delta', () => {
    const refreshed = stateFromTimeline({
      pageInfo: { hasOlder: false },
      conversation: { id: 'conversation-1', title: '刷新恢复', titleSource: 'assistant', status: 'active' },
      eventCursors: [{ runId: 'run-1', after: 0 }],
      turns: [{
        id: 'turn-1',
        turnIndex: 0,
        status: 'running',
        input: { id: 'turn-1:input', type: 'user_message', createdAt: '2026-07-28T00:00:00Z', parts: [{ id: 'input:0', partIndex: 0, type: 'text', text: '继续' }] },
        selectedRun: { id: 'run-1', runIndex: 0, status: 'running', items: [] },
      }],
    })
    const first = reduceAIEvent(refreshed, event({}))
    const second = reduceAIEvent(first, event({
      eventId: 'event-2',
      eventSequence: 2,
      payload: { itemId: 'item-1', contentPartId: 'item-1:0', partIndex: 0, delta: ' after refresh', timelineIndex: 0 },
    }))

    expect(second.blocks.find(block => block.id === 'item-1')).toMatchObject({ status: 'streaming', text: 'hello after refresh' })
  })

  it('rejects malformed live payloads before advancing the sequence', () => {
    expect(() => reduceAIEvent(emptyAIAssistantState, event({
      payload: { itemId: 'item-1', delta: 'missing required fields' },
    }))).toThrow('ai_invalid_stream_event_payload')
    expect(emptyAIAssistantState.lastEventSequences['run-1']).toBeUndefined()
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
      contentPartId: 'new-answer:0',
      payload: { itemId: 'new-answer', contentPartId: 'new-answer:0', partIndex: 0, delta: 'answer', timelineIndex: 0, createdAt: '2026-07-28T00:00:00Z' },
    }))
    expect(streamed.blocks.map(block => block.id)).toEqual([
      'turn-older:input',
      'turn-new:input',
      'new-answer',
    ])
  })

  it('does not regress a newer live item revision when a delayed timeline snapshot arrives', () => {
    const optimistic = addOptimisticTurn(emptyAIAssistantState, {
      turnId: 'turn',
      turnIndex: 0,
      runId: 'run',
      text: 'question',
    })
    const started = reduceAIEvent(optimistic, event({
      turnId: 'turn',
      runId: 'run',
      itemId: 'answer',
      contentPartId: 'answer:0',
      payload: { itemId: 'answer', contentPartId: 'answer:0', partIndex: 0, delta: 'streamed', timelineIndex: 0, createdAt: '2026-07-28T00:00:00Z' },
    }))
    const live = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      turnId: 'turn',
      runId: 'run',
      itemId: 'answer',
      contentPartId: 'answer:0',
      payload: { itemId: 'answer', contentPartId: 'answer:0', partIndex: 0, delta: ' answer', timelineIndex: 0 },
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
            { id: 'call', timelineIndex: 0, revision: 1, createdAt: '2026-07-28T00:00:01Z', type: 'tool_call', status: 'completed', parts: [], toolCall: { id: 'tool', operationId: 'runtime.read', callIndex: 0, result: { summaryKey: 'aiAssistant.resultAvailable' } } },
            { id: 'result', timelineIndex: 1, revision: 1, createdAt: '2026-07-28T00:00:02Z', type: 'tool_result', status: 'completed', relatedItemId: 'call', parts: [] },
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
        operationId: 'request_choice',
        arguments: { schemaVersion: 1, generationId: 'database-list', title: '正在整理数据库候选' },
        timelineIndex: 2,
      },
      item: toolItem({
        id: 'card-item',
        timelineIndex: 2,
        toolCall: { id: 'card-generation', operationId: 'request_choice', callIndex: 2, status: 'running', arguments: { schemaVersion: 1, generationId: 'database-list', title: '正在整理数据库候选' } },
      }),
    }))
    const completed = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'tool.completed',
      toolCallId: 'card-generation',
      payload: {
        operationId: 'request_choice',
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
          operationId: 'request_choice',
          callIndex: 2,
          status: 'succeeded',
          arguments: { schemaVersion: 1, generationId: 'database-list', title: '数据库候选', template: 'candidates', cards: [] },
        },
      }),
    }))

    expect(completed.blocks).toHaveLength(1)
    expect(completed.blocks[0]).toMatchObject({
      type: 'tool_call',
      operationId: 'request_choice',
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
      itemId: 'message-1',
      contentPartId: 'message-1:0',
      payload: { itemId: 'message-1', contentPartId: 'message-1:0', partIndex: 0, delta: '先检查资源。', timelineIndex: 0, createdAt: '2026-07-28T00:00:00Z' },
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
      contentPartId: 'message-2:0',
      payload: { itemId: 'message-2', contentPartId: 'message-2:0', partIndex: 0, delta: '接下来检查部署。', timelineIndex: 2, createdAt: '2026-07-28T00:00:00Z' },
    }))

    expect(followUp.blocks.map(block => block.id)).toEqual(['message-1', 'tool-item', 'message-2'])
  })

  it('stops projection and requests a snapshot when an event sequence has a gap', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({}))
    const gap = reduceAIEvent(first, event({
      eventId: 'event-3',
      eventSequence: 3,
      payload: { itemId: 'item-1', contentPartId: 'item-1:0', partIndex: 0, delta: ' must not apply before recovery', timelineIndex: 0 },
    }))

    expect(gap.blocks[0]).toMatchObject({ text: 'hello' })
    expect(gap.desyncedRunIds.has('run-1')).toBe(true)
    expect(gap.lastEventSequences['run-1']).toBe(1)
  })

  it('accepts replayed missing events in order and clears desync at the observed high watermark', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({}))
    const gap = reduceAIEvent(first, event({
      eventId: 'event-3',
      eventSequence: 3,
      payload: { itemId: 'item-1', contentPartId: 'item-1:0', partIndex: 0, delta: ' third', timelineIndex: 0 },
    }))
    const second = reduceAIEvent(gap, event({
      eventId: 'event-2',
      eventSequence: 2,
      payload: { itemId: 'item-1', contentPartId: 'item-1:0', partIndex: 0, delta: ' second', timelineIndex: 0 },
    }))
    const recovered = reduceAIEvent(second, event({
      eventId: 'event-3',
      eventSequence: 3,
      payload: { itemId: 'item-1', contentPartId: 'item-1:0', partIndex: 0, delta: ' third', timelineIndex: 0 },
    }))

    expect(second.lastEventSequences['run-1']).toBe(2)
    expect(second.desyncedRunIds.has('run-1')).toBe(true)
    expect(recovered.lastEventSequences['run-1']).toBe(3)
    expect(recovered.desyncedRunIds.has('run-1')).toBe(false)
    expect(recovered.desyncRecoverySequences['run-1']).toBeUndefined()
    expect(recovered.blocks[0]).toMatchObject({ text: 'hello second third' })
  })

  it('records provider-reported total tokens as the latest confirmed conversation context', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({ eventId: 'event-1', eventSequence: 1, type: 'run.started', item: undefined, payload: {} }))
    const completed = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { status: 'reported', inputTokens: 25600, outputTokens: 512, totalTokens: 26112 }, modelId: 'aimod_test', maxContextTokensSnapshot: 32_000 },
    }))
    expect(completed.runUsage['run-1']).toEqual({ status: 'reported', promptTokens: 25600, modelId: 'aimod_test', maxContextTokensSnapshot: 32_000 })
    expect(completed.contextUsage).toEqual({
      status: 'reported',
      runId: 'run-1',
      modelId: 'aimod_test',
      usedTokens: 26112,
      maxContextTokensSnapshot: 32_000,
      recordedAt: '2026-07-28T00:00:00Z',
    })

    const withoutUsage = reduceAIEvent(completed, event({
      eventId: 'event-3',
      eventSequence: 3,
      type: 'model.completed',
      item: undefined,
      payload: {},
    }))
    expect(withoutUsage.runUsage['run-1']).toEqual({ status: 'unavailable' })
    expect(withoutUsage.contextUsage).toEqual(completed.contextUsage)
  })

  it('replaces the confirmed context with the latest total and allows compaction to reduce it', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({ eventId: 'event-1', eventSequence: 1, type: 'run.started', item: undefined, payload: {} }))
    const first = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { status: 'reported', inputTokens: 1000, outputTokens: 500, totalTokens: 1500 }, modelId: 'aimod_test', maxContextTokensSnapshot: 32_000 },
    }))
    expect(first.runUsage['run-1']?.promptTokens).toBe(1000)
    expect(first.contextUsage?.usedTokens).toBe(1500)
    const second = reduceAIEvent(first, event({
      eventId: 'event-3',
      eventSequence: 3,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { status: 'reported', inputTokens: 200, outputTokens: 300, totalTokens: 500 }, modelId: 'aimod_test', maxContextTokensSnapshot: 32_000 },
    }))
    expect(second.runUsage['run-1']?.promptTokens).toBe(200)
    expect(second.contextUsage?.usedTokens).toBe(500)
  })

  it('keeps the last confirmed conversation context when a new run starts', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({
      eventId: 'event-1',
      eventSequence: 1,
      type: 'model.completed',
      item: undefined,
      payload: { usage: { status: 'reported', inputTokens: 1000, outputTokens: 200, totalTokens: 1200 }, modelId: 'aimod_test', maxContextTokensSnapshot: 32_000 },
    }))
    const optimistic = addOptimisticTurn(first, {
      turnId: 'turn-2',
      turnIndex: 1,
      runId: 'run-2',
      text: 'continue',
    })
    const nextRun = reduceAIEvent(optimistic, event({
      eventId: 'event-next-1',
      eventSequence: 1,
      runId: 'run-2',
      turnId: 'turn-2',
      type: 'run.started',
      item: undefined,
      payload: {},
    }))

    expect(nextRun.runUsage['run-1']).toEqual({ status: 'reported', promptTokens: 1000, modelId: 'aimod_test', maxContextTokensSnapshot: 32_000 })
    expect(nextRun.runUsage['run-2']).toEqual({ status: 'unavailable' })
    expect(nextRun.contextUsage).toEqual(first.contextUsage)
  })
})
