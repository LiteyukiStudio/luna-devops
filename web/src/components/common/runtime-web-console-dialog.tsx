import type { ComponentProps, ReactNode } from 'react'
import { Maximize2, Minimize2, Minus, X } from 'lucide-react'
import { useState } from 'react'
import { LazyLoadBoundary } from '@/components/common/lazy-load-boundary'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

interface RuntimeWebConsoleDialogProps {
  children: (context: { container: string, fullscreen: boolean }) => ReactNode
  closeLabel: string
  containerLabel: string
  containerPlaceholder: string
  description: string
  exitFullscreenLabel: string
  fullscreenLabel: string
  loadingLabel: string
  open: boolean
  resourceKey: string
  resourceLabel: string
  title: string
  onOpenChange: (open: boolean) => void
}

/** Shared terminal window chrome for application releases and cluster pods. */
export function RuntimeWebConsoleDialog({
  children,
  closeLabel,
  containerLabel,
  containerPlaceholder,
  description,
  exitFullscreenLabel,
  fullscreenLabel,
  loadingLabel,
  open,
  resourceKey,
  resourceLabel,
  title,
  onOpenChange,
}: RuntimeWebConsoleDialogProps) {
  const [containerState, setContainerState] = useState({ resourceKey: '', value: '' })
  const [fullscreen, setFullscreen] = useState(false)
  const container = containerState.resourceKey === resourceKey ? containerState.value : ''

  const changeOpen = (nextOpen: boolean) => {
    if (!nextOpen) {
      setContainerState({ resourceKey: '', value: '' })
      setFullscreen(false)
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent
        className={cn(
          'overflow-visible border-0 bg-transparent p-0 shadow-none',
          fullscreen
            ? 'h-[calc(100dvh-1rem)] max-h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] max-w-none'
            : 'max-h-[calc(100vh-2rem)] max-w-[min(94vw,96rem)]',
        )}
        showCloseButton={false}
      >
        <DialogTitle className="sr-only">{title}</DialogTitle>
        <DialogDescription className="sr-only">{description}</DialogDescription>
        <div className={cn('overflow-hidden rounded-md border border-zinc-800 bg-zinc-950 text-zinc-100 shadow-2xl', fullscreen && 'flex h-full min-h-0 flex-col')}>
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 bg-zinc-900 px-5 py-3">
            <div className="flex min-w-0 items-center gap-2">
              <WindowControlButton
                icon={<X className="size-2.5" strokeWidth={3} />}
                label={closeLabel}
                tone="close"
                onClick={() => changeOpen(false)}
              />
              <WindowControlButton
                aria-hidden="true"
                disabled
                icon={<Minus className="size-2.5" strokeWidth={3} />}
                tone="minimize"
              />
              <WindowControlButton
                icon={fullscreen ? <Minimize2 className="size-2.5" strokeWidth={3} /> : <Maximize2 className="size-2.5" strokeWidth={3} />}
                label={fullscreen ? exitFullscreenLabel : fullscreenLabel}
                tone="fullscreen"
                onClick={() => setFullscreen(value => !value)}
              />
              <span className="ml-3 min-w-0 truncate font-mono text-xs text-zinc-400">{resourceLabel || '-'}</span>
            </div>
            <label className="flex min-w-0 items-center gap-2 font-mono text-xs text-zinc-400">
              <span>{containerLabel}</span>
              <Input
                className="h-7 w-32 border-zinc-700 bg-zinc-950 px-2 text-zinc-100 shadow-none placeholder:text-zinc-600 focus-visible:border-emerald-500 focus-visible:ring-0"
                placeholder={containerPlaceholder}
                value={container}
                onChange={event => setContainerState({ resourceKey, value: event.target.value })}
              />
            </label>
          </div>
          <div className={fullscreen ? 'min-h-0 flex-1' : undefined}>
            <LazyLoadBoundary
              fallback={<TerminalLoadingState fullscreen={fullscreen} label={loadingLabel} />}
              resetKey={`${resourceKey}:${container}`}
            >
              {children({ container, fullscreen })}
            </LazyLoadBoundary>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function TerminalLoadingState({ fullscreen, label }: { fullscreen: boolean, label: string }) {
  return <div className={cn('grid place-items-center bg-slate-950 text-sm text-zinc-400', fullscreen ? 'h-full min-h-[28rem]' : 'h-[29.5rem]')} role="status">{label}</div>
}

function WindowControlButton({
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
} & Omit<ComponentProps<typeof Button>, 'children' | 'onClick'>) {
  return (
    <Button
      {...props}
      aria-label={label}
      className={cn(
        'group size-4 rounded-full border p-0 outline-none focus-visible:ring-2 disabled:cursor-default',
        tone === 'close' && 'border-red-400/60 bg-red-500 shadow-sm shadow-red-950/50 hover:bg-red-400 focus-visible:ring-red-300/50',
        tone === 'minimize' && 'border-yellow-500/60 bg-yellow-400 shadow-sm shadow-yellow-950/40',
        tone === 'fullscreen' && 'border-emerald-600/60 bg-emerald-500 shadow-sm shadow-emerald-950/40 hover:bg-emerald-400 focus-visible:ring-emerald-300/50',
      )}
      disabled={disabled}
      size="icon"
      tabIndex={disabled ? -1 : undefined}
      type="button"
      variant="ghost"
      onClick={onClick}
    >
      <span className="grid place-items-center text-black/70 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
        {icon}
      </span>
    </Button>
  )
}
