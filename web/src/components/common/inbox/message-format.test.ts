import type { InboxMessage } from '@/api'
import { describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { inboxMessageText } from './message-format'

function message(overrides: Partial<InboxMessage> = {}): InboxMessage {
  return {
    id: 'imsg_test',
    type: 'project.member_added',
    category: 'project',
    priority: 'normal',
    actorId: 'usr_actor',
    projectId: 'prj_test',
    resourceType: 'project',
    resourceId: 'prj_test',
    titleKey: 'inbox.messages.project.member_added.title',
    contentKey: 'inbox.messages.project.member_added.content',
    params: { actorName: 'Snowy', projectName: 'Luna' },
    actionRequestId: '',
    deepLink: '/projects/prj_test',
    groupKey: '',
    createdAt: '2026-08-01T00:00:00Z',
    updatedAt: '2026-08-01T00:00:00Z',
    ...overrides,
  }
}

describe('inboxMessageText', () => {
  it('translates a whitelisted message key with safe parameters', async () => {
    await i18next.changeLanguage('zh-CN')
    expect(inboxMessageText(message(), i18next.t)).toEqual({
      title: '你已加入项目空间',
      content: 'Snowy 已将你加入「Luna」。',
    })
  })

  it('does not render unknown backend keys or structured parameter values', async () => {
    await i18next.changeLanguage('zh-CN')
    const result = inboxMessageText(message({
      titleKey: '<img src=x onerror=alert(1)>',
      contentKey: 'untrusted.content',
      params: { message: '<b>unsafe</b>', nested: { secret: 'hidden' } },
    }), i18next.t)

    expect(result.title).toBe('平台消息')
    expect(result.content).toContain('相关资源：prj_test')
    expect(result.content).not.toContain('unsafe')
    expect(result.content).not.toContain('hidden')
  })

  it('ignores unsupported parameter types', async () => {
    await i18next.changeLanguage('en-US')
    const result = inboxMessageText(message({ params: { actorName: ['invalid'], projectName: { invalid: true } } }), i18next.t)
    expect(result.title).toBe('You joined a project')
    expect(result.content).not.toContain('invalid')
  })
})
