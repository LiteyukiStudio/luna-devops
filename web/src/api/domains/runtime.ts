import type { ClusterResource, ClusterResourceEvent, ClusterResourceYAML, PaginatedResponse, Release, ReleaseImageCandidates, ReleaseLog, ReleaseRuntimeExecResult, ReleaseRuntimeLog, RuntimeCluster, RuntimeClusterResourceListParams } from '../types'
import { optionalProjectQuery, request, runtimeClusterResourceListQuery } from '../core'

export const runtimeApi = {
  listRuntimeClusters: (projectId?: string) => request<RuntimeCluster[]>(`/runtime/clusters${optionalProjectQuery(projectId)}`),
  createRuntimeCluster: (payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>('/runtime/clusters', { method: 'POST', body: JSON.stringify(payload) }),
  updateRuntimeCluster: (clusterId: string, payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRuntimeCluster: (clusterId: string) =>
    request<void>(`/runtime/clusters/${clusterId}`, { method: 'DELETE' }),
  testRuntimeCluster: (clusterId: string) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}/test`, { method: 'POST' }),
  listRuntimeClusterResources: (clusterId: string, params: { kind: string, namespace?: string, projectId?: string, applicationId?: string, environmentId?: string }) => {
    const search = new URLSearchParams({ kind: params.kind })
    if (params.namespace)
      search.set('namespace', params.namespace)
    if (params.projectId)
      search.set('projectId', params.projectId)
    if (params.applicationId)
      search.set('applicationId', params.applicationId)
    if (params.environmentId)
      search.set('environmentId', params.environmentId)
    return request<ClusterResource[]>(`/runtime/clusters/${clusterId}/resources?${search.toString()}`)
  },
  listRuntimeClusterResourcesPage: (clusterId: string, params: RuntimeClusterResourceListParams) =>
    request<PaginatedResponse<ClusterResource>>(`/runtime/clusters/${clusterId}/resources?${runtimeClusterResourceListQuery(params)}`),
  listRuntimeClusterResourceEvents: (clusterId: string, params: { kind: string, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ kind: params.kind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<ClusterResourceEvent[]>(`/runtime/clusters/${clusterId}/resource-events?${search.toString()}`)
  },
  getRuntimeClusterResourceYAML: (clusterId: string, params: { kind: string, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ kind: params.kind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<ClusterResourceYAML>(`/runtime/clusters/${clusterId}/resource-yaml?${search.toString()}`)
  },
  deleteRuntimeClusterResource: (clusterId: string, params: { kind: string, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ kind: params.kind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<void>(`/runtime/clusters/${clusterId}/resources?${search.toString()}`, { method: 'DELETE' })
  },
  listReleases: (projectId: string) =>
    request<Release[]>(`/projects/${projectId}/releases`),
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
