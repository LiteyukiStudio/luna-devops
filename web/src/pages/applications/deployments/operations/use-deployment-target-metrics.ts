import type { DeploymentTargetMetrics } from '@/api'
import { useEffect, useState } from 'react'
import { deploymentTargetMetricsStreamUrl } from '@/api'
import { createTracedEventSource } from '@/lib/telemetry'

export type MetricsStreamState
  = | { kind: 'disabled' }
    | { kind: 'connecting' }
    | { kind: 'unavailable', status: DeploymentTargetMetrics['status'] }
    | { kind: 'ready', metrics: DeploymentTargetMetrics }

function createUnavailableMetrics(reason: string): DeploymentTargetMetrics {
  return {
    available: false,
    configuredReplicas: 0,
    containerCount: 0,
    cpuLimitMilli: 0,
    cpuRequestMilli: 0,
    cpuUsageMilli: 0,
    desiredReplicas: 0,
    readyReplicas: 0,
    availableReplicas: 0,
    memoryLimitBytes: 0,
    memoryRequestBytes: 0,
    memoryUsageBytes: 0,
    podCount: 0,
    replicaCpuLimitMilli: 0,
    replicaCpuRequestMilli: 0,
    replicaMemoryLimitBytes: 0,
    replicaMemoryRequestBytes: 0,
    reason,
    status: 'unavailable',
    updatedAt: new Date().toISOString(),
  }
}

/**
 * 订阅部署目标实时运行指标（SSE）；用量、request 保底与 limit 均为全部副本、全部容器的合计。
 */
export function useDeploymentTargetMetricsStream({
  applicationId,
  enabled,
  projectId,
  targetId,
}: {
  applicationId: string
  enabled: boolean
  projectId: string
  targetId: string
}): MetricsStreamState {
  const active = Boolean(enabled && projectId && applicationId && targetId)
  const streamKey = active ? `${projectId}:${applicationId}:${targetId}` : ''
  const [metricsState, setMetricsState] = useState<{ metrics: DeploymentTargetMetrics, streamKey: string } | null>(null)
  const metrics = active && metricsState?.streamKey === streamKey ? metricsState.metrics : null

  useEffect(() => {
    if (!active)
      return
    let disposed = false
    const source = createTracedEventSource(deploymentTargetMetricsStreamUrl(projectId, applicationId, targetId), { withCredentials: true }, 'runtime.metrics.stream')
    const handleMetrics = (event: MessageEvent) => {
      if (disposed)
        return
      try {
        setMetricsState({ metrics: JSON.parse(event.data) as DeploymentTargetMetrics, streamKey })
      }
      catch {
        setMetricsState({ metrics: createUnavailableMetrics('invalid_payload'), streamKey })
      }
    }
    const handleError = () => {
      if (disposed)
        return
      setMetricsState({ metrics: createUnavailableMetrics('stream_disconnected'), streamKey })
    }
    source.addEventListener('metrics', handleMetrics)
    source.addEventListener('error', handleError)
    return () => {
      disposed = true
      source.removeEventListener('metrics', handleMetrics)
      source.removeEventListener('error', handleError)
      source.close()
      setMetricsState(current => (current?.streamKey === streamKey ? null : current))
    }
  }, [active, applicationId, projectId, streamKey, targetId])

  if (!active)
    return { kind: 'disabled' }
  if (!metrics)
    return { kind: 'connecting' }
  if (!metrics.available)
    return { kind: 'unavailable', status: metrics.status }
  return { kind: 'ready', metrics }
}
