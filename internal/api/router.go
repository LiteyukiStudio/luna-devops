package api

import (
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/observability"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, cfg Config) *gin.Engine {
	return NewRouterWithStaticFSAndMetricsConfig(db, nil, nil, cfg)
}

func NewRouterWithStaticFSAndMetricsConfig(db *gorm.DB, staticFS fs.FS, httpMetrics *observability.HTTPMetrics, cfg Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	if debugLogEnabled(cfg) {
		debugLogWithConfig(cfg, "api log level set to debug")
	}
	router := gin.New()
	router.HandleMethodNotAllowed = true
	configureTrustedProxies(router, cfg.TrustedProxyCIDRs)
	middlewares := []gin.HandlerFunc{
		transportapi.RuntimeModeMiddleware(cfg.Mode),
		telemetry.QueryTraceContextMiddleware(),
		transportapi.RequestIDMiddleware(),
		telemetry.GinTracingMiddleware("luna-devops-api"),
		telemetry.GinAccessLogMiddleware(),
		transportapi.RecoveryMiddleware(),
		transportapi.ErrorResponseMiddleware(),
		securityHeaders(cfg),
		cors(cfg),
		csrfOriginGuard(cfg),
	}
	if httpMetrics != nil {
		middlewares = append(middlewares, httpMetrics.GinMiddleware())
	}
	router.Use(middlewares...)

	handlers := NewHandlersWithConfig(db, cfg)
	domains := handlers.domains
	registerStaticUI(router, staticFS, func() string {
		return handlers.configs.get([]string{siteBrandColorPresetKey})[siteBrandColorPresetKey]
	})

	router.GET("/healthz", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/.well-known/oauth-authorization-server", domains.identity.GetOAuthAuthorizationServerMetadata)
	router.GET("/internal/v1/ai/provider-config", domains.ai.GetAIProviderConfigInternal)

	v1 := router.Group("/api/v1", domains.ai.AIToolExecutionIdentityMiddleware())
	{
		v1.GET("/meta", domains.platform.GetAPIMeta)
		v1.POST("/public/configs", handlers.GetPublicConfigs)
		v1.POST("/auth/login", domains.identity.Login)
		v1.POST("/auth/login/resume", domains.identity.ResumeLogin)
		v1.POST("/auth/logout", domains.identity.Logout)
		v1.GET("/auth/registration", domains.identity.GetAuthRegistrationStatus)
		v1.POST("/auth/registration/email/code", domains.identity.RequestEmailRegistrationCode)
		v1.POST("/auth/registration/email", domains.identity.CompleteEmailRegistration)
		v1.GET("/auth/registration/settings", domains.identity.GetAuthRegistrationSettings)
		v1.PUT("/auth/registration/settings", domains.identity.PlatformAdminMiddleware(), domains.identity.UpdateAuthRegistrationSettings)
		v1.GET("/mail/settings", domains.identity.PlatformAdminMiddleware(), domains.notification.GetPlatformMailSettings)
		v1.PUT("/mail/settings", domains.identity.PlatformAdminMiddleware(), domains.notification.UpdatePlatformMailSettings)
		v1.POST("/mail/settings/test", domains.identity.PlatformAdminMiddleware(), domains.notification.TestPlatformMailSettings)
		v1.GET("/auth/providers", domains.identity.ListAuthProviders)
		v1.GET("/auth/oidc/callback-url", domains.identity.GetOIDCCallbackURL)
		v1.POST("/auth/providers", domains.identity.PlatformAdminMiddleware(), domains.identity.CreateAuthProvider)
		v1.PUT("/auth/providers/:providerId", domains.identity.PlatformAdminMiddleware(), domains.identity.UpdateAuthProvider)
		v1.GET("/auth/admission-policy", domains.identity.GetAuthAdmissionPolicy)
		v1.PUT("/auth/admission-policy", domains.identity.UpdateAuthAdmissionPolicy)
		v1.GET("/auth/oidc/:providerId/start", domains.identity.StartOIDC)
		v1.GET("/auth/oidc/callback", domains.identity.CompleteOIDC)
		v1.GET("/users/me", domains.identity.GetCurrentUser)
		v1.PUT("/users/me", domains.identity.UpdateCurrentUser)
		v1.PUT("/users/me/password", domains.identity.UpdateMyPassword)
		v1.GET("/me/notification-preferences", domains.notification.GetMyNotificationPreferences)
		v1.PUT("/me/notification-preferences", domains.notification.UpdateMyNotificationPreferences)
		v1.GET("/me/notification-presets", domains.notification.ListMyNotificationPresets)
		v1.GET("/me/notification-channels", domains.notification.ListMyNotificationChannels)
		v1.POST("/me/notification-channels", domains.notification.CreateMyNotificationChannel)
		v1.PUT("/me/notification-channels/:channelId", domains.notification.UpdateMyNotificationChannel)
		v1.DELETE("/me/notification-channels/:channelId", domains.notification.DeleteMyNotificationChannel)
		v1.POST("/me/notification-channels/:channelId/test", domains.notification.TestMyNotificationChannel)
		v1.GET("/me/notification-deliveries", domains.notification.ListMyNotificationDeliveries)
		v1.GET("/inbox", domains.notification.ListInboxMessages)
		v1.GET("/inbox/unread-count", domains.notification.GetInboxUnreadCount)
		v1.GET("/inbox/stream", domains.notification.StreamInboxChanges)
		v1.GET("/inbox/:messageId", domains.notification.GetInboxMessage)
		v1.POST("/inbox/:messageId/read", domains.notification.MarkInboxMessageRead)
		v1.POST("/inbox/read-all", domains.notification.MarkAllInboxMessagesRead)
		v1.POST("/inbox/:messageId/archive", domains.notification.ArchiveInboxMessage)
		v1.POST("/inbox/action-requests/:requestId/decision", domains.notification.DecideInboxActionRequest)
		v1.POST("/telemetry/v1/traces", domains.platform.RelayBrowserTraces)
		v1.GET("/ai/capabilities", domains.ai.GetAICapabilities)
		v1.GET("/ai/models", domains.ai.ListAIModels)
		v1.POST("/ai-tools/web-search", domains.ai.ExecuteAIWebSearch)
		v1.POST("/ai-tools/fetch-web-page", domains.ai.ExecuteAIFetchWebPage)
		v1.GET("/configs/ai/models", domains.identity.PlatformAdminMiddleware(), domains.ai.ListAIModelConfigs)
		v1.POST("/configs/ai/models", domains.identity.PlatformAdminMiddleware(), domains.ai.CreateAIModel)
		v1.PUT("/configs/ai/models/:id", domains.identity.PlatformAdminMiddleware(), domains.ai.UpdateAIModel)
		v1.DELETE("/configs/ai/models/:id", domains.identity.PlatformAdminMiddleware(), domains.ai.DeleteAIModel)
		v1.POST("/configs/ai/provider/test", domains.identity.PlatformAdminMiddleware(), domains.ai.TestAIProviderConnection)
		v1.POST("/configs/ai/observability/test", domains.identity.PlatformAdminMiddleware(), domains.ai.TestAgentObservabilitySource)
		v1.GET("/ai/observability/overview", domains.identity.PlatformAdminMiddleware(), domains.ai.GetAgentObservabilityOverview)
		v1.GET("/ai/observability/conversations", domains.identity.PlatformAdminMiddleware(), domains.ai.ListAgentObservabilityConversations)
		v1.GET("/ai/observability/turns", domains.identity.PlatformAdminMiddleware(), domains.ai.ListAgentObservabilityTurns)
		v1.GET("/ai/observability/tools", domains.identity.PlatformAdminMiddleware(), domains.ai.ListAgentObservabilityTools)
		v1.GET("/ai/observability/tools/:operationId/calls", domains.identity.PlatformAdminMiddleware(), domains.ai.ListAgentObservabilityToolCalls)
		v1.GET("/ai/observability/conversations/:conversationId", domains.identity.PlatformAdminMiddleware(), domains.ai.GetAgentObservabilityConversation)
		v1.GET("/ai/observability/traces/:traceId", domains.identity.PlatformAdminMiddleware(), domains.ai.GetAgentObservabilityTrace)
		v1.GET("/ai/conversations", domains.ai.ProxyAIRequest)
		v1.POST("/ai/conversations", domains.ai.ProxyAIRequest)
		v1.GET("/ai/conversations/:conversationId", domains.ai.ProxyAIRequest)
		v1.PATCH("/ai/conversations/:conversationId", domains.ai.ProxyAIRequest)
		v1.DELETE("/ai/conversations/:conversationId", domains.ai.ProxyAIRequest)
		v1.GET("/ai/conversations/:conversationId/timeline", domains.ai.ProxyAIRequest)
		v1.POST("/ai/conversations/:conversationId/turns", domains.ai.ProxyAIRequest)
		v1.POST("/ai/conversations/:conversationId/tool-actions", domains.ai.ProxyAIRequest)
		v1.GET("/ai/turns/:turnId/runs", domains.ai.ProxyAIRequest)
		v1.POST("/ai/turns/:turnId/runs", domains.ai.ProxyAIRequest)
		v1.GET("/ai/runs/:runId", domains.ai.ProxyAIRequest)
		v1.GET("/ai/runs/:runId/events", domains.ai.ProxyAIRequest)
		v1.POST("/ai/runs/:runId/cancel", domains.ai.ProxyAIRequest)
		v1.POST("/ai/runs/:runId/input", domains.ai.ProxyAIRequest)
		v1.GET("/ai/progress/projects/:projectId/:operationType/:operationId", domains.ai.GetAIProgress)
		v1.GET("/ai/progress/projects/:projectId/:operationType/:operationId/stream", domains.ai.StreamAIProgress)
		v1.POST("/ai/runs/:runId/approvals/:toolCallId/decision", domains.ai.ProxyAIRequest)
		v1.GET("/users/me/external-identities", domains.identity.ListMyExternalIdentities)
		v1.DELETE("/users/me/external-identities/:identityId", domains.identity.UnbindMyExternalIdentity)
		v1.GET("/oauth/applications", domains.identity.ListOAuthApplications)
		v1.POST("/oauth/applications", domains.identity.CreateOAuthApplication)
		v1.PUT("/oauth/applications/:applicationId", domains.identity.UpdateOAuthApplication)
		v1.POST("/oauth/applications/:applicationId/rotate-secret", domains.identity.RotateOAuthApplicationSecret)
		v1.DELETE("/oauth/applications/:applicationId", domains.identity.DeleteOAuthApplication)
		v1.GET("/oauth/grants", domains.identity.ListMyOAuthGrants)
		v1.DELETE("/oauth/grants/:grantId", domains.identity.RevokeMyOAuthGrant)
		v1.GET("/oauth/authorize", domains.identity.GetOAuthAuthorizationRequest)
		v1.POST("/oauth/authorize", domains.identity.DecideOAuthAuthorization)
		v1.POST("/oauth/device/authorization", domains.identity.StartOAuthDeviceAuthorization)
		v1.GET("/oauth/device/verification", domains.identity.GetOAuthDeviceVerification)
		v1.POST("/oauth/device/verification", domains.identity.DecideOAuthDeviceVerification)
		v1.POST("/oauth/token", domains.identity.ExchangeOAuthToken)
		v1.POST("/oauth/revoke", domains.identity.RevokeOAuthToken)
		v1.GET("/users", domains.identity.ListUsers)
		v1.POST("/users", domains.identity.PlatformAdminMiddleware(), domains.identity.CreateUser)
		v1.PUT("/users/:userId", domains.identity.PlatformAdminMiddleware(), domains.identity.UpdateUser)
		v1.GET("/configs/definitions", handlers.ListConfigDefinitions)
		v1.GET("/configs", handlers.GetConfigs)
		v1.PUT("/configs", handlers.UpdateConfigs)
		v1.GET("/data-retention/catalog", domains.platform.ListDataRetentionCatalog)
		v1.POST("/data-retention/preview", domains.platform.PreviewDataRetention)
		v1.POST("/data-retention/cleanup", domains.identity.PlatformAdminMiddleware(), domains.platform.CleanupDataRetention)

		v1.GET("/git/providers", domains.git.ListGitProviders)
		v1.POST("/git/providers", domains.git.CreateGitProvider)
		v1.PUT("/git/providers/:providerId", domains.git.UpdateGitProvider)
		v1.DELETE("/git/providers/:providerId", domains.git.DeleteGitProvider)
		v1.GET("/git/providers/:providerId/oauth/start", domains.git.StartGitOAuth)
		v1.GET("/git/oauth/callback", domains.git.CompleteGitOAuth)
		v1.POST("/git/webhooks/:bindingId", domains.git.ReceiveGitWebhook)
		v1.GET("/git/accounts", domains.git.ListGitAccounts)
		v1.POST("/git/accounts", domains.git.CreateGitAccount)
		v1.PUT("/git/accounts/:accountId", domains.git.UpdateGitAccount)
		v1.DELETE("/git/accounts/:accountId", domains.git.DeleteGitAccount)
		v1.POST("/git/accounts/:accountId/refresh", domains.git.RefreshGitAccount)
		v1.GET("/git/accounts/:accountId/repositories", domains.git.ListGitRepositories)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/branches", domains.git.ListGitBranches)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/build-options", domains.git.GetGitRepositoryBuildOptions)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/contents", domains.git.ListGitContents)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/file", domains.git.ReadGitFile)

		v1.GET("/registries", domains.registry.ListArtifactRegistries)
		v1.POST("/registries", domains.registry.CreateArtifactRegistry)
		v1.PUT("/registries/:registryId", domains.registry.UpdateArtifactRegistry)
		v1.DELETE("/registries/:registryId", domains.registry.DeleteArtifactRegistry)
		v1.POST("/registries/:registryId/test", domains.registry.TestArtifactRegistry)
		v1.GET("/registries/:registryId/image-template-default", domains.registry.GetRegistryImageTemplateDefault)
		v1.GET("/registries/:registryId/repositories/search", domains.registry.SearchRegistryRepositories)
		v1.GET("/registries/:registryId/repository-tags", domains.registry.ListRegistryRepositoryTags)
		v1.GET("/registry-credentials", domains.registry.ListAllRegistryCredentials)
		v1.GET("/registries/:registryId/credentials", domains.registry.ListRegistryCredentials)
		v1.POST("/registries/:registryId/credentials", domains.registry.CreateRegistryCredential)
		v1.PUT("/registries/:registryId/credentials/:credentialId", domains.registry.UpdateRegistryCredential)
		v1.DELETE("/registries/:registryId/credentials/:credentialId", domains.registry.DeleteRegistryCredential)
		v1.GET("/container-images", domains.registry.ListContainerImages)
		v1.POST("/container-images", domains.registry.CreateContainerImage)

		v1.GET("/build/variable-sets", domains.build.ListBuildVariableSets)
		v1.GET("/build/environment-config", domains.build.GetBuildEnvironmentConfig)
		v1.PUT("/build/environment-config", domains.build.UpdateBuildEnvironmentConfig)
		v1.GET("/build/templates", domains.build.ListBuildTemplates)
		v1.POST("/build/templates/:templateId/preview", domains.build.PreviewBuildTemplate)
		v1.POST("/build/variable-sets", domains.build.CreateBuildVariableSet)
		v1.PUT("/build/variable-sets/:setId", domains.build.UpdateBuildVariableSet)
		v1.DELETE("/build/variable-sets/:setId", domains.build.DeleteBuildVariableSet)

		v1.GET("/runtime/clusters", domains.runtime.ListRuntimeClusters)
		v1.GET("/runtime/clusters/pressure", domains.runtime.ObserveRuntimeClusterPressure)
		v1.POST("/runtime/clusters", domains.runtime.CreateRuntimeCluster)
		v1.PUT("/runtime/clusters/:clusterId", domains.runtime.UpdateRuntimeCluster)
		v1.DELETE("/runtime/clusters/:clusterId", domains.runtime.DeleteRuntimeCluster)
		v1.POST("/runtime/clusters/:clusterId/test", domains.runtime.TestRuntimeCluster)
		v1.GET("/runtime/clusters/:clusterId/resources", domains.runtime.ListRuntimeClusterResources)
		v1.DELETE("/runtime/clusters/:clusterId/resources", domains.runtime.DeleteRuntimeClusterResource)
		v1.GET("/runtime/clusters/:clusterId/resource-yaml", domains.runtime.GetRuntimeClusterResourceYAML)
		v1.GET("/runtime/clusters/:clusterId/resource-events", domains.runtime.ListRuntimeClusterResourceEvents)
		v1.POST("/runtime/clusters/:clusterId/pods/terminal/authorize", domains.runtime.AuthorizeRuntimeClusterPodTerminal)
		v1.GET("/runtime/clusters/:clusterId/pods/terminal", domains.runtime.StreamRuntimeClusterPodTerminal)
		v1.GET("/system-components", domains.runtime.ListSystemComponents)
		v1.POST("/app-templates/:templateId/system-install", domains.runtime.InstallSystemAppTemplate)
		v1.GET("/notifications/presets", domains.notification.ListNotificationPresets)
		v1.POST("/notifications/presets/:presetId/channels", domains.notification.CreateNotificationChannelFromPreset)
		v1.GET("/notifications/channels", domains.notification.ListNotificationChannels)
		v1.POST("/notifications/channels", domains.notification.CreateNotificationChannel)
		v1.PUT("/notifications/channels/:channelId", domains.notification.UpdateNotificationChannel)
		v1.DELETE("/notifications/channels/:channelId", domains.notification.DeleteNotificationChannel)
		v1.POST("/notifications/channels/:channelId/test", domains.notification.TestNotificationChannel)
		v1.GET("/notifications/templates", domains.notification.ListNotificationTemplates)
		v1.POST("/notifications/templates", domains.notification.CreateNotificationTemplate)
		v1.PUT("/notifications/templates/:templateId", domains.notification.UpdateNotificationTemplate)
		v1.DELETE("/notifications/templates/:templateId", domains.notification.DeleteNotificationTemplate)
		v1.GET("/notifications/rules", domains.notification.ListNotificationRules)
		v1.POST("/notifications/rules", domains.notification.CreateNotificationRule)
		v1.PUT("/notifications/rules/:ruleId", domains.notification.UpdateNotificationRule)
		v1.DELETE("/notifications/rules/:ruleId", domains.notification.DeleteNotificationRule)
		v1.GET("/notifications/deliveries", domains.notification.ListNotificationDeliveries)
		v1.GET("/events", domains.platform.ListPlatformEvents)
		v1.GET("/events/catalog", domains.platform.ListPlatformEventCatalog)
		v1.GET("/events/:eventId", domains.platform.GetPlatformEvent)
		v1.GET("/dashboard", domains.platform.GetDashboard)

		v1.GET("/app-templates", domains.application.ListAppTemplates)
		v1.GET("/app-templates/:templateId", domains.application.GetAppTemplate)

		v1.GET("/projects", domains.project.ListProjects)
		v1.GET("/projects/pins", domains.project.ListProjectPins)
		v1.PUT("/projects/order", domains.project.UpdateProjectOrder)
		v1.POST("/projects", domains.project.CreateProject)
		v1.POST("/projects/:projectId/billing-owner-transfer-requests", domains.project.CreateBillingOwnerTransferRequest)
		v1.GET("/projects/:projectId", domains.project.GetProject)
		v1.PUT("/projects/:projectId", domains.project.UpdateProject)
		v1.DELETE("/projects/:projectId", domains.project.DeleteProject)
		v1.GET("/projects/:projectId/runtime-config-sets", domains.runtime.ListProjectRuntimeConfigSets)
		v1.POST("/projects/:projectId/runtime-config-sets", domains.runtime.CreateProjectRuntimeConfigSet)
		v1.PUT("/projects/:projectId/runtime-config-sets/:setId", domains.runtime.UpdateProjectRuntimeConfigSet)
		v1.PUT("/projects/:projectId/runtime-config-sets/:setId/runtime-secrets", domains.runtime.UpdateProjectRuntimeConfigSetRuntimeSecrets)
		v1.DELETE("/projects/:projectId/runtime-config-sets/:setId", domains.runtime.DeleteProjectRuntimeConfigSet)
		v1.PUT("/projects/:projectId/pin", domains.project.PinProject)
		v1.DELETE("/projects/:projectId/pin", domains.project.UnpinProject)
		v1.GET("/projects/:projectId/registries/default", domains.registry.GetDefaultArtifactRegistry)
		v1.GET("/projects/:projectId/hooks", domains.project.ListProjectHookConfigs)
		v1.POST("/projects/:projectId/hooks", domains.project.CreateProjectHookConfig)
		v1.PUT("/projects/:projectId/hooks/:hookId", domains.project.UpdateProjectHookConfig)
		v1.DELETE("/projects/:projectId/hooks/:hookId", domains.project.DeleteProjectHookConfig)
		v1.GET("/projects/:projectId/topology", domains.project.GetProjectTopology)
		v1.GET("/projects/:projectId/service-bindings", domains.application.ListServiceBindings)
		v1.POST("/projects/:projectId/service-bindings", domains.application.CreateServiceBinding)
		v1.PUT("/projects/:projectId/service-bindings/:bindingId", domains.application.UpdateServiceBinding)
		v1.DELETE("/projects/:projectId/service-bindings/:bindingId", domains.application.DeleteServiceBinding)
		v1.POST("/projects/:projectId/service-bindings/:bindingId/check", domains.application.CheckServiceBinding)
		v1.GET("/projects/:projectId/topology-edges", domains.application.ListProjectTopologyEdges)
		v1.POST("/projects/:projectId/topology-edges", domains.application.CreateProjectTopologyEdge)
		v1.PUT("/projects/:projectId/topology-edges/:edgeId", domains.application.UpdateProjectTopologyEdge)
		v1.DELETE("/projects/:projectId/topology-edges/:edgeId", domains.application.DeleteProjectTopologyEdge)
		v1.GET("/projects/:projectId/hook-runs", domains.project.ListProjectHookRuns)
		v1.GET("/projects/:projectId/hook-runs/:runId/logs", domains.project.GetProjectHookRunLog)
		v1.POST("/projects/:projectId/app-templates/:templateId/install", domains.application.InstallAppTemplate)
		v1.GET("/projects/:projectId/volumes", domains.volume.ListProjectVolumes)
		v1.POST("/projects/:projectId/volumes", domains.volume.CreateProjectVolume)
		v1.GET("/projects/:projectId/volume-storage-classes", domains.volume.ListProjectVolumeStorageClasses)
		v1.GET("/projects/:projectId/volumes/:volumeId", domains.volume.GetProjectVolume)
		v1.PATCH("/projects/:projectId/volumes/:volumeId", domains.volume.UpdateProjectVolume)
		v1.DELETE("/projects/:projectId/volumes/:volumeId", domains.volume.DeleteProjectVolume)
		v1.POST("/projects/:projectId/volumes/:volumeId/retry", domains.volume.RetryProjectVolumeOperation)
		v1.POST("/projects/:projectId/volumes/:volumeId/deletion-preview", domains.volume.PreviewProjectVolumeDeletion)
		v1.POST("/projects/:projectId/volume-imports", domains.volume.CreateVolumeImport)
		v1.PUT("/projects/:projectId/volume-imports/:transferId/content", domains.volume.UploadVolumeImportContent)
		v1.POST("/projects/:projectId/volumes/:volumeId/exports", domains.volume.CreateVolumeExport)
		v1.GET("/projects/:projectId/volume-transfers", domains.volume.ListVolumeTransfers)
		v1.GET("/projects/:projectId/volume-transfers/:transferId", domains.volume.GetVolumeTransfer)
		v1.POST("/projects/:projectId/volume-transfers/:transferId/retry", domains.volume.RetryVolumeTransfer)
		v1.POST("/projects/:projectId/volume-transfers/:transferId/cancel", domains.volume.CancelVolumeTransfer)
		v1.POST("/projects/:projectId/volume-transfers/:transferId/download-authorizations", domains.volume.AuthorizeVolumeTransferDownload)
		v1.GET("/projects/:projectId/volume-transfers/:transferId/content", domains.volume.DownloadVolumeTransferContent)
		v1.GET("/projects/:projectId/volume-transfers/:transferId/manifest", domains.volume.DownloadVolumeTransferManifest)

		v1.GET("/projects/:projectId/members", domains.project.ListProjectMembers)
		v1.GET("/projects/:projectId/member-candidates", domains.project.SearchProjectMemberCandidates)
		v1.POST("/projects/:projectId/members", domains.project.CreateProjectMember)
		v1.PUT("/projects/:projectId/members/:memberId", domains.project.UpdateProjectMember)
		v1.DELETE("/projects/:projectId/members/:memberId", domains.project.DeleteProjectMember)

		v1.GET("/projects/:projectId/applications", domains.application.ListApplications)
		v1.POST("/projects/:projectId/applications", domains.application.CreateApplication)
		v1.GET("/projects/:projectId/applications/:applicationId", domains.application.GetApplication)
		v1.GET("/projects/:projectId/applications/:applicationId/topology", domains.application.GetApplicationTopology)
		v1.PUT("/projects/:projectId/applications/:applicationId", domains.application.UpdateApplication)
		v1.GET("/projects/:projectId/applications/:applicationId/deletion-preview", domains.application.PreviewApplicationDeletion)
		v1.DELETE("/projects/:projectId/applications/:applicationId", domains.application.DeleteApplication)
		v1.GET("/projects/:projectId/applications/:applicationId/deployment-targets", domains.deployment.ListDeploymentTargets)
		v1.POST("/projects/:projectId/applications/:applicationId/deployment-targets", domains.deployment.CreateDeploymentTarget)
		v1.GET("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/export", domains.deployment.ExportDeploymentTargetBundle)
		v1.POST("/projects/:projectId/applications/:applicationId/deployment-target-imports/preview", domains.deployment.PreviewDeploymentTargetBundleImport)
		v1.POST("/projects/:projectId/applications/:applicationId/deployment-target-imports/reference-candidates", domains.deployment.ListDeploymentTargetBundleReferenceCandidates)
		v1.POST("/projects/:projectId/applications/:applicationId/deployment-target-imports", domains.deployment.ImportDeploymentTargetBundle)
		v1.PUT("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId", domains.deployment.UpdateDeploymentTarget)
		v1.PUT("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/runtime-secrets", domains.deployment.UpdateDeploymentTargetRuntimeSecrets)
		v1.GET("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/runtime-secrets", domains.deployment.GetDeploymentTargetRuntimeSecretsSummary)
		v1.POST("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/restart", domains.deployment.RestartDeploymentTarget)
		v1.GET("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/metrics/stream", domains.deployment.StreamDeploymentTargetMetrics)
		v1.DELETE("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId", domains.deployment.DeleteDeploymentTarget)
		v1.GET("/projects/:projectId/build-runs", domains.build.ListBuildRuns)
		v1.POST("/projects/:projectId/build-runs/trigger", domains.build.TriggerBuildRun)
		v1.GET("/projects/:projectId/build-runs/:runId", domains.build.GetBuildRun)
		v1.POST("/projects/:projectId/build-runs/:runId/retry", domains.build.RetryBuildRun)
		v1.POST("/projects/:projectId/build-runs/:runId/cancel", domains.build.CancelBuildRun)
		v1.DELETE("/projects/:projectId/build-runs/:runId", domains.build.DeleteBuildRun)
		v1.GET("/projects/:projectId/build-jobs", domains.build.ListBuildJobs)
		v1.GET("/projects/:projectId/build-jobs/:jobId", domains.build.GetBuildJob)
		v1.GET("/projects/:projectId/build-jobs/:jobId/logs", domains.build.GetBuildJobLogs)
		v1.GET("/projects/:projectId/build-jobs/:jobId/logs/stream", domains.build.StreamBuildJobLogs)
		v1.GET("/projects/:projectId/releases", domains.deployment.ListReleases)
		v1.GET("/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/release-image-candidates", domains.deployment.ListReleaseImageCandidates)
		v1.POST("/projects/:projectId/releases", domains.deployment.CreateRelease)
		v1.GET("/projects/:projectId/releases/:releaseId", domains.deployment.GetRelease)
		v1.GET("/projects/:projectId/releases/:releaseId/logs", domains.deployment.GetReleaseLogs)
		v1.GET("/projects/:projectId/releases/:releaseId/runtime-logs", domains.deployment.GetReleaseRuntimeLogs)
		v1.POST("/projects/:projectId/releases/:releaseId/exec", domains.deployment.ExecReleaseRuntimeCommand)
		v1.POST("/projects/:projectId/releases/:releaseId/terminal/authorize", domains.deployment.AuthorizeReleaseRuntimeTerminal)
		v1.GET("/projects/:projectId/releases/:releaseId/terminal", domains.deployment.StreamReleaseRuntimeTerminal)
		v1.POST("/projects/:projectId/releases/:releaseId/rollback", domains.deployment.RollbackRelease)
		v1.GET("/projects/:projectId/gateway-routes", domains.gateway.ListGatewayRoutes)
		v1.POST("/projects/:projectId/gateway-routes", domains.gateway.CreateGatewayRoute)
		v1.GET("/projects/:projectId/gateway-routes/:routeId", domains.gateway.GetGatewayRoute)
		v1.PUT("/projects/:projectId/gateway-routes/:routeId", domains.gateway.UpdateGatewayRoute)
		v1.DELETE("/projects/:projectId/gateway-routes/:routeId", domains.gateway.DeleteGatewayRoute)
		v1.GET("/projects/:projectId/gateway-routes/check-domain", domains.gateway.CheckGatewayDomain)
		v1.GET("/projects/:projectId/repository-bindings", domains.git.ListRepositoryBindings)
		v1.POST("/projects/:projectId/repository-bindings", domains.git.CreateRepositoryBinding)
		v1.PUT("/projects/:projectId/repository-bindings/:bindingId", domains.git.UpdateRepositoryBinding)
		v1.DELETE("/projects/:projectId/repository-bindings/:bindingId", domains.git.DeleteRepositoryBinding)
		v1.POST("/projects/:projectId/repository-bindings/:bindingId/webhook", domains.git.CreateRepositoryWebhook)
		v1.POST("/projects/:projectId/repository-bindings/:bindingId/webhook/reconfigure", domains.git.ReconfigureRepositoryWebhook)

		v1.GET("/billing/summary", domains.billing.GetBillingSummary)
		v1.GET("/billing/deployment-spend", domains.billing.ListBillingDeploymentSpend)
		v1.GET("/billing/ledger", domains.billing.ListBillingLedgerEntries)
		v1.GET("/billing/usage-records", domains.billing.ListBillingUsageRecords)
		v1.GET("/billing/rate-rules", domains.billing.ListBillingRateRules)
		v1.PUT("/billing/rate-rules", domains.billing.UpdateBillingRateRules)
		v1.POST("/billing/wallet-transactions", domains.billing.CreateBillingWalletTransaction)
		v1.POST("/billing/external-transactions", domains.billing.CreateExternalBillingTransaction)
		v1.POST("/billing/gateway-traffic/hello", domains.billing.CreateGatewayTrafficProbeHello)
		v1.POST("/billing/gateway-traffic", domains.billing.CreateGatewayTrafficUsage)
		v1.GET("/billing/gateway-traffic-status", domains.billing.GetGatewayTrafficStatus)

		v1.GET("/access-tokens/scopes", domains.identity.ListAccessTokenScopes)
		v1.GET("/access-tokens", domains.identity.ListAccessTokens)
		v1.POST("/access-tokens", domains.identity.CreateAccessToken)
		v1.DELETE("/access-tokens/:tokenId", domains.identity.RevokeAccessToken)
	}

	registerSwaggerUI(router)
	return router
}

func configureTrustedProxies(router *gin.Engine, cidrs []string) {
	if err := router.SetTrustedProxies(cidrs); err == nil {
		return
	}
	// Config parsing already validates CIDRs. Keep this boundary fail-closed if
	// a future Gin version rejects a previously accepted representation.
	_ = router.SetTrustedProxies(nil)
}

func cors(cfg Config) gin.HandlerFunc {
	allowedOrigins := configuredAllowedOrigins(cfg)
	return func(ctx *gin.Context) {
		origin := strings.TrimSpace(ctx.GetHeader("Origin"))
		if origin != "" && containsString(allowedOrigins, origin) {
			ctx.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept-Language, Idempotency-Key, If-Match")
			ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
			ctx.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, Content-Length, Content-Disposition, Retry-After")
			ctx.Writer.Header().Add("Vary", "Origin")
		}

		if ctx.Request.Method == http.MethodOptions {
			if origin != "" && !containsString(allowedOrigins, origin) {
				transportapi.WriteErrorCode(ctx, http.StatusForbidden, "request.origin_forbidden", "request origin is not allowed")
				ctx.Abort()
				return
			}
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		ctx.Next()
	}
}

func securityHeaders(cfg Config) gin.HandlerFunc {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: https:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"object-src 'none'",
		"manifest-src 'self'",
		"frame-ancestors 'self'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")
	enableHSTS := hstsEnabled(cfg)
	return func(ctx *gin.Context) {
		ctx.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		ctx.Writer.Header().Set("X-Frame-Options", "SAMEORIGIN")
		ctx.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		ctx.Writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		ctx.Writer.Header().Set("Content-Security-Policy", csp)
		if enableHSTS {
			ctx.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		ctx.Next()
	}
}

func hstsEnabled(cfg Config) bool {
	return cfg.EnableHSTS
}

func csrfOriginGuard(cfg Config) gin.HandlerFunc {
	allowedOrigins := configuredAllowedOrigins(cfg)
	return func(ctx *gin.Context) {
		if !requiresCSRForiginCheck(ctx) {
			ctx.Next()
			return
		}
		if _, err := ctx.Cookie(sessionCookieName); err != nil {
			ctx.Next()
			return
		}
		if strings.HasPrefix(strings.ToLower(ctx.GetHeader("Authorization")), "bearer ") {
			ctx.Next()
			return
		}

		if requestOriginAllowed(ctx, allowedOrigins) {
			ctx.Next()
			return
		}

		transportapi.WriteError(ctx, http.StatusForbidden, "请求来源不受信任，请刷新页面后重试")
		ctx.Abort()
	}
}

func requiresCSRForiginCheck(ctx *gin.Context) bool {
	switch ctx.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	path := ctx.Request.URL.Path
	if strings.HasPrefix(path, "/api/v1/git/webhooks/") {
		return false
	}
	return true
}

func requestOriginAllowed(ctx *gin.Context, allowedOrigins []string) bool {
	if origin := strings.TrimSpace(ctx.GetHeader("Origin")); origin != "" {
		return containsString(allowedOrigins, origin)
	}
	referer := strings.TrimSpace(ctx.GetHeader("Referer"))
	if referer == "" {
		return false
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	return containsString(allowedOrigins, strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/"))
}

func configuredAllowedOrigins(cfg Config) []string {
	return append([]string(nil), cfg.AllowedOrigins...)
}

func originFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/")
}
