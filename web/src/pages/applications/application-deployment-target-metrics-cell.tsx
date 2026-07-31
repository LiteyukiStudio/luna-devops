import type { DeploymentTargetMetrics } from '@/api'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { deploymentTargetMetricsStreamUrl } from '@/api'
import { StatusValueBadge } from '@/components/common/status-badge'
import { createTracedEventSource } from '@/lib/telemetry'
import { formatMetricsBytes, formatMetricsPercent } from './application-deployments-panel-utils'

export function DeploymentTargetMetricsCell({ applicationId, enabled, projectId, targetId }: {
  applicationId: string
  enabled: boolean
  projectId: string
  targetId: string
}) {
  const { i18n, t } = useTranslation()
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
          containerCount: 0,
          cpuCapacityMilli: 0,
          cpuUsageMilli: 0,
          cpuUsagePercent: 0,
          memoryCapacityBytes: 0,
          memoryUsageBytes: 0,
          memoryUsagePercent: 0,
          podCount: 0,
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
    return <span className="text-muted-foreground">-</span>
  if (!metrics)
    return <span className="text-xs text-muted-foreground">{t('deploymentsPage.metricsConnecting')}</span>
  if (!metrics.available)
    return <StatusValueBadge value={metrics.status} />

  const memoryLabel = `${formatMetricsBytes(metrics.memoryUsageBytes, i18n.language)} / ${formatMetricsBytes(metrics.memoryCapacityBytes, i18n.language)}`

  return (
    <div className="grid min-w-36 gap-1 text-xs">
      <div className="flex items-center justify-between gap-3">
        <span className="text-muted-foreground">{t('deploymentsPage.metricsCpu')}</span>
        <span className="font-medium tabular-nums">{formatMetricsPercent(metrics.cpuUsagePercent, i18n.language)}</span>
      </div>
      <div className="flex items-center justify-between gap-3">
        <span className="text-muted-foreground">{t('deploymentsPage.metricsMemory')}</span>
        <span className="font-medium tabular-nums">{memoryLabel}</span>
      </div>
    </div>
  )
}
