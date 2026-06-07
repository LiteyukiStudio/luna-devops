package api

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB) *gin.Engine {
	if debugLogEnabled() {
		gin.SetMode(gin.DebugMode)
		debugLog("api log level set to debug")
	}
	router := gin.New()
	router.Use(gin.Logger(), recoveryMiddleware(), errorResponseMiddleware(), securityHeaders(), cors(), csrfOriginGuard())

	handlers := NewHandlers(db)

	router.GET("/healthz", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := router.Group("/api/v1")
	{
		v1.POST("/public/configs", handlers.GetPublicConfigs)
		v1.GET("/auth/bootstrap", handlers.GetBootstrapStatus)
		v1.POST("/auth/bootstrap/admin", handlers.InitializeAdmin)
		v1.POST("/auth/login", handlers.Login)
		v1.POST("/auth/login/resume", handlers.ResumeLogin)
		v1.POST("/auth/logout", handlers.Logout)
		v1.GET("/auth/providers", handlers.ListAuthProviders)
		v1.POST("/auth/providers", handlers.CreateAuthProvider)
		v1.PUT("/auth/providers/:providerId", handlers.UpdateAuthProvider)
		v1.GET("/auth/admission-policy", handlers.GetAuthAdmissionPolicy)
		v1.PUT("/auth/admission-policy", handlers.UpdateAuthAdmissionPolicy)
		v1.GET("/auth/oidc/:providerId/start", handlers.StartOIDC)
		v1.GET("/auth/oidc/callback", handlers.CompleteOIDC)
		v1.GET("/users/me", handlers.GetCurrentUser)
		v1.PUT("/users/me", handlers.UpdateCurrentUser)
		v1.GET("/users/me/external-identities", handlers.ListMyExternalIdentities)
		v1.DELETE("/users/me/external-identities/:identityId", handlers.UnbindMyExternalIdentity)
		v1.GET("/users", handlers.ListUsers)
		v1.POST("/users", handlers.CreateUser)
		v1.PUT("/users/:userId", handlers.UpdateUser)
		v1.GET("/configs/definitions", handlers.ListConfigDefinitions)
		v1.GET("/configs", handlers.GetConfigs)
		v1.PUT("/configs", handlers.UpdateConfigs)

		v1.GET("/git/providers", handlers.ListGitProviders)
		v1.POST("/git/providers", handlers.CreateGitProvider)
		v1.PUT("/git/providers/:providerId", handlers.UpdateGitProvider)
		v1.DELETE("/git/providers/:providerId", handlers.DeleteGitProvider)
		v1.GET("/git/providers/:providerId/oauth/start", handlers.StartGitOAuth)
		v1.GET("/git/oauth/callback", handlers.CompleteGitOAuth)
		v1.POST("/git/webhooks/:bindingId", handlers.ReceiveGitWebhook)
		v1.GET("/git/accounts", handlers.ListGitAccounts)
		v1.POST("/git/accounts", handlers.CreateGitAccount)
		v1.PUT("/git/accounts/:accountId", handlers.UpdateGitAccount)
		v1.DELETE("/git/accounts/:accountId", handlers.DeleteGitAccount)
		v1.POST("/git/accounts/:accountId/refresh", handlers.RefreshGitAccount)
		v1.GET("/git/accounts/:accountId/repositories", handlers.ListGitRepositories)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/branches", handlers.ListGitBranches)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/build-options", handlers.GetGitRepositoryBuildOptions)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/contents", handlers.ListGitContents)
		v1.GET("/git/accounts/:accountId/repositories/:owner/:repo/file", handlers.ReadGitFile)

		v1.GET("/registries", handlers.ListArtifactRegistries)
		v1.POST("/registries", handlers.CreateArtifactRegistry)
		v1.PUT("/registries/:registryId", handlers.UpdateArtifactRegistry)
		v1.DELETE("/registries/:registryId", handlers.DeleteArtifactRegistry)
		v1.POST("/registries/:registryId/test", handlers.TestArtifactRegistry)
		v1.GET("/registries/:registryId/repositories/search", handlers.SearchRegistryRepositories)
		v1.GET("/registries/:registryId/repository-tags", handlers.ListRegistryRepositoryTags)
		v1.GET("/registries/:registryId/credentials", handlers.ListRegistryCredentials)
		v1.POST("/registries/:registryId/credentials", handlers.CreateRegistryCredential)
		v1.DELETE("/registries/:registryId/credentials/:credentialId", handlers.DeleteRegistryCredential)
		v1.GET("/container-images", handlers.ListContainerImages)
		v1.POST("/container-images", handlers.CreateContainerImage)

		v1.GET("/build/providers", handlers.ListBuildProviders)
		v1.POST("/build/providers", handlers.CreateBuildProvider)
		v1.PUT("/build/providers/:providerId", handlers.UpdateBuildProvider)
		v1.DELETE("/build/providers/:providerId", handlers.DeleteBuildProvider)
		v1.GET("/build/builders", handlers.ListBuilderAgents)
		v1.DELETE("/build/builders/:builderId", handlers.DeleteBuilderAgent)
		v1.GET("/build/variable-sets", handlers.ListBuildVariableSets)
		v1.POST("/build/variable-sets", handlers.CreateBuildVariableSet)
		v1.PUT("/build/variable-sets/:setId", handlers.UpdateBuildVariableSet)
		v1.DELETE("/build/variable-sets/:setId", handlers.DeleteBuildVariableSet)

		v1.GET("/runtime/clusters", handlers.ListRuntimeClusters)
		v1.POST("/runtime/clusters", handlers.CreateRuntimeCluster)
		v1.PUT("/runtime/clusters/:clusterId", handlers.UpdateRuntimeCluster)
		v1.DELETE("/runtime/clusters/:clusterId", handlers.DeleteRuntimeCluster)
		v1.POST("/runtime/clusters/:clusterId/test", handlers.TestRuntimeCluster)

		v1.GET("/projects", handlers.ListProjects)
		v1.GET("/projects/pins", handlers.ListProjectPins)
		v1.POST("/projects", handlers.CreateProject)
		v1.GET("/projects/:projectId", handlers.GetProject)
		v1.PUT("/projects/:projectId", handlers.UpdateProject)
		v1.DELETE("/projects/:projectId", handlers.DeleteProject)
		v1.PUT("/projects/:projectId/pin", handlers.PinProject)
		v1.DELETE("/projects/:projectId/pin", handlers.UnpinProject)
		v1.GET("/projects/:projectId/registries/default", handlers.GetDefaultArtifactRegistry)

		v1.GET("/projects/:projectId/members", handlers.ListProjectMembers)
		v1.POST("/projects/:projectId/members", handlers.CreateProjectMember)
		v1.PUT("/projects/:projectId/members/:memberId", handlers.UpdateProjectMember)
		v1.DELETE("/projects/:projectId/members/:memberId", handlers.DeleteProjectMember)

		v1.GET("/projects/:projectId/applications", handlers.ListApplications)
		v1.POST("/projects/:projectId/applications", handlers.CreateApplication)
		v1.GET("/projects/:projectId/applications/:applicationId", handlers.GetApplication)
		v1.PUT("/projects/:projectId/applications/:applicationId", handlers.UpdateApplication)
		v1.DELETE("/projects/:projectId/applications/:applicationId", handlers.DeleteApplication)
		v1.GET("/projects/:projectId/build-runs", handlers.ListBuildRuns)
		v1.POST("/projects/:projectId/build-runs/trigger", handlers.TriggerBuildRun)
		v1.GET("/projects/:projectId/build-runs/:runId", handlers.GetBuildRun)
		v1.POST("/projects/:projectId/build-runs/:runId/retry", handlers.RetryBuildRun)
		v1.GET("/projects/:projectId/build-jobs", handlers.ListBuildJobs)
		v1.GET("/projects/:projectId/build-jobs/:jobId", handlers.GetBuildJob)
		v1.GET("/projects/:projectId/build-jobs/:jobId/logs", handlers.GetBuildJobLogs)
		v1.GET("/projects/:projectId/environments", handlers.ListEnvironments)
		v1.POST("/projects/:projectId/environments", handlers.CreateEnvironment)
		v1.PUT("/projects/:projectId/environments/:environmentId", handlers.UpdateEnvironment)
		v1.DELETE("/projects/:projectId/environments/:environmentId", handlers.DeleteEnvironment)
		v1.GET("/projects/:projectId/releases", handlers.ListReleases)
		v1.POST("/projects/:projectId/releases", handlers.CreateRelease)
		v1.POST("/projects/:projectId/releases/:releaseId/rollback", handlers.RollbackRelease)
		v1.GET("/projects/:projectId/gateway-routes", handlers.ListGatewayRoutes)
		v1.POST("/projects/:projectId/gateway-routes", handlers.CreateGatewayRoute)
		v1.PUT("/projects/:projectId/gateway-routes/:routeId", handlers.UpdateGatewayRoute)
		v1.DELETE("/projects/:projectId/gateway-routes/:routeId", handlers.DeleteGatewayRoute)
		v1.GET("/projects/:projectId/gateway-routes/check-domain", handlers.CheckGatewayDomain)
		v1.GET("/projects/:projectId/repository-bindings", handlers.ListRepositoryBindings)
		v1.POST("/projects/:projectId/repository-bindings", handlers.CreateRepositoryBinding)
		v1.PUT("/projects/:projectId/repository-bindings/:bindingId", handlers.UpdateRepositoryBinding)
		v1.DELETE("/projects/:projectId/repository-bindings/:bindingId", handlers.DeleteRepositoryBinding)
		v1.POST("/projects/:projectId/repository-bindings/:bindingId/webhook", handlers.CreateRepositoryWebhook)

		v1.GET("/access-tokens", handlers.ListAccessTokens)
		v1.POST("/access-tokens", handlers.CreateAccessToken)
		v1.DELETE("/access-tokens/:tokenId", handlers.RevokeAccessToken)
	}

	return router
}

func cors() gin.HandlerFunc {
	allowedOrigins := configuredAllowedOrigins()
	return func(ctx *gin.Context) {
		origin := strings.TrimSpace(ctx.GetHeader("Origin"))
		if origin != "" && containsString(allowedOrigins, origin) {
			ctx.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept-Language")
			ctx.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			ctx.Writer.Header().Add("Vary", "Origin")
		}

		if ctx.Request.Method == http.MethodOptions {
			if origin != "" && !containsString(allowedOrigins, origin) {
				ctx.AbortWithStatus(http.StatusForbidden)
				return
			}
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		ctx.Next()
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		ctx.Writer.Header().Set("X-Frame-Options", "SAMEORIGIN")
		ctx.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		ctx.Writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		ctx.Next()
	}
}

func csrfOriginGuard() gin.HandlerFunc {
	allowedOrigins := configuredAllowedOrigins()
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

		writeError(ctx, http.StatusForbidden, "请求来源不受信任，请刷新页面后重试")
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

func configuredAllowedOrigins() []string {
	origins := normalizeList(strings.Split(os.Getenv("APP_CORS_ORIGINS"), ","), false)
	if publicBase := originFromURL(os.Getenv("PUBLIC_BASE_URL")); publicBase != "" {
		origins = append(origins, publicBase)
	}
	if configRuntimeMode() == "development" {
		origins = append(origins,
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://localhost:4173",
			"http://127.0.0.1:4173",
			"http://localhost:4174",
			"http://127.0.0.1:4174",
			"http://localhost:4184",
			"http://127.0.0.1:4184",
		)
	}
	return normalizeList(origins, false)
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

func configRuntimeMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if mode == "production" {
		return "production"
	}
	return "development"
}
