import type { AIEvent, AITimeline } from '@/api'
import { describe, expect, it } from 'vitest'
import { addOptimisticTurn, emptyAIAssistantState, isValidAITimeline, mergeTimelineSnapshot, reduceAIEvent, stateFromTimeline } from './state'

function event(overrides: Partial<AIEvent>): AIEvent {
  return {
    version: 1,
    eventId: 'event-1',
    eventSequence: 1,
    type: 'content.delta',
    conversationId: 'conversation-1',
    turnId: 'turn-1',
    runId: 'run-1',
    itemId: 'item-1',
    occurredAt: '2026-07-28T00:00:00Z',
    payload: { delta: 'hello' },
    ...overrides,
  }
}

describe('aI assistant state', () => {
  it('rejects incomplete timeline snapshots fail closed', () => {
    expect(isValidAITimeline({ conversation: { id: 'c' }, turns: [] })).toBe(false)
    expect(isValidAITimeline({ conversation: { id: 'c' }, turns: [], eventCursors: [] })).toBe(true)
  })

  it('merges streaming deltas and ignores duplicate or stale events', () => {
    const first = reduceAIEvent(emptyAIAssistantState, event({}))
    const duplicate = reduceAIEvent(first, event({}))
    const second = reduceAIEvent(duplicate, event({ eventId: 'event-2', eventSequence: 2, payload: { delta: ' world' } }))
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
    }))
    expect(streamed.blocks.map(block => block.id)).toEqual([
      'turn-older:input',
      'turn-new:input',
      'new-answer',
    ])
  })

  it('does not regress a longer live block when a delayed timeline snapshot arrives', () => {
    const live = reduceAIEvent(addOptimisticTurn(emptyAIAssistantState, {
      turnId: 'turn',
      turnIndex: 0,
      runId: 'run',
      text: 'question',
    }), event({ itemId: 'answer', payload: { delta: 'streamed answer', timelineIndex: 0 } }))
    const snapshot: AITimeline = {
      conversation: { id: 'conversation-1', title: 't', titleSource: 'assistant', status: 'active' },
      eventCursors: [{ runId: 'run', after: 0 }],
      turns: [{
        id: 'turn',
        turnIndex: 0,
        status: 'running',
        input: { id: 'turn:input', type: 'user_message', parts: [{ id: 'input', partIndex: 0, type: 'text', text: 'question' }] },
        selectedRun: {
          id: 'run',
          runIndex: 0,
          status: 'running',
          items: [{ id: 'answer', timelineIndex: 0, type: 'assistant_message', status: 'streaming', parts: [{ id: 'answer:0', partIndex: 0, type: 'text', text: 'streamed' }] }],
        },
      }],
    }
    expect(mergeTimelineSnapshot(live, snapshot).blocks.at(-1)).toMatchObject({ text: 'streamed answer' })
  })

  it('maps a started run to the active running state', () => {
    const state = reduceAIEvent(emptyAIAssistantState, event({ type: 'run.started' }))
    expect(state.runStatuses['run-1']).toBe('running')
  })

  it('renders a stable failure notice from a terminal SSE event', () => {
    const state = reduceAIEvent(emptyAIAssistantState, event({
      type: 'run.failed',
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

  it('restores a failed run notice from a timeline snapshot', () => {
    const timeline: AITimeline = {
      conversation: { id: 'c', title: 't', titleSource: 'assistant', status: 'active' },
      eventCursors: [],
      turns: [{
        id: 'turn',
        turnIndex: 0,
        status: 'failed',
        input: { id: 'input', type: 'user_message', parts: [{ id: 'p', partIndex: 0, type: 'text', text: 'check' }] },
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
      conversation: { id: 'c', title: 't', titleSource: 'assistant', status: 'active' },
      eventCursors: [],
      turns: [{
        id: 'turn',
        turnIndex: 0,
        status: 'completed',
        input: { id: 'input', type: 'user_message', parts: [{ id: 'p', partIndex: 0, type: 'text', text: 'check' }] },
        selectedRun: {
          id: 'run',
          runIndex: 0,
          status: 'completed',
          items: [
            { id: 'call', timelineIndex: 0, type: 'tool_call', status: 'completed', parts: [], toolCall: { id: 'tool', operationId: 'runtime.read', callIndex: 0 } },
            { id: 'result', timelineIndex: 1, type: 'tool_result', status: 'completed', relatedItemId: 'call', parts: [{ id: 'r', partIndex: 0, type: 'structured_data', data: { summaryKey: 'aiAssistant.resultAvailable' } }] },
          ],
        },
      }],
    }
    const projected = stateFromTimeline(timeline)
    expect(projected.blocks).toHaveLength(2)
    expect(projected.blocks[1]).toMatchObject({ type: 'tool_call', result: { summaryKey: 'aiAssistant.resultAvailable' } })
  })

  it('binds a dangerous tool confirmation to its arguments and row version', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({
      type: 'tool.started',
      toolCallId: 'tool-1',
      payload: { operationId: 'restartDeploymentTarget', arguments: { targetId: 'target-1' }, argumentsHash: 'sha256:abc', expectedVersion: 1 },
    }))
    const approval = reduceAIEvent(started, event({
      eventId: 'event-2',
      eventSequence: 2,
      type: 'approval.required',
      toolCallId: 'tool-1',
      payload: { argumentsHash: 'sha256:abc', expectedVersion: 2 },
    }))
    expect(approval.blocks[0]).toMatchObject({
      type: 'tool_call',
      status: 'awaiting_approval',
      argumentsHash: 'sha256:abc',
      expectedVersion: 2,
    })
    expect(approval.runStatuses['run-1']).toBe('waiting_approval')
  })

  it('retains a failed tool error code and request id from the live event', () => {
    const started = reduceAIEvent(emptyAIAssistantState, event({
      type: 'tool.started',
      toolCallId: 'tool-1',
      payload: { operationId: 'listApplications', arguments: { projectId: 'prj_1' } },
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
    }))

    expect(failed.blocks[0]).toMatchObject({
      type: 'tool_call',
      status: 'failed',
      titleKey: 'ai.tool_storage_unavailable',
      result: {
        requestId: 'req_tool_failure',
      },
    })
  })
})
