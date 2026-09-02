package billingapi

import (
	"context"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	ConfigValues(keys []string) map[string]string
	DefaultRuntimeClusterID(ctx context.Context) string
	ObserveSystemComponentInstallations(ctx context.Context, items []model.SystemComponentInstallation)
	SystemComponentForBearerToken(token, componentID string, ctx context.Context) (model.SystemComponentInstallation, bool)
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

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) configValues(keys []string) map[string]string {
	return h.host.ConfigValues(keys)
}

func (h *Handler) defaultRuntimeClusterID(ctx context.Context) string {
	return h.host.DefaultRuntimeClusterID(ctx)
}

func (h *Handler) observeSystemComponentInstallations(ctx context.Context, items []model.SystemComponentInstallation) {
	h.host.ObserveSystemComponentInstallations(ctx, items)
}

func (h *Handler) systemComponentForBearerToken(token, componentID string, ctx context.Context) (model.SystemComponentInstallation, bool) {
	return h.host.SystemComponentForBearerToken(token, componentID, ctx)
}

type paginationParams = transportapi.PaginationParams
type paginatedResponseBody[T any] = transportapi.PaginatedResponseBody[T]

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
func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) paginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func orderByClause(pagination paginationParams, allowedFields map[string]string, defaultColumn string) string {
	return transportapi.OrderByClause(pagination, allowedFields, defaultColumn)
}
func markLiveObservationResponse(ctx *gin.Context) {
	transportapi.MarkLiveObservationResponse(ctx)
}
func normalizeStringList(values []string) []string {
	return transportapi.NormalizeStringList(values)
}
