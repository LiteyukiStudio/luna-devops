import type { Release } from '@/api'
import { Maximize2, Minimize2 } from 'lucide-react'
import { lazy, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { apiBaseOrigin } from '@/api'
import { CliCommandSnippet } from '@/components/common/cli-command-snippet'
import { LazyLoadBoundary } from '@/components/common/lazy-load-boundary'
import { OutputViewerSheet } from '@/components/common/output-viewer-sheet'
import { StatusValueBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

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
  const [container, setContainer] = useState('')
  const [expanded, setExpanded] = useState(false)

  return (
    <OutputViewerSheet
      contentClassName={expanded ? 'w-screen' : undefined}
      description={t('deploymentsPage.webConsoleDescription')}
      title={t('deploymentsPage.webConsole')}
      titleAccessory={<StatusValueBadge labelKeyPrefix="buildsPage.statuses" value={release.status} />}
      onClose={onClose}
    >
      <div className="grid shrink-0 gap-2 border-b border-border bg-muted/30 px-4 py-3">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground" title={release.id}>{release.id}</span>
          <label className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
            <span>{t('deploymentsPage.container')}</span>
            <Input
              className="h-8 w-36 bg-background font-mono text-xs"
              placeholder={t('deploymentsPage.webConsoleContainerPlaceholder')}
              value={container}
              onChange={event => setContainer(event.target.value)}
            />
          </label>
          <Button
            aria-label={expanded ? t('deploymentsPage.exitFullscreen') : t('deploymentsPage.fullscreen')}
            size="icon"
            title={expanded ? t('deploymentsPage.exitFullscreen') : t('deploymentsPage.fullscreen')}
            type="button"
            variant="ghost"
            onClick={() => setExpanded(value => !value)}
          >
            {expanded ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}
          </Button>
        </div>
        <CliCommandSnippet
          spec={{
            command: 'deployment.exec',
            server: apiBaseOrigin(),
            projectId,
            applicationId: release.applicationId,
            targetId: release.deploymentTargetId,
            container,
          }}
        />
      </div>
      <div className="min-h-0 flex-1 bg-zinc-950">
        <LazyLoadBoundary
          fallback={<div className="grid h-full min-h-80 place-items-center text-sm text-zinc-400" role="status">{t('common.loading')}</div>}
          resetKey={`${release.id}:${container}`}
        >
          <RuntimeTerminalPanel
            key={`${release.id}:${container}`}
            container={container}
            fullscreen
            projectId={projectId}
            release={release}
          />
        </LazyLoadBoundary>
      </div>
    </OutputViewerSheet>
  )
}
