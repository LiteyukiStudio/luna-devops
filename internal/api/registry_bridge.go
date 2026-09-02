package api

import (
	"context"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/registryapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type registryHost struct {
	handlers *Handlers
}

func (host registryHost) DBFor(ctx *gin.Context) *gorm.DB {
	return host.handlers.dbFor(ctx)
}

func (host registryHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host registryHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}

func (host registryHost) ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return resolveListVisibility(ctx, user)
}

func (host registryHost) ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return host.handlers.applyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (host registryHost) ApplyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User, ctx context.Context) *gorm.DB {
	return host.handlers.applyScopedResourceVisibilityForUser(query, resourceType, user, ctx)
}

func (host registryHost) ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return host.handlers.applyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}

func (host registryHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeScopedOwnerWithProjects(ctx, user, rawScope, rawOwnerRef, rawProjectIDs, globalError)
}

func (host registryHost) NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeCredentialScopeWithinParent(ctx, user, rawScope, rawProjectIDs, parentScope, parentProjectIDs, globalError)
}

func (host registryHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return host.handlers.canManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (host registryHost) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return host.handlers.canInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (host registryHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return host.handlers.canUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (host registryHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return host.handlers.replaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}

func (host registryHost) ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return host.handlers.scopedResourceProjectIDs(resourceType, resourceID, ctx)
}

func (host registryHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return host.handlers.scopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}

func (host registryHost) ScopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return host.handlers.scopedResourceDefaultProjectIDMap(resourceType, resourceIDs, ctx)
}

func (host registryHost) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return host.handlers.findProjectForCurrentUserByID(ctx, projectID)
}

func (host registryHost) ProjectIDsForUser(ctx context.Context, userID string) []string {
	return host.handlers.projectIDsForUser(ctx, userID)
}

func (host registryHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProjectByID(ctx, projectID, action)
}

func (host registryHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}

func (host registryHost) WriteProjectAuthorizationError(ctx *gin.Context, err error) {
	writeProjectAuthorizationError(ctx, err)
}

func (host registryHost) SecretAvailable() bool {
	return host.handlers.secrets.Available()
}

func (host registryHost) StoreSecret(ctx context.Context, value, userID, resource string) string {
	return host.handlers.secrets.StoreContext(ctx, value, userID, resource)
}

func (host registryHost) ResolveSecret(ctx context.Context, ref string) string {
	return host.handlers.secrets.ResolveContext(ctx, ref)
}

func (host registryHost) EgressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy {
	return host.handlers.egressPolicyForUser(user, ctx)
}

func (host registryHost) AllowRegistrySearch(ctx *gin.Context, userID string) bool {
	limit := 60
	if host.handlers.mode == "development" {
		limit = developmentRateLimit
	}
	allowed, err := host.handlers.rateLimiter.allow(ctx.Request.Context(), "registry_search:"+userID, limit, time.Minute)
	if allowed {
		return true
	}
	if err != nil && host.handlers.mode == "development" {
		return true
	}
	writeError(ctx, http.StatusTooManyRequests, "镜像搜索请求过于频繁，请稍后再试")
	return false
}

func (h *Handlers) registryAPI() *registryapi.Handler {
	return registryapi.New(registryHost{handlers: h})
}

func (h *Handlers) ListArtifactRegistries(ctx *gin.Context) {
	h.registryAPI().ListArtifactRegistries(ctx)
}

func (h *Handlers) CreateArtifactRegistry(ctx *gin.Context) {
	h.registryAPI().CreateArtifactRegistry(ctx)
}

func (h *Handlers) UpdateArtifactRegistry(ctx *gin.Context) {
	h.registryAPI().UpdateArtifactRegistry(ctx)
}

func (h *Handlers) DeleteArtifactRegistry(ctx *gin.Context) {
	h.registryAPI().DeleteArtifactRegistry(ctx)
}

func (h *Handlers) GetDefaultArtifactRegistry(ctx *gin.Context) {
	h.registryAPI().GetDefaultArtifactRegistry(ctx)
}

func (h *Handlers) TestArtifactRegistry(ctx *gin.Context) {
	h.registryAPI().TestArtifactRegistry(ctx)
}

func (h *Handlers) GetRegistryImageTemplateDefault(ctx *gin.Context) {
	h.registryAPI().GetRegistryImageTemplateDefault(ctx)
}

func (h *Handlers) SearchRegistryRepositories(ctx *gin.Context) {
	h.registryAPI().SearchRegistryRepositories(ctx)
}

func (h *Handlers) ListRegistryRepositoryTags(ctx *gin.Context) {
	h.registryAPI().ListRegistryRepositoryTags(ctx)
}

func (h *Handlers) ListRegistryCredentials(ctx *gin.Context) {
	h.registryAPI().ListRegistryCredentials(ctx)
}

func (h *Handlers) ListAllRegistryCredentials(ctx *gin.Context) {
	h.registryAPI().ListAllRegistryCredentials(ctx)
}

func (h *Handlers) CreateRegistryCredential(ctx *gin.Context) {
	h.registryAPI().CreateRegistryCredential(ctx)
}

func (h *Handlers) UpdateRegistryCredential(ctx *gin.Context) {
	h.registryAPI().UpdateRegistryCredential(ctx)
}

func (h *Handlers) DeleteRegistryCredential(ctx *gin.Context) {
	h.registryAPI().DeleteRegistryCredential(ctx)
}

func (h *Handlers) ListContainerImages(ctx *gin.Context) {
	h.registryAPI().ListContainerImages(ctx)
}

func (h *Handlers) CreateContainerImage(ctx *gin.Context) {
	h.registryAPI().CreateContainerImage(ctx)
}

func (h *Handlers) registryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return h.registryAPI().RegistryPushCredentialForProject(user, registry, projectID, ctx)
}

func (h *Handlers) registryCredentialInput(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryprovider.Credential {
	return h.registryAPI().RegistryCredentialInput(ctx, user, registry)
}

func (h *Handlers) pingRegistry(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryapi.RegistryTestResult {
	return h.registryAPI().PingRegistry(ctx, user, registry)
}

func registryResponse(registry model.ArtifactRegistry) registryapi.ArtifactRegistryOutput {
	return registryapi.RegistryResponse(registry)
}

func normalizeRegistryProvider(value string) string {
	return registryapi.NormalizeRegistryProvider(value)
}

func containerImagePageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return registryapi.ContainerImagePageQuery(query, pagination)
}
