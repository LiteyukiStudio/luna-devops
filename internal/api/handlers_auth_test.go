package api

import (
	"context"
	"encoding/json"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"gorm.io/gorm"
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

func TestUpdateCurrentUserInputDistinguishesThemeInheritance(t *testing.T) {
	var omitted updateCurrentUserInput
	if err := json.Unmarshal([]byte(`{"name":"Luna"}`), &omitted); err != nil {
		t.Fatalf("unmarshal omitted theme: %v", err)
	}
	if omitted.BrandColorPreset != nil {
		t.Fatalf("omitted brandColorPreset = %q, want nil", *omitted.BrandColorPreset)
	}
	if omitted.InterfaceStyle != nil {
		t.Fatalf("omitted interfaceStyle = %q, want nil", *omitted.InterfaceStyle)
	}

	var inherited updateCurrentUserInput
	if err := json.Unmarshal([]byte(`{"brandColorPreset":""}`), &inherited); err != nil {
		t.Fatalf("unmarshal inherited theme: %v", err)
	}
	if inherited.BrandColorPreset == nil || *inherited.BrandColorPreset != "" {
		t.Fatalf("inherited brandColorPreset = %#v, want explicit empty value", inherited.BrandColorPreset)
	}

	var inheritedStyle updateCurrentUserInput
	if err := json.Unmarshal([]byte(`{"interfaceStyle":""}`), &inheritedStyle); err != nil {
		t.Fatalf("unmarshal inherited interface style: %v", err)
	}
	if inheritedStyle.InterfaceStyle == nil || *inheritedStyle.InterfaceStyle != "" {
		t.Fatalf("inherited interfaceStyle = %#v, want explicit empty value", inheritedStyle.InterfaceStyle)
	}
}

func TestCurrentUserResponseIncludesBrandColorPreference(t *testing.T) {
	response := currentUserResponse(model.User{BrandColorPreset: "teal", InterfaceStyle: "minimal", Password: "hash"})
	if response["brandColorPreset"] != "teal" {
		t.Fatalf("brandColorPreset = %v, want teal", response["brandColorPreset"])
	}
	if response["passwordSet"] != true {
		t.Fatalf("passwordSet = %v, want true", response["passwordSet"])
	}
	if response["interfaceStyle"] != "minimal" {
		t.Fatalf("interfaceStyle = %v, want minimal", response["interfaceStyle"])
	}
}

func TestAuthRegistrationSettingsResponseContainsOnlyRegistrationPolicy(t *testing.T) {
	response := authRegistrationSettingsResponse(model.AuthRegistrationSettings{
		AllowEmailRegistration:        true,
		AllowOIDCRegistration:         true,
		AllowExternalIdentityPassword: true,
	})
	for _, key := range []string{"smtpHost", "smtpPort", "smtpSecurity", "smtpUsername", "smtpPasswordSet", "smtpFromAddress", "smtpFromName"} {
		if _, exposed := response[key]; exposed {
			t.Fatalf("registration settings unexpectedly expose %q", key)
		}
	}
}

func TestRegistrationEmailUsesGlobalPlatformMailSender(t *testing.T) {
	challenge := model.EmailRegistrationChallenge{Email: "operator@example.com", Language: "zh-CN"}
	called := false
	err := sendRegistrationEmailWith(t.Context(), nil, nil, challenge, "123456", func(
		ctx context.Context,
		db *gorm.DB,
		resolver notification.SecretResolver,
		recipient string,
		message notification.RenderedMessage,
	) (notification.SendResult, error) {
		called = true
		if ctx == nil || db != nil || resolver != nil {
			t.Fatalf("unexpected sender dependencies: ctx=%v db=%v resolver=%v", ctx, db, resolver)
		}
		if recipient != challenge.Email {
			t.Fatalf("recipient = %q, want %q", recipient, challenge.Email)
		}
		if !strings.Contains(message.Subject, "邮箱验证码") || !strings.Contains(message.Body, "123456") {
			t.Fatalf("registration message = %#v", message)
		}
		return notification.SendResult{StatusCode: 250}, nil
	})
	if err != nil {
		t.Fatalf("sendRegistrationEmailWith() error = %v", err)
	}
	if !called {
		t.Fatal("global platform mail sender was not called")
	}
}

func TestCreateRememberTokenDefaultsToNoOp(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	h := &Handlers{}
	h.domains = newDomainHandlers(h)

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

	if !strings.HasPrefix(sessionToken, "sess_") || session.TokenHash != transportapi.HashToken(sessionToken) {
		t.Fatalf("invalid session token metadata: token=%q hash=%q", sessionToken, session.TokenHash)
	}
	if !session.ExpiresAt.Equal(now.Add(sessionDuration)) {
		t.Fatalf("session expiry = %v", session.ExpiresAt)
	}
	if !strings.HasPrefix(rememberToken, "rem_") || remember.TokenHash != transportapi.HashToken(rememberToken) {
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
		{name: "profile only", originalRole: authz.PlatformRoleUser, nextRole: authz.PlatformRoleUser, want: false},
		{name: "role changed", originalRole: authz.PlatformRoleUser, nextRole: authz.PlatformRoleAdmin, want: true},
		{name: "account disabled", originalRole: authz.PlatformRoleUser, nextRole: authz.PlatformRoleUser, nextDisabled: true, want: true},
		{name: "password changed", originalRole: authz.PlatformRoleUser, nextRole: authz.PlatformRoleUser, passwordChanged: true, want: true},
		{name: "account enabled", originalRole: authz.PlatformRoleUser, nextRole: authz.PlatformRoleUser, originallyDisabled: true, want: false},
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

func TestDevelopmentAdminFreeQuotaCredits(t *testing.T) {
	t.Run("defaults to a positive local quota", func(t *testing.T) {
		credits, err := developmentAdminFreeQuotaCredits("1000")
		if err != nil {
			t.Fatalf("developmentAdminFreeQuotaCredits() error = %v", err)
		}
		if credits.String() != "1000" {
			t.Fatalf("developmentAdminFreeQuotaCredits() = %s, want 1000", credits)
		}
	})

	t.Run("zero disables the grant", func(t *testing.T) {
		credits, err := developmentAdminFreeQuotaCredits("0")
		if err != nil {
			t.Fatalf("developmentAdminFreeQuotaCredits() error = %v", err)
		}
		if !credits.IsZero() {
			t.Fatalf("developmentAdminFreeQuotaCredits() = %s, want 0", credits)
		}
	})

	for _, value := range []string{"invalid", "-1"} {
		t.Run("rejects "+value, func(t *testing.T) {
			if _, err := developmentAdminFreeQuotaCredits(value); err == nil {
				t.Fatalf("developmentAdminFreeQuotaCredits() accepted %q", value)
			}
		})
	}
}

func TestAuthProviderResponseHidesStoredClientSecret(t *testing.T) {
	provider := model.AuthProvider{ClientSecretRef: "secret-id:sec_test"}

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
	h.domains = newDomainHandlers(h)
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "global",
		Variables: `{"PUBLIC_FLAG":"true","API_URL":"https://api.example.com"}`,
	}

	output, err := h.domains.build.BuildVariableSetResponseForUser(model.User{ID: "usr_member", Role: authz.PlatformRoleUser}, set, context.Background())
	if err != nil {
		t.Fatalf("buildVariableSetResponseForUser() error = %v", err)
	}

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
	h.domains = newDomainHandlers(h)
	set := model.BuildVariableSet{
		ID:        "bvs_test",
		Scope:     "user",
		OwnerRef:  "usr_owner",
		Variables: `{"PUBLIC_FLAG":"true"}`,
	}

	output, err := h.domains.build.BuildVariableSetResponseForUser(model.User{ID: "usr_owner", Role: authz.PlatformRoleUser}, set, context.Background())
	if err != nil {
		t.Fatalf("buildVariableSetResponseForUser() error = %v", err)
	}

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
	codec, err := secret.NewCodec("test-key")
	if err != nil {
		t.Fatal(err)
	}
	existingSecretRef := codec.Encrypt("old-secret")
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

func TestResolveSecretRejectsNonCanonicalReferences(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "test-key")
	t.Setenv("OIDC_TEST_SECRET", "env-secret")
	codec, err := secret.NewCodec("test-key")
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{secrets: secret.NewStore(nil, nil, codec)}
	h.domains = newDomainHandlers(h)

	for _, ref := range []string{codec.Encrypt("inline-secret"), "literal:literal-secret", "plain-secret", "env:OIDC_TEST_SECRET"} {
		if resolved := h.resolveSecretContext(context.Background(), ref); resolved != "" {
			t.Fatalf("non-canonical ref %q resolved to %q", ref, resolved)
		}
	}
}
