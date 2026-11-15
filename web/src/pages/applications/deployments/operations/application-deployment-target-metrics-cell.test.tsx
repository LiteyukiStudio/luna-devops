import type { DeploymentTargetMetrics } from '@/api'
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DeploymentTargetMetricsCell } from './application-deployment-target-metrics-cell'
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

describe('deployment target metrics cell', () => {
  beforeEach(() => {
    FakeEventSource.instances = []
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('keeps compact table cell layout when detailed is false', () => {
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
    expect(screen.queryByText('Live usage')).not.toBeInTheDocument()
  })

  it('renders dual resource ring cards in detailed mode with tiered colors and request markers', () => {
    const { container } = render(
      <DeploymentTargetMetricsCell
        applicationId="app_1"
        detailed
        enabled
        projectId="prj_1"
        targetId="tgt_1"
      />,
    )

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
    expect(screen.getByText(/\(5%\)/)).toBeInTheDocument()
    expect(screen.getByText(/\(15%\)/)).toBeInTheDocument()
    expect(screen.getByText('50m / 1')).toBeInTheDocument()
    expect(screen.getByText('1.2Gi / 8Gi')).toBeInTheDocument()

    expect(container.querySelector('.text-warning')).toBeInTheDocument()
    expect(container.querySelector('.text-danger')).toBeInTheDocument()
    expect(container.querySelectorAll('line')).toHaveLength(2)
  })

  it('renders unavailable badge and disabled fallback in detailed mode', () => {
    const { rerender } = render(
      <DeploymentTargetMetricsCell
        applicationId="app_1"
        detailed
        enabled
        projectId="prj_1"
        targetId="tgt_1"
      />,
    )

    act(() => {
      FakeEventSource.instances[0].emit('metrics', {
        ...sampleMetrics,
        available: false,
        status: 'unavailable',
      })
    })

    expect(screen.getByText('Unavailable')).toBeInTheDocument()

    rerender(
      <DeploymentTargetMetricsCell
        applicationId="app_1"
        detailed
        enabled={false}
        projectId="prj_1"
        targetId="tgt_1"
      />,
    )

    expect(screen.getByText('-')).toBeInTheDocument()
  })
})
