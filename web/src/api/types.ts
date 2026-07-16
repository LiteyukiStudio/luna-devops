// Domain DTOs shared by the API client and UI pages.

export interface Project {
  id: string
  slug: string
  name: string
  description: string
  namespaceStrategy: string
  maxConcurrentBuilds: number
  webConsoleEnabled: boolean
  billingOwnerUserId: string
  billingOwner?: ProjectBillingOwner
  systemKey: string
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

export interface ProjectBillingOwner {
  id: string
  email: string
  name: string
  avatarUrl: string
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

export interface ProjectMemberCandidate {
  id: string
  email: string
  name: string
  avatarUrl: string
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

export interface DataExportAuthorization {
  ticket: string
  expiresAt: string
}

export interface AppTemplateValueDefinition {
  key: string
  label: string
  description: string
  default: string
  required: boolean
  secret: boolean
  autoGenerate: boolean
}

export interface AppTemplate {
  id: string
  slug: string
  name: string
  description: string
  category: string
  kind?: 'application' | 'system_component' | string
  systemComponent?: string
  icon: string
  officialWebsite: string
  officialRepository: string
  popularityWeight: number
  image: string
  version: string
  servicePort: number
  defaultReplicas: number
  defaultCPU: string
  defaultMemory: string
  dataRetentionEnabled: boolean
  dataMountPath: string
  dataCapacity: string
  values: AppTemplateValueDefinition[]
}

export interface AppTemplateInstallation {
  id: string
  templateId: string
  templateVersion: string
  projectId: string
  applicationId: string
  deploymentTargetId: string
  releaseId: string
  status: string
  message: string
  valuesSnapshot: string
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface AppTemplateInstallPayload {
  applicationName: string
  applicationSlug: string
  deploymentName: string
  stage: string
  clusterId: string
  namespace: string
  imageRef: string
  replicas: number
  cpuRequest: string
  memoryRequest: string
  dataCapacity: string
  installNow: boolean
  values: Record<string, string>
}

export interface AppTemplateInstallResponse {
  installation: AppTemplateInstallation
  application: Application
  deploymentTarget: DeploymentTarget
  release?: Release
}

export interface SystemComponentInstallation {
  id: string
  componentId: string
  componentVersion: string
  runtimeClusterId: string
  projectId: string
  applicationId: string
  deploymentTargetId: string
  releaseId: string
  namespace: string
  status: string
  message: string
  controllerType: string
  mode: string
  config: string
  lastError: string
  installedBy: string
  createdAt: string
  updatedAt: string
}

export interface SystemComponentStatusResponse {
  items: SystemComponentInstallation[]
  gatewayTrafficProbeEnabled: boolean
}

export interface SystemComponentInstallPayload {
  clusterId: string
  namespace?: string
  mode?: string
  apiBaseUrl: string
  traefikMetricsUrl?: string
}

export interface SystemComponentInstallResponse {
  installation: SystemComponentInstallation
  application?: Application
  deploymentTarget?: DeploymentTarget
  release?: Release
}

export interface GatewayTrafficStatus {
  available: boolean
  installed: boolean
  status: string
  componentId: string
  installableTemplateId: string
  lastHeartbeatAt?: string | null
  lastReportedAt?: string | null
  lastWindowStart?: string | null
  lastWindowEnd?: string | null
  lastError: string
}

export interface NotificationChannel {
  id: string
  projectId: string
  name: string
  adapterKind: 'webhook' | 'smtp' | string
  configJson: string
  config?: unknown
  secretSet?: Record<string, boolean>
  enabled: boolean
  lastDeliveryStatus: string
  lastDeliveryError: string
  lastDeliveredAt?: string | null
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface NotificationTemplate {
  id: string
  projectId: string
  name: string
  eventType: string
  adapterKind: 'webhook' | 'smtp' | string
  locale: string
  subjectTemplate: string
  bodyTemplate: string
  jsonBodyTemplate: string
  enabled: boolean
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface NotificationRuleFilter {
  severities?: string[]
  projectIds?: string[]
  applicationIds?: string[]
  deploymentTargetIds?: string[]
}

export interface NotificationRule {
  id: string
  projectId: string
  name: string
  eventTypesJson: string
  filterJson: string
  channelIdsJson: string
  templateId: string
  locale: string
  enabled: boolean
  lastMatchedEventId: string
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface NotificationDelivery {
  id: string
  projectId: string
  eventId: string
  eventType: string
  severity: string
  channelId: string
  adapterKind: string
  ruleId: string
  templateId: string
  status: string
  attemptCount: number
  durationMillis: number
  errorMessage: string
  requestSnapshot: string
  responseSnippet: string
  queuedAt: string
  startedAt?: string | null
  finishedAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface NotificationPreset {
  id: string
  name: string
  description: string
  adapterKind: string
  secretFields: string[]
}

export interface PlatformEventEntityRef {
  id: string
  name: string
  slug: string
}

export interface PlatformEventSnapshot {
  id?: string
  type?: string
  severity?: string
  message?: string
  occurredAt?: string
  project?: PlatformEventEntityRef
  application?: PlatformEventEntityRef
  deploymentTarget?: PlatformEventEntityRef
  actor?: { id: string, name: string, email: string }
  build?: Record<string, unknown>
  release?: Record<string, unknown>
  hook?: Record<string, unknown>
  gateway?: Record<string, unknown>
  certificate?: Record<string, unknown>
}

export interface PlatformEvent {
  id: string
  type: string
  category: string
  severity: 'info' | 'warning' | 'error' | string
  status: 'in_progress' | 'succeeded' | 'failed' | 'canceled' | string
  projectId: string
  applicationId: string
  deploymentTargetId: string
  resourceType: string
  resourceId: string
  actorId: string
  summaryKey: string
  message: string
  correlationId: string
  traceId: string
  occurredAt: string
  createdAt: string
  detail: PlatformEventSnapshot
  links: Record<string, string>
  deliveryCount: number
}

export interface PlatformEventCatalogEntry {
  type: string
  category: string
  defaultSeverity: string
  recommendedNotify: boolean
}

export interface PlatformEventListParams extends PaginationParams {
  scope?: 'mine' | 'all'
  projectId?: string
  projectIds?: string[]
  applicationId?: string
  applicationIds?: string[]
  deploymentTargetId?: string
  deploymentTargetIds?: string[]
  category?: string
  categories?: string[]
  type?: string
  types?: string[]
  severity?: string
  severities?: string[]
  status?: string
  statuses?: string[]
  dateFrom?: string
  dateTo?: string
}

export interface NotificationChannelPayload {
  name: string
  adapterKind: string
  config: unknown
  secrets?: Record<string, string>
  enabled: boolean
}

export interface NotificationTemplatePayload {
  name: string
  eventType: string
  adapterKind: string
  locale: string
  subjectTemplate: string
  bodyTemplate: string
  jsonBodyTemplate: string
  enabled: boolean
}

export interface NotificationRulePayload {
  name: string
  eventTypes: string[]
  filter: NotificationRuleFilter
  channelIds: string[]
  templateId: string
  locale: string
  enabled: boolean
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
  provider: 'harbor' | 'dockerhub' | 'gitea-registry' | 'generic-oci'
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
  usage: 'push-pull' | 'push' | 'pull'
  scope: 'global' | 'project' | 'user'
  ownerRef: string
  projectIds: string[]
  repositoryTemplate: string
  tagTemplate: string
  passwordSet: boolean
  tokenSet: boolean
  createdAt: string
}

export interface RegistryImageTemplateDefault {
  targetImageRef: string
  targetRepository: string
  targetTag: string
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

export interface ReleaseImageCandidate {
  key: string
  source: 'registry' | 'build' | 'target' | string
  label: string
  imageRef: string
  buildRunId: string
  tag: string
  digest: string
  sourceCommit: string
  createdAt: string
}

export interface ReleaseImageCandidates {
  items: ReleaseImageCandidate[]
  registryAvailable: boolean
  registryError: string
  fallbackUsed: boolean
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

export type RuntimeConfigRefMode = 'live' | 'snapshot'

export interface DeploymentRuntimeConfigRef {
  setId: string
  mode: RuntimeConfigRefMode
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
  buildArgs: string
  buildEnvironmentId: string
  buildCpuRequest: string
  buildMemoryRequest: string
  buildTimeoutSeconds: number
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
  stage: 'dev' | 'test' | 'staging' | 'prod'
  clusterId: string
  namespace: string
  workloadType: 'Deployment' | 'StatefulSet' | string
  replicas: number
  cpuRequest: string
  memoryRequest: string
  cpuLimit: string
  memoryLimit: string
  imagePullPolicy: '' | 'IfNotPresent' | 'Always' | 'Never' | string
  containerCommand: string
  containerArgs: string
  lifecycle: string
  initContainers: string
  sidecarContainers: string
  readinessProbe: string
  livenessProbe: string
  startupProbe: string
  runAsUser: string
  runAsGroup: string
  fsGroup: string
  fsGroupChangePolicy: '' | 'OnRootMismatch' | 'Always' | string
  readOnlyRootFilesystem: boolean
  allowPrivilegeEscalation: '' | 'true' | 'false' | string
  capabilityAdd: string
  capabilityDrop: string
  nodeSelector: string
  tolerations: string
  affinity: string
  topologySpreadConstraints: string
  priorityClassName: string
  serviceAccountName?: string
  automountServiceAccountToken?: string
  serviceType: '' | 'ClusterIP' | 'NodePort' | 'LoadBalancer' | string
  serviceAnnotations: string
  serviceExternalTrafficPolicy: '' | 'Cluster' | 'Local' | string
  serviceSessionAffinity: '' | 'None' | 'ClientIP' | string
  autoScalingEnabled: boolean
  autoScalingMinReplicas: number
  autoScalingMaxReplicas: number
  autoScalingCpuPercent: number
  autoScalingMemoryPercent: number
  autoScalingBehavior: string
  servicePort: number
  servicePorts: DeploymentServicePort[]
  sourceType: 'repository' | 'image'
  repositoryBindingId: string
  dockerfilePath: string
  buildContext: string
  buildDirectory: string
  buildArgs: string
  buildEnvironmentId: string
  buildCpuRequest: string
  buildMemoryRequest: string
  buildTimeoutSeconds: number
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
  runtimeConfigRefs: DeploymentRuntimeConfigRef[]
  envVars: string
  configRefs: string
  secretRefsSet: boolean
  configFiles: string
  secretFilesSet: boolean
  dataRetentionEnabled: boolean
  dataCapacity: string
  dataMountPath: string
  dataVolumes: string
  dataStorageClassName: string
  dataAccessMode: '' | 'ReadWriteOnce' | 'ReadWriteMany' | 'ReadOnlyMany' | string
  dataVolumeMode: '' | 'Filesystem' | 'Block' | string
  requireApproval: boolean
  webConsoleEnabled: boolean | null
  enabled: boolean
  deleteStatus: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteMessage: string
  deleteStartedAt?: string | null
  deleteFinishedAt?: string | null
  createdBy: string
  createdAt: string
}

export interface DeploymentServicePort {
  name: string
  port: number
  appProtocol?: string
}

export interface DeploymentTargetMetrics {
  available: boolean
  reason?: string
  podCount: number
  containerCount: number
  cpuUsageMilli: number
  cpuCapacityMilli: number
  cpuUsagePercent: number
  memoryUsageBytes: number
  memoryCapacityBytes: number
  memoryUsagePercent: number
  updatedAt: string
}

export type DeploymentTargetPayload = Omit<DeploymentTarget, 'id' | 'projectId' | 'applicationId' | 'createdBy' | 'createdAt' | 'buildVariableSetIds' | 'runtimeConfigSetIds' | 'runtimeConfigRefs' | 'secretRefsSet' | 'secretFilesSet' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt'> & {
  buildVariableSetIds: string | string[]
  runtimeConfigSetIds: string | string[]
  runtimeConfigRefs: DeploymentRuntimeConfigRef[]
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
  gatewayRootDomain: string
  gatewayDomainSuffixes: string[]
  gatewayPublicScheme: 'http' | 'https'
  gatewayPublicPort: number
  gatewayProvider: 'gateway-api'
  gatewayControllerType: 'traefik' | 'generic'
  gatewayClassName: string
  gatewayName: string
  gatewayNamespace: string
  gatewayHttpListenerName: string
  gatewayHttpListenerPort: number
  gatewayHttpsListenerName: string
  gatewayHttpsListenerPort: number
  gatewayTlsSecretName: string
  gatewayTlsSecretNamespace: string
  gatewayCertIssuerKind: 'ClusterIssuer' | 'Issuer'
  gatewayCertIssuerName: string
  gatewayCertificateNamespace: string
  gatewayWildcardCertEnabled: boolean
  gatewayWildcardCertDomain: string
  gatewayWildcardCertSecretName: string
  gatewayExternalTLSMode: 'none' | 'gateway' | 'upstream'
  gatewayForwardedHeadersMode: 'preserve' | 'overwrite' | 'none'
  gatewayTrustedProxyCIDRs: string
  gatewayDefaultRequestHeaders: string
  gatewayDefaultResponseHeaders: string
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
  children?: ClusterResource[]
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

export interface RuntimeClusterResourceListParams extends PaginationParams {
  kind: string
  namespace?: string
  projectId?: string
  applicationId?: string
  environmentId?: string
}

export interface Release {
  id: string
  projectId: string
  applicationId: string
  environmentId: string
  deploymentTargetId: string
  buildRunId: string
  imageRef: string
  forceImagePull: boolean
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
  domainSuffix: string
  path: string
  servicePort: number
  tlsMode: 'http-only' | 'http-challenge' | 'manual-cert'
  certificateStatus: 'disabled' | 'pending' | 'issued' | 'failed' | 'expired'
  certificateMessage?: string
  certificateNotAfter?: string | null
  certificateIssuerKind?: string
  certificateIssuerName?: string
  cnameName: string
  cnameTarget: string
  accessUrl: string
  dnsStatus: 'pending' | 'verified' | 'failed'
  status: 'pending' | 'ready' | 'active' | 'disabled' | 'failed'
  enabled: boolean
  parentGatewayName: string
  parentGatewayNamespace: string
  sectionName: string
  pathMatchType: 'PathPrefix' | 'Exact'
  requestHeaders: string
  responseHeaders: string
  urlRewrite: string
  requestRedirect: string
  backendWeight: number
  hostnameAliases: string
  routeSummary: string
  conditions: Array<{ type: string, status: string, reason: string, message: string, observedGeneration: number }>
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

export interface AccessTokenScopeDefinition {
  value: string
  group: string
  recommended: boolean
  creatableByUser: boolean
  requiresAdminRole: boolean
}

export interface AccessTokenScopeCatalog {
  items: AccessTokenScopeDefinition[]
}

export interface BillingSummary {
  balanceCredits: string
  todaySpend: string
  monthSpend: string
  periodSpend: string
  pendingSpend: string
  availableCredits: string
  lowBalanceLimit: string
  balanceStatus: 'ok' | 'low' | 'insufficient' | string
  monthlyCategories: BillingSpendCategory[]
  periodCategories: BillingSpendCategory[]
}

export interface BillingSpendCategory {
  category: 'build' | 'runtime' | 'storage' | 'gateway' | 'adjustment' | 'other' | string
  amountCredits: string
}

export interface BillingDeploymentSpend {
  projectId: string
  projectName: string
  projectSlug: string
  applicationId: string
  applicationName: string
  applicationSlug: string
  deploymentTargetId: string
  deploymentTargetName: string
  deploymentTargetStage: string
  amountCredits: string
  buildCredits: string
  runtimeCredits: string
  storageCredits: string
  gatewayCredits: string
  otherCredits: string
}

export interface BillingRateRule {
  id: string
  meter: string
  unit: string
  creditsPerUnit: string
  enabled: boolean
  description: string
  createdAt: string
  updatedAt: string
}

export interface BillingRateRulePayload {
  meter: string
  creditsPerUnit: string
  enabled: boolean
}

export interface BillingWalletTransactionPayload {
  amountCredits: string
  type: 'credit' | 'adjustment'
  description: string
  userId: string
}

export interface GatewayTrafficUsagePayload {
  routeId: string
  responseBytes: number
  requestCount?: number
  periodStart: string
  periodEnd: string
}

export interface BillingUsageSettlementResult {
  status: 'settled' | 'already_settled' | string
}

export interface BillingLedgerEntry {
  id: string
  userId: string
  projectId: string
  applicationId: string
  applicationName: string
  applicationSlug: string
  type: 'debit' | 'credit' | 'adjustment' | string
  amountCredits: string
  balanceAfterCredits: string
  reason: string
  meter: string
  usageRecordId: string
  resourceType: string
  resourceId: string
  description: string
  createdBy: string
  createdAt: string
}

export interface BillingUsageRecord {
  id: string
  projectId: string
  billedUserId: string
  applicationId: string
  applicationName: string
  applicationSlug: string
  meter: string
  quantity: string
  unit: string
  amountCredits: string
  resourceType: string
  resourceId: string
  periodStart: string
  periodEnd: string
  status: 'pending' | 'settled' | 'failed' | string
  metadata: string
  settledAt?: string | null
  createdAt: string
  updatedAt: string
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

export interface BillingListParams extends PaginationParams {
  projectIds?: string[]
  type?: string
  meter?: string
  periodStart?: string
  periodEnd?: string
  userId?: string
}

export interface BillingPeriodParams {
  periodStart?: string
  periodEnd?: string
  accountScope?: 'current'
  userId?: string
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
  avatarUrl: string
  authType: 'local' | 'oidc'
  role: 'platform_admin' | 'user'
  language: 'zh-CN' | 'en-US'
  disabled: boolean
  mfaEnabled: boolean
  balanceCredits: string
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
  label?: string
  description?: string
  labelKey?: string
  descriptionKey?: string
  type: 'string' | 'textarea' | 'select' | 'boolean' | 'number'
  public: boolean
  default: string
  options?: string[]
}

export interface DataRetentionDataset {
  key: string
  configKey: string
  defaultDays: number
}

export interface DataRetentionCatalogResponse {
  items: DataRetentionDataset[]
}

export interface DataRetentionPayload {
  datasets: string[]
  startAt: string
  endAt: string
}

export interface DataRetentionResult {
  dataset: string
  matched: number
  deleted: number
}

export interface DataRetentionResultResponse {
  items: DataRetentionResult[]
}

export interface MFAStatus {
  enabled: boolean
  pending: boolean
  policyEnabled: boolean
  enrollmentReauthMode: 'password' | 'fresh_session'
  confirmedAt?: string | null
  recoveryCodesRemaining: number
}

export const mfaPurposes = [
  'runtime_exec',
  'runtime_terminal',
  'data_export',
  'secret_update',
  'registry_credential_update',
  'kubeconfig_update',
  'auth_provider_update',
  'user_admin_update',
  'mfa_manage',
  'security_settings_update',
  'data_retention_cleanup',
] as const

export type MFAPurpose = typeof mfaPurposes[number]

export interface MFAChallenge {
  purpose: MFAPurpose
}

export interface MFAEnrollmentRequest {
  currentPassword?: string
}

export interface MFAEnrollment {
  secret: string
  otpauthUrl: string
  qrCodeDataUrl?: string
}

export interface MFARecoveryCodes {
  recoveryCodes: string[]
}

export type MFAVerifyPayload
  = | { code: string, purpose: MFAPurpose }
    | { recoveryCode: string, purpose: MFAPurpose }

export interface MFAVerifyResponse {
  verified: boolean
  purpose: MFAPurpose
}

export interface BootstrapStatus {
  mode: 'development' | 'production'
  initialized: boolean
  devLoginEnabled: boolean
  bootstrapTokenRequired?: boolean
  devLoginHint?: {
    email: string
    password: string
  }
}
