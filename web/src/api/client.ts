import type { aiApi } from './domains/ai'
import type { applicationsApi } from './domains/applications'
import type { authApi } from './domains/auth'
import type { buildsApi } from './domains/builds'
import type { dashboardApi } from './domains/dashboard'
import type { eventsApi } from './domains/events'
import type { gatewayApi } from './domains/gateway'
import type { gitApi } from './domains/git'
import type { inboxApi } from './domains/inbox'
import type { metaApi } from './domains/meta'
import type { notificationsApi } from './domains/notifications'
import type { oauthApi } from './domains/oauth'
import type { projectsApi } from './domains/projects'
import type { registriesApi } from './domains/registries'
import type { runtimeApi } from './domains/runtime'
import type { topologyApi } from './domains/topology'
import type { volumesApi } from './domains/volumes'

type ApiClient = typeof aiApi
  & typeof applicationsApi
  & typeof authApi
  & typeof buildsApi
  & typeof dashboardApi
  & typeof eventsApi
  & typeof gatewayApi
  & typeof gitApi
  & typeof inboxApi
  & typeof metaApi
  & typeof notificationsApi
  & typeof oauthApi
  & typeof projectsApi
  & typeof registriesApi
  & typeof runtimeApi
  & typeof topologyApi
  & typeof volumesApi

const domainLoaders = {
  ai: () => import('./domains/ai').then(module => module.aiApi),
  applications: () => import('./domains/applications').then(module => module.applicationsApi),
  auth: () => import('./domains/auth').then(module => module.authApi),
  builds: () => import('./domains/builds').then(module => module.buildsApi),
  dashboard: () => import('./domains/dashboard').then(module => module.dashboardApi),
  events: () => import('./domains/events').then(module => module.eventsApi),
  gateway: () => import('./domains/gateway').then(module => module.gatewayApi),
  git: () => import('./domains/git').then(module => module.gitApi),
  inbox: () => import('./domains/inbox').then(module => module.inboxApi),
  meta: () => import('./domains/meta').then(module => module.metaApi),
  notifications: () => import('./domains/notifications').then(module => module.notificationsApi),
  oauth: () => import('./domains/oauth').then(module => module.oauthApi),
  projects: () => import('./domains/projects').then(module => module.projectsApi),
  registries: () => import('./domains/registries').then(module => module.registriesApi),
  runtime: () => import('./domains/runtime').then(module => module.runtimeApi),
  topology: () => import('./domains/topology').then(module => module.topologyApi),
  volumes: () => import('./domains/volumes').then(module => module.volumesApi),
} as const

type DomainName = keyof typeof domainLoaders
type ApiMethod = (...args: never[]) => unknown
type DomainApi = Record<string, ApiMethod>

const domainOperations = {
  ai: [
    'getAICapabilities',
    'listAIModels',
    'listAIModelConfigs',
    'createAIModel',
    'updateAIModel',
    'listAIConversations',
    'createAIConversation',
    'updateAIConversation',
    'renameAIConversation',
    'deleteAIConversation',
    'getAIConversationTimeline',
    'createAITurn',
    'executeAIToolAction',
    'listPendingAIUIActions',
    'acknowledgeAIUIAction',
    'cancelAIRun',
    'decideAIToolApproval',
    'resumeAIToolMFA',
    'submitAIRunInput',
  ],
  applications: [
    'listApplications',
    'listApplicationsPage',
    'getApplication',
    'getApplicationTopology',
    'createApplication',
    'updateApplication',
    'previewApplicationDeletion',
    'deleteApplication',
    'listDeploymentTargets',
    'listDeploymentTargetsPage',
    'createDeploymentTarget',
    'updateDeploymentTarget',
    'getDeploymentTargetRuntimeSecretsSummary',
    'updateDeploymentTargetRuntimeSecrets',
    'exportDeploymentTargetBundle',
    'previewDeploymentTargetBundleImport',
    'listDeploymentTargetBundleReferenceCandidates',
    'importDeploymentTargetBundle',
    'restartDeploymentTarget',
    'deleteDeploymentTarget',
    'listRepositoryBindings',
    'listRepositoryBindingsPage',
    'createRepositoryBinding',
    'updateRepositoryBinding',
    'deleteRepositoryBinding',
    'createRepositoryWebhook',
    'reconfigureRepositoryWebhook',
  ],
  auth: [
    'getPublicConfigs',
    'getBootstrapStatus',
    'initializeAdmin',
    'login',
    'resumeLogin',
    'logout',
    'getAuthRegistrationStatus',
    'getAuthRegistrationSettings',
    'updateAuthRegistrationSettings',
    'requestEmailRegistrationCode',
    'completeEmailRegistration',
    'getMFAStatus',
    'enrollMFA',
    'confirmMFAEnrollment',
    'verifyMFA',
    'regenerateMFARecoveryCodes',
    'disableMFA',
    'getOIDCCallbackConfig',
    'listAuthProviders',
    'createAuthProvider',
    'updateAuthProvider',
    'getAuthAdmissionPolicy',
    'updateAuthAdmissionPolicy',
    'getCurrentUser',
    'updateCurrentUser',
    'updateMyPassword',
    'listMyExternalIdentities',
    'unbindMyExternalIdentity',
    'listUsers',
    'createUser',
    'updateUser',
    'resetUserMFA',
    'listConfigDefinitions',
    'getConfigs',
    'updateConfigs',
    'testAgentObservabilitySource',
    'getAgentObservabilityOverview',
    'listAgentObservabilityConversations',
    'listAgentObservabilityTurns',
    'getAgentObservabilityConversation',
    'getAgentObservabilityTrace',
    'getDataRetentionCatalog',
    'previewDataRetention',
    'cleanupDataRetention',
  ],
  builds: [
    'getBuildEnvironmentConfig',
    'updateBuildEnvironmentConfig',
    'listBuildTemplates',
    'previewBuildTemplate',
    'listBuildVariableSets',
    'listBuildVariableSetsPage',
    'createBuildVariableSet',
    'updateBuildVariableSet',
    'deleteBuildVariableSet',
    'listProjectRuntimeConfigSets',
    'listProjectRuntimeConfigSetsPage',
    'createProjectRuntimeConfigSet',
    'updateProjectRuntimeConfigSet',
    'updateProjectRuntimeConfigSetRuntimeSecrets',
    'deleteProjectRuntimeConfigSet',
    'listProjectHooks',
    'listProjectHooksPage',
    'createProjectHook',
    'updateProjectHook',
    'deleteProjectHook',
    'listProjectHookRuns',
    'getProjectHookRunLogs',
    'listBuildRuns',
    'listBuildRunsPage',
    'triggerBuildRun',
    'retryBuildRun',
    'cancelBuildRun',
    'deleteBuildRun',
    'listBuildJobs',
    'listBuildJobsPage',
    'getBuildJobLogs',
  ],
  dashboard: ['getDashboard'],
  events: ['listPlatformEvents', 'getPlatformEvent', 'listPlatformEventCatalog'],
  gateway: [
    'listGatewayRoutes',
    'listGatewayRoutesPage',
    'createGatewayRoute',
    'updateGatewayRoute',
    'deleteGatewayRoute',
    'checkGatewayDomain',
    'listAccessTokens',
    'listAccessTokenScopes',
    'createAccessToken',
    'revokeAccessToken',
  ],
  git: [
    'listGitProviders',
    'listGitProvidersPage',
    'createGitProvider',
    'updateGitProvider',
    'deleteGitProvider',
    'listGitAccounts',
    'listGitAccountsPage',
    'createGitAccount',
    'updateGitAccount',
    'deleteGitAccount',
    'refreshGitAccount',
    'listGitRepositories',
    'listGitBranches',
    'readGitFile',
    'listGitContents',
    'getGitRepositoryBuildOptions',
  ],
  inbox: [
    'listInboxMessages',
    'getInboxMessage',
    'getInboxUnreadCount',
    'markInboxMessageRead',
    'markAllInboxMessagesRead',
    'archiveInboxMessage',
    'decideInboxActionRequest',
  ],
  meta: ['getAPIMeta'],
  notifications: [
    'listNotificationPresets',
    'createNotificationChannelFromPreset',
    'listNotificationChannels',
    'createNotificationChannel',
    'updateNotificationChannel',
    'deleteNotificationChannel',
    'testNotificationChannel',
    'listNotificationTemplates',
    'createNotificationTemplate',
    'updateNotificationTemplate',
    'deleteNotificationTemplate',
    'listNotificationRules',
    'createNotificationRule',
    'updateNotificationRule',
    'deleteNotificationRule',
    'listNotificationDeliveries',
  ],
  oauth: [
    'listOAuthApplications',
    'createOAuthApplication',
    'updateOAuthApplication',
    'rotateOAuthApplicationSecret',
    'deleteOAuthApplication',
    'listMyOAuthGrants',
    'revokeMyOAuthGrant',
    'getOAuthAuthorizationRequest',
    'decideOAuthAuthorization',
    'getOAuthDeviceVerification',
    'decideOAuthDeviceVerification',
  ],
  projects: [
    'listProjects',
    'listProjectsPage',
    'listAppTemplates',
    'installAppTemplate',
    'listSystemComponents',
    'installSystemAppTemplate',
    'getBillingSummary',
    'getGatewayTrafficStatus',
    'listBillingDeploymentSpend',
    'listBillingLedgerEntries',
    'listBillingUsageRecords',
    'listBillingRateRules',
    'updateBillingRateRules',
    'createBillingWalletTransaction',
    'createGatewayTrafficUsage',
    'listProjectPins',
    'updateProjectOrder',
    'createProject',
    'getProject',
    'updateProject',
    'deleteProject',
    'pinProject',
    'unpinProject',
    'createBillingOwnerTransferRequest',
    'listProjectMembers',
    'listProjectMembersPage',
    'searchProjectMemberCandidates',
    'createProjectMember',
    'updateProjectMember',
    'deleteProjectMember',
  ],
  registries: [
    'listRegistries',
    'listRegistriesPage',
    'createRegistry',
    'updateRegistry',
    'deleteRegistry',
    'testRegistry',
    'getRegistryImageTemplateDefault',
    'getDefaultRegistry',
    'listRegistryCredentials',
    'listRegistryCredentialsPage',
    'listAllRegistryCredentialsPage',
    'createRegistryCredential',
    'updateRegistryCredential',
    'deleteRegistryCredential',
    'searchRegistryRepositories',
    'listRegistryRepositoryTags',
    'listContainerImages',
    'createContainerImage',
  ],
  runtime: [
    'listRuntimeClusters',
    'listRuntimeClustersPage',
    'createRuntimeCluster',
    'updateRuntimeCluster',
    'deleteRuntimeCluster',
    'testRuntimeCluster',
    'listRuntimeClusterResources',
    'listRuntimeClusterResourcesPage',
    'listRuntimeClusterResourceEvents',
    'getRuntimeClusterResourceYAML',
    'deleteRuntimeClusterResource',
    'authorizeRuntimeClusterPodTerminal',
    'listReleases',
    'listReleasesPage',
    'listReleaseImageCandidates',
    'createRelease',
    'getReleaseLogs',
    'getReleaseRuntimeLogs',
    'execReleaseRuntimeCommand',
    'authorizeReleaseRuntimeTerminal',
    'rollbackRelease',
  ],
  topology: [
    'getProjectTopology',
    'listServiceBindings',
    'createServiceBinding',
    'updateServiceBinding',
    'deleteServiceBinding',
    'checkServiceBinding',
    'listProjectTopologyEdges',
    'createProjectTopologyEdge',
    'updateProjectTopologyEdge',
    'deleteProjectTopologyEdge',
  ],
  volumes: [
    'listProjectVolumes',
    'createProjectVolume',
    'listProjectVolumeStorageClasses',
    'getProjectVolume',
    'updateProjectVolume',
    'previewProjectVolumeDeletion',
    'deleteProjectVolume',
    'retryProjectVolumeOperation',
    'createVolumeImport',
    'getVolumeImportUploadOffset',
    'uploadVolumeImportChunk',
    'completeVolumeImportUpload',
    'createVolumeExport',
    'listVolumeTransfers',
    'getVolumeTransfer',
    'retryVolumeTransfer',
    'cancelVolumeTransfer',
    'authorizeVolumeTransferDownload',
    'volumeTransferContentURL',
    'volumeTransferManifestURL',
    'headVolumeTransferContent',
    'headVolumeTransferManifest',
    'downloadVolumeTransferManifest',
    'downloadVolumeTransferContent',
  ],
} satisfies Record<DomainName, readonly (keyof ApiClient)[]>

type RegisteredOperation = (typeof domainOperations)[DomainName][number]
const allOperationsRegistered: Exclude<keyof ApiClient, RegisteredOperation> extends never ? true : never = true
void allOperationsRegistered

const operationDomains = new Map<string, DomainName>()
for (const [domain, operations] of Object.entries(domainOperations) as [DomainName, readonly string[]][]) {
  for (const operation of operations)
    operationDomains.set(operation, domain)
}

const loadedDomains = new Map<DomainName, Promise<DomainApi>>()
const methodCache = new Map<string, ApiMethod>()

function loadDomain(domain: DomainName) {
  const existing = loadedDomains.get(domain)
  if (existing)
    return existing
  const pending = domainLoaders[domain]() as Promise<DomainApi>
  loadedDomains.set(domain, pending)
  return pending
}

function lazyMethod(operation: string): ApiMethod {
  const cached = methodCache.get(operation)
  if (cached)
    return cached
  const domain = operationDomains.get(operation)
  if (!domain)
    throw new Error(`Unknown API operation: ${operation}`)
  const method: ApiMethod = (...args) => loadDomain(domain).then(api => api[operation](...args))
  methodCache.set(operation, method)
  return method
}

/**
 * 保持既有 `api.operation()` 调用形式，同时让每个业务 domain 在首次调用时才加载。
 * 这避免公共 barrel 把所有 API domain 与其依赖合并进首屏共享 chunk。
 */
export const api = new Proxy({} as ApiClient, {
  get(target, property, receiver) {
    if (Reflect.has(target, property))
      return Reflect.get(target, property, receiver)
    if (typeof property !== 'string')
      return undefined
    return lazyMethod(property)
  },
})
