import type { AgentObservabilityConversationLoop } from '@/api'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentConversationLoopView } from './agent-conversation-loop'
import '@/i18n'

describe('agent conversation loop view', () => {
  it('renders progress items instead of dropping them from the admin view', () => {
    const loop: AgentObservabilityConversationLoop = {
      loopIndex: 1,
      items: [{
        id: 'progress-1',
        timelineIndex: 0,
        type: 'progress',
        status: 'completed',
        text: '正在检查部署状态。',
        createdAt: '2026-10-20T00:00:00Z',
      }],
    }

    render(<AgentConversationLoopView loop={loop} onViewTrace={vi.fn()} />)

    expect(screen.getByText(/Execution progress|执行过程|執行過程|実行過程|실행 과정/)).toBeInTheDocument()
    expect(screen.getByText('正在检查部署状态。')).toBeInTheDocument()
  })
})
