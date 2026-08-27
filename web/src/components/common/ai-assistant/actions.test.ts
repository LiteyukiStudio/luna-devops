import type { AIActionContext } from './actions'
import { describe, expect, it, vi } from 'vitest'
import { executeAIUIAction, getAIUIActionTargetPath } from './actions'

function context(overrides: Partial<AIActionContext> = {}): AIActionContext {
  return {
    navigate: vi.fn(),
    ...overrides,
  }
}

describe('aI UI action registry', () => {
  it('builds registered routes from validated parameters', async () => {
    const ctx = context()
    expect(await executeAIUIAction({ version: 1, type: 'navigate', payload: { routeName: 'application.detail', params: { projectId: 'p1', applicationId: 'a1' }, query: { tab: 'builds' } } }, ctx)).toBe(true)
    expect(ctx.navigate).toHaveBeenCalledWith('/projects/p1/apps/a1?tab=builds')
    expect(getAIUIActionTargetPath({ version: 1, type: 'navigate', payload: { routeName: 'application.detail', params: { projectId: 'p1', applicationId: 'a1' }, query: { tab: 'builds' } } })).toBe('/projects/p1/apps/a1?tab=builds')
  })

  it('navigates to registered settings and catalog pages', async () => {
    const ctx = context()
    expect(await executeAIUIAction({ version: 1, type: 'navigate', payload: { routeName: 'app-templates', params: {}, query: {} } }, ctx)).toBe(true)
    expect(await executeAIUIAction({ version: 1, type: 'navigate', payload: { routeName: 'settings.notifications', params: {}, query: {} } }, ctx)).toBe(true)
    expect(ctx.navigate).toHaveBeenNthCalledWith(1, '/app-templates')
    expect(ctx.navigate).toHaveBeenNthCalledWith(2, '/settings/notifications')
  })

  it('rejects external and unregistered routes fail closed', async () => {
    const ctx = context()
    const unsafe = { version: 1, type: 'navigate', payload: { routeName: 'https://evil.example', params: {}, query: {} } } as never
    expect(await executeAIUIAction(unsafe, ctx)).toBe(false)
    expect(ctx.navigate).not.toHaveBeenCalled()
  })

  it('turns message and controlled-tool options into a new user request', async () => {
    const sendMessage = vi.fn(async () => {})
    const requestTool = vi.fn(async () => {})
    const ctx = context({ sendMessage, requestTool })
    expect(await executeAIUIAction({ version: 1, type: 'send_message', label: '继续', payload: { message: '继续诊断' } }, ctx)).toBe(true)
    expect(await executeAIUIAction({ version: 1, type: 'request_tool', label: '重试', payload: { operationId: 'retryBuildRun', arguments: { runId: 'run_1' }, message: '请重试构建 run_1' } }, ctx)).toBe(true)
    expect(sendMessage).toHaveBeenNthCalledWith(1, '继续诊断')
    expect(requestTool).toHaveBeenCalledWith(expect.objectContaining({
      type: 'request_tool',
      payload: { operationId: 'retryBuildRun', arguments: { runId: 'run_1' }, message: '请重试构建 run_1' },
    }))
    expect(sendMessage).toHaveBeenCalledOnce()
  })
})
