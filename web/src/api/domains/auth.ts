import type { AuthAdmissionPolicy, AuthProvider, BootstrapStatus, ConfigDefinition, CurrentUser, ExternalIdentity, MFAEnrollment, MFAEnrollmentRequest, MFARecoveryCodes, MFAStatus, MFAVerifyPayload, MFAVerifyResponse, OIDCCallbackConfig, PaginatedResponse, PaginationParams, User } from '../types'
import { paginationQuery, request } from '../core'

export const authApi = {
  getPublicConfigs: (keys: string[]) =>
    request<Record<string, string>>('/public/configs', { method: 'POST', body: JSON.stringify({ keys }) }),
  getBootstrapStatus: () => request<BootstrapStatus>('/auth/bootstrap'),
  initializeAdmin: (payload: { email: string, name: string, password: string, language: 'zh-CN' | 'en-US', rememberMe: boolean, bootstrapToken: string }) =>
    request<{ user: CurrentUser }>('/auth/bootstrap/admin', { method: 'POST', body: JSON.stringify(payload) }),
  login: (payload: { email: string, password: string, rememberMe: boolean }) =>
    request<{ user: CurrentUser }>('/auth/login', { method: 'POST', body: JSON.stringify(payload) }),
  resumeLogin: (payload: { userId: string }) =>
    request<{ user: CurrentUser }>('/auth/login/resume', { method: 'POST', body: JSON.stringify(payload) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  getMFAStatus: () => request<MFAStatus>('/auth/mfa/status'),
  enrollMFA: (payload: MFAEnrollmentRequest) => request<MFAEnrollment>('/auth/mfa/totp/enroll', { method: 'POST', body: JSON.stringify(payload) }),
  confirmMFAEnrollment: (payload: { code: string }) =>
    request<MFARecoveryCodes>('/auth/mfa/totp/confirm', { method: 'POST', body: JSON.stringify(payload) }),
  verifyMFA: (payload: MFAVerifyPayload) =>
    request<MFAVerifyResponse>('/auth/mfa/verify', { method: 'POST', body: JSON.stringify(payload) }),
  regenerateMFARecoveryCodes: () =>
    request<MFARecoveryCodes>('/auth/mfa/recovery-codes', { method: 'POST' }),
  disableMFA: () => request<void>('/auth/mfa', { method: 'DELETE' }),
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
  updateCurrentUser: (payload: { name?: string, avatarUrl?: string, language?: 'zh-CN' | 'en-US' }) =>
    request<CurrentUser>('/users/me', { method: 'PUT', body: JSON.stringify(payload) }),
  listMyExternalIdentities: () => request<ExternalIdentity[]>('/users/me/external-identities'),
  unbindMyExternalIdentity: (identityId: string) =>
    request<void>(`/users/me/external-identities/${identityId}`, { method: 'DELETE' }),
  listUsers: (params: PaginationParams) =>
    request<PaginatedResponse<User>>(`/users?${paginationQuery(params)}`),
  createUser: (payload: { email: string, name: string, password: string, role: 'platform_admin' | 'user', language: 'zh-CN' | 'en-US', disabled: boolean }) =>
    request<User>('/users', { method: 'POST', body: JSON.stringify(payload) }),
  updateUser: (userId: string, payload: { email: string, name: string, password?: string, role: 'platform_admin' | 'user', language: 'zh-CN' | 'en-US', disabled: boolean }) =>
    request<User>(`/users/${userId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  resetUserMFA: (userId: string) => request<void>(`/users/${userId}/mfa`, { method: 'DELETE' }),
  listConfigDefinitions: () => request<ConfigDefinition[]>('/configs/definitions'),
  getConfigs: () => request<Record<string, string>>('/configs'),
  updateConfigs: (values: Record<string, unknown>) =>
    request<Record<string, string>>('/configs', { method: 'PUT', body: JSON.stringify({ values }) }),
}
