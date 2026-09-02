package buildapi

import (
	"context"
	"sort"

	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const scopedResourceBuildVariableSet = "build_variable_set"

type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool)
	ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool)
	NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool)
	CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool
	CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error)
	ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error
	ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string
	ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string
	ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error)
	WriteProjectAuthorizationError(ctx *gin.Context, err error)
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	EnsureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool
	RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool)
	MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool)
	ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool
	RegistryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool)
	StoreSecret(ctx context.Context, value, userID, resource string) string
	ResolveSecret(ctx context.Context, ref string) string
	BuildQueueAvailable() bool
	EnqueueBuildRun(ctx context.Context, payload tasks.BuildRunPayload) error
	ApplicationCanMutate(app model.Application) bool
	NormalizeDeploymentSourceType(value string) string
	NormalizeBuildResourceQuantityValue(value, fallbackValue, label string) (string, error)
	NormalizeBuildTimeoutSecondsValue(value int) int
}

type Handler struct {
	host Host
}

// Handlers keeps the migrated receiver declarations compact while Handler is
// the domain entry point used by the root API bridge.
type Handlers = Handler

func New(host Host) *Handler {
	return &Handler{host: host}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }

func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}

func (h *Handler) currentUser(ctx *gin.Context) (model.User, bool) {
	return h.host.CurrentUser(ctx)
}

func (h *Handler) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProject(ctx, action)
}

func (h *Handler) resolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return h.host.ResolveListVisibility(ctx, user)
}

func (h *Handler) applyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return h.host.ApplyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (h *Handler) normalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	return h.host.NormalizeScopedOwnerWithProjects(ctx, user, scope, ownerRef, projectIDs, globalError)
}

func (h *Handler) canManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return h.host.CanManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (h *Handler) canInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return h.host.CanInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handler) replaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return h.host.ReplaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}

func (h *Handler) scopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return h.host.ScopedResourceProjectIDs(resourceType, resourceID, ctx)
}

func (h *Handler) scopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.host.ScopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}

func (h *Handler) projectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return h.host.ProjectRoleActionAllowed(ctx, user, projectID, action)
}

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) ensureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool {
	return h.host.EnsureBillingAllowsNewBuild(ctx, projectID)
}

func (h *Handler) requireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return h.host.RequireContinuousAuthorizationBinding(ctx, user)
}

func (h *Handler) monitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.host.MonitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}

func (h *Handler) projectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return h.host.ProjectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}

func (h *Handler) registryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return h.host.RegistryPushCredentialForProject(user, registry, projectID, ctx)
}

func (h *Handler) storeSecret(ctx context.Context, value, userID, resource string) string {
	return h.host.StoreSecret(ctx, value, userID, resource)
}

func (h *Handler) resolveSecret(ctx context.Context, ref string) string {
	return h.host.ResolveSecret(ctx, ref)
}

func (h *Handler) applicationCanMutate(app model.Application) bool {
	return h.host.ApplicationCanMutate(app)
}

func (h *Handler) normalizeDeploymentSourceType(value string) string {
	return h.host.NormalizeDeploymentSourceType(value)
}

func (h *Handler) normalizeBuildResourceQuantityValue(value, fallbackValue, label string) (string, error) {
	return h.host.NormalizeBuildResourceQuantityValue(value, fallbackValue, label)
}

func (h *Handler) normalizeBuildTimeoutSecondsValue(value int) int {
	return h.host.NormalizeBuildTimeoutSecondsValue(value)
}

type paginationParams = transportapi.PaginationParams

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }

func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}

func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}

func writeLocalizedErrorCode(ctx *gin.Context, status int, code, detail, publicMessageKey string) {
	transportapi.WriteLocalizedErrorCode(ctx, status, code, detail, publicMessageKey)
}

func fallback(value, defaultValue string) string {
	return transportapi.Fallback(value, defaultValue)
}

func paginationFromQueryWithSort(ctx *gin.Context, allowedFields map[string]string, defaultField string) paginationParams {
	return transportapi.PaginationFromQueryWithSort(ctx, allowedFields, defaultField)
}

func paginatedResponse[T any](items []T, total int64, pagination paginationParams) transportapi.PaginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}

func orderByClause(pagination paginationParams, allowedFields map[string]string, defaultColumn string) string {
	return transportapi.OrderByClause(pagination, allowedFields, defaultColumn)
}

func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	return transportapi.ApplySearch(ctx, query, columns...)
}

func normalizeStringList(values []string) []string {
	return transportapi.NormalizeStringList(values)
}

func jsonList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func sortedProjectIDs(projectIDs []string) []string {
	result := normalizeStringList(projectIDs)
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (h *Handler) writeProjectAuthorizationError(ctx *gin.Context, err error) {
	h.host.WriteProjectAuthorizationError(ctx, err)
}

func writeContinuousAuthorizationRevoked(ctx *gin.Context) {
	projectapi.WriteContinuousAuthorizationRevoked(ctx)
}

func replaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	return projectapi.ReplaceRequestContext(ctx, requestCtx)
}
