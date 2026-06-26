import type { AppTemplate, AppTemplateInstallPayload, AppTemplateInstallResponse, BillingDeploymentSpend, BillingLedgerEntry, BillingListParams, BillingRateRule, BillingRateRulePayload, BillingSummary, BillingUsageRecord, BillingUsageSettlementResult, BillingWalletTransactionPayload, GatewayTrafficUsagePayload, PaginatedResponse, PaginationParams, Project, ProjectListParams, ProjectMember, ProjectPin } from '../types'
import { billingQuery, billingSummaryQuery, paginationQuery, request } from '../core'

export const projectsApi = {
  listProjects: () => request<Project[]>('/projects'),
  listProjectsPage: (params: ProjectListParams) =>
    request<PaginatedResponse<Project>>(`/projects?${paginationQuery(params)}`),
  listAppTemplates: () => request<AppTemplate[]>('/app-templates'),
  installAppTemplate: (projectId: string, templateId: string, payload: AppTemplateInstallPayload) =>
    request<AppTemplateInstallResponse>(`/projects/${projectId}/app-templates/${encodeURIComponent(templateId)}/install`, { method: 'POST', body: JSON.stringify(payload) }),
  getBillingSummary: (projectIds?: string[]) =>
    request<BillingSummary>(`/billing/summary${billingSummaryQuery(projectIds)}`),
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
  createProject: (payload: Pick<Project, 'slug' | 'name' | 'description' | 'maxConcurrentBuilds'>) =>
    request<Project>('/projects', { method: 'POST', body: JSON.stringify(payload) }),
  getProject: (projectId: string) => request<Project>(`/projects/${projectId}`),
  updateProject: (projectId: string, payload: Pick<Project, 'slug' | 'name' | 'description' | 'maxConcurrentBuilds'>) =>
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
  createProjectMember: (projectId: string, payload: { email: string, role: ProjectMember['role'] }) =>
    request<ProjectMember>(`/projects/${projectId}/members`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectMember: (projectId: string, memberId: string, payload: { role: ProjectMember['role'] }) =>
    request<ProjectMember>(`/projects/${projectId}/members/${memberId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectMember: (projectId: string, memberId: string) =>
    request<void>(`/projects/${projectId}/members/${memberId}`, { method: 'DELETE' }),
}
