import type { Application, ApplicationDeletionPreview, ApplicationListParams, ApplicationPayload, ApplicationTopology, DeploymentTarget, DeploymentTargetBundle, DeploymentTargetBundleImportRequest, DeploymentTargetBundlePreview, DeploymentTargetBundlePreviewRequest, DeploymentTargetPayload, DeploymentTargetRuntimeSecretsPayload, DeploymentTargetRuntimeSecretsSummary, PaginatedResponse, PaginationParams, RepositoryBinding, RepositoryBindingPayload, RuntimeSecretMutationResponse } from '../types'
import { paginationQuery, request } from '../core'
import { selectionItems, selectionPageParams } from '../selection-page'

export const applicationsApi = {
  listApplications: (projectId: string) =>
    request<PaginatedResponse<Application>>(`/projects/${projectId}/applications?${paginationQuery(selectionPageParams)}`).then(selectionItems),
  listApplicationsPage: (projectId: string, params: ApplicationListParams) => {
    const search = new URLSearchParams(paginationQuery(params))
    if (params.includeRuntime)
      search.set('includeRuntime', 'true')
    return request<PaginatedResponse<Application>>(`/projects/${projectId}/applications?${search.toString()}`)
  },
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
  deleteApplication: (projectId: string, applicationId: string) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`, { method: 'DELETE' }),
  listDeploymentTargets: (projectId: string, applicationId: string) =>
    request<PaginatedResponse<DeploymentTarget>>(`/projects/${projectId}/applications/${applicationId}/deployment-targets?${paginationQuery(selectionPageParams)}`).then(selectionItems),
  listDeploymentTargetsPage: (projectId: string, applicationId: string, params: PaginationParams) =>
    request<PaginatedResponse<DeploymentTarget>>(`/projects/${projectId}/applications/${applicationId}/deployment-targets?${paginationQuery(params)}`),
  createDeploymentTarget: (projectId: string, applicationId: string, payload: DeploymentTargetPayload) =>
    request<DeploymentTarget>(`/projects/${projectId}/applications/${applicationId}/deployment-targets`, { method: 'POST', body: JSON.stringify(payload) }),
  updateDeploymentTarget: (projectId: string, applicationId: string, targetId: string, payload: DeploymentTargetPayload) =>
    request<DeploymentTarget>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  getDeploymentTargetRuntimeSecretsSummary: (projectId: string, applicationId: string, targetId: string) =>
    request<DeploymentTargetRuntimeSecretsSummary>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/runtime-secrets`),
  updateDeploymentTargetRuntimeSecrets: (projectId: string, applicationId: string, targetId: string, payload: DeploymentTargetRuntimeSecretsPayload) =>
    request<RuntimeSecretMutationResponse>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/runtime-secrets`, { method: 'PUT', body: JSON.stringify(payload) }),
  exportDeploymentTargetBundle: (projectId: string, applicationId: string, targetId: string) =>
    request<DeploymentTargetBundle>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/export`),
  previewDeploymentTargetBundleImport: (projectId: string, applicationId: string, payload: DeploymentTargetBundlePreviewRequest) =>
    request<DeploymentTargetBundlePreview>(`/projects/${projectId}/applications/${applicationId}/deployment-target-imports/preview`, { method: 'POST', body: JSON.stringify(payload) }),
  importDeploymentTargetBundle: (projectId: string, applicationId: string, payload: DeploymentTargetBundleImportRequest) =>
    request<DeploymentTarget>(`/projects/${projectId}/applications/${applicationId}/deployment-target-imports`, { method: 'POST', body: JSON.stringify(payload) }),
  restartDeploymentTarget: (projectId: string, applicationId: string, targetId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}/deployment-targets/${targetId}/restart`, { method: 'POST' }),
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
