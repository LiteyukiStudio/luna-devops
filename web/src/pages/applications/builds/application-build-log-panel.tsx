import type { BuildJob, BuildRun } from '@/api'
import { RotateCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AIDeliveryDiagnosisCard } from '@/components/common/ai-delivery-diagnosis-card'
import { LogViewer } from '@/components/common/log-viewer'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { StatusIndicator } from '@/components/common/status-indicator'
import { statusIndicatorStateFor } from '@/components/common/status-tone'
import { Button } from '@/components/ui/button'
import { shortBuildId } from '@/pages/applications/application-config-utils'

export function ApplicationBuildLogPanel({
  applicationId,
  content,
  job,
  jobsError = false,
  jobsLoading = false,
  loading,
  onActionExecuted,
  onClose,
  onRetry,
  projectId,
  retrying = false,
  run,
  streamError = false,
}: {
  applicationId: string
  content: string
  job: BuildJob | null
  jobsError?: boolean
  jobsLoading?: boolean
  loading: boolean
  onActionExecuted?: () => void
  onClose: () => void
  onRetry?: () => void
  projectId: string
  retrying?: boolean
  run?: BuildRun
  streamError?: boolean
}) {
  const { t } = useTranslation()
  if (!job && !run)
    return null
  const displayId = job?.id ?? run?.id ?? ''
  const displayStatus = job?.status ?? run?.status ?? 'failed'
  const waitingForJob = !job && !jobsError && (jobsLoading || run?.status === 'queued' || run?.status === 'running')
  const queuedWithoutLogs = job?.status === 'queued' && !content && !streamError
  const failed = (!job && jobsError) || streamError
  const isWaiting = !failed && (loading || waitingForJob || queuedWithoutLogs)
  const showErrorPlaceholder = !content && failed
  const viewerState = showErrorPlaceholder ? 'error' : isWaiting ? 'loading' : 'ready'
  return (
    <OutputViewerSheet
      description={<span className="font-mono text-xs">{displayId}</span>}
      title={t('buildsPage.logsTitle', { id: shortBuildId(displayId) })}
      titleAccessory={<StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={displayStatus} />}
      onClose={onClose}
    >
      {run?.aiDiagnosis && (
        <div className="shrink-0 border-b border-border p-4">
          <AIDeliveryDiagnosisCard
            applicationId={applicationId}
            deploymentTargetId={run.deploymentTargetId}
            diagnosis={run.aiDiagnosis}
            projectId={projectId}
            onActionExecuted={onActionExecuted}
            onNavigateEditTarget={onClose}
          />
        </div>
      )}
      <LogViewer
        className="flex-1"
        content={content}
        downloadName={`build-${displayId}.log`}
        emptyLabel={t('buildsPage.noLogs')}
        errorLabel={t('common.unavailable')}
        live={isWaiting}
        loadingLabel={t('common.logViewer.waiting')}
        meta={(
          <>
            <StatusIndicator
              className="text-xs"
              label={isWaiting ? t('buildsPage.logsStreaming') : failed ? t('common.unavailable') : t('buildsPage.logsUpdated')}
              state={isWaiting ? 'running' : failed ? statusIndicatorStateFor('failed') : 'neutral'}
            />
            {Boolean(content) && failed && onRetry && (
              <Button
                className="h-7 px-2.5 text-xs"
                disabled={retrying}
                size="sm"
                type="button"
                variant="outline"
                onClick={onRetry}
              >
                <RotateCw className="size-3.5" />
                {t('common.retry')}
              </Button>
            )}
          </>
        )}
        resetKey={displayId}
        state={viewerState}
        onRetry={onRetry}
      />
    </OutputViewerSheet>
  )
}
