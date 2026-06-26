import type { Release } from '@/api'
import { Maximize2, Minimize2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
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
  const [fullscreen, setFullscreen] = useState(false)

  return (
    <Dialog
      open={Boolean(release)}
      onOpenChange={(open) => {
        if (!open) {
          setContainerState({ releaseId: '', value: '' })
          setFullscreen(false)
        }
        onOpenChange(open)
      }}
    >
      <DialogContent className={fullscreen ? 'h-screen max-h-screen w-screen max-w-none rounded-none border-0 p-0' : 'max-w-5xl p-0'}>
        <DialogHeader className={fullscreen ? 'sr-only' : undefined}>
          <div className="border-b border-border px-5 py-4">
            <DialogTitle>{t('deploymentsPage.webConsole')}</DialogTitle>
            <DialogDescription>{t('deploymentsPage.webConsoleDescription')}</DialogDescription>
          </div>
        </DialogHeader>
        <div className={fullscreen ? 'flex h-screen min-h-0 bg-zinc-950 p-0' : 'grid gap-4 px-5 pb-5'}>
          <div className={fullscreen ? 'relative flex min-h-0 flex-1 flex-col overflow-hidden bg-zinc-950 text-zinc-100' : 'overflow-hidden rounded-md border border-zinc-800 bg-zinc-950 text-zinc-100 shadow-xl'}>
            {fullscreen && (
              <Button
                className="absolute right-4 top-4 z-20 border-zinc-700 bg-zinc-900/90 text-zinc-100 shadow-lg hover:bg-zinc-800 hover:text-zinc-100"
                size="sm"
                type="button"
                variant="outline"
                onClick={() => setFullscreen(false)}
              >
                <Minimize2 className="size-4" />
                {t('deploymentsPage.exitFullscreen')}
              </Button>
            )}
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 bg-zinc-900 px-4 py-2">
              <div className="flex items-center gap-2">
                <span className="size-3 rounded-full bg-red-500" />
                <span className="size-3 rounded-full bg-yellow-400" />
                <span className="size-3 rounded-full bg-emerald-500" />
                <span className="ml-2 font-mono text-xs text-zinc-400">{release?.id ?? '-'}</span>
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
                {!fullscreen && (
                  <Button
                    className="h-7 border-zinc-700 bg-zinc-950 px-2 text-xs text-zinc-100 hover:bg-zinc-800 hover:text-zinc-100"
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => setFullscreen(true)}
                  >
                    <Maximize2 className="size-3.5" />
                    {t('deploymentsPage.fullscreen')}
                  </Button>
                )}
              </div>
            </div>
            <div className={fullscreen ? 'min-h-0 flex-1' : undefined}>
              <ApplicationRuntimeTerminalPanel container={container} fullscreen={fullscreen} projectId={projectId} release={release} />
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
