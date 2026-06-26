import type { BuildJob } from '@/api'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AutoFollowLog } from '@/components/common/auto-follow-log'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { shortBuildId } from './application-config-utils'

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
    <div className="fixed inset-0 z-50 bg-black/20" onClick={onClose}>
      <aside
        className="absolute right-0 top-0 flex h-full w-full max-w-3xl flex-col border-l border-border bg-background shadow-xl"
        onClick={event => event.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <h2 className="truncate text-base font-semibold">{t('buildsPage.logsTitle', { id: shortBuildId(job.id) })}</h2>
              <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={job.status} />
            </div>
            <p className="text-sm text-muted-foreground">{loading ? t('buildsPage.logsStreaming') : t('buildsPage.logsUpdated')}</p>
          </div>
          <Button aria-label={t('common.close')} size="icon" variant="ghost" onClick={onClose}>
            <X className="size-4" />
          </Button>
        </div>
        <AutoFollowLog
          className="min-h-0 flex-1 bg-zinc-950 p-4 font-mono text-sm leading-6 text-zinc-100"
          content={content}
          emptyFallback={t('buildsPage.noLogs')}
          resetKey={job.id}
        />
      </aside>
    </div>
  )
}
