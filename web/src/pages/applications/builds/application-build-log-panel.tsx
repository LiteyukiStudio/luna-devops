import type { BuildJob } from '@/api'
import { useTranslation } from 'react-i18next'
import { AutoFollowLog } from '@/components/common/auto-follow-log'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
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
    <Sheet open onOpenChange={open => !open && onClose()}>
      <SheetContent className="w-[min(100vw,48rem)] max-w-none gap-0 p-0 sm:max-w-none" side="right">
        <SheetHeader className="gap-1 border-b border-border px-4 py-3 pr-14">
          <div className="flex min-w-0 items-center gap-2">
            <SheetTitle className="truncate text-base">{t('buildsPage.logsTitle', { id: shortBuildId(job.id) })}</SheetTitle>
            <StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={job.status} />
          </div>
          <SheetDescription>{loading ? t('buildsPage.logsStreaming') : t('buildsPage.logsUpdated')}</SheetDescription>
        </SheetHeader>
        <AutoFollowLog
          className="min-h-0 flex-1 bg-zinc-950 p-4 font-mono text-sm leading-6 text-zinc-100"
          content={content}
          emptyFallback={t('buildsPage.noLogs')}
          resetKey={job.id}
        />
      </SheetContent>
    </Sheet>
  )
}
