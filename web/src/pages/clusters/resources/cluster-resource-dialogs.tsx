import type { ClusterResource, ClusterResourceEvent, ClusterResourceYAML, RuntimeCluster } from '@/api'
import { useTranslation } from 'react-i18next'
import { CodeEditor } from '@/components/common/code-editor'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ToolViewportSkeleton } from '@/components/common/loading-states'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ClusterResourceWebConsoleDialog } from './cluster-resource-web-console-dialog'
import { ClusterResourceEventsList } from './cluster-resources-panel'

export function ClusterResourceDialogs({
  cluster,
  consoleResource,
  deletePending,
  deleteResourcesPending,
  eventResource,
  events,
  eventsLoading,
  resourceToDelete,
  resourcesToDelete,
  yaml,
  yamlLoading,
  yamlResource,
  onCloseConsole,
  onCloseEvents,
  onCloseYAML,
  onConfirmDelete,
  onConfirmDeleteResources,
  onResourceToDeleteChange,
  onResourcesToDeleteChange,
}: {
  cluster: RuntimeCluster | null
  consoleResource: ClusterResource | null
  deletePending: boolean
  deleteResourcesPending: boolean
  eventResource: ClusterResource | null
  events: ClusterResourceEvent[]
  eventsLoading: boolean
  resourceToDelete: ClusterResource | null
  resourcesToDelete: ClusterResource[]
  yaml?: ClusterResourceYAML
  yamlLoading: boolean
  yamlResource: ClusterResource | null
  onCloseConsole: () => void
  onCloseEvents: () => void
  onCloseYAML: () => void
  onConfirmDelete: () => void
  onConfirmDeleteResources: () => void
  onResourceToDeleteChange: (resource: ClusterResource | null) => void
  onResourcesToDeleteChange: (resources: ClusterResource[]) => void
}) {
  const { t } = useTranslation()

  return (
    <>
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('clustersPage.deleteResourceDescription', { kind: resourceToDelete?.kind ?? '', namespace: resourceToDelete?.namespace || '-', name: resourceToDelete?.name ?? '' })}
        open={Boolean(resourceToDelete)}
        pending={deletePending}
        title={t('clustersPage.deleteResourceTitle')}
        onConfirm={onConfirmDelete}
        onOpenChange={open => !open && onResourceToDeleteChange(null)}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('clustersPage.deleteResourcesDescription', { count: resourcesToDelete.length })}
        open={resourcesToDelete.length > 0}
        pending={deleteResourcesPending}
        title={t('clustersPage.deleteResourcesTitle')}
        onConfirm={onConfirmDeleteResources}
        onOpenChange={open => !open && onResourcesToDeleteChange([])}
      />
      <Dialog open={Boolean(eventResource)} onOpenChange={open => !open && onCloseEvents()}>
        <DialogContent className="flex max-h-[min(88vh,42rem)] w-[min(92vw,56rem)] max-w-[92vw] min-w-0 flex-col overflow-hidden p-0">
          <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
            <DialogTitle>{t('clustersPage.resourceEventsTitle')}</DialogTitle>
            <DialogDescription>
              {eventResource ? t('clustersPage.resourceEventsDescription', { kind: eventResource.kind, namespace: eventResource.namespace || '-', name: eventResource.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            <ClusterResourceEventsList events={events} loading={eventsLoading} />
          </div>
        </DialogContent>
      </Dialog>
      <ClusterResourceWebConsoleDialog
        cluster={cluster}
        pod={consoleResource}
        onOpenChange={open => !open && onCloseConsole()}
      />
      <Dialog open={Boolean(yamlResource)} onOpenChange={open => !open && onCloseYAML()}>
        <DialogContent className="flex max-h-[min(88vh,46rem)] w-[min(92vw,64rem)] max-w-[92vw] min-w-0 flex-col overflow-hidden p-0">
          <DialogHeader className="shrink-0 border-b border-border p-5 pb-4">
            <DialogTitle>{t('clustersPage.resourceYamlTitle')}</DialogTitle>
            <DialogDescription>
              {yamlResource ? t('clustersPage.resourceYamlDescription', { kind: yamlResource.kind, namespace: yamlResource.namespace || '-', name: yamlResource.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            {yamlLoading
              ? <ToolViewportSkeleton />
              : <CodeEditor height="32rem" language="yaml" readOnly value={yaml?.yaml ?? ''} onChange={() => {}} />}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
