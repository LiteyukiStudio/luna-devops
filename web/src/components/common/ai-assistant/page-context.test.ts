import { describe, expect, it } from 'vitest'
import { buildAIPageContext } from './page-context'

describe('ai assistant page context', () => {
  it('describes the current resource, active tab, locale, and time zone without volatile timestamps or static lists', () => {
    const context = buildAIPageContext(
      '/projects/prj_1/apps/app_1',
      '?tab=builds&buildRunId=build_1&clusterId=clu_1&token=secret',
      'zh-CN',
      { hash: '#releaseId=rel_1', timeZone: 'Asia/Shanghai' },
    )

    expect(context).toEqual({
      schemaVersion: 1,
      routeName: 'application.detail',
      routeTemplate: '/projects/:projectId/apps/:applicationId',
      pathname: '/projects/prj_1/apps/app_1',
      pageKind: 'application',
      projectId: 'prj_1',
      applicationId: 'app_1',
      clusterId: 'clu_1',
      activeTab: 'builds',
      view: {
        query: { tab: 'builds', buildRunId: 'build_1', clusterId: 'clu_1' },
        hash: { releaseId: 'rel_1' },
        selectedResourceIds: ['build_1', 'rel_1'],
      },
      locale: 'zh-CN',
      timeZone: 'Asia/Shanghai',
    })
    expect(context.view.query).not.toHaveProperty('token')
    expect(context).not.toHaveProperty('client')
    expect(context).not.toHaveProperty('navigation')
    expect(context.view).not.toHaveProperty('availableTabs')
  })

  it('recognizes /ai, /ai-assistant, deployment editor, and /settings/* routes', () => {
    expect(buildAIPageContext('/ai', '', 'zh-CN', { timeZone: 'UTC' })).toMatchObject({
      routeName: 'ai-assistant',
      routeTemplate: '/ai',
      pageKind: 'ai-assistant',
    })
    expect(buildAIPageContext('/ai-assistant', '', 'zh-CN', { timeZone: 'UTC' })).toMatchObject({
      routeName: 'ai-assistant',
      routeTemplate: '/ai-assistant',
      pageKind: 'ai-assistant',
    })
    expect(buildAIPageContext('/settings/account', '', 'zh-CN', { timeZone: 'UTC' })).toMatchObject({
      routeName: 'settings.account',
      routeTemplate: '/settings/account',
      pageKind: 'settings',
    })
    expect(buildAIPageContext('/settings/custom-section', '', 'zh-CN', { timeZone: 'UTC' })).toMatchObject({
      routeName: 'settings',
      routeTemplate: '/settings/:section',
      pageKind: 'settings',
    })
    expect(buildAIPageContext('/projects/prj_1/apps/app_1/deployments/tgt_1/edit', '', 'zh-CN', { timeZone: 'UTC' })).toMatchObject({
      routeName: 'application.detail',
      projectId: 'prj_1',
      applicationId: 'app_1',
      targetId: 'tgt_1',
    })
  })

  it('falls back to a bounded unknown-page context', () => {
    expect(buildAIPageContext('/not-registered', '?redirect=https://evil.example', 'en-US', {
      timeZone: 'UTC',
    })).toEqual({
      schemaVersion: 1,
      routeName: 'unknown',
      routeTemplate: '',
      pathname: '/not-registered',
      pageKind: 'unknown',
      view: {
        query: {},
        hash: {},
        selectedResourceIds: [],
      },
      locale: 'en-US',
      timeZone: 'UTC',
    })
    // 非法路径参数不得被提取进结构化资源标识
    const malformed = buildAIPageContext('/projects/prj%20bad/apps/app_1/deployments/tgt_1/edit', '', 'zh-CN', { timeZone: 'UTC' })
    expect(malformed.routeName).toBe('unknown')
    expect(malformed).not.toHaveProperty('projectId')
    expect(malformed).not.toHaveProperty('applicationId')
    expect(malformed).not.toHaveProperty('targetId')
  })
})
