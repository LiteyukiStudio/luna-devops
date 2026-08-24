import type { AgentObservabilityConversation, AgentObservabilityConversationDetail } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import i18next from '@/i18n'
import { AgentConversationDetailSheet } from './agent-conversation-detail-sheet'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, getAgentObservabilityConversation: vi.fn() } }
})

const conversation: AgentObservabilityConversation = {
  id: 'conversation-1',
  title: 'Deployment diagnosis',
  user: { id: 'user-1', name: 'User', email: 'user@example.com', avatarUrl: '' },
  turnCount: 1,
  traceCount: 1,
  createdAt: '2026-08-13T00:00:00Z',
  updatedAt: '2026-08-13T00:01:00Z',
}

const detail: AgentObservabilityConversationDetail = {
  ...conversation,
  turns: [{
    id: 'turn-1',
    turnIndex: 0,
    status: 'completed',
    userMessage: 'Deploy the service',
    assistantMessage: 'Deployment completed',
    runId: 'run-1',
    traceId: '0123456789abcdef0123456789abcdef',
    inputTokens: 1200,
    outputTokens: 340,
    cacheReadInputTokens: 0,
    cacheWriteInputTokens: null,
    reasoningOutputTokens: 45,
    durationMs: 1000,
    createdAt: '2026-08-13T00:00:00Z',
    loops: [],
  }],
  turnPage: 1,
  turnPageSize: 20,
  totalTurnPages: 1,
}

describe('agent conversation observability detail', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
    vi.mocked(api.getAgentObservabilityConversation).mockResolvedValue(detail)
  })

  it('shows the five-field token breakdown for every conversation turn', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <AgentConversationDetailSheet
          conversation={conversation}
          turnPage={1}
          onOpenChange={vi.fn()}
          onTurnPageChange={vi.fn()}
          onViewTrace={vi.fn()}
        />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('第 1 轮')).toBeInTheDocument()
    expect(usageValue('输入 Token')).toBe('1,200')
    expect(usageValue('输出 Token')).toBe('340')
    expect(usageValue('缓存读取 Token')).toBe('0')
    expect(usageValue('缓存写入 Token')).toBe('—')
    expect(usageValue('推理输出 Token')).toBe('45')
  })
})

function usageValue(label: string) {
  return screen.getByText(label).closest('div')?.querySelector('dd')?.textContent
}
