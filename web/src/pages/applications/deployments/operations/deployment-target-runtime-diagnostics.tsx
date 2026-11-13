import type { DeploymentTargetEvent, DeploymentTargetRuntimePod, Release, RuntimeContainerStatusSnapshot } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { Activity, AlertTriangle, Box, Eye, History, RefreshCw, Terminal } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { Section } from '@/components/common/section'
import { StatusBadge, StatusValueBadge } from '@/components/common/status-badge'
import { formatAbsoluteDateTime, formatSmartDateTime } from '@/components/common/time-format'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

export function DeploymentTargetRuntimeDiagnostics({
  applicationId,
  deployed,
  enabled,
  projectId,
  release,
  targetId,
  webConsoleEnabled,
  onOpenPodConsole,
  onViewPodLogs,
}: {
  applicationId: string
  deployed: boolean
  enabled: boolean
  projectId: string
  release?: Release
  targetId: string
  webConsoleEnabled: boolean
  onOpenPodConsole?: (release: Release, pod: string) => void
  onViewPodLogs?: (release: Release, pod: string, previous: boolean) => void
}) {
  if (!enabled || (!deployed && !release))
    return null
  return (
    <ActiveDeploymentTargetRuntimeDiagnostics
      applicationId={applicationId}
      projectId={projectId}
      release={release}
      targetId={targetId}
      webConsoleEnabled={webConsoleEnabled}
      onOpenPodConsole={onOpenPodConsole}
      onViewPodLogs={onViewPodLogs}
    />
  )
}

function ActiveDeploymentTargetRuntimeDiagnostics({
  applicationId,
  projectId,
  release,
  targetId,
  webConsoleEnabled,
  onOpenPodConsole,
  onViewPodLogs,
}: {
  applicationId: string
  projectId: string
  release?: Release
  targetId: string
  webConsoleEnabled: boolean
  onOpenPodConsole?: (release: Release, pod: string) => void
  onViewPodLogs?: (release: Release, pod: string, previous: boolean) => void
}) {
  const { t } = useTranslation()
  const [warningOnly, setWarningOnly] = useState(false)

  const podsQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-target-detail-pods', projectId, applicationId, targetId],
    queryFn: ({ signal }) => api.listDeploymentTargetRuntimePods(projectId, applicationId, targetId, { sortBy: 'restartCount', sortOrder: 'desc' }, signal),
    enabled: Boolean(projectId && applicationId && targetId),
    refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
  })

  const eventsQuery = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['deployment-target-events', projectId, applicationId, targetId, warningOnly ? 'Warning' : 'all'],
    queryFn: ({ signal }) => api.listDeploymentTargetEvents(projectId, applicationId, targetId, {
      pageSize: 20,
      type: warningOnly ? 'Warning' : undefined,
    }, signal),
    enabled: Boolean(projectId && applicationId && targetId),
    refetchInterval: WORKFLOW_STATUS_REFETCH_INTERVAL_MS,
  })

  const podsUnavailable = podsQuery.isError || podsQuery.data?.status === 'unavailable'
  const pods = podsQuery.data?.status === 'ready' ? podsQuery.data.items : []
  const eventsUnavailable = eventsQuery.isError || eventsQuery.data?.status === 'unavailable'
  const events = eventsQuery.data?.status === 'ready' ? eventsQuery.data.items : []

  return (
    <>
      <Section
        description={t('deploymentsPage.runtimePodsSectionDescription')}
        icon={<Box className="size-4" aria-hidden="true" />}
        title={t('deploymentsPage.runtimePodsSection')}
        tools={(
          <Button
            aria-label={t('common.refresh')}
            className="h-8 w-8"
            disabled={podsQuery.isFetching}
            size="icon"
            type="button"
            variant="ghost"
            onClick={() => void podsQuery.refetch()}
          >
            <RefreshCw className={`size-4 ${podsQuery.isFetching ? 'motion-safe:animate-spin' : ''}`} />
          </Button>
        )}
        variant="bordered"
      >
        {podsQuery.isLoading
          ? (
              <div className="grid gap-2">
                <Skeleton className="h-16 w-full" />
                <Skeleton className="h-16 w-full" />
              </div>
            )
          : podsUnavailable
            ? (
                <ErrorState
                  description={t('deploymentsPage.runtimePodsUnavailable')}
                  retrying={podsQuery.isFetching}
                  title={t('common.unavailable')}
                  onRetry={() => void podsQuery.refetch()}
                />
              )
            : pods.length === 0
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
                    {pods.map(pod => (
                      <RuntimePodCard
                        key={pod.name}
                        pod={pod}
                        release={release}
                        webConsoleEnabled={webConsoleEnabled}
                        onOpenPodConsole={onOpenPodConsole}
                        onViewPodLogs={onViewPodLogs}
                      />
                    ))}
                  </div>
                )}
      </Section>

      <Section
        description={t('deploymentsPage.runtimeEventsSectionDescription')}
        icon={<Activity className="size-4" aria-hidden="true" />}
        title={t('deploymentsPage.runtimeEventsSection')}
        tools={(
          <div className="flex items-center gap-1.5">
            <Button
              aria-pressed={warningOnly}
              className="h-8 px-2.5 text-xs"
              size="sm"
              type="button"
              variant={warningOnly ? 'secondary' : 'outline'}
              onClick={() => setWarningOnly(value => !value)}
            >
              <AlertTriangle className="size-3.5" />
              {t('deploymentsPage.runtimeEventsWarningOnly')}
            </Button>
            <Button
              aria-label={t('common.refresh')}
              className="h-8 w-8"
              disabled={eventsQuery.isFetching}
              size="icon"
              type="button"
              variant="ghost"
              onClick={() => void eventsQuery.refetch()}
            >
              <RefreshCw className={`size-4 ${eventsQuery.isFetching ? 'motion-safe:animate-spin' : ''}`} />
            </Button>
          </div>
        )}
        variant="bordered"
      >
        {eventsQuery.isLoading
          ? (
              <div className="grid gap-2">
                <Skeleton className="h-14 w-full" />
                <Skeleton className="h-14 w-full" />
              </div>
            )
          : eventsUnavailable
            ? (
                <ErrorState
                  description={t('deploymentsPage.runtimeEventsUnavailableDescription')}
                  retrying={eventsQuery.isFetching}
                  title={t('common.unavailable')}
                  onRetry={() => void eventsQuery.refetch()}
                />
              )
            : events.length === 0
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
                    {events.map(event => (
                      <RuntimeEventItem key={`${event.involvedKind}-${event.involvedName}-${event.reason}-${event.lastSeen}`} event={event} />
                    ))}
                  </div>
                )}
      </Section>
    </>
  )
}

function RuntimePodCard({
  pod,
  release,
  webConsoleEnabled,
  onOpenPodConsole,
  onViewPodLogs,
}: {
  pod: DeploymentTargetRuntimePod
  release?: Release
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

  return (
    <div className="grid gap-2 rounded-control border border-border bg-surface-subtle/50 p-3 text-xs">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-mono font-medium text-foreground">{pod.name}</span>
          <StatusValueBadge labelKeyPrefix="deploymentsPage.runtimeStatuses" value={effectiveStatus} />
        </div>
        {release && (
          <div className="flex flex-wrap items-center gap-1">
            {onViewPodLogs && (
              <Button
                className="h-7 px-2 text-xs"
                size="sm"
                type="button"
                variant="outline"
                onClick={() => onViewPodLogs(release, pod.name, false)}
              >
                <Eye className="size-3.5" />
                {t('deploymentsPage.runtimePodLogsAction')}
              </Button>
            )}
            {onViewPodLogs && hasPreviousCrash && (
              <Button
                className="h-7 px-2 text-xs"
                size="sm"
                type="button"
                variant="outline"
                onClick={() => onViewPodLogs(release, pod.name, true)}
              >
                <History className="size-3.5" />
                {t('deploymentsPage.runtimePreviousLogs')}
              </Button>
            )}
            {onOpenPodConsole && (
              <Button
                className="h-7 px-2 text-xs"
                disabled={!canOpenConsole}
                size="sm"
                title={!webConsoleEnabled ? t('deploymentsPage.webConsoleDisabledHint') : undefined}
                type="button"
                variant="ghost"
                onClick={() => onOpenPodConsole(release, pod.name)}
              >
                <Terminal className="size-3.5" />
                {t('deploymentsPage.webConsole')}
              </Button>
            )}
          </div>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-muted-foreground">
        <span>{t('deploymentsPage.runtimePodReadySummary', { ready: readyContainers, total: totalContainers })}</span>
        <span className={pod.restartCount > 0 ? 'font-medium text-warning' : undefined}>
          {t('deploymentsPage.runtimePodRestarts', { count: pod.restartCount })}
        </span>
        {pod.nodeName && <span>{t('deploymentsPage.runtimePodNode', { node: pod.nodeName })}</span>}
        <span title={formatAbsoluteDateTime(pod.createdAt)}>{formatSmartDateTime(pod.createdAt, t)}</span>
      </div>

      {abnormalContainers.map(status => (
        <ContainerDiagnosticNote key={`${status.init ? 'init' : 'app'}-${status.name}`} status={status} />
      ))}

      {abnormalContainers.length === 0 && failedConditions.slice(0, 1).map(condition => (
        <div key={condition.type} className="rounded-control border border-warning/30 bg-warning/10 px-2.5 py-1.5 text-foreground">
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
    <div className="grid gap-1 rounded-control border border-destructive/25 bg-destructive/5 px-2.5 py-1.5 text-foreground">
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
          <span className="font-medium text-foreground">{event.reason || '-'}</span>
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
