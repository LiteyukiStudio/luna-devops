import type { AIConversation } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIConversationList } from './conversation-list'

const conversations: AIConversation[] = [
  {
    id: 'conversation-1',
    title: '构建失败诊断',
    titleSource: 'assistant',
    status: 'active',
    createdAt: '2026-07-29T00:00:00.000Z',
    updatedAt: '2026-07-29T00:00:00.000Z',
  },
  {
    id: 'conversation-2',
    title: '我的固定标题',
    titleSource: 'user',
    status: 'active',
    createdAt: '2026-07-29T01:00:00.000Z',
    updatedAt: '2026-07-29T01:00:00.000Z',
  },
]

function renderList(onDeleteMany = vi.fn(async () => {})) {
  render(
    <AIConversationList
      activeId="conversation-1"
      conversations={conversations}
      deleting={false}
      loading={false}
      runningConversationIds={new Set()}
      search=""
      onDeleteMany={onDeleteMany}
      onRename={vi.fn()}
      onSearch={vi.fn()}
      onSelect={vi.fn()}
    />,
  )
  return onDeleteMany
}

describe('ai conversation list', () => {
  it('enters an explicit selection mode and bulk deletes the selected conversations', async () => {
    await i18next.changeLanguage('zh-CN')
    const user = userEvent.setup()
    const onDeleteMany = renderList()

    await user.click(screen.getByRole('button', { name: '批量选择会话' }))
    await user.click(screen.getByRole('button', { name: '全选' }))

    expect(screen.getByText('已选 2 项')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '删除选中的会话' }))
    await user.click(screen.getByRole('button', { name: '删除 2 个会话' }))

    expect(onDeleteMany).toHaveBeenCalledWith(['conversation-1', 'conversation-2'])
  })

  it('exposes the permanent manual-title lock in the conversation list', async () => {
    renderList()

    expect(screen.getByLabelText(i18next.t('aiAssistant.conversations.manualTitleLocked'))).toBeInTheDocument()
  })

  it('keeps navigation and new-conversation controls out of the conversation-list header', async () => {
    await i18next.changeLanguage('zh-CN')
    renderList()

    expect(screen.queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '新建会话' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '批量选择会话' })).toBeInTheDocument()
  })
})
