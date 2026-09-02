package runtimeapi

import (
	"context"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/gatewayapi"
	"github.com/LiteyukiStudio/devops/internal/api/identityapi"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/runtimeaccess"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const scopedResourceRuntimeCluster = "runtime_cluster"

type runtimeTerminalAuthorizationBinding = projectapi.ContinuousAuthorizationBinding
type continuousAuthorizationBinding = projectapi.ContinuousAuthorizationBinding

// Host exposes only cross-domain composition capabilities required by the
// runtime HTTP domain. The runtime package never imports the root api package.
type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	RequirePlatformAdmin(ctx *gin.Context) bool
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool)
	EnsurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error)
	EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool
	EnsureRuntimeConfigSetCanMutate(ctx *gin.Context, set model.ProjectRuntimeConfigSet) bool
	DeleteStatusCanStart(status string) bool
	ResourceDeleteAlreadyStarted(err error) bool
	MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error
	MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error
	FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool)
	ProjectAuthorizer(ctx context.Context) authz.ProjectAuthorizer
	ApplyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool)
	ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool)
	NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool)
	ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error
	ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string
	ProjectIDsForUser(ctx context.Context, userID string) []string
	CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error)
	CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool
	CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool
	CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool)
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	AuditWithSafeMetadata(userID, action, resource string, success bool, message string, metadata any, ctx context.Context)
	EnqueueDeployRun(ctx context.Context, release model.Release) bool
	EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool
	ObserveDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget
	MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool)
	ContinuousAuthorizationActive(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool
	SecretStore() secret.Store
	RuntimeTerminalRedis() redis.UniversalClient
	Mode() string
	AllowedOrigin(origin string) bool
	TaskQueueAvailable() bool
	TemplateApplicationIcon(template appstore.Template) string
	ShortID(value string) string
	NextReleaseRevision(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error)
	DeploymentTargetResponse(target model.DeploymentTarget) any
	LegacyGatewayRootDomain() string
}

type Handler struct {
	host        Host
	secrets     secret.Store
	ticketRedis redis.UniversalClient
	mode        string
}

type Handlers = Handler

func New(host Host) *Handler {
	return &Handler{
		host:        host,
		secrets:     host.SecretStore(),
		ticketRedis: host.RuntimeTerminalRedis(),
		mode:        host.Mode(),
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
func (h *Handler) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProject(ctx, action)
}
func (h *Handler) authorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProjectByID(ctx, projectID, action)
}
func (h *Handler) ensurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	return h.host.EnsurePlatformSystemProject(user, ctx)
}
func (h *Handler) ensureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return h.host.EnsureProjectCanMutate(ctx, project)
}
func (h *Handler) ensureRuntimeConfigSetCanMutate(ctx *gin.Context, set model.ProjectRuntimeConfigSet) bool {
	return h.host.EnsureRuntimeConfigSetCanMutate(ctx, set)
}
func (h *Handler) deleteStatusCanStart(status string) bool {
	return h.host.DeleteStatusCanStart(status)
}
func (h *Handler) markResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	return h.host.MarkResourceDeleting(tx, resource, resourceID)
}
func (h *Handler) markResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error {
	return h.host.MarkResourceDeleteFailed(db, resource, resourceID, message)
}
func (h *Handler) legacyGatewayRootDomain() string { return h.host.LegacyGatewayRootDomain() }
func (h *Handler) findProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return h.host.FindProjectForCurrentUserByID(ctx, projectID)
}
func (h *Handler) projectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return h.host.ProjectAuthorizer(ctx)
}
func (h *Handler) applyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool) {
	return h.host.ApplyScopedResourceVisibility(ctx, query, resourceType, user, projectID)
}
func (h *Handler) applyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return h.host.ApplyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}
func (h *Handler) normalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	return h.host.NormalizeScopedOwnerWithProjects(ctx, user, scope, ownerRef, projectIDs, globalError)
}
func (h *Handler) replaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return h.host.ReplaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}
func (h *Handler) scopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return h.host.ScopedResourceProjectIDs(resourceType, resourceID, ctx)
}
func (h *Handler) projectIDsForUser(ctx context.Context, userID string) []string {
	return h.host.ProjectIDsForUser(ctx, userID)
}
func (h *Handler) canInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return h.host.CanInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}
func (h *Handler) canManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return h.host.CanManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}
func (h *Handler) canUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return h.host.CanUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}
func (h *Handler) currentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return h.host.CurrentSessionFromCookie(ctx)
}
func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}
func (h *Handler) enqueueDeployRun(ctx context.Context, release model.Release) bool {
	return h.host.EnqueueDeployRun(ctx, release)
}
func (h *Handler) enqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return h.host.EnqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}
func (h *Handler) observeDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	return h.host.ObserveDeploymentTarget(ctx, project, target)
}
func (h *Handler) monitorContinuousAuthorization(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.host.MonitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}
func (h *Handler) continuousAuthorizationActive(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return h.host.ContinuousAuthorizationActive(ctx, binding, authorizationAllowed)
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
func writeArgumentErrorCode(ctx *gin.Context, status int, code, detail, path string, allowedValues []string, retryable bool) {
	transportapi.WriteArgumentErrorCode(ctx, status, code, detail, path, allowedValues, retryable)
}
func requestLanguage(ctx *gin.Context) string { return transportapi.RequestLanguage(ctx) }
func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func paginationFromQueryWithSort(ctx *gin.Context, allowedFields map[string]string, defaultField string) paginationParams {
	return transportapi.PaginationFromQueryWithSort(ctx, allowedFields, defaultField)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) transportapi.PaginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func paginateSlice[T any](items []T, pagination paginationParams) []T {
	return transportapi.PaginateSlice(items, pagination)
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
func fallback(value, defaultValue string) string { return transportapi.Fallback(value, defaultValue) }
func fallbackInt(value, defaultValue int) int    { return transportapi.FallbackInt(value, defaultValue) }
func randomHex(length int) string                { return transportapi.RandomHex(length) }
func hashToken(value string) string              { return transportapi.HashToken(value) }
func normalizeStringList(values []string) []string {
	return transportapi.NormalizeStringList(values)
}
func terminalDisconnectedMessage(ctx *gin.Context, detail string) []byte {
	return transportapi.TerminalDisconnectedMessage(ctx, detail)
}
func resolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return projectapi.ResolveListVisibility(ctx, user)
}
func writeProjectAuthorizationError(ctx *gin.Context, err error) {
	projectapi.WriteProjectAuthorizationError(ctx, err)
}
func sortedProjectIDs(projectIDs []string) []string {
	return projectapi.SortedProjectIDs(projectIDs)
}
func normalizeOwnerScope(value string) string { return projectapi.NormalizeOwnerScope(value) }
func parseGatewayHeaderMap(value string, allowPrivileged bool) (map[string]string, error) {
	return gatewayapi.ParseGatewayHeaderMap(value, allowPrivileged)
}
func encodeGatewayHeaderMap(values map[string]string) string {
	return gatewayapi.EncodeGatewayHeaderMap(values)
}
func normalizeBuildConcurrency(value, defaultValue int) int {
	return buildapi.NormalizeBuildConcurrency(value, defaultValue)
}
func decodeSecretRefs(raw string) map[string]string { return buildapi.DecodeSecretRefs(raw) }
func isBuildEnvKey(value string) bool               { return buildapi.IsBuildEnvKey(value) }

func runtimeSecretKeys(raw string) []string {
	refs := decodeSecretRefs(raw)
	keys := make([]string, 0, len(refs))
	for key, ref := range refs {
		if isBuildEnvKey(key) && strings.TrimSpace(ref) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func runtimeConfigMap(raw string) map[string]string {
	values, err := runtimeconfig.DecodeKeyValue(raw)
	if err != nil {
		return map[string]string{}
	}
	return values
}

func resourceCanMutateDuringDelete(status string) bool {
	status = strings.TrimSpace(status)
	return status == "" || status == "active" || status == "delete_failed"
}

func runtimeWebConsoleEnabled(project model.Project, target model.DeploymentTarget) bool {
	return runtimeaccess.Enabled(project.WebConsoleEnabled, target.WebConsoleEnabled)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func setRuntimeSecretNoStoreHeaders(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store, private")
	ctx.Header("Pragma", "no-cache")
	ctx.Header("X-Content-Type-Options", "nosniff")
}

const defaultClusterBuildConcurrency = buildapi.DefaultClusterBuildConcurrency

var gatewayHostSegmentPattern = gatewayapi.GatewayHostSegmentPattern

func continuousAuthorizationBindingForAccessToken(userID string, token model.AccessToken) continuousAuthorizationBinding {
	return projectapi.ContinuousAuthorizationBindingForAccessToken(userID, token)
}
func continuousAccessTokenSubject(tokenID string) string {
	return projectapi.ContinuousAccessTokenSubject(tokenID)
}
func requestUsesBearerToken(ctx *gin.Context) bool { return identityapi.RequestUsesBearerToken(ctx) }
func currentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	return identityapi.CurrentAccessTokenFromContext(ctx)
}
func containsString(values []string, target string) bool {
	return identityapi.ContainsString(values, target)
}

const lunaCLIApplicationID = identityapi.LunaCLIApplicationID

type runtimeClusterAuditMetadata = identityapi.RuntimeClusterAuditMetadata

type safeAuditMetadata interface {
	runtimeClusterAuditMetadata
}

func auditWithSafeMetadata[T safeAuditMetadata](h *Handlers, userID, action, resource string, success bool, message string, metadata T, ctx context.Context) {
	h.host.AuditWithSafeMetadata(userID, action, resource, success, message, metadata, ctx)
}
