import type { MetricsStreamState } from './use-deployment-target-metrics'
import { CircleHelp } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { UsageRingGraphic } from '@/components/common/usage-ring'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import {
  formatLiveUsageBytes,
  formatLiveUsageCPU,
  formatMetricsBytes,
  formatMetricsCPU,
  formatMetricsPair,
  formatMetricsPercent,
} from '@/pages/applications/deployments/application-deployments-panel-utils'

/** 「运行资源与实例」顶栏右侧的实时指标状态胶囊。 */
export function MetricsStatusPill({ streamState }: { streamState: MetricsStreamState }) {
  const { t } = useTranslation()
  if (streamState.kind === 'disabled')
    return null

  const label = streamState.kind === 'ready'
    ? t('deploymentsPage.metricsAllReplicas', { replicas: streamState.metrics.desiredReplicas })
    : streamState.kind === 'connecting'
      ? t('deploymentsPage.metricsConnecting')
      : t('deploymentsPage.metricsUnavailable')

  return (
    <span className="inline-flex items-center gap-1.5 rounded-md border border-border bg-surface-subtle px-2 py-0.5 text-xs text-muted-foreground">
      <span
        aria-hidden="true"
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          streamState.kind === 'ready' ? 'bg-success-solid' : 'bg-muted-foreground/60',
        )}
      />
      <span>{label}</span>
    </span>
  )
}

/** 详情抽屉使用的双圆环运行指标卡（CPU 与内存并排展示，连接中/不可用时保持卡片几何不跳变）。 */
export function DeploymentTargetMetricsOverview({ streamState }: { streamState: MetricsStreamState }) {
  const { i18n, t } = useTranslation()
  const locale = i18n.language
  const metrics = streamState.kind === 'ready' ? streamState.metrics : null

  return (
    <div aria-label={t('deploymentsPage.runtimeMetrics')} className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <ResourceMetricCard
        limitText={metrics ? formatMetricsCPU(metrics.cpuLimitMilli, locale) : '-'}
        limitTotal={metrics?.cpuLimitMilli ?? 0}
        liveUsageLabel={t('deploymentsPage.metricsLiveUsage')}
        locale={locale}
        perReplicaLabel={t('deploymentsPage.metricsPerReplica')}
        perReplicaLim={metrics ? formatMetricsCPU(metrics.replicaCpuLimitMilli, locale) : '-'}
        perReplicaReq={metrics ? formatMetricsCPU(metrics.replicaCpuRequestMilli, locale) : '-'}
        percent={metrics?.cpuUsagePercent}
        requestFloorLabel={t('deploymentsPage.metricsRequestFloor')}
        requestFloorTooltip={t('deploymentsPage.metricsRequestFloorTooltip')}
        requestText={metrics ? formatMetricsCPU(metrics.cpuRequestMilli, locale) : '-'}
        requestTotal={metrics?.cpuRequestMilli ?? 0}
        title={t('deploymentsPage.metricsCpu')}
        usedText={metrics ? formatLiveUsageCPU(metrics.cpuUsageMilli, locale) : '-'}
      />
      <ResourceMetricCard
        limitText={metrics ? formatMetricsBytes(metrics.memoryLimitBytes, locale) : '-'}
        limitTotal={metrics?.memoryLimitBytes ?? 0}
        liveUsageLabel={t('deploymentsPage.metricsLiveUsage')}
        locale={locale}
        perReplicaLabel={t('deploymentsPage.metricsPerReplica')}
        perReplicaLim={metrics ? formatMetricsBytes(metrics.replicaMemoryLimitBytes, locale) : '-'}
        perReplicaReq={metrics ? formatMetricsBytes(metrics.replicaMemoryRequestBytes, locale) : '-'}
        percent={metrics?.memoryUsagePercent}
        requestFloorLabel={t('deploymentsPage.metricsRequestFloor')}
        requestFloorTooltip={t('deploymentsPage.metricsRequestFloorTooltip')}
        requestText={metrics ? formatMetricsBytes(metrics.memoryRequestBytes, locale) : '-'}
        requestTotal={metrics?.memoryRequestBytes ?? 0}
        title={t('deploymentsPage.metricsMemory')}
        usedText={metrics ? formatLiveUsageBytes(metrics.memoryUsageBytes, locale) : '-'}
      />
    </div>
  )
}

/** 单 Pod 实例实时 CPU / 内存内联小圆环（20px 圆环 + 资源名 + 数值与百分比，不可用时降级为 `-`）。 */
export function PodResourceMiniRing({
  label,
  limitText,
  locale,
  percent,
  usedText,
}: {
  label: string
  limitText?: string
  locale: string
  percent?: number
  usedText?: string
}) {
  const hasUsage = usedText !== undefined && usedText !== '-'
  const valueText = hasUsage
    ? (limitText && limitText !== '-' ? formatMetricsPair(usedText, limitText) : usedText)
    : '-'
  const percentText = formatMetricsPercent(percent, locale)

  return (
    <div className="inline-flex items-center gap-2 text-xs">
      <UsageRingGraphic minVisualPercent={4} percent={hasUsage ? percent : undefined} size={20} strokeWidth={2.5} />
      <span className="shrink-0 text-muted-foreground">{label}</span>
      {hasUsage
        ? (
            <span className="shrink-0 font-mono font-medium tabular-nums text-foreground">
              {valueText}
              {' '}
              <span className="font-normal text-muted-foreground">{`(${percentText})`}</span>
            </span>
          )
        : <span className="shrink-0 font-mono tabular-nums text-muted-foreground">-</span>}
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
  requestFloorTooltip,
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
  requestFloorTooltip: string
  requestText: string
  requestTotal: number
  title: string
  usedText: string
}) {
  const { t } = useTranslation()
  const requestRatio = limitTotal > 0 && requestTotal > 0 && requestTotal < limitTotal
    ? requestTotal / limitTotal
    : null
  const requestPercentText = limitTotal > 0 && requestTotal > 0
    ? `${Math.round((requestTotal / limitTotal) * 100).toLocaleString(locale)}%`
    : null
  const livePair = usedText === '-' && limitText === '-' ? '-' : formatMetricsPair(usedText, limitText)
  const perReplicaPair = perReplicaReq === '-' && perReplicaLim === '-' ? '-' : formatMetricsPair(perReplicaReq, perReplicaLim)
  const hasPercent = percent !== undefined && Number.isFinite(percent) && percent >= 0

  return (
    <div className="flex items-center gap-4 rounded-xl border border-border bg-surface-subtle/50 p-4 shadow-surface-sm">
      <UsageRingGraphic
        centerContent={(
          <>
            <span className={cn('text-sm font-semibold tabular-nums', hasPercent ? 'text-foreground' : 'text-muted-foreground')}>
              {formatMetricsPercent(percent, locale)}
            </span>
            <span className="text-[11px] text-muted-foreground">{title}</span>
          </>
        )}
        minVisualPercent={2}
        percent={percent}
        requestRatio={requestRatio}
        size={76}
        strokeWidth={6.5}
      />
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex items-baseline justify-between gap-3">
          <span className="shrink-0 text-xs text-muted-foreground">{liveUsageLabel}</span>
          <span className="font-mono text-sm font-semibold tabular-nums text-foreground">{livePair}</span>
        </div>
        <div className="space-y-1 border-t border-border pt-1.5 text-xs">
          <div className="flex items-baseline justify-between gap-3">
            <span className="inline-flex shrink-0 items-center gap-1 text-muted-foreground">
              <span>{requestFloorLabel}</span>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    aria-label={`${requestFloorLabel}${t('common.helpSuffix')}`}
                    className="inline-flex size-3.5 items-center justify-center rounded-sm text-muted-foreground/80 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    type="button"
                  >
                    <CircleHelp aria-hidden="true" className="size-3" />
                  </button>
                </TooltipTrigger>
                <TooltipContent className="max-w-72 leading-5" side="top">
                  {requestFloorTooltip}
                </TooltipContent>
              </Tooltip>
            </span>
            <span className="font-mono font-medium tabular-nums text-foreground">
              {requestText}
              {requestPercentText !== null && (
                <span className="font-normal text-muted-foreground">{` (${requestPercentText})`}</span>
              )}
            </span>
          </div>
          <div className="flex items-baseline justify-between gap-3">
            <span className="shrink-0 text-muted-foreground">{perReplicaLabel}</span>
            <span className="font-mono font-medium tabular-nums text-foreground">{perReplicaPair}</span>
          </div>
        </div>
      </div>
    </div>
  )
}
