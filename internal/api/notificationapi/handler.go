package notificationapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var ErrInboxDecisionUnavailable = errors.New("inbox decision handler is unavailable")

type SecretStore interface {
	ResolveContext(ctx context.Context, ref string) string
	StoreContext(ctx context.Context, plaintext, createdBy, resource string) string
	StoreContextWithDB(ctx context.Context, db *gorm.DB, plaintext, createdBy, resource string) (string, error)
	DeleteRefContextWithDB(ctx context.Context, db *gorm.DB, ref, resource string) error
}

type InboxService interface {
	List(ctx context.Context, input inbox.ListInput) (inbox.ListResult, error)
	Get(ctx context.Context, userID, messageID string) (model.InboxMessage, error)
	GetActionRequest(ctx context.Context, userID, requestID string) (model.InboxActionRequest, error)
	GetActionRequests(ctx context.Context, userID string, requestIDs []string) (map[string]model.InboxActionRequest, error)
	UnreadCount(ctx context.Context, userID string) (int64, error)
	MarkRead(ctx context.Context, userID, messageID string) error
	MarkAllRead(ctx context.Context, userID string) error
	Archive(ctx context.Context, userID, messageID string) error
}

type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	RequirePlatformAdmin(ctx *gin.Context) bool
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	Secrets() SecretStore
	NormalizeRegistrationEmail(value string) (string, error)
	AllowPersonalNotificationTest(ctx *gin.Context, userID string) bool
	InboxService() InboxService
	InboxDecisionAvailable() bool
	DecideInboxAction(ctx context.Context, user model.User, requestID, decision string, expectedVersion int64) error
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

func (h *Handler) requirePlatformAdmin(ctx *gin.Context) bool {
	return h.host.RequirePlatformAdmin(ctx)
}

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) secrets() SecretStore {
	return h.host.Secrets()
}

func (h *Handler) normalizedRegistrationEmail(value string) (string, error) {
	return h.host.NormalizeRegistrationEmail(value)
}

func (h *Handler) allowPersonalNotificationTest(ctx *gin.Context, userID string) bool {
	return h.host.AllowPersonalNotificationTest(ctx, userID)
}

func (h *Handler) inboxService() InboxService {
	return h.host.InboxService()
}

func (h *Handler) inboxDecisionAvailable() bool {
	return h.host.InboxDecisionAvailable()
}

func (h *Handler) decideInboxAction(ctx context.Context, user model.User, requestID, decision string, expectedVersion int64) error {
	return h.host.DecideInboxAction(ctx, user, requestID, decision, expectedVersion)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func decodeStringList(value string) []string {
	var values []string
	if json.Unmarshal([]byte(value), &values) != nil {
		return nil
	}
	return values
}
