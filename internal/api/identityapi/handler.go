package identityapi

import (
	"context"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const rememberCookiePrefix = "lyd_remember_"
const sessionCookieName = "lyd_session"

type InitialAdminConfig = sharedconfig.InitialAdminConfig

type Limiter interface {
	Allow(context.Context, string, int, time.Duration) (bool, error)
	Reset(context.Context, string) error
}

type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentAIPlatformUser(ctx *gin.Context) (model.User, bool)
	CurrentAIPlatformSession(ctx *gin.Context) (userID, sessionID string, ok bool)
	ExternalBaseURL(ctx *gin.Context) string
	AdminConfiguredEgressContext(ctx context.Context, timeout time.Duration) context.Context
	NormalizeUserBrandColorPreset(value string) (string, bool)
	NormalizeUserInterfaceStyle(value string) (string, bool)
	Mode() string
	PublicBaseURL() string
	TrustedProxyCIDRs() []string
	RateLimiter() Limiter
	OAuthStateStore() OAuthStateStore
	SecretStore() secret.Store
}

type handlerConfig struct {
	PublicBaseURL     string
	TrustedProxyCIDRs []string
}

type Handler struct {
	host        Host
	mode        string
	config      handlerConfig
	rateLimiter Limiter
	oauthStates OAuthStateStore
	secrets     secret.Store
}

// Handlers keeps the migrated receivers unchanged while Handler remains the
// identity domain entry point used by the root API bridge.
type Handlers = Handler

func New(host Host) *Handler {
	return &Handler{
		host: host,
		mode: host.Mode(),
		config: handlerConfig{
			PublicBaseURL:     host.PublicBaseURL(),
			TrustedProxyCIDRs: append([]string(nil), host.TrustedProxyCIDRs()...),
		},
		rateLimiter: host.RateLimiter(),
		oauthStates: host.OAuthStateStore(),
		secrets:     host.SecretStore(),
	}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }

func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}

func (h *Handler) currentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	return h.host.CurrentAIPlatformUser(ctx)
}

func (h *Handler) currentAIPlatformSession(ctx *gin.Context) (string, string, bool) {
	return h.host.CurrentAIPlatformSession(ctx)
}

func (h *Handler) externalBaseURL(ctx *gin.Context) string {
	return h.host.ExternalBaseURL(ctx)
}

func (h *Handler) adminConfiguredEgressContext(ctx context.Context, timeout time.Duration) context.Context {
	return h.host.AdminConfiguredEgressContext(ctx, timeout)
}

type paginationParams = transportapi.PaginationParams

func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
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

func writeScopeInsufficientError(ctx *gin.Context, requiredScope string) {
	transportapi.WriteScopeInsufficientError(ctx, requiredScope)
}

func writeScopeContractUnavailableError(ctx *gin.Context, detail string) {
	transportapi.WriteScopeContractUnavailableError(ctx, detail)
}

func requestLanguage(ctx *gin.Context) string  { return transportapi.RequestLanguage(ctx) }
func normalizeLanguage(language string) string { return transportapi.NormalizeLanguage(language) }
func fallback(value, defaultValue string) string {
	return transportapi.Fallback(value, defaultValue)
}
func randomHex(length int) string       { return transportapi.RandomHex(length) }
func hashToken(token string) string     { return transportapi.HashToken(token) }
func requestID(ctx *gin.Context) string { return transportapi.RequestID(ctx) }
