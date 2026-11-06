import type { Release } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, ApiError } from '@/api'
import { LogViewer } from '@/components/common/log-viewer'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { StatusDot } from '@/components/common/status-dot'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'
import { RuntimePodSelect } from '@/pages/applications/runtime/runtime-pod-select'

export function ApplicationReleaseLogsPanel({
  initialView = 'deployment',
  projectId,
  release,
  onClose,
}: {
  initialView?: 'deployment' | 'runtime'
  projectId: string
  release: Release
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [logView, setLogView] = useState<'deployment' | 'runtime'>(initialView)
  const [pod, setPod] = useState('')
  const releaseLogs = useQuery({
    queryKey: ['release-logs', projectId, release.id],
    queryFn: ({ signal }) => api.getReleaseLogs(projectId, release.id, signal),
    enabled: Boolean(projectId && logView === 'deployment'),
    refetchInterval: release.status === 'running' || release.status === 'pending' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const runtimeLogs = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['release-runtime-logs', projectId, release.id, pod],
    queryFn: ({ signal }) => api.getReleaseRuntimeLogs(projectId, release.id, { pod: pod || undefined, tailLines: 500 }, signal),
    enabled: Boolean(projectId && logView === 'runtime'),
    refetchInterval: logView === 'runtime' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
    // 副本缺失或容器未启动是确定结论，立即展示原因，不做退避重试。
    retry: (failureCount, error) => !(error instanceof ApiError && error.status < 500) && failureCount < 3,
  })
  const deploymentLive = release.status === 'running' || release.status === 'pending'
  const runtimeErrorCode = runtimeLogs.error instanceof ApiError ? runtimeLogs.error.code : ''
  const runtimeErrorLabel = runtimeErrorCode === 'runtime.pod_not_found'
    ? t('deploymentsPage.runtimePodMissing')
    : runtimeErrorCode === 'runtime.container_not_started'
      ? t('deploymentsPage.runtimeContainerNotStarted')
      : t('common.unavailable')
  const liveDot = <StatusDot className="text-xs" label={t('common.logViewer.live')} pulse tone="info" />

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
          <LogViewer
            className="flex-1"
            content={runtimeLogs.data?.content}
            downloadName={`release-${release.id}-${runtimeLogs.data?.pod || 'runtime'}.log`}
            emptyLabel={t('deploymentsPage.emptyLogs')}
            errorLabel={runtimeErrorLabel}
            live
            meta={(
              <>
                <RuntimePodSelect projectId={projectId} release={release} value={pod} onValueChange={setPod} />
                {runtimeLogs.data && !runtimeLogs.isError && (
                  <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                    {t('deploymentsPage.runtimeLogSource', { pod: runtimeLogs.data.pod, container: runtimeLogs.data.container })}
                  </span>
                )}
                {liveDot}
              </>
            )}
            resetKey={`${release.id}:runtime:${pod}`}
            state={runtimeLogs.isError ? 'error' : runtimeLogs.isLoading ? 'loading' : 'ready'}
            onRetry={() => void runtimeLogs.refetch()}
          />
        </TabsContent>
      </Tabs>
    </OutputViewerSheet>
  )
}
