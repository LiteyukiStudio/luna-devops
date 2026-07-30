import type { RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ContentTabs } from '@/components/common/content-tabs'
import { Button } from '@/components/ui/button'
import { NativeSelect as Select } from '@/components/ui/native-select'
import { TabsContent } from '@/components/ui/tabs'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'
import { ClusterFormDialog } from './cluster-form-dialog'
import { canManageCluster } from './cluster-helpers'
import { ClusterResourceDialogs } from './cluster-resource-dialogs'
import { ClusterResourcesPanel } from './cluster-resources-panel'
import { RuntimeClusterTable } from './runtime-cluster-table'
import { useClusterResources } from './use-cluster-resources'

const RESOURCE_TABS = ['namespaces', 'workloads', 'services', 'configs', 'storage']

export function ClustersPage() {
  const { t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState('clusters')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [dialogRevision, setDialogRevision] = useState(0)
  const [editingCluster, setEditingCluster] = useState<RuntimeCluster | null>(null)
  const [clusterToDelete, setClusterToDelete] = useState<RuntimeCluster | null>(null)
  const [clusterPage, setClusterPage] = useState(1)
  const [clusterPageSize, setClusterPageSize] = useState(10)
  const projects = useQuery({ queryKey: ['projects'], queryFn: api.listProjects })
  const clusters = useQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['runtime-clusters', 'page', clusterPage, clusterPageSize],
    queryFn: () => api.listRuntimeClustersPage({ page: clusterPage, pageSize: clusterPageSize, sortBy: 'createdAt', sortOrder: 'desc' }),
  })
  const clusterOptions = useQuery({ ...liveObservationQueryPolicy, queryKey: ['runtime-clusters', 'options'], queryFn: () => api.listRuntimeClusters() })
  const manageableClusters = useMemo(
    () => (clusterOptions.data ?? []).filter(cluster => canManageCluster(cluster, user?.id, user?.role)),
    [clusterOptions.data, user?.id, user?.role],
  )
  const resources = useClusterResources({ activeTab, manageableClusters, user })

  const deleteCluster = useMutation({
    mutationFn: api.deleteRuntimeCluster,
    onSuccess: () => {
      toast.success(t('deploymentsPage.clusterDeleted'))
      setClusterToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['runtime-clusters'] })
    },
    onError: error => toast.error(error.message),
  })
  const testCluster = useMutation({
    mutationFn: api.testRuntimeCluster,
    onSuccess: () => toast.success(t('deploymentsPage.clusterTested')),
    onError: error => toast.error(error.message),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ['runtime-clusters'] }),
  })

  const openClusterDialog = (cluster?: RuntimeCluster) => {
    setEditingCluster(cluster ?? null)
    setDialogRevision(revision => revision + 1)
    setDialogOpen(true)
  }

  return (
    <div className="grid gap-4">
      <ContentTabs
        tabs={[
          { label: t('clustersPage.runtimeClustersTab'), value: 'clusters' },
          { label: t('clustersPage.namespacesTab'), value: 'namespaces' },
          { label: t('clustersPage.workloadsTab'), value: 'workloads' },
          { label: t('clustersPage.servicesTab'), value: 'services' },
          { label: t('clustersPage.configsTab'), value: 'configs' },
          { label: t('clustersPage.storageTab'), value: 'storage' },
        ]}
        tools={(
          <div className="flex flex-wrap items-center gap-2">
            {activeTab === 'clusters'
              ? (
                  <Button onClick={() => openClusterDialog()}>
                    <Plus className="size-4" />
                    {t('deploymentsPage.createCluster')}
                  </Button>
                )
              : (
                  <>
                    <Select
                      aria-label={t('clustersPage.selectResourceCluster')}
                      className="h-9"
                      containerClassName="w-52 max-w-full"
                      disabled={manageableClusters.length === 0}
                      value={resources.selectedResourceCluster?.id ?? ''}
                      onChange={event => resources.selectResourceCluster(event.target.value)}
                    >
                      {manageableClusters.length > 0
                        ? manageableClusters.map(cluster => <option key={cluster.id} value={cluster.id}>{cluster.name}</option>)
                        : <option value="">{t('clustersPage.noManageableClusterTitle')}</option>}
                    </Select>
                    <Button
                      disabled={!resources.selectedResourceCluster || resources.clusterResources.isFetching}
                      variant="secondary"
                      onClick={() => resources.clusterResources.refetch()}
                    >
                      <RefreshCw className="size-4" />
                      {t('common.refresh')}
                    </Button>
                    {resources.visibleSelectedResourceKeys.length > 0 && (
                      <span className="text-xs text-muted-foreground">
                        {t('clustersPage.selectedResources', { count: resources.selectedDeletableResources.length })}
                      </span>
                    )}
                    <Button
                      disabled={resources.selectedDeletableResources.length === 0 || resources.deleteResources.isPending}
                      variant="destructive"
                      onClick={() => resources.setResourcesToDelete(resources.selectedDeletableResources)}
                    >
                      <Trash2 className="size-4" />
                      {t('clustersPage.deleteSelectedResources')}
                    </Button>
                  </>
                )}
          </div>
        )}
        value={activeTab}
        onValueChange={(value) => {
          setActiveTab(value)
          resources.resetPageAndSelection()
        }}
      >
        <TabsContent value="clusters">
          <RuntimeClusterTable
            clusters={clusters.data?.items ?? []}
            pagination={{
              page: clusters.data?.page ?? clusterPage,
              pageSize: clusters.data?.pageSize ?? clusterPageSize,
              total: clusters.data?.total ?? 0,
              totalPages: clusters.data?.totalPages ?? 0,
              onPageChange: setClusterPage,
              onPageSizeChange: (nextPageSize) => {
                setClusterPageSize(nextPageSize)
                setClusterPage(1)
              },
            }}
            projects={projects.data ?? []}
            user={user}
            onDelete={setClusterToDelete}
            onEdit={openClusterDialog}
            onTest={clusterId => testCluster.mutate(clusterId)}
          />
        </TabsContent>
        {RESOURCE_TABS.map(tab => (
          <TabsContent key={tab} value={tab}>
            <ClusterResourcesPanel
              items={activeTab === tab ? resources.activeResourceItems : []}
              loading={activeTab === tab && resources.clusterResources.isFetching}
              pagination={activeTab === tab ? resources.resourcePagination : undefined}
              selectedCluster={resources.selectedResourceCluster}
              selectedResourceKeys={activeTab === tab ? resources.selectedResourceKeys : []}
              tab={tab}
              user={user}
              onDeleteResource={resources.setResourceToDelete}
              onOpenConsole={resources.setConsoleResource}
              onOpenEvents={resources.setEventResource}
              onOpenYAML={resources.setYamlResource}
              onSelectionChange={resources.setSelectedResourceKeys}
            />
          </TabsContent>
        ))}
      </ContentTabs>

      <ClusterFormDialog
        key={dialogRevision}
        editingCluster={editingCluster}
        open={dialogOpen}
        projects={projects.data ?? []}
        user={user}
        onOpenChange={setDialogOpen}
        onSaved={() => {
          setDialogOpen(false)
          setEditingCluster(null)
        }}
      />
      <ConfirmDialog
        cancelText={t('common.cancel')}
        confirmText={t('common.delete')}
        description={t('deploymentsPage.deleteClusterDescription')}
        open={Boolean(clusterToDelete)}
        title={t('deploymentsPage.deleteClusterTitle')}
        onConfirm={() => clusterToDelete && deleteCluster.mutate(clusterToDelete.id)}
        onOpenChange={open => !open && setClusterToDelete(null)}
      />
      <ClusterResourceDialogs
        cluster={resources.selectedResourceCluster ?? null}
        consoleResource={resources.consoleResource}
        deletePending={resources.deleteResource.isPending}
        deleteResourcesPending={resources.deleteResources.isPending}
        eventResource={resources.eventResource}
        events={resources.resourceEvents.data ?? []}
        eventsLoading={resources.resourceEvents.isFetching}
        resourceToDelete={resources.resourceToDelete}
        resourcesToDelete={resources.resourcesToDelete}
        yaml={resources.resourceYAML.data}
        yamlLoading={resources.resourceYAML.isFetching}
        yamlResource={resources.yamlResource}
        onCloseConsole={() => resources.setConsoleResource(null)}
        onCloseEvents={() => resources.setEventResource(null)}
        onCloseYAML={() => resources.setYamlResource(null)}
        onConfirmDelete={() => resources.resourceToDelete && resources.deleteResource.mutate(resources.resourceToDelete)}
        onConfirmDeleteResources={() => resources.resourcesToDelete.length > 0 && resources.deleteResources.mutate(resources.resourcesToDelete)}
        onResourceToDeleteChange={resources.setResourceToDelete}
        onResourcesToDeleteChange={resources.setResourcesToDelete}
      />
    </div>
  )
}
