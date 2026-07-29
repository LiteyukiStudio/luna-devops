import type { AIActionContext } from './ai-assistant-actions'
import { describe, expect, it, vi } from 'vitest'
import { executeAIUIAction } from './ai-assistant-actions'

function context(overrides: Partial<AIActionContext> = {}): AIActionContext {
  return {
    pathname: '/events',
    search: '',
    navigate: vi.fn(),
    queryClient: { invalidateQueries: vi.fn() } as never,
    ...overrides,
  }
}

describe('aI UI action registry', () => {
  it('builds registered routes from validated parameters', async () => {
    const ctx = context()
    expect(await executeAIUIAction({ version: 1, type: 'navigate', payload: { routeName: 'application.detail', params: { projectId: 'p1', applicationId: 'a1' }, query: { tab: 'builds' } } }, ctx)).toBe(true)
    expect(ctx.navigate).toHaveBeenCalledWith('/projects/p1/apps/a1?tab=builds')
  })

  it('rejects external and unregistered routes fail closed', async () => {
    const ctx = context()
    const unsafe = { version: 1, type: 'navigate', payload: { routeName: 'https://evil.example', params: {}, query: {} } } as never
    expect(await executeAIUIAction(unsafe, ctx)).toBe(false)
    expect(ctx.navigate).not.toHaveBeenCalled()
  })

  it('only sets event filters on the registered target page', async () => {
    const wrongPage = context({ pathname: '/dashboard' })
    expect(await executeAIUIAction({ version: 1, type: 'set_filters', payload: { targetId: 'events', values: { category: 'release' } } }, wrongPage)).toBe(false)
  })
})
