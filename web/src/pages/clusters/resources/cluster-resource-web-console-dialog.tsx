import type { ClusterResource, RuntimeCluster } from '@/api'
import { lazy } from 'react'
import { useTranslation } from 'react-i18next'
import { runtimeClusterPodTerminalUrl } from '@/api'
import { RuntimeWebConsoleDialog } from '@/components/common/runtime-web-console-dialog'

const RuntimeTerminalPanel = lazy(() =>
  import('@/components/common/runtime-terminal-panel').then(module => ({ default: module.RuntimeTerminalPanel })),
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
  const podLabel = pod?.namespace && pod?.name ? `${pod.namespace}/${pod.name}` : ''

  return (
    <RuntimeWebConsoleDialog
      closeLabel={t('common.close')}
      containerLabel={t('deploymentsPage.container')}
      containerPlaceholder={t('deploymentsPage.webConsoleContainerPlaceholder')}
      description={t('clustersPage.webConsoleDescription')}
      exitFullscreenLabel={t('deploymentsPage.exitFullscreen')}
      fullscreenLabel={t('deploymentsPage.fullscreen')}
      loadingLabel={t('common.loading')}
      open={Boolean(cluster && pod)}
      resourceKey={podKey}
      resourceLabel={podLabel}
      title={t('clustersPage.webConsole')}
      onOpenChange={onOpenChange}
    >
      {({ container, fullscreen }) => {
        const socketUrl = cluster?.id && pod?.namespace && pod?.name
          ? runtimeClusterPodTerminalUrl(cluster.id, pod.namespace, pod.name, container)
          : ''
        return (
          <RuntimeTerminalPanel
            key={`${podKey}:${container}`}
            container={container}
            fullscreen={fullscreen}
            projectId=""
            ready={Boolean(socketUrl)}
            release={null}
            socketUrl={socketUrl}
          />
        )
      }}
    </RuntimeWebConsoleDialog>
  )
}
