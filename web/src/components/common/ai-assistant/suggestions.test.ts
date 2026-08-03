import type { AIBlock } from './state'
import { beforeEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { resolveAISuggestions } from './suggestions'

describe('ai assistant suggestion selection', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('uses localized page presets before the first user turn', () => {
    const suggestions = resolveAISuggestions([], '/projects', i18next.t, false, true)

    expect(suggestions?.sourceKey).toBe('preset:/projects')
    expect(suggestions?.actions).toHaveLength(3)
    expect(suggestions?.actions.map(action => 'label' in action ? action.label : undefined)).toEqual([
      '查看项目空间',
      '创建项目空间',
      '部署代码仓库',
    ])
    expect(suggestions?.actions[0]).toMatchObject({ tone: 'primary', type: 'send_message' })
  })

  it('uses the latest completed agent options after a turn and hides stale options after a new message', () => {
    const blocks: AIBlock[] = [
      message('user-1', 0),
      options('options-1', 3, '第一轮建议'),
    ]

    expect(resolveAISuggestions(blocks, '/projects', i18next.t, false, false)?.sourceKey).toBe('agent:options-1')
    expect(resolveAISuggestions([...blocks, message('user-2', 4)], '/projects', i18next.t, false, false)).toBeNull()
  })

  it('hides suggestions while a run is active', () => {
    expect(resolveAISuggestions([], '/dashboard', i18next.t, true, true)).toBeNull()
  })

  it('does not flash presets before an existing conversation timeline loads', () => {
    expect(resolveAISuggestions([], '/projects', i18next.t, false, false)).toBeNull()
  })
})

function message(id: string, index: number): AIBlock {
  return {
    id,
    turnId: id,
    index,
    type: 'message',
    role: 'user',
    status: 'completed',
    text: id,
    createdAt: '2026-08-01T09:00:00+08:00',
  }
}

function options(id: string, index: number, label: string): AIBlock {
  return {
    id,
    turnId: 'turn-1',
    runId: 'run-1',
    index,
    type: 'tool_call',
    toolCallId: id,
    operationId: 'create_options',
    visibility: 'internal',
    status: 'succeeded',
    arguments: {},
    uiActions: [{
      version: 1,
      id,
      repeatable: false,
      type: 'send_message',
      label,
      payload: { message: label },
    }],
  }
}
