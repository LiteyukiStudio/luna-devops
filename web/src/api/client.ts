import type {
  AccessToken,
  Application,
  ApplicationPayload,
  ArtifactRegistry,
  ArtifactRegistryPayload,
  AuthAdmissionPolicy,
  AuthProvider,
  BootstrapStatus,
  BuildJob,
  BuildLog,
  BuildRun,
  BuildRunListParams,
  BuildVariableSet,
  BuildVariableSetPayload,
  ClusterResource,
  ClusterResourceEvent,
  ClusterResourceYAML,
  ConfigDefinition,
  ContainerImage,
  CurrentUser,
  DeploymentTarget,
  DeploymentTargetPayload,
  Environment,
  ExternalIdentity,
  GatewayDomainCheckResult,
  GatewayRoute,
  GitAccount,
  GitBranch,
  GitContentItem,
  GitFileContent,
  GitProvider,
  GitRepository,
  GitRepositoryBuildOptions,
  HookRun,
  HookRunLog,
  OIDCCallbackConfig,
  PaginatedResponse,
  PaginationParams,
  Project,
  ProjectHookConfig,
  ProjectHookConfigPayload,
  ProjectListParams,
  ProjectMember,
  ProjectPin,
  ProjectRuntimeConfigSet,
  ProjectRuntimeConfigSetPayload,
  RegistryCredential,
  RegistryRepositoryItem,
  RegistryTagItem,
  RegistryTestResult,
  Release,
  ReleaseLog,
  ReleaseRuntimeExecResult,
  ReleaseRuntimeLog,
  RepositoryBinding,
  RepositoryBindingPayload,
  RuntimeCluster,
  User,
} from './types'

import i18next from '@/i18n'

export type {
  AccessToken,
  Application,
  ApplicationPayload,
  ArtifactRegistry,
  ArtifactRegistryPayload,
  AuthAdmissionPolicy,
  AuthProvider,
  BootstrapStatus,
  BuildJob,
  BuildLog,
  BuildRun,
  BuildRunListParams,
  BuildVariableSet,
  BuildVariableSetPayload,
  ClusterResource,
  ClusterResourceEvent,
  ClusterResourceYAML,
  ConfigDefinition,
  ContainerImage,
  CurrentUser,
  DeploymentTarget,
  DeploymentTargetHookBinding,
  DeploymentTargetPayload,
  Environment,
  ExternalIdentity,
  GatewayDomainCheckResult,
  GatewayRoute,
  GitAccount,
  GitBranch,
  GitContentItem,
  GitFileContent,
  GitProvider,
  GitRepository,
  GitRepositoryBuildOptions,
  HookPhase,
  HookRun,
  HookRunLog,
  OIDCCallbackConfig,
  PaginatedResponse,
  PaginationParams,
  Project,
  ProjectHookConfig,
  ProjectHookConfigPayload,
  ProjectListParams,
  ProjectListScope,
  ProjectMember,
  ProjectPin,
  ProjectRuntimeConfigSet,
  ProjectRuntimeConfigSetPayload,
  RegistryCredential,
  RegistryRepositoryItem,
  RegistryTagItem,
  RegistryTestResult,
  Release,
  ReleaseLog,
  ReleaseRuntimeExecResult,
  ReleaseRuntimeLog,
  RepositoryBinding,
  RepositoryBindingPayload,
  RuntimeCluster,
  User,
} from './types'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

interface ApiErrorBody {
  code?: unknown
  detail?: unknown
  error?: unknown
}

export class ApiError extends Error {
  code: string
  detail?: string
  path: string
  status: number

  constructor(message: string, options: { code?: string, detail?: string, path: string, status: number }) {
    super(message)
    this.name = 'ApiError'
    this.code = options.code || 'request.failed'
    this.detail = options.detail
    this.path = options.path
    this.status = options.status
  }
}

function optionalProjectQuery(projectId?: unknown) {
  if (typeof projectId !== 'string')
    return ''
  const normalized = projectId.trim()
  return normalized ? `?projectId=${encodeURIComponent(normalized)}` : ''
}

function paginationQuery(params: PaginationParams & { scope?: string }) {
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
  if (params.scope)
    search.set('scope', params.scope)
  return search.toString()
}

function paginationWithProjectQuery(params: PaginationParams & { projectId?: string }) {
  const search = new URLSearchParams(paginationQuery(params))
  if (params.projectId)
    search.set('projectId', params.projectId)
  return search.toString()
}

function buildRunListQuery(params: BuildRunListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  if (params.applicationId)
    search.set('applicationId', params.applicationId)
  if (params.deploymentTargetId)
    search.set('deploymentTargetId', params.deploymentTargetId)
  if (params.status)
    search.set('status', params.status)
  if (params.triggerType)
    search.set('triggerType', params.triggerType)
  if (params.sourceBranch)
    search.set('sourceBranch', params.sourceBranch)
  if (params.createdBy)
    search.set('createdBy', params.createdBy)
  return search.toString()
}

function translatedErrorMessage(code: string) {
  const translationKey = code ? `errors.${code}` : ''
  return translationKey && i18next.exists(translationKey) ? i18next.t(translationKey) : ''
}

function fallbackMessageForStatus(status: number) {
  if (status === 401)
    return translatedErrorMessage('auth.unauthorized')
  if (status === 403)
    return translatedErrorMessage('auth.forbidden')
  if (status === 404)
    return translatedErrorMessage('resource.not_found')
  if (status === 409)
    return translatedErrorMessage('resource.conflict')
  if (status === 429)
    return translatedErrorMessage('rate_limited')
  if (status >= 500)
    return translatedErrorMessage('internal_error')
  return translatedErrorMessage('request.failed')
}

async function parseErrorBody(response: Response): Promise<ApiErrorBody> {
  const contentType = response.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) {
    return response.json().catch(() => ({}))
  }
  const text = await response.text().catch(() => '')
  return text.trim() ? { error: text.trim() } : {}
}

async function apiErrorFromResponse(response: Response, path: string) {
  const body = await parseErrorBody(response)
  const code = typeof body.code === 'string' && body.code.trim() ? body.code.trim() : ''
  const detail = typeof body.detail === 'string' && body.detail.trim() ? body.detail.trim() : ''
  const bodyError = typeof body.error === 'string' && body.error.trim() ? body.error.trim() : ''
  const message = translatedErrorMessage(code) || detail || bodyError || fallbackMessageForStatus(response.status) || response.statusText
  return new ApiError(message, {
    code: code || `http.${response.status}`,
    detail: detail || bodyError,
    path,
    status: response.status,
  })
}

function apiNetworkError(path: string, error: unknown) {
  const detail = error instanceof Error ? error.message : String(error)
  const message = translatedErrorMessage('network.failed') || detail
  return new ApiError(message, {
    code: 'network.failed',
    detail,
    path,
    status: 0,
  })
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const { headers, ...requestOptions } = options ?? {}
  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      ...requestOptions,
      credentials: 'include',
      headers: {
        'Accept-Language': i18next.language,
        'Content-Type': 'application/json',
        ...headers,
      },
    })
  }
  catch (error) {
    throw apiNetworkError(path, error)
  }

  if (!response.ok) {
    throw await apiErrorFromResponse(response, path)
  }

  if (response.status === 204)
    return undefined as T

  return response.json()
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

export function buildJobLogsStreamUrl(projectId: string, jobId: string, after = 0) {
  const query = new URLSearchParams({ after: String(Math.max(0, after)) })
  return `${API_BASE_URL}/projects/${encodeURIComponent(projectId)}/build-jobs/${encodeURIComponent(jobId)}/logs/stream?${query.toString()}`
}

export function deploymentTargetDataExportUrl(projectId: string, applicationId: string, targetId: string) {
  return `${API_BASE_URL}/projects/${encodeURIComponent(projectId)}/applications/${encodeURIComponent(applicationId)}/deployment-targets/${encodeURIComponent(targetId)}/data-export`
}

export function releaseRuntimeTerminalUrl(projectId: string, releaseId: string, container = '') {
  const base = API_BASE_URL.startsWith('http://') || API_BASE_URL.startsWith('https://')
    ? API_BASE_URL
    : `${window.location.origin}${API_BASE_URL.startsWith('/') ? '' : '/'}${API_BASE_URL}`
  const url = new URL(`${base}/projects/${encodeURIComponent(projectId)}/releases/${encodeURIComponent(releaseId)}/terminal`)
  if (container.trim())
    url.searchParams.set('container', container.trim())
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

export function gitOAuthStartUrl(providerId: string, redirect = '/projects', frontendOrigin = window.location.origin, callbackOrigin = apiBaseOrigin()) {
  const params = new URLSearchParams({ callbackOrigin, frontendOrigin, redirect })
  return `${API_BASE_URL}/git/providers/${providerId}/oauth/start?${params.toString()}`
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
  listConfigDefinitions: () => request<ConfigDefinition[]>('/configs/definitions'),
  getConfigs: () => request<Record<string, string>>('/configs'),
  updateConfigs: (values: Record<string, unknown>) =>
    request<Record<string, string>>('/configs', { method: 'PUT', body: JSON.stringify({ values }) }),
  listGitProviders: (projectId?: string) =>
    request<GitProvider[]>(`/git/providers${optionalProjectQuery(projectId)}`),
  listGitProvidersPage: (params: PaginationParams & { projectId?: string }) =>
    request<PaginatedResponse<GitProvider>>(`/git/providers?${paginationWithProjectQuery(params)}`),
  createGitProvider: (payload: Omit<GitProvider, 'id' | 'createdAt' | 'clientSecretSet'> & { scope?: GitProvider['scope'], ownerRef?: string, clientSecret?: string }) =>
    request<GitProvider>('/git/providers', { method: 'POST', body: JSON.stringify(payload) }),
  updateGitProvider: (providerId: string, payload: Omit<GitProvider, 'id' | 'createdAt' | 'clientSecretSet'> & { scope?: GitProvider['scope'], ownerRef?: string, clientSecret?: string }) =>
    request<GitProvider>(`/git/providers/${providerId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGitProvider: (providerId: string) =>
    request<void>(`/git/providers/${providerId}`, { method: 'DELETE' }),
  listGitAccounts: (projectId?: string) =>
    request<GitAccount[]>(`/git/accounts${optionalProjectQuery(projectId)}`),
  listGitAccountsPage: (params: PaginationParams & { projectId?: string }) =>
    request<PaginatedResponse<GitAccount>>(`/git/accounts?${paginationWithProjectQuery(params)}`),
  createGitAccount: (payload: Omit<GitAccount, 'id' | 'userId' | 'scopes' | 'createdAt' | 'accessTokenSet' | 'refreshTokenSet'> & { scope?: GitAccount['scope'], ownerRef?: string, scopes: string[], accessToken?: string, refreshToken?: string }) =>
    request<GitAccount>('/git/accounts', { method: 'POST', body: JSON.stringify(payload) }),
  updateGitAccount: (accountId: string, payload: Omit<GitAccount, 'id' | 'userId' | 'scopes' | 'createdAt' | 'accessTokenSet' | 'refreshTokenSet'> & { scope?: GitAccount['scope'], ownerRef?: string, scopes: string[], accessToken?: string, refreshToken?: string }) =>
    request<GitAccount>(`/git/accounts/${accountId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGitAccount: (accountId: string) =>
    request<void>(`/git/accounts/${accountId}`, { method: 'DELETE' }),
  refreshGitAccount: (accountId: string) =>
    request<GitAccount>(`/git/accounts/${accountId}/refresh`, { method: 'POST' }),
  listGitRepositories: (accountId: string, params: { page: number, pageSize: number, search?: string, includePublic?: boolean }) => {
    const search = new URLSearchParams({ page: String(params.page), pageSize: String(params.pageSize) })
    if (params.search)
      search.set('search', params.search)
    if (params.includePublic)
      search.set('includePublic', 'true')
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
  listProjectsPage: (params: ProjectListParams) =>
    request<PaginatedResponse<Project>>(`/projects?${paginationQuery(params)}`),
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

  listRegistries: (projectId?: string) =>
    request<ArtifactRegistry[]>(`/registries${projectId ? `?projectId=${encodeURIComponent(projectId)}` : ''}`),
  listRegistriesPage: (params: PaginationParams & { projectId?: string }) =>
    request<PaginatedResponse<ArtifactRegistry>>(`/registries?${paginationWithProjectQuery(params)}`),
  createRegistry: (payload: ArtifactRegistryPayload) =>
    request<ArtifactRegistry>('/registries', { method: 'POST', body: JSON.stringify(payload) }),
  updateRegistry: (registryId: string, payload: ArtifactRegistryPayload) =>
    request<ArtifactRegistry>(`/registries/${registryId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRegistry: (registryId: string) =>
    request<void>(`/registries/${registryId}`, { method: 'DELETE' }),
  testRegistry: (registryId: string) =>
    request<RegistryTestResult>(`/registries/${registryId}/test`, { method: 'POST' }),
  getDefaultRegistry: (projectId: string) =>
    request<ArtifactRegistry>(`/projects/${projectId}/registries/default`),
  listRegistryCredentials: (registryId: string) =>
    request<RegistryCredential[]>(`/registries/${registryId}/credentials`),
  listRegistryCredentialsPage: (registryId: string, params: PaginationParams) =>
    request<PaginatedResponse<RegistryCredential>>(`/registries/${registryId}/credentials?${paginationQuery(params)}`),
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
  listContainerImages: (params: PaginationParams & { projectId?: string } = { page: 1, pageSize: 20 }) => {
    const search = new URLSearchParams(paginationQuery(params))
    if (params.projectId)
      search.set('projectId', params.projectId)
    return request<PaginatedResponse<ContainerImage>>(`/container-images?${search.toString()}`)
  },
  createContainerImage: (payload: Omit<ContainerImage, 'id' | 'createdBy' | 'createdAt' | 'imageRef'>) =>
    request<ContainerImage>('/container-images', { method: 'POST', body: JSON.stringify(payload) }),

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

  listRuntimeClusters: (projectId?: string) => request<RuntimeCluster[]>(`/runtime/clusters${optionalProjectQuery(projectId)}`),
  createRuntimeCluster: (payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>('/runtime/clusters', { method: 'POST', body: JSON.stringify(payload) }),
  updateRuntimeCluster: (clusterId: string, payload: Omit<RuntimeCluster, 'id' | 'createdBy' | 'createdAt' | 'kubeconfigSet' | 'lastCheckedAt'> & { kubeconfig?: string }) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteRuntimeCluster: (clusterId: string) =>
    request<void>(`/runtime/clusters/${clusterId}`, { method: 'DELETE' }),
  testRuntimeCluster: (clusterId: string) =>
    request<RuntimeCluster>(`/runtime/clusters/${clusterId}/test`, { method: 'POST' }),
  listRuntimeClusterResources: (clusterId: string, params: { kind: string, namespace?: string, projectId?: string, applicationId?: string, environmentId?: string }) => {
    const search = new URLSearchParams({ kind: params.kind })
    if (params.namespace)
      search.set('namespace', params.namespace)
    if (params.projectId)
      search.set('projectId', params.projectId)
    if (params.applicationId)
      search.set('applicationId', params.applicationId)
    if (params.environmentId)
      search.set('environmentId', params.environmentId)
    return request<ClusterResource[]>(`/runtime/clusters/${clusterId}/resources?${search.toString()}`)
  },
  listRuntimeClusterResourceEvents: (clusterId: string, params: { kind: string, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ kind: params.kind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<ClusterResourceEvent[]>(`/runtime/clusters/${clusterId}/resource-events?${search.toString()}`)
  },
  getRuntimeClusterResourceYAML: (clusterId: string, params: { kind: string, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ kind: params.kind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<ClusterResourceYAML>(`/runtime/clusters/${clusterId}/resource-yaml?${search.toString()}`)
  },
  deleteRuntimeClusterResource: (clusterId: string, params: { kind: string, namespace?: string, name: string }) => {
    const search = new URLSearchParams({ kind: params.kind, name: params.name })
    if (params.namespace)
      search.set('namespace', params.namespace)
    return request<void>(`/runtime/clusters/${clusterId}/resources?${search.toString()}`, { method: 'DELETE' })
  },
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
  getReleaseLogs: (projectId: string, releaseId: string) =>
    request<ReleaseLog>(`/projects/${projectId}/releases/${releaseId}/logs`),
  getReleaseRuntimeLogs: (projectId: string, releaseId: string, params: { container?: string, tailLines?: number } = {}) => {
    const search = new URLSearchParams()
    if (params.container)
      search.set('container', params.container)
    if (params.tailLines)
      search.set('tailLines', String(params.tailLines))
    const query = search.toString()
    return request<ReleaseRuntimeLog>(`/projects/${projectId}/releases/${releaseId}/runtime-logs${query ? `?${query}` : ''}`)
  },
  execReleaseRuntimeCommand: (projectId: string, releaseId: string, payload: { command: string, container?: string }) =>
    request<ReleaseRuntimeExecResult>(`/projects/${projectId}/releases/${releaseId}/exec`, { method: 'POST', body: JSON.stringify(payload) }),
  rollbackRelease: (projectId: string, releaseId: string) =>
    request<Release>(`/projects/${projectId}/releases/${releaseId}/rollback`, { method: 'POST' }),

  listGatewayRoutes: (projectId: string) =>
    request<GatewayRoute[]>(`/projects/${projectId}/gateway-routes`),
  listGatewayRoutesPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<GatewayRoute>>(`/projects/${projectId}/gateway-routes?${paginationQuery(params)}`),
  createGatewayRoute: (projectId: string, payload: Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt'>) =>
    request<GatewayRoute>(`/projects/${projectId}/gateway-routes`, { method: 'POST', body: JSON.stringify(payload) }),
  updateGatewayRoute: (projectId: string, routeId: string, payload: Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt'>) =>
    request<GatewayRoute>(`/projects/${projectId}/gateway-routes/${routeId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGatewayRoute: (projectId: string, routeId: string) =>
    request<void>(`/projects/${projectId}/gateway-routes/${routeId}`, { method: 'DELETE' }),
  checkGatewayDomain: (projectId: string, host: string, routeId?: string) =>
    request<GatewayDomainCheckResult>(`/projects/${projectId}/gateway-routes/check-domain?${new URLSearchParams({ host, ...(routeId ? { routeId } : {}) }).toString()}`),

  listAccessTokens: (params: PaginationParams) =>
    request<PaginatedResponse<AccessToken>>(`/access-tokens?${paginationQuery(params)}`),
  createAccessToken: (payload: { name: string, scope: string, expiresInDays: number }) =>
    request<{ token: AccessToken, accessToken: string }>('/access-tokens', { method: 'POST', body: JSON.stringify(payload) }),
  revokeAccessToken: (tokenId: string) =>
    request<AccessToken>(`/access-tokens/${tokenId}`, { method: 'DELETE' }),
}
