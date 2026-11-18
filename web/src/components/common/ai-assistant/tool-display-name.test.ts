import { describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { toolDisplayName } from './tool-display-name'

describe('ai tool display names', () => {
  it('localizes common platform and internal tools without exposing operation ids', async () => {
    await i18next.changeLanguage('zh-CN')

    expect(toolDisplayName(i18next.t, 'listApplications')).toBe('查询应用列表')
    expect(toolDisplayName(i18next.t, 'createRelease')).toBe('创建发布')
    expect(toolDisplayName(i18next.t, 'fetchWebPage')).toBe('读取网页')
    expect(toolDisplayName(i18next.t, 'present_card')).toBe('展示信息卡片')
    expect(toolDisplayName(i18next.t, 'request_input')).toBe('请求结构化输入')
    expect(toolDisplayName(i18next.t, 'request_choice')).toBe('请求选择')
    expect(toolDisplayName(i18next.t, 'load_skill')).toBe('加载领域技能')
    expect(toolDisplayName(i18next.t, 'getProjectDiagnostics')).toBe('查看项目诊断概览')
    expect(toolDisplayName(i18next.t, 'getDeploymentTargetDiagnostics')).toBe('查看部署诊断快照')
  })

  it('uses a localized safe fallback for newly registered operations', async () => {
    await i18next.changeLanguage('zh-CN')
    expect(toolDisplayName(i18next.t, 'futureUnknownOperation')).toBe('平台操作')

    await i18next.changeLanguage('en-US')
    expect(toolDisplayName(i18next.t, 'futureUnknownOperation')).toBe('Platform operation')
  })
})
