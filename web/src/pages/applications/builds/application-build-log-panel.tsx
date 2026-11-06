import type { BuildJob } from '@/api'
import { useTranslation } from 'react-i18next'
import { LogViewer } from '@/components/common/log-viewer'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { StatusIndicator } from '@/components/common/status-indicator'
import { shortBuildId } from '@/pages/applications/application-config-utils'

export function ApplicationBuildLogPanel({ content, job, loading, onClose }: {
  content: string
  job: BuildJob | null
  loading: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  if (!job)
    return null
  return (
    <OutputViewerSheet
      description={<span className="font-mono text-xs">{job.id}</span>}
      title={t('buildsPage.logsTitle', { id: shortBuildId(job.id) })}
      titleAccessory={<StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={job.status} />}
      onClose={onClose}
    >
      <LogViewer
        className="flex-1"
        content={content}
        downloadName={`build-${job.id}.log`}
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
        resetKey={job.id}
        state={loading ? 'loading' : 'ready'}
      />
    </OutputViewerSheet>
  )
}
