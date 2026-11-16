import type { MetricsStreamState } from './use-deployment-target-metrics'
import type { DeploymentTargetRuntimeDiagnosticsState } from './use-deployment-target-runtime-diagnostics'
import type {
  DeploymentTargetEvent,
  DeploymentTargetPodMetrics,
  DeploymentTargetRuntimePod,
  Release,
  RuntimeContainerStatusSnapshot,
} from '@/api'
import { Activity, AlertTriangle, ArrowRight, Box, Eye, History, RefreshCw, Terminal } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime, formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  formatLiveUsageBytes,
  formatLiveUsageCPU,
  formatMetricsBytes,
  formatMetricsCPU,
} from '@/pages/applications/deployments/application-deployments-panel-utils'
import { PodResourceMiniRing } from './deployment-target-metrics-cards'

export function RuntimeEventsWarningBanner({
  latestReason,
  onViewEvents,
  warningCount,
}: {
  latestReason?: string
  onViewEvents: () => void
  warningCount: number
}) {
  const { t } = useTranslation()
  if (warningCount <= 0)
    return null

  return (
    <div
      className="flex flex-wrap items-center justify-between gap-3 rounded-control border border-warning-border bg-warning-subtle px-3.5 py-2.5 text-xs"
      role="status"
    >
      <div className="flex min-w-0 items-center gap-2">
        <AlertTriangle className="size-3.5 shrink-0 text-warning" aria-hidden="true" />
        <span className="font-semibold text-warning">
          {t('deploymentsPage.runtimeEventsWarningBanner', { count: warningCount })}
        </span>
        {latestReason && <span className="truncate text-muted-foreground">{latestReason}</span>}
      </div>
      <Button
        className="h-auto shrink-0 p-0 text-xs font-medium text-warning hover:bg-transparent hover:underline"
        size="sm"
        type="button"
        variant="ghost"
        onClick={onViewEvents}
      >
        {t('deploymentsPage.runtimeEventsViewAction')}
        <ArrowRight className="size-3.5" aria-hidden="true" />
      </Button>
    </div>
  )
}

export function DeploymentTargetRuntimePodsList({
  diagnostics,
  metricsStream,
  release,
  webConsoleEnabled,
  onOpenPodConsole,
  onViewPodLogs,
}: {
  diagnostics: DeploymentTargetRuntimeDiagnosticsState
  metricsStream: MetricsStreamState
  release?: Release
  webConsoleEnabled: boolean
  onOpenPodConsole?: (release: Release, pod: string) => void
  onViewPodLogs?: (release: Release, pod: string, previous: boolean) => void
}) {
  const { i18n, t } = useTranslation()
  const locale = i18n.language
  const metrics = metricsStream.kind === 'ready' ? metricsStream.metrics : null
  const podMetricsByName = useMemo(() => {
    const map = new Map<string, DeploymentTargetPodMetrics>()
    for (const item of metrics?.pods ?? [])
      map.set(item.podName, item)
    return map
  }, [metrics?.pods])

  if (!diagnostics.active)
    return null

  const replicaCpuLimitText = metrics ? formatMetricsCPU(metrics.replicaCpuLimitMilli, locale) : undefined
  const replicaMemoryLimitText = metrics ? formatMetricsBytes(metrics.replicaMemoryLimitBytes, locale) : undefined
  const listTitle = !diagnostics.podsLoading && !diagnostics.podsUnavailable
    ? t('deploymentsPage.runtimePodsListCount', { count: diagnostics.pods.length })
    : t('deploymentsPage.runtimePodsList')

  return (
    <div className="grid gap-2.5 border-t border-border pt-3.5">
      <div className="text-xs font-medium text-muted-foreground">{listTitle}</div>
      {diagnostics.podsLoading
        ? (
            <div className="grid gap-2">
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
            </div>
          )
        : diagnostics.podsUnavailable
          ? (
              <ErrorState
                description={t('deploymentsPage.runtimePodsUnavailable')}
                retrying={diagnostics.podsFetching}
                title={t('common.unavailable')}
                onRetry={diagnostics.refetchPods}
              />
            )
          : diagnostics.pods.length === 0
            ? (
                <EmptyState
                  description={t('deploymentsPage.runtimePodsEmptyDescription')}
                  icon={<Box className="size-5" aria-hidden="true" />}
                  title={t('deploymentsPage.runtimePodsEmpty')}
                  variant="plain"
                />
              )
            : (
                <div className="grid gap-2.5">
                  {diagnostics.pods.map(pod => (
                    <RuntimePodCard
                      key={pod.name}
                      locale={locale}
                      pod={pod}
                      podMetrics={podMetricsByName.get(pod.name)}
                      release={release}
                      replicaCpuLimitText={replicaCpuLimitText}
                      replicaMemoryLimitText={replicaMemoryLimitText}
                      webConsoleEnabled={webConsoleEnabled}
                      onOpenPodConsole={onOpenPodConsole}
                      onViewPodLogs={onViewPodLogs}
                    />
                  ))}
                </div>
              )}
    </div>
  )
}

export function DeploymentTargetRuntimeEventsSection({
  diagnostics,
  onToggleWarningOnly,
  warningOnly,
}: {
  diagnostics: DeploymentTargetRuntimeDiagnosticsState
  onToggleWarningOnly: () => void
  warningOnly: boolean
}) {
  const { t } = useTranslation()
  if (!diagnostics.active) {
    return (
      <section className="grid gap-3 rounded-container border border-border bg-surface-raised p-4 shadow-surface-sm">
        <h3 className="text-sm font-semibold text-foreground">{t('deploymentsPage.runtimeEventsSection')}</h3>
        <EmptyState
          description={t('deploymentsPage.runtimeEventsEmptyDescription')}
          icon={<Activity className="size-5" aria-hidden="true" />}
          title={t('deploymentsPage.runtimeEventsEmpty')}
          variant="plain"
        />
      </section>
    )
  }

  const visibleEvents = warningOnly ? diagnostics.warningEvents : diagnostics.events

  return (
    <section className="grid gap-3 rounded-container border border-border bg-surface-raised p-4 shadow-surface-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="text-sm font-semibold text-foreground">{t('deploymentsPage.runtimeEventsSection')}</h3>
        <div className="flex items-center gap-1.5">
          <Button
            aria-pressed={warningOnly}
            className="h-7 px-2.5 text-xs"
            size="sm"
            type="button"
            variant={warningOnly ? 'secondary' : 'outline'}
            onClick={onToggleWarningOnly}
          >
            <AlertTriangle className="size-3.5" aria-hidden="true" />
            {t('deploymentsPage.runtimeEventsWarningOnly')}
          </Button>
          <Button
            aria-label={t('common.refresh')}
            className="size-7"
            disabled={diagnostics.eventsFetching}
            size="icon"
            type="button"
            variant="outline"
            onClick={diagnostics.refetchEvents}
          >
            <RefreshCw className={cn('size-3.5', diagnostics.eventsFetching && 'motion-safe:animate-spin')} />
          </Button>
        </div>
      </div>

      {diagnostics.eventsLoading
        ? (
            <div className="grid gap-2">
              <Skeleton className="h-14 w-full" />
              <Skeleton className="h-14 w-full" />
            </div>
          )
        : diagnostics.eventsUnavailable
          ? (
              <ErrorState
                description={t('deploymentsPage.runtimeEventsUnavailableDescription')}
                retrying={diagnostics.eventsFetching}
                title={t('common.unavailable')}
                onRetry={diagnostics.refetchEvents}
              />
            )
          : visibleEvents.length === 0
            ? (
                <EmptyState
                  description={warningOnly ? t('deploymentsPage.runtimeEventsNoWarningsDescription') : t('deploymentsPage.runtimeEventsEmptyDescription')}
                  icon={<Activity className="size-5" aria-hidden="true" />}
                  title={warningOnly ? t('deploymentsPage.runtimeEventsNoWarnings') : t('deploymentsPage.runtimeEventsEmpty')}
                  variant="plain"
                />
              )
            : (
                <div className="divide-y divide-separator-subtle">
                  {visibleEvents.map(event => (
                    <RuntimeEventItem key={`${event.involvedKind}-${event.involvedName}-${event.reason}-${event.lastSeen}`} event={event} />
                  ))}
                </div>
              )}
    </section>
  )
}

function RuntimePodCard({
  locale,
  pod,
  podMetrics,
  release,
  replicaCpuLimitText,
  replicaMemoryLimitText,
  webConsoleEnabled,
  onOpenPodConsole,
  onViewPodLogs,
}: {
  locale: string
  pod: DeploymentTargetRuntimePod
  podMetrics?: DeploymentTargetPodMetrics
  release?: Release
  replicaCpuLimitText?: string
  replicaMemoryLimitText?: string
  webConsoleEnabled: boolean
  onOpenPodConsole?: (release: Release, pod: string) => void
  onViewPodLogs?: (release: Release, pod: string, previous: boolean) => void
}) {
  const { t } = useTranslation()
  const effectiveStatus = pod.status || (pod.terminating ? 'terminating' : pod.ready ? 'ready' : 'not-ready')
  const readyContainers = pod.readyContainers ?? (pod.ready ? pod.containers.length : 0)
  const totalContainers = pod.totalContainers ?? Math.max(pod.containers.length, 1)
  const allStatuses = pod.containerStatuses ?? []
  const abnormalContainers = allStatuses.filter(item =>
    item.currentState.state === 'waiting'
    || (item.currentState.state === 'terminated' && (item.currentState.exitCode ?? 0) !== 0)
    || Boolean(item.lastTermination),
  )
  const failedConditions = (pod.conditions ?? []).filter(condition => condition.reason === 'Unschedulable' || condition.message)
  const hasPreviousCrash = pod.restartCount > 0 || allStatuses.some(item => Boolean(item.lastTermination))
  const canOpenConsole = Boolean(release && webConsoleEnabled && (release.status === 'succeeded' || release.status === 'running'))

  const podCpuUsedText = podMetrics ? formatLiveUsageCPU(podMetrics.cpuUsageMilli, locale) : undefined
  const podMemoryUsedText = podMetrics ? formatLiveUsageBytes(podMetrics.memoryUsageBytes, locale) : undefined

  return (
    <div className="grid gap-2.5 rounded-xl border border-border bg-surface-subtle/40 p-3.5 text-xs shadow-surface-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-mono font-medium text-foreground">{pod.name}</span>
          <StatusValueBadge labelKeyPrefix="deploymentsPage.runtimeStatuses" value={effectiveStatus} />
        </div>
        {release && (
          <div className="flex flex-wrap items-center gap-1.5">
            {onViewPodLogs && (
              <Button
                className="h-7 px-2.5 text-xs"
                size="sm"
                type="button"
                variant="outline"
                onClick={() => onViewPodLogs(release, pod.name, false)}
              >
                <Eye className="size-3.5" aria-hidden="true" />
                {t('deploymentsPage.runtimePodLogsAction')}
              </Button>
            )}
            {onViewPodLogs && hasPreviousCrash && (
              <Button
                className="h-7 px-2.5 text-xs"
                size="sm"
                type="button"
                variant="outline"
                onClick={() => onViewPodLogs(release, pod.name, true)}
              >
                <History className="size-3.5" aria-hidden="true" />
                {t('deploymentsPage.runtimePreviousLogs')}
              </Button>
            )}
            {onOpenPodConsole && (
              <Button
                className="h-7 px-2.5 text-xs"
                disabled={!canOpenConsole}
                size="sm"
                title={!webConsoleEnabled ? t('deploymentsPage.webConsoleDisabledHint') : undefined}
                type="button"
                variant="outline"
                onClick={() => onOpenPodConsole(release, pod.name)}
              >
                <Terminal className="size-3.5" aria-hidden="true" />
                {t('deploymentsPage.webConsole')}
              </Button>
            )}
          </div>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
        <PodResourceMiniRing
          label={t('deploymentsPage.metricsCpu')}
          limitText={replicaCpuLimitText}
          locale={locale}
          percent={podMetrics?.cpuUsagePercent}
          usedText={podCpuUsedText}
        />
        <PodResourceMiniRing
          label={t('deploymentsPage.metricsMemory')}
          limitText={replicaMemoryLimitText}
          locale={locale}
          percent={podMetrics?.memoryUsagePercent}
          usedText={podMemoryUsedText}
        />
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 tabular-nums text-muted-foreground">
        <span>{t('deploymentsPage.runtimePodReadySummary', { ready: readyContainers, total: totalContainers })}</span>
        <span className={pod.restartCount > 0 ? 'font-semibold text-warning' : undefined}>
          {t('deploymentsPage.runtimePodRestarts', { count: pod.restartCount })}
        </span>
        {pod.nodeName && <span>{t('deploymentsPage.runtimePodNode', { node: pod.nodeName })}</span>}
        <span title={formatAbsoluteDateTime(pod.createdAt)}>{formatSmartDateTime(pod.createdAt, t)}</span>
      </div>

      {abnormalContainers.map(status => (
        <ContainerDiagnosticNote key={`${status.init ? 'init' : 'app'}-${status.name}`} status={status} />
      ))}

      {abnormalContainers.length === 0 && failedConditions.slice(0, 1).map(condition => (
        <div key={condition.type} className="rounded-control border border-warning-border bg-warning-subtle px-2.5 py-1.5 text-foreground">
          <span className="font-medium">{condition.reason || condition.type}</span>
          {condition.message && <span className="text-muted-foreground">{` · ${condition.message}`}</span>}
        </div>
      ))}
    </div>
  )
}

function ContainerDiagnosticNote({ status }: { status: RuntimeContainerStatusSnapshot }) {
  const { t } = useTranslation()
  const prefix = status.init ? `${status.name} (init)` : status.name
  const current = status.currentState
  const last = status.lastTermination

  return (
    <div className="grid gap-1 rounded-control border border-danger-border bg-danger-subtle px-2.5 py-1.5 text-foreground">
      {(current.state === 'waiting' || (current.state === 'terminated' && (current.exitCode ?? 0) !== 0)) && (
        <div className="break-words">
          <span className="font-mono font-medium">{prefix}</span>
          <span>{` · ${current.reason || current.state}`}</span>
          {current.exitCode !== undefined && <span>{` (${t('deploymentsPage.runtimeExitCode', { code: current.exitCode })})`}</span>}
          {current.message && <span className="text-muted-foreground">{` — ${current.message}`}</span>}
        </div>
      )}
      {last && (
        <div className="break-words text-muted-foreground">
          <span className="font-medium text-foreground">{t('deploymentsPage.runtimeLastTermination')}</span>
          <span>{`: ${last.reason || 'Terminated'}`}</span>
          {last.exitCode !== undefined && <span>{` (${t('deploymentsPage.runtimeExitCode', { code: last.exitCode })})`}</span>}
          {last.finishedAt && <span>{` · ${formatAbsoluteDateTime(last.finishedAt)}`}</span>}
          {last.message && <span>{` — ${last.message}`}</span>}
        </div>
      )}
    </div>
  )
}

function RuntimeEventItem({ event }: { event: DeploymentTargetEvent }) {
  const { t } = useTranslation()
  const isWarning = event.type === 'Warning'

  return (
    <div className="grid gap-1 py-2.5 text-xs first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <StatusBadge tone={isWarning ? 'warning' : 'neutral'}>
            {isWarning ? t('deploymentsPage.runtimeEventTypeWarning') : t('deploymentsPage.runtimeEventTypeNormal')}
          </StatusBadge>
          <span className="font-semibold text-foreground">{event.reason || '-'}</span>
          <span className="truncate font-mono text-muted-foreground">{`${event.involvedKind}/${event.involvedName}`}</span>
          {event.count > 1 && <span className="tabular-nums text-muted-foreground">{`×${event.count}`}</span>}
        </div>
        <span className="shrink-0 tabular-nums text-muted-foreground" title={formatAbsoluteDateTime(event.lastSeen)}>
          {formatSmartDateTime(event.lastSeen, t)}
        </span>
      </div>
      <p className="break-words leading-5 text-muted-foreground">{event.message}</p>
    </div>
  )
}
