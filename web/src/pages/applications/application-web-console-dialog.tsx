import type { ButtonHTMLAttributes, ReactNode } from 'react'
import type { Release } from '@/api'
import { Maximize2, Minimize2, Minus, X } from 'lucide-react'
import { lazy, Suspense, useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

const ApplicationRuntimeTerminalPanel = lazy(() =>
  import('./application-runtime-terminal-panel').then(module => ({ default: module.ApplicationRuntimeTerminalPanel })),
)

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
  const [fullscreen, setFullscreen] = useState(false)
  const container = containerState.releaseId === releaseId ? containerState.value : ''
  const authorizeTerminal = useCallback(async () => {
    if (!releaseId)
      return
    await api.authorizeReleaseRuntimeTerminal(projectId, releaseId)
  }, [projectId, releaseId])
  const closeDialog = () => {
    setContainerState({ releaseId: '', value: '' })
    setFullscreen(false)
    onOpenChange(false)
  }

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
      <DialogContent
        className={cn(
          'overflow-visible border-0 bg-transparent p-0 shadow-none',
          fullscreen
            ? 'h-[calc(100dvh-1rem)] max-h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] max-w-none'
            : 'max-h-[calc(100vh-2rem)] max-w-[min(94vw,96rem)]',
        )}
        showCloseButton={false}
      >
        <DialogTitle className="sr-only">{t('deploymentsPage.webConsole')}</DialogTitle>
        <DialogDescription className="sr-only">{t('deploymentsPage.webConsoleDescription')}</DialogDescription>
        <div className={cn('overflow-hidden rounded-md border border-zinc-800 bg-zinc-950 text-zinc-100 shadow-2xl', fullscreen && 'flex h-full min-h-0 flex-col')}>
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 bg-zinc-900 px-5 py-3">
            <div className="flex items-center gap-2">
              <WindowControlButton
                icon={<X className="size-2.5" strokeWidth={3} />}
                label={t('common.close')}
                tone="close"
                onClick={closeDialog}
              />
              <WindowControlButton
                aria-hidden="true"
                disabled
                icon={<Minus className="size-2.5" strokeWidth={3} />}
                tone="minimize"
              />
              <WindowControlButton
                icon={fullscreen ? <Minimize2 className="size-2.5" strokeWidth={3} /> : <Maximize2 className="size-2.5" strokeWidth={3} />}
                label={fullscreen ? t('deploymentsPage.exitFullscreen') : t('deploymentsPage.fullscreen')}
                tone="fullscreen"
                onClick={() => setFullscreen(value => !value)}
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
          <div className={fullscreen ? 'min-h-0 flex-1' : undefined}>
            <Suspense fallback={<div className={fullscreen ? 'h-full min-h-[28rem] bg-slate-950' : 'h-[29.5rem] bg-slate-950'} />}>
              <ApplicationRuntimeTerminalPanel authorize={authorizeTerminal} fullscreen={fullscreen} container={container} projectId={projectId} release={release} />
            </Suspense>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

export function WindowControlButton({
  disabled,
  icon,
  label,
  tone,
  onClick,
  ...props
}: {
  disabled?: boolean
  icon: ReactNode
  label?: string
  tone: 'close' | 'minimize' | 'fullscreen'
  onClick?: () => void
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      aria-label={label}
      className={cn(
        'group grid size-4 place-items-center rounded-full border outline-none transition focus-visible:ring-2 disabled:cursor-default',
        tone === 'close' && 'border-red-400/60 bg-red-500 shadow-sm shadow-red-950/50 hover:bg-red-400 focus-visible:ring-red-300/50',
        tone === 'minimize' && 'border-yellow-500/60 bg-yellow-400 shadow-sm shadow-yellow-950/40',
        tone === 'fullscreen' && 'border-emerald-600/60 bg-emerald-500 shadow-sm shadow-emerald-950/40 hover:bg-emerald-400 focus-visible:ring-emerald-300/50',
      )}
      disabled={disabled}
      tabIndex={disabled ? -1 : undefined}
      type="button"
      onClick={onClick}
      {...props}
    >
      <span className="grid place-items-center text-black/70 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
        {icon}
      </span>
    </button>
  )
}
