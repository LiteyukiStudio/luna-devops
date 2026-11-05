import type { ReactNode } from 'react'
import type { TerminalConnectionState } from './use-terminal-connection'
import { LoaderCircle, Maximize2, Minimize2, RotateCw, SquareTerminal, X } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LazyLoadBoundary } from '@/components/common/lazy-load-boundary'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { TerminalConnectionStatus, TerminalPlaceholder, ViewerToolbar, ViewerToolbarButton } from './terminal-chrome'
import { useTerminalConnection } from './use-terminal-connection'

interface RuntimeWebConsoleRenderContext {
  /** 拼进终端面板 key，重连时递增。 */
  attempt: number
  container: string
  fullscreen: boolean
  onConnectionChange: (state: TerminalConnectionState) => void
}

interface RuntimeWebConsoleDialogProps {
  children: (context: RuntimeWebConsoleRenderContext) => ReactNode
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
  toolbar?: (context: { container: string }) => ReactNode
  title: string
  onOpenChange: (open: boolean) => void
}

/** Terminal window chrome for cluster pod consoles. */
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
  toolbar,
  title,
  onOpenChange,
}: RuntimeWebConsoleDialogProps) {
  const { t } = useTranslation()
  const containerInputId = useId()
  const [containerState, setContainerState] = useState({ resourceKey: '', value: '' })
  const [fullscreen, setFullscreen] = useState(false)
  const connection = useTerminalConnection()
  const container = containerState.resourceKey === resourceKey ? containerState.value : ''

  const changeOpen = (nextOpen: boolean) => {
    if (!nextOpen) {
      setContainerState({ resourceKey: '', value: '' })
      setFullscreen(false)
      connection.reset()
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent
        className={cn(
          'flex flex-col gap-0 overflow-hidden p-0',
          fullscreen
            ? 'h-[calc(100dvh-1rem)] max-h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] max-w-none'
            : 'max-h-[calc(100vh-2rem)] w-[min(94vw,80rem)] max-w-none',
        )}
        showCloseButton={false}
      >
        <ViewerToolbar
          actions={(
            <>
              <ViewerToolbarButton
                disabled={connection.state === 'connecting' || connection.state === 'idle'}
                icon={<RotateCw aria-hidden="true" />}
                label={t('common.terminal.reconnect')}
                onClick={connection.reconnect}
              />
              <ViewerToolbarButton
                icon={fullscreen ? <Minimize2 aria-hidden="true" /> : <Maximize2 aria-hidden="true" />}
                label={fullscreen ? exitFullscreenLabel : fullscreenLabel}
                onClick={() => setFullscreen(value => !value)}
              />
              <ViewerToolbarButton icon={<X aria-hidden="true" />} label={closeLabel} onClick={() => changeOpen(false)} />
            </>
          )}
        >
          <div className="flex min-w-0 items-center gap-2">
            <SquareTerminal aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
            <DialogTitle className="shrink-0 text-sm">{title}</DialogTitle>
            <span className="min-w-0 truncate font-mono text-xs text-muted-foreground" title={resourceLabel}>{resourceLabel || '-'}</span>
          </div>
          <TerminalConnectionStatus state={connection.state} />
          <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <label htmlFor={containerInputId}>{containerLabel}</label>
            <Input
              className="h-8 w-36 font-mono text-xs"
              id={containerInputId}
              placeholder={containerPlaceholder}
              value={container}
              onChange={event => setContainerState({ resourceKey, value: event.target.value })}
            />
          </div>
        </ViewerToolbar>
        <DialogDescription className="sr-only">{description}</DialogDescription>
        {toolbar && (
          <div className="shrink-0 border-b border-border bg-surface-subtle px-4 py-2">
            {toolbar({ container })}
          </div>
        )}
        <div className={cn('bg-terminal', fullscreen && 'min-h-0 flex-1')}>
          <LazyLoadBoundary
            fallback={(
              <TerminalPlaceholder
                className={fullscreen ? 'min-h-[28rem]' : 'h-[29.5rem]'}
                icon={<LoaderCircle className="motion-safe:animate-spin" />}
                label={loadingLabel}
              />
            )}
            resetKey={`${resourceKey}:${container}:${connection.attempt}`}
          >
            {children({ attempt: connection.attempt, container, fullscreen, onConnectionChange: connection.onStateChange })}
          </LazyLoadBoundary>
        </div>
      </DialogContent>
    </Dialog>
  )
}
