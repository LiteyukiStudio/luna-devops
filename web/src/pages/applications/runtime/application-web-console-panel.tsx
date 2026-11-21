import type { Release } from '@/api'
import { LoaderCircle, RotateCw } from 'lucide-react'
import { lazy, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LazyLoadBoundary } from '@/components/common/lazy-load-boundary'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { TerminalConnectionStatus, TerminalPlaceholder, ViewerToolbar, ViewerToolbarButton } from '@/components/common/terminal-chrome'
import { useTerminalConnection } from '@/components/common/use-terminal-connection'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { EMPTY_SELECT_VALUE, fromSelectValue, toSelectValue } from '@/components/ui/select-value'
import { RuntimePodSelect } from './runtime-pod-select'
import { collectRuntimeContainerOptions, useReleaseRuntimePods } from './use-release-runtime-pods'

const RuntimeTerminalPanel = lazy(() =>
  import('@/components/common/runtime-terminal-panel').then(module => ({ default: module.RuntimeTerminalPanel })),
)

export function ApplicationWebConsolePanel({
  initialContainer = '',
  initialPod = '',
  projectId,
  release,
  onClose,
}: {
  initialContainer?: string
  initialPod?: string
  projectId: string
  release: Release
  onClose: () => void
}) {
  const { t } = useTranslation()
  const containerInputId = useId()
  const [pod, setPod] = useState(initialPod)
  const [container, setContainer] = useState(initialContainer)
  const runtimePods = useReleaseRuntimePods(projectId, release)
  const containerOptions = useMemo(
    () => collectRuntimeContainerOptions(runtimePods.data?.status === 'ready' ? runtimePods.data.items : [], pod, false),
    [pod, runtimePods.data],
  )
  const connection = useTerminalConnection()
  const terminalKey = `${release.id}:${pod}:${container}:${connection.attempt}`

  return (
    <OutputViewerSheet
      description={t('deploymentsPage.webConsoleDescription')}
      title={t('deploymentsPage.webConsole')}
      titleAccessory={<StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />}
      onClose={onClose}
    >
      <ViewerToolbar
        actions={(
          <ViewerToolbarButton
            disabled={connection.state === 'connecting' || connection.state === 'idle'}
            icon={<RotateCw aria-hidden="true" />}
            label={t('common.terminal.reconnect')}
            onClick={connection.reconnect}
          />
        )}
      >
        <TerminalConnectionStatus state={connection.state} />
        <RuntimePodSelect projectId={projectId} release={release} value={pod} onValueChange={setPod} />
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <label htmlFor={containerInputId}>{t('deploymentsPage.container')}</label>
          {containerOptions.length > 0
            ? (
                <Select
                  value={toSelectValue(container)}
                  onValueChange={next => setContainer(fromSelectValue(next))}
                >
                  <SelectTrigger className="h-8 w-40 font-mono text-xs" id={containerInputId} size="sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={EMPTY_SELECT_VALUE}>{t('deploymentsPage.webConsoleContainerPlaceholder')}</SelectItem>
                    {containerOptions.map(option => (
                      <SelectItem key={option.name} value={option.name}>{option.name}</SelectItem>
                    ))}
                    {container && !containerOptions.some(option => option.name === container) && (
                      <SelectItem value={container}>{container}</SelectItem>
                    )}
                  </SelectContent>
                </Select>
              )
            : (
                <Input
                  className="h-8 w-36 font-mono text-xs"
                  id={containerInputId}
                  placeholder={t('deploymentsPage.webConsoleContainerPlaceholder')}
                  value={container}
                  onChange={event => setContainer(event.target.value)}
                />
              )}
        </div>
      </ViewerToolbar>
      <div className="min-h-0 flex-1 bg-terminal">
        <LazyLoadBoundary
          fallback={<TerminalPlaceholder icon={<LoaderCircle className="motion-safe:animate-spin" />} label={t('common.loading')} />}
          resetKey={terminalKey}
        >
          <RuntimeTerminalPanel
            key={terminalKey}
            container={container}
            fullscreen
            pod={pod}
            projectId={projectId}
            release={release}
            onConnectionChange={connection.onStateChange}
          />
        </LazyLoadBoundary>
      </div>
    </OutputViewerSheet>
  )
}
