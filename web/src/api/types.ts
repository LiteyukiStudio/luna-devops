import type { PlatformRoleValue, ProjectRoleValue } from '@/lib/roles'

// Domain DTOs shared by the API client and UI pages.

export type LiveObservationStatus
  = | 'ready'
    | 'scaled-to-zero'
    | 'degraded'
    | 'progressing'
    | 'not-found'
    | 'not-configured'
    | 'unavailable'
    | 'unknown'
    | 'declared'

export type AgentObservabilitySource = 'prometheus' | 'loki' | 'tempo'
export type AgentObservabilityRange = '1h' | '6h' | '24h' | '7d' | '30d' | '1y'

export interface APIMeta {
  serverVersion: string
  features?: {
    kubectlGateway?: boolean
  }
}

export interface AgentObservabilityTestResult {
  source: AgentObservabilitySource
  reachable: boolean
  dataAvailable: boolean
  latencyMs: number
  code: string
}

export interface AgentObservabilityPoint { timestamp: number, value: number }
export interface AgentObservabilitySeries { labels: Record<string, string>, points: AgentObservabilityPoint[] }
export interface AgentObservabilityLog { timestamp: string, line: string, labels: Record<string, string> }
export interface AgentObservabilityTrace {
  traceId: string
  rootServiceName: string
  rootTraceName: string
  startTimeUnixNano: string
  durationMs: number
}
export interface AgentObservabilityTraceSpanEvent {
  name: string
  timeUnixNano: string
  attributes: Record<string, string>
}
export interface AgentObservabilityTraceSpan {
  spanId: string
  parentSpanId: string
  name: string
  serviceName: string
  kind: string
  status: string
  startTimeUnixNano: string
  startOffsetMs: number
  durationMs: number
  attributes: Record<string, string>
  events: AgentObservabilityTraceSpanEvent[]
  raw: Record<string, unknown>
}
export interface AgentObservabilityUsage {
  inputTokens: number
  outputTokens: number
  cacheReadInputTokens: number | null
  cacheWriteInputTokens: number | null
  reasoningOutputTokens: number | null
  cacheHitRate: number | null
}
export interface AgentObservabilityTraceDetail {
  traceId: string
  durationMs: number
  spanCount: number
  errorCount: number
  usage: AgentObservabilityUsage | null
  spans: AgentObservabilityTraceSpan[]
  context?: AgentObservabilityTraceContext
}
export interface AgentObservabilityConversationUser {
  id: string
  name: string
  email: string
  avatarUrl: string
}
export interface AgentObservabilityConversation {
  id: string
  title: string
  user: AgentObservabilityConversationUser
  turnCount: number
  traceCount: number
  createdAt: string
  updatedAt: string
}
export interface AgentObservabilityConversationTurn {
  id: string
  turnIndex: number
  status: string
  userMessage: string
  assistantMessage: string
  runId: string
  traceId: string
  inputTokens: number
  outputTokens: number
  cacheReadInputTokens: number | null
  cacheWriteInputTokens: number | null
  reasoningOutputTokens: number | null
  durationMs: number
  createdAt: string
  loops: AgentObservabilityConversationLoop[]
}
export interface AgentObservabilityTurn {
  id: string
  conversationId: string
  conversationTitle: string
  user: AgentObservabilityConversationUser
  turnIndex: number
  status: string
  userMessage: string
  assistantMessage: string
  runId: string
  traceId: string
  inputTokens: number
  outputTokens: number
  cacheReadInputTokens: number | null
  cacheWriteInputTokens: number | null
  reasoningOutputTokens: number | null
  toolCallCount: number
  durationMs: number
  createdAt: string
}
export interface AgentObservabilityToolSummary {
  operationId: string
  totalCalls: number
  succeededCalls: number
  failedCalls: number
  otherCalls: number
  successRate: number
  lastCalledAt: string
}
export interface AgentObservabilityToolCall extends AgentObservabilityConversationToolCall {
  durationMs: number
  runId: string
  turnId: string
  turnIndex: number
  conversationId: string
  conversationTitle: string
  user: AgentObservabilityConversationUser
  createdAt: string
}
export interface AgentObservabilityConversationLoop {
  loopIndex: number
  items: AgentObservabilityConversationRunItem[]
}
export interface AgentObservabilityConversationRunItem {
  id: string
  timelineIndex: number
  type: 'reasoning_summary' | 'assistant_message' | 'tool_call'
  status: string
  text: string
  toolCall?: AgentObservabilityConversationToolCall
  createdAt: string
}
export interface AgentObservabilityConversationToolCall {
  id: string
  operationId: string
  status: string
  arguments: Record<string, unknown>
  result?: unknown
  errorCode?: string
  durationMs?: number
  traceId?: string
}
export interface AgentObservabilityTraceContext {
  conversation: AgentObservabilityConversation
  turn: AgentObservabilityConversationTurn
}
export interface AgentObservabilityOverview {
  generatedAt: string
  range: AgentObservabilityRange
  summary: AgentObservabilityUsage & {
    toolCalls: number
    toolSuccessRate: number
    turnCount: number
    turnSuccessRate: number
    runDurationP95: number
  }
  sourceStatus: Partial<Record<AgentObservabilitySource, 'ready' | 'unavailable'>>
  observationCode: string
}

export interface Project {
  id: string
  identifier: string
  kubernetesNamespace: string
  name: string
  description: string
  namespaceStrategy: string
  maxConcurrentBuilds: number
  webConsoleEnabled: boolean
  currentUserRole?: ProjectRoleValue
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
  role: ProjectRoleValue
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
  identifier: string
  name: string
  icon: string
  deleteStatus: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteMessage: string
  deleteStartedAt?: string | null
  deleteFinishedAt?: string | null
  deploymentSummary?: ApplicationDeploymentSummary
  createdAt: string
  updatedAt: string
}

export interface ApplicationDeploymentSummary {
  targetCount: number
  desiredReplicas: number
  readyReplicas: number
  status: LiveObservationStatus | 'not-deployed'
  targets: ApplicationDeploymentTargetSummary[]
}

export interface ApplicationDeploymentTargetSummary {
  id: string
  stage: string
  desiredReplicas: number
  readyReplicas: number
  status: LiveObservationStatus | 'disabled'
}

export interface ApplicationTopologyTarget {
  id: string
  name: string
  stage: string
  clusterId: string
  clusterName: string
  namespace: string
}

export interface ApplicationTopologyNode {
  id: string
  kind: string
  name: string
  namespace: string
  status: string
  summary: string
  clusterId: string
  clusterName: string
  deploymentTargetId: string
}

export interface ApplicationTopologyEdge {
  id: string
  source: string
  target: string
  type: string
}

export interface ApplicationTopologyWarning {
  code: string
  deploymentTargetId: string
  deploymentTargetName: string
  clusterId: string
  clusterName: string
}

export interface ApplicationTopology {
  generatedAt: string
  targets: ApplicationTopologyTarget[]
  nodes: ApplicationTopologyNode[]
  edges: ApplicationTopologyEdge[]
  warnings: ApplicationTopologyWarning[]
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

export type AppTemplateDataVolume
  = | {
    logicalName: string
    sourceType: 'projectVolume'
    mountPath?: string
    devicePath?: string
    readOnly?: boolean
  }
  | {
    logicalName: string
    sourceType: 'emptyDir'
    mountPath: string
    emptyDir?: {
      medium?: '' | 'Memory'
      sizeLimit?: string
    }
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
  dataVolumes: AppTemplateDataVolume[]
  values: AppTemplateValueDefinition[]
}

export type AppTemplateSummary = Omit<AppTemplate, 'values'> & {
  valueCount: number
  requiredValueCount: number
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
  applicationIdentifier: string
  deploymentName: string
  stage: string
  clusterId: string
  imageRef: string
  replicas: number
  cpuRequest: string
  memoryRequest: string
  projectVolumeId?: string
  installNow: boolean
  provisionAccess?: boolean
  values: Record<string, string>
}

export interface AppTemplateInstallResponse {
  installation: AppTemplateInstallation
  application: Application
  deploymentTarget: DeploymentTarget
  release?: Release
}

export interface ApplicationDeletionPreview {
  hasPersistentData: boolean
  targets: Array<{
    deploymentTargetId: string
    deploymentTargetName: string
    volumes: Array<{
      bindingId: string
      projectVolumeId: string
      displayName: string
      logicalName: string
      mountPath?: string
      devicePath?: string
      activationState: string
    }>
  }>
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
  runtimeStatus: string
  observationCode?: string
  observedAt?: string | null
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
  image?: string
  provisionAccess?: boolean
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
  observationCode: string
  observedAt?: string | null
  lastReportedAt?: string | null
  lastWindowStart?: string | null
  lastWindowEnd?: string | null
  lastError: string
}

export interface NotificationChannel {
  id: string
  projectId: string
  ownerUserId: string
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

export interface NotificationRuleAdvancedFilter {
  severities?: string[]
  applicationIds?: string[]
  deploymentTargetIds?: string[]
}

export type NotificationRuleFilter = NotificationRuleAdvancedFilter & (
  | { scope: 'projects', projectIds: string[] }
  | { scope: 'all', projectIds?: never }
)

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
  createdBy: string
  createdAt: string
  updatedAt: string
}

export interface NotificationDelivery {
  id: string
  projectId: string
  recipientUserId: string
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
  configTemplate: string
  jsonBodyTemplate: string
  secretFields: string[]
}

export interface MailSettings {
  host: string
  port: number
  security: 'none' | 'starttls' | 'tls'
  username: string
  passwordSet: boolean
  fromAddress: string
  fromName: string
  personalEmailCooldownSeconds: number
}

export interface MyNotificationPreferences {
  emailEnabled: boolean
  eventTypes: string[]
}

export type MyNotificationPreset = Pick<NotificationPreset, 'id' | 'name' | 'description' | 'secretFields'>

export interface MyNotificationChannelCreatePayload {
  name: string
  presetId: string
  secrets: Record<string, string>
  enabled: boolean
}

export interface MyNotificationChannelUpdatePayload {
  name: string
  secrets: Record<string, string>
  enabled: boolean
}

export interface MyNotificationChannel {
  id: string
  ownerUserId: string
  name: string
  adapterKind: 'webhook'
  config: Record<string, unknown>
  enabled: boolean
  secretSet: Record<string, boolean>
  lastDeliveryStatus: string
  lastDeliveryError: string
  lastDeliveredAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface PlatformEventEntityRef {
  id: string
  name: string
  identifier: string
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
  resourceOwnerUserId: string
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

export interface DashboardEntityRef {
  id: string
  name: string
  identifier: string
}

export interface DashboardActivity {
  id: string
  type: string
  category: string
  severity: string
  status: string
  message: string
  project?: DashboardEntityRef
  application?: DashboardEntityRef
  deploymentTarget?: DashboardEntityRef
  resourceType: string
  resourceId: string
  links: Record<string, string>
  occurredAt: string
}

export interface DashboardProjectShortcut {
  id: string
  name: string
  identifier: string
  description: string
  pinned: boolean
  applicationCount: number
  latestActivity?: DashboardActivity | null
}

export interface DashboardAttentionItem {
  key: string
  category: string
  severity: string
  occurrences: number
  latest: DashboardActivity
}

export interface DashboardReadinessItem {
  status: string
  available: number
  total: number
}

export interface DashboardOverview {
  generatedAt: string
  summary: {
    projects: number
    applications: number
    activeBuilds: number
    activeReleases: number
    attentionItems: number
    healthyClusters: number
    totalClusters: number
  }
  projects: DashboardProjectShortcut[]
  attention: DashboardAttentionItem[]
  activities: DashboardActivity[]
  readiness: {
    clusters: DashboardReadinessItem
    registries: DashboardReadinessItem
  }
}

export interface PlatformEventListParams extends PaginationParams {
  visibility?: ResultVisibility
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
  status: LiveObservationStatus
  observationCode?: string
  observedAt?: string | null
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
  webhookEnabled: boolean
  webhookStatus: LiveObservationStatus
  webhookObservationCode?: string
  webhookObservedAt?: string | null
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

export type RepositoryBindingPayload = Pick<RepositoryBinding, 'applicationId' | 'gitAccountId' | 'owner' | 'repo' | 'cloneUrl' | 'defaultBranch'> & {
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
  detectedFiles: string[]
  recommendedTemplateIds: string[]
  exposedPorts?: Record<string, number[]>
  strategy: string
  truncated: boolean
  durationMs: number
}

export interface BuildTemplateParameter {
  key: string
  type: 'select' | 'command' | 'path' | 'identifier' | 'port' | string
  required: boolean
  defaultValue: string
  options?: string[]
}

export interface BuildTemplate {
  id: string
  version: string
  runtime: string
  category: string
  defaultServicePort: number
  parameters: BuildTemplateParameter[]
}

export interface BuildTemplatePreview {
  templateId: string
  version: string
  values: Record<string, string>
  dockerfile: string
  checksum: string
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

export type BuildEnvironmentScope = 'global' | 'application' | 'deployment'

export interface BuildEnvironmentConfig {
  scope: BuildEnvironmentScope
  scopeRef: string
  variables: Record<string, string>
  secrets: Record<string, boolean>
}

export interface BuildEnvironmentConfigParams {
  scope: BuildEnvironmentScope
  projectId?: string
  applicationId?: string
  deploymentTargetId?: string
}

export interface BuildEnvironmentConfigPayload {
  variables: Record<string, string>
  secrets: Record<string, string>
}

export interface RuntimeEnvironmentVariableInput {
  key: string
  valueMode: 'public'
  value: string
}

export interface RuntimeEnvironmentVariable {
  key: string
  valueMode: 'public' | 'secret'
  value?: string
  configured: boolean
}

export interface ProjectRuntimeConfigSet {
  id: string
  projectId: string
  name: string
  environmentVariables: RuntimeEnvironmentVariable[]
  configFiles: string
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

export type ProjectRuntimeConfigSetPayload = Omit<ProjectRuntimeConfigSet, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'environmentVariables' | 'secretFilesSet' | 'deleteStatus' | 'deleteMessage'> & {
  environmentVariables: RuntimeEnvironmentVariableInput[]
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
  buildDefinitionMode: 'repository_dockerfile' | 'template'
  buildTemplateId: string
  buildTemplateVersion: string
  buildTemplateValues: string
  buildTemplateChecksum: string
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

export interface DeploymentDataVolumeInput {
  logicalName: string
  sourceType: 'projectVolume' | 'emptyDir'
  projectVolumeId?: string
  mountPath?: string
  devicePath?: string
  readOnly?: boolean
  emptyDir?: {
    medium?: '' | 'Memory'
    sizeLimit?: string
  }
}

export interface DeploymentDataVolume extends DeploymentDataVolumeInput {
  bindingId: string
  activationState: 'reserved' | 'active' | 'release_pending' | 'error' | string
  readOnly: boolean
}

export interface DeploymentTarget {
  id: string
  projectId: string
  applicationId: string
  environmentId: string
  name: string
  stage: string
  kubernetesName: string
  clusterId: string
  workloadType: 'Deployment' | 'StatefulSet' | string
  replicas: number
  cpuRequest: string
  memoryRequest: string
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
  capabilityDrop: string
  nodeSelector: string
  tolerations: string
  affinity: string
  topologySpreadConstraints: string
  priorityClassName: string
  serviceAnnotations: string
  serviceSessionAffinity: '' | 'None' | 'ClientIP' | string
  autoScalingEnabled: boolean
  autoScalingMinReplicas: number
  autoScalingMaxReplicas: number
  autoScalingCpuPercent: number
  autoScalingMemoryPercent: number
  autoScalingBehavior: string
  servicePorts: DeploymentServicePort[]
  sourceType: 'repository' | 'image'
  repositoryBindingId: string
  buildDefinitionMode: 'repository_dockerfile' | 'template'
  buildTemplateId: string
  buildTemplateVersion: string
  buildTemplateValues: string
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
  buildVariableSetIds: string[]
  buildHooksEnabled: boolean
  buildHookBindings: DeploymentTargetHookBinding[]
  autoDeploy: boolean
  branchPattern: string
  tagPattern: string
  concurrencyPolicy: 'queue' | 'parallel'
  runtimeConfigRefs: DeploymentRuntimeConfigRef[]
  environmentVariables: RuntimeEnvironmentVariable[]
  configFiles: string
  secretFilesSet: boolean
  dataVolumes: DeploymentDataVolume[]
  requireApproval: boolean
  webConsoleEnabled: boolean | null
  enabled: boolean
  status: string
  observationCode?: string
  lastCheckedAt?: string | null
  desiredReplicas: number
  updatedReplicas: number
  readyReplicas: number
  availableReplicas: number
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
  status: 'ready' | 'unavailable'
  reason?: string
  configuredReplicas: number
  desiredReplicas: number
  readyReplicas: number
  availableReplicas: number
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

export interface DeploymentTargetRuntimeSecretsSummary {
  environmentVariables: RuntimeEnvironmentVariable[]
}

export interface DeploymentTargetRuntimeSecretsPayload {
  items: Array<{
    key: string
    valueMode: 'secret'
    operation: 'set' | 'generate' | 'clear'
    value?: string
    generation?: { length?: number, encoding?: 'base64' | 'hex' | 'alphanumeric' | 'numeric' }
  }>
}

export interface RuntimeSecretMutationResponse {
  configuredKeys: string[]
  generatedKeys: string[]
  clearedKeys: string[]
  environmentVariables: RuntimeEnvironmentVariable[]
}

export type DeploymentTargetPayload = Omit<DeploymentTarget, 'id' | 'projectId' | 'applicationId' | 'kubernetesName' | 'createdBy' | 'createdAt' | 'buildVariableSetIds' | 'runtimeConfigRefs' | 'environmentVariables' | 'secretFilesSet' | 'dataVolumes' | 'status' | 'observationCode' | 'lastCheckedAt' | 'desiredReplicas' | 'updatedReplicas' | 'readyReplicas' | 'availableReplicas' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt'> & {
  buildVariableSetIds: string[]
  buildVariables?: Record<string, string>
  buildSecrets?: Record<string, string>
  runtimeConfigRefs: DeploymentRuntimeConfigRef[]
  environmentVariables: RuntimeEnvironmentVariableInput[]
  secretFiles?: string
  dataVolumes: DeploymentDataVolumeInput[]
}

export interface DeploymentBundleReferenceDescriptor {
  name?: string
  type?: string
  scope?: string
  owner?: string
  repository?: string
  namespace?: string
  mode?: string
  phase?: string
  runOrder?: number
  logicalName?: string
  mountPath?: string
  devicePath?: string
  readOnly?: boolean
  accessMode?: string
  volumeMode?: string
  storageClassName?: string
  clusterName?: string
  clusterType?: string
}

export interface DeploymentBundleReference {
  key: string
  kind: 'repositoryBinding' | 'runtimeCluster' | 'artifactRegistry' | 'buildVariableSet' | 'runtimeConfigSet' | 'hookConfig' | 'projectVolume'
  required: boolean
  usage: string
  source: DeploymentBundleReferenceDescriptor
}

export interface DeploymentBundleSecretRequirement {
  key: string
  target: 'build' | 'runtimeEnv' | 'runtimeFile'
  name?: string
  path?: string
}

export interface DeploymentTargetBundle {
  schemaVersion: 1
  kind: 'luna-devops.deployment-target'
  exportedAt: string
  configuration: DeploymentTargetPayload
  references: DeploymentBundleReference[]
  secretRequirements: DeploymentBundleSecretRequirement[]
  omissions: string[]
}

export interface DeploymentBundleReferenceCandidate {
  id: string
  name: string
  description?: string
  matched: boolean
  compatible: boolean
}

export interface DeploymentBundleReferenceCandidatePage extends PaginatedResponse<DeploymentBundleReferenceCandidate> {}

export interface DeploymentBundleReferenceCandidateListRequest {
  reference: DeploymentBundleReference
}

export interface DeploymentBundleReferenceResolution extends DeploymentBundleReference {
  status: 'resolved' | 'missing' | 'ambiguous' | 'forbidden' | 'incompatible'
  resolvedId?: string
  candidates: DeploymentBundleReferenceCandidate[]
  candidateCount: number
  truncated: boolean
  code?: string
}

export interface DeploymentTargetBundlePreviewRequest {
  bundle: DeploymentTargetBundle
  mappings?: Record<string, string>
  overrides?: { name?: string, stage?: string }
}

export interface DeploymentTargetBundleImportRequest extends DeploymentTargetBundlePreviewRequest {
  digest: string
  secretValues?: Record<string, string>
}

export interface DeploymentTargetBundlePreview {
  digest: string
  status: 'ready' | 'requires_mapping' | 'invalid'
  summary: { name: string, stage: string, sourceType: 'repository' | 'image' }
  references: DeploymentBundleReferenceResolution[]
  secretRequirements: DeploymentBundleSecretRequirement[]
  warnings: string[]
}

export type ApplicationPayload = Pick<Application, 'name' | 'identifier' | 'icon'>

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
  cpuRequestPercent: number
  memoryRequestPercent: number
  cpuLimitPercent: number
  memoryLimitPercent: number
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
  kubeGatewayEnabled?: boolean
  status: string
  deleteStatus?: 'active' | 'deleting' | 'delete_failed' | 'deleted' | string
  deleteStartedAt?: string | null
  deleteFinishedAt?: string | null
  deleteObservationCode?: string
  lastCheckedAt?: string | null
  createdBy: string
  createdAt: string
}

export type RuntimeClusterPressureLevel = 'idle' | 'light' | 'moderate' | 'heavy' | 'full' | 'unavailable'

export interface RuntimeClusterPressureResource {
  requests: number
  allocatable: number
  usage?: number
  requestPercent: number
  usagePercent?: number
}

export interface RuntimeClusterPressure {
  clusterId: string
  status: 'ready' | 'unavailable'
  pressureLevel: RuntimeClusterPressureLevel
  pressureScore?: number
  observationCode?: string
  observedAt: string
  details?: {
    cpu: RuntimeClusterPressureResource
    memory: RuntimeClusterPressureResource
    nodeCount: number
    podCount: number
    metricsAvailable: boolean
  }
}

export type RuntimeClusterResourceCategory = 'namespaces' | 'workloads' | 'services' | 'configs' | 'storage'

export type RuntimeClusterResourceKind
  = | 'Namespace'
    | 'Deployment'
    | 'StatefulSet'
    | 'Pod'
    | 'HorizontalPodAutoscaler'
    | 'Service'
    | 'HTTPRoute'
    | 'Gateway'
    | 'ConfigMap'
    | 'Secret'
    | 'PersistentVolumeClaim'

export interface ClusterResource {
  id: string
  kind: RuntimeClusterResourceKind
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
  resourceCategory: RuntimeClusterResourceCategory
  namespace?: string
  projectId?: string
  visibility?: ResultVisibility
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
  truncated: boolean
  durationMs: number
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

export interface KubeCredential {
  id: string
  name: string
  scopes: string[]
  status: 'active' | 'expired' | 'revoked' | string
  expiresAt: string
  createdAt: string
  bindingCount: number
}

export interface KubeCredentialBinding {
  id: string
  projectId: string
  runtimeClusterId: string
  applicationId?: string | null
  namespace: string
  contextName: string
  createdAt?: string
}

export interface CreateKubeCredentialContextInput {
  projectId: string
  runtimeClusterId: string
  applicationId?: string
}

export interface CreateKubeCredentialInput {
  name: string
  expiresInDays: 1 | 7 | 30
  scopes: string[]
  contexts: CreateKubeCredentialContextInput[]
}

export interface CreateKubeCredentialResponse {
  credential: KubeCredential
  bindings: KubeCredentialBinding[]
  kubeconfig: string
}

export type KubeGatewayVerb
  = | 'get'
    | 'list'
    | 'watch'
    | 'create'
    | 'update'
    | 'patch'
    | 'delete'
    | 'deletecollection'
    | 'connect'
    | string

export type KubeGatewayAction
  = | 'project:read'
    | 'deployment:read'
    | 'deployment:update'
    | 'deployment:restart'
    | 'deployment:delete'
    | 'deployment:exec'
    | 'secret:read_summary'
    | 'secret:view_value'
    | 'secret:update'
    | 'volume:read'
    | 'volume:write'
    | 'volume:delete'
    | 'gateway:read'
    | 'gateway:manage'
    | 'cluster:read'
    | 'cluster:manage'
    | string

export interface RuntimeClusterKubeGatewayRule {
  apiGroup: string
  apiVersion: string
  resource: string
  subresources: string[]
  verbs: KubeGatewayVerb[]
  action: KubeGatewayAction
}

export interface RuntimeClusterKubeGateway {
  enabled: boolean
  extraResourceRules: RuntimeClusterKubeGatewayRule[]
  status: 'disabled' | 'reconciling' | 'ready' | 'unavailable' | string
  observationCode: string
  lastCheckedAt?: string | null
}

export interface UpdateRuntimeClusterKubeGatewayInput {
  enabled: boolean
  extraResourceRules: RuntimeClusterKubeGatewayRule[]
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

export interface OAuthApplication {
  id: string
  ownerUserId?: string
  name: string
  description: string
  homepageUrl: string
  logoUrl: string
  clientId: string
  redirectUris: string[]
  allowedScopes: string
  accessTokenLifetimeDays: number
  revokedAt: string | null
  createdAt: string
  updatedAt: string
}

export interface OAuthApplicationInput {
  name: string
  description?: string
  homepageUrl?: string
  logoUrl?: string
  redirectUris: string[]
  allowedScopes: string
  accessTokenLifetimeDays?: number
}

export interface OAuthApplicationSecretResponse {
  application: OAuthApplication
  clientSecret: string
}

export interface OAuthGrant {
  id: string
  application: OAuthApplication
  scope: string
  createdAt: string
  updatedAt: string
}

export interface OAuthAuthorizationRequest {
  application: OAuthApplication
  scope: string
  accessTokenLifetimeDays: number
  previouslyAuthorized: boolean
}

export interface OAuthAuthorizationDecision {
  approved?: boolean
  clientId: string
  redirectUri: string
  scope: string
  state?: string
  codeChallenge: string
  codeChallengeMethod: string
}

export interface OAuthAuthorizationDecisionResponse {
  redirectUrl: string
}

export interface OAuthProtocolError {
  error: string
  error_description: string
  requestId: string
}

export interface OAuthDeviceVerification {
  application: OAuthApplication
  scope: string
  userCode: string
  expiresAt: string
}

export interface OAuthDeviceVerificationDecision {
  approved: boolean
  userCode: string
}

export interface OAuthDeviceVerificationResult {
  status: 'approved' | 'denied'
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
  projectIdentifier: string
  applicationId: string
  applicationName: string
  applicationIdentifier: string
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
  applicationIdentifier: string
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
  applicationIdentifier: string
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

export interface ApplicationListParams extends PaginationParams {
  includeRuntime?: boolean
}

export type ResultVisibility = 'related' | 'all'

export interface ProjectListParams extends PaginationParams {
  visibility?: ResultVisibility
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

export type InboxCategory = 'action' | 'project' | 'billing' | 'security' | 'delivery' | 'system'
export type InboxPriority = 'low' | 'normal' | 'high' | 'critical'
export type InboxFilter = 'all' | 'unread' | 'action'
export type InboxActionRequestStatus = 'pending' | 'processing' | 'completed' | 'rejected' | 'cancelled' | 'expired' | 'failed'
export type InboxDecision = 'accept' | 'reject'

export interface InboxActionRequestSummary {
  id: string
  type: string
  status: InboxActionRequestStatus
  rowVersion: number
  expiresAt?: string | null
  allowedDecisions: InboxDecision[]
}

export interface InboxActionRequest {
  id: string
  type: string
  requesterUserId: string
  recipientUserId: string
  projectId: string
  resourceType: string
  resourceId: string
  status: InboxActionRequestStatus
  rowVersion: number
  expiresAt?: string | null
  respondedAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface InboxMessage {
  id: string
  type: string
  category: InboxCategory
  priority: InboxPriority
  actorId: string
  projectId: string
  resourceType: string
  resourceId: string
  titleKey: string
  contentKey: string
  params: Record<string, unknown>
  actionRequestId: string
  actionRequest?: InboxActionRequestSummary | null
  deepLink: string
  groupKey: string
  readAt?: string | null
  expiresAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface InboxListParams extends PaginationParams {
  filter: InboxFilter
  category?: InboxCategory
}

export interface InboxUnreadCount {
  unreadCount: number
}

export interface CurrentUser {
  id: string
  email: string
  name: string
  avatarUrl: string
  passwordSet: boolean
  role: PlatformRoleValue
  language: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR'
  brandColorPreset: '' | 'aurora' | 'harbor' | 'sunset' | 'botanical' | 'meadow' | 'citrus' | 'gold' | 'orange' | 'red' | 'pink' | 'violet' | 'blue' | 'cyan' | 'teal' | 'green' | 'lime'
  interfaceStyle: '' | 'minimal' | 'themed'
  permissions: string[]
}

export interface User {
  id: string
  email: string
  name: string
  avatarUrl: string
  passwordSet: boolean
  role: PlatformRoleValue
  language: 'zh-CN' | 'zh-TW' | 'en-US' | 'ja-JP' | 'ko-KR'
  disabled: boolean
  balanceCredits: string
  createdAt: string
}

export interface AuthRegistrationStatus {
  emailRegistrationEnabled: boolean
  oidcRegistrationEnabled: boolean
  externalIdentityPasswordEnabled: boolean
}

export interface AuthRegistrationSettings {
  allowEmailRegistration: boolean
  allowOidcRegistration: boolean
  allowExternalIdentityPassword: boolean
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
  defaultRole: PlatformRoleValue
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
