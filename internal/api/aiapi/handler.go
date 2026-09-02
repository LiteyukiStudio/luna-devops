package aiapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const AssistantEnabledConfigKey = aiAssistantEnabledConfigKey
const AccessModeConfigKey = aiAccessModeConfigKey
const TextInputLimitBytes = aiTextInputLimitBytes
const RequestBodyLimitBytes = aiRequestBodyLimitBytes
const RunIDHeader = aiRunIDHeader
const ToolCallIDHeader = aiToolCallIDHeader

type ConfigDefinition struct {
	Type    string
	Default string
	Options []string
}

type PlatformActor struct {
	UserID    string
	SessionID string
	ProjectID string
}

// Host is the narrow platform boundary used by the AI HTTP domain.
// It keeps aiapi independent from the root internal/api package.
type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool)
	SetCurrentUser(ctx *gin.Context, user model.User)
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	ConfigAvailable() bool
	ConfigValues(keys []string) map[string]string
	AllConfigValues() map[string]string
	ConfigDefinition(key string) *ConfigDefinition
	ResolveSecret(ctx context.Context, ref string) string
	AIAgent() aiagent.Client
	AIDeploymentEnabled() bool
	AIActorOverride(ctx *gin.Context) (aiagent.ActorContext, string, bool, bool)
	FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool)
	AIAgentCallbackServiceToken() string
	AIPlatformActor(ctx context.Context) (PlatformActor, bool)
	WithAIPlatformActor(ctx context.Context, actor PlatformActor) context.Context
	AIToolService() *aitool.Service
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	MonitorProjectAuthorization(ctx *gin.Context, streamCtx context.Context, user model.User, projectID string, action authz.Action, revoke func()) (<-chan struct{}, bool)
	WriteContinuousAuthorizationRevoked(ctx *gin.Context)
}

type Handler struct {
	host Host
}

func New(host Host) *Handler {
	return &Handler{host: host}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }
func (h *Handler) currentUser(ctx *gin.Context) (model.User, bool) {
	return h.host.CurrentUser(ctx)
}
func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}
func (h *Handler) configValues(keys []string) map[string]string {
	return h.host.ConfigValues(keys)
}
func (h *Handler) allConfigValues() map[string]string { return h.host.AllConfigValues() }
func (h *Handler) configDefinitionByKey(key string) *ConfigDefinition {
	return h.host.ConfigDefinition(key)
}
func (h *Handler) resolveSecret(ctx context.Context, ref string) string {
	return h.host.ResolveSecret(ctx, ref)
}

type paginationParams = transportapi.PaginationParams

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }
func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}
func writeErrorKey(ctx *gin.Context, status int, message, key string) {
	transportapi.WriteErrorKey(ctx, status, message, key)
}
func requestLanguage(ctx *gin.Context) string { return transportapi.RequestLanguage(ctx) }
func requestID(ctx *gin.Context) string       { return transportapi.RequestID(ctx) }
func errorEnvelope(ctx *gin.Context, status int, code string) gin.H {
	return transportapi.ErrorEnvelope(ctx, status, code)
}
func isDevelopmentRequest(ctx *gin.Context) bool { return transportapi.IsDevelopmentRequest(ctx) }
func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func requestContext(ctx *gin.Context) context.Context {
	if ctx == nil || ctx.Request == nil {
		panic("request context is required")
	}
	return ctx.Request.Context()
}

func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func isBooleanConfigValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "1", "0", "yes", "no", "on", "off", "enabled", "disabled":
		return true
	default:
		return false
	}
}

func configOptionAllowed(value string, options []string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func splitConfigList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			items = append(items, field)
		}
	}
	return items
}

func replaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	original := ctx.Request
	ctx.Request = original.WithContext(requestCtx)
	return func() { ctx.Request = original }
}

func writeSSE(writer http.ResponseWriter, event, idValue string, data any) {
	payload, _ := json.Marshal(data)
	if idValue != "" {
		_, _ = fmt.Fprintf(writer, "id: %s\n", idValue)
	}
	if event != "" {
		_, _ = fmt.Fprintf(writer, "event: %s\n", event)
	}
	_, _ = fmt.Fprintf(writer, "data: %s\n\n", payload)
}

func flushSSE(writer http.ResponseWriter) {
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
}
