import i18next from '../i18n'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

export interface Project {
  id: string
  slug: string
  name: string
  description: string
  namespaceStrategy: string
  createdAt: string
}

export interface ProjectMember {
  id: string
  projectId: string
  userId: string
  role: 'owner' | 'admin' | 'developer' | 'viewer'
  email: string
  name: string
}

export interface Application {
  id: string
  projectId: string
  slug: string
  name: string
  sourceType: 'repository' | 'image'
  repositoryUrl: string
  imageReference: string
  dockerfilePath: string
  buildContext: string
  servicePort: number
  createdAt: string
}

export interface AccessToken {
  id: string
  name: string
  scope: string
  expiresAt?: string
  revokedAt?: string
  createdAt: string
}

export interface CurrentUser {
  id: string
  email: string
  name: string
  authType: 'local' | 'oidc'
  role: string
  language: 'zh-CN' | 'en-US'
  permissions: string[]
}

export interface User {
  id: string
  email: string
  name: string
  authType: 'local' | 'oidc'
  role: 'platform_admin' | 'user'
  language: 'zh-CN' | 'en-US'
  disabled: boolean
  createdAt: string
}

export interface AuthProvider {
  id: string
  type: 'oidc'
  name: string
  enabled: boolean
  issuerUrl: string
  clientId: string
  clientSecretRef: string
  scopes: string
  groupClaim: string
  emailClaim: string
  usernameClaim: string
  isDefault: boolean
  createdAt: string
}

export interface ExternalIdentity {
  id: string
  userId: string
  providerId: string
  providerName: string
  subject: string
  email: string
  emailVerified: boolean
  username: string
  lastLoginAt?: string
  createdAt: string
}

export interface AuthAdmissionPolicy {
  id: string
  allowLocalLogin: boolean
  allowOidcLogin: boolean
  allowedEmailDomains: string[]
  allowedOidcGroups: string[]
  invitedEmails: string[]
  defaultRole: 'platform_admin' | 'user'
}

export interface ConfigDefinition {
  key: string
  label: string
  description: string
  type: 'string'
  public: boolean
  default: string
}

export interface BootstrapStatus {
  mode: 'development' | 'production'
  initialized: boolean
  devLoginEnabled: boolean
  devLoginHint?: {
    email: string
    password: string
  }
}

export function oidcStartUrl(providerId: string, mode: 'login' | 'bind', redirect = '/projects') {
  const params = new URLSearchParams({ mode, redirect })
  return `${API_BASE_URL}/auth/oidc/${providerId}/start?${params.toString()}`
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    credentials: 'include',
    headers: {
      'Accept-Language': i18next.language,
      'Content-Type': 'application/json',
      ...options?.headers,
    },
    ...options,
  })

  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(body.error ?? response.statusText)
  }

  if (response.status === 204)
    return undefined as T

  return response.json()
}

export const api = {
  getPublicConfigs: (keys: string[]) =>
    request<Record<string, string>>('/public/configs', { method: 'POST', body: JSON.stringify({ keys }) }),
  getBootstrapStatus: () => request<BootstrapStatus>('/auth/bootstrap'),
  initializeAdmin: (payload: { email: string, name: string, password: string, language: 'zh-CN' | 'en-US' }) =>
    request<{ user: CurrentUser }>('/auth/bootstrap/admin', { method: 'POST', body: JSON.stringify(payload) }),
  login: (payload: { email: string, password: string }) =>
    request<{ user: CurrentUser }>('/auth/login', { method: 'POST', body: JSON.stringify(payload) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
  listAuthProviders: (includeDisabled = false) =>
    request<AuthProvider[]>(`/auth/providers${includeDisabled ? '?includeDisabled=true' : ''}`),
  createAuthProvider: (payload: Omit<AuthProvider, 'id' | 'type' | 'createdAt'> & { type?: 'oidc' }) =>
    request<AuthProvider>('/auth/providers', { method: 'POST', body: JSON.stringify(payload) }),
  updateAuthProvider: (providerId: string, payload: Omit<AuthProvider, 'id' | 'type' | 'createdAt'> & { type?: 'oidc' }) =>
    request<AuthProvider>(`/auth/providers/${providerId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  getAuthAdmissionPolicy: () => request<AuthAdmissionPolicy>('/auth/admission-policy'),
  updateAuthAdmissionPolicy: (payload: Omit<AuthAdmissionPolicy, 'id'>) =>
    request<AuthAdmissionPolicy>('/auth/admission-policy', { method: 'PUT', body: JSON.stringify(payload) }),
  getCurrentUser: () => request<CurrentUser>('/users/me'),
  updateCurrentUser: (payload: { language: 'zh-CN' | 'en-US' }) =>
    request<CurrentUser>('/users/me', { method: 'PUT', body: JSON.stringify(payload) }),
  listMyExternalIdentities: () => request<ExternalIdentity[]>('/users/me/external-identities'),
  unbindMyExternalIdentity: (identityId: string) =>
    request<void>(`/users/me/external-identities/${identityId}`, { method: 'DELETE' }),
  listUsers: () => request<User[]>('/users'),
  createUser: (payload: { email: string, name: string, password: string, role: 'platform_admin' | 'user', language: 'zh-CN' | 'en-US', disabled: boolean }) =>
    request<User>('/users', { method: 'POST', body: JSON.stringify(payload) }),
  updateUser: (userId: string, payload: { email: string, name: string, password?: string, role: 'platform_admin' | 'user', language: 'zh-CN' | 'en-US', disabled: boolean }) =>
    request<User>(`/users/${userId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  listConfigDefinitions: () => request<ConfigDefinition[]>('/configs/definitions'),
  updateConfigs: (values: Record<string, string>) =>
    request<Record<string, string>>('/configs', { method: 'PUT', body: JSON.stringify({ values }) }),
  listProjects: () => request<Project[]>('/projects'),
  createProject: (payload: Pick<Project, 'slug' | 'name' | 'description'>) =>
    request<Project>('/projects', { method: 'POST', body: JSON.stringify(payload) }),
  updateProject: (projectId: string, payload: Pick<Project, 'slug' | 'name' | 'description'>) =>
    request<Project>(`/projects/${projectId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProject: (projectId: string) =>
    request<void>(`/projects/${projectId}`, { method: 'DELETE' }),
  listProjectMembers: (projectId: string) => request<ProjectMember[]>(`/projects/${projectId}/members`),
  createProjectMember: (projectId: string, payload: { email: string, role: ProjectMember['role'] }) =>
    request<ProjectMember>(`/projects/${projectId}/members`, { method: 'POST', body: JSON.stringify(payload) }),
  updateProjectMember: (projectId: string, memberId: string, payload: { role: ProjectMember['role'] }) =>
    request<ProjectMember>(`/projects/${projectId}/members/${memberId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProjectMember: (projectId: string, memberId: string) =>
    request<void>(`/projects/${projectId}/members/${memberId}`, { method: 'DELETE' }),

  listApplications: (projectId: string) =>
    request<Application[]>(`/projects/${projectId}/applications`),
  getApplication: (projectId: string, applicationId: string) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`),
  createApplication: (projectId: string, payload: Omit<Application, 'id' | 'projectId' | 'createdAt'>) =>
    request<Application>(`/projects/${projectId}/applications`, { method: 'POST', body: JSON.stringify(payload) }),
  parseApplicationConfig: (projectId: string, content: string) =>
    request<Omit<Application, 'id' | 'projectId' | 'createdAt'>>(`/projects/${projectId}/applications/parse-config`, { method: 'POST', body: JSON.stringify({ content }) }),
  updateApplication: (projectId: string, applicationId: string, payload: Omit<Application, 'id' | 'projectId' | 'createdAt'>) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteApplication: (projectId: string, applicationId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}`, { method: 'DELETE' }),

  listAccessTokens: () => request<AccessToken[]>('/access-tokens'),
  createAccessToken: (payload: { name: string, scope: string, expiresInDays: number }) =>
    request<{ token: AccessToken, accessToken: string }>('/access-tokens', { method: 'POST', body: JSON.stringify(payload) }),
  revokeAccessToken: (tokenId: string) =>
    request<AccessToken>(`/access-tokens/${tokenId}`, { method: 'DELETE' }),
}
