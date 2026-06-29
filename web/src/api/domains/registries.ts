import type { ArtifactRegistry, ArtifactRegistryPayload, ContainerImage, PaginatedResponse, PaginationParams, RegistryCredential, RegistryRepositoryItem, RegistryTagItem, RegistryTestResult } from '../types'
import { paginationQuery, paginationWithProjectQuery, request } from '../core'

export const registriesApi = {
  listRegistries: (projectId?: string) =>
    request<ArtifactRegistry[]>(`/registries${projectId ? `?projectId=${encodeURIComponent(projectId)}` : ''}`),
  listRegistriesPage: (params: PaginationParams & { projectId?: string }) =>
    request<PaginatedResponse<ArtifactRegistry>>(`/registries?${paginationWithProjectQuery(params)}`),
  createRegistry: (payload: ArtifactRegistryPayload) =>
    request<ArtifactRegistry>('/registries', { method: 'POST', body: JSON.stringify(payload) }),
  updateRegistry: (registryId: string, payload: ArtifactRegistryPayload) =>
    request<ArtifactRegistry>(`/registries/${registryId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRegistry: (registryId: string) =>
    request<void>(`/registries/${registryId}`, { method: 'DELETE' }),
  testRegistry: (registryId: string) =>
    request<RegistryTestResult>(`/registries/${registryId}/test`, { method: 'POST' }),
  getDefaultRegistry: (projectId: string) =>
    request<ArtifactRegistry>(`/projects/${projectId}/registries/default`),
  listRegistryCredentials: (registryId: string) =>
    request<RegistryCredential[]>(`/registries/${registryId}/credentials`),
  listRegistryCredentialsPage: (registryId: string, params: PaginationParams) =>
    request<PaginatedResponse<RegistryCredential>>(`/registries/${registryId}/credentials?${paginationQuery(params)}`),
  createRegistryCredential: (registryId: string, payload: { name: string, username: string, password?: string, token?: string, scope: RegistryCredential['scope'], accessScope: RegistryCredential['accessScope'], repositoryTemplate: string, tagTemplate: string }) =>
    request<RegistryCredential>(`/registries/${registryId}/credentials`, { method: 'POST', body: JSON.stringify(payload) }),
  deleteRegistryCredential: (registryId: string, credentialId: string) =>
    request<void>(`/registries/${registryId}/credentials/${credentialId}`, { method: 'DELETE' }),
  searchRegistryRepositories: (registryId: string, params: { search?: string, page?: number, pageSize?: number }) => {
    const search = new URLSearchParams({ page: String(params.page ?? 1), pageSize: String(params.pageSize ?? 10) })
    if (params.search)
      search.set('search', params.search)
    return request<{ items: RegistryRepositoryItem[], page: number, pageSize: number, total: number, limited: boolean }>(`/registries/${registryId}/repositories/search?${search.toString()}`)
  },
  listRegistryRepositoryTags: (registryId: string, repository: string, limit = 20) => {
    const search = new URLSearchParams({ repository, limit: String(limit) })
    return request<{ items: RegistryTagItem[], total: number, limited: boolean }>(`/registries/${registryId}/repository-tags?${search.toString()}`)
  },
  listContainerImages: (params: PaginationParams & { projectId?: string } = { page: 1, pageSize: 20 }) => {
    const search = new URLSearchParams(paginationQuery(params))
    if (params.projectId)
      search.set('projectId', params.projectId)
    return request<PaginatedResponse<ContainerImage>>(`/container-images?${search.toString()}`)
  },
  createContainerImage: (payload: Omit<ContainerImage, 'id' | 'createdBy' | 'createdAt' | 'imageRef'>) =>
    request<ContainerImage>('/container-images', { method: 'POST', body: JSON.stringify(payload) }),
}
