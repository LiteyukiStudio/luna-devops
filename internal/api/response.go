package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

const requestIDContextKey = "luna_request_id"

const terminalDisconnectedErrorCode = "runtime.terminal_disconnected"

func requestIDMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestID := id.New("req")
		ctx.Set(requestIDContextKey, requestID)
		ctx.Header("X-Request-ID", requestID)
		ctx.Request = ctx.Request.WithContext(telemetry.ContextWithRequestID(ctx.Request.Context(), requestID))
		ctx.Next()
	}
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

func writeErrorKey(ctx *gin.Context, status int, language, key string) {
	ctx.JSON(status, gin.H{
		"code":      key,
		"error":     messageFor(language, key),
		"requestId": requestID(ctx),
	})
}

func writeErrorKeyWithDetails(
	ctx *gin.Context,
	status int,
	language, key string,
	details gin.H,
) {
	response := gin.H{
		"code":      key,
		"error":     messageFor(language, key),
		"requestId": requestID(ctx),
	}
	if config.RuntimeMode() == "development" {
		response["details"] = details
	}
	ctx.JSON(status, response)
}

func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	if code == "" {
		code = defaultErrorCode(status)
	}
	requestID := requestID(ctx)
	if config.RuntimeMode() == "development" {
		ctx.JSON(status, gin.H{"code": code, "error": detail, "detail": detail, "requestId": requestID})
		return
	}
	ctx.JSON(status, gin.H{
		"code":      code,
		"error":     publicErrorMessage(status, requestLanguage(ctx)),
		"requestId": requestID,
	})
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
	if config.RuntimeMode() == "development" {
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

func publicErrorMessage(status int, language string) string {
	messages := publicErrorMessages[normalizeLanguage(language)]
	if status >= 500 {
		return messages[http.StatusInternalServerError]
	}
	if message, ok := messages[status]; ok {
		return message
	}
	return messages[0]
}

var publicErrorMessages = map[string]map[int]string{
	"zh-CN": {
		0:                              "请求处理失败",
		http.StatusBadRequest:          "请求参数不正确",
		http.StatusUnauthorized:        "请先登录",
		http.StatusForbidden:           "没有权限执行该操作",
		http.StatusNotFound:            "资源不存在",
		http.StatusConflict:            "资源状态冲突",
		http.StatusTooManyRequests:     "请求过于频繁，请稍后再试",
		http.StatusBadGateway:          "上游服务调用失败，请稍后再试",
		http.StatusInternalServerError: "服务暂时不可用，请稍后再试",
	},
	"en-US": {
		0:                              "The request could not be completed.",
		http.StatusBadRequest:          "The request parameters are invalid.",
		http.StatusUnauthorized:        "Please sign in first.",
		http.StatusForbidden:           "You do not have permission to perform this action.",
		http.StatusNotFound:            "The requested resource was not found.",
		http.StatusConflict:            "The resource state conflicts with this request.",
		http.StatusTooManyRequests:     "Too many requests. Please try again later.",
		http.StatusBadGateway:          "The upstream service request failed. Please try again later.",
		http.StatusInternalServerError: "The service is temporarily unavailable. Please try again later.",
	},
}

func messageFor(language, key string) string {
	messages := localizedMessages[normalizeLanguage(language)]
	if message, ok := messages[key]; ok {
		return message
	}
	return localizedMessages["zh-CN"][key]
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
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
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
		"mfa_required":                     "需要完成敏感操作二次验证",
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
		"mfa_required":                     "Additional verification is required for this sensitive action.",
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
	},
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
