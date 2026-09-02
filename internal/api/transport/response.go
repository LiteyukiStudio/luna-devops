package transport

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/credential"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

const (
	requestIDContextKey   = "luna_request_id"
	runtimeModeContextKey = "luna_runtime_mode"
)

const terminalDisconnectedErrorCode = "runtime.terminal_disconnected"

// Keep this transport message key local so the response package remains a
// leaf and does not depend on the build API adapter.
const buildPushCredentialRequiredCode = "build.registry_push_credential_required"

func requestIDMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestID := id.New("req")
		ctx.Set(requestIDContextKey, requestID)
		ctx.Header("X-Request-ID", requestID)
		ctx.Request = ctx.Request.WithContext(telemetry.ContextWithRequestID(ctx.Request.Context(), requestID))
		ctx.Next()
	}
}

func runtimeModeMiddleware(mode string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		setRuntimeMode(ctx, mode)
		ctx.Next()
	}
}

func setRuntimeMode(ctx *gin.Context, mode string) {
	if ctx == nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(mode), "development") {
		ctx.Set(runtimeModeContextKey, "development")
		return
	}
	ctx.Set(runtimeModeContextKey, "production")
}

func isDevelopmentRequest(ctx *gin.Context) bool {
	if ctx == nil {
		return false
	}
	mode, exists := ctx.Get(runtimeModeContextKey)
	return exists && mode == "development"
}

func requestID(ctx *gin.Context) string {
	if value, exists := ctx.Get(requestIDContextKey); exists {
		if requestID, ok := value.(string); ok && requestID != "" {
			return requestID
		}
	}
	requestID := id.New("req")
	ctx.Set(requestIDContextKey, requestID)
	ctx.Header("X-Request-ID", requestID)
	return requestID
}

func bindJSON(ctx *gin.Context, value any) bool {
	if err := ctx.ShouldBindJSON(value); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", err.Error())
		return false
	}
	return true
}

func writeError(ctx *gin.Context, status int, message string) {
	code := defaultErrorCode(status)
	if code == "internal_error" {
		code = internalErrorCode(ctx)
	}
	writeErrorCode(ctx, status, code, message)
}

func internalErrorCode(ctx *gin.Context) string {
	route := strings.TrimSpace(ctx.FullPath())
	if route == "" {
		return "internal_error"
	}
	var code strings.Builder
	code.WriteString("internal_error.")
	code.WriteString(strings.ToLower(ctx.Request.Method))
	for _, char := range route {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			code.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			code.WriteRune(char + ('a' - 'A'))
		default:
			if code.Len() > 0 && !strings.HasSuffix(code.String(), "_") {
				code.WriteByte('_')
			}
		}
	}
	return strings.TrimSuffix(code.String(), "_")
}

func writeErrorKey(ctx *gin.Context, status int, _ string, key string) {
	telemetry.SetHTTPError(ctx, key, key)
	ctx.JSON(status, errorEnvelope(ctx, status, key))
}

func writeErrorKeyWithDetails(
	ctx *gin.Context,
	status int,
	_ string, key string,
	details gin.H,
) {
	telemetry.SetHTTPError(ctx, key, key)
	response := errorEnvelope(ctx, status, key)
	if isDevelopmentRequest(ctx) {
		response["details"] = details
	}
	ctx.JSON(status, response)
}

func writeScopeInsufficientError(ctx *gin.Context, requiredScope string) {
	const code = "auth.token.scope_insufficient"
	telemetry.SetHTTPError(ctx, code, code)
	response := errorEnvelope(ctx, http.StatusForbidden, code)
	response["requiredScope"] = strings.TrimSpace(requiredScope)
	ctx.JSON(http.StatusForbidden, response)
}

func writeScopeContractUnavailableError(ctx *gin.Context, detail string) {
	writeErrorCode(
		ctx,
		http.StatusServiceUnavailable,
		"auth.token.scope_contract_unavailable",
		detail,
	)
}

func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	if code == "" {
		code = defaultErrorCode(status)
	}
	detail = telemetry.RedactText(detail)
	telemetry.SetHTTPError(ctx, code, detail)
	if isDevelopmentRequest(ctx) {
		response := errorEnvelope(ctx, status, code)
		response["developerDetail"] = detail
		ctx.JSON(status, response)
		return
	}
	ctx.JSON(status, errorEnvelope(ctx, status, code))
}

func writeArgumentErrorCode(ctx *gin.Context, status int, code, detail, path string, allowedValues []string, retryable bool) {
	if code == "" {
		code = defaultErrorCode(status)
	}
	detail = telemetry.RedactText(detail)
	telemetry.SetHTTPError(ctx, code, detail)
	response := errorEnvelope(ctx, status, code)
	response["retryable"] = retryable
	response["path"] = path
	response["allowedValues"] = allowedValues
	if isDevelopmentRequest(ctx) {
		response["developerDetail"] = detail
	}
	ctx.JSON(status, response)
}

// writeLocalizedErrorCode exposes a stable frontend message key in production
// while preserving the original credential-redacted diagnostic in development.
func writeLocalizedErrorCode(ctx *gin.Context, status int, code, detail, publicMessageKey string) {
	if isDevelopmentRequest(ctx) {
		writeErrorCode(ctx, status, code, detail)
		return
	}
	if code == "" {
		code = defaultErrorCode(status)
	}
	telemetry.SetHTTPError(ctx, code, detail)
	response := errorEnvelope(ctx, status, code)
	response["message"] = "errors." + publicMessageKey
	ctx.JSON(status, response)
}

func errorEnvelope(ctx *gin.Context, status int, code string) gin.H {
	response := gin.H{
		"code":      code,
		"message":   publicErrorMessageKey(status),
		"requestId": requestID(ctx),
	}
	if traceID := traceID(ctx); traceID != "" {
		response["traceId"] = traceID
	}
	return response
}

func traceID(ctx *gin.Context) string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	spanContext := trace.SpanContextFromContext(ctx.Request.Context())
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String()
}

func publicErrorMessageKey(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "errors.request.invalid"
	case http.StatusUnauthorized:
		return "errors.auth.unauthorized"
	case http.StatusForbidden:
		return "errors.auth.forbidden"
	case http.StatusNotFound:
		return "errors.resource.not_found"
	case http.StatusConflict:
		return "errors.resource.conflict"
	case http.StatusTooManyRequests:
		return "errors.rate_limited"
	default:
		if status >= 500 {
			return "errors.internal_error"
		}
		return "errors.request.failed"
	}
}

func errorResponseMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Next()
		if len(ctx.Errors) == 0 || ctx.Writer.Written() {
			return
		}
		writeErrorCode(ctx, http.StatusInternalServerError, "internal_error", ctx.Errors.Last().Error())
	}
}

func recoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(ctx *gin.Context, recovered any) {
		writeErrorCode(ctx, http.StatusInternalServerError, "internal_error", fmt.Sprint(recovered))
		ctx.Abort()
	})
}

func terminalDisconnectedMessage(ctx *gin.Context, detail string) []byte {
	if isDevelopmentRequest(ctx) {
		return []byte("\r\nterminal disconnected: " + detail + "\r\n")
	}
	return []byte(fmt.Sprintf(
		"\r\n%s (code=%s, requestId=%s)\r\n",
		messageFor(requestLanguage(ctx), terminalDisconnectedErrorCode),
		terminalDisconnectedErrorCode,
		requestID(ctx),
	))
}

func defaultErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "request.invalid"
	case http.StatusUnauthorized:
		return "auth.unauthorized"
	case http.StatusForbidden:
		return "auth.forbidden"
	case http.StatusNotFound:
		return "resource.not_found"
	case http.StatusConflict:
		return "resource.conflict"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusBadGateway:
		return "upstream.failed"
	default:
		if status >= 500 {
			return "internal_error"
		}
		return "request.failed"
	}
}

func messageFor(language, key string) string {
	messages := localizedMessages[normalizeLanguage(language)]
	if message, ok := messages[key]; ok {
		return message
	}
	return localizedMessages["zh-CN"][key]
}

func normalizeLanguage(language string) string {
	if language == "en-US" {
		return "en-US"
	}
	return "zh-CN"
}

func requestLanguage(ctx *gin.Context) string {
	if ctx == nil || ctx.Request == nil {
		return "zh-CN"
	}
	if strings.Contains(strings.ToLower(ctx.GetHeader("Accept-Language")), "en") {
		return "en-US"
	}
	return "zh-CN"
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func fallbackInt(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

func randomHex(length int) string {
	return credential.RandomHex(length)
}

func hashToken(token string) string {
	return credential.Hash(token)
}

var localizedMessages = map[string]map[string]string{
	"zh-CN": {
		"auth.login.invalid":               "邮箱或密码不正确",
		"auth.session.missing":             "请先登录",
		"auth.session.expired":             "登录会话已过期，请重新登录",
		"auth.account.disabled":            "账号不可用，请联系平台管理员",
		"auth.token.invalid":               "访问凭据无效、已过期或已被吊销，请重新登录",
		"auth.token.scope_insufficient":    "当前访问凭据没有执行该操作所需的权限",
		"auth.oauth.grant_invalid":         "OAuth 授权已失效，请重新登录",
		"auth.oauth.application_invalid":   "OAuth 应用不可用，请重新登录",
		terminalDisconnectedErrorCode:      "终端连接已断开",
		"service_binding_in_use":           "该应用或部署配置仍被服务关系引用，请先删除相关关系",
		"application.delete_in_progress":   "应用正在删除中，请等待资源清理完成或删除失败后重试。",
		"config.admin.required":            "只有平台管理员可以修改站点配置",
		"ai.user_not_allowed":              "当前站点仅允许平台管理员使用 AI 助手",
		"git.network_failed":               "Git 平台连接失败，请检查服务端网络、代理/VPN、DNS 解析或 FakeIP 设置后重试。",
		"git.upstream_failed":              "Git 上游接口调用失败，请稍后重试",
		"git.permission_denied":            "Git 凭据权限不足，无法访问该仓库或配置 Webhook。请检查凭据权限后重试。",
		"git.repository_not_found":         "Git 仓库不存在，或当前凭据无权访问该仓库。请检查仓库和 Git 凭据。",
		"git.validation_failed":            "Git 平台拒绝了本次配置请求。请检查仓库、Webhook 地址和凭据权限。",
		"git.webhook_callback_unreachable": "Webhook 回调地址无法被 GitHub/Gitea 从公网访问。请配置可公网访问的 PUBLIC_BASE_URL 后重新配置 Webhook。",
		"git.webhook_callback_invalid":     "Webhook 回调地址格式无效。请配置以 http/https 开头的 PUBLIC_BASE_URL 后重新配置 Webhook。",
		"git.webhook_already_exists":       "该仓库可能已经存在相同回调地址的 Webhook。请在 Git 平台确认后重试或使用现有 Webhook。",
		"git.webhook_rate_limited":         "Git 平台暂时限制了 Webhook 创建请求，请稍后重试。",
		"git.provider_required":            "公开仓库访问需要指定 Git 平台。请先选择 Git 平台。",
		"registry.authentication_required": "该镜像站或仓库需要登录凭据才能访问。请在凭据管理中配置拉取凭据后重试。",
		buildPushCredentialRequiredCode:    "目标镜像站没有当前用户或项目空间可用的推送凭据。请先绑定 push 或 push-pull 凭据后重试。",
	},
	"en-US": {
		"auth.login.invalid":               "Email or password is incorrect",
		"auth.session.missing":             "Please sign in first",
		"auth.session.expired":             "Your session has expired. Please sign in again",
		"auth.account.disabled":            "This account is unavailable. Contact a platform administrator",
		"auth.token.invalid":               "The access credential is invalid, expired, or revoked. Sign in again",
		"auth.token.scope_insufficient":    "The access credential does not have the permission required for this operation",
		"auth.oauth.grant_invalid":         "The OAuth authorization is no longer valid. Sign in again",
		"auth.oauth.application_invalid":   "The OAuth application is unavailable. Sign in again",
		terminalDisconnectedErrorCode:      "The terminal connection was closed.",
		"service_binding_in_use":           "This application or deployment target is still referenced by a service relation. Remove the relation first.",
		"application.delete_in_progress":   "The application is being deleted. Wait for resource cleanup to finish, or retry after deletion fails.",
		"project.delete_in_progress":       "The project space is being deleted. Wait for resource cleanup to finish.",
		"config.admin.required":            "Only platform administrators can update site settings",
		"ai.user_not_allowed":              "This site restricts AI assistant access to platform administrators",
		"git.network_failed":               "Failed to connect to the Git platform. Check server network, proxy/VPN, DNS resolution, or FakeIP settings and try again.",
		"git.upstream_failed":              "Git upstream request failed. Try again later.",
		"git.permission_denied":            "The Git credential does not have enough permission to access the repository or configure the webhook. Check its permissions and try again.",
		"git.repository_not_found":         "The Git repository does not exist, or the current credential cannot access it. Check the repository and Git credential.",
		"git.validation_failed":            "The Git platform rejected this configuration request. Check the repository, webhook callback URL, and credential permissions.",
		"git.webhook_callback_unreachable": "The webhook callback URL cannot be reached by GitHub/Gitea from the public Internet. Configure a public PUBLIC_BASE_URL and reconfigure the webhook.",
		"git.webhook_callback_invalid":     "The webhook callback URL is invalid. Configure PUBLIC_BASE_URL with an http/https URL and reconfigure the webhook.",
		"git.webhook_already_exists":       "A webhook with the same callback URL may already exist in this repository. Check the Git platform and retry or use the existing webhook.",
		"git.webhook_rate_limited":         "The Git platform is temporarily limiting webhook creation requests. Try again later.",
		"git.provider_required":            "A Git provider must be selected when browsing public repositories. Please pick a Git provider first.",
		"registry.authentication_required": "This registry or repository requires credentials. Configure a pull credential in Credential Management and try again.",
		buildPushCredentialRequiredCode:    "The target registry has no push credential available to this user or project space. Bind a push or push-pull credential and try again.",
	},
}

const TerminalDisconnectedErrorCode = terminalDisconnectedErrorCode

func RequestIDMiddleware() gin.HandlerFunc { return requestIDMiddleware() }

func RuntimeModeMiddleware(mode string) gin.HandlerFunc { return runtimeModeMiddleware(mode) }

func SetRuntimeMode(ctx *gin.Context, mode string) { setRuntimeMode(ctx, mode) }

func IsDevelopmentRequest(ctx *gin.Context) bool { return isDevelopmentRequest(ctx) }

func RequestID(ctx *gin.Context) string { return requestID(ctx) }

func BindJSON(ctx *gin.Context, value any) bool { return bindJSON(ctx, value) }

func WriteError(ctx *gin.Context, status int, message string) { writeError(ctx, status, message) }

func InternalErrorCode(ctx *gin.Context) string { return internalErrorCode(ctx) }

func WriteErrorKey(ctx *gin.Context, status int, message, key string) {
	writeErrorKey(ctx, status, message, key)
}

func WriteErrorKeyWithDetails(ctx *gin.Context, status int, message, key string, details gin.H) {
	writeErrorKeyWithDetails(ctx, status, message, key, details)
}

func WriteScopeInsufficientError(ctx *gin.Context, requiredScope string) {
	writeScopeInsufficientError(ctx, requiredScope)
}

func WriteScopeContractUnavailableError(ctx *gin.Context, detail string) {
	writeScopeContractUnavailableError(ctx, detail)
}

func WriteErrorCode(ctx *gin.Context, status int, code, detail string) {
	writeErrorCode(ctx, status, code, detail)
}

func WriteArgumentErrorCode(ctx *gin.Context, status int, code, detail, path string, allowedValues []string, retryable bool) {
	writeArgumentErrorCode(ctx, status, code, detail, path, allowedValues, retryable)
}

func WriteLocalizedErrorCode(ctx *gin.Context, status int, code, detail, publicMessageKey string) {
	writeLocalizedErrorCode(ctx, status, code, detail, publicMessageKey)
}

func ErrorEnvelope(ctx *gin.Context, status int, code string) gin.H {
	return errorEnvelope(ctx, status, code)
}

func TraceID(ctx *gin.Context) string { return traceID(ctx) }

func PublicErrorMessageKey(status int) string { return publicErrorMessageKey(status) }

func ErrorResponseMiddleware() gin.HandlerFunc { return errorResponseMiddleware() }

func RecoveryMiddleware() gin.HandlerFunc { return recoveryMiddleware() }

func TerminalDisconnectedMessage(ctx *gin.Context, detail string) []byte {
	return terminalDisconnectedMessage(ctx, detail)
}

func DefaultErrorCode(status int) string { return defaultErrorCode(status) }

func MessageFor(language, key string) string { return messageFor(language, key) }

func NormalizeLanguage(language string) string { return normalizeLanguage(language) }

func RequestLanguage(ctx *gin.Context) string { return requestLanguage(ctx) }

func Fallback(value, defaultValue string) string { return fallback(value, defaultValue) }

func FallbackInt(value, defaultValue int) int { return fallbackInt(value, defaultValue) }

func RandomHex(length int) string { return randomHex(length) }

func HashToken(token string) string { return hashToken(token) }
