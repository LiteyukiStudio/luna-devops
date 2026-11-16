import { useTranslation } from 'react-i18next'
import { formatMetricsBytes, formatMetricsPair, formatMetricsPercent } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { MetricsStreamFallback } from './deployment-target-metrics-fallback'
import { useDeploymentTargetMetricsStream } from './use-deployment-target-metrics'

/** 列表单元格使用的紧凑运行指标视图。 */
export function DeploymentTargetMetricsCell({ applicationId, enabled, projectId, targetId }: {
  applicationId: string
  enabled: boolean
  projectId: string
  targetId: string
}) {
  const { i18n, t } = useTranslation()
  const streamState = useDeploymentTargetMetricsStream({ applicationId, enabled, projectId, targetId })

  if (streamState.kind !== 'ready')
    return <MetricsStreamFallback state={streamState} />

  const { metrics } = streamState
  const locale = i18n.language
  const memoryLabel = formatMetricsPair(
    formatMetricsBytes(metrics.memoryUsageBytes, locale),
    formatMetricsBytes(metrics.memoryLimitBytes, locale),
  )

  return (
    <div className="grid min-w-36 gap-1 text-xs">
      <span className="text-muted-foreground">{t('deploymentsPage.metricsAllReplicas', { replicas: metrics.desiredReplicas })}</span>
      <MetricsRow label={t('deploymentsPage.metricsCpu')} value={formatMetricsPercent(metrics.cpuUsagePercent, locale)} />
      <MetricsRow label={t('deploymentsPage.metricsMemory')} value={memoryLabel} />
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
