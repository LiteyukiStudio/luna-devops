import type { Release } from '@/api'
import { lazy } from 'react'
import { useTranslation } from 'react-i18next'
import { RuntimeWebConsoleDialog } from '@/components/common/runtime-web-console-dialog'

const RuntimeTerminalPanel = lazy(() =>
  import('@/components/common/runtime-terminal-panel').then(module => ({ default: module.RuntimeTerminalPanel })),
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

  return (
    <RuntimeWebConsoleDialog
      closeLabel={t('common.close')}
      containerLabel={t('deploymentsPage.container')}
      containerPlaceholder={t('deploymentsPage.webConsoleContainerPlaceholder')}
      description={t('deploymentsPage.webConsoleDescription')}
      exitFullscreenLabel={t('deploymentsPage.exitFullscreen')}
      fullscreenLabel={t('deploymentsPage.fullscreen')}
      loadingLabel={t('common.loading')}
      open={Boolean(release)}
      resourceKey={releaseId}
      resourceLabel={releaseId}
      title={t('deploymentsPage.webConsole')}
      onOpenChange={onOpenChange}
    >
      {({ container, fullscreen }) => (
        <RuntimeTerminalPanel
          key={`${releaseId}:${container}`}
          container={container}
          fullscreen={fullscreen}
          projectId={projectId}
          release={release}
        />
      )}
    </RuntimeWebConsoleDialog>
  )
}
