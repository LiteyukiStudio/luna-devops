package registryapi

import (
	"context"
	"sort"
	"strings"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	scopedResourceArtifactRegistry   = "artifact_registry"
	scopedResourceRegistryCredential = "registry_credential"
)

// Host exposes the platform capabilities required by the registry HTTP domain.
// It intentionally contains no dependency on the root api package.
type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool)
	ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool)
	ApplyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User, ctx context.Context) *gorm.DB
	ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB
	NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool)
	NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool)
	CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool
	CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error)
	CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool
	ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error
	ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string
	ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string
	ScopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string
	FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool)
	ProjectIDsForUser(ctx context.Context, userID string) []string
	AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool)
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	WriteProjectAuthorizationError(ctx *gin.Context, err error)
	SecretAvailable() bool
	StoreSecret(ctx context.Context, value, userID, resource string) string
	ResolveSecret(ctx context.Context, ref string) string
	EgressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy
	AllowRegistrySearch(ctx *gin.Context, userID string) bool
}

type Handler struct {
	host Host
}

func New(host Host) *Handler {
	return &Handler{host: host}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB {
	return h.host.DBFor(ctx)
}

func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}

func (h *Handler) currentUser(ctx *gin.Context) (model.User, bool) {
	return h.host.CurrentUser(ctx)
}

func (h *Handler) resolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return h.host.ResolveListVisibility(ctx, user)
}

func (h *Handler) applyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return h.host.ApplyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (h *Handler) applyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User, ctx context.Context) *gorm.DB {
	return h.host.ApplyScopedResourceVisibilityForUser(query, resourceType, user, ctx)
}

func (h *Handler) applyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return h.host.ApplyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}

func (h *Handler) normalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.host.NormalizeScopedOwnerWithProjects(ctx, user, rawScope, rawOwnerRef, rawProjectIDs, globalError)
}

func (h *Handler) normalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.host.NormalizeCredentialScopeWithinParent(ctx, user, rawScope, rawProjectIDs, parentScope, parentProjectIDs, globalError)
}

func (h *Handler) canManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return h.host.CanManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (h *Handler) canInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return h.host.CanInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handler) canUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return h.host.CanUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
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

func (h *Handler) scopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.host.ScopedResourceDefaultProjectIDMap(resourceType, resourceIDs, ctx)
}

func (h *Handler) findProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return h.host.FindProjectForCurrentUserByID(ctx, projectID)
}

func (h *Handler) projectIDsForUser(ctx context.Context, userID string) []string {
	return h.host.ProjectIDsForUser(ctx, userID)
}

func (h *Handler) authorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProjectByID(ctx, projectID, action)
}

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) writeProjectAuthorizationError(ctx *gin.Context, err error) {
	h.host.WriteProjectAuthorizationError(ctx, err)
}

func (h *Handler) secretAvailable() bool {
	return h.host.SecretAvailable()
}

func (h *Handler) storeSecret(ctx context.Context, value, userID, resource string) string {
	return h.host.StoreSecret(ctx, value, userID, resource)
}

func (h *Handler) resolveSecret(ctx context.Context, ref string) string {
	return h.host.ResolveSecret(ctx, ref)
}

func (h *Handler) egressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy {
	return h.host.EgressPolicyForUser(user, ctx)
}

type paginationParams = transportapi.PaginationParams
type paginatedResponseBody[T any] = transportapi.PaginatedResponseBody[T]

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }
func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}
func writeErrorKey(ctx *gin.Context, status int, message, key string) {
	transportapi.WriteErrorKey(ctx, status, message, key)
}
func paginationFromQueryWithSort(ctx *gin.Context, allowedFields map[string]string, defaultField string) paginationParams {
	return transportapi.PaginationFromQueryWithSort(ctx, allowedFields, defaultField)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) paginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func orderByClause(pagination paginationParams, allowedFields map[string]string, defaultColumn string) string {
	return transportapi.OrderByClause(pagination, allowedFields, defaultColumn)
}
func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	return transportapi.ApplySearch(ctx, query, columns...)
}
func markLiveObservationResponse(ctx *gin.Context) {
	transportapi.MarkLiveObservationResponse(ctx)
}
func fallback(value, defaultValue string) string {
	return transportapi.Fallback(value, defaultValue)
}
func requestLanguage(ctx *gin.Context) string {
	return transportapi.RequestLanguage(ctx)
}
func positiveInt(value string, fallbackValue int) int {
	return transportapi.ParsePositiveInt(value, fallbackValue)
}
func requestContext(ctx *gin.Context) context.Context {
	if ctx == nil || ctx.Request == nil {
		panic("request context is required")
	}
	return ctx.Request.Context()
}

func normalizeList(values []string, preserveCase bool) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if !preserveCase {
			normalized = strings.ToLower(normalized)
		}
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	return result
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return normalizeList(strings.Split(value, ","), false)
}

func jsonList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func sortedProjectIDs(projectIDs []string) []string {
	result := transportapi.NormalizeStringList(projectIDs)
	sort.Strings(result)
	return result
}
