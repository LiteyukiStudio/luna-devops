import type { AgentObservabilityOverview, AgentObservabilityRange, AgentObservabilitySource, AgentObservabilityTestResult, AgentObservabilityToolCall, AgentObservabilityToolSummary, AgentObservabilityTraceDetail, AgentObservabilityTurn, AuthAdmissionPolicy, AuthProvider, AuthRegistrationSettings, AuthRegistrationStatus, BootstrapStatus, ConfigDefinition, CurrentUser, DataRetentionCatalogResponse, DataRetentionPayload, DataRetentionResultResponse, ExternalIdentity, OIDCCallbackConfig, PaginatedResponse, PaginationParams, User } from '../types'
import type { PlatformRoleValue } from '@/lib/roles'
import { paginationQuery, request } from '../core'

export const authApi = {
  getPublicConfigs: (keys: string[]) =>
    request<Record<string, string>>('/public/configs', { method: 'POST', body: JSON.stringify({ keys }) }),
  getBootstrapStatus: () => request<BootstrapStatus>('/auth/bootstrap'),
  initializeAdmin: (payload: { email: string, name: string, password: string, language: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR', rememberMe: boolean, bootstrapToken: string }) =>
    request<{ user: CurrentUser }>('/auth/bootstrap/admin', { method: 'POST', body: JSON.stringify(payload) }),
  login: (payload: { email: string, password: string, rememberMe: boolean }) =>
    request<{ user: CurrentUser }>('/auth/login', { method: 'POST', body: JSON.stringify(payload) }),
  resumeLogin: (payload: { userId: string }) =>
    request<{ user: CurrentUser }>('/auth/login/resume', { method: 'POST', body: JSON.stringify(payload) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  getAuthRegistrationStatus: () => request<AuthRegistrationStatus>('/auth/registration'),
  getAuthRegistrationSettings: () => request<AuthRegistrationSettings>('/auth/registration/settings'),
  updateAuthRegistrationSettings: (payload: AuthRegistrationSettings) =>
    request<AuthRegistrationSettings>('/auth/registration/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  requestEmailRegistrationCode: (payload: { email: string, language: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR' }) =>
    request<{ challengeId: string, expiresAt: string }>('/auth/registration/email/code', { method: 'POST', body: JSON.stringify(payload) }),
  completeEmailRegistration: (payload: { challengeId: string, code: string, email: string, name: string, password: string, language: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR', rememberMe: boolean }) =>
    request<{ user: CurrentUser }>('/auth/registration/email', { method: 'POST', body: JSON.stringify(payload) }),
  getOIDCCallbackConfig: () => request<OIDCCallbackConfig>('/auth/oidc/callback-url'),
  listAuthProviders: (includeDisabled = false) =>
    request<AuthProvider[]>(`/auth/providers${includeDisabled ? '?includeDisabled=true' : ''}`),
  createAuthProvider: (payload: Omit<AuthProvider, 'id' | 'type' | 'createdAt' | 'clientSecretSet'> & { type?: 'oidc', clientSecret?: string }) =>
    request<AuthProvider>('/auth/providers', { method: 'POST', body: JSON.stringify(payload) }),
  updateAuthProvider: (providerId: string, payload: Omit<AuthProvider, 'id' | 'type' | 'createdAt' | 'clientSecretSet'> & { type?: 'oidc', clientSecret?: string }) =>
    request<AuthProvider>(`/auth/providers/${providerId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  getAuthAdmissionPolicy: () => request<AuthAdmissionPolicy>('/auth/admission-policy'),
  updateAuthAdmissionPolicy: (payload: Omit<AuthAdmissionPolicy, 'id'>) =>
    request<AuthAdmissionPolicy>('/auth/admission-policy', { method: 'PUT', body: JSON.stringify(payload) }),
  getCurrentUser: () => request<CurrentUser>('/users/me'),
  updateCurrentUser: (payload: { name?: string, avatarUrl?: string, language?: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR', brandColorPreset?: CurrentUser['brandColorPreset'], interfaceStyle?: CurrentUser['interfaceStyle'] }) =>
    request<CurrentUser>('/users/me', { method: 'PUT', body: JSON.stringify(payload) }),
  updateMyPassword: (payload: { currentPassword?: string, newPassword: string }) =>
    request<void>('/users/me/password', { method: 'PUT', body: JSON.stringify(payload) }),
  listMyExternalIdentities: () => request<ExternalIdentity[]>('/users/me/external-identities'),
  unbindMyExternalIdentity: (identityId: string) =>
    request<void>(`/users/me/external-identities/${identityId}`, { method: 'DELETE' }),
  listUsers: (params: PaginationParams) =>
    request<PaginatedResponse<User>>(`/users?${paginationQuery(params)}`),
  createUser: (payload: { email: string, name: string, password: string, role: PlatformRoleValue, language: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR', disabled: boolean }) =>
    request<User>('/users', { method: 'POST', body: JSON.stringify(payload) }),
  updateUser: (userId: string, payload: { email: string, name: string, password?: string, role: PlatformRoleValue, language: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR', disabled: boolean }) =>
    request<User>(`/users/${userId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  listConfigDefinitions: () => request<ConfigDefinition[]>('/configs/definitions'),
  getConfigs: () => request<Record<string, string>>('/configs'),
  updateConfigs: (values: Record<string, unknown>) =>
    request<Record<string, string>>('/configs', { method: 'PUT', body: JSON.stringify({ values }) }),
  testAgentObservabilitySource: (payload: { source: AgentObservabilitySource, url: string, token?: string, tenantId?: string }) =>
    request<AgentObservabilityTestResult>('/configs/ai/observability/test', { method: 'POST', body: JSON.stringify(payload) }),
  getAgentObservabilityOverview: (range: AgentObservabilityRange) =>
    request<AgentObservabilityOverview>(`/ai/observability/overview?range=${range}`),
  listAgentObservabilityTurns: (params: { range: AgentObservabilityRange, page: number, pageSize: number, search?: string }) =>
    request<PaginatedResponse<AgentObservabilityTurn>>(`/ai/observability/turns?${paginationQuery({ ...params, sortBy: 'createdAt', sortOrder: 'desc' })}&range=${params.range}`),
  listAgentObservabilityTools: (params: { range: AgentObservabilityRange, page: number, pageSize: number, search?: string }) =>
    request<PaginatedResponse<AgentObservabilityToolSummary>>(`/ai/observability/tools?${paginationQuery({ ...params, sortBy: 'lastCalledAt', sortOrder: 'desc' })}&range=${params.range}`),
  listAgentObservabilityToolCalls: (operationId: string, params: { range: AgentObservabilityRange, page: number, pageSize: number }) =>
    request<PaginatedResponse<AgentObservabilityToolCall>>(`/ai/observability/tools/${encodeURIComponent(operationId)}/calls?${paginationQuery({ ...params, sortBy: 'createdAt', sortOrder: 'desc' })}&range=${params.range}`),
  getAgentObservabilityTrace: (traceId: string) =>
    request<AgentObservabilityTraceDetail>(`/ai/observability/traces/${encodeURIComponent(traceId)}`),
  getDataRetentionCatalog: () => request<DataRetentionCatalogResponse>('/data-retention/catalog'),
  previewDataRetention: (payload: DataRetentionPayload) =>
    request<DataRetentionResultResponse>('/data-retention/preview', { method: 'POST', body: JSON.stringify(payload) }),
  cleanupDataRetention: (payload: DataRetentionPayload) =>
    request<DataRetentionResultResponse>('/data-retention/cleanup', { method: 'POST', body: JSON.stringify(payload) }),
}
