import type { ClusterResource, RuntimeCluster } from '@/api'
import { Maximize2, Minimize2, Minus, X } from 'lucide-react'
import { lazy, Suspense, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { runtimeClusterPodTerminalUrl } from '@/api'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { WindowControlButton } from '@/pages/applications/application-web-console-dialog'

const ApplicationRuntimeTerminalPanel = lazy(() =>
  import('@/pages/applications/application-runtime-terminal-panel').then(module => ({ default: module.ApplicationRuntimeTerminalPanel })),
)

export function ClusterResourceWebConsoleDialog({
  cluster,
  pod,
  onOpenChange,
}: {
  cluster: RuntimeCluster | null
  pod: ClusterResource | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const podKey = `${cluster?.id ?? ''}:${pod?.namespace ?? ''}:${pod?.name ?? ''}`
  const [containerState, setContainerState] = useState({ podKey: '', value: '' })
  const [fullscreen, setFullscreen] = useState(false)
  const container = containerState.podKey === podKey ? containerState.value : ''
  const socketUrl = useMemo(() => {
    if (!cluster?.id || !pod?.namespace || !pod?.name)
      return ''
    return runtimeClusterPodTerminalUrl(cluster.id, pod.namespace, pod.name, container)
  }, [cluster?.id, container, pod?.name, pod?.namespace])
  const closeDialog = () => {
    setContainerState({ podKey: '', value: '' })
    setFullscreen(false)
    onOpenChange(false)
  }

  return (
    <Dialog
      open={Boolean(cluster && pod)}
      onOpenChange={(open) => {
        if (!open) {
          setContainerState({ podKey: '', value: '' })
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
        <DialogTitle className="sr-only">{t('clustersPage.webConsole')}</DialogTitle>
        <DialogDescription className="sr-only">{t('clustersPage.webConsoleDescription')}</DialogDescription>
        <div className={cn('overflow-hidden rounded-md border border-zinc-800 bg-zinc-950 text-zinc-100 shadow-2xl', fullscreen && 'flex h-full min-h-0 flex-col')}>
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 bg-zinc-900 px-5 py-3">
            <div className="flex min-w-0 items-center gap-2">
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
              <span className="ml-3 min-w-0 truncate font-mono text-xs text-zinc-400">
                {pod?.namespace && pod?.name ? `${pod.namespace}/${pod.name}` : '-'}
              </span>
            </div>
            <div className="flex min-w-0 flex-wrap items-center justify-end gap-2">
              <label className="flex min-w-0 items-center gap-2 font-mono text-xs text-zinc-400">
                <span>{t('deploymentsPage.container')}</span>
                <input
                  className="h-7 w-32 rounded border border-zinc-700 bg-zinc-950 px-2 text-zinc-100 outline-none transition placeholder:text-zinc-600 focus:border-emerald-500"
                  placeholder={t('deploymentsPage.webConsoleContainerPlaceholder')}
                  value={container}
                  onChange={event => setContainerState({ podKey, value: event.target.value })}
                />
              </label>
            </div>
          </div>
          <div className={fullscreen ? 'min-h-0 flex-1' : undefined}>
            <Suspense fallback={<div className={fullscreen ? 'h-full min-h-[28rem] bg-slate-950' : 'h-[29.5rem] bg-slate-950'} />}>
              <ApplicationRuntimeTerminalPanel
                container={container}
                fullscreen={fullscreen}
                projectId=""
                ready={Boolean(socketUrl)}
                release={null}
                socketUrl={socketUrl}
              />
            </Suspense>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
