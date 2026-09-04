package api

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	scopedResourceGitProvider        = "git_provider"
	scopedResourceGitAccount         = "git_account"
	scopedResourceArtifactRegistry   = "artifact_registry"
	scopedResourceRegistryCredential = "registry_credential"
	scopedResourceBuildVariableSet   = "build_variable_set"
	scopedResourceRuntimeCluster     = "runtime_cluster"
)

type projectHost struct {
	domainHost
}

func (host projectHost) AIConversationProjectID(ctx *gin.Context) string {
	actor, _ := ctx.Request.Context().Value(aiPlatformActorContextKey{}).(aiPlatformActor)
	return actor.ProjectID
}

func (host projectHost) ProjectIDsForUser(ctx context.Context, userID string) []string {
	return host.handlers.projects.IDsForUserContext(ctx, userID)
}

func (host projectHost) ProjectHasAnotherOwner(ctx context.Context, projectID, memberID string) bool {
	return host.handlers.projects.HasAnotherOwnerContext(ctx, projectID, memberID)
}

func (host projectHost) ContinuousAuthorizationInterval() time.Duration {
	return host.handlers.continuousAuthorizationInterval
}

func (h *Handlers) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.domains.project.AuthorizeProject(ctx, action)
}

func (h *Handlers) authorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.domains.project.AuthorizeProjectByID(ctx, projectID, action)
}

func (h *Handlers) projectActionAllowed(ctx context.Context, subject authz.ProjectSubject, projectID string, action authz.Action) (bool, error) {
	return h.domains.project.ProjectActionAllowed(ctx, subject, projectID, action)
}

func (h *Handlers) projectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return h.domains.project.ProjectRoleActionAllowed(ctx, user, projectID, action)
}

func (h *Handlers) projectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return h.domains.project.ProjectMemberActionAllowed(ctx, projectID, userID, action)
}

func (h *Handlers) projectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return h.domains.project.ProjectAuthorizer(ctx)
}

func writeProjectAuthorizationError(ctx *gin.Context, err error) {
	projectapi.WriteProjectAuthorizationError(ctx, err)
}

func (h *Handlers) projectIDsForUser(ctx context.Context, userID string) []string {
	return h.domains.project.ProjectIDsForUser(ctx, userID)
}

func (h *Handlers) findProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return h.domains.project.FindProjectForCurrentUserByID(ctx, projectID)
}

func (h *Handlers) findProject(ctx *gin.Context) (model.Project, bool) {
	return h.domains.project.FindProject(ctx)
}

func resolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return projectapi.ResolveListVisibility(ctx, user)
}

func normalizeOwnerScope(value string) string { return projectapi.NormalizeOwnerScope(value) }

func (h *Handlers) normalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.domains.project.NormalizeScopedOwnerWithProjects(ctx, user, rawScope, rawOwnerRef, rawProjectIDs, globalError)
}

func (h *Handlers) normalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.domains.project.NormalizeCredentialScopeWithinParent(ctx, user, rawScope, rawProjectIDs, parentScope, parentProjectIDs, globalError)
}

func (h *Handlers) canManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return h.domains.project.CanManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (h *Handlers) canInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return h.domains.project.CanInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handlers) canUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return h.domains.project.CanUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handlers) applyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool) {
	return h.domains.project.ApplyScopedResourceVisibility(ctx, query, resourceType, user, projectID)
}

func (h *Handlers) applyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return h.domains.project.ApplyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (h *Handlers) applyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User, ctx context.Context) *gorm.DB {
	return h.domains.project.ApplyScopedResourceVisibilityForUser(query, resourceType, user, ctx)
}

func (h *Handlers) applyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return h.domains.project.ApplyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}

func applyScopedResourceVisibilityQuery(query *gorm.DB, bindingDB *gorm.DB, resourceType, userID, projectID string, projectIDs []string, includeAllProjects, includeUnboundProjectScope bool) *gorm.DB {
	return projectapi.ApplyScopedResourceVisibilityQuery(query, bindingDB, resourceType, userID, projectID, projectIDs, includeAllProjects, includeUnboundProjectScope)
}

func (h *Handlers) replaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return h.domains.project.ReplaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}

func (h *Handlers) scopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return h.domains.project.ScopedResourceProjectIDs(resourceType, resourceID, ctx)
}

func (h *Handlers) scopedResourceProjectIDsResult(resourceType, resourceID string, ctx context.Context) ([]string, error) {
	return h.domains.project.ScopedResourceProjectIDsResult(resourceType, resourceID, ctx)
}

func (h *Handlers) scopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.domains.project.ScopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}

func (h *Handlers) scopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.domains.project.ScopedResourceDefaultProjectIDMap(resourceType, resourceIDs, ctx)
}

func sortedProjectIDs(projectIDs []string) []string { return projectapi.SortedProjectIDs(projectIDs) }

func normalizeHookPhase(value string) string { return projectapi.NormalizeHookPhase(value) }

func (h *Handlers) decideInboxAction(ctx context.Context, user model.User, requestID, decision string, expectedVersion int64) error {
	return h.domains.project.DecideInboxAction(ctx, user, requestID, decision, expectedVersion)
}

type continuousAuthorizationBinding = projectapi.ContinuousAuthorizationBinding
type runtimeTerminalAuthorizationBinding = continuousAuthorizationBinding

func (h *Handlers) currentContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (continuousAuthorizationBinding, bool) {
	return h.domains.project.CurrentContinuousAuthorizationBinding(ctx, user)
}

func continuousAuthorizationBindingForAccessToken(userID string, token model.AccessToken) continuousAuthorizationBinding {
	return projectapi.ContinuousAuthorizationBindingForAccessToken(userID, token)
}

func (h *Handlers) requireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (continuousAuthorizationBinding, bool) {
	return h.domains.project.RequireContinuousAuthorizationBinding(ctx, user)
}

func writeContinuousAuthorizationRevoked(ctx *gin.Context) {
	projectapi.WriteContinuousAuthorizationRevoked(ctx)
}

func continuousAccessTokenSubject(tokenID string) string {
	return projectapi.ContinuousAccessTokenSubject(tokenID)
}

func continuousAuthorizationAccessTokenID(subject string) (string, bool) {
	return projectapi.ContinuousAuthorizationAccessTokenID(subject)
}

func (h *Handlers) continuousAuthorizationCheckInterval() time.Duration {
	return h.domains.project.ContinuousAuthorizationCheckInterval()
}

func (h *Handlers) monitorContinuousAuthorization(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.domains.project.MonitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}

func (h *Handlers) monitorContinuousAuthorizationWithInterval(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func(), checkInterval time.Duration) (<-chan struct{}, bool) {
	return h.domains.project.MonitorContinuousAuthorizationWithInterval(ctx, binding, authorizationAllowed, revoke, checkInterval)
}

func (h *Handlers) continuousAuthorizationActive(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return h.domains.project.ContinuousAuthorizationActive(ctx, binding, authorizationAllowed)
}

func (h *Handlers) projectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return h.domains.project.ProjectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}

func replaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	return projectapi.ReplaceRequestContext(ctx, requestCtx)
}
