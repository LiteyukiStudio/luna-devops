import type { Application, ApplicationPayload, DeploymentTarget, DeploymentTargetPayload, PaginatedResponse, PaginationParams, RepositoryBinding, RepositoryBindingPayload } from '../types'
import { paginationQuery, request } from '../core'

export const applicationsApi = {
  listApplications: (projectId: string) =>
    request<Application[]>(`/projects/${projectId}/applications`),
  listApplicationsPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<Application>>(`/projects/${projectId}/applications?${paginationQuery(params)}`),
  getApplication: (projectId: string, applicationId: string) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`),
  createApplication: (projectId: string, payload: ApplicationPayload) =>
    request<Application>(`/projects/${projectId}/applications`, { method: 'POST', body: JSON.stringify(payload) }),
  updateApplication: (projectId: string, applicationId: string, payload: ApplicationPayload) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteApplication: (projectId: string, applicationId: string) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`, { method: 'DELETE' }),
  listDeploymentTargets: (projectId: string, applicationId: string) =>
    request<DeploymentTarget[]>(`/projects/${projectId}/applications/${applicationId}/deployment-targets`),
  listDeploymentTargetsPage: (projectId: string, applicationId: string, params: PaginationParams) =>
    request<PaginatedResponse<DeploymentTarget>>(`/projects/${projectId}/applications/${applicationId}/deployment-targets?${paginationQuery(params)}`),
  createDeploymentTarget: (projectId: string, applicationId: string, payload: DeploymentTargetPayload) =>
    request<DeploymentTarget>(`/projects/${projectId}/applications/${applicationId}/deployment-targets`, { method: 'POST', body: JSON.stringify(payload) }),
  updateDeploymentTarget: (projectId: string, applicationId: string, targetId: string, payload: DeploymentTargetPayload) =>
    request<DeploymentTarget>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  restartDeploymentTarget: (projectId: string, applicationId: string, targetId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/restart`, { method: 'POST' }),
  authorizeDeploymentTargetDataExport: (projectId: string, applicationId: string, targetId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/data-export/authorize`, { method: 'POST' }),
  deleteDeploymentTarget: (projectId: string, applicationId: string, targetId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}`, { method: 'DELETE' }),
  listRepositoryBindings: (projectId: string) =>
    request<RepositoryBinding[]>(`/projects/${projectId}/repository-bindings`),
  listRepositoryBindingsPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<RepositoryBinding>>(`/projects/${projectId}/repository-bindings?${paginationQuery(params)}`),
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
