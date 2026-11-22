package identityapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

const (
	oauthAuthorizationCodeTTL             = 5 * time.Minute
	oauthRefreshTokenTTL                  = 365 * 24 * time.Hour
	oauthCredentialRateLimit              = 30
	oauthIPRateLimit                      = 300
	oauthDynamicRegistrationIPRateLimit   = 20
	oauthDynamicRegistrationBurstLimit    = 200
	oauthDynamicRegistrationMaxBodyBytes  = 16 * 1024
	oauthRateLimitWindow                  = time.Minute
	oauthDefaultDynamicClientLifetimeDays = 30
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
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if !validManualOAuthRedirectURI(value) {
			return nil, false
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized, len(normalized) > 0 && len(normalized) <= 10
}

func validateDynamicClientRedirectURIs(values []string) ([]string, bool) {
	if len(values) == 0 || len(values) > 10 {
		return nil, false
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || value != raw || !validDynamicOAuthRedirectURI(value) {
			return nil, false
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized, len(normalized) > 0 && len(normalized) <= 10
}

func validManualOAuthRedirectURI(value string) bool {
	parsed, hostname, ok := parseWellFormedOAuthURI(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	return isLoopbackOAuthHost(hostname)
}

func validDynamicOAuthRedirectURI(value string) bool {
	parsed, hostname, ok := parseWellFormedOAuthURI(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "http" {
		return isLoopbackOAuthHost(hostname)
	}
	if parsed.Scheme != "https" {
		return false
	}
	if isLoopbackOAuthHost(hostname) {
		return true
	}
	return isPublicOAuthHost(hostname)
}

func parseWellFormedOAuthURI(value string) (*url.URL, string, bool) {
	if value == "" || len(value) > 2048 || strings.Contains(value, "*") || strings.Contains(value, "#") {
		return nil, "", false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return nil, "", false
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Opaque != "" || parsed.Fragment != "" || parsed.User != nil {
		return nil, "", false
	}
	if portText := parsed.Port(); portText != "" {
		port, portErr := strconv.Atoi(portText)
		if portErr != nil || port < 1 || port > 65535 {
			return nil, "", false
		}
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return nil, "", false
	}
	return parsed, hostname, true
}

func isLoopbackOAuthHost(hostname string) bool {
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
}

func validDynamicClientMetadataURI(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, hostname, ok := parseWellFormedOAuthURI(value)
	if !ok || parsed.Scheme != "https" {
		return false
	}
	return isPublicOAuthHost(hostname)
}

var reservedNonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func isPublicOAuthHost(hostname string) bool {
	if addr, err := netip.ParseAddr(hostname); err == nil {
		addr = addr.Unmap()
		if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() ||
			addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
			addr.IsMulticast() || addr.IsUnspecified() {
			return false
		}
		for _, prefix := range reservedNonPublicPrefixes {
			if prefix.Contains(addr) {
				return false
			}
		}
		return true
	}
	if len(hostname) > 253 || strings.ContainsAny(hostname, ":%") ||
		strings.HasPrefix(hostname, ".") || strings.HasSuffix(hostname, ".") {
		return false
	}
	for _, reservedSuffix := range []string{".localhost", ".local", ".internal", ".home.arpa"} {
		if strings.HasSuffix(hostname, reservedSuffix) {
			return false
		}
	}
	labels := strings.Split(hostname, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !isValidDNSLabel(label) {
			return false
		}
	}
	return isValidPublicTLDLabel(labels[len(labels)-1])
}

func isValidDNSLabel(label string) bool {
	if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		ch := label[i]
		if !(ch >= 'a' && ch <= 'z') && !(ch >= '0' && ch <= '9') && ch != '-' {
			return false
		}
	}
	return true
}

func isValidPublicTLDLabel(tld string) bool {
	if len(tld) < 2 {
		return false
	}
	if strings.HasPrefix(tld, "xn--") {
		return len(tld) > 4
	}
	for i := 0; i < len(tld); i++ {
		if tld[i] < 'a' || tld[i] > 'z' {
			return false
		}
	}
	return true
}

func validOptionalHTTPURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.User == nil
}

func oauthApplicationRedirectURIAllowed(application model.OAuthApplication, redirectURI string) bool {
	for _, allowed := range decodeStringList(application.RedirectURIs) {
		if subtle.ConstantTimeCompare([]byte(allowed), []byte(redirectURI)) == 1 {
			return true
		}
	}
	if !application.IsPublicClient() {
		return false
	}
	requestedParsed, requestedHost, ok := parseWellFormedOAuthURI(redirectURI)
	if !ok || requestedParsed.Scheme != "http" || !isLoopbackOAuthHost(requestedHost) {
		return false
	}
	for _, allowed := range decodeStringList(application.RedirectURIs) {
		allowedParsed, allowedHost, allowedOK := parseWellFormedOAuthURI(allowed)
		if !allowedOK || allowedParsed.Scheme != "http" || allowedHost != requestedHost {
			continue
		}
		if allowedParsed.Path == requestedParsed.Path && allowedParsed.RawQuery == requestedParsed.RawQuery {
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
	if len(value) != 43 {
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
	for i := 0; i < len(verifier); i++ {
		ch := verifier[i]
		if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') &&
			ch != '-' && ch != '.' && ch != '_' && ch != '~' {
			return false
		}
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
	telemetry.SetHTTPError(ctx, "oauth."+code, description)
	if status == http.StatusUnauthorized && ctx.Writer.Header().Get("WWW-Authenticate") == "" {
		ctx.Header("WWW-Authenticate", "Basic")
	}
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
	rawAuthHeader := strings.TrimSpace(ctx.GetHeader("Authorization"))
	basicClientID, basicSecret, basicOK := ctx.Request.BasicAuth()
	basicDecodeFailed := false
	if basicOK {
		unescapedID, idErr := url.QueryUnescape(basicClientID)
		unescapedSecret, secretErr := url.QueryUnescape(basicSecret)
		if idErr != nil || secretErr != nil {
			basicDecodeFailed = true
		} else {
			basicClientID = unescapedID
			basicSecret = unescapedSecret
		}
	}
	trimmedFormClientID := strings.TrimSpace(formClientID)
	clientID := trimmedFormClientID
	clientSecret := formClientSecret
	if basicOK {
		clientID = strings.TrimSpace(basicClientID)
		clientSecret = basicSecret
	}
	if !h.allowOAuthTokenClientAttempt(ctx, clientID, flow, credential) {
		return oauthClientAuthentication{}, false
	}
	if (rawAuthHeader != "" && !basicOK) || basicDecodeFailed {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return oauthClientAuthentication{}, false
	}
	if basicOK && formClientSecret != "" {
		oauthError(ctx, http.StatusBadRequest, "invalid_request", "Multiple client authentication mechanisms must not be used")
		return oauthClientAuthentication{}, false
	}
	if basicOK && trimmedFormClientID != "" && trimmedFormClientID != clientID {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return oauthClientAuthentication{}, false
	}
	var application model.OAuthApplication
	if clientID == "" || h.dbFor(ctx).First(&application, "client_id = ? and revoked_at is null", clientID).Error != nil {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return oauthClientAuthentication{}, false
	}

	switch application.EffectiveTokenEndpointAuthMethod() {
	case model.OAuthTokenEndpointAuthMethodNone:
		if basicOK || clientSecret != "" || formClientSecret != "" || application.ClientSecretHash != "" {
			oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
			return oauthClientAuthentication{}, false
		}
		return oauthClientAuthentication{
			applicationID: application.ID,
			clientID:      application.ClientID,
			public:        true,
		}, true
	case model.OAuthTokenEndpointAuthMethodClientSecretBasic, model.OAuthTokenEndpointAuthMethodClientSecretPost:
		if application.RegistrationSource == model.OAuthRegistrationSourceDynamic {
			if application.EffectiveTokenEndpointAuthMethod() == model.OAuthTokenEndpointAuthMethodClientSecretBasic && !basicOK {
				oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
				return oauthClientAuthentication{}, false
			}
			if application.EffectiveTokenEndpointAuthMethod() == model.OAuthTokenEndpointAuthMethodClientSecretPost && basicOK {
				oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
				return oauthClientAuthentication{}, false
			}
		}
		clientSecretHash := hashToken(clientSecret)
		if clientSecret == "" || application.ClientSecretHash == "" ||
			subtle.ConstantTimeCompare([]byte(application.ClientSecretHash), []byte(clientSecretHash)) != 1 {
			oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
			return oauthClientAuthentication{}, false
		}
		return oauthClientAuthentication{
			applicationID:    application.ID,
			clientID:         application.ClientID,
			clientSecretHash: clientSecretHash,
		}, true
	default:
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return oauthClientAuthentication{}, false
	}
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
		{subject: "oauth_client_ip_id:" + sourceIP + ":" + hashToken(strings.TrimSpace(clientID)), limit: credentialLimit},
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

func (h *Handlers) allowOAuthDynamicRegistrationAttempt(ctx *gin.Context) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	ipLimit := oauthDynamicRegistrationIPRateLimit
	burstLimit := oauthDynamicRegistrationBurstLimit
	if h.mode == "development" {
		ipLimit = developmentRateLimit
		burstLimit = developmentRateLimit
	}
	sourceIP := h.oauthClientSourceIP(ctx)
	for _, attempt := range []struct {
		subject string
		limit   int
	}{
		{subject: "oauth_register_ip:" + sourceIP, limit: ipLimit},
		{subject: "oauth_register_global", limit: burstLimit},
	} {
		allowed, err := h.rateLimiter.Allow(ctx.Request.Context(), attempt.subject, attempt.limit, oauthRateLimitWindow)
		if allowed {
			continue
		}
		if err != nil && h.mode == "development" {
			continue
		}
		writeOAuthRateLimitError(ctx, "temporarily_unavailable", "Dynamic client registration is temporarily rate limited")
		return false
	}
	return true
}

func writeOAuthRateLimitError(ctx *gin.Context, code, description string) {
	ctx.Header("Retry-After", "60")
	oauthError(ctx, http.StatusTooManyRequests, code, description)
}

const (
	errOAuthInvalidScope       oauthSentinelError = "invalid OAuth scope"
	errOAuthUnauthorizedClient oauthSentinelError = "unauthorized OAuth client"
)
