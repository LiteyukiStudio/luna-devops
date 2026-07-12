import type { AppTemplate, AppTemplateInstallPayload, AppTemplateInstallResponse, BillingDeploymentSpend, BillingLedgerEntry, BillingListParams, BillingPeriodParams, BillingRateRule, BillingRateRulePayload, BillingSummary, BillingUsageRecord, BillingUsageSettlementResult, BillingWalletTransactionPayload, GatewayTrafficStatus, GatewayTrafficUsagePayload, PaginatedResponse, PaginationParams, Project, ProjectListParams, ProjectMember, ProjectMemberCandidate, ProjectPin, SystemComponentInstallPayload, SystemComponentInstallResponse, SystemComponentStatusResponse } from '../types'
import { billingQuery, billingSummaryQuery, paginationQuery, request } from '../core'

export const projectsApi = {
  listProjects: () => request<Project[]>('/projects'),
  listProjectsPage: (params: ProjectListParams) =>
    request<PaginatedResponse<Project>>(`/projects?${paginationQuery(params)}`),
  listAppTemplates: () => request<AppTemplate[]>('/app-templates'),
  installAppTemplate: (projectId: string, templateId: string, payload: AppTemplateInstallPayload) =>
    request<AppTemplateInstallResponse>(`/projects/${projectId}/app-templates/${encodeURIComponent(templateId)}/install`, { method: 'POST', body: JSON.stringify(payload) }),
  listSystemComponents: (params?: { componentId?: string, clusterId?: string }) => {
    const search = new URLSearchParams()
    if (params?.componentId)
      search.set('componentId', params.componentId)
    if (params?.clusterId)
      search.set('clusterId', params.clusterId)
    const suffix = search.toString() ? `?${search.toString()}` : ''
    return request<SystemComponentStatusResponse>(`/system-components${suffix}`)
  },
  installSystemAppTemplate: (templateId: string, payload: SystemComponentInstallPayload) =>
    request<SystemComponentInstallResponse>(`/app-templates/${encodeURIComponent(templateId)}/system-install`, { method: 'POST', body: JSON.stringify(payload) }),
  getBillingSummary: (projectIds?: string[], period?: BillingPeriodParams) =>
    request<BillingSummary>(`/billing/summary${billingSummaryQuery(projectIds, period)}`),
  getGatewayTrafficStatus: () => request<GatewayTrafficStatus>('/billing/gateway-traffic-status'),
  listBillingDeploymentSpend: (params: BillingListParams) =>
    request<PaginatedResponse<BillingDeploymentSpend>>(`/billing/deployment-spend?${billingQuery(params)}`),
  listBillingLedgerEntries: (params: BillingListParams) =>
    request<PaginatedResponse<BillingLedgerEntry>>(`/billing/ledger?${billingQuery(params)}`),
  listBillingUsageRecords: (params: BillingListParams) =>
    request<PaginatedResponse<BillingUsageRecord>>(`/billing/usage-records?${billingQuery(params)}`),
  listBillingRateRules: () => request<BillingRateRule[]>('/billing/rate-rules'),
  updateBillingRateRules: (rules: BillingRateRulePayload[]) =>
    request<BillingRateRule[]>('/billing/rate-rules', { method: 'PUT', body: JSON.stringify({ rules }) }),
  createBillingWalletTransaction: (payload: BillingWalletTransactionPayload) =>
    request<BillingLedgerEntry>('/billing/wallet-transactions', { method: 'POST', body: JSON.stringify(payload) }),
  createGatewayTrafficUsage: (payload: GatewayTrafficUsagePayload) =>
    request<BillingUsageSettlementResult>('/billing/gateway-traffic', { method: 'POST', body: JSON.stringify(payload) }),
  listProjectPins: () => request<ProjectPin[]>('/projects/pins'),
  updateProjectOrder: (projectIds: string[]) =>
    request<{ projectIds: string[] }>('/projects/order', { method: 'PUT', body: JSON.stringify({ projectIds }) }),
  createProject: (payload: Pick<Project, 'slug' | 'name' | 'description' | 'maxConcurrentBuilds' | 'webConsoleEnabled'>) =>
    request<Project>('/projects', { method: 'POST', body: JSON.stringify(payload) }),
  getProject: (projectId: string) => request<Project>(`/projects/${projectId}`),
  updateProject: (projectId: string, payload: Pick<Project, 'slug' | 'name' | 'description' | 'maxConcurrentBuilds' | 'webConsoleEnabled'>) =>
    request<Project>(`/projects/${projectId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProject: (projectId: string) =>
    request<void>(`/projects/${projectId}`, { method: 'DELETE' }),
  pinProject: (projectId: string) =>
    request<ProjectPin>(`/projects/${projectId}/pin`, { method: 'PUT' }),
  unpinProject: (projectId: string) =>
    request<void>(`/projects/${projectId}/pin`, { method: 'DELETE' }),
  listProjectMembers: (projectId: string) => request<ProjectMember[]>(`/projects/${projectId}/members`),
  listProjectMembersPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<ProjectMember>>(`/projects/${projectId}/members?${paginationQuery(params)}`),
  searchProjectMemberCandidates: (projectId: string, params: { search: string, limit?: number }) => {
    const search = new URLSearchParams({ search: params.search })
    if (params.limit)
      search.set('limit', String(params.limit))
    return request<ProjectMemberCandidate[]>(`/projects/${projectId}/member-candidates?${search.toString()}`)
  },
  createProjectMember: (projectId: string, payload: { userId: string, role: ProjectMember['role'] }) =>
    request<ProjectMember>(`/projects/${projectId}/members`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectMember: (projectId: string, memberId: string, payload: { role: ProjectMember['role'] }) =>
    request<ProjectMember>(`/projects/${projectId}/members/${memberId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectMember: (projectId: string, memberId: string) =>
    request<void>(`/projects/${projectId}/members/${memberId}`, { method: 'DELETE' }),
}
