import type { Release } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { AutoFollowLog } from '@/components/common/auto-follow-log'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

export function ApplicationReleaseLogsPanel({
  projectId,
  release,
  onClose,
}: {
  projectId: string
  release: Release
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [logView, setLogView] = useState<'deployment' | 'runtime'>('deployment')
  const releaseLogs = useQuery({
    queryKey: ['release-logs', projectId, release.id],
    queryFn: ({ signal }) => api.getReleaseLogs(projectId, release.id, signal),
    enabled: Boolean(projectId && logView === 'deployment'),
    refetchInterval: release.status === 'running' || release.status === 'pending' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const runtimeLogs = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['release-runtime-logs', projectId, release.id],
    queryFn: ({ signal }) => api.getReleaseRuntimeLogs(projectId, release.id, { tailLines: 500 }, signal),
    enabled: Boolean(projectId && logView === 'runtime'),
    refetchInterval: logView === 'runtime' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const releaseLogFallback = releaseLogs.isError
    ? t('common.unavailable')
    : releaseLogs.isLoading ? t('common.loading') : t('deploymentsPage.emptyLogs')
  const runtimeLogFallback = runtimeLogs.isError
    ? t('common.unavailable')
    : runtimeLogs.isLoading ? t('common.loading') : t('deploymentsPage.emptyLogs')

  return (
    <OutputViewerSheet
      description={release.id}
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
        <TabsContent className="min-h-0 flex-1 overflow-hidden" value="deployment">
          <AutoFollowLog
            className="h-full bg-zinc-950 p-4 font-mono text-sm leading-6 text-zinc-100"
            content={releaseLogs.isError ? undefined : releaseLogs.data?.content}
            emptyFallback={releaseLogFallback}
            resetKey={`${release.id}:deployment`}
          />
        </TabsContent>
        <TabsContent className="min-h-0 flex-1 overflow-hidden" value="runtime">
          <div className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] bg-zinc-950">
            {runtimeLogs.data && !runtimeLogs.isError && (
              <div className="border-b border-zinc-800 px-4 py-2 font-mono text-xs text-zinc-400">
                {t('deploymentsPage.runtimeLogSource', { pod: runtimeLogs.data.pod, container: runtimeLogs.data.container })}
              </div>
            )}
            <AutoFollowLog
              className="min-h-0 bg-zinc-950 p-4 font-mono text-sm leading-6 text-zinc-100"
              content={runtimeLogs.isError ? undefined : runtimeLogs.data?.content}
              emptyFallback={runtimeLogFallback}
              resetKey={`${release.id}:runtime`}
            />
          </div>
        </TabsContent>
      </Tabs>
    </OutputViewerSheet>
  )
}
