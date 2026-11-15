import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { formatMetricsBytes, formatMetricsCPU, formatMetricsPercent } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { MetricsStreamFallback } from './deployment-target-metrics-fallback'
import { useDeploymentTargetMetricsStream } from './use-deployment-target-metrics'

/** 详情抽屉使用的双圆环运行指标卡（CPU 与内存并排展示）。 */
export function DeploymentTargetMetricsCards({ applicationId, enabled, projectId, targetId }: {
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

  return (
    <div aria-label={t('deploymentsPage.runtimeMetrics')} className="grid gap-3">
      <div className="text-xs text-muted-foreground">
        {t('deploymentsPage.metricsAllReplicas', { replicas: metrics.desiredReplicas })}
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <ResourceMetricCard
          limitText={formatMetricsCPU(metrics.cpuLimitMilli, locale)}
          limitTotal={metrics.cpuLimitMilli}
          liveUsageLabel={t('deploymentsPage.metricsLiveUsage')}
          locale={locale}
          perReplicaLabel={t('deploymentsPage.metricsPerReplica')}
          perReplicaLim={formatMetricsCPU(metrics.replicaCpuLimitMilli, locale)}
          perReplicaReq={formatMetricsCPU(metrics.replicaCpuRequestMilli, locale)}
          percent={metrics.cpuUsagePercent}
          requestFloorLabel={t('deploymentsPage.metricsRequestFloor')}
          requestText={formatMetricsCPU(metrics.cpuRequestMilli, locale)}
          requestTotal={metrics.cpuRequestMilli}
          title={t('deploymentsPage.metricsCpu')}
          usedText={formatMetricsCPU(metrics.cpuUsageMilli, locale)}
        />
        <ResourceMetricCard
          limitText={formatMetricsBytes(metrics.memoryLimitBytes, locale)}
          limitTotal={metrics.memoryLimitBytes}
          liveUsageLabel={t('deploymentsPage.metricsLiveUsage')}
          locale={locale}
          perReplicaLabel={t('deploymentsPage.metricsPerReplica')}
          perReplicaLim={formatMetricsBytes(metrics.replicaMemoryLimitBytes, locale)}
          perReplicaReq={formatMetricsBytes(metrics.replicaMemoryRequestBytes, locale)}
          percent={metrics.memoryUsagePercent}
          requestFloorLabel={t('deploymentsPage.metricsRequestFloor')}
          requestText={formatMetricsBytes(metrics.memoryRequestBytes, locale)}
          requestTotal={metrics.memoryRequestBytes}
          title={t('deploymentsPage.metricsMemory')}
          usedText={formatMetricsBytes(metrics.memoryUsageBytes, locale)}
        />
      </div>
    </div>
  )
}

function ResourceMetricCard({
  limitText,
  limitTotal,
  liveUsageLabel,
  locale,
  perReplicaLabel,
  perReplicaLim,
  perReplicaReq,
  percent,
  requestFloorLabel,
  requestText,
  requestTotal,
  title,
  usedText,
}: {
  limitText: string
  limitTotal: number
  liveUsageLabel: string
  locale: string
  perReplicaLabel: string
  perReplicaLim: string
  perReplicaReq: string
  percent: number | undefined
  requestFloorLabel: string
  requestText: string
  requestTotal: number
  title: string
  usedText: string
}) {
  const requestRatio = limitTotal > 0 && requestTotal > 0 && requestTotal < limitTotal
    ? requestTotal / limitTotal
    : null
  const requestPercentText = limitTotal > 0 && requestTotal > 0
    ? `${Math.round((requestTotal / limitTotal) * 100).toLocaleString(locale)}%`
    : null

  return (
    <div className="flex items-center gap-4 rounded-xl border border-border bg-surface-subtle/40 p-4">
      <ResourceUsageRing
        centerBottom={title}
        centerTop={formatMetricsPercent(percent, locale)}
        percent={percent}
        requestRatio={requestRatio}
      />
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex items-baseline justify-between gap-3">
          <span className="shrink-0 text-xs text-muted-foreground">{liveUsageLabel}</span>
          <span className="font-mono text-sm font-semibold tabular-nums">
            {usedText}
            {' '}
            <span className="text-xs font-normal text-muted-foreground">
              /
              {' '}
              {limitText}
            </span>
          </span>
        </div>
        <div className="space-y-1 border-t border-border pt-1.5 text-xs">
          <div className="flex items-baseline justify-between gap-3">
            <span className="shrink-0 text-muted-foreground">{requestFloorLabel}</span>
            <span className="font-mono font-medium tabular-nums">
              {requestText}
              {requestPercentText !== null && (
                <span className="font-normal text-muted-foreground">
                  {' '}
                  (
                  {requestPercentText}
                  )
                </span>
              )}
            </span>
          </div>
          <div className="flex items-baseline justify-between gap-3">
            <span className="shrink-0 text-muted-foreground">{perReplicaLabel}</span>
            <span className="font-mono font-medium tabular-nums">
              {perReplicaReq}
              {' '}
              /
              {' '}
              {perReplicaLim}
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}

function ResourceUsageRing({
  centerBottom,
  centerTop,
  percent,
  requestRatio,
}: {
  centerBottom: string
  centerTop: string
  percent: number | undefined
  requestRatio: number | null
}) {
  const size = 76
  const stroke = 7
  const radius = (size - stroke) / 2
  const circumference = 2 * Math.PI * radius
  const hasPercent = percent !== undefined && Number.isFinite(percent) && percent >= 0
  const clamped = hasPercent ? Math.min(100, Math.max(0, percent)) : 0
  // 极低非零负载保留至少 2% 弧度，避免 0.1% 看起来完全空环。
  const visualPct = hasPercent ? (clamped > 0 && clamped < 2 ? 2 : clamped) : 0
  const dashOffset = circumference - (visualPct / 100) * circumference

  const toneClass = !hasPercent
    ? 'text-muted-foreground'
    : percent >= 90
      ? 'text-danger'
      : percent >= 70
        ? 'text-warning'
        : 'text-success'

  let markerLine: { x1: string, x2: string, y1: string, y2: string } | null = null
  if (requestRatio !== null && requestRatio > 0 && requestRatio < 1) {
    const angle = (requestRatio * 360 - 90) * (Math.PI / 180)
    const center = size / 2
    markerLine = {
      x1: (center + (radius - stroke * 0.55) * Math.cos(angle)).toFixed(1),
      y1: (center + (radius - stroke * 0.55) * Math.sin(angle)).toFixed(1),
      x2: (center + (radius + stroke * 0.55) * Math.cos(angle)).toFixed(1),
      y2: (center + (radius + stroke * 0.55) * Math.sin(angle)).toFixed(1),
    }
  }

  return (
    <div className="relative inline-flex size-[76px] shrink-0 items-center justify-center">
      <svg aria-hidden="true" className="size-[76px] -rotate-90" viewBox={`0 0 ${size} ${size}`}>
        <circle
          className="text-border/60"
          cx={size / 2}
          cy={size / 2}
          fill="none"
          r={radius}
          stroke="currentColor"
          strokeWidth={stroke}
        />
        {hasPercent && (
          <circle
            className={cn('transition-[stroke-dashoffset] duration-200 motion-reduce:transition-none', toneClass)}
            cx={size / 2}
            cy={size / 2}
            fill="none"
            r={radius}
            stroke="currentColor"
            strokeDasharray={circumference.toFixed(2)}
            strokeDashoffset={dashOffset.toFixed(2)}
            strokeLinecap="round"
            strokeWidth={stroke}
          />
        )}
      </svg>
      {markerLine && (
        <svg aria-hidden="true" className="pointer-events-none absolute inset-0 size-[76px]" viewBox={`0 0 ${size} ${size}`}>
          <line
            className="text-muted-foreground/70"
            stroke="currentColor"
            strokeLinecap="round"
            strokeWidth="1.5"
            x1={markerLine.x1}
            x2={markerLine.x2}
            y1={markerLine.y1}
            y2={markerLine.y2}
          />
        </svg>
      )}
      <div className="absolute inset-0 flex flex-col items-center justify-center text-center leading-tight">
        <span className={cn('text-sm font-semibold tabular-nums', hasPercent ? 'text-foreground' : 'text-muted-foreground')}>
          {centerTop}
        </span>
        <span className="text-[11px] text-muted-foreground">{centerBottom}</span>
      </div>
    </div>
  )
}
