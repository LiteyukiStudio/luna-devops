import type { AgentObservabilityTrace, AgentObservabilityTraceDetail } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import i18next from '@/i18n'
import { AgentTraceDetailSheet } from './agent-trace-detail-sheet'

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return { ...actual, api: { ...actual.api, getAgentObservabilityTrace: vi.fn() } }
})

const trace: AgentObservabilityTrace = {
  traceId: 'trace-1',
  rootServiceName: 'luna-agent',
  rootTraceName: 'invoke_agent Luna Agent',
  startTimeUnixNano: '1000000000',
  durationMs: 50,
}

const detail: AgentObservabilityTraceDetail = {
  traceId: trace.traceId,
  durationMs: 50,
  spanCount: 0,
  errorCount: 0,
  usage: {
    inputTokens: 200,
    outputTokens: 40,
    cacheReadInputTokens: 100,
    cacheWriteInputTokens: null,
    reasoningOutputTokens: 20,
    cacheHitRate: 50,
  },
  spans: [],
}

describe('agent trace detail usage', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValue(detail)
  })

  it('shows the typed usage for the current trace with its cache hit rate', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <AgentTraceDetailSheet trace={trace} onOpenChange={vi.fn()} />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('缓存命中率')).toBeInTheDocument()
    expect(usageValue('输入')).toBe('200')
    expect(usageValue('输出')).toBe('40')
    expect(usageValue('缓存读取')).toBe('100')
    expect(usageValue('缓存写入')).toBe('—')
    expect(usageValue('推理输出')).toBe('20')
    expect(usageValue('缓存命中率')).toBe('50%')
  })

  it('shows unavailable values instead of reusing unrelated usage when the trace aggregate is incomplete', async () => {
    vi.mocked(api.getAgentObservabilityTrace).mockResolvedValueOnce({ ...detail, usage: null })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={queryClient}>
        <AgentTraceDetailSheet trace={trace} onOpenChange={vi.fn()} />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('运行成功')).toBeInTheDocument()
    expect(usageValue('输入')).toBe('—')
    expect(usageValue('缓存读取')).toBe('—')
    expect(usageValue('缓存命中率')).toBe('—')
  })
})

function usageValue(label: string) {
  return screen.getByText(label).closest('div')?.querySelector('dd')?.textContent
}
