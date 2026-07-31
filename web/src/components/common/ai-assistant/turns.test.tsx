import type { AIBlock } from './state'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIAssistantTimeline } from './timeline'
import { groupAIAssistantBlocksByTurn } from './turns'

const blocks: AIBlock[] = [
  {
    id: 'final-message',
    turnId: 'turn-1',
    index: 3,
    type: 'message',
    role: 'assistant',
    status: 'completed',
    text: '最终答复',
    createdAt: '2026-08-01T09:03:00+08:00',
  },
  {
    id: 'user-message',
    turnId: 'turn-1',
    index: -1,
    type: 'message',
    role: 'user',
    status: 'completed',
    text: '检查项目状态',
    createdAt: '2026-08-01T09:02:00+08:00',
  },
  {
    id: 'tool-call',
    turnId: 'turn-1',
    runId: 'run-1',
    index: 2,
    type: 'tool_call',
    toolCallId: 'tool-call-1',
    operationId: 'listProjects',
    status: 'succeeded',
    arguments: {},
    result: { summaryKey: 'ai.tool.result.completed' },
    uiActions: [],
  },
  {
    id: 'thinking',
    turnId: 'turn-1',
    index: 0,
    type: 'thinking',
    status: 'completed',
    display: 'summary',
    text: '先分析当前项目',
  },
  {
    id: 'progress-message',
    turnId: 'turn-1',
    index: 1,
    type: 'message',
    role: 'assistant',
    status: 'completed',
    text: '正在查询项目状态',
    createdAt: '2026-08-01T09:02:30+08:00',
  },
]

describe('ai assistant turn topology', () => {
  it('groups one user input and all ordered response blocks into one turn', () => {
    const turns = groupAIAssistantBlocksByTurn(blocks)

    expect(turns).toHaveLength(1)
    expect(turns[0].userMessage?.id).toBe('user-message')
    expect(turns[0].responseBlocks.map(block => block.id)).toEqual([
      'thinking',
      'progress-message',
      'tool-call',
      'final-message',
    ])
  })

  it('renders interleaved thinking, messages and tools inside one assistant reply', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <MemoryRouter>
        <AIAssistantTimeline
          blocks={blocks}
          error={null}
          generating={false}
          loading={false}
          onAction={vi.fn(async () => true)}
          onApproval={vi.fn(async () => {})}
          onMFA={vi.fn(async () => {})}
          onResend={vi.fn()}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )

    expect(screen.getAllByText('检查项目状态')).toHaveLength(1)
    const replies = container.querySelectorAll('[data-ai-reply]')
    expect(replies).toHaveLength(1)
    expect(container.querySelector('[data-ai-user-message] [data-ai-message-group]')).toHaveClass('max-w-[78%]')
    expect(container.querySelector('[data-ai-reply] [data-ai-message-group]')).toHaveClass('max-w-[78%]')
    const replyText = replies[0].textContent ?? ''
    expect(replyText.indexOf('先分析当前项目')).toBeLessThan(replyText.indexOf('正在查询项目状态'))
    expect(replyText.indexOf('正在查询项目状态')).toBeLessThan(replyText.indexOf('listProjects'))
    expect(replyText.indexOf('listProjects')).toBeLessThan(replyText.indexOf('最终答复'))
  })

  it('keeps timestamps visible and exposes message actions inside the stable message group', async () => {
    await i18next.changeLanguage('zh-CN')
    const onResend = vi.fn()
    const writeText = vi.fn(async () => {})
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const { container } = render(
      <MemoryRouter>
        <AIAssistantTimeline
          blocks={blocks}
          error={null}
          generating={false}
          loading={false}
          onAction={vi.fn(async () => true)}
          onApproval={vi.fn(async () => {})}
          onMFA={vi.fn(async () => {})}
          onResend={onResend}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )

    expect(container.querySelectorAll('[data-ai-message-group]')).toHaveLength(2)
    expect(container.querySelectorAll('time')).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: '复制消息' })).toHaveLength(2)
    fireEvent.click(screen.getByRole('button', { name: '重新发送' }))
    expect(onResend).toHaveBeenCalledWith('检查项目状态')
    fireEvent.click(screen.getAllByRole('button', { name: '复制消息' })[1])
    expect(writeText).toHaveBeenCalledWith('正在查询项目状态\n\n最终答复')
  })

  it('hides internal maintenance tools without leaving an empty assistant bubble', () => {
    const { container } = render(
      <MemoryRouter>
        <AIAssistantTimeline
          blocks={[
            {
              id: 'user-only',
              turnId: 'turn-hidden-tool',
              index: -1,
              type: 'message',
              role: 'user',
              status: 'completed',
              text: '帮我看看',
              createdAt: '2026-08-01T09:04:00+08:00',
            },
            {
              id: 'rename-tool',
              turnId: 'turn-hidden-tool',
              runId: 'run-hidden-tool',
              index: 0,
              type: 'tool_call',
              toolCallId: 'rename-tool-call',
              operationId: 'rename_conversation',
              titleKey: 'aiAssistant.tools.renameConversation',
              status: 'succeeded',
              arguments: { title: '新的会话标题' },
              result: { summaryKey: 'aiAssistant.tools.renameConversationCompleted' },
              uiActions: [],
            },
          ]}
          error={null}
          generating={false}
          loading={false}
          onAction={vi.fn(async () => true)}
          onApproval={vi.fn(async () => {})}
          onMFA={vi.fn(async () => {})}
          onResend={vi.fn()}
          onRetry={vi.fn()}
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('帮我看看')).toBeInTheDocument()
    expect(screen.queryByText('更新会话名称')).not.toBeInTheDocument()
    expect(container.querySelector('[data-ai-reply]')).not.toBeInTheDocument()
  })
})
