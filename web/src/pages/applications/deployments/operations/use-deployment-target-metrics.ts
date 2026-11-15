import type { DeploymentTargetMetrics } from '@/api'
import { useEffect, useState } from 'react'
import { deploymentTargetMetricsStreamUrl } from '@/api'
import { createTracedEventSource } from '@/lib/telemetry'

export type MetricsStreamState
  = | { kind: 'disabled' }
    | { kind: 'connecting' }
    | { kind: 'unavailable', status: DeploymentTargetMetrics['status'] }
    | { kind: 'ready', metrics: DeploymentTargetMetrics }

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
  const [metricsState, setMetricsState] = useState<{ metrics: DeploymentTargetMetrics | null, targetId: string } | null>(null)
  const metrics = metricsState?.targetId === targetId ? metricsState.metrics : null

  useEffect(() => {
    if (!enabled || !projectId || !applicationId || !targetId)
      return
    const source = createTracedEventSource(deploymentTargetMetricsStreamUrl(projectId, applicationId, targetId), { withCredentials: true }, 'runtime.metrics.stream')
    const handleMetrics = (event: MessageEvent) => {
      try {
        setMetricsState({ metrics: JSON.parse(event.data) as DeploymentTargetMetrics, targetId })
      }
      catch {
        setMetricsState({ metrics: null, targetId })
      }
    }
    const handleError = () => {
      setMetricsState({
        metrics: {
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
          reason: 'stream_disconnected',
          status: 'unavailable',
          updatedAt: new Date().toISOString(),
        },
        targetId,
      })
    }
    source.addEventListener('metrics', handleMetrics)
    source.addEventListener('error', handleError)
    return () => {
      source.removeEventListener('metrics', handleMetrics)
      source.removeEventListener('error', handleError)
      source.close()
    }
  }, [applicationId, enabled, projectId, targetId])

  if (!enabled)
    return { kind: 'disabled' }
  if (!metrics)
    return { kind: 'connecting' }
  if (!metrics.available)
    return { kind: 'unavailable', status: metrics.status }
  return { kind: 'ready', metrics }
}
