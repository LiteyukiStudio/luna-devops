import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { liveObservationQueryPolicy } from '@/lib/live-observation-query'

const SELECTOR_PAGE_SIZE = 50

export function useProjectRuntimeClusters(projectId: string, enabled = true) {
  return useInfiniteQuery({
    queryKey: ['project-volume-clusters', projectId],
    queryFn: ({ pageParam }) => api.listRuntimeClustersPage({
      page: pageParam,
      pageSize: SELECTOR_PAGE_SIZE,
      projectId,
      sortBy: 'name',
      sortOrder: 'asc',
    }),
    initialPageParam: 1,
    getNextPageParam: page => page.page < page.totalPages ? page.page + 1 : undefined,
    enabled: enabled && Boolean(projectId),
  })
}

export function useProjectVolumeStorageClasses(projectId: string, clusterId: string, enabled = true) {
  return useInfiniteQuery({
    ...liveObservationQueryPolicy,
    queryKey: ['project-volume-storage-classes', projectId, clusterId],
    queryFn: ({ pageParam }) => api.listProjectVolumeStorageClasses(projectId, clusterId, {
      page: pageParam,
      pageSize: SELECTOR_PAGE_SIZE,
      sortBy: 'name',
      sortOrder: 'asc',
    }),
    initialPageParam: 1,
    getNextPageParam: page => page.page < page.totalPages ? page.page + 1 : undefined,
    enabled: enabled && Boolean(projectId) && Boolean(clusterId),
  })
}
