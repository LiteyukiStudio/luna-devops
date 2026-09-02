package platformapi

import (
	"context"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Host interface {
	Config() sharedconfig.APIConfig
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool)
	ProjectIDsForUser(ctx context.Context, userID string) []string
	EnsurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error)
	ObserveRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster)
	DashboardRegistryAvailable(ctx context.Context, user model.User, registry model.ArtifactRegistry) bool
	RequirePlatformAdmin(ctx *gin.Context) bool
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	AllowRate(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type Handler struct {
	host Host
}

// Handlers keeps migrated receivers unchanged while Handler remains the
// platform-domain entry point used by the root API bridge.
type Handlers = Handler

func New(host Host) *Handler { return &Handler{host: host} }

func (h *Handler) config() sharedconfig.APIConfig  { return h.host.Config() }
func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }
func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}
func (h *Handler) currentUser(ctx *gin.Context) (model.User, bool) {
	return h.host.CurrentUser(ctx)
}
func (h *Handler) resolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return h.host.ResolveListVisibility(ctx, user)
}
func (h *Handler) projectIDsForUser(ctx context.Context, userID string) []string {
	return h.host.ProjectIDsForUser(ctx, userID)
}
func (h *Handler) ensurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	return h.host.EnsurePlatformSystemProject(user, ctx)
}
func (h *Handler) observeRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster) {
	h.host.ObserveRuntimeClusters(ctx, clusters)
}
func (h *Handler) requirePlatformAdmin(ctx *gin.Context) bool {
	return h.host.RequirePlatformAdmin(ctx)
}
func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}
func (h *Handler) allowRate(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return h.host.AllowRate(ctx, key, limit, window)
}

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }
func paginationFromQuery(ctx *gin.Context) transportapi.PaginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func paginatedResponse[T any](items []T, total int64, pagination transportapi.PaginationParams) transportapi.PaginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func orderByClause(pagination transportapi.PaginationParams, allowedFields map[string]string, defaultColumn string) string {
	return transportapi.OrderByClause(pagination, allowedFields, defaultColumn)
}
func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	return transportapi.ApplySearch(ctx, query, columns...)
}
func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}
func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}
