package identityapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestOAuthApplicationResponseNormalizesEmptyRedirectURIs(t *testing.T) {
	response := oauthApplicationToResponse(model.OAuthApplication{})
	if response.RedirectURIs == nil || len(response.RedirectURIs) != 0 {
		t.Fatalf("redirectUris = %#v, want a non-nil empty slice", response.RedirectURIs)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal OAuth application response: %v", err)
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode OAuth application response: %v", err)
	}
	if got := string(fields["redirectUris"]); got != "[]" {
		t.Fatalf("serialized redirectUris = %s, want []", got)
	}
}

func TestNormalizeOAuthScopeRejectsWildcardScope(t *testing.T) {
	if scope := normalizeOAuthScope("*"); scope != "" {
		t.Fatalf("normalizeOAuthScope(*) = %q, want empty", scope)
	}
	if scope := normalizeOAuthScope("user:read, project:read"); scope != "project:read,user:read" && scope != "user:read,project:read" {
		t.Fatalf("normalizeOAuthScope(valid) = %q", scope)
	}
}

func TestOAuthRevokeRejectsMissingOrBlankToken(t *testing.T) {
	router := gin.New()
	router.POST("/api/v1/oauth/revoke", (&Handlers{}).RevokeOAuthToken)

	for _, test := range []struct {
		name string
		form url.Values
	}{
		{name: "missing", form: url.Values{}},
		{name: "blank", form: url.Values{"token": {"  "}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/oauth/revoke",
				strings.NewReader(test.form.Encode()),
			)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("revoke empty token status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			var response oauthProtocolErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode OAuth error response: %v", err)
			}
			if response.Code != "invalid_request" {
				t.Fatalf("revoke empty token error = %q, want invalid_request", response.Code)
			}
		})
	}
}

func TestValidateDynamicClientRedirectURIs(t *testing.T) {
	validCases := [][]string{
		{"http://127.0.0.1:54123/oauth/callback"},
		{"http://localhost:8765/callback"},
		{"http://[::1]:3000/callback"},
		{"https://client.example.com/oauth/callback"},
		{"https://127.0.0.1:8443/callback"},
	}
	for _, uris := range validCases {
		got, ok := validateDynamicClientRedirectURIs(uris)
		if !ok || len(got) != len(uris) {
			t.Fatalf("validateDynamicClientRedirectURIs(%#v) = (%#v, %v), want valid", uris, got, ok)
		}
	}

	invalidCases := []struct {
		name string
		uris []string
	}{
		{name: "empty slice", uris: nil},
		{name: "javascript scheme", uris: []string{"javascript:alert(1)"}},
		{name: "data scheme", uris: []string{"data:text/html,<script>alert(1)</script>"}},
		{name: "wildcard host", uris: []string{"https://*.example.com/callback"}},
		{name: "wildcard path", uris: []string{"https://example.com/*"}},
		{name: "fragment", uris: []string{"https://example.com/callback#frag"}},
		{name: "empty hash fragment", uris: []string{"https://example.com/callback#"}},
		{name: "userinfo", uris: []string{"https://user:pass@example.com/callback"}},
		{name: "http private lan", uris: []string{"http://192.168.31.10:8080/callback"}},
		{name: "http public non-https", uris: []string{"http://example.com/callback"}},
		{name: "https private lan", uris: []string{"https://192.168.1.10/callback"}},
		{name: "https link-local metadata", uris: []string{"https://169.254.169.254/callback"}},
		{name: "https cgnat", uris: []string{"https://100.64.0.10/callback"}},
		{name: "https single-label intranet", uris: []string{"https://intranet/callback"}},
		{name: "https mDNS local", uris: []string{"https://service.local/callback"}},
		{name: "https whatwg shorthand ipv4", uris: []string{"https://127.1:8443/callback"}},
		{name: "https whatwg octal ipv4", uris: []string{"https://0177.0.0.1:8443/callback"}},
		{name: "https whatwg hex ipv4", uris: []string{"https://0x7f.0.0.1:8443/callback"}},
		{name: "https metadata decimal ipv4", uris: []string{"https://169.254.43518/callback"}},
		{name: "https ipv6 zone id", uris: []string{"https://[fe80::1%25eth0.100]/callback"}},
		{name: "https nat64 metadata", uris: []string{"https://[64:ff9b::169.254.169.254]/callback"}},
		{name: "https empty dns label", uris: []string{"https://evil..com/callback"}},
		{name: "mixed valid and invalid must fail closed", uris: []string{"http://127.0.0.1:8080/cb", "javascript:alert(1)"}},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := validateDynamicClientRedirectURIs(tc.uris); ok {
				t.Fatalf("validateDynamicClientRedirectURIs(%#v) unexpectedly succeeded with %#v", tc.uris, got)
			}
		})
	}
}

func TestDynamicClientRegistrationValidationAndRateLimit(t *testing.T) {
	server := miniredis.RunT(t)
	limiter := newRateLimiter(server.Addr())
	h := &Handlers{
		mode:        "production",
		rateLimiter: limiter,
	}
	t.Cleanup(func() { _ = limiter.RedisClient().Close() })

	router := gin.New()
	router.POST("/api/v1/oauth/register", h.RegisterOAuthClient)

	badPayloads := []struct {
		name      string
		body      string
		wantError string
	}{
		{name: "malformed json", body: "{not-json", wantError: "invalid_client_metadata"},
		{name: "oversized body", body: `{"redirect_uris":["https://example.com/cb"],"client_name":"` + strings.Repeat("a", oauthDynamicRegistrationMaxBodyBytes) + `"}`, wantError: "invalid_client_metadata"},
		{name: "missing redirect_uris", body: `{"token_endpoint_auth_method":"none"}`, wantError: "invalid_redirect_uri"},
		{name: "unsafe redirect_uri", body: `{"redirect_uris":["http://10.0.0.8:3000/cb"],"token_endpoint_auth_method":"none"}`, wantError: "invalid_redirect_uri"},
		{name: "unsupported auth method", body: `{"redirect_uris":["http://127.0.0.1:3000/cb"],"token_endpoint_auth_method":"private_key_jwt"}`, wantError: "invalid_client_metadata"},
		{name: "unsupported grant type", body: `{"redirect_uris":["http://127.0.0.1:3000/cb"],"grant_types":["client_credentials"]}`, wantError: "invalid_client_metadata"},
		{name: "refresh_token without authorization_code", body: `{"redirect_uris":["http://127.0.0.1:3000/cb"],"grant_types":["refresh_token"]}`, wantError: "invalid_client_metadata"},
		{name: "unsupported response_types", body: `{"redirect_uris":["http://127.0.0.1:3000/cb"],"response_types":["token"]}`, wantError: "invalid_client_metadata"},
		{name: "wildcard scope", body: `{"redirect_uris":["http://127.0.0.1:3000/cb"],"scope":"*"}`, wantError: "invalid_client_metadata"},
		{name: "unknown scope", body: `{"redirect_uris":["http://127.0.0.1:3000/cb"],"scope":"project:read unknown:scope"}`, wantError: "invalid_client_metadata"},
		{name: "loopback logo_uri SSRF/probe", body: `{"redirect_uris":["http://127.0.0.1:3000/cb"],"logo_uri":"https://127.0.0.1:8443/logo.png"}`, wantError: "invalid_client_metadata"},
	}

	for _, tc := range badPayloads {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/register", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = "198.51.100.10:12345"
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			var errResp oauthProtocolErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if errResp.Code != tc.wantError {
				t.Fatalf("error = %q, want %q (%s)", errResp.Code, tc.wantError, errResp.Description)
			}
		})
	}

	for attempt := 0; attempt < oauthDynamicRegistrationIPRateLimit+1; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/register", bytes.NewBufferString(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.77:54321"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if attempt < oauthDynamicRegistrationIPRateLimit {
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("attempt %d status = %d, want 400", attempt+1, rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("rate-limited attempt status = %d, want 429: %s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Retry-After") != "60" {
			t.Fatalf("Retry-After = %q, want 60", rec.Header().Get("Retry-After"))
		}
	}

	// Non-JSON Content-Type (browser no-cors text/plain spam) must be rejected immediately.
	textReq := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/register", bytes.NewBufferString(`{"redirect_uris":["https://example.com/cb"]}`))
	textReq.Header.Set("Content-Type", "text/plain")
	textReq.RemoteAddr = "198.51.100.99:44321"
	textRec := httptest.NewRecorder()
	router.ServeHTTP(textRec, textReq)
	if textRec.Code != http.StatusBadRequest {
		t.Fatalf("text/plain Content-Type status = %d, want 400: %s", textRec.Code, textRec.Body.String())
	}
}

func TestManualRedirectURIsAndPublicLoopbackEphemeralPortMatching(t *testing.T) {
	// Self-hosted manual OAuth apps still permit intranet HTTPS callbacks, while failing closed on mixed invalid URIs.
	if got, ok := normalizeOAuthRedirectURIs([]string{"https://app.corp.internal/oauth/callback", "https://10.0.1.20/cb"}); !ok || len(got) != 2 {
		t.Fatalf("normalizeOAuthRedirectURIs(intranet https) = (%#v, %v), want valid", got, ok)
	}
	if got, ok := normalizeOAuthRedirectURIs([]string{"https://app.example.com/cb", "javascript:alert(1)"}); ok {
		t.Fatalf("normalizeOAuthRedirectURIs(mixed invalid) unexpectedly succeeded: %#v", got)
	}

	publicApp := model.OAuthApplication{
		TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthMethodNone,
		RedirectURIs:            encodeStringList([]string{"http://127.0.0.1:53124/oauth/callback"}),
	}
	if !oauthApplicationRedirectURIAllowed(publicApp, "http://127.0.0.1:61999/oauth/callback") {
		t.Fatal("expected public client loopback HTTP ephemeral port to match per RFC 8252 §7.3")
	}
	if oauthApplicationRedirectURIAllowed(publicApp, "http://localhost:61999/oauth/callback") {
		t.Fatal("loopback matching must not cross-match 127.0.0.1 with localhost")
	}
	if oauthApplicationRedirectURIAllowed(publicApp, "http://127.0.0.1:61999/other-path") {
		t.Fatal("loopback matching must require identical path")
	}

	confidentialApp := model.OAuthApplication{
		TokenEndpointAuthMethod: model.OAuthTokenEndpointAuthMethodClientSecretBasic,
		RedirectURIs:            encodeStringList([]string{"http://127.0.0.1:53124/oauth/callback"}),
	}
	if oauthApplicationRedirectURIAllowed(confidentialApp, "http://127.0.0.1:61999/oauth/callback") {
		t.Fatal("confidential client must require exact port match")
	}
}
