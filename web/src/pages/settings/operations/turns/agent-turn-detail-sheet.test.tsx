import type { AgentObservabilityTraceDetail, AgentObservabilityTurn } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import i18next from '@/i18n'
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

describe('agent turn detail explorer', () => {
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
    expect((await screen.findAllByText('唤起 Agent')).length).toBeGreaterThan(0)
    expect(usageValue('输入')).toBe('100')
    expect(usageValue('缓存读取')).toBe('25')
    expect(usageValue('缓存命中率')).toBe('25%')
    unmount()

    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue({ ...detail, usage: null })
    renderSheet()
    expect((await screen.findAllByText('唤起 Agent')).length).toBeGreaterThan(0)
    expect(usageValue('输入')).toBe('—')
    expect(usageValue('输出')).toBe('—')
    expect(usageValue('缓存读取')).toBe('—')
    expect(usageValue('缓存写入')).toBe('—')
    expect(usageValue('推理输出')).toBe('—')
    expect(usageValue('缓存命中率')).toBe('—')
  })

  it('copies every span even when infrastructure spans are hidden in AI execution flow view', async () => {
    const writeText = vi.fn(async (_value: string) => {})
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    renderSheet()

    expect((await screen.findAllByText('唤起 Agent')).length).toBeGreaterThan(0)
    expect(screen.queryByText('http.request')).not.toBeInTheDocument()
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

  it('renders baseline context once at the top and inspects per-step delta vs full context on demand', async () => {
    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue({
      ...detail,
      spans: [
        ...detail.spans,
        {
          spanId: 'llm-2',
          parentSpanId: 'root',
          name: 'chat deepseek-chat',
          serviceName: 'luna-agent',
          kind: 'client',
          status: 'ok',
          startTimeUnixNano: '30',
          startOffsetMs: 30,
          durationMs: 18,
          attributes: {
            'gen_ai.operation.name': 'chat',
            'gen_ai.system_instructions': '[{"type":"text","content":"你是 Luna DevOps 平台助手"}]',
            'gen_ai.input.messages': JSON.stringify([
              { role: 'user', parts: [{ type: 'text', content: '上一轮历史提问' }] },
              { role: 'assistant', parts: [{ type: 'text', content: '上一轮历史回答' }] },
              { role: 'user', parts: [{ type: 'text', content: '排查 user-api 重启' }] },
              { role: 'assistant', parts: [{ type: 'tool_call', id: 'call_1', name: 'listApplications', arguments: {} }] },
              { role: 'tool', parts: [{ type: 'tool_call_response', id: 'call_1', response: { status: 'OOMKilled' } }] },
            ]),
            'gen_ai.tool.definitions': JSON.stringify([
              { type: 'function', name: 'getDeploymentTargetLogs', description: '读取部署目标日志', parameters: { type: 'object', properties: { tailLines: { type: 'integer' } } } },
            ]),
          },
          events: [],
          raw: { spanId: 'llm-2' },
        },
      ],
      flow: {
        contentCaptured: true,
        payloadBreakdown: {
          systemBytes: 400,
          toolSchemaBytes: 800,
          historyBytes: 1200,
          userInputBytes: 200,
          toolResultBytes: 600,
          modelOutputBytes: 300,
        },
        initialContext: {
          systemInstructions: '你是 Luna DevOps 平台助手',
          pageContext: { projectId: 'proj_core' },
          historyTurnCount: 1,
          historyMessages: [
            { role: 'user', text: '上一轮历史提问' },
            { role: 'assistant', text: '上一轮历史回答' },
          ],
          initialTools: ['listApplications'],
        },
        nodes: [
          {
            spanId: 'root',
            parentNodeId: '',
            depth: 0,
            kind: 'agent',
            name: 'Luna Agent',
            spanName: 'invoke_agent Luna Agent',
            status: 'ok',
            startTimeUnixNano: '10',
            startOffsetMs: 10,
            durationMs: 40,
            inputPreview: { role: 'user', text: '排查 user-api 重启' },
            outputPreview: { role: 'assistant', text: '已定位 OOMKilled' },
          },
          {
            spanId: 'llm-2',
            parentNodeId: 'root',
            depth: 1,
            kind: 'llm',
            stepIndex: 2,
            purpose: 'assistant',
            name: 'deepseek-chat',
            spanName: 'chat deepseek-chat',
            status: 'ok',
            startTimeUnixNano: '30',
            startOffsetMs: 30,
            durationMs: 18,
            inputTokens: 1600,
            outputTokens: 90,
            omittedPrefixCount: 4,
            inputPreview: { role: 'tool', toolNames: ['listApplications'], text: 'OOMKilled', itemCount: 1 },
            outputPreview: { role: 'assistant', text: '已定位 OOMKilled' },
            deltaMessages: [
              { role: 'tool', toolCallId: 'call_1', toolName: 'listApplications', toolResult: { status: 'OOMKilled' } },
            ],
            outputMessage: {
              role: 'assistant',
              reasoning: '分析 Pod 退出码 137',
              text: '已定位 OOMKilled，建议调高内存配额。',
            },
            addedTools: ['getDeploymentTargetLogs'],
          },
        ],
      },
    })

    renderSheet()

    // Initial context banner is collapsed at top by default (history is NOT dumped into step delta!)
    expect(await screen.findByText('本轮初始上下文基线')).toBeInTheDocument()
    expect(screen.queryByText('上一轮历史提问')).not.toBeInTheDocument()
    expect(screen.getByText('已省略前序 4 条未变更上下文消息')).toBeInTheDocument()
    expect(screen.getByText('分析 Pod 退出码 137')).toBeInTheDocument()

    // Expand initial context banner once at the top
    fireEvent.click(screen.getByRole('button', { name: /本轮初始上下文基线/ }))
    expect(screen.getByText('上一轮历史提问')).toBeInTheDocument()

    // Switch inspector to Full Context & expand omitted prefix + single tool schema on demand
    fireEvent.click(screen.getByRole('button', { name: '完整上下文' }))
    fireEvent.click(screen.getByRole('button', { name: '展开前序 4 条前置上下文消息' }))
    expect(screen.getAllByText(/排查 user-api 重启/).length).toBeGreaterThan(0)

    // Switch to Span Waterfall view to inspect underlying HTTP span
    fireEvent.click(screen.getByRole('button', { name: 'Span 瀑布图' }))
    expect(screen.getByText('http.request')).toBeInTheDocument()
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
