import type { Application, ApplicationDeletionPreview, ApplicationPayload, ApplicationTopology, DataExportAuthorization, DeploymentTarget, DeploymentTargetPayload, PaginatedResponse, PaginationParams, RepositoryBinding, RepositoryBindingPayload, RetainedVolume } from '../types'
import { paginationQuery, request } from '../core'
import { selectionItems, selectionPageParams } from '../selection-page'

export const applicationsApi = {
  listApplications: (projectId: string) =>
    request<PaginatedResponse<Application>>(`/projects/${projectId}/applications?${paginationQuery(selectionPageParams)}`).then(selectionItems),
  listApplicationsPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<Application>>(`/projects/${projectId}/applications?${paginationQuery(params)}`),
  getApplication: (projectId: string, applicationId: string) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`),
  getApplicationTopology: (projectId: string, applicationId: string) =>
    request<ApplicationTopology>(`/projects/${projectId}/applications/${applicationId}/topology`),
  createApplication: (projectId: string, payload: ApplicationPayload) =>
    request<Application>(`/projects/${projectId}/applications`, { method: 'POST', body: JSON.stringify(payload) }),
  updateApplication: (projectId: string, applicationId: string, payload: ApplicationPayload) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  previewApplicationDeletion: (projectId: string, applicationId: string) =>
    request<ApplicationDeletionPreview>(`/projects/${projectId}/applications/${applicationId}/deletion-preview`),
  deleteApplication: (projectId: string, applicationId: string, dataAction: 'retain' | 'delete') =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`, { method: 'DELETE', body: JSON.stringify({ dataAction }) }),
  listRetainedVolumes: (projectId: string) =>
    request<PaginatedResponse<RetainedVolume>>(`/projects/${projectId}/retained-volumes?${paginationQuery(selectionPageParams)}`).then(selectionItems),
  deleteRetainedVolume: (projectId: string, retainedVolumeId: string) =>
    request<void>(`/projects/${projectId}/retained-volumes/${retainedVolumeId}`, { method: 'DELETE' }),
  listDeploymentTargets: (projectId: string, applicationId: string) =>
    request<PaginatedResponse<DeploymentTarget>>(`/projects/${projectId}/applications/${applicationId}/deployment-targets?${paginationQuery(selectionPageParams)}`).then(selectionItems),
  listDeploymentTargetsPage: (projectId: string, applicationId: string, params: PaginationParams) =>
    request<PaginatedResponse<DeploymentTarget>>(`/projects/${projectId}/applications/${applicationId}/deployment-targets?${paginationQuery(params)}`),
  createDeploymentTarget: (projectId: string, applicationId: string, payload: DeploymentTargetPayload) =>
    request<DeploymentTarget>(`/projects/${projectId}/applications/${applicationId}/deployment-targets`, { method: 'POST', body: JSON.stringify(payload) }),
  updateDeploymentTarget: (projectId: string, applicationId: string, targetId: string, payload: DeploymentTargetPayload) =>
    request<DeploymentTarget>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  restartDeploymentTarget: (projectId: string, applicationId: string, targetId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/restart`, { method: 'POST' }),
  authorizeDeploymentTargetDataExport: (projectId: string, applicationId: string, targetId: string) =>
    request<DataExportAuthorization>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/data-export/authorize`, { method: 'POST' }),
  deleteDeploymentTarget: (projectId: string, applicationId: string, targetId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}`, { method: 'DELETE' }),
  listRepositoryBindings: (projectId: string, applicationId?: string) => {
    const search = new URLSearchParams(paginationQuery(selectionPageParams))
    if (applicationId)
      search.set('applicationId', applicationId)
    return request<PaginatedResponse<RepositoryBinding>>(`/projects/${projectId}/repository-bindings?${search.toString()}`).then(selectionItems)
  },
  listRepositoryBindingsPage: (projectId: string, params: PaginationParams & { applicationId?: string }) => {
    const search = new URLSearchParams(paginationQuery(params))
    if (params.applicationId)
      search.set('applicationId', params.applicationId)
    return request<PaginatedResponse<RepositoryBinding>>(`/projects/${projectId}/repository-bindings?${search.toString()}`)
  },
  createRepositoryBinding: (projectId: string, payload: RepositoryBindingPayload) =>
    request<RepositoryBinding>(`/projects/${projectId}/repository-bindings`, { method: 'POST', body: JSON.stringify(payload) }),
  updateRepositoryBinding: (projectId: string, bindingId: string, payload: RepositoryBindingPayload) =>
    request<RepositoryBinding>(`/projects/${projectId}/repository-bindings/${bindingId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRepositoryBinding: (projectId: string, bindingId: string) =>
    request<void>(`/projects/${projectId}/repository-bindings/${bindingId}`, { method: 'DELETE' }),
  createRepositoryWebhook: (projectId: string, bindingId: string) =>
    request<RepositoryBinding>(`/projects/${projectId}/repository-bindings/${bindingId}/webhook`, { method: 'POST' }),
  reconfigureRepositoryWebhook: (projectId: string, bindingId: string) =>
    request<RepositoryBinding>(`/projects/${projectId}/repository-bindings/${bindingId}/webhook/reconfigure`, { method: 'POST' }),
}
