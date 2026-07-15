import type { ClusterResourcePagination } from './cluster-resources-panel'
import type { ClusterResource, CurrentUser, RuntimeCluster } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { canDeleteClusterResource } from './cluster-resource-utils'

const RESOURCE_PAGE_SIZE_OPTIONS = [10, 20, 50, 100]
interface ResourceViewState { scope: string, page: number, selectedKeys: string[] }

export function useClusterResources({ activeTab, manageableClusters, user }: {
  activeTab: string
  manageableClusters: RuntimeCluster[]
  user?: CurrentUser
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedResourceClusterId, setSelectedResourceClusterId] = useState('')
  const [resourceViewState, setResourceViewState] = useState<ResourceViewState>({ scope: '', page: 1, selectedKeys: [] })
  const [resourcePageSize, setResourcePageSize] = useState(10)
  const [resourceToDelete, setResourceToDelete] = useState<ClusterResource | null>(null)
  const [resourcesToDelete, setResourcesToDelete] = useState<ClusterResource[]>([])
  const [eventResource, setEventResource] = useState<ClusterResource | null>(null)
  const [consoleResource, setConsoleResource] = useState<ClusterResource | null>(null)
  const [yamlResource, setYamlResource] = useState<ClusterResource | null>(null)

  const effectiveResourceClusterId = manageableClusters.some(cluster => cluster.id === selectedResourceClusterId)
    ? selectedResourceClusterId
    : manageableClusters[0]?.id ?? ''
  const selectedResourceCluster = manageableClusters.find(cluster => cluster.id === effectiveResourceClusterId)
  const resourceKind = activeTab === 'clusters' ? 'namespaces' : activeTab
  const resourceScope = `${effectiveResourceClusterId}:${resourceKind}`
  const currentResourceView = resourceViewState.scope === resourceScope
    ? resourceViewState
    : { scope: resourceScope, page: 1, selectedKeys: [] }
  const resourcePage = currentResourceView.page
  const selectedResourceKeys = currentResourceView.selectedKeys
  const updateResourceView = (update: (current: ResourceViewState) => ResourceViewState) => {
    setResourceViewState(current => update(current.scope === resourceScope ? current : { scope: resourceScope, page: 1, selectedKeys: [] }))
  }
  const setSelectedResourceKeys = (selectedKeys: string[]) => {
    updateResourceView(current => ({ ...current, selectedKeys }))
  }
  const setResourcePage = (page: number) => {
    updateResourceView(current => ({ ...current, page }))
  }
  const clusterResources = useQuery({
    queryKey: ['runtime-cluster-resources', selectedResourceCluster?.id, resourceKind, resourcePage, resourcePageSize],
    queryFn: () => api.listRuntimeClusterResourcesPage(selectedResourceCluster?.id ?? '', {
      kind: resourceKind,
      page: resourcePage,
      pageSize: resourcePageSize,
      sortBy: 'updatedAt',
      sortOrder: 'desc',
    }),
    enabled: activeTab !== 'clusters' && Boolean(selectedResourceCluster?.id),
  })
  const activeResourceItems = useMemo(
    () => activeTab === 'clusters' ? [] : clusterResources.data?.items ?? [],
    [activeTab, clusterResources.data?.items],
  )
  const activeResourceKeySet = useMemo(() => new Set(activeResourceItems.map(item => item.id)), [activeResourceItems])
  const visibleSelectedResourceKeys = useMemo(
    () => selectedResourceKeys.filter(key => activeResourceKeySet.has(key)),
    [activeResourceKeySet, selectedResourceKeys],
  )
  const selectedDeletableResources = useMemo(() => {
    const selectedKeys = new Set(visibleSelectedResourceKeys)
    return activeResourceItems.filter(item => selectedKeys.has(item.id) && canDeleteClusterResource(user, item))
  }, [activeResourceItems, user, visibleSelectedResourceKeys])
  const resourceEvents = useQuery({
    queryKey: ['runtime-cluster-resource-events', selectedResourceCluster?.id, eventResource?.kind, eventResource?.namespace, eventResource?.name],
    queryFn: () => api.listRuntimeClusterResourceEvents(selectedResourceCluster?.id ?? '', {
      kind: eventResource?.kind ?? '',
      namespace: eventResource?.namespace,
      name: eventResource?.name ?? '',
    }),
    enabled: Boolean(selectedResourceCluster?.id && eventResource),
  })
  const resourceYAML = useQuery({
    queryKey: ['runtime-cluster-resource-yaml', selectedResourceCluster?.id, yamlResource?.kind, yamlResource?.namespace, yamlResource?.name],
    queryFn: () => api.getRuntimeClusterResourceYAML(selectedResourceCluster?.id ?? '', {
      kind: yamlResource?.kind ?? '',
      namespace: yamlResource?.namespace,
      name: yamlResource?.name ?? '',
    }),
    enabled: Boolean(selectedResourceCluster?.id && yamlResource),
  })

  const deleteResource = useMutation({
    mutationFn: (resource: ClusterResource) => api.deleteRuntimeClusterResource(effectiveResourceClusterId, {
      kind: resource.kind,
      namespace: resource.namespace,
      name: resource.name,
    }),
    onSuccess: () => {
      toast.success(t('clustersPage.resourceDeleted'))
      setResourceToDelete(null)
      queryClient.invalidateQueries({ queryKey: ['runtime-cluster-resources', selectedResourceCluster?.id, resourceKind] })
    },
    onError: error => toast.error(error.message),
  })
  const deleteResources = useMutation({
    mutationFn: async (resources: ClusterResource[]) => {
      for (const resource of resources) {
        await api.deleteRuntimeClusterResource(effectiveResourceClusterId, {
          kind: resource.kind,
          namespace: resource.namespace,
          name: resource.name,
        })
      }
    },
    onSuccess: (_, resources) => {
      toast.success(t('clustersPage.resourcesDeleted', { count: resources.length }))
      setResourcesToDelete([])
      setSelectedResourceKeys([])
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['runtime-cluster-resources', selectedResourceCluster?.id, resourceKind] })
    },
    onError: error => toast.error(error.message),
  })

  const resetPageAndSelection = () => {
    setResourceViewState({ scope: resourceScope, page: 1, selectedKeys: [] })
  }
  const selectResourceCluster = (clusterId: string) => {
    setSelectedResourceClusterId(clusterId)
  }
  const resourcePagination: ClusterResourcePagination | undefined = activeTab === 'clusters'
    ? undefined
    : {
        page: clusterResources.data?.page ?? resourcePage,
        pageSize: clusterResources.data?.pageSize ?? resourcePageSize,
        pageInfoLabel: t('pagination.pageInfo', {
          page: clusterResources.data?.page ?? resourcePage,
          total: clusterResources.data?.total ?? 0,
          totalPages: clusterResources.data?.totalPages ?? 0,
        }),
        pageSizeOptions: RESOURCE_PAGE_SIZE_OPTIONS,
        total: clusterResources.data?.total ?? 0,
        totalPages: clusterResources.data?.totalPages ?? 0,
        onPageChange: (page) => {
          setResourcePage(page)
          setSelectedResourceKeys([])
        },
        onPageSizeChange: (pageSize) => {
          setResourcePageSize(pageSize)
          resetPageAndSelection()
        },
      }

  return {
    activeResourceItems,
    clusterResources,
    consoleResource,
    deleteResource,
    deleteResources,
    eventResource,
    resourceEvents,
    resourcePagination,
    resourceToDelete,
    resourceYAML,
    resourcesToDelete,
    selectedDeletableResources,
    selectedResourceCluster,
    selectedResourceKeys,
    visibleSelectedResourceKeys,
    yamlResource,
    resetPageAndSelection,
    selectResourceCluster,
    setConsoleResource,
    setEventResource,
    setResourceToDelete,
    setResourcesToDelete,
    setSelectedResourceKeys,
    setYamlResource,
  }
}
