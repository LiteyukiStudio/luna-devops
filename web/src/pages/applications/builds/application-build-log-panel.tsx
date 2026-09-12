import type { BuildJob } from '@/api'
import { useTranslation } from 'react-i18next'
import { AutoFollowLog } from '@/components/common/auto-follow-log'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
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
      description={loading ? t('buildsPage.logsStreaming') : t('buildsPage.logsUpdated')}
      title={t('buildsPage.logsTitle', { id: shortBuildId(job.id) })}
      titleAccessory={<StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={job.status} />}
      onClose={onClose}
    >
      <AutoFollowLog
        className="min-h-0 flex-1 bg-zinc-950 p-4 font-mono text-sm leading-6 text-zinc-100"
        content={content}
        emptyFallback={t('buildsPage.noLogs')}
        resetKey={job.id}
      />
    </OutputViewerSheet>
  )
}
