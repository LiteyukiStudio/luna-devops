import { beforeEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { resolveAIPresetSuggestions } from './suggestions'

describe('ai assistant suggestion selection', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('uses localized page presets before the first user turn', () => {
    const suggestions = resolveAIPresetSuggestions('/projects', i18next.t, true)

    expect(suggestions?.sourceKey).toBe('preset:/projects')
    expect(suggestions?.actions).toHaveLength(3)
    expect(suggestions?.actions.map(action => 'label' in action ? action.label : undefined)).toEqual([
      '查看项目空间',
      '创建项目空间',
      '部署代码仓库',
    ])
    expect(suggestions?.actions[0]).toMatchObject({ tone: 'primary', type: 'send_message' })
  })

  it('does not flash presets before an existing conversation timeline loads', () => {
    expect(resolveAIPresetSuggestions('/projects', i18next.t, false)).toBeNull()
  })
})
