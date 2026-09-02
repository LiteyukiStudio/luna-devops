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
	handlers *Handlers
}

func (host projectHost) DBFor(ctx *gin.Context) *gorm.DB {
	return host.handlers.dbFor(ctx)
}

func (host projectHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host projectHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}

func (host projectHost) EnsurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	return host.handlers.ensurePlatformSystemProject(user, ctx)
}

func (host projectHost) AIConversationProjectID(ctx *gin.Context) string {
	return aiConversationProjectID(ctx)
}

func (host projectHost) EnqueueEnabledProjectAccessKubeGateways(ctx context.Context, userID string, includeGlobal bool) error {
	return host.handlers.enqueueEnabledProjectAccessKubeGateways(ctx, userID, includeGlobal)
}

func (host projectHost) EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return host.handlers.enqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}

func (host projectHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}

func (host projectHost) ProjectIDsForUser(ctx context.Context, userID string) []string {
	return host.handlers.projects.IDsForUserContext(ctx, userID)
}

func (host projectHost) ProjectHasAnotherOwner(ctx context.Context, projectID, memberID string) bool {
	return host.handlers.projects.HasAnotherOwnerContext(ctx, projectID, memberID)
}

func (host projectHost) RequestUsesBearerToken(ctx *gin.Context) bool {
	return requestUsesBearerToken(ctx)
}

func (host projectHost) CurrentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	return currentAccessTokenFromContext(ctx)
}

func (host projectHost) CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return host.handlers.currentSessionFromCookie(ctx)
}

func (host projectHost) ContinuousAuthorizationInterval() time.Duration {
	return host.handlers.continuousAuthorizationInterval
}

func (h *Handlers) projectAPI() *projectapi.Handler {
	return projectapi.New(projectHost{handlers: h})
}

func (h *Handlers) ListProjects(ctx *gin.Context)       { h.projectAPI().ListProjects(ctx) }
func (h *Handlers) CreateProject(ctx *gin.Context)      { h.projectAPI().CreateProject(ctx) }
func (h *Handlers) GetProject(ctx *gin.Context)         { h.projectAPI().GetProject(ctx) }
func (h *Handlers) UpdateProject(ctx *gin.Context)      { h.projectAPI().UpdateProject(ctx) }
func (h *Handlers) DeleteProject(ctx *gin.Context)      { h.projectAPI().DeleteProject(ctx) }
func (h *Handlers) ListProjectPins(ctx *gin.Context)    { h.projectAPI().ListProjectPins(ctx) }
func (h *Handlers) PinProject(ctx *gin.Context)         { h.projectAPI().PinProject(ctx) }
func (h *Handlers) UnpinProject(ctx *gin.Context)       { h.projectAPI().UnpinProject(ctx) }
func (h *Handlers) UpdateProjectOrder(ctx *gin.Context) { h.projectAPI().UpdateProjectOrder(ctx) }
func (h *Handlers) ListProjectMembers(ctx *gin.Context) { h.projectAPI().ListProjectMembers(ctx) }
func (h *Handlers) SearchProjectMemberCandidates(ctx *gin.Context) {
	h.projectAPI().SearchProjectMemberCandidates(ctx)
}
func (h *Handlers) CreateProjectMember(ctx *gin.Context) { h.projectAPI().CreateProjectMember(ctx) }
func (h *Handlers) UpdateProjectMember(ctx *gin.Context) { h.projectAPI().UpdateProjectMember(ctx) }
func (h *Handlers) DeleteProjectMember(ctx *gin.Context) { h.projectAPI().DeleteProjectMember(ctx) }
func (h *Handlers) CreateBillingOwnerTransferRequest(ctx *gin.Context) {
	h.projectAPI().CreateBillingOwnerTransferRequest(ctx)
}
func (h *Handlers) ListProjectHookConfigs(ctx *gin.Context) {
	h.projectAPI().ListProjectHookConfigs(ctx)
}
func (h *Handlers) CreateProjectHookConfig(ctx *gin.Context) {
	h.projectAPI().CreateProjectHookConfig(ctx)
}
func (h *Handlers) UpdateProjectHookConfig(ctx *gin.Context) {
	h.projectAPI().UpdateProjectHookConfig(ctx)
}
func (h *Handlers) DeleteProjectHookConfig(ctx *gin.Context) {
	h.projectAPI().DeleteProjectHookConfig(ctx)
}
func (h *Handlers) ListProjectHookRuns(ctx *gin.Context)  { h.projectAPI().ListProjectHookRuns(ctx) }
func (h *Handlers) GetProjectHookRunLog(ctx *gin.Context) { h.projectAPI().GetProjectHookRunLog(ctx) }
func (h *Handlers) GetProjectTopology(ctx *gin.Context)   { h.projectAPI().GetProjectTopology(ctx) }

func (h *Handlers) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.projectAPI().AuthorizeProject(ctx, action)
}

func (h *Handlers) authorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.projectAPI().AuthorizeProjectByID(ctx, projectID, action)
}

func (h *Handlers) projectActionAllowed(ctx context.Context, subject authz.ProjectSubject, projectID string, action authz.Action) (bool, error) {
	return h.projectAPI().ProjectActionAllowed(ctx, subject, projectID, action)
}

func (h *Handlers) projectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return h.projectAPI().ProjectRoleActionAllowed(ctx, user, projectID, action)
}

func (h *Handlers) projectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return h.projectAPI().ProjectMemberActionAllowed(ctx, projectID, userID, action)
}

func (h *Handlers) projectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return h.projectAPI().ProjectAuthorizer(ctx)
}

func writeProjectAuthorizationError(ctx *gin.Context, err error) {
	projectapi.WriteProjectAuthorizationError(ctx, err)
}

func (h *Handlers) projectIDsForUser(ctx context.Context, userID string) []string {
	return h.projectAPI().ProjectIDsForUser(ctx, userID)
}

func (h *Handlers) findProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return h.projectAPI().FindProjectForCurrentUserByID(ctx, projectID)
}

func (h *Handlers) findProject(ctx *gin.Context) (model.Project, bool) {
	return h.projectAPI().FindProject(ctx)
}

func resolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return projectapi.ResolveListVisibility(ctx, user)
}

func normalizeOwnerScope(value string) string { return projectapi.NormalizeOwnerScope(value) }

func (h *Handlers) normalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.projectAPI().NormalizeScopedOwnerWithProjects(ctx, user, rawScope, rawOwnerRef, rawProjectIDs, globalError)
}

func (h *Handlers) normalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.projectAPI().NormalizeCredentialScopeWithinParent(ctx, user, rawScope, rawProjectIDs, parentScope, parentProjectIDs, globalError)
}

func (h *Handlers) canManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return h.projectAPI().CanManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (h *Handlers) canInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return h.projectAPI().CanInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handlers) canUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return h.projectAPI().CanUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handlers) applyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool) {
	return h.projectAPI().ApplyScopedResourceVisibility(ctx, query, resourceType, user, projectID)
}

func (h *Handlers) applyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return h.projectAPI().ApplyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (h *Handlers) applyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User, ctx context.Context) *gorm.DB {
	return h.projectAPI().ApplyScopedResourceVisibilityForUser(query, resourceType, user, ctx)
}

func (h *Handlers) applyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return h.projectAPI().ApplyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}

func applyScopedResourceVisibilityQuery(query *gorm.DB, bindingDB *gorm.DB, resourceType, userID, projectID string, projectIDs []string, includeAllProjects, includeUnboundProjectScope bool) *gorm.DB {
	return projectapi.ApplyScopedResourceVisibilityQuery(query, bindingDB, resourceType, userID, projectID, projectIDs, includeAllProjects, includeUnboundProjectScope)
}

func (h *Handlers) replaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return h.projectAPI().ReplaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}

func (h *Handlers) scopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return h.projectAPI().ScopedResourceProjectIDs(resourceType, resourceID, ctx)
}

func (h *Handlers) scopedResourceProjectIDsResult(resourceType, resourceID string, ctx context.Context) ([]string, error) {
	return h.projectAPI().ScopedResourceProjectIDsResult(resourceType, resourceID, ctx)
}

func (h *Handlers) scopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.projectAPI().ScopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}

func (h *Handlers) scopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.projectAPI().ScopedResourceDefaultProjectIDMap(resourceType, resourceIDs, ctx)
}

func sortedProjectIDs(projectIDs []string) []string { return projectapi.SortedProjectIDs(projectIDs) }

func normalizeHookPhase(value string) string { return projectapi.NormalizeHookPhase(value) }

func (h *Handlers) decideInboxAction(ctx context.Context, user model.User, requestID, decision string, expectedVersion int64) error {
	return h.projectAPI().DecideInboxAction(ctx, user, requestID, decision, expectedVersion)
}

type continuousAuthorizationBinding = projectapi.ContinuousAuthorizationBinding
type runtimeTerminalAuthorizationBinding = continuousAuthorizationBinding

func (h *Handlers) currentContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (continuousAuthorizationBinding, bool) {
	return h.projectAPI().CurrentContinuousAuthorizationBinding(ctx, user)
}

func continuousAuthorizationBindingForAccessToken(userID string, token model.AccessToken) continuousAuthorizationBinding {
	return projectapi.ContinuousAuthorizationBindingForAccessToken(userID, token)
}

func (h *Handlers) requireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (continuousAuthorizationBinding, bool) {
	return h.projectAPI().RequireContinuousAuthorizationBinding(ctx, user)
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
	return h.projectAPI().ContinuousAuthorizationCheckInterval()
}

func (h *Handlers) monitorContinuousAuthorization(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.projectAPI().MonitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}

func (h *Handlers) monitorContinuousAuthorizationWithInterval(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func(), checkInterval time.Duration) (<-chan struct{}, bool) {
	return h.projectAPI().MonitorContinuousAuthorizationWithInterval(ctx, binding, authorizationAllowed, revoke, checkInterval)
}

func (h *Handlers) continuousAuthorizationActive(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return h.projectAPI().ContinuousAuthorizationActive(ctx, binding, authorizationAllowed)
}

func (h *Handlers) projectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return h.projectAPI().ProjectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}

func replaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	return projectapi.ReplaceRequestContext(ctx, requestCtx)
}
