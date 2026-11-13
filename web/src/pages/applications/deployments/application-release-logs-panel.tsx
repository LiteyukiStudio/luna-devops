import type { Release } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { Clock, History } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, ApiError } from '@/api'
import { LogViewer } from '@/components/common/log-viewer'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { StatusIndicator } from '@/components/common/status-indicator'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { RuntimePodSelect } from '@/pages/applications/runtime/runtime-pod-select'
import { collectRuntimeContainerOptions, useReleaseRuntimePods } from '@/pages/applications/runtime/use-release-runtime-pods'

const tailLineOptions = [200, 500, 2000] as const

export function ApplicationReleaseLogsPanel({
  initialContainer = '',
  initialPod = '',
  initialPrevious = false,
  initialView = 'deployment',
  projectId,
  release,
  onClose,
}: {
  initialContainer?: string
  initialPod?: string
  initialPrevious?: boolean
  initialView?: 'deployment' | 'runtime'
  projectId: string
  release: Release
  onClose: () => void
}) {
  const { t } = useTranslation()
  const containerSelectId = useId()
  const tailSelectId = useId()
  const [logView, setLogView] = useState<'deployment' | 'runtime'>(initialView)
  const [pod, setPod] = useState(initialPod)
  const [container, setContainer] = useState(initialContainer)
  const [previous, setPrevious] = useState(initialPrevious)
  const [timestamps, setTimestamps] = useState(false)
  const [tailLines, setTailLines] = useState<number>(500)
  const runtimePods = useReleaseRuntimePods(projectId, release)
  const podItems = useMemo(
    () => (runtimePods.data?.status === 'ready' ? runtimePods.data.items : []),
    [runtimePods.data],
  )
  const containerOptions = useMemo(
    () => collectRuntimeContainerOptions(podItems, pod, true),
    [pod, podItems],
  )
  const activePodSnapshot = useMemo(
    () => (pod ? podItems.find(item => item.name === pod) : podItems.find(item => item.restartCount > 0)),
    [pod, podItems],
  )
  const hasCrashHistory = Boolean(
    activePodSnapshot
    && (activePodSnapshot.restartCount > 0 || activePodSnapshot.containerStatuses?.some(status => Boolean(status.lastTermination))),
  )
  const releaseLogs = useQuery({
    queryKey: ['release-logs', projectId, release.id],
    queryFn: ({ signal }) => api.getReleaseLogs(projectId, release.id, signal),
    enabled: Boolean(projectId && logView === 'deployment'),
    refetchInterval: release.status === 'running' || release.status === 'pending' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const runtimeLogs = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['release-runtime-logs', projectId, release.id, pod, container, previous, timestamps, tailLines],
    queryFn: ({ signal }) => api.getReleaseRuntimeLogs(projectId, release.id, {
      ...(pod ? { pod } : {}),
      ...(container ? { container } : {}),
      tailLines,
      ...(previous ? { previous: true } : {}),
      ...(timestamps ? { timestamps: true } : {}),
    }, signal),
    enabled: Boolean(projectId && logView === 'runtime'),
    refetchInterval: logView === 'runtime' && !previous ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
    // 副本缺失、容器未启动或无上次崩溃日志是确定结论，立即展示原因，不做退避重试。
    retry: (failureCount, error) => !(error instanceof ApiError && error.status < 500) && failureCount < 3,
  })
  const deploymentLive = release.status === 'running' || release.status === 'pending'
  const runtimeErrorCode = runtimeLogs.error instanceof ApiError ? runtimeLogs.error.code : ''
  const runtimeErrorLabel = runtimeErrorCode === 'runtime.pod_not_found'
    ? t('deploymentsPage.runtimePodMissing')
    : runtimeErrorCode === 'runtime.container_not_started'
      ? t('deploymentsPage.runtimeContainerNotStarted')
      : runtimeErrorCode === 'runtime.previous_log_not_found'
        ? t('deploymentsPage.runtimePreviousLogNotFound')
        : t('common.unavailable')
  const liveDot = <StatusIndicator className="text-xs" label={t('common.logViewer.live')} state="running" />

  return (
    <OutputViewerSheet
      description={<span className="font-mono text-xs">{release.id}</span>}
      title={t('deploymentsPage.releaseLogs')}
      titleAccessory={<StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />}
      onClose={onClose}
    >
      <Tabs className="min-h-0 flex-1 gap-0" value={logView} onValueChange={value => setLogView(value as 'deployment' | 'runtime')}>
        <div className="shrink-0 border-b border-border px-4">
          <TabsList className="border-b-0">
            {(['deployment', 'runtime'] as const).map(view => (
              <TabsTrigger key={view} value={view}>{t(`deploymentsPage.logViews.${view}`)}</TabsTrigger>
            ))}
          </TabsList>
        </div>
        <TabsContent className="flex min-h-0 flex-1 flex-col overflow-hidden" value="deployment">
          <LogViewer
            className="flex-1"
            content={releaseLogs.data?.content}
            downloadName={`release-${release.id}.log`}
            emptyLabel={t('deploymentsPage.emptyLogs')}
            live={deploymentLive}
            meta={deploymentLive ? liveDot : undefined}
            resetKey={`${release.id}:deployment`}
            state={releaseLogs.isError ? 'error' : releaseLogs.isLoading ? 'loading' : 'ready'}
            onRetry={() => void releaseLogs.refetch()}
          />
        </TabsContent>
        <TabsContent className="flex min-h-0 flex-1 flex-col overflow-hidden" value="runtime">
          {hasCrashHistory && !previous && activePodSnapshot && (
            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border bg-warning/10 px-4 py-2 text-xs">
              <span className="text-foreground">
                {t('deploymentsPage.runtimeCrashHistoryHint', { pod: activePodSnapshot.name, count: activePodSnapshot.restartCount })}
              </span>
              <Button
                className="h-7 px-2.5 text-xs"
                size="sm"
                type="button"
                variant="outline"
                onClick={() => {
                  if (!pod)
                    setPod(activePodSnapshot.name)
                  setPrevious(true)
                }}
              >
                <History className="size-3.5" />
                {t('deploymentsPage.runtimeSwitchToPreviousLogs')}
              </Button>
            </div>
          )}
          <LogViewer
            className="flex-1"
            content={runtimeLogs.data?.content}
            downloadName={`release-${release.id}-${runtimeLogs.data?.pod || 'runtime'}${previous ? '-previous' : ''}.log`}
            emptyLabel={t('deploymentsPage.emptyLogs')}
            errorLabel={runtimeErrorLabel}
            live={!previous}
            meta={(
              <>
                <RuntimePodSelect projectId={projectId} release={release} value={pod} onValueChange={setPod} />
                {containerOptions.length > 0 && (
                  <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                    <label htmlFor={containerSelectId}>{t('deploymentsPage.container')}</label>
                    <NativeSelect
                      className="h-8 w-36 font-mono text-xs"
                      containerClassName="w-auto"
                      id={containerSelectId}
                      value={container}
                      onChange={event => setContainer(event.target.value)}
                    >
                      <option value="">{t('deploymentsPage.webConsoleContainerPlaceholder')}</option>
                      {containerOptions.map(option => (
                        <option key={option.name} value={option.name}>
                          {option.init ? `${option.name} (init)` : option.name}
                        </option>
                      ))}
                    </NativeSelect>
                  </div>
                )}
                <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                  <label htmlFor={tailSelectId}>{t('deploymentsPage.runtimeTailLines')}</label>
                  <NativeSelect
                    className="h-8 w-24 text-xs"
                    containerClassName="w-auto"
                    id={tailSelectId}
                    value={String(tailLines)}
                    onChange={event => setTailLines(Number(event.target.value) || 500)}
                  >
                    {tailLineOptions.map(lines => (
                      <option key={lines} value={String(lines)}>{t('deploymentsPage.runtimeTailLinesOption', { count: lines })}</option>
                    ))}
                  </NativeSelect>
                </div>
                <Button
                  aria-pressed={previous}
                  className="h-8 px-2.5 text-xs"
                  size="sm"
                  type="button"
                  variant={previous ? 'secondary' : 'outline'}
                  onClick={() => setPrevious(value => !value)}
                >
                  <History className="size-3.5" />
                  {t('deploymentsPage.runtimePreviousLogs')}
                </Button>
                <Button
                  aria-pressed={timestamps}
                  className="h-8 px-2.5 text-xs"
                  size="sm"
                  type="button"
                  variant={timestamps ? 'secondary' : 'outline'}
                  onClick={() => setTimestamps(value => !value)}
                >
                  <Clock className="size-3.5" />
                  {t('deploymentsPage.runtimeLogTimestamps')}
                </Button>
                {runtimeLogs.data && !runtimeLogs.isError && (
                  <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                    {t('deploymentsPage.runtimeLogSource', { pod: runtimeLogs.data.pod, container: runtimeLogs.data.container })}
                  </span>
                )}
                {!previous && liveDot}
              </>
            )}
            resetKey={`${release.id}:runtime:${pod}:${container}:${previous}:${timestamps}:${tailLines}`}
            state={runtimeLogs.isError ? 'error' : runtimeLogs.isLoading ? 'loading' : 'ready'}
            onRetry={() => void runtimeLogs.refetch()}
          />
        </TabsContent>
      </Tabs>
    </OutputViewerSheet>
  )
}
