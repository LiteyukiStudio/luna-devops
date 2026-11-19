package identityapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type stubIdentityHost struct {
	db *gorm.DB
}

func (s *stubIdentityHost) DBFor(_ *gin.Context) *gorm.DB            { return s.db }
func (s *stubIdentityHost) DBWithContext(_ context.Context) *gorm.DB { return s.db }
func (s *stubIdentityHost) CurrentAIPlatformUser(_ *gin.Context) (model.User, bool) {
	return model.User{}, false
}
func (s *stubIdentityHost) CurrentAIPlatformSession(_ *gin.Context) (string, string, bool) {
	return "", "", false
}
func (s *stubIdentityHost) ExternalBaseURL(_ *gin.Context) string {
	return "https://devops.example.com"
}
func (s *stubIdentityHost) AdminConfiguredEgressContext(ctx context.Context, _ time.Duration) context.Context {
	return ctx
}
func (s *stubIdentityHost) NormalizeUserBrandColorPreset(value string) (string, bool) {
	if value == "" {
		return "", true
	}
	switch value {
	case "blue", "violet", "teal", "green", "orange", "pink":
		return value, true
	default:
		return "", false
	}
}
func (s *stubIdentityHost) Mode() string                     { return "development" }
func (s *stubIdentityHost) PublicBaseURL() string            { return "https://devops.example.com" }
func (s *stubIdentityHost) TrustedProxyCIDRs() []string      { return nil }
func (s *stubIdentityHost) RateLimiter() Limiter             { return nil }
func (s *stubIdentityHost) OAuthStateStore() OAuthStateStore { return nil }
func (s *stubIdentityHost) SecretStore() secret.Store        { return secret.Store{} }

func TestIdentityAIToolsRegisterWithExactSpecsSensitivePathsAndVerifications(t *testing.T) {
	reg := aitool.NewRegistry()
	New(&stubIdentityHost{}).RegisterAITools(reg)

	if reg.Len() != 7 {
		t.Fatalf("registered tools = %d, want 7", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences: %v", err)
	}

	cases := []struct {
		name           string
		domain         string
		tier           string
		effect         string
		adminOnly      bool
		preview        bool
		sensitivePaths []string
		verifications  int
	}{
		{name: "ops_get_my_account_security", domain: aitool.DomainOps, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		{name: "ops_update_my_profile", domain: aitool.DomainOps, tier: aitool.ApprovalTierL1, effect: aitool.EffectWrite, verifications: 1},
		{name: "ops_revoke_my_credential_or_grant", domain: aitool.DomainOps, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, preview: true, verifications: 4},
		{name: "admin_list_users_and_auth", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead, adminOnly: true},
		{name: "admin_save_user", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, adminOnly: true, preview: true, sensitivePaths: []string{"password"}, verifications: 1},
		{name: "admin_configure_auth_policy", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, adminOnly: true, preview: true, verifications: 1},
		{name: "admin_save_oidc_provider", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, adminOnly: true, preview: true, sensitivePaths: []string{"clientSecret"}, verifications: 1},
	}

	for _, tc := range cases {
		tool, ok := reg.Get(tc.name)
		if !ok {
			t.Fatalf("missing tool %s", tc.name)
		}
		if tool.Catalog.Domain != tc.domain || tool.Catalog.ApprovalTier != tc.tier || tool.Catalog.Effect != tc.effect {
			t.Fatalf("%s metadata = (%s, %s, %s), want (%s, %s, %s)",
				tc.name, tool.Catalog.Domain, tool.Catalog.ApprovalTier, tool.Catalog.Effect, tc.domain, tc.tier, tc.effect)
		}
		if tool.Catalog.AdminOnly != tc.adminOnly || tool.Catalog.GlobalSessionOnly != tc.adminOnly {
			t.Fatalf("%s adminOnly/globalSessionOnly = (%v, %v), want %v",
				tc.name, tool.Catalog.AdminOnly, tool.Catalog.GlobalSessionOnly, tc.adminOnly)
		}
		if tool.HasPreview != tc.preview {
			t.Fatalf("%s HasPreview = %v, want %v", tc.name, tool.HasPreview, tc.preview)
		}
		if !slices.Equal(tool.Catalog.SensitivePaths, tc.sensitivePaths) {
			t.Fatalf("%s sensitivePaths = %v, want %v", tc.name, tool.Catalog.SensitivePaths, tc.sensitivePaths)
		}
		if len(tool.Catalog.Verification) != tc.verifications {
			t.Fatalf("%s verifications len = %d, want %d", tc.name, len(tool.Catalog.Verification), tc.verifications)
		}

		role := authz.PlatformRoleUser
		if tc.adminOnly {
			role = authz.PlatformRoleAdmin
		}
		for _, sc := range tool.Catalog.RequiredScopes {
			if norm := authz.NormalizeOAuthScope(sc); norm != sc || !authz.UserCanAuthorizeOAuthScope(role, sc) {
				t.Fatalf("%s invalid requiredScope %q for role %s", tc.name, sc, role)
			}
		}
	}
}

func TestIdentityAIToolsPokaYokeAndAntiLockoutGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := New(&stubIdentityHost{})
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	parentCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	parentCtx.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
	admin := model.User{ID: "usr_admin", Email: "admin@example.com", Name: "Admin", Role: authz.PlatformRoleAdmin}
	tc := aitool.NewToolContext(parentCtx, nil, admin, "ses_1", "conv_1", "", "run_1", "call_1", "", "model", nil)

	// 1. ops_update_my_profile rejects empty string avatarUrl with hint to use clearAvatar
	profileTool, _ := reg.Get("ops_update_my_profile")
	if _, err := profileTool.Execute(tc, json.RawMessage(`{"avatarUrl":""}`)); err == nil {
		t.Fatal("expected empty avatarUrl to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "user.avatar_empty_forbidden" || te.Hint == "" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 2. admin_save_oidc_provider forbids removing mandatory openid scope
	oidcTool, _ := reg.Get("admin_save_oidc_provider")
	if _, err := oidcTool.Preview(tc, json.RawMessage(`{"provider":"ap_1","removeScopes":["openid"]}`)); err == nil {
		t.Fatal("expected removing openid scope to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "argument_invalid" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 3. Self-protection fence in validateAndDiffUserUpdate blocks self-disable, self-demote, and self-password-reset
	boolTrue := true
	roleUser := authz.PlatformRoleUser
	pw := "newpassword123"
	for _, patch := range []adminSaveUserInput{
		{Disabled: &boolTrue},
		{Role: &roleUser},
		{Password: &pw},
	} {
		if _, _, _, err := h.validateAndDiffUserUpdate(nil, admin, admin, patch); err == nil {
			t.Fatalf("expected self-mutation %+v to fail", patch)
		} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "auth.self_credential_mutation_forbidden" {
			t.Fatalf("unexpected self-mutation error: %#v", err)
		}
	}

	// 4. auditResourceType recognizes all 7 required domain prefixes
	for _, action := range []string{
		"system_component.install",
		"build_environment.update",
		"mail.settings.update",
		"config.update",
		"platform_app_template.save",
		"notification.channel.create",
		"data_retention.cleanup",
		"external_identity.unbind",
	} {
		if got := auditResourceType(action); got == "unknown" {
			t.Fatalf("auditResourceType(%q) = unknown", action)
		}
	}
}
