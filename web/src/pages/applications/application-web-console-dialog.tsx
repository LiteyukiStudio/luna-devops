import type { Release } from '@/api'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { ApplicationRuntimeTerminalPanel } from './application-runtime-terminal-panel'

export function ApplicationWebConsoleDialog({
  projectId,
  release,
  onOpenChange,
}: {
  projectId: string
  release: Release | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const releaseId = release?.id ?? ''
  const [containerState, setContainerState] = useState({ releaseId: '', value: '' })
  const container = containerState.releaseId === releaseId ? containerState.value : ''

  return (
    <Dialog
      open={Boolean(release)}
      onOpenChange={(open) => {
        if (!open)
          setContainerState({ releaseId: '', value: '' })
        onOpenChange(open)
      }}
    >
      <DialogContent
        className="max-h-[calc(100vh-2rem)] max-w-[min(94vw,96rem)] overflow-visible border-0 bg-transparent p-0 shadow-none"
        showCloseButton={false}
      >
        <DialogTitle className="sr-only">{t('deploymentsPage.webConsole')}</DialogTitle>
        <DialogDescription className="sr-only">{t('deploymentsPage.webConsoleDescription')}</DialogDescription>
        <div className="overflow-hidden rounded-md border border-zinc-800 bg-zinc-950 text-zinc-100 shadow-2xl">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 bg-zinc-900 px-5 py-3">
            <div className="flex items-center gap-2">
              <DialogClose
                aria-label={t('common.close')}
                className="size-4 rounded-full bg-red-500 shadow-sm shadow-red-950/50 outline-none transition hover:bg-red-400 focus:ring-2 focus:ring-red-300/50"
                type="button"
              />
              <button
                aria-hidden="true"
                className="size-4 cursor-default rounded-full bg-yellow-400 opacity-80 shadow-sm shadow-yellow-950/40"
                disabled
                tabIndex={-1}
                type="button"
              />
              <button
                aria-hidden="true"
                className="size-4 cursor-default rounded-full bg-emerald-500 opacity-80 shadow-sm shadow-emerald-950/40"
                disabled
                tabIndex={-1}
                type="button"
              />
              <span className="ml-3 font-mono text-xs text-zinc-400">{release?.id ?? '-'}</span>
            </div>
            <div className="flex min-w-0 flex-wrap items-center justify-end gap-2 pr-0 sm:pr-0">
              <label className="flex min-w-0 items-center gap-2 font-mono text-xs text-zinc-400">
                <span>{t('deploymentsPage.container')}</span>
                <input
                  className="h-7 w-32 rounded border border-zinc-700 bg-zinc-950 px-2 text-zinc-100 outline-none transition placeholder:text-zinc-600 focus:border-emerald-500"
                  placeholder={t('deploymentsPage.webConsoleContainerPlaceholder')}
                  value={container}
                  onChange={event => setContainerState({ releaseId, value: event.target.value })}
                />
              </label>
            </div>
          </div>
          <ApplicationRuntimeTerminalPanel container={container} projectId={projectId} release={release} />
        </div>
      </DialogContent>
    </Dialog>
  )
}
