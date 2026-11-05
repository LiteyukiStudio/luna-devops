import type { GatewayRoute } from '@/api'
import { describe, expect, it } from 'vitest'
import { displayAccessUrl, gatewayRouteAccessUrl, primaryGatewayRoute } from './gateway-route-url'

function route(overrides: Partial<GatewayRoute>): GatewayRoute {
  return { accessUrl: '', deleteStatus: 'active', enabled: true, host: '', id: 'route', path: '/', status: 'ready', tlsMode: 'http-challenge', ...overrides } as GatewayRoute
}

describe('gateway route url', () => {
  it('prefers the authoritative access url and derives one from host otherwise', () => {
    expect(gatewayRouteAccessUrl(route({ accessUrl: 'https://app.example.com' }))).toBe('https://app.example.com')
    expect(gatewayRouteAccessUrl(route({ host: 'app.example.com', path: '/api', tlsMode: 'http-only' }))).toBe('http://app.example.com/api')
  })

  it('rejects non-http protocols', () => {
    expect(gatewayRouteAccessUrl(route({ accessUrl: 'javascript:alert(1)' }))).toBe('')
  })

  it('picks a ready enabled route as the primary address', () => {
    const pending = route({ accessUrl: 'https://pending.example.com', id: 'pending', status: 'pending' })
    const ready = route({ accessUrl: 'https://ready.example.com', id: 'ready' })
    const disabled = route({ accessUrl: 'https://disabled.example.com', enabled: false, id: 'disabled' })
    expect(primaryGatewayRoute([disabled, pending, ready])?.id).toBe('ready')
    expect(primaryGatewayRoute([disabled, pending])?.id).toBe('pending')
    expect(primaryGatewayRoute([disabled])).toBeUndefined()
    expect(displayAccessUrl('https://ready.example.com/')).toBe('ready.example.com')
  })
})
