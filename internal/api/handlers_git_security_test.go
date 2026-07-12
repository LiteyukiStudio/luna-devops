package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/LiteyukiStudio/devops/internal/model"
	gitprovider "github.com/LiteyukiStudio/devops/internal/provider/git"
)

func TestNormalizeAccessTokenScopeRejectsWildcardAndUnknownScopes(t *testing.T) {
	if scope := normalizeAccessTokenScope("*"); scope != "" {
		t.Fatalf("expected wildcard scope to be rejected, got %q", scope)
	}
	if scope := normalizeAccessTokenScope("project:read,git:read"); scope != "project:read,git:read" {
		t.Fatalf("expected normalized scope list, got %q", scope)
	}
	if scope := normalizeAccessTokenScope("billing:write"); scope != "billing:write" {
		t.Fatalf("expected billing write scope, got %q", scope)
	}
	if scope := normalizeAccessTokenScope("project:read,unknown:write"); scope != "" {
		t.Fatalf("expected unknown scope to be rejected, got %q", scope)
	}
}

func TestUserCannotCreateAdministrativeAccessTokenScope(t *testing.T) {
	user := model.User{Role: "user"}
	if userCanCreateAccessTokenScope(user, "user:manage") {
		t.Fatal("expected normal user to be blocked from user:manage")
	}
	if userCanCreateAccessTokenScope(user, "config:write") {
		t.Fatal("expected normal user to be blocked from config:write")
	}
	if userCanCreateAccessTokenScope(user, "project:write") {
		t.Fatal("expected normal user to be blocked from project:write without project role context")
	}
	if userCanCreateAccessTokenScope(user, "billing:write") {
		t.Fatal("expected normal user to be blocked from billing:write")
	}
	if !userCanCreateAccessTokenScope(user, "project:read,git:read") {
		t.Fatal("expected normal user to create read scopes")
	}
	if !userCanCreateAccessTokenScope(user, "billing:read") {
		t.Fatal("expected normal user to create billing:read")
	}
}

func TestRegistryResponseExposesCredentialSetOnly(t *testing.T) {
	output := registryResponse(model.ArtifactRegistry{CredentialRef: "regc_secret"})
	if !output.CredentialSet {
		t.Fatal("expected credentialSet to be true")
	}
}

func TestVerifyGitWebhookSignatureSupportsGitHubAndGiteaHeaders(t *testing.T) {
	body := []byte(`{"after":"abc"}`)
	signature := hmacSHA256Hex(body, "secret")

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", "sha256="+signature)
	if !verifyGitWebhookSignature(headers, body, "secret") {
		t.Fatal("expected GitHub webhook signature to verify")
	}

	headers = http.Header{}
	headers.Set("X-Gitea-Signature", signature)
	if !verifyGitWebhookSignature(headers, body, "secret") {
		t.Fatal("expected Gitea webhook signature to verify")
	}

	headers.Set("X-Gitea-Signature", "bad")
	if verifyGitWebhookSignature(headers, body, "secret") {
		t.Fatal("expected invalid webhook signature to fail")
	}
}

func TestGitUpstreamErrorStatusAndCodeMapsWebhookLocalhost(t *testing.T) {
	status, code := gitUpstreamErrorStatusAndCode(&gitprovider.UpstreamError{
		StatusCode: http.StatusUnprocessableEntity,
		Message:    "Validation Failed",
		Details: []gitprovider.UpstreamErrorDetail{{
			Resource: "Hook",
			Code:     "custom",
			Field:    "url",
			Message:  "url is not supported because it isn't reachable over the public Internet (localhost)",
		}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if code != "git.webhook_callback_unreachable" {
		t.Fatalf("code = %q", code)
	}
}

func TestGitUpstreamErrorStatusAndCodeMapsNetworkFailure(t *testing.T) {
	status, code := gitUpstreamErrorStatusAndCode(errors.New("dial tcp: lookup github.com: no such host"))
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", status, http.StatusBadGateway)
	}
	if code != "git.network_failed" {
		t.Fatalf("code = %q", code)
	}
}

func TestGitWebhookCommitSHAReadsAfterField(t *testing.T) {
	sha := gitWebhookCommitSHA([]byte(`{"after":"abc123","sha":"ignored"}`))
	if sha != "abc123" {
		t.Fatalf("sha = %q", sha)
	}
}

func TestParseGitWebhookPushPayloadReadsBranchAndAuthor(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-GitHub-Event", "push")
	payload, ok := parseGitWebhookPushPayload(headers, []byte(`{
		"ref":"refs/heads/main",
		"after":"abc123",
		"pusher":{"name":"snowy","email":"snowy@example.com"},
		"head_commit":{"author":{"name":"Author","email":"author@example.com"}}
	}`))
	if !ok {
		t.Fatal("expected push payload to parse")
	}
	if payload.SourceBranch != "main" || payload.SourceTag != "" || payload.CommitSHA != "abc123" {
		t.Fatalf("payload source = %#v", payload)
	}
	if payload.TriggeredByName != "snowy" || payload.SourceAuthorEmail != "author@example.com" {
		t.Fatalf("payload actor = %#v", payload)
	}
}

func TestParseGitWebhookPushPayloadReadsTagAndDeletion(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Gitea-Event", "push")
	payload, ok := parseGitWebhookPushPayload(headers, []byte(`{
		"ref":"refs/tags/v1.0.0",
		"after":"0000000000000000000000000000000000000000"
	}`))
	if !ok {
		t.Fatal("expected tag push payload to parse")
	}
	if payload.SourceTag != "v1.0.0" || !payload.Deleted {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestPathEscapePathPreservesPathSegments(t *testing.T) {
	escaped := gitprovider.PathEscapePath("docs/app yaml.yml")
	if escaped != "docs/app%20yaml.yml" {
		t.Fatalf("escaped path = %q", escaped)
	}
}

func TestFilterGitRepositoriesMatchesNameAndFullName(t *testing.T) {
	repos := []gitprovider.Repository{
		{Name: "api", FullName: "luna/api"},
		{Name: "web", FullName: "luna/web"},
	}
	filtered := gitprovider.FilterRepositories(repos, "API")
	if len(filtered) != 1 || filtered[0].Name != "api" {
		t.Fatalf("filtered = %#v", filtered)
	}
}

func TestGitOAuthEndpointDefaultsGitHub(t *testing.T) {
	endpoint, err := gitprovider.OAuthEndpoint(model.GitProvider{Type: "github"})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.AuthURL != "https://github.com/login/oauth/authorize" {
		t.Fatalf("auth url = %q", endpoint.AuthURL)
	}
}

func TestSanitizeFrontendOrigin(t *testing.T) {
	defaultOrigin := "http://127.0.0.1:8080"

	if got := sanitizeFrontendOrigin("", defaultOrigin); got != defaultOrigin {
		t.Fatalf("empty origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("mailto://bad.example", defaultOrigin); got != defaultOrigin {
		t.Fatalf("bad scheme origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("http://127.0.0.1:5173", defaultOrigin); got != "http://127.0.0.1:5173" {
		t.Fatalf("same host origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("http://localhost:5173", defaultOrigin); got != "http://localhost:5173" {
		t.Fatalf("loopback host origin = %q", got)
	}
	if got := sanitizeFrontendOrigin("http://evil.example", defaultOrigin); got != defaultOrigin {
		t.Fatalf("different host origin = %q", got)
	}
}

func TestBuildFrontendRedirect(t *testing.T) {
	defaultOrigin := "http://127.0.0.1:8080"
	redirectPath := "/code-repositories"
	accountID := "gita_xxx"
	target := buildFrontendRedirect(defaultOrigin, "http://127.0.0.1:5173", redirectPath, accountID)
	if target != "http://127.0.0.1:5173/code-repositories?gitAccountId=gita_xxx" {
		t.Fatalf("redirect = %q", target)
	}

	target = buildFrontendRedirect(defaultOrigin, "http://localhost:5173", "/projects?tab=apps", accountID)
	if target != "http://localhost:5173/projects?tab=apps&gitAccountId=gita_xxx" {
		t.Fatalf("redirect with query = %q", target)
	}
}

func TestGitOAuthCallbackURL(t *testing.T) {
	if got := gitOAuthCallbackURL("http://localhost:5173/"); got != "http://localhost:5173/api/v1/git/oauth/callback" {
		t.Fatalf("callback url = %q", got)
	}
}

func TestOIDCCallbackURL(t *testing.T) {
	if got := oidcCallbackURL("https://studio.example.com/"); got != "https://studio.example.com/api/v1/auth/oidc/callback" {
		t.Fatalf("callback url = %q", got)
	}
	if got := oidcCallbackURL(""); got != "" {
		t.Fatalf("empty callback url = %q", got)
	}
}

func TestOIDCAdmissionEmailHonorsVerifiedRequirement(t *testing.T) {
	claims := oidcIdentityClaims{Email: "USER@example.com", EmailVerified: false}
	if _, ok := oidcAdmissionEmail(claims, true); ok {
		t.Fatal("expected unverified email to be rejected when verification is required")
	}

	email, ok := oidcAdmissionEmail(claims, false)
	if !ok {
		t.Fatal("expected unverified email to be accepted when verification is optional")
	}
	if email != "user@example.com" {
		t.Fatalf("email = %q", email)
	}

	if _, ok := oidcAdmissionEmail(oidcIdentityClaims{}, false); ok {
		t.Fatal("expected empty email to be rejected")
	}
}

func TestGitExternalBaseURLPrefersPublicEnv(t *testing.T) {
	h := &Handlers{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/git/oauth/start", nil)

	t.Setenv("PUBLIC_BASE_URL", "https://studio.example.com/")

	if got := h.externalBaseURL(ctx); got != "https://studio.example.com" {
		t.Fatalf("externalBaseURL = %q", got)
	}
}

func TestGitExternalBaseURLReturnsEmptyWhenNotConfigured(t *testing.T) {
	h := &Handlers{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/git/oauth/start", nil)

	t.Setenv("PUBLIC_BASE_URL", "")

	if got := h.externalBaseURL(ctx); got != "" {
		t.Fatalf("externalBaseURL = %q", got)
	}
}

func TestConfiguredAllowedOriginsUsesPublicBaseAndEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "https://studio.example.com/app")
	t.Setenv("APP_CORS_ORIGINS", "https://admin.example.com, https://studio.example.com")

	origins := configuredAllowedOrigins()
	if !containsString(origins, "https://studio.example.com") {
		t.Fatalf("expected PUBLIC_BASE_URL origin, got %#v", origins)
	}
	if !containsString(origins, "https://admin.example.com") {
		t.Fatalf("expected APP_CORS_ORIGINS origin, got %#v", origins)
	}
	if containsString(origins, "http://localhost:5173") {
		t.Fatalf("did not expect development origin in production, got %#v", origins)
	}
}

func TestConfiguredAllowedOriginsDefaultsToProductionMode(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("APP_CORS_ORIGINS", "")

	origins := configuredAllowedOrigins()
	if containsString(origins, "http://localhost:5173") {
		t.Fatalf("did not expect development origin when APP_ENV is unset, got %#v", origins)
	}
	if containsString(origins, "http://127.0.0.1:5173") {
		t.Fatalf("did not expect development origin when APP_ENV is unset, got %#v", origins)
	}
}

func TestRequestOriginAllowedRejectsUntrustedOrigin(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/users/me", nil)
	ctx.Request.Header.Set("Origin", "https://evil.example.com")

	if requestOriginAllowed(ctx, []string{"https://studio.example.com"}) {
		t.Fatal("expected untrusted origin to be rejected")
	}

	ctx.Request.Header.Set("Origin", "https://studio.example.com")
	if !requestOriginAllowed(ctx, []string{"https://studio.example.com"}) {
		t.Fatal("expected trusted origin to be accepted")
	}
}

func TestRateLimiterUsesRedisOnly(t *testing.T) {
	limiter := newRateLimiter("localhost:6379")
	if limiter.redis == nil {
		t.Fatal("expected redis client")
	}
}

func TestTrustedProxyConfigurationIgnoresForwardedAddressByDefault(t *testing.T) {
	router := gin.New()
	configureTrustedProxies(router, nil)
	router.GET("/client-ip", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, ctx.ClientIP())
	})

	request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Body.String() != "192.0.2.10" {
		t.Fatalf("client IP = %q", recorder.Body.String())
	}
}

func TestTrustedProxyConfigurationAcceptsForwardedAddressFromConfiguredCIDR(t *testing.T) {
	router := gin.New()
	configureTrustedProxies(router, []string{"192.0.2.0/24"})
	router.GET("/client-ip", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, ctx.ClientIP())
	})

	request := httptest.NewRequest(http.MethodGet, "/client-ip", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Body.String() != "203.0.113.9" {
		t.Fatalf("client IP = %q", recorder.Body.String())
	}
}

func TestWriteErrorCodeHidesDetailInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeError(ctx, http.StatusBadRequest, "duplicate key value violates unique constraint users_email_key")

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "request.invalid" {
		t.Fatalf("code = %q", body["code"])
	}
	if body["error"] == "duplicate key value violates unique constraint users_email_key" || body["detail"] != "" {
		t.Fatalf("production response leaked detail: %#v", body)
	}
}

func TestWriteErrorCodeIncludesDetailInDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeError(ctx, http.StatusBadRequest, "validation detail")

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "validation detail" || body["detail"] != "validation detail" {
		t.Fatalf("development response should include detail: %#v", body)
	}
}

func TestPersonalGitAccountIsOnlyUsableByOwner(t *testing.T) {
	h := &Handlers{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := model.GitAccount{
		UserID:      "usr_owner",
		Scope:       "project",
		OwnerRef:    "prj_demo",
		AccessScope: "personal",
	}

	if !h.canUseGitAccount(ctx, model.User{ID: "usr_owner", Role: "user"}, account) {
		t.Fatal("expected owner to use personal Git account")
	}
	if h.canUseGitAccount(ctx, model.User{ID: "usr_other", Role: "user"}, account) {
		t.Fatal("expected another project member to be blocked from personal Git account")
	}
	if h.canUseGitAccount(ctx, model.User{ID: "usr_admin", Role: "platform_admin"}, account) {
		t.Fatal("expected platform admin to be blocked from using another user's personal Git account")
	}
}
