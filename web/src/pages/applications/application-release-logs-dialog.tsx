import type { Release } from '@/api'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { AutoFollowLog } from '@/components/common/auto-follow-log'
import { SegmentedTabsList } from '@/components/common/segmented-control'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsContent } from '@/components/ui/tabs'
import { WORKFLOW_STATUS_REFETCH_INTERVAL_MS } from '@/lib/polling'

export function ApplicationReleaseLogsDialog({
  logView,
  projectId,
  release,
  setLogView,
  onOpenChange,
}: {
  logView: 'deployment' | 'runtime'
  projectId: string
  release: Release | null
  setLogView: (view: 'deployment' | 'runtime') => void
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const releaseLogs = useQuery({
    queryKey: ['release-logs', projectId, release?.id],
    queryFn: () => api.getReleaseLogs(projectId, release!.id),
    enabled: Boolean(projectId && release),
    refetchInterval: release?.status === 'running' || release?.status === 'pending' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })
  const runtimeLogs = useQuery({
    queryKey: ['release-runtime-logs', projectId, release?.id],
    queryFn: () => api.getReleaseRuntimeLogs(projectId, release!.id, { tailLines: 500 }),
    enabled: Boolean(projectId && release && logView === 'runtime'),
    refetchInterval: release?.status === 'running' || release?.status === 'pending' ? WORKFLOW_STATUS_REFETCH_INTERVAL_MS : false,
  })

  return (
    <Dialog open={Boolean(release)} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t('deploymentsPage.releaseLogs')}</DialogTitle>
          <DialogDescription>{release?.id}</DialogDescription>
        </DialogHeader>
        <Tabs className="gap-3" value={logView} onValueChange={value => setLogView(value as 'deployment' | 'runtime')}>
          <SegmentedTabsList
            items={(['deployment', 'runtime'] as const).map(view => ({
              label: t(`deploymentsPage.logViews.${view}`),
              value: view,
            }))}
            layoutId="release-log-view-active"
            value={logView}
          />
          <TabsContent value="deployment">
            <AutoFollowLog
              className="max-h-[60vh] rounded-md border border-border bg-muted p-3 text-xs leading-relaxed text-foreground"
              content={releaseLogs.data?.content}
              emptyFallback={t('deploymentsPage.emptyLogs')}
              resetKey={`${release?.id ?? ''}:deployment`}
            />
          </TabsContent>
          <TabsContent className="grid gap-3" value="runtime">
            {runtimeLogs.data && (
              <div className="text-xs text-muted-foreground">
                {t('deploymentsPage.runtimeLogSource', { pod: runtimeLogs.data.pod, container: runtimeLogs.data.container })}
              </div>
            )}
            <AutoFollowLog
              className="max-h-[60vh] rounded-md border border-border bg-muted p-3 text-xs leading-relaxed text-foreground"
              content={runtimeLogs.data?.content}
              emptyFallback={runtimeLogs.isLoading ? t('common.loading') : t('deploymentsPage.emptyLogs')}
              resetKey={`${release?.id ?? ''}:runtime`}
            />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
