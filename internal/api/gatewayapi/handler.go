package gatewayapi

import (
	"context"
	"strings"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Host exposes the composition capabilities required by the gateway HTTP
// domain without creating a dependency on the root api package.
type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool
	EnsureGatewayRouteCanMutate(ctx *gin.Context, route model.GatewayRoute) bool
	DeleteStatusCanStart(status string) bool
	ResourceDeleteAlreadyStarted(err error) bool
	MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error
	MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error
	EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool
	EnqueueGatewayApply(ctx context.Context, route model.GatewayRoute, actorID string) bool
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	ProjectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool)
	SecretStore() secret.Store
	NormalizeStage(value string) string
	NormalizeGatewayPublicScheme(value string) string
	NormalizePort(value, fallbackValue int) int
	ApplicationCanMutate(application model.Application) bool
	RuntimeProjectNamespace(project model.Project) string
	DeploymentTargetResourceName(target model.DeploymentTarget) string
	RuntimeClusterForDeploymentTarget(ctx context.Context, target model.DeploymentTarget) (model.RuntimeCluster, error)
}

type Handler struct {
	host    Host
	secrets secret.Store
}

// Handlers keeps the migrated receiver names compact while Handler remains the
// public domain entry point used by the root API bridge.
type Handlers = Handler

func New(host Host) *Handler {
	return &Handler{host: host, secrets: host.SecretStore()}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }

func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}

func (h *Handler) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProject(ctx, action)
}

func (h *Handler) ensureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return h.host.EnsureProjectCanMutate(ctx, project)
}

func (h *Handler) ensureGatewayRouteCanMutate(ctx *gin.Context, route model.GatewayRoute) bool {
	return h.host.EnsureGatewayRouteCanMutate(ctx, route)
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

func (h *Handler) enqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return h.host.EnqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) projectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return h.host.ProjectMemberActionAllowed(ctx, projectID, userID, action)
}

func (h *Handler) normalizeStage(value string) string {
	return h.host.NormalizeStage(value)
}

func (h *Handler) normalizeGatewayPublicScheme(value string) string {
	return h.host.NormalizeGatewayPublicScheme(value)
}

func (h *Handler) normalizePort(value, fallbackValue int) int {
	return h.host.NormalizePort(value, fallbackValue)
}

func (h *Handler) applicationCanMutate(application model.Application) bool {
	return h.host.ApplicationCanMutate(application)
}

func (h *Handler) runtimeProjectNamespace(project model.Project) string {
	return h.host.RuntimeProjectNamespace(project)
}

func (h *Handler) deploymentTargetResourceName(target model.DeploymentTarget) string {
	return h.host.DeploymentTargetResourceName(target)
}

type paginationParams = transportapi.PaginationParams

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }

func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}

func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
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

func markLiveObservationResponse(ctx *gin.Context) {
	transportapi.MarkLiveObservationResponse(ctx)
}

func fallback(value, defaultValue string) string {
	return transportapi.Fallback(value, defaultValue)
}

func fallbackInt(value, defaultValue int) int {
	return transportapi.FallbackInt(value, defaultValue)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
