import type { AIEvent, AITimeline } from '@/api'
import { describe, expect, it } from 'vitest'
import { emptyAIAssistantState, isValidAITimeline, reduceAIEvent, stateFromTimeline } from './ai-assistant-state'

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

  it('maps a started run to the active running state', () => {
    const state = reduceAIEvent(emptyAIAssistantState, event({ type: 'run.started' }))
    expect(state.runStatuses['run-1']).toBe('running')
  })

  it('projects tool result into its tool call without a duplicate block', () => {
    const timeline: AITimeline = {
      conversation: { id: 'c', title: 't', status: 'active' },
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
})
