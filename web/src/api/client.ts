import i18next from '@/i18n'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

function optionalProjectQuery(projectId?: unknown) {
  if (typeof projectId !== 'string')
    return ''
  const normalized = projectId.trim()
  return normalized ? `?projectId=${encodeURIComponent(normalized)}` : ''
}

export interface Project {
  id: string
  slug: string
  name: string
  description: string
  namespaceStrategy: string
  createdAt: string
}

export interface ProjectPin extends Project {
  pinnedAt: string
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
  gitAccountId: string
  repositoryUrl: string
  imageReference: string
  dockerfilePath: string
  buildContext: string
  servicePort: number
  createdAt: string
}

export interface GitProvider {
  id: string
  type: 'github' | 'gitea' | 'gitlab'
  name: string
  baseUrl: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  authType: 'oauth' | 'github-app' | 'pat'
  clientId: string
  clientSecretSet: boolean
  enabled: boolean
  createdAt: string
}

export interface GitAccount {
  id: string
  userId: string
  providerId: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  externalUserId: string
  username: string
  avatarUrl: string
  scopes: string
  accessScope: 'personal' | 'provider'
  accessTokenSet: boolean
  refreshTokenSet: boolean
  status: 'connected' | 'expired' | 'revoked'
  createdAt: string
}

export interface RepositoryBinding {
  id: string
  projectId: string
  applicationId: string
  gitProviderId: string
  gitAccountId: string
  owner: string
  repo: string
  cloneUrl: string
  defaultBranch: string
  webhookStatus: 'pending' | 'created' | 'disabled' | 'failed'
  providerName?: string
  providerType?: GitProvider['type']
  accountUsername?: string
  accountOwnerEmail?: string
  accountOwnerName?: string
  applicationName?: string
  createdAt: string
}

export interface GitRepository {
  owner: string
  name: string
  fullName: string
  cloneUrl: string
  defaultBranch: string
  private: boolean
}

export interface GitBranch {
  name: string
  sha: string
}

export interface GitFileContent {
  path: string
  name: string
  ref: string
  sha: string
  content: string
  encoding: string
}

export interface GitContentItem {
  path: string
  name: string
  type: 'file' | 'dir' | string
  sha: string
}

export interface GitRepositoryBuildOptions {
  dockerfiles: string[]
  directories: string[]
  strategy: string
  truncated: boolean
  durationMs: number
}

export interface ArtifactRegistry {
  id: string
  name: string
  provider: 'harbor' | 'dockerhub' | 'gitea-registry'
  endpoint: string
  namespace: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  credentialSet: boolean
  isDefault: boolean
  capabilities: string[]
  createdBy: string
  createdAt: string
}

export interface RegistryCredential {
  id: string
  registryId: string
  name: string
  username: string
  scope: 'push-pull' | 'push' | 'pull'
  accessScope: 'personal' | 'registry'
  passwordSet: boolean
  tokenSet: boolean
  createdAt: string
}

export interface RegistryTestResult {
  success: boolean
  statusCode: number
  message: string
  endpoint: string
}

export interface ContainerImage {
  id: string
  projectId: string
  applicationId: string
  registryId: string
  repository: string
  tag: string
  digest: string
  imageRef: string
  sourceCommit: string
  buildRunId: string
  sourceType: 'build' | 'manual-image'
  scanStatus: 'unknown' | 'pending' | 'scanning' | 'passed' | 'failed'
  createdBy: string
  createdAt: string
}

export interface RegistryRepositoryItem {
  name: string
  description: string
  private: boolean
}

export interface RegistryTagItem {
  name: string
  digest: string
}

export interface BuildProvider {
  id: string
  name: string
  type: 'platform'
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  config: string
  enabled: boolean
  createdBy: string
  createdAt: string
}

export interface BuildVariableSet {
  id: string
  name: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  variables: string | Record<string, string>
  enabled: boolean
  createdBy: string
  createdAt: string
}

export interface BuildRun {
  id: string
  projectId: string
  applicationId: string
  buildProviderId: string
  buildVariableSetIds: string | string[]
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled'
  triggerType: 'manual' | 'webhook' | 'push' | 'tag' | 'api'
  sourceBranch: string
  sourceTag: string
  sourceCommit: string
  dockerfilePath: string
  buildContext: string
  buildDirectory: string
  targetRegistryId: string
  targetRepository: string
  targetTag: string
  imageRef: string
  imageDigest: string
  cacheConfig: string
  cpuCoreSeconds: number
  memoryMbSeconds: number
  creditCost: number
  createdBy: string
  createdAt: string
}

export interface BuildJob {
  id: string
  buildRunId: string
  projectId: string
  type: string
  status: string
  message: string
  logRef: string
  attempts: number
  createdAt: string
}

export interface BuildLog {
  id: string
  buildRunId: string
  buildJobId: string
  projectId: string
  content: string
  createdAt: string
  updatedAt: string
}

export interface RuntimeCluster {
  id: string
  name: string
  type: 'kubernetes' | 'k3s' | 'docker-compose'
  endpoint: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  kubeconfigSet: boolean
  isDefault: boolean
  status: string
  lastCheckedAt?: string
  createdBy: string
  createdAt: string
}

export interface Environment {
  id: string
  projectId: string
  name: string
  slug: string
  stage: 'dev' | 'test' | 'staging' | 'prod'
  clusterId: string
  namespace: string
  replicas: number
  cpuRequest: string
  memoryRequest: string
  envVars: string
  configRefs: string
  secretRefs: string
  createdBy: string
  createdAt: string
}

export interface Release {
  id: string
  projectId: string
  applicationId: string
  environmentId: string
  buildRunId: string
  imageRef: string
  type: 'deploy' | 'rollback'
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  revision: number
  rollbackFromId: string
  message: string
  createdBy: string
  createdAt: string
}

export interface GatewayRoute {
  id: string
  projectId: string
  applicationId: string
  environmentId: string
  host: string
  path: string
  servicePort: number
  tlsMode: 'http-only' | 'http-challenge' | 'manual-cert'
  certificateStatus: 'disabled' | 'pending' | 'issued' | 'failed' | 'expired'
  cnameName: string
  cnameTarget: string
  dnsStatus: 'pending' | 'verified' | 'failed'
  status: 'pending' | 'ready' | 'failed'
  isDefault: boolean
  createdBy: string
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

export interface PaginationParams {
  page: number
  pageSize: number
  search?: string
  sortBy?: string
  sortOrder?: 'asc' | 'desc'
}

export interface PaginatedResponse<T> {
  items: T[]
  page: number
  pageSize: number
  sortBy: string
  sortOrder: 'asc' | 'desc'
  total: number
  totalPages: number
}

export interface CurrentUser {
  id: string
  email: string
  name: string
  avatarUrl: string
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
  clientSecretSet: boolean
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
  type: 'string' | 'textarea'
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

export function apiBaseOrigin() {
  if (!API_BASE_URL.startsWith('http://') && !API_BASE_URL.startsWith('https://')) {
    return window.location.origin
  }
  try {
    return new URL(API_BASE_URL).origin
  }
  catch {
    return window.location.origin
  }
}

export function gitOAuthStartUrl(providerId: string, redirect = '/projects', frontendOrigin = window.location.origin) {
  const params = new URLSearchParams({ redirect, frontendOrigin })
  return `${API_BASE_URL}/git/providers/${providerId}/oauth/start?${params.toString()}`
}

function paginationQuery(params: PaginationParams) {
  const search = new URLSearchParams({
    page: String(params.page),
    pageSize: String(params.pageSize),
  })
  if (params.sortBy)
    search.set('sortBy', params.sortBy)
  if (params.sortOrder)
    search.set('sortOrder', params.sortOrder)
  if (params.search)
    search.set('search', params.search)
  return search.toString()
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
    if (typeof body.detail === 'string' && body.detail.trim())
      throw new Error(body.detail)
    const translationKey = typeof body.code === 'string' ? `errors.${body.code}` : ''
    const translated = translationKey && i18next.exists(translationKey) ? i18next.t(translationKey) : ''
    throw new Error(translated || body.error || response.statusText)
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
  resumeLogin: (payload: { userId: string }) =>
    request<{ user: CurrentUser }>('/auth/login/resume', { method: 'POST', body: JSON.stringify(payload) }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
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
  listConfigDefinitions: () => request<ConfigDefinition[]>('/configs/definitions'),
  getConfigs: () => request<Record<string, string>>('/configs'),
  updateConfigs: (values: Record<string, unknown>) =>
    request<Record<string, string>>('/configs', { method: 'PUT', body: JSON.stringify({ values }) }),
  listGitProviders: (projectId?: string) =>
    request<GitProvider[]>(`/git/providers${optionalProjectQuery(projectId)}`),
  createGitProvider: (payload: Omit<GitProvider, 'id' | 'createdAt' | 'clientSecretSet'> & { scope?: GitProvider['scope'], ownerRef?: string, clientSecret?: string }) =>
    request<GitProvider>('/git/providers', { method: 'POST', body: JSON.stringify(payload) }),
  updateGitProvider: (providerId: string, payload: Omit<GitProvider, 'id' | 'createdAt' | 'clientSecretSet'> & { scope?: GitProvider['scope'], ownerRef?: string, clientSecret?: string }) =>
    request<GitProvider>(`/git/providers/${providerId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGitProvider: (providerId: string) =>
    request<void>(`/git/providers/${providerId}`, { method: 'DELETE' }),
  listGitAccounts: (projectId?: string) =>
    request<GitAccount[]>(`/git/accounts${optionalProjectQuery(projectId)}`),
  createGitAccount: (payload: Omit<GitAccount, 'id' | 'userId' | 'scopes' | 'createdAt' | 'accessTokenSet' | 'refreshTokenSet'> & { scope?: GitAccount['scope'], ownerRef?: string, scopes: string[], accessToken?: string, refreshToken?: string }) =>
    request<GitAccount>('/git/accounts', { method: 'POST', body: JSON.stringify(payload) }),
  updateGitAccount: (accountId: string, payload: Omit<GitAccount, 'id' | 'userId' | 'scopes' | 'createdAt' | 'accessTokenSet' | 'refreshTokenSet'> & { scope?: GitAccount['scope'], ownerRef?: string, scopes: string[], accessToken?: string, refreshToken?: string }) =>
    request<GitAccount>(`/git/accounts/${accountId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGitAccount: (accountId: string) =>
    request<void>(`/git/accounts/${accountId}`, { method: 'DELETE' }),
  refreshGitAccount: (accountId: string) =>
    request<GitAccount>(`/git/accounts/${accountId}/refresh`, { method: 'POST' }),
  listGitRepositories: (accountId: string, params: { page: number, pageSize: number, search?: string }) => {
    const search = new URLSearchParams({ page: String(params.page), pageSize: String(params.pageSize) })
    if (params.search)
      search.set('search', params.search)
    return request<{ items: GitRepository[], page: number, pageSize: number }>(`/git/accounts/${accountId}/repositories?${search.toString()}`)
  },
  listGitBranches: (accountId: string, owner: string, repo: string, params?: { search?: string, limit?: number }) => {
    const search = new URLSearchParams()
    if (params?.search)
      search.set('search', params.search)
    if (params?.limit)
      search.set('limit', String(params.limit))
    const suffix = search.toString() ? `?${search.toString()}` : ''
    return request<{ items: GitBranch[], total: number, matchedTotal: number, limited: boolean }>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/branches${suffix}`)
  },
  readGitFile: (accountId: string, owner: string, repo: string, path: string, ref?: string) => {
    const search = new URLSearchParams({ path })
    if (ref)
      search.set('ref', ref)
    return request<GitFileContent>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/file?${search.toString()}`)
  },
  listGitContents: (accountId: string, owner: string, repo: string, path = '', ref?: string) => {
    const search = new URLSearchParams()
    if (path)
      search.set('path', path)
    if (ref)
      search.set('ref', ref)
    return request<GitContentItem[]>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/contents?${search.toString()}`)
  },
  getGitRepositoryBuildOptions: (accountId: string, owner: string, repo: string, ref?: string) => {
    const search = new URLSearchParams()
    if (ref)
      search.set('ref', ref)
    const suffix = search.toString() ? `?${search.toString()}` : ''
    return request<GitRepositoryBuildOptions>(`/git/accounts/${accountId}/repositories/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/build-options${suffix}`)
  },
  listProjects: () => request<Project[]>('/projects'),
  listProjectsPage: (params: PaginationParams) =>
    request<PaginatedResponse<Project>>(`/projects?${paginationQuery(params)}`),
  listProjectPins: () => request<ProjectPin[]>('/projects/pins'),
  createProject: (payload: Pick<Project, 'slug' | 'name' | 'description'>) =>
    request<Project>('/projects', { method: 'POST', body: JSON.stringify(payload) }),
  getProject: (projectId: string) => request<Project>(`/projects/${projectId}`),
  updateProject: (projectId: string, payload: Pick<Project, 'slug' | 'name' | 'description'>) =>
    request<Project>(`/projects/${projectId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteProject: (projectId: string) =>
    request<void>(`/projects/${projectId}`, { method: 'DELETE' }),
  pinProject: (projectId: string) =>
    request<ProjectPin>(`/projects/${projectId}/pin`, { method: 'PUT' }),
  unpinProject: (projectId: string) =>
    request<void>(`/projects/${projectId}/pin`, { method: 'DELETE' }),
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
  updateApplication: (projectId: string, applicationId: string, payload: Omit<Application, 'id' | 'projectId' | 'createdAt'>) =>
    request<Application>(`/projects/${projectId}/applications/${applicationId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteApplication: (projectId: string, applicationId: string) =>
    request<void>(`/projects/${projectId}/applications/${applicationId}`, { method: 'DELETE' }),
  listRepositoryBindings: (projectId: string) =>
    request<RepositoryBinding[]>(`/projects/${projectId}/repository-bindings`),
  createRepositoryBinding: (projectId: string, payload: Omit<RepositoryBinding, 'id' | 'projectId' | 'gitProviderId' | 'createdAt' | 'providerName' | 'providerType' | 'accountUsername' | 'accountOwnerEmail' | 'accountOwnerName' | 'applicationName'>) =>
    request<RepositoryBinding>(`/projects/${projectId}/repository-bindings`, { method: 'POST', body: JSON.stringify(payload) }),
  updateRepositoryBinding: (projectId: string, bindingId: string, payload: Omit<RepositoryBinding, 'id' | 'projectId' | 'gitProviderId' | 'createdAt' | 'providerName' | 'providerType' | 'accountUsername' | 'accountOwnerEmail' | 'accountOwnerName' | 'applicationName'>) =>
    request<RepositoryBinding>(`/projects/${projectId}/repository-bindings/${bindingId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRepositoryBinding: (projectId: string, bindingId: string) =>
    request<void>(`/projects/${projectId}/repository-bindings/${bindingId}`, { method: 'DELETE' }),
  createRepositoryWebhook: (projectId: string, bindingId: string) =>
    request<RepositoryBinding>(`/projects/${projectId}/repository-bindings/${bindingId}/webhook`, { method: 'POST' }),

  listRegistries: (projectId?: string) =>
    request<ArtifactRegistry[]>(`/registries${projectId ? `?projectId=${encodeURIComponent(projectId)}` : ''}`),
  createRegistry: (payload: Omit<ArtifactRegistry, 'id' | 'credentialSet' | 'createdBy' | 'createdAt'>) =>
    request<ArtifactRegistry>('/registries', { method: 'POST', body: JSON.stringify(payload) }),
  updateRegistry: (registryId: string, payload: Omit<ArtifactRegistry, 'id' | 'credentialSet' | 'createdBy' | 'createdAt'>) =>
    request<ArtifactRegistry>(`/registries/${registryId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRegistry: (registryId: string) =>
    request<void>(`/registries/${registryId}`, { method: 'DELETE' }),
  testRegistry: (registryId: string) =>
    request<RegistryTestResult>(`/registries/${registryId}/test`, { method: 'POST' }),
  getDefaultRegistry: (projectId: string) =>
    request<ArtifactRegistry>(`/projects/${projectId}/registries/default`),
  listRegistryCredentials: (registryId: string) =>
    request<RegistryCredential[]>(`/registries/${registryId}/credentials`),
  createRegistryCredential: (registryId: string, payload: { name: string, username: string, password?: string, token?: string, scope: RegistryCredential['scope'], accessScope: RegistryCredential['accessScope'] }) =>
    request<RegistryCredential>(`/registries/${registryId}/credentials`, { method: 'POST', body: JSON.stringify(payload) }),
  deleteRegistryCredential: (registryId: string, credentialId: string) =>
    request<void>(`/registries/${registryId}/credentials/${credentialId}`, { method: 'DELETE' }),
  searchRegistryRepositories: (registryId: string, params: { search?: string, page?: number, pageSize?: number }) => {
    const search = new URLSearchParams({ page: String(params.page ?? 1), pageSize: String(params.pageSize ?? 10) })
    if (params.search)
      search.set('search', params.search)
    return request<{ items: RegistryRepositoryItem[], page: number, pageSize: number, total: number, limited: boolean }>(`/registries/${registryId}/repositories/search?${search.toString()}`)
  },
  listRegistryRepositoryTags: (registryId: string, repository: string, limit = 20) => {
    const search = new URLSearchParams({ repository, limit: String(limit) })
    return request<{ items: RegistryTagItem[], total: number, limited: boolean }>(`/registries/${registryId}/repository-tags?${search.toString()}`)
  },
  listContainerImages: (projectId?: string) =>
    request<ContainerImage[]>(`/container-images${projectId ? `?projectId=${encodeURIComponent(projectId)}` : ''}`),
  createContainerImage: (payload: Omit<ContainerImage, 'id' | 'createdBy' | 'createdAt' | 'imageRef'>) =>
    request<ContainerImage>('/container-images', { method: 'POST', body: JSON.stringify(payload) }),

  listBuildProviders: (projectId?: string) =>
    request<BuildProvider[]>(`/build/providers${optionalProjectQuery(projectId)}`),
  createBuildProvider: (payload: Omit<BuildProvider, 'id' | 'createdBy' | 'createdAt'>) =>
    request<BuildProvider>('/build/providers', { method: 'POST', body: JSON.stringify(payload) }),
  updateBuildProvider: (providerId: string, payload: Omit<BuildProvider, 'id' | 'createdBy' | 'createdAt'>) =>
    request<BuildProvider>(`/build/providers/${providerId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteBuildProvider: (providerId: string) =>
    request<void>(`/build/providers/${providerId}`, { method: 'DELETE' }),
  listBuildVariableSets: (projectId?: string) =>
    request<BuildVariableSet[]>(`/build/variable-sets${optionalProjectQuery(projectId)}`),
  createBuildVariableSet: (payload: Omit<BuildVariableSet, 'id' | 'createdBy' | 'createdAt'>) =>
    request<BuildVariableSet>('/build/variable-sets', { method: 'POST', body: JSON.stringify(payload) }),
  updateBuildVariableSet: (setId: string, payload: Omit<BuildVariableSet, 'id' | 'createdBy' | 'createdAt'>) =>
    request<BuildVariableSet>(`/build/variable-sets/${setId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteBuildVariableSet: (setId: string) =>
    request<void>(`/build/variable-sets/${setId}`, { method: 'DELETE' }),
  listBuildRuns: (projectId: string) =>
    request<BuildRun[]>(`/projects/${projectId}/build-runs`),
  triggerBuildRun: (projectId: string, payload: Partial<BuildRun>) =>
    request<BuildRun>(`/projects/${projectId}/build-runs/trigger`, { method: 'POST', body: JSON.stringify(payload) }),
  listBuildJobs: (projectId: string, buildRunId?: string) =>
    request<BuildJob[]>(`/projects/${projectId}/build-jobs${buildRunId ? `?buildRunId=${encodeURIComponent(buildRunId)}` : ''}`),
  getBuildJobLogs: (projectId: string, jobId: string) =>
    request<BuildLog>(`/projects/${projectId}/build-jobs/${jobId}/logs`),

  listRuntimeClusters: (projectId?: string) => request<RuntimeCluster[]>(`/runtime/clusters${optionalProjectQuery(projectId)}`),
  createRuntimeCluster: (payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>('/runtime/clusters', { method: 'POST', body: JSON.stringify(payload) }),
  updateRuntimeCluster: (clusterId: string, payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRuntimeCluster: (clusterId: string) =>
    request<void>(`/runtime/clusters/${clusterId}`, { method: 'DELETE' }),
  testRuntimeCluster: (clusterId: string) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}/test`, { method: 'POST' }),
  listEnvironments: (projectId: string) =>
    request<Environment[]>(`/projects/${projectId}/environments`),
  createEnvironment: (projectId: string, payload: Omit<Environment, 'id' | 'projectId' | 'createdBy' | 'createdAt'>) =>
    request<Environment>(`/projects/${projectId}/environments`, { method: 'POST', body: JSON.stringify(payload) }),
  updateEnvironment: (projectId: string, environmentId: string, payload: Omit<Environment, 'id' | 'projectId' | 'createdBy' | 'createdAt'>) =>
    request<Environment>(`/projects/${projectId}/environments/${environmentId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteEnvironment: (projectId: string, environmentId: string) =>
    request<void>(`/projects/${projectId}/environments/${environmentId}`, { method: 'DELETE' }),
  listReleases: (projectId: string) =>
    request<Release[]>(`/projects/${projectId}/releases`),
  createRelease: (projectId: string, payload: Omit<Release, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'rollbackFromId'>) =>
    request<Release>(`/projects/${projectId}/releases`, { method: 'POST', body: JSON.stringify(payload) }),
  rollbackRelease: (projectId: string, releaseId: string) =>
    request<Release>(`/projects/${projectId}/releases/${releaseId}/rollback`, { method: 'POST' }),

  listGatewayRoutes: (projectId: string) =>
    request<GatewayRoute[]>(`/projects/${projectId}/gateway-routes`),
  createGatewayRoute: (projectId: string, payload: Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget'> & { applicationSlug?: string, stage?: string }) =>
    request<GatewayRoute>(`/projects/${projectId}/gateway-routes`, { method: 'POST', body: JSON.stringify(payload) }),
  updateGatewayRoute: (projectId: string, routeId: string, payload: Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget'> & { applicationSlug?: string, stage?: string }) =>
    request<GatewayRoute>(`/projects/${projectId}/gateway-routes/${routeId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGatewayRoute: (projectId: string, routeId: string) =>
    request<void>(`/projects/${projectId}/gateway-routes/${routeId}`, { method: 'DELETE' }),
  checkGatewayDomain: (projectId: string, host: string) =>
    request<{ available: boolean, host: string }>(`/projects/${projectId}/gateway-routes/check-domain?host=${encodeURIComponent(host)}`),

  listAccessTokens: (params: PaginationParams) =>
    request<PaginatedResponse<AccessToken>>(`/access-tokens?${paginationQuery(params)}`),
  createAccessToken: (payload: { name: string, scope: string, expiresInDays: number }) =>
    request<{ token: AccessToken, accessToken: string }>('/access-tokens', { method: 'POST', body: JSON.stringify(payload) }),
  revokeAccessToken: (tokenId: string) =>
    request<AccessToken>(`/access-tokens/${tokenId}`, { method: 'DELETE' }),
}
