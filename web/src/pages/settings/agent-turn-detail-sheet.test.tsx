import type { AgentObservabilityTraceDetail, AgentObservabilityTurn } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import i18next from '@/i18n'
import { availableToolNames } from './agent-available-tools'
import { AgentTurnDetailSheet } from './agent-turn-detail-sheet'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, getAgentObservabilityTrace: vi.fn() } }
})

const turn: AgentObservabilityTurn = {
  id: 'turn-1',
  conversationId: 'conversation-1',
  conversationTitle: 'Deployment diagnosis',
  user: { id: 'user-1', name: 'User', email: 'user@example.com', avatarUrl: '' },
  turnIndex: 0,
  status: 'completed',
  userMessage: 'Deploy the service',
  assistantMessage: 'Deployment completed',
  runId: 'run-1',
  traceId: 'trace-1',
  inputTokens: 10,
  outputTokens: 20,
  cacheReadInputTokens: 0,
  cacheWriteInputTokens: null,
  reasoningOutputTokens: 5,
  toolCallCount: 1,
  durationMs: 50,
  createdAt: '2026-08-13T00:00:00Z',
}

const detail: AgentObservabilityTraceDetail = {
  traceId: 'trace-1',
  durationMs: 50,
  spanCount: 2,
  errorCount: 0,
  usage: {
    inputTokens: 100,
    outputTokens: 40,
    cacheReadInputTokens: 25,
    cacheWriteInputTokens: 0,
    reasoningOutputTokens: 5,
    cacheHitRate: 25,
  },
  spans: [
    { spanId: 'external', parentSpanId: 'root', name: 'http.request', serviceName: 'luna-devops-api', kind: 'client', status: 'ok', startTimeUnixNano: '20', startOffsetMs: 20, durationMs: 5, attributes: {}, events: [], raw: { spanId: 'external' } },
    { spanId: 'root', parentSpanId: '', name: 'agent.run.execute', serviceName: 'luna-agent', kind: 'internal', status: 'ok', startTimeUnixNano: '10', startOffsetMs: 10, durationMs: 40, attributes: {}, events: [], raw: { spanId: 'root' } },
  ],
}

describe('available model tool inventory', () => {
  it('parses, deduplicates, and sorts the effective tool set', () => {
    expect(availableToolNames({
      name: 'agent.tools.available',
      attributes: { 'luna.agent.available_tool.names': '["listProjects","createGatewayRoute","listProjects"]' },
    })).toEqual(['createGatewayRoute', 'listProjects'])
  })

  it('rejects malformed or unrelated span attributes', () => {
    expect(availableToolNames({ name: 'agent.model.stream', attributes: { 'luna.agent.available_tool.names': '["listProjects"]' } })).toEqual([])
    expect(availableToolNames({ name: 'agent.tools.available', attributes: { 'luna.agent.available_tool.names': '{broken' } })).toEqual([])
  })
})

describe('agent turn detail diagnostic actions', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue(detail)
  })

  it('prominently displays the trace id and copies it from the header', async () => {
    const writeText = vi.fn(async (_value: string) => {})
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    renderSheet()

    expect(screen.getByText('trace-1')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '复制 Trace ID' }))

    await waitFor(() => expect(writeText).toHaveBeenCalledWith('trace-1'))
  })

  it('shows only typed usage for the current trace and never falls back to the turn snapshot', async () => {
    const { unmount } = renderSheet()
    expect(await screen.findByText('唤起 Agent')).toBeInTheDocument()
    expect(usageValue('输入')).toBe('100')
    expect(usageValue('缓存读取')).toBe('25')
    expect(usageValue('缓存命中率')).toBe('25%')
    unmount()

    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue({ ...detail, usage: null })
    renderSheet()
    expect(await screen.findByText('唤起 Agent')).toBeInTheDocument()
    expect(usageValue('输入')).toBe('—')
    expect(usageValue('输出')).toBe('—')
    expect(usageValue('缓存读取')).toBe('—')
    expect(usageValue('缓存写入')).toBe('—')
    expect(usageValue('推理输出')).toBe('—')
    expect(usageValue('缓存命中率')).toBe('—')
  })

  it('copies every span even when external services remain hidden', async () => {
    const writeText = vi.fn(async (_value: string) => {})
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    renderSheet()

    expect(await screen.findByText('唤起 Agent')).toBeInTheDocument()
    expect(screen.queryByText('访问外部服务')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '复制诊断 JSON' }))

    await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    const exported = JSON.parse(writeText.mock.calls[0][0])
    expect(exported.spans.map((span: { spanId: string }) => span.spanId)).toEqual(['root', 'external'])
  })

  it('downloads the same versioned diagnostic package as a JSON file', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:diagnostic')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    renderSheet()

    const downloadButton = await screen.findByRole('button', { name: '下载诊断 JSON' })
    await waitFor(() => expect(downloadButton).toBeEnabled())
    fireEvent.click(downloadButton)

    expect(createObjectURL).toHaveBeenCalledOnce()
    expect(click).toHaveBeenCalledOnce()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:diagnostic')
    const blob = createObjectURL.mock.calls[0][0] as Blob
    expect(JSON.parse(await blob.text())).toMatchObject({ schemaVersion: 1, kind: 'luna-devops.agent-turn-diagnostic', turn: { id: turn.id } })
    click.mockRestore()
    createObjectURL.mockRestore()
    revokeObjectURL.mockRestore()
  })

  it('renders the effective model tool inventory as compact capsules', async () => {
    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue({
      ...detail,
      spans: [
        ...detail.spans,
        {
          spanId: 'tools',
          parentSpanId: 'root',
          name: 'agent.tools.available',
          serviceName: 'luna-agent',
          kind: 'internal',
          status: 'ok',
          startTimeUnixNano: '15',
          startOffsetMs: 15,
          durationMs: 0.1,
          attributes: {
            'luna.agent.available_tool.count': '2',
            'luna.agent.available_tool.names': '["createGatewayRoute","listGatewayRoutes"]',
          },
          events: [],
          raw: { spanId: 'tools' },
        },
      ],
    })
    renderSheet()

    expect(await screen.findByText('下发模型工具 · 2 个')).toBeInTheDocument()
    expect(screen.getByText('createGatewayRoute')).toBeInTheDocument()
    expect(screen.getByText('listGatewayRoutes')).toBeInTheDocument()
  })

  it('shows official cache-write and unavailable-usage Span attributes', async () => {
    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue({
      ...detail,
      spans: [
        ...detail.spans,
        {
          spanId: 'model',
          parentSpanId: 'root',
          name: 'chat gpt-5',
          serviceName: 'luna-agent',
          kind: 'client',
          status: 'ok',
          startTimeUnixNano: '30',
          startOffsetMs: 30,
          durationMs: 10,
          attributes: {
            'gen_ai.operation.name': 'chat',
            'gen_ai.usage.cache_write.input_tokens': '1234',
            'luna.gen_ai.usage.status': 'unavailable',
            'luna.gen_ai.usage.unavailable_reason': 'missing_usage',
          },
          events: [],
          raw: { spanId: 'model' },
        },
      ],
    })
    renderSheet()

    fireEvent.click(await screen.findByRole('button', { name: /模型生成回复/ }))

    expect(screen.getByText('缓存写入输入')).toBeInTheDocument()
    expect(screen.getByText('1,234')).toBeInTheDocument()
    expect(screen.getByText('用量状态')).toBeInTheDocument()
    expect(screen.getByText('不可用')).toBeInTheDocument()
    expect(screen.getByText('用量不可用原因')).toBeInTheDocument()
    expect(screen.getByText('上游未返回用量')).toBeInTheDocument()
  })
})

function renderSheet() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentTurnDetailSheet turn={turn} onOpenChange={vi.fn()} />
    </QueryClientProvider>,
  )
}

function usageValue(label: string) {
  return screen.getByText(label).closest('div')?.querySelector('dd')?.textContent
}
