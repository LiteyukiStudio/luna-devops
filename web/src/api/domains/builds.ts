import type { BuildEnvironmentConfig, BuildEnvironmentConfigParams, BuildEnvironmentConfigPayload, BuildJob, BuildLog, BuildRun, BuildRunListParams, BuildTemplate, BuildTemplatePreview, BuildVariableSet, BuildVariableSetPayload, DeploymentTargetRuntimeSecretsPayload, HookRun, HookRunLog, PaginatedResponse, PaginationParams, ProjectHookConfig, ProjectHookConfigPayload, ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload, ResultVisibility, RuntimeSecretMutationResponse } from '../types'
import { buildRunListQuery, paginationQuery, paginationWithProjectQuery, request } from '../core'
import { selectionItems, selectionPageParams } from '../selection-page'

export const buildsApi = {
  getBuildEnvironmentConfig: (params: BuildEnvironmentConfigParams) =>
    request<BuildEnvironmentConfig>(`/build/environment-config?${buildEnvironmentConfigQuery(params)}`),
  updateBuildEnvironmentConfig: (params: BuildEnvironmentConfigParams, payload: BuildEnvironmentConfigPayload) =>
    request<BuildEnvironmentConfig>(`/build/environment-config?${buildEnvironmentConfigQuery(params)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  listBuildTemplates: () => request<BuildTemplate[]>('/build/templates'),
  previewBuildTemplate: (templateId: string, version: string, values: Record<string, string>) =>
    request<BuildTemplatePreview>(`/build/templates/${templateId}/preview`, { method: 'POST', body: JSON.stringify({ values, version }) }),
  listBuildVariableSets: (projectId?: string, visibility?: ResultVisibility) =>
    request<PaginatedResponse<BuildVariableSet>>(`/build/variable-sets?${paginationWithProjectQuery({ ...selectionPageParams, projectId, visibility })}`).then(selectionItems),
  listBuildVariableSetsPage: (params: PaginationParams & { projectId?: string, visibility?: ResultVisibility }) =>
    request<PaginatedResponse<BuildVariableSet>>(`/build/variable-sets?${paginationWithProjectQuery(params)}`),
  createBuildVariableSet: (payload: BuildVariableSetPayload) =>
    request<BuildVariableSet>('/build/variable-sets', { method: 'POST', body: JSON.stringify(payload) }),
  updateBuildVariableSet: (setId: string, payload: BuildVariableSetPayload) =>
    request<BuildVariableSet>(`/build/variable-sets/${setId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteBuildVariableSet: (setId: string) =>
    request<void>(`/build/variable-sets/${setId}`, { method: 'DELETE' }),
  listProjectRuntimeConfigSets: (projectId: string) =>
    request<PaginatedResponse<ProjectRuntimeConfigSet>>(`/projects/${projectId}/runtime-config-sets?${paginationQuery(selectionPageParams)}`).then(selectionItems),
  listProjectRuntimeConfigSetsPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<ProjectRuntimeConfigSet>>(`/projects/${projectId}/runtime-config-sets?${paginationQuery(params)}`),
  createProjectRuntimeConfigSet: (projectId: string, payload: ProjectRuntimeConfigSetPayload) =>
    request<ProjectRuntimeConfigSet>(`/projects/${projectId}/runtime-config-sets`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectRuntimeConfigSet: (projectId: string, setId: string, payload: ProjectRuntimeConfigSetPayload) =>
    request<ProjectRuntimeConfigSet>(`/projects/${projectId}/runtime-config-sets/${setId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  updateProjectRuntimeConfigSetRuntimeSecrets: (projectId: string, setId: string, payload: DeploymentTargetRuntimeSecretsPayload) =>
    request<RuntimeSecretMutationResponse>(`/projects/${projectId}/runtime-config-sets/${setId}/runtime-secrets`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectRuntimeConfigSet: (projectId: string, setId: string) =>
    request<void>(`/projects/${projectId}/runtime-config-sets/${setId}`, { method: 'DELETE' }),
  listProjectHooks: (projectId: string) =>
    request<PaginatedResponse<ProjectHookConfig>>(`/projects/${projectId}/hooks?${paginationQuery(selectionPageParams)}`).then(selectionItems),
  listProjectHooksPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<ProjectHookConfig>>(`/projects/${projectId}/hooks?${paginationQuery(params)}`),
  createProjectHook: (projectId: string, payload: ProjectHookConfigPayload) =>
    request<ProjectHookConfig>(`/projects/${projectId}/hooks`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectHook: (projectId: string, hookId: string, payload: ProjectHookConfigPayload) =>
    request<ProjectHookConfig>(`/projects/${projectId}/hooks/${hookId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectHook: (projectId: string, hookId: string) =>
    request<void>(`/projects/${projectId}/hooks/${hookId}`, { method: 'DELETE' }),
  listProjectHookRuns: (projectId: string, params: { phase?: string, buildRunId?: string, releaseId?: string } = {}) => {
    const search = new URLSearchParams(paginationQuery(selectionPageParams))
    if (params.phase)
      search.set('phase', params.phase)
    if (params.buildRunId)
      search.set('buildRunId', params.buildRunId)
    if (params.releaseId)
      search.set('releaseId', params.releaseId)
    const query = search.toString()
    return request<PaginatedResponse<HookRun>>(`/projects/${projectId}/hook-runs?${query}`).then(selectionItems)
  },
  getProjectHookRunLogs: (projectId: string, runId: string) =>
    request<HookRunLog>(`/projects/${projectId}/hook-runs/${runId}/logs`),
  listBuildRuns: (projectId: string, applicationId?: string) =>
    request<PaginatedResponse<BuildRun>>(`/projects/${projectId}/build-runs?${buildRunListQuery({ ...selectionPageParams, applicationId })}`).then(selectionItems),
  listBuildRunsPage: (projectId: string, params: BuildRunListParams) =>
    request<PaginatedResponse<BuildRun>>(`/projects/${projectId}/build-runs?${buildRunListQuery(params)}`),
  triggerBuildRun: (projectId: string, payload: Partial<BuildRun>) =>
    request<BuildRun>(`/projects/${projectId}/build-runs/trigger`, { method: 'POST', body: JSON.stringify(payload) }),
  retryBuildRun: (projectId: string, runId: string) =>
    request<BuildRun>(`/projects/${projectId}/build-runs/${runId}/retry`, { method: 'POST' }),
  cancelBuildRun: (projectId: string, runId: string) =>
    request<BuildRun>(`/projects/${projectId}/build-runs/${runId}/cancel`, { method: 'POST' }),
  deleteBuildRun: (projectId: string, runId: string) =>
    request<void>(`/projects/${projectId}/build-runs/${runId}`, { method: 'DELETE' }),
  listBuildJobs: (projectId: string, buildRunId?: string, applicationId?: string) => {
    const query = new URLSearchParams(paginationQuery(selectionPageParams))
    if (buildRunId)
      query.set('buildRunId', buildRunId)
    if (applicationId)
      query.set('applicationId', applicationId)
    return request<PaginatedResponse<BuildJob>>(`/projects/${projectId}/build-jobs?${query.toString()}`).then(selectionItems)
  },
  listBuildJobsPage: (projectId: string, params: PaginationParams, buildRunId?: string, applicationId?: string) => {
    const query = new URLSearchParams(paginationQuery(params))
    if (buildRunId)
      query.set('buildRunId', buildRunId)
    if (applicationId)
      query.set('applicationId', applicationId)
    return request<PaginatedResponse<BuildJob>>(`/projects/${projectId}/build-jobs?${query.toString()}`)
  },
  getBuildJobLogs: (projectId: string, jobId: string) =>
    request<BuildLog>(`/projects/${projectId}/build-jobs/${jobId}/logs`),
}

function buildEnvironmentConfigQuery(params: BuildEnvironmentConfigParams) {
  const query = new URLSearchParams({ scope: params.scope })
  if (params.projectId)
    query.set('projectId', params.projectId)
  if (params.applicationId)
    query.set('applicationId', params.applicationId)
  if (params.deploymentTargetId)
    query.set('deploymentTargetId', params.deploymentTargetId)
  return query.toString()
}
