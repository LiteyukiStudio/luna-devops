import { describe, expect, it } from 'vitest'
import { buildAIPageContext } from './page-context'

describe('ai assistant page context', () => {
  it('describes the current resource, view, navigation, locale, and client time', () => {
    const context = buildAIPageContext(
      '/projects/prj_1/apps/app_1',
      '?tab=builds&buildRunId=build_1&token=secret',
      'zh-CN',
      { hash: '#releaseId=rel_1', now: new Date('2026-07-29T08:30:00.000Z'), timeZone: 'Asia/Shanghai' },
    )

    expect(context).toMatchObject({
      schemaVersion: 1,
      routeName: 'application.detail',
      routeTemplate: '/projects/:projectId/apps/:applicationId',
      pageKind: 'application',
      projectId: 'prj_1',
      applicationId: 'app_1',
      activeTab: 'builds',
      view: {
        query: { tab: 'builds', buildRunId: 'build_1' },
        hash: { releaseId: 'rel_1' },
        selectedResourceIds: ['build_1', 'rel_1'],
        availableTabs: expect.arrayContaining(['builds', 'deployments', 'gateway']),
      },
      client: {
        locale: 'zh-CN',
        timeZone: 'Asia/Shanghai',
        timestamp: '2026-07-29T08:30:00.000Z',
      },
    })
    expect(context.view.query).not.toHaveProperty('token')
  })

  it('falls back to a bounded unknown-page context', () => {
    expect(buildAIPageContext('/not-registered', '?redirect=https://evil.example', 'en-US', {
      now: new Date('2026-07-29T00:00:00.000Z'),
      timeZone: 'UTC',
    })).toMatchObject({
      routeName: 'unknown',
      routeTemplate: '',
      navigation: { relatedRouteNames: ['dashboard', 'projects'] },
      view: { query: {} },
    })
  })
})
