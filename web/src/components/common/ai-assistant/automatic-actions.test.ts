import type { AIEvent } from '@/api'
import { describe, expect, it } from 'vitest'
import { automaticRouteActionFromEvent } from './automatic-actions'

function event(payload: Record<string, unknown>, type = 'tool.completed'): AIEvent {
  return {
    version: 1,
    eventId: 'evt_1',
    eventSequence: 1,
    type,
    conversationId: 'conversation_1',
    turnId: 'turn_1',
    runId: 'run_1',
    toolCallId: 'tool_1',
    occurredAt: new Date(0).toISOString(),
    payload,
  }
}

describe('automatic frontend actions', () => {
  it('accepts only automatic navigation emitted by navigate_to_route', () => {
    const action = {
      version: 1,
      type: 'navigate',
      activation: 'automatic',
      repeatable: false,
      payload: { routeName: 'projects', params: {}, query: {} },
    }
    expect(automaticRouteActionFromEvent(event({
      operationId: 'navigate_to_route',
      uiActions: [action],
    }))).toEqual(action)

    expect(automaticRouteActionFromEvent(event({
      operationId: 'create_options',
      uiActions: [action],
    }))).toBeNull()
    expect(automaticRouteActionFromEvent(event({
      operationId: 'navigate_to_route',
      uiActions: [{ ...action, activation: 'manual' }],
    }))).toBeNull()
    expect(automaticRouteActionFromEvent(event({
      operationId: 'navigate_to_route',
      uiActions: [action],
    }, 'run.completed'))).toBeNull()
  })
})
