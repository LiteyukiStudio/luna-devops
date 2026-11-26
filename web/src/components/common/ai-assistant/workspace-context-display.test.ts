import { beforeEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { formatWorkspaceContextDisplay } from './workspace-context-display'

describe('workspace context display formatter', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('formats application and deployment editor routes without site brand suffix', () => {
    expect(formatWorkspaceContextDisplay(
      { pathname: '/projects/prj_1/apps/app_1', search: '' },
      i18next.t,
      { projectName: '核心业务', applicationName: '网关服务' },
    )).toEqual({
      kind: 'application',
      label: '应用：核心业务 / 网关服务',
    })

    expect(formatWorkspaceContextDisplay(
      { pathname: '/projects/prj_1/apps/app_1/deployments/new', search: '' },
      i18next.t,
      { projectName: '核心业务', applicationName: '网关服务' },
    )).toEqual({
      kind: 'application',
      label: '新增部署配置：核心业务 / 网关服务',
    })

    expect(formatWorkspaceContextDisplay(
      { pathname: '/projects/prj_1/apps/app_1/deployments/dplt_1/edit', search: '' },
      i18next.t,
      { projectName: '核心业务', applicationName: '网关服务' },
    )).toEqual({
      kind: 'application',
      label: '编辑部署配置：核心业务 / 网关服务',
    })
  })

  it('formats project workspace overview and specific section tabs', () => {
    expect(formatWorkspaceContextDisplay(
      { pathname: '/projects/prj_demo', search: '' },
      i18next.t,
      { projectName: 'Demo' },
    )).toEqual({
      kind: 'project',
      label: '项目：Demo',
    })

    expect(formatWorkspaceContextDisplay(
      { pathname: '/projects/prj_demo', search: '?tab=volumes' },
      i18next.t,
      { projectName: 'Demo' },
    )).toEqual({
      kind: 'project',
      label: '项目：Demo / 数据卷',
    })

    expect(formatWorkspaceContextDisplay(
      { pathname: '/projects/prj_demo', search: '?tab=billing' },
      i18next.t,
      { projectName: 'Demo' },
    )).toEqual({
      kind: 'project',
      label: '项目：Demo / 账单',
    })
  })

  it('resolves static console pages cleanly without raw path prefixes', () => {
    expect(formatWorkspaceContextDisplay({ pathname: '/dashboard', search: '' }, i18next.t)).toEqual({
      kind: 'dashboard',
      label: '看板',
    })
    expect(formatWorkspaceContextDisplay({ pathname: '/settings/site', search: '' }, i18next.t)).toEqual({
      kind: 'settings',
      label: '全局设置',
    })
  })
})
