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
  toolCallCount: 1,
  durationMs: 50,
  createdAt: '2026-08-13T00:00:00Z',
}

const detail: AgentObservabilityTraceDetail = {
  traceId: 'trace-1',
  durationMs: 50,
  spanCount: 2,
  errorCount: 0,
  spans: [
    { spanId: 'external', parentSpanId: 'root', name: 'http.request', serviceName: 'luna-devops-api', kind: 'client', status: 'ok', startTimeUnixNano: '20', startOffsetMs: 20, durationMs: 5, attributes: {}, events: [], raw: { spanId: 'external' } },
    { spanId: 'root', parentSpanId: '', name: 'agent.run.execute', serviceName: 'luna-agent', kind: 'internal', status: 'ok', startTimeUnixNano: '10', startOffsetMs: 10, durationMs: 40, attributes: {}, events: [], raw: { spanId: 'root' } },
  ],
}

describe('agent turn detail diagnostic actions', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue(detail)
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
})

function renderSheet() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentTurnDetailSheet turn={turn} onOpenChange={vi.fn()} />
    </QueryClientProvider>,
  )
}
