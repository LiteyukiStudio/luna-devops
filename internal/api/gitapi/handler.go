package gitapi

import (
	"context"
	"sort"
	"strings"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	scopedResourceGitProvider = "git_provider"
	scopedResourceGitAccount  = "git_account"
)

type GitOAuthStateValue struct {
	ProviderID     string `json:"providerId"`
	UserID         string `json:"userId"`
	RedirectPath   string `json:"redirectPath"`
	FrontendOrigin string `json:"frontendOrigin"`
	CallbackOrigin string `json:"callbackOrigin"`
}

type gitOAuthStateValue = GitOAuthStateValue

type OAuthStateStore interface {
	SaveGit(ctx context.Context, state string, value GitOAuthStateValue, ttl time.Duration) error
	ConsumeGit(ctx context.Context, state string) (GitOAuthStateValue, bool, error)
}

type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	RequirePlatformAdmin(ctx *gin.Context) bool
	ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool)
	ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool)
	NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool)
	NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, scope string, projectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool)
	CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool
	CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error)
	CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool
	ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error
	ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string
	ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	WriteProjectAuthorizationError(ctx *gin.Context, err error)
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	SecretStore() secret.Store
	OAuthStateStore() OAuthStateStore
	PublicBaseURL() string
	DebugLog(format string, args ...any)
	EgressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy
	EgressContextForUser(ctx context.Context, user model.User, timeout time.Duration) context.Context
	PrepareBuildRunRequest(user model.User, run *model.BuildRun, ctx context.Context) error
	QueueBuildRun(ctx context.Context, user model.User, run model.BuildRun) (model.BuildRun, error)
	DeploymentTargetMatchesBuildRun(target model.DeploymentTarget, run model.BuildRun) bool
	BuildRunActorName(user model.User) string
}

type Handler struct {
	host        Host
	secrets     secret.Store
	oauthStates OAuthStateStore
}

// Handlers keeps the migrated receivers compact while exposing Handler as the
// domain entry point used by the root API bridge.
type Handlers = Handler

func New(host Host) *Handler {
	return &Handler{
		host:        host,
		secrets:     host.SecretStore(),
		oauthStates: host.OAuthStateStore(),
	}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }
func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}
func (h *Handler) currentUser(ctx *gin.Context) (model.User, bool) {
	return h.host.CurrentUser(ctx)
}
func (h *Handler) requirePlatformAdmin(ctx *gin.Context) bool {
	return h.host.RequirePlatformAdmin(ctx)
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
func (h *Handler) normalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, scope string, projectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.host.NormalizeCredentialScopeWithinParent(ctx, user, scope, projectIDs, parentScope, parentProjectIDs, globalError)
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
func (h *Handler) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProject(ctx, action)
}
func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}
func (h *Handler) debugLog(format string, args ...any) { h.host.DebugLog(format, args...) }
func (h *Handler) egressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy {
	return h.host.EgressPolicyForUser(user, ctx)
}
func (h *Handler) egressContextForUser(ctx context.Context, user model.User, timeout time.Duration) context.Context {
	return h.host.EgressContextForUser(ctx, user, timeout)
}
func (h *Handler) prepareBuildRunRequest(user model.User, run *model.BuildRun, ctx context.Context) error {
	return h.host.PrepareBuildRunRequest(user, run, ctx)
}
func (h *Handler) queueBuildRun(ctx context.Context, user model.User, run model.BuildRun) (model.BuildRun, error) {
	return h.host.QueueBuildRun(ctx, user, run)
}

type paginationParams = transportapi.PaginationParams

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }
func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}
func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}
func writeErrorKey(ctx *gin.Context, status int, message, key string) {
	transportapi.WriteErrorKey(ctx, status, message, key)
}
func requestLanguage(ctx *gin.Context) string { return transportapi.RequestLanguage(ctx) }
func fallback(value, defaultValue string) string {
	return transportapi.Fallback(value, defaultValue)
}
func randomHex(length int) string { return transportapi.RandomHex(length) }
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
func markLiveObservationResponse(ctx *gin.Context) { transportapi.MarkLiveObservationResponse(ctx) }

func requestContext(ctx *gin.Context) context.Context { return ctx.Request.Context() }

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

func sanitizeRedirectPath(path string) string {
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") {
		return path
	}
	return "/projects"
}

func shortDebugHash(value string) string {
	if value == "" {
		return ""
	}
	hashed := transportapi.HashToken(value)
	if len(hashed) <= 12 {
		return hashed
	}
	return hashed[:12]
}
