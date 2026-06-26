import type { BuildJob, BuildLog, BuildRun, BuildRunListParams, BuildVariableSet, BuildVariableSetPayload, HookRun, HookRunLog, PaginatedResponse, PaginationParams, ProjectHookConfig, ProjectHookConfigPayload, ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload } from '../types'
import { buildRunListQuery, optionalProjectQuery, paginationQuery, paginationWithProjectQuery, request } from '../core'

export const buildsApi = {
  listBuildVariableSets: (projectId?: string) =>
    request<BuildVariableSet[]>(`/build/variable-sets${optionalProjectQuery(projectId)}`),
  listBuildVariableSetsPage: (params: PaginationParams & { projectId?: string }) =>
    request<PaginatedResponse<BuildVariableSet>>(`/build/variable-sets?${paginationWithProjectQuery(params)}`),
  createBuildVariableSet: (payload: BuildVariableSetPayload) =>
    request<BuildVariableSet>('/build/variable-sets', { method: 'POST', body: JSON.stringify(payload) }),
  updateBuildVariableSet: (setId: string, payload: BuildVariableSetPayload) =>
    request<BuildVariableSet>(`/build/variable-sets/${setId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteBuildVariableSet: (setId: string) =>
    request<void>(`/build/variable-sets/${setId}`, { method: 'DELETE' }),
  listProjectRuntimeConfigSets: (projectId: string) =>
    request<ProjectRuntimeConfigSet[]>(`/projects/${projectId}/runtime-config-sets`),
  listProjectRuntimeConfigSetsPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<ProjectRuntimeConfigSet>>(`/projects/${projectId}/runtime-config-sets?${paginationQuery(params)}`),
  createProjectRuntimeConfigSet: (projectId: string, payload: ProjectRuntimeConfigSetPayload) =>
    request<ProjectRuntimeConfigSet>(`/projects/${projectId}/runtime-config-sets`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectRuntimeConfigSet: (projectId: string, setId: string, payload: ProjectRuntimeConfigSetPayload) =>
    request<ProjectRuntimeConfigSet>(`/projects/${projectId}/runtime-config-sets/${setId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectRuntimeConfigSet: (projectId: string, setId: string) =>
    request<void>(`/projects/${projectId}/runtime-config-sets/${setId}`, { method: 'DELETE' }),
  listProjectHooks: (projectId: string) =>
    request<ProjectHookConfig[]>(`/projects/${projectId}/hooks`),
  createProjectHook: (projectId: string, payload: ProjectHookConfigPayload) =>
    request<ProjectHookConfig>(`/projects/${projectId}/hooks`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectHook: (projectId: string, hookId: string, payload: ProjectHookConfigPayload) =>
    request<ProjectHookConfig>(`/projects/${projectId}/hooks/${hookId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectHook: (projectId: string, hookId: string) =>
    request<void>(`/projects/${projectId}/hooks/${hookId}`, { method: 'DELETE' }),
  listProjectHookRuns: (projectId: string, params: { phase?: string, buildRunId?: string, releaseId?: string } = {}) => {
    const search = new URLSearchParams()
    if (params.phase)
      search.set('phase', params.phase)
    if (params.buildRunId)
      search.set('buildRunId', params.buildRunId)
    if (params.releaseId)
      search.set('releaseId', params.releaseId)
    const query = search.toString()
    return request<HookRun[]>(`/projects/${projectId}/hook-runs${query ? `?${query}` : ''}`)
  },
  getProjectHookRunLogs: (projectId: string, runId: string) =>
    request<HookRunLog>(`/projects/${projectId}/hook-runs/${runId}/logs`),
  listBuildRuns: (projectId: string) =>
    request<BuildRun[]>(`/projects/${projectId}/build-runs`),
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
  listBuildJobs: (projectId: string, buildRunId?: string) =>
    request<BuildJob[]>(`/projects/${projectId}/build-jobs${buildRunId ? `?buildRunId=${encodeURIComponent(buildRunId)}` : ''}`),
  listBuildJobsPage: (projectId: string, params: PaginationParams, buildRunId?: string) => {
    const query = new URLSearchParams(paginationQuery(params))
    if (buildRunId)
      query.set('buildRunId', buildRunId)
    return request<PaginatedResponse<BuildJob>>(`/projects/${projectId}/build-jobs?${query.toString()}`)
  },
  getBuildJobLogs: (projectId: string, jobId: string) =>
    request<BuildLog>(`/projects/${projectId}/build-jobs/${jobId}/logs`),
}
