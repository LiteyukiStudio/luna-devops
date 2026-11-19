package identityapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	oauthAuthorizationCodeTTL = 5 * time.Minute
	oauthRefreshTokenTTL      = 365 * 24 * time.Hour
	oauthCredentialRateLimit  = 30
	oauthIPRateLimit          = 300
	oauthRateLimitWindow      = time.Minute
)

var allowedOAuthAccessTokenLifetimeDays = map[int]bool{0: true, 1: true, 7: true, 30: true, 90: true}

type oauthClientAuthentication struct {
	applicationID    string
	clientID         string
	clientSecretHash string
	public           bool
}

func encodeStringList(values []string) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

func decodeStringList(value string) []string {
	var values []string
	if json.Unmarshal([]byte(value), &values) != nil {
		return nil
	}
	return values
}

func normalizeOAuthRedirectURIs(values []string) ([]string, bool) {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] || !validOAuthRedirectURI(value) {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized, len(normalized) > 0 && len(normalized) <= 10
}

func validOAuthRedirectURI(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Fragment != "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	hostname := strings.ToLower(parsed.Hostname())
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}

func validOptionalHTTPURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.User == nil
}

func exactRedirectURIAllowed(application model.OAuthApplication, redirectURI string) bool {
	for _, allowed := range decodeStringList(application.RedirectURIs) {
		if subtle.ConstantTimeCompare([]byte(allowed), []byte(redirectURI)) == 1 {
			return true
		}
	}
	return false
}

func oauthScopeSubset(requested, allowed string) bool {
	allowedSet := map[string]bool{}
	for _, scope := range splitOAuthScopes(allowed) {
		allowedSet[scope] = true
	}
	for _, scope := range splitOAuthScopes(requested) {
		if !allowedSet[scope] {
			return false
		}
	}
	return true
}

func splitOAuthScopes(scopeText string) []string {
	return strings.FieldsFunc(scopeText, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}

func oauthScopeText(scopeText string) string {
	return strings.Join(splitOAuthScopes(scopeText), " ")
}

func validPKCEChallenge(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func verifyPKCE(verifier, challenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	actual := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(actual), []byte(challenge)) == 1
}

func appendOAuthRedirectValues(redirectURI string, values url.Values) string {
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		return ""
	}
	query := parsed.Query()
	for key, items := range values {
		for _, item := range items {
			query.Set(key, item)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (h *Handlers) oauthCookieUser(ctx *gin.Context) (model.User, bool) {
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.ImpersonatorID != "" {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.missing")
		return model.User{}, false
	}
	var user model.User
	if err := h.dbFor(ctx).First(&user, "id = ? and disabled = ?", session.UserID, false).Error; err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}
	return user, true
}

func oauthError(ctx *gin.Context, status int, code, description string) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(status, oauthProtocolErrorResponse{
		Code:        code,
		Description: description,
		RequestID:   requestID(ctx),
	})
}

func bindOAuthForm(ctx *gin.Context, value any) bool {
	if err := ctx.ShouldBind(value); err != nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_request", "OAuth form request is invalid")
		return false
	}
	return true
}

func (h *Handlers) authenticateOAuthTokenClient(
	ctx *gin.Context,
	_ bool,
	flow oauthClientAttemptFlow,
	formClientID string,
	formClientSecret string,
	credential string,
) (oauthClientAuthentication, bool) {
	clientID, clientSecret, basicOK := ctx.Request.BasicAuth()
	if !basicOK {
		clientID = strings.TrimSpace(formClientID)
		clientSecret = formClientSecret
	}
	if !h.allowOAuthTokenClientAttempt(ctx, clientID, flow, credential) {
		return oauthClientAuthentication{}, false
	}
	var application model.OAuthApplication
	if clientID == "" || h.dbFor(ctx).First(&application, "client_id = ? and revoked_at is null", clientID).Error != nil {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return oauthClientAuthentication{}, false
	}
	clientSecretHash := hashToken(clientSecret)
	if clientSecret == "" || subtle.ConstantTimeCompare([]byte(application.ClientSecretHash), []byte(clientSecretHash)) != 1 {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return oauthClientAuthentication{}, false
	}
	return oauthClientAuthentication{
		applicationID: application.ID, clientID: application.ClientID, clientSecretHash: clientSecretHash,
	}, true
}

func normalizeOAuthScope(scopeText string) string {
	return authz.NormalizeOAuthScope(scopeText)
}

func userCanAuthorizeOAuthScope(user model.User, scopeText string) bool {
	return authz.UserCanAuthorizeOAuthScope(user.Role, scopeText)
}

type oauthClientAttemptFlow string

const (
	oauthClientAttemptAuthorizationCode oauthClientAttemptFlow = "authorization_code"
	oauthClientAttemptRefresh           oauthClientAttemptFlow = "refresh"
	oauthClientAttemptRevoke            oauthClientAttemptFlow = "revoke"
	oauthClientAttemptUnsupported       oauthClientAttemptFlow = "unsupported"
)

func oauthClientAttemptFlowForGrantType(grantType string) oauthClientAttemptFlow {
	switch grantType {
	case "authorization_code":
		return oauthClientAttemptAuthorizationCode
	case "refresh_token":
		return oauthClientAttemptRefresh
	default:
		return oauthClientAttemptUnsupported
	}
}

func (h *Handlers) allowOAuthTokenClientAttempt(ctx *gin.Context, clientID string, flow oauthClientAttemptFlow, credential string) bool {
	return h.allowOAuthClientAttemptForFlow(ctx, clientID, flow, credential)
}

func (h *Handlers) allowOAuthClientAttemptForFlow(ctx *gin.Context, clientID string, _ oauthClientAttemptFlow, _ string) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	credentialLimit := oauthCredentialRateLimit
	ipLimit := oauthIPRateLimit
	if h.mode == "development" {
		credentialLimit = developmentRateLimit
		ipLimit = developmentRateLimit
	}
	type oauthRateLimitAttempt struct {
		subject string
		limit   int
	}
	sourceIP := h.oauthClientSourceIP(ctx)
	attempts := []oauthRateLimitAttempt{
		{subject: "oauth_client_ip:" + sourceIP, limit: ipLimit},
		{subject: "oauth_client_id:" + hashToken(strings.TrimSpace(clientID)), limit: credentialLimit},
	}
	for _, attempt := range attempts {
		allowed, err := h.rateLimiter.Allow(ctx.Request.Context(), attempt.subject, attempt.limit, oauthRateLimitWindow)
		if allowed {
			continue
		}
		if err != nil && h.mode == "development" {
			continue
		}
		writeOAuthRateLimitError(ctx, "temporarily_unavailable", "Client authentication is temporarily rate limited")
		return false
	}
	return true
}

func (h *Handlers) oauthClientSourceIP(ctx *gin.Context) string {
	remoteText := strings.TrimSpace(ctx.RemoteIP())
	remoteIP := net.ParseIP(remoteText)
	clientIP := net.ParseIP(ctx.ClientIP())
	if remoteIP == nil {
		if remoteText == "" {
			return "unknown"
		}
		return remoteText
	}
	if clientIP != nil && !remoteIP.Equal(clientIP) &&
		ipInCIDRs(remoteIP, h.config.TrustedProxyCIDRs) &&
		!ipInCIDRs(clientIP, h.config.TrustedProxyCIDRs) {
		return clientIP.String()
	}
	return remoteIP.String()
}

func ipInCIDRs(ip net.IP, cidrs []string) bool {
	for _, value := range cidrs {
		_, network, err := net.ParseCIDR(value)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func writeOAuthRateLimitError(ctx *gin.Context, code, description string) {
	ctx.Header("Retry-After", "60")
	oauthError(ctx, http.StatusTooManyRequests, code, description)
}

const errOAuthInvalidScope oauthSentinelError = "invalid OAuth scope"
