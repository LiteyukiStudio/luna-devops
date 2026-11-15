import type { DeploymentTargetMetrics } from '@/api'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { deploymentTargetMetricsStreamUrl } from '@/api'
import { StatusValueBadge } from '@/components/common/status-badge'
import { createTracedEventSource } from '@/lib/telemetry'
import { cn } from '@/lib/utils'
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
  const memoryLabel = `${formatMetricsBytes(metrics.memoryUsageBytes, locale)} / ${formatMetricsBytes(metrics.memoryLimitBytes, locale)}`

  if (detailed) {
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
            perReplicaLim={formatMetricsCPU(metrics.replicaCpuLimitMilli, locale)}
            perReplicaReq={formatMetricsCPU(metrics.replicaCpuRequestMilli, locale)}
            perReplicaLabel={t('deploymentsPage.metricsPerReplica')}
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
            perReplicaLim={formatMetricsBytes(metrics.replicaMemoryLimitBytes, locale)}
            perReplicaReq={formatMetricsBytes(metrics.replicaMemoryRequestBytes, locale)}
            perReplicaLabel={t('deploymentsPage.metricsPerReplica')}
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

  return (
    <div className="grid min-w-36 gap-1 text-xs">
      <span className="text-muted-foreground">{t('deploymentsPage.metricsAllReplicas', { replicas: metrics.desiredReplicas })}</span>
      <MetricsRow label={t('deploymentsPage.metricsCpu')} value={formatMetricsPercent(metrics.cpuUsagePercent, locale)} />
      <MetricsRow label={t('deploymentsPage.metricsMemory')} value={memoryLabel} />
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

function MetricsRow({ label, value }: { label: string, value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
    </div>
  )
}
