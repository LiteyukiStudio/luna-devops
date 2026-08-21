package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	oauthAuthorizationCodeTTL = 5 * time.Minute
	oauthRefreshTokenTTL      = 365 * 24 * time.Hour
	oauthDeviceCodeTTL        = 10 * time.Minute
	oauthDevicePollInterval   = 5 * time.Second

	oauthDeviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"
	lunaCLIApplicationID     = "oapp_luna_cli"
	lunaCLIClientID          = "luna-cli"
)

var allowedOAuthAccessTokenLifetimeDays = map[int]bool{0: true, 1: true, 7: true, 30: true, 90: true}

type oauthApplicationResponse struct {
	model.OAuthApplication
	RedirectURIs []string `json:"redirectUris"`
}

type oauthGrantResponse struct {
	ID          string                   `json:"id"`
	Application oauthApplicationResponse `json:"application"`
	Scope       string                   `json:"scope"`
	CreatedAt   time.Time                `json:"createdAt"`
	UpdatedAt   time.Time                `json:"updatedAt"`
}

func oauthApplicationToResponse(application model.OAuthApplication) oauthApplicationResponse {
	return oauthApplicationResponse{OAuthApplication: application, RedirectURIs: decodeStringList(application.RedirectURIs)}
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
	ctx.JSON(status, gin.H{
		"error":             code,
		"error_description": description,
		"requestId":         requestID(ctx),
	})
}

func (h *Handlers) authenticateOAuthClient(ctx *gin.Context) (model.OAuthApplication, bool) {
	clientID, clientSecret, basicOK := ctx.Request.BasicAuth()
	if !basicOK {
		clientID = strings.TrimSpace(ctx.PostForm("client_id"))
		clientSecret = ctx.PostForm("client_secret")
	}
	if !h.allowOAuthClientAttempt(ctx, clientID) {
		return model.OAuthApplication{}, false
	}
	var application model.OAuthApplication
	if clientID == "" || clientSecret == "" || h.dbFor(ctx).First(&application, "client_id = ? and revoked_at is null", clientID).Error != nil {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return model.OAuthApplication{}, false
	}
	if subtle.ConstantTimeCompare([]byte(application.ClientSecretHash), []byte(hashToken(clientSecret))) != 1 {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return model.OAuthApplication{}, false
	}
	return application, true
}

func (h *Handlers) authenticateOAuthTokenClient(ctx *gin.Context, allowPublic bool) (model.OAuthApplication, bool) {
	clientID, clientSecret, basicOK := ctx.Request.BasicAuth()
	if !basicOK {
		clientID = strings.TrimSpace(ctx.PostForm("client_id"))
		clientSecret = ctx.PostForm("client_secret")
	}
	if !h.allowOAuthClientAttempt(ctx, clientID) {
		return model.OAuthApplication{}, false
	}
	var application model.OAuthApplication
	if clientID == "" || h.dbFor(ctx).First(&application, "client_id = ? and revoked_at is null", clientID).Error != nil {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return model.OAuthApplication{}, false
	}
	if allowPublic && application.ID == lunaCLIApplicationID && application.ClientID == lunaCLIClientID && clientSecret == "" && !basicOK {
		return application, true
	}
	if clientSecret == "" || subtle.ConstantTimeCompare([]byte(application.ClientSecretHash), []byte(hashToken(clientSecret))) != 1 {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return model.OAuthApplication{}, false
	}
	return application, true
}

func recommendedOAuthScope(user model.User) string {
	return authz.NormalizeOAuthScope(strings.Join(authz.RecommendedOAuthScopes(user.Role), ","))
}

func normalizeOAuthScope(scopeText string) string {
	return authz.NormalizeOAuthScope(scopeText)
}

func userCanAuthorizeOAuthScope(user model.User, scopeText string) bool {
	return authz.UserCanAuthorizeOAuthScope(user.Role, scopeText)
}

func (h *Handlers) allowOAuthClientAttempt(ctx *gin.Context, clientID string) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	limit := 30
	if h.mode == "development" {
		limit = developmentRateLimit
	}
	subjects := []string{
		"oauth_client_ip:" + ctx.ClientIP(),
		"oauth_client_id:" + hashToken(strings.TrimSpace(clientID)),
	}
	for _, subject := range subjects {
		allowed, err := h.rateLimiter.allow(ctx.Request.Context(), subject, limit, time.Minute)
		if allowed {
			continue
		}
		if err != nil && h.mode == "development" {
			continue
		}
		oauthError(ctx, http.StatusTooManyRequests, "temporarily_unavailable", "Client authentication is temporarily rate limited")
		return false
	}
	return true
}

func (h *Handlers) allowOAuthDeviceVerificationAttempt(ctx *gin.Context, userID string) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	limit := 30
	if h.mode == "development" {
		limit = developmentRateLimit
	}
	subjects := []string{
		"oauth_device_verify_ip:" + ctx.ClientIP(),
		"oauth_device_verify_user:" + hashToken(strings.TrimSpace(userID)),
	}
	for _, subject := range subjects {
		allowed, err := h.rateLimiter.allow(ctx.Request.Context(), subject, limit, time.Minute)
		if allowed {
			continue
		}
		if err != nil && h.mode == "development" {
			continue
		}
		writeErrorCode(ctx, http.StatusTooManyRequests, "oauth.device.rate_limited", "Device verification is temporarily rate limited")
		return false
	}
	return true
}

func revokeOAuthGrant(tx *gorm.DB, grantID string, now time.Time) error {
	if err := tx.Model(&model.OAuthGrant{}).Where("id = ? and revoked_at is null", grantID).Update("revoked_at", now).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.AccessToken{}).Where("oauth_grant_id = ? and revoked_at is null", grantID).Update("revoked_at", now).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.OAuthRefreshToken{}).Where("grant_id = ? and revoked_at is null", grantID).Update("revoked_at", now).Error; err != nil {
		return err
	}
	return nil
}

func revokeOAuthApplication(tx *gorm.DB, applicationID string, now time.Time) error {
	var grants []model.OAuthGrant
	if err := tx.Where("application_id = ? and revoked_at is null", applicationID).Find(&grants).Error; err != nil {
		return err
	}
	for _, grant := range grants {
		if err := revokeOAuthGrant(tx, grant.ID, now); err != nil {
			return err
		}
	}
	return nil
}
