import type { Release } from '@/api'
import { LoaderCircle, RotateCw } from 'lucide-react'
import { lazy, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { apiBaseOrigin } from '@/api'
import { CliCommandSnippet } from '@/components/common/cli-command-snippet'
import { LazyLoadBoundary } from '@/components/common/lazy-load-boundary'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { TerminalConnectionStatus, TerminalPlaceholder, ViewerToolbar, ViewerToolbarButton } from '@/components/common/terminal-chrome'
import { useTerminalConnection } from '@/components/common/use-terminal-connection'
import { Input } from '@/components/ui/input'
import { RuntimePodSelect } from './runtime-pod-select'

const RuntimeTerminalPanel = lazy(() =>
  import('@/components/common/runtime-terminal-panel').then(module => ({ default: module.RuntimeTerminalPanel })),
)

export function ApplicationWebConsolePanel({
  projectId,
  release,
  onClose,
}: {
  projectId: string
  release: Release
  onClose: () => void
}) {
  const { t } = useTranslation()
  const containerInputId = useId()
  const [pod, setPod] = useState('')
  const [container, setContainer] = useState('')
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
          <Input
            className="h-8 w-36 font-mono text-xs"
            id={containerInputId}
            placeholder={t('deploymentsPage.webConsoleContainerPlaceholder')}
            value={container}
            onChange={event => setContainer(event.target.value)}
          />
        </div>
      </ViewerToolbar>
      <div className="shrink-0 border-b border-border bg-surface-subtle px-4 py-2">
        <CliCommandSnippet
          spec={{
            command: 'deployment.exec',
            server: apiBaseOrigin(),
            projectId,
            applicationId: release.applicationId,
            targetId: release.deploymentTargetId,
            pod,
            container,
          }}
        />
      </div>
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
