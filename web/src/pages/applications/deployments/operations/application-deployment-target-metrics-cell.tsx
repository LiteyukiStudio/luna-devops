import type { DeploymentTargetMetrics } from '@/api'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { deploymentTargetMetricsStreamUrl } from '@/api'
import { StatusValueBadge } from '@/components/common/status-badge'
import { createTracedEventSource } from '@/lib/telemetry'
import { formatMetricsBytes, formatMetricsCPU, formatMetricsPercent } from '@/pages/applications/deployments/application-deployments-panel-utils'

/**
 * 实时运行指标：用量、request 保底与 limit 都是全部副本、全部容器（含 sidecar）的合计，来自同一次
 * Kubernetes 观察；detailed 额外展示 request 保底与每副本 Pod 模板 request/limit。
 */
export function DeploymentTargetMetricsCell({ applicationId, detailed = false, enabled, projectId, targetId }: {
  applicationId: string
  detailed?: boolean
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
    return <span className="text-muted-foreground">-</span>
  if (!metrics)
    return <span className="text-xs text-muted-foreground">{t('deploymentsPage.metricsConnecting')}</span>
  if (!metrics.available)
    return <StatusValueBadge value={metrics.status} />

  const locale = i18n.language
  const cpuLabel = `${formatMetricsCPU(metrics.cpuUsageMilli, locale)} / ${formatMetricsCPU(metrics.cpuLimitMilli, locale)}`
  const memoryLabel = `${formatMetricsBytes(metrics.memoryUsageBytes, locale)} / ${formatMetricsBytes(metrics.memoryLimitBytes, locale)}`

  return (
    <div className="grid min-w-36 gap-1 text-xs">
      <span className="text-muted-foreground">{t('deploymentsPage.metricsAllReplicas', { replicas: metrics.desiredReplicas })}</span>
      <MetricsRow label={t('deploymentsPage.metricsCpu')} value={detailed ? `${cpuLabel} · ${formatMetricsPercent(metrics.cpuUsagePercent, locale)}` : formatMetricsPercent(metrics.cpuUsagePercent, locale)} />
      <MetricsRow label={t('deploymentsPage.metricsMemory')} value={detailed ? `${memoryLabel} · ${formatMetricsPercent(metrics.memoryUsagePercent, locale)}` : memoryLabel} />
      {detailed && (
        <>
          <MetricsRow
            label={t('deploymentsPage.metricsRequestFloor')}
            value={`${formatMetricsCPU(metrics.cpuRequestMilli, locale)} · ${formatMetricsBytes(metrics.memoryRequestBytes, locale)}`}
          />
          <span className="mt-1 text-muted-foreground">{t('deploymentsPage.metricsPerReplica')}</span>
          <MetricsRow
            label={t('deploymentsPage.metricsCpu')}
            value={`${formatMetricsCPU(metrics.replicaCpuRequestMilli, locale)} / ${formatMetricsCPU(metrics.replicaCpuLimitMilli, locale)}`}
          />
          <MetricsRow
            label={t('deploymentsPage.metricsMemory')}
            value={`${formatMetricsBytes(metrics.replicaMemoryRequestBytes, locale)} / ${formatMetricsBytes(metrics.replicaMemoryLimitBytes, locale)}`}
          />
        </>
      )}
    </div>
  )
}

function MetricsRow({ label, value }: { label: string, value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
    </div>
  )
}
