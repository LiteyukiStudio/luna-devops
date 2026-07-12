package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
)

func TestLoginInputRequiresExplicitRememberChoice(t *testing.T) {
	var defaultInput loginInput
	if err := json.Unmarshal([]byte(`{"email":"user@example.com","password":"password"}`), &defaultInput); err != nil {
		t.Fatalf("unmarshal default login input: %v", err)
	}
	if defaultInput.RememberMe {
		t.Fatal("rememberMe must default to false")
	}

	var rememberedInput loginInput
	if err := json.Unmarshal([]byte(`{"email":"user@example.com","password":"password","rememberMe":true}`), &rememberedInput); err != nil {
		t.Fatalf("unmarshal remembered login input: %v", err)
	}
	if !rememberedInput.RememberMe {
		t.Fatal("rememberMe=true must be preserved")
	}
}

func TestCreateRememberTokenDefaultsToNoOp(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	h := &Handlers{}

	if !h.createRememberToken(ctx, "usr_test") {
		t.Fatal("omitted remember choice should succeed without issuing a token")
	}
	if got := recorder.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("unexpected remember cookie: %#v", got)
	}
}

func TestGeneratedSessionCredentialsUseExpectedLifetimeAndHash(t *testing.T) {
	now := time.Date(2026, time.July, 12, 1, 2, 3, 0, time.UTC)
	session, sessionToken := newUserSession("usr_test", "", now)
	remember, rememberToken := newUserRememberToken("usr_test", now)

	if !strings.HasPrefix(sessionToken, "sess_") || session.TokenHash != hashToken(sessionToken) {
		t.Fatalf("invalid session token metadata: token=%q hash=%q", sessionToken, session.TokenHash)
	}
	if !session.ExpiresAt.Equal(now.Add(sessionDuration)) {
		t.Fatalf("session expiry = %v", session.ExpiresAt)
	}
	if !strings.HasPrefix(rememberToken, "rem_") || remember.TokenHash != hashToken(rememberToken) {
		t.Fatalf("invalid remember token metadata: token=%q hash=%q", rememberToken, remember.TokenHash)
	}
	if remember.FamilyID == "" {
		t.Fatal("remember token family must be set")
	}
	if !remember.ExpiresAt.Equal(now.Add(rememberDuration)) {
		t.Fatalf("remember expiry = %v", remember.ExpiresAt)
	}
}

func TestSessionCookiePersistenceMatchesRememberChoice(t *testing.T) {
	for _, tc := range []struct {
		name       string
		persistent bool
		wantMaxAge int
	}{
		{name: "browser session", persistent: false, wantMaxAge: 0},
		{name: "remembered session", persistent: true, wantMaxAge: int(sessionDuration / time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			setSessionCookie(ctx, "sess_test", true, tc.persistent)

			cookies := recorder.Result().Cookies()
			if len(cookies) != 1 || cookies[0].MaxAge != tc.wantMaxAge {
				t.Fatalf("cookies = %#v, want Max-Age %d", cookies, tc.wantMaxAge)
			}
		})
	}
}

func TestClearAuthenticationCookies(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	clearSessionCookie(ctx)
	clearRememberCookie(ctx, "usr:test")

	cookies := recorder.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookie count = %d", len(cookies))
	}
	if cookies[0].Name != sessionCookieName || cookies[0].MaxAge >= 0 {
		t.Fatalf("session cookie was not cleared: %#v", cookies[0])
	}
	if cookies[1].Name != rememberCookieNameForUser("usr:test") || cookies[1].MaxAge >= 0 {
		t.Fatalf("remember cookie was not cleared: %#v", cookies[1])
	}
}

func TestBootstrapTokenMatchesExactValue(t *testing.T) {
	if !bootstrapTokenMatches("bootstrap-secret", "bootstrap-secret") {
		t.Fatal("equal bootstrap tokens must match")
	}
	if bootstrapTokenMatches("bootstrap-secret", "bootstrap-secret ") {
		t.Fatal("bootstrap token comparison must be exact")
	}
	if bootstrapTokenMatches("bootstrap-secret", "different") {
		t.Fatal("different bootstrap tokens must not match")
	}
}

func TestUserSecurityChangesRevokeAuthentication(t *testing.T) {
	cases := []struct {
		name               string
		originalRole       string
		nextRole           string
		originallyDisabled bool
		nextDisabled       bool
		passwordChanged    bool
		want               bool
	}{
		{name: "profile only", originalRole: "user", nextRole: "user", want: false},
		{name: "role changed", originalRole: "user", nextRole: "platform_admin", want: true},
		{name: "account disabled", originalRole: "user", nextRole: "user", nextDisabled: true, want: true},
		{name: "password changed", originalRole: "user", nextRole: "user", passwordChanged: true, want: true},
		{name: "account enabled", originalRole: "user", nextRole: "user", originallyDisabled: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRevokeUserAuthentication(tc.originalRole, tc.nextRole, tc.originallyDisabled, tc.nextDisabled, tc.passwordChanged)
			if got != tc.want {
				t.Fatalf("shouldRevokeUserAuthentication() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBootstrapStatusIncludesDevLoginHintInDevelopment(t *testing.T) {
	t.Setenv("LOCAL_ADMIN_EMAIL", "Admin@Example.com")
	t.Setenv("LOCAL_ADMIN_PASSWORD", "secret-password")

	status := bootstrapStatusResponse("development", true)

	if status["devLoginEnabled"] != true {
		t.Fatalf("expected dev login enabled in development, got %v", status["devLoginEnabled"])
	}
	hint, ok := status["devLoginHint"].(gin.H)
	if !ok {
		t.Fatalf("expected devLoginHint map, got %T", status["devLoginHint"])
	}
	if hint["email"] != "admin@example.com" {
		t.Fatalf("expected normalized dev email, got %q", hint["email"])
	}
	if hint["password"] != "secret-password" {
		t.Fatalf("expected configured dev password, got %q", hint["password"])
	}
}

func TestAuthProviderResponseHidesStoredClientSecret(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	provider := model.AuthProvider{ClientSecretRef: secret.Encrypt("super-secret")}

	output := authProviderResponse(provider)

	if output.ClientSecretRef != "" {
		t.Fatalf("expected stored client secret ref to be hidden, got %q", output.ClientSecretRef)
	}
	if !output.ClientSecretSet {
		t.Fatal("expected clientSecretSet to be true")
	}
}

func TestBuildVariableSetResponseHidesVariablesWithoutInspectPermission(t *testing.T) {
	h := &Handlers{}
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "global",
		Variables: `{"PUBLIC_FLAG":"true","API_URL":"https://api.example.com"}`,
	}

	output := h.buildVariableSetResponseForUser(model.User{ID: "usr_member", Role: "user"}, set)

	if output.CanInspectVariables {
		t.Fatal("expected regular user to be unable to inspect global build variables")
	}
	if output.Variables != "{}" {
		t.Fatalf("expected variables to be hidden, got %q", output.Variables)
	}
	if output.VariableCount != 2 {
		t.Fatalf("expected variable count to remain visible, got %d", output.VariableCount)
	}
}

func TestBuildVariableSetResponseShowsVariablesWithInspectPermission(t *testing.T) {
	h := &Handlers{}
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "user",
		OwnerRef:  "usr_owner",
		Variables: `{"PUBLIC_FLAG":"true"}`,
	}

	output := h.buildVariableSetResponseForUser(model.User{ID: "usr_owner", Role: "user"}, set)

	if !output.CanInspectVariables {
		t.Fatal("expected owner to inspect personal build variables")
	}
	if output.Variables != set.Variables {
		t.Fatalf("expected variables to be visible, got %q", output.Variables)
	}
	if output.VariableCount != 1 {
		t.Fatalf("expected variable count to be 1, got %d", output.VariableCount)
	}
}

func TestAuthProviderFromInputPreservesExistingSecret(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	existingSecretRef := secret.Encrypt("old-secret")
	provider, ok := authProviderFromInput(authProviderInput{
		Type:      "oidc",
		Name:      "Casdoor",
		IssuerURL: "https://sso.example.com",
		ClientID:  "devops",
	}, "ap_existing", existingSecretRef)

	if !ok {
		t.Fatal("expected auth provider input to be valid")
	}
	if provider.ClientSecretRef != existingSecretRef {
		t.Fatalf("expected existing secret ref to be preserved, got %q", provider.ClientSecretRef)
	}
}

func TestResolveSecretSupportsStoredAndEnvRefsOnly(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	t.Setenv("OIDC_TEST_SECRET", "env-secret")
	h := &Handlers{}

	if secret := h.resolveSecret(secret.Encrypt("stored-secret")); secret != "stored-secret" {
		t.Fatalf("expected stored secret, got %q", secret)
	}
	if secret := h.resolveSecret("literal:literal-secret"); secret != "" {
		t.Fatalf("expected literal secret ref to be rejected, got %q", secret)
	}
	if secret := h.resolveSecret("plain-secret"); secret != "" {
		t.Fatalf("expected bare secret ref to be rejected, got %q", secret)
	}
	if secret := h.resolveSecret("env:OIDC_TEST_SECRET"); secret != "env-secret" {
		t.Fatalf("expected env secret, got %q", secret)
	}
}
