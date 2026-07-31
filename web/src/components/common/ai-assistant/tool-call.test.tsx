import type { ToolCallBlock } from './tool-call'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIToolCallCard } from './tool-call'

function toolBlock(status: ToolCallBlock['status']): ToolCallBlock {
  return {
    id: 'tool-item',
    turnId: 'turn-1',
    runId: 'run-1',
    index: 0,
    type: 'tool_call',
    toolCallId: 'tool-call-1',
    operationId: 'listApplications',
    status,
    arguments: {},
    uiActions: [],
  }
}

describe('ai assistant tool status icon', () => {
  it('animates only a genuinely running tool call', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={toolBlock('running')}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
        onMFA={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-tool-status-icon="running"]')).toHaveClass('animate-spin')
  })

  it('uses a non-loading danger icon after a tool call fails', () => {
    const { container } = render(
      <AIToolCallCard
        block={toolBlock('failed')}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
        onMFA={vi.fn(async () => {})}
      />,
    )

    const icon = container.querySelector('[data-ai-tool-status-icon="failed"]')
    expect(icon).toHaveClass('text-danger')
    expect(icon).not.toHaveClass('animate-spin')
    expect(container.querySelector('[data-ai-tool-status-icon="running"]')).not.toBeInTheDocument()
  })

  it('keeps the collapsed row compact and moves result details behind expansion', () => {
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('succeeded'),
          durationMs: 128,
          result: { summaryKey: 'aiAssistant.resultAvailable' },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
        onMFA={vi.fn(async () => {})}
      />,
    )

    const details = container.querySelector('details')
    const summary = container.querySelector('[data-ai-tool-summary]')
    expect(summary).toHaveClass('min-h-9')
    expect(summary).toHaveTextContent('listApplications')
    expect(summary).toHaveTextContent('已完成')
    expect(summary).not.toHaveTextContent('工具已返回结果')
    expect(details).not.toHaveAttribute('open')

    fireEvent.click(summary!)

    expect(details).toHaveAttribute('open')
    expect(screen.getByText('工具已返回结果')).toBeInTheDocument()
    expect(screen.getByText('耗时')).toBeInTheDocument()
    expect(screen.getByText('128 ms')).toBeInTheDocument()
  })

  it('shows a safe failure reason and request id without exposing backend details', async () => {
    await i18next.changeLanguage('zh-CN')
    render(
      <AIToolCallCard
        block={{
          ...toolBlock('failed'),
          errorCode: 'ai.tool_storage_unavailable',
          result: {
            summaryKey: 'ai.tool.result.completed',
            requestId: 'req_tool_failure',
            errorCode: 'ai.tool_storage_unavailable',
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
        onMFA={vi.fn(async () => {})}
      />,
    )

    fireEvent.click(screen.getByText('listApplications'))
    expect(screen.getByText('平台数据暂时无法读取，请稍后重试；如持续失败，请提供请求编号。')).toBeInTheDocument()
    expect(screen.getByText('请求编号')).toBeInTheDocument()
    expect(screen.getByText('req_tool_failure')).toBeInTheDocument()
    expect(screen.queryByText(/select .* from/i)).not.toBeInTheDocument()
  })

  it('renders bounded non-sensitive response data and card validation issues', async () => {
    await i18next.changeLanguage('zh-CN')
    render(
      <AIToolCallCard
        block={{
          ...toolBlock('failed'),
          operationId: 'prepare_interaction_cards',
          errorCode: 'ai.provider_invalid_tool_arguments',
          result: {
            summaryKey: 'aiAssistant.cards.failed',
            errorCode: 'ai.provider_invalid_tool_arguments',
            data: { items: [{ id: 'app-1', name: 'PostgreSQL' }], total: 1 },
            issues: [{
              code: 'too_small',
              path: 'cards.0.sections',
              message: 'Too small: expected array to have >=1 items',
            }],
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
        onMFA={vi.fn(async () => {})}
      />,
    )

    fireEvent.click(screen.getByText('prepare_interaction_cards'))
    expect(screen.getByText('模型生成的工具参数不符合要求，请查看校验详情。')).toBeInTheDocument()
    expect(screen.getByText('cards.0.sections')).toBeInTheDocument()
    expect(screen.getByText('Too small: expected array to have >=1 items')).toBeInTheDocument()
    expect(screen.getByText(/"name": "PostgreSQL"/)).toBeInTheDocument()
  })

  it('offers reject, approve, and current-run approve-all decisions for a bound high-risk call', async () => {
    await i18next.changeLanguage('zh-CN')
    const onApproval = vi.fn(async () => {})
    render(
      <AIToolCallCard
        block={{
          ...toolBlock('awaiting_approval'),
          argumentsHash: `sha256:${'a'.repeat(64)}`,
          expectedVersion: 2,
        }}
        onAction={vi.fn(async () => true)}
        onApproval={onApproval}
        onMFA={vi.fn(async () => {})}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '全部同意' }))
    await waitFor(() => expect(onApproval).toHaveBeenCalledWith(
      expect.objectContaining({ runId: 'run-1', toolCallId: 'tool-call-1' }),
      'approve_all',
      undefined,
    ))
  })
})
