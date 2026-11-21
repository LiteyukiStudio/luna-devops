import type { AgentObservabilityToolCall, AgentObservabilityToolSummary } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import i18next from '@/i18n'
import { AgentToolDetailSheet } from './agent-tool-detail-sheet'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listAgentObservabilityToolCalls: vi.fn(),
      getAgentObservabilityTrace: vi.fn(),
    },
  }
})

const summary: AgentObservabilityToolSummary = {
  operationId: 'listApplications',
  totalCalls: 4,
  succeededCalls: 3,
  failedCalls: 1,
  otherCalls: 0,
  successRate: 75,
  lastCalledAt: '2026-08-20T04:00:00Z',
}

const call: AgentObservabilityToolCall = {
  id: 'tool-1',
  operationId: 'listApplications',
  status: 'failed',
  arguments: { projectId: 'project-1' },
  result: { code: 'application.list_failed' },
  errorCode: 'application.list_failed',
  durationMs: 42,
  traceId: 'trace-tool-1',
  runId: 'run-1',
  turnId: 'turn-1',
  turnIndex: 1,
  conversationId: 'conversation-1',
  conversationTitle: 'Deployment diagnosis',
  user: { id: 'user-1', name: 'Admin', email: 'admin@example.com', avatarUrl: '' },
  createdAt: '2026-08-20T04:00:00Z',
}

describe('agent tool detail sheet', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
    vi.mocked(api.listAgentObservabilityToolCalls).mockResolvedValue({
      items: [call],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
  })

  it('shows period success and redacted call arguments and results', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <AgentToolDetailSheet range="24h" summary={summary} onOpenChange={vi.fn()} />
      </QueryClientProvider>,
    )

    expect(screen.getByText('75.0%')).toBeInTheDocument()
    expect(await screen.findByText('Deployment diagnosis')).toBeInTheDocument()
    expect(screen.getByText(/"projectId": "project-1"/)).toBeInTheDocument()
    expect(screen.getAllByText('application.list_failed').length).toBeGreaterThan(0)

    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue({
      traceId: 'trace-tool-1',
      durationMs: 42,
      spanCount: 1,
      errorCount: 1,
      usage: null,
      spans: [{
        spanId: 'tool-span-1',
        parentSpanId: '',
        name: 'execute_tool listApplications',
        serviceName: 'luna-agent',
        kind: 'internal',
        status: 'error',
        startTimeUnixNano: '100',
        startOffsetMs: 0,
        durationMs: 42,
        attributes: {
          'gen_ai.operation.name': 'execute_tool',
          'gen_ai.tool.name': 'listApplications',
          'gen_ai.tool.call.id': 'tool-1',
        },
        events: [],
        raw: { spanId: 'tool-span-1' },
      }],
    })
    fireEvent.click(screen.getByRole('button', { name: '查看工具调用链' }))
    expect(await screen.findByText('trace-tool-1')).toBeInTheDocument()
    expect(await screen.findByText('tool-span-1')).toBeInTheDocument()
  })
})
