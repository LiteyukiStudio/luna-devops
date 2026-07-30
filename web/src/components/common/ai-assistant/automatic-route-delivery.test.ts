import type { AIUIActionAcknowledgement } from '@/api'
import { describe, expect, it, vi } from 'vitest'
import { executeAutomaticRouteDelivery } from './automatic-route-delivery'

const delivery = {
  actionId: 'aiuia_12345',
  expiresAt: '2030-01-01T00:00:00.000Z',
  action: {
    version: 1 as const,
    type: 'navigate' as const,
    activation: 'automatic' as const,
    repeatable: false,
    payload: { routeName: 'projects', params: {}, query: {} },
  },
}

describe('automatic route delivery', () => {
  it('acknowledges only after the expected route has landed', async () => {
    const acknowledgements: Array<Omit<AIUIActionAcknowledgement, 'clientInstanceId'>> = []
    expect(await executeAutomaticRouteDelivery({
      delivery,
      execute: vi.fn(async () => true),
      acknowledge: vi.fn(async (_, acknowledgement) => { acknowledgements.push(acknowledgement) }),
      currentPath: () => '/projects',
      waitForPath: async expectedPath => expectedPath === '/projects',
      now: () => Date.parse('2029-01-01T00:00:00.000Z'),
    })).toBe(true)
    expect(acknowledgements).toEqual([{ status: 'succeeded', actualPath: '/projects' }])
  })

  it('reports a terminal failure when navigation does not land', async () => {
    const acknowledge = vi.fn(async () => {})
    expect(await executeAutomaticRouteDelivery({
      delivery,
      execute: vi.fn(async () => true),
      acknowledge,
      currentPath: () => '/dashboard',
      waitForPath: async () => false,
      now: () => Date.parse('2029-01-01T00:00:00.000Z'),
    })).toBe(false)
    expect(acknowledge).toHaveBeenCalledWith('aiuia_12345', {
      status: 'failed',
      actualPath: '/dashboard',
      errorCode: 'ai.ui_action_navigation_timeout',
    })
  })

  it('does not execute an expired delivery', async () => {
    const execute = vi.fn(async () => true)
    const acknowledge = vi.fn(async () => {})
    expect(await executeAutomaticRouteDelivery({
      delivery,
      execute,
      acknowledge,
      currentPath: () => '/dashboard',
      now: () => Date.parse('2031-01-01T00:00:00.000Z'),
    })).toBe(false)
    expect(execute).not.toHaveBeenCalled()
    expect(acknowledge).toHaveBeenCalledWith('aiuia_12345', {
      status: 'failed',
      errorCode: 'ai.ui_action_expired',
    })
  })
})
