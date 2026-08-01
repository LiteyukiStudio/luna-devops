import type { AIEvent } from '@/api'
import { describe, expect, it } from 'vitest'
import { automaticRouteDeliveryFromEvent, automaticRouteDeliveryFromPending } from './automatic-actions'

function event(payload: Record<string, unknown>, type = 'tool.completed'): AIEvent {
  return {
    version: 2,
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
    expect(automaticRouteDeliveryFromEvent(event({
      operationId: 'navigate_to_route',
      uiActions: [action],
      uiActionDelivery: { actionId: 'aiuia_12345', expiresAt: '2030-01-01T00:00:00.000Z' },
    }))).toEqual({
      action,
      actionId: 'aiuia_12345',
      expiresAt: '2030-01-01T00:00:00.000Z',
    })

    expect(automaticRouteDeliveryFromEvent(event({
      operationId: 'create_options',
      uiActions: [action],
    }))).toBeNull()
    expect(automaticRouteDeliveryFromEvent(event({
      operationId: 'navigate_to_route',
      uiActions: [{ ...action, activation: 'manual' }],
    }))).toBeNull()
    expect(automaticRouteDeliveryFromEvent(event({
      operationId: 'navigate_to_route',
      uiActions: [action],
    }, 'run.completed'))).toBeNull()
  })

  it('validates a pending delivery before replaying it', () => {
    const action = {
      version: 1 as const,
      type: 'navigate' as const,
      activation: 'automatic' as const,
      repeatable: false,
      payload: { routeName: 'projects', params: {}, query: {} },
    }
    expect(automaticRouteDeliveryFromPending({
      actionId: 'aiuia_12345',
      runId: 'run_1',
      toolCallId: 'tool_1',
      action,
      attempts: 2,
      expiresAt: '2030-01-01T00:00:00.000Z',
    })).toEqual({
      action,
      actionId: 'aiuia_12345',
      expiresAt: '2030-01-01T00:00:00.000Z',
    })
  })
})
