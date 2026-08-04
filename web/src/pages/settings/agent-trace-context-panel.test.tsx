import type { AgentObservabilityTraceContext } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AgentTraceContextPanel } from './agent-trace-context-panel'
import '@/i18n'

const context: AgentObservabilityTraceContext = {
  conversation: {
    id: 'conversation-1',
    title: '检查发布故障',
    user: { id: 'user-1', name: '测试用户', email: 'user@example.com', avatarUrl: '' },
    turnCount: 1,
    traceCount: 1,
    createdAt: '2026-08-04T00:00:00Z',
    updatedAt: '2026-08-04T00:01:00Z',
  },
  turn: {
    id: 'turn-1',
    turnIndex: 0,
    status: 'completed',
    userMessage: '为什么发布失败？',
    assistantMessage: '镜像拉取失败。',
    runId: 'run-1',
    traceId: '0123456789abcdef0123456789abcdef',
    durationMs: 1200,
    createdAt: '2026-08-04T00:00:00Z',
    loops: [{
      loopIndex: 1,
      items: [
        {
          id: 'thinking-1',
          timelineIndex: 1,
          type: 'reasoning_summary',
          status: 'completed',
          text: '先检查发布状态。',
          createdAt: '2026-08-04T00:00:01Z',
        },
        {
          id: 'tool-item-1',
          timelineIndex: 2,
          type: 'tool_call',
          status: 'completed',
          text: '',
          createdAt: '2026-08-04T00:00:02Z',
          toolCall: {
            id: 'tool-call-1',
            operationId: 'getDeployment',
            status: 'succeeded',
            arguments: { deploymentId: 'deployment-1' },
            result: { status: 'failed' },
            traceId: '0123456789abcdef0123456789abcdef',
          },
        },
        {
          id: 'message-1',
          timelineIndex: 3,
          type: 'assistant_message',
          status: 'completed',
          text: '镜像拉取失败。',
          createdAt: '2026-08-04T00:00:03Z',
        },
      ],
    }],
  },
}

describe('agent trace context panel', () => {
  it('replays the linked conversation and selects its tool span', async () => {
    const onSelectToolCall = vi.fn()
    render(<AgentTraceContextPanel context={context} onSelectToolCall={onSelectToolCall} />)

    expect(screen.getByText('检查发布故障')).toBeInTheDocument()
    expect(screen.getByText('为什么发布失败？')).toBeInTheDocument()
    expect(screen.getByText('先检查发布状态。')).toBeInTheDocument()
    expect(screen.getByText('镜像拉取失败。')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /查看工具调用链|View tool trace/ }))
    expect(onSelectToolCall).toHaveBeenCalledWith(context.turn.loops[0].items[1].toolCall)
  })
})
