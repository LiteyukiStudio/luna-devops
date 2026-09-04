package api

import (
	"context"
	"errors"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// domainHost exposes the root package facilities shared by multiple domain APIs.
// Domain-specific adapters embed it and only keep behavior that is unique to that domain.
type domainHost struct {
	handlers *Handlers
}

func (host domainHost) DBFor(ctx *gin.Context) *gorm.DB {
	return host.handlers.dbFor(ctx)
}

func (host domainHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host domainHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}

func (host domainHost) CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return host.handlers.currentSessionFromCookie(ctx)
}

func (domainHost) CurrentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	return currentAccessTokenFromContext(ctx)
}

func (domainHost) RequestUsesBearerToken(ctx *gin.Context) bool {
	return requestUsesBearerToken(ctx)
}

func (host domainHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}

func (host domainHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProjectByID(ctx, projectID, action)
}

func (host domainHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	return host.handlers.requirePlatformAdmin(ctx)
}

func (host domainHost) EnsurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	return host.handlers.ensurePlatformSystemProject(user, ctx)
}

func (host domainHost) EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return host.handlers.ensureProjectCanMutate(ctx, project)
}

func (host domainHost) EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return host.handlers.ensureBillingAllowsDeployChange(ctx, projectID)
}

func (host domainHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}

func (host domainHost) SecretStore() secret.Store {
	return host.handlers.secrets
}

func (host domainHost) StoreSecret(ctx context.Context, value, userID, resource string) string {
	return host.handlers.secrets.StoreContext(ctx, value, userID, resource)
}

func (host domainHost) ResolveSecret(ctx context.Context, ref string) string {
	return host.handlers.secrets.ResolveContext(ctx, ref)
}

func (host domainHost) PublicBaseURL() string {
	return host.handlers.config.PublicBaseURL
}

func (host domainHost) Mode() string {
	return host.handlers.mode
}

func (host domainHost) AllowedOrigin(origin string) bool {
	return containsString(host.handlers.config.AllowedOrigins, origin)
}

func (domainHost) DeleteStatusCanStart(status string) bool {
	return deleteStatusCanStart(status)
}

func (domainHost) ResourceDeleteAlreadyStarted(err error) bool {
	return errors.Is(err, errResourceDeleteAlreadyStarted)
}

func (domainHost) MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	return markResourceDeleting(tx, resource, resourceID)
}

func (domainHost) MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error {
	return markResourceDeleteFailed(db, resource, resourceID, message)
}

func (domainHost) ResourceCanMutateDuringDelete(status string) bool {
	return resourceCanMutateDuringDelete(status)
}

func (host domainHost) EnqueueDeployRun(ctx context.Context, release model.Release) bool {
	return host.handlers.enqueueDeployRun(ctx, release)
}

func (host domainHost) EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return host.handlers.enqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}

func (host domainHost) ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return resolveListVisibility(ctx, user)
}

func (host domainHost) ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return host.handlers.applyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (host domainHost) ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return host.handlers.applyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}

func (host domainHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeScopedOwnerWithProjects(ctx, user, scope, ownerRef, projectIDs, globalError)
}

func (host domainHost) NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, scope string, projectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeCredentialScopeWithinParent(ctx, user, scope, projectIDs, parentScope, parentProjectIDs, globalError)
}

func (host domainHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return host.handlers.canManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (host domainHost) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return host.handlers.canInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (host domainHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return host.handlers.canUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (host domainHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return host.handlers.replaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}

func (host domainHost) ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return host.handlers.scopedResourceProjectIDs(resourceType, resourceID, ctx)
}

func (host domainHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return host.handlers.scopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}

func (host domainHost) WriteProjectAuthorizationError(ctx *gin.Context, err error) {
	writeProjectAuthorizationError(ctx, err)
}

func (host domainHost) ProjectIDsForUser(ctx context.Context, userID string) []string {
	return host.handlers.projectIDsForUser(ctx, userID)
}

func (host domainHost) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return host.handlers.findProjectForCurrentUserByID(ctx, projectID)
}

func (host domainHost) ProjectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return host.handlers.projectMemberActionAllowed(ctx, projectID, userID, action)
}

func (host domainHost) ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return host.handlers.projectRoleActionAllowed(ctx, user, projectID, action)
}

func (host domainHost) ProjectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return host.handlers.projectAuthorizer(ctx)
}

func (host domainHost) RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return host.handlers.requireContinuousAuthorizationBinding(ctx, user)
}

func (host domainHost) MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return host.handlers.monitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}

func (host domainHost) ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return host.handlers.projectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}

func (host domainHost) RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return host.handlers.domains.runtime.RuntimeClusterForProjectUse(ctx, user, projectID, clusterID)
}

func (host domainHost) RuntimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return host.handlers.domains.runtime.RuntimeClusterForDeploymentTarget(ctx, target)
}

func (domainHost) RuntimeProjectNamespace(project model.Project) string {
	return runtimeapi.RuntimeProjectNamespace(project)
}

func (domainHost) DeploymentTargetResourceName(target model.DeploymentTarget) string {
	return deploymentTargetResourceName(target)
}

func (domainHost) ApplicationCanMutate(application model.Application) bool {
	return applicationapi.ApplicationCanMutate(application)
}

func (host domainHost) RegistryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return host.handlers.registryPushCredentialForProject(user, registry, projectID, ctx)
}

func (host domainHost) EgressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy {
	return host.handlers.egressPolicyForUser(user, ctx)
}
