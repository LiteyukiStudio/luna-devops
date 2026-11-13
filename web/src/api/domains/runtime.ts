import type { ClusterResource, ClusterResourceEvent, ClusterResourceYAML, DeploymentTargetEventPage, DeploymentTargetRuntimePodPage, EgressException, EgressExceptionPayload, NetworkServiceShare, PaginatedResponse, PaginationParams, Release, ReleaseImageCandidates, ReleaseLog, ReleaseRuntimeLog, ResultVisibility, RuntimeCluster, RuntimeClusterNetworkPolicyObservation, RuntimeClusterPressure, RuntimeClusterResourceCategory, RuntimeClusterResourceKind, RuntimeClusterResourceListParams } from '../types'
import { paginationQuery, paginationWithProjectQuery, request, runtimeClusterResourceListQuery } from '../core'
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
  observeRuntimeClusterNetworkPolicies: (clusterId: string, signal?: AbortSignal) =>
    request<RuntimeClusterNetworkPolicyObservation>(`/runtime/clusters/${clusterId}/network-policies`, { signal }),
  listRuntimeClusterEgressExceptions: (clusterId: string, params: PaginationParams) =>
    request<PaginatedResponse<EgressException>>(`/runtime/clusters/${clusterId}/egress-exceptions?${paginationQuery(params)}`),
  createRuntimeClusterEgressException: (clusterId: string, payload: EgressExceptionPayload) =>
    request<EgressException>(`/runtime/clusters/${clusterId}/egress-exceptions`, { method: 'POST', body: JSON.stringify(payload) }),
  updateRuntimeClusterEgressException: (clusterId: string, exceptionId: string, payload: EgressExceptionPayload) =>
    request<EgressException>(`/runtime/clusters/${clusterId}/egress-exceptions/${exceptionId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  listRuntimeClusterNetworkShareApprovals: (clusterId: string, params: PaginationParams) =>
    request<PaginatedResponse<NetworkServiceShare>>(`/runtime/clusters/${clusterId}/network-share-approvals?${paginationQuery(params)}`),
  approveRuntimeClusterNetworkShare: (clusterId: string, shareId: string) =>
    request<NetworkServiceShare>(`/runtime/clusters/${clusterId}/network-share-approvals/${shareId}/approve`, { method: 'POST' }),
  deleteRuntimeClusterEgressException: (clusterId: string, exceptionId: string) =>
    request<void>(`/runtime/clusters/${clusterId}/egress-exceptions/${exceptionId}`, { method: 'DELETE' }),
  createRuntimeCluster: (payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>('/runtime/clusters', { method: 'POST', body: JSON.stringify(payload) }),
  updateRuntimeCluster: (clusterId: string, payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRuntimeCluster: (clusterId: string) =>
    request<void>(`/runtime/clusters/${clusterId}`, { method: 'DELETE' }),
  testRuntimeCluster: (clusterId: string) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}/test`, { method: 'POST' }),
  listRuntimeClusterResources: (clusterId: string, params: { resourceCategory: RuntimeClusterResourceCategory, namespace?: string, projectId?: string, visibility?: ResultVisibility, applicationId?: string, deploymentTargetId?: string }) => {
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
  listReleaseImageCandidates: (projectId: string, applicationId: string, targetId: string) =>
    request<ReleaseImageCandidates>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/release-image-candidates`),
  createRelease: (projectId: string, payload: Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId' | 'failureCode'>) =>
    request<Release>(`/projects/${projectId}/releases`, { method: 'POST', body: JSON.stringify(payload) }),
  getReleaseLogs: (projectId: string, releaseId: string, signal?: AbortSignal) =>
    request<ReleaseLog>(`/projects/${projectId}/releases/${releaseId}/logs`, { signal }),
  listDeploymentTargetRuntimePods: (projectId: string, applicationId: string, targetId: string, params: { releaseId?: string, search?: string, sortBy?: 'recommended' | 'name' | 'createdAt' | 'restartCount', sortOrder?: 'asc' | 'desc' } = {}, signal?: AbortSignal) => {
    const search = new URLSearchParams(paginationQuery({
      ...selectionPageParams,
      sortBy: params.sortBy ?? 'recommended',
      sortOrder: params.sortOrder,
      search: params.search?.trim(),
    }))
    if (params.releaseId)
      search.set('releaseId', params.releaseId)
    return request<DeploymentTargetRuntimePodPage>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/pods?${search.toString()}`, { signal })
  },
  listDeploymentTargetEvents: (projectId: string, applicationId: string, targetId: string, params: { type?: 'Warning' | 'Normal', search?: string, page?: number, pageSize?: number } = {}, signal?: AbortSignal) => {
    const search = new URLSearchParams(paginationQuery({
      page: params.page ?? 1,
      pageSize: params.pageSize ?? 50,
      sortBy: 'lastSeen',
      sortOrder: 'desc',
      search: params.search?.trim(),
    }))
    if (params.type)
      search.set('type', params.type)
    return request<DeploymentTargetEventPage>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/events?${search.toString()}`, { signal })
  },
  getDeploymentTargetRuntimeLogs: (projectId: string, applicationId: string, targetId: string, params: { pod?: string, container?: string, releaseId?: string, tailLines?: number, previous?: boolean, timestamps?: boolean, preferAbnormal?: boolean } = {}, signal?: AbortSignal) => {
    const search = new URLSearchParams()
    if (params.pod)
      search.set('pod', params.pod)
    if (params.container)
      search.set('container', params.container)
    if (params.releaseId)
      search.set('releaseId', params.releaseId)
    if (params.tailLines)
      search.set('tailLines', String(params.tailLines))
    if (params.previous)
      search.set('previous', 'true')
    if (params.timestamps)
      search.set('timestamps', 'true')
    if (params.preferAbnormal !== undefined)
      search.set('preferAbnormal', params.preferAbnormal ? 'true' : 'false')
    const query = search.toString()
    return request<ReleaseRuntimeLog>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/runtime-logs${query ? `?${query}` : ''}`, { signal })
  },
  getReleaseRuntimeLogs: (projectId: string, releaseId: string, params: { pod?: string, container?: string, tailLines?: number, previous?: boolean, timestamps?: boolean, preferAbnormal?: boolean } = {}, signal?: AbortSignal) => {
    const search = new URLSearchParams()
    if (params.pod)
      search.set('pod', params.pod)
    if (params.container)
      search.set('container', params.container)
    if (params.tailLines)
      search.set('tailLines', String(params.tailLines))
    if (params.previous)
      search.set('previous', 'true')
    if (params.timestamps)
      search.set('timestamps', 'true')
    if (params.preferAbnormal)
      search.set('preferAbnormal', 'true')
    const query = search.toString()
    return request<ReleaseRuntimeLog>(`/projects/${projectId}/releases/${releaseId}/runtime-logs${query ? `?${query}` : ''}`, { signal })
  },
  rollbackRelease: (projectId: string, releaseId: string) =>
    request<Release>(`/projects/${projectId}/releases/${releaseId}/rollback`, { method: 'POST' }),
  cancelRelease: (projectId: string, releaseId: string) =>
    request<Release>(`/projects/${projectId}/releases/${releaseId}/cancel`, { method: 'POST' }),
}
