import type { BuildJob, BuildRun } from '@/api'
import { useTranslation } from 'react-i18next'
import { AIDeliveryDiagnosisCard } from '@/components/common/ai-delivery-diagnosis-card'
import { LogViewer } from '@/components/common/log-viewer'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { StatusIndicator } from '@/components/common/status-indicator'
import { shortBuildId } from '@/pages/applications/application-config-utils'

export function ApplicationBuildLogPanel({
  applicationId,
  content,
  job,
  loading,
  onActionExecuted,
  onClose,
  projectId,
  run,
}: {
  applicationId: string
  content: string
  job: BuildJob | null
  loading: boolean
  onActionExecuted?: () => void
  onClose: () => void
  projectId: string
  run?: BuildRun
}) {
  const { t } = useTranslation()
  if (!job && !run)
    return null
  const displayId = job?.id ?? run?.id ?? ''
  const displayStatus = job?.status ?? run?.status ?? 'failed'
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
        live={loading}
        loadingLabel={t('common.logViewer.waiting')}
        meta={(
          <StatusIndicator
            className="text-xs"
            label={loading ? t('buildsPage.logsStreaming') : t('buildsPage.logsUpdated')}
            state={loading ? 'running' : 'neutral'}
          />
        )}
        resetKey={displayId}
        state={loading ? 'loading' : 'ready'}
      />
    </OutputViewerSheet>
  )
}
