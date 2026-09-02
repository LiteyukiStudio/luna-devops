import type { ClusterResource, ClusterResourceEvent, ClusterResourceYAML, PaginatedResponse, PaginationParams, Release, ReleaseImageCandidates, ReleaseLog, ReleaseRuntimeExecResult, ReleaseRuntimeLog, ResultVisibility, RuntimeCluster, RuntimeClusterPressure, RuntimeClusterResourceCategory, RuntimeClusterResourceKind, RuntimeClusterResourceListParams } from '../types'
import { paginationWithProjectQuery, request, runtimeClusterResourceListQuery } from '../core'
import { selectionItems, selectionPageParams } from '../selection-page'

export const runtimeApi = {
  listRuntimeClusters: (projectId?: string, visibility?: ResultVisibility) =>
    request<PaginatedResponse<RuntimeCluster>>(`/runtime/clusters?${paginationWithProjectQuery({ ...selectionPageParams, projectId, visibility })}`).then(selectionItems),
  listRuntimeClustersPage: (params: PaginationParams & { projectId?: string, visibility?: ResultVisibility }) =>
    request<PaginatedResponse<RuntimeCluster>>(`/runtime/clusters?${paginationWithProjectQuery(params)}`),
  observeRuntimeClusterPressure: (clusterIds: string[], projectId?: string) => {
    const query = new URLSearchParams()
    for (const clusterId of clusterIds)
      query.append('clusterId', clusterId)
    if (projectId)
      query.set('projectId', projectId)
    return request<{ items: RuntimeClusterPressure[] }>(`/runtime/clusters/pressure?${query.toString()}`).then(response => response.items)
  },
  createRuntimeCluster: (payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>('/runtime/clusters', { method: 'POST', body: JSON.stringify(payload) }),
  updateRuntimeCluster: (clusterId: string, payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRuntimeCluster: (clusterId: string) =>
    request<void>(`/runtime/clusters/${clusterId}`, { method: 'DELETE' }),
  testRuntimeCluster: (clusterId: string) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}/test`, { method: 'POST' }),
  listRuntimeClusterResources: (clusterId: string, params: { resourceCategory: RuntimeClusterResourceCategory, namespace?: string, projectId?: string, visibility?: ResultVisibility, applicationId?: string, environmentId?: string }) => {
    const query = runtimeClusterResourceListQuery({ ...selectionPageParams, ...params })
    return request<PaginatedResponse<ClusterResource>>(`/runtime/clusters/${clusterId}/resources?${query}`).then(selectionItems)
  },
  listRuntimeClusterResourcesPage: (clusterId: string, params: RuntimeClusterResourceListParams) =>
    request<PaginatedResponse<ClusterResource>>(`/runtime/clusters/${clusterId}/resources?${runtimeClusterResourceListQuery(params)}`),
  listRuntimeClusterResourceEvents: (clusterId: string, params: { resourceKind: RuntimeClusterResourceKind, namespace?: string, name: string }) => {
    const search = new URLSearchParams({
      resourceKind: params.resourceKind,
      name: params.name,
      page: String(selectionPageParams.page),
      pageSize: String(selectionPageParams.pageSize),
    })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<PaginatedResponse<ClusterResourceEvent>>(`/runtime/clusters/${clusterId}/resource-events?${search.toString()}`).then(selectionItems)
  },
  getRuntimeClusterResourceYAML: (clusterId: string, params: { resourceKind: RuntimeClusterResourceKind, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ resourceKind: params.resourceKind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<ClusterResourceYAML>(`/runtime/clusters/${clusterId}/resource-yaml?${search.toString()}`)
  },
  deleteRuntimeClusterResource: (clusterId: string, params: { resourceKind: RuntimeClusterResourceKind, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ resourceKind: params.resourceKind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<void>(`/runtime/clusters/${clusterId}/resources?${search.toString()}`, { method: 'DELETE' })
  },
  listReleases: (projectId: string, applicationId?: string) => {
    const search = new URLSearchParams(paginationWithProjectQuery(selectionPageParams))
    if (applicationId)
      search.set('applicationId', applicationId)
    return request<PaginatedResponse<Release>>(`/projects/${projectId}/releases?${search.toString()}`).then(selectionItems)
  },
  listReleasesPage: (projectId: string, params: PaginationParams & { applicationId?: string }) => {
    const search = new URLSearchParams(paginationWithProjectQuery(params))
    if (params.applicationId)
      search.set('applicationId', params.applicationId)
    return request<PaginatedResponse<Release>>(`/projects/${projectId}/releases?${search.toString()}`)
  },
  listReleaseImageCandidates: (projectId: string, applicationId: string, targetId: string) =>
    request<ReleaseImageCandidates>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/release-image-candidates`),
  createRelease: (projectId: string, payload: Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>) =>
    request<Release>(`/projects/${projectId}/releases`, { method: 'POST', body: JSON.stringify(payload) }),
  getReleaseLogs: (projectId: string, releaseId: string) =>
    request<ReleaseLog>(`/projects/${projectId}/releases/${releaseId}/logs`),
  getReleaseRuntimeLogs: (projectId: string, releaseId: string, params: { container?: string, tailLines?: number } = {}) => {
    const search = new URLSearchParams()
    if (params.container)
      search.set('container', params.container)
    if (params.tailLines)
      search.set('tailLines', String(params.tailLines))
    const query = search.toString()
    return request<ReleaseRuntimeLog>(`/projects/${projectId}/releases/${releaseId}/runtime-logs${query ? `?${query}` : ''}`)
  },
  execReleaseRuntimeCommand: (projectId: string, releaseId: string, payload: { command: string, container?: string }) =>
    request<ReleaseRuntimeExecResult>(`/projects/${projectId}/releases/${releaseId}/exec`, { method: 'POST', body: JSON.stringify(payload) }),
  rollbackRelease: (projectId: string, releaseId: string) =>
    request<Release>(`/projects/${projectId}/releases/${releaseId}/rollback`, { method: 'POST' }),
}
