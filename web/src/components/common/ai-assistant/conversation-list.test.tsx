import type { AIConversation } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIConversationList } from './conversation-list'
import { formatAIConversationTimestamp } from './conversation-timestamp'

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
  it('formats conversation timestamps by calendar distance', () => {
    const now = new Date(2026, 7, 5, 18, 30)

    expect(formatAIConversationTimestamp(new Date(2026, 7, 5, 16, 20).toISOString(), 'zh-CN', now)).toBe('16:20')
    expect(formatAIConversationTimestamp(new Date(2026, 7, 4, 16, 20).toISOString(), 'zh-CN', now)).toBe('8月4日 16:20')
    expect(formatAIConversationTimestamp(new Date(2025, 11, 31, 16, 20).toISOString(), 'zh-CN', now)).toBe('2025年12月31日 16:20')
  })

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

  it('uses a touch-friendly mobile header and keeps the list open while switching conversations', async () => {
    await i18next.changeLanguage('zh-CN')
    const user = userEvent.setup()
    const onBack = vi.fn()
    const onSelect = vi.fn()
    render(
      <AIConversationList
        activeId="conversation-1"
        conversations={conversations}
        deleting={false}
        loading={false}
        runningConversationIds={new Set()}
        search=""
        variant="mobile"
        onBack={onBack}
        onDeleteMany={vi.fn(async () => {})}
        onRename={vi.fn()}
        onSearch={vi.fn()}
        onSelect={onSelect}
      />,
    )

    expect(screen.getAllByRole('button', { name: '操作' })[0]).toHaveClass('opacity-100')
    await user.click(screen.getByRole('button', { name: /构建失败诊断/ }))
    expect(onSelect).toHaveBeenCalledWith('conversation-1')
    expect(screen.getByRole('heading', { name: '会话' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '管理' }))
    expect(screen.getByText('选择会话')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '返回' }))
    expect(onBack).toHaveBeenCalledOnce()
    // 移动端半屏覆盖不再有“关闭整个助手”的按钮，避免误触
    expect(screen.queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()
  })

  it('provides an explicit return action in the compact desktop drawer', async () => {
    await i18next.changeLanguage('zh-CN')
    const user = userEvent.setup()
    const onBack = vi.fn()
    render(
      <AIConversationList
        conversations={conversations}
        deleting={false}
        loading={false}
        runningConversationIds={new Set()}
        search=""
        variant="drawer"
        onBack={onBack}
        onDeleteMany={vi.fn(async () => {})}
        onRename={vi.fn()}
        onSearch={vi.fn()}
        onSelect={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '返回' }))
    expect(onBack).toHaveBeenCalledOnce()
  })

  it('shows distinct empty and filtered states', async () => {
    await i18next.changeLanguage('zh-CN')
    const { rerender } = render(
      <AIConversationList
        conversations={[]}
        deleting={false}
        loading={false}
        runningConversationIds={new Set()}
        search=""
        onDeleteMany={vi.fn(async () => {})}
        onRename={vi.fn()}
        onSearch={vi.fn()}
        onSelect={vi.fn()}
      />,
    )
    expect(screen.getByText('暂无会话')).toBeInTheDocument()

    rerender(
      <AIConversationList
        conversations={[]}
        deleting={false}
        loading={false}
        runningConversationIds={new Set()}
        search="missing"
        onDeleteMany={vi.fn(async () => {})}
        onRename={vi.fn()}
        onSearch={vi.fn()}
        onSelect={vi.fn()}
      />,
    )
    expect(screen.getByText('没有匹配的会话')).toBeInTheDocument()
  })

  it('provides a reliable manual fallback for loading conversations beyond the first page', async () => {
    await i18next.changeLanguage('zh-CN')
    const user = userEvent.setup()
    const onLoadMore = vi.fn(async () => {})
    render(
      <AIConversationList
        conversations={conversations}
        deleting={false}
        hasMore
        loading={false}
        loadingMore={false}
        runningConversationIds={new Set()}
        search=""
        onDeleteMany={vi.fn(async () => {})}
        onLoadMore={onLoadMore}
        onRename={vi.fn()}
        onSearch={vi.fn()}
        onSelect={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '加载更多会话' }))
    expect(onLoadMore).toHaveBeenCalledOnce()
  })
})
