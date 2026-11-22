import type { DeploymentTargetMetrics } from '@/api'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { DeploymentTargetMetricsCell } from './application-deployment-target-metrics-cell'
import {
  DeploymentTargetMetricsOverview,
  MetricsStatusPill,
  PodResourceMiniRing,
} from './deployment-target-metrics-cards'
import { useDeploymentTargetMetricsStream } from './use-deployment-target-metrics'
import '@/i18n'

type Listener = (event: MessageEvent) => void

class FakeEventSource {
  static instances: FakeEventSource[] = []
  listeners: Record<string, Listener[]> = {}
  closed = false
  url: string

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: Listener) {
    this.listeners[type] ??= []
    this.listeners[type].push(listener)
  }

  removeEventListener(type: string, listener: Listener) {
    this.listeners[type] = (this.listeners[type] ?? []).filter(item => item !== listener)
  }

  emit(type: string, data?: unknown) {
    const event = { data: data === undefined ? '' : JSON.stringify(data) } as MessageEvent
    for (const listener of this.listeners[type] ?? [])
      listener(event)
  }

  close() {
    this.closed = true
  }
}

vi.mock('@/lib/telemetry', () => ({
  createTracedEventSource: vi.fn((url: string) => new FakeEventSource(url) as unknown as EventSource),
}))

const sampleMetrics: DeploymentTargetMetrics = {
  available: true,
  availableReplicas: 1,
  configuredReplicas: 1,
  containerCount: 1,
  cpuLimitMilli: 1000,
  cpuRequestMilli: 50,
  cpuUsageMilli: 1,
  cpuUsagePercent: 0.1,
  desiredReplicas: 1,
  memoryLimitBytes: 8 * 1024 ** 3,
  memoryRequestBytes: Math.round(1.2 * 1024 ** 3),
  memoryUsageBytes: Math.round(9.5 * 1024 ** 2),
  memoryUsagePercent: 0.1,
  podCount: 1,
  readyReplicas: 1,
  replicaCpuLimitMilli: 1000,
  replicaCpuRequestMilli: 50,
  replicaMemoryLimitBytes: 8 * 1024 ** 3,
  replicaMemoryRequestBytes: Math.round(1.2 * 1024 ** 3),
  status: 'ready',
  updatedAt: '2026-11-16T00:00:00Z',
}

function StreamedMetricsHarness({ enabled }: { enabled: boolean }) {
  const streamState = useDeploymentTargetMetricsStream({
    applicationId: 'app_1',
    enabled,
    projectId: 'prj_1',
    targetId: 'tgt_1',
  })
  return (
    <TooltipProvider>
      <MetricsStatusPill streamState={streamState} />
      <DeploymentTargetMetricsOverview streamState={streamState} />
    </TooltipProvider>
  )
}

describe('deployment target metrics cell, dual-ring overview, and pod mini-rings', () => {
  beforeEach(() => {
    FakeEventSource.instances = []
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders compact table cell layout in DeploymentTargetMetricsCell with unit merging', () => {
    render(
      <DeploymentTargetMetricsCell
        applicationId="app_1"
        enabled
        projectId="prj_1"
        targetId="tgt_1"
      />,
    )

    expect(screen.getByText('Connecting metrics')).toBeInTheDocument()

    act(() => {
      FakeEventSource.instances[0].emit('metrics', sampleMetrics)
    })

    expect(screen.getByText('Total across all 1 replicas')).toBeInTheDocument()
    expect(screen.getByText('0.1%')).toBeInTheDocument()
    expect(screen.getByText('9.5Mi / 8Gi')).toBeInTheDocument()

    act(() => {
      FakeEventSource.instances[0].emit('metrics', {
        ...sampleMetrics,
        memoryUsageBytes: Math.round(1.2 * 1024 ** 3),
      })
    })
    expect(screen.getByText('1.2 / 8Gi')).toBeInTheDocument()
  })

  it('renders dual resource ring cards with tiered colors, unit merging, request tick marks, and tooltip', async () => {
    const user = userEvent.setup()
    const { container } = render(<StreamedMetricsHarness enabled />)

    expect(screen.getByText('Connecting metrics')).toBeInTheDocument()

    act(() => {
      FakeEventSource.instances[0].emit('metrics', {
        ...sampleMetrics,
        cpuUsageMilli: 850,
        cpuUsagePercent: 85,
        memoryUsageBytes: 7.5 * 1024 ** 3,
        memoryUsagePercent: 93.8,
      })
    })

    expect(screen.getByText('Total across all 1 replicas')).toBeInTheDocument()
    expect(screen.getAllByText('Live usage')).toHaveLength(2)
    expect(screen.getByText('85%')).toBeInTheDocument()
    expect(screen.getByText('93.8%')).toBeInTheDocument()
    expect(screen.getByText('850m / 1')).toBeInTheDocument()
    expect(screen.getByText('7.5 / 8Gi')).toBeInTheDocument()
    expect(screen.getByText(/\(5%\)/)).toBeInTheDocument()
    expect(screen.getByText(/\(15%\)/)).toBeInTheDocument()
    expect(screen.getByText('50m / 1')).toBeInTheDocument()
    expect(screen.getByText('1.2 / 8Gi')).toBeInTheDocument()

    expect(container.querySelector('.text-warning')).toBeInTheDocument()
    expect(container.querySelector('.text-danger')).toBeInTheDocument()
    expect(container.querySelectorAll('line')).toHaveLength(2)

    const helpButtons = screen.getAllByRole('button', { name: /Request floor/ })
    await user.hover(helpButtons[0])
    expect((await screen.findAllByText(/Sum of container requests across desired replicas/)).length).toBeGreaterThan(0)
  })

  it('keeps stable card layout and degrades gracefully when unavailable or disabled', () => {
    const { rerender } = render(<StreamedMetricsHarness enabled />)

    act(() => {
      FakeEventSource.instances[0].emit('metrics', {
        ...sampleMetrics,
        available: false,
        status: 'unavailable',
      })
    })

    expect(screen.getByText('Metrics unavailable')).toBeInTheDocument()
    expect(screen.getAllByText('Live usage')).toHaveLength(2)

    rerender(<StreamedMetricsHarness enabled={false} />)
    expect(screen.queryByText('Metrics unavailable')).not.toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(4)
  })

  it('renders PodResourceMiniRing with inline 20px ring, merged units, and unavailable fallback', () => {
    const { container } = render(
      <div>
        <PodResourceMiniRing label="CPU" limitText="1" locale="en-US" percent={94} usedText="940m" />
        <PodResourceMiniRing label="Memory" limitText="2Gi" locale="en-US" percent={81} usedText="1.62Gi" />
        <PodResourceMiniRing label="CPU" limitText="1" locale="en-US" />
      </div>,
    )

    expect(screen.getByText('940m / 1')).toBeInTheDocument()
    expect(screen.getByText('(94%)')).toBeInTheDocument()
    expect(screen.getByText('1.62 / 2Gi')).toBeInTheDocument()
    expect(screen.getByText('(81%)')).toBeInTheDocument()
    expect(screen.getByText('-')).toBeInTheDocument()
    expect(container.querySelector('.text-danger')).toBeInTheDocument()
    expect(container.querySelector('.text-warning')).toBeInTheDocument()
  })

  it('returns disabled when targetId is empty, transitions to unavailable on malformed SSE JSON, and clears stale metrics when toggled off and on', () => {
    const { rerender } = render(
      <DeploymentTargetMetricsCell
        applicationId="app_1"
        enabled
        projectId="prj_1"
        targetId=""
      />,
    )
    expect(screen.queryByText('Connecting metrics')).not.toBeInTheDocument()
    expect(FakeEventSource.instances).toHaveLength(0)

    const { rerender: rerenderHarness } = render(<StreamedMetricsHarness enabled />)
    expect(screen.getByText('Connecting metrics')).toBeInTheDocument()

    // Malformed SSE payload must degrade to unavailable rather than staying in infinite connecting state
    act(() => {
      for (const listener of FakeEventSource.instances[0].listeners.metrics ?? [])
        listener({ data: '{invalid-json' } as MessageEvent)
    })
    expect(screen.getByText('Metrics unavailable')).toBeInTheDocument()

    // Valid SSE payload populates metrics
    act(() => {
      FakeEventSource.instances[0].emit('metrics', sampleMetrics)
    })
    expect(screen.getByText('9.5Mi / 8Gi')).toBeInTheDocument()

    // Toggling enabled true -> false -> true must not replay stale metrics before the new stream delivers data
    rerender(<div />)
    rerenderHarness(<StreamedMetricsHarness enabled={false} />)
    expect(screen.queryByText('9.5Mi / 8Gi')).not.toBeInTheDocument()

    rerenderHarness(<StreamedMetricsHarness enabled />)
    expect(screen.getByText('Connecting metrics')).toBeInTheDocument()
    expect(screen.queryByText('9.5Mi / 8Gi')).not.toBeInTheDocument()
  })
})
