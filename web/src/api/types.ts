// Domain DTOs shared by the API client and UI pages.

export interface Project {
  id: string
  slug: string
  name: string
  description: string
  namespaceStrategy: string
  maxConcurrentBuilds: number
  deleteStatus: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteMessage: string
  deleteStartedAt?: string | null
  deleteFinishedAt?: string | null
  dashboardOrder: number
  lastUsedAt?: string | null
  useCount: number
  createdAt: string
  updatedAt: string
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
  icon: string
  deleteStatus: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteMessage: string
  deleteStartedAt?: string | null
  deleteFinishedAt?: string | null
  dataRetentionMode: 'retain' | 'retained' | 'deleted' | string
  createdAt: string
}

export interface GitProvider {
  id: string
  type: 'github' | 'gitea' | 'gitlab'
  name: string
  baseUrl: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  projectIds: string[]
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
  projectIds: string[]
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
  webhookId: string
  webhookCallbackUrl: string
  lastEvent: string
  lastCommitSha: string
  lastWebhookAt?: string
  providerName?: string
  providerType?: GitProvider['type']
  accountUsername?: string
  accountOwnerEmail?: string
  accountOwnerName?: string
  applicationName?: string
  createdAt: string
}

export type RepositoryBindingPayload = Pick<RepositoryBinding, 'applicationId' | 'gitAccountId' | 'owner' | 'repo' | 'cloneUrl' | 'defaultBranch' | 'webhookStatus'> & {
  autoConfigureWebhook?: boolean
}

export interface GitRepository {
  owner: string
  name: string
  fullName: string
  cloneUrl: string
  defaultBranch: string
  private: boolean
  source: 'accessible' | 'public'
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
  exposedPorts?: Record<string, number[]>
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
  projectIds: string[]
  defaultProjectIds: string[]
  credentialSet: boolean
  isDefault: boolean
  capabilities: string[]
  createdBy: string
  createdAt: string
}

export type ArtifactRegistryPayload = Omit<ArtifactRegistry, 'id' | 'namespace' | 'credentialSet' | 'defaultProjectIds' | 'createdBy' | 'createdAt'>

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

export interface BuildVariableSet {
  id: string
  name: string
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  projectIds: string[]
  variables: string | Record<string, string>
  variableCount?: number
  canInspectVariables?: boolean
  secrets: Record<string, boolean>
  enabled: boolean
  createdBy: string
  createdAt: string
}

export type BuildVariableSetPayload = Omit<BuildVariableSet, 'id' | 'createdBy' | 'createdAt' | 'secrets' | 'variableCount' | 'canInspectVariables'> & {
  secrets: Record<string, string>
}

export interface ProjectRuntimeConfigSet {
  id: string
  projectId: string
  name: string
  envVars: string
  configFiles: string
  secretRefsSet: boolean
  secretFilesSet: boolean
  enabled: boolean
  deleteStatus: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteMessage: string
  createdBy: string
  createdAt: string
  affectedDeploymentTargetCount?: number
}

export type ProjectRuntimeConfigSetPayload = Omit<ProjectRuntimeConfigSet, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'secretRefsSet' | 'secretFilesSet' | 'deleteStatus' | 'deleteMessage'> & {
  secretRefs?: string
  secretFiles?: string
}

export type HookPhase = 'prePull' | 'postPull' | 'preBuild' | 'postBuild' | 'prePush' | 'postPush' | 'preDeployment' | 'postDeployment'

export interface ProjectHookConfig {
  id: string
  projectId: string
  name: string
  script: string
  shell: 'sh' | 'bash'
  timeoutSeconds: number
  failurePolicy: 'fail' | 'ignore'
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type ProjectHookConfigPayload = Omit<ProjectHookConfig, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'updatedAt'>

export interface HookRun {
  id: string
  projectId: string
  hookConfigId: string
  buildRunId: string
  buildJobId: string
  releaseId: string
  applicationId: string
  environmentId: string
  deploymentTargetId: string
  name: string
  phase: HookPhase
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'timeout' | 'skipped' | string
  scriptSnapshot: string
  shell: ProjectHookConfig['shell']
  imageRef: string
  timeoutSeconds: number
  failurePolicy: ProjectHookConfig['failurePolicy']
  exitCode: number
  message: string
  startedAt?: string | null
  finishedAt?: string | null
  createdAt: string
}

export interface HookRunLog {
  id?: string
  hookRunId: string
  projectId: string
  content: string
  createdAt?: string
  updatedAt?: string
}

export interface BuildRun {
  id: string
  projectId: string
  applicationId: string
  deploymentTargetId: string
  buildLabels: string
  buildVariableSetIds: string | string[]
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'lost' | 'timeout'
  triggerType: 'manual' | 'webhook' | 'push' | 'tag' | 'api' | 'retry'
  sourceBranch: string
  sourceTag: string
  sourceCommit: string
  dockerfilePath: string
  buildContext: string
  buildDirectory: string
  targetRegistryId: string
  targetImageRef?: string
  targetRepository: string
  targetTag: string
  imageRef: string
  imageDigest: string
  cacheConfig: string
  cpuCoreSeconds: number
  memoryMbSeconds: number
  creditCost: number
  startedAt?: string
  finishedAt?: string
  createdBy: string
  triggeredByName: string
  triggeredByEmail: string
  sourceAuthorName: string
  sourceAuthorEmail: string
  createdAt: string
}

export interface DeploymentTargetHookBinding {
  id?: string
  projectId?: string
  applicationId?: string
  deploymentTargetId?: string
  hookConfigId: string
  phase: HookPhase
  runOrder: number
  createdAt?: string
  updatedAt?: string
}

export interface DeploymentTarget {
  id: string
  projectId: string
  applicationId: string
  environmentId: string
  name: string
  servicePort: number
  sourceType: 'repository' | 'image'
  repositoryBindingId: string
  dockerfilePath: string
  buildContext: string
  buildDirectory: string
  targetRegistryId: string
  targetRepository: string
  targetTag: string
  targetImageRef?: string
  imageRef: string
  buildLabels: string
  buildVariableSetIds: string | string[]
  buildHooksEnabled: boolean
  buildHookBindings: DeploymentTargetHookBinding[]
  autoDeploy: boolean
  branchPattern: string
  tagPattern: string
  concurrencyPolicy: 'queue' | 'parallel'
  runtimeConfigSetIds: string | string[]
  envVars: string
  configRefs: string
  secretRefsSet: boolean
  configFiles: string
  secretFilesSet: boolean
  dataRetentionEnabled: boolean
  dataCapacity: string
  dataMountPath: string
  dataVolumes: string
  requireApproval: boolean
  enabled: boolean
  deleteStatus: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteMessage: string
  deleteStartedAt?: string | null
  deleteFinishedAt?: string | null
  createdBy: string
  createdAt: string
}

export type DeploymentTargetPayload = Omit<DeploymentTarget, 'id' | 'projectId' | 'applicationId' | 'createdBy' | 'createdAt' | 'buildVariableSetIds' | 'runtimeConfigSetIds' | 'secretRefsSet' | 'secretFilesSet' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt'> & {
  buildVariableSetIds: string | string[]
  runtimeConfigSetIds: string | string[]
  secretRefs?: string
  secretFiles?: string
}

export type ApplicationPayload = Pick<Application, 'name' | 'slug' | 'icon'>

export interface BuildJob {
  id: string
  buildRunId: string
  projectId: string
  type: string
  status: string
  message: string
  logRef: string
  attempts: number
  leaseUntil?: string | null
  lastHeartbeatAt?: string | null
  executorId?: string
  executorName?: string
  startedAt?: string
  finishedAt?: string
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
  projectIds: string[]
  kubeconfig?: string
  kubeconfigSet: boolean
  isDefault: boolean
  maxConcurrentBuilds: number
  status: string
  lastCheckedAt?: string
  createdBy: string
  createdAt: string
}

export interface ClusterResource {
  id: string
  kind: string
  name: string
  namespace: string
  status: string
  summary: string
  projectId: string
  applicationId: string
  environmentId: string
  deploymentTargetId: string
  releaseId: string
  routeId: string
  projectName: string
  applicationName: string
  deploymentTargetName: string
  labels: Record<string, string>
  createdAt: string
  updatedAt: string
}

export interface ClusterResourceEvent {
  id: string
  type: string
  reason: string
  message: string
  source: string
  count: number
  firstSeen: string
  lastSeen: string
}

export interface ClusterResourceYAML {
  yaml: string
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
  deploymentTargetId: string
  buildRunId: string
  imageRef: string
  type: 'deploy' | 'rollback'
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  revision: number
  rollbackFromId: string
  message: string
  startedAt?: string
  finishedAt?: string
  createdBy: string
  createdAt: string
}

export interface ReleaseLog {
  id: string
  releaseId: string
  projectId: string
  content: string
  createdAt: string
  updatedAt: string
}

export interface ReleaseRuntimeLog {
  pod: string
  container: string
  content: string
}

export interface ReleaseRuntimeExecResult {
  pod: string
  container: string
  stdout: string
  stderr: string
  exitCode: number
}

export interface GatewayRoute {
  id: string
  projectId: string
  applicationId: string
  environmentId: string
  deploymentTargetId: string
  host: string
  path: string
  servicePort: number
  tlsMode: 'http-only' | 'http-challenge' | 'manual-cert'
  certificateStatus: 'disabled' | 'pending' | 'issued' | 'failed' | 'expired'
  cnameName: string
  cnameTarget: string
  accessUrl: string
  dnsStatus: 'pending' | 'verified' | 'failed'
  status: 'pending' | 'ready' | 'active' | 'disabled' | 'failed'
  enabled: boolean
  deleteStatus: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteMessage: string
  deleteStartedAt?: string | null
  deleteFinishedAt?: string | null
  isDefault: boolean
  createdBy: string
  createdAt: string
}

export interface GatewayDomainCheckResult {
  available: boolean
  host: string
  status: 'available' | 'current' | 'conflict'
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

export type ProjectListScope = 'related' | 'all'

export interface ProjectListParams extends PaginationParams {
  scope?: ProjectListScope
}

export interface BuildRunListParams extends PaginationParams {
  applicationId?: string
  deploymentTargetId?: string
  status?: BuildRun['status']
  triggerType?: BuildRun['triggerType']
  sourceBranch?: string
  createdBy?: string
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

export interface OIDCCallbackConfig {
  publicBaseUrl: string
  callbackUrl: string
  configured: boolean
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
  requireVerifiedOidcEmail: boolean
  allowedEmailDomains: string[]
  allowedOidcGroups: string[]
  invitedEmails: string[]
  defaultRole: 'platform_admin' | 'user'
}

export interface ConfigDefinition {
  key: string
  label: string
  description: string
  type: 'string' | 'textarea' | 'select'
  public: boolean
  default: string
  options?: string[]
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
