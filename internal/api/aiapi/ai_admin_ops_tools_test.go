package aiapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type stubAIAdminOpsHost struct {
	db                *gorm.DB
	configs           map[string]string
	aiAgent           aiagent.Client
	deploymentEnabled bool
	webTools          *aitool.Service
	reg               *aitool.Registry
}

func (s *stubAIAdminOpsHost) DBFor(_ *gin.Context) *gorm.DB { return s.db }
func (s *stubAIAdminOpsHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	if v, ok := ctx.Get("luna.devops.current_user"); ok {
		if u, ok := v.(model.User); ok {
			return u, true
		}
	}
	return model.User{}, false
}
func (s *stubAIAdminOpsHost) CurrentSessionFromCookie(_ *gin.Context) (model.UserSession, bool) {
	return model.UserSession{}, false
}
func (s *stubAIAdminOpsHost) SetCurrentUser(ctx *gin.Context, user model.User) {
	ctx.Set("luna.devops.current_user", user)
}
func (s *stubAIAdminOpsHost) AuditWithContext(_, _, _ string, _ bool, _ string, _ context.Context) {}
func (s *stubAIAdminOpsHost) ConfigAvailable() bool                                                { return true }
func (s *stubAIAdminOpsHost) ConfigValues(keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := s.configs[k]; ok {
			out[k] = v
		}
	}
	return out
}
func (s *stubAIAdminOpsHost) AllConfigValues() map[string]string {
	out := make(map[string]string, len(s.configs))
	for k, v := range s.configs {
		out[k] = v
	}
	return out
}
func (s *stubAIAdminOpsHost) ConfigDefinition(_ string) *ConfigDefinition { return nil }
func (s *stubAIAdminOpsHost) ResolveSecret(_ context.Context, _ string) string {
	return ""
}
func (s *stubAIAdminOpsHost) AIAgent() aiagent.Client { return s.aiAgent }
func (s *stubAIAdminOpsHost) AIDeploymentEnabled() bool {
	return s.deploymentEnabled
}
func (s *stubAIAdminOpsHost) AIActorOverride(_ *gin.Context) (aiagent.ActorContext, string, bool, bool) {
	return aiagent.ActorContext{}, "", false, false
}
func (s *stubAIAdminOpsHost) FindProjectForCurrentUserByID(_ *gin.Context, _ string) (model.Project, bool) {
	return model.Project{}, false
}
func (s *stubAIAdminOpsHost) AIAgentCallbackServiceToken() string         { return "token" }
func (s *stubAIAdminOpsHost) AIAgentPreviousCallbackServiceToken() string { return "" }
func (s *stubAIAdminOpsHost) AIPlatformActor(_ context.Context) (PlatformActor, bool) {
	return PlatformActor{}, false
}
func (s *stubAIAdminOpsHost) WithAIPlatformActor(ctx context.Context, _ PlatformActor) context.Context {
	return ctx
}
func (s *stubAIAdminOpsHost) AIToolService() *aitool.Service   { return s.webTools }
func (s *stubAIAdminOpsHost) AIToolRegistry() *aitool.Registry { return s.reg }
func (s *stubAIAdminOpsHost) AuthenticateMCPBearerToken(_ *gin.Context) (model.User, model.AccessToken, bool) {
	return model.User{}, model.AccessToken{}, false
}
func (s *stubAIAdminOpsHost) ExternalBaseURL(_ *gin.Context) string {
	return "https://devops.example.com"
}
func (s *stubAIAdminOpsHost) MCPPreviewSigningSecret() []byte {
	return []byte("test-mcp-secret-32-bytes-minimum")
}
func (s *stubAIAdminOpsHost) AppVersion() string { return "0.1.0-test" }
func (s *stubAIAdminOpsHost) AuthorizeProject(_ *gin.Context, _ authz.Action) (model.User, model.Project, bool) {
	return model.User{}, model.Project{}, false
}
func (s *stubAIAdminOpsHost) AuthorizeProjectByID(_ *gin.Context, _ string, _ authz.Action) (model.User, model.Project, bool) {
	return model.User{}, model.Project{}, false
}
func (s *stubAIAdminOpsHost) AuthorizeProjectRef(_ *gin.Context, _ string, _ authz.Action) (model.User, model.Project, bool) {
	return model.User{}, model.Project{}, false
}
func (s *stubAIAdminOpsHost) MonitorProjectAuthorization(_ *gin.Context, _ context.Context, _ model.User, _ string, _ authz.Action, _ func()) (<-chan struct{}, bool) {
	return nil, false
}
func (s *stubAIAdminOpsHost) WriteContinuousAuthorizationRevoked(_ *gin.Context) {}

type stubAIAdminOpsAgentClient struct {
	lastActor aiagent.ActorContext
}

func (c *stubAIAdminOpsAgentClient) Do(_ context.Context, actor aiagent.ActorContext, _ aiagent.Request) (*aiagent.Response, error) {
	c.lastActor = actor
	return &aiagent.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"reachable":true,"model":"gpt-test"}`)),
	}, nil
}

func TestAIAdminOpsToolsRegisterWithExactSpecsSensitivePathsAndScopes(t *testing.T) {
	reg := aitool.NewRegistry()
	New(&stubAIAdminOpsHost{}).RegisterAITools(reg)

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
		{name: "ops_web_search", domain: aitool.DomainOps, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		{name: "ops_web_fetch", domain: aitool.DomainOps, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		{name: "admin_list_ai_models", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead, adminOnly: true},
		{name: "admin_inspect_agent_observability", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead, adminOnly: true},
		{name: "admin_save_ai_model", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, adminOnly: true, preview: true, verifications: 1},
		{name: "admin_delete_ai_model", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite, adminOnly: true, preview: true, verifications: 1},
		{name: "admin_test_ai_integration", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectExecute, adminOnly: true, preview: true, sensitivePaths: []string{"token"}},
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
			t.Fatalf("%s verifications = %d, want %d", tc.name, len(tool.Catalog.Verification), tc.verifications)
		}

		role := authz.PlatformRoleUser
		if tc.adminOnly {
			role = authz.PlatformRoleAdmin
		}
		if !aiOperationScopesAllowed(role, tool.Catalog.RequiredScopes) {
			t.Fatalf("%s scopes %v rejected by aiOperationScopesAllowed for role %s", tc.name, tool.Catalog.RequiredScopes, role)
		}
	}

	delTool, _ := reg.Get("admin_delete_ai_model")
	if delTool.Catalog.Verification[0].Expect != "absent" {
		t.Fatalf("expected admin_delete_ai_model verification expect=absent, got %q", delTool.Catalog.Verification[0].Expect)
	}
}

func TestAIAdminOpsToolsValidationAndCookieFreeProviderTest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	agentStub := &stubAIAdminOpsAgentClient{}
	host := &stubAIAdminOpsHost{
		configs: map[string]string{
			AssistantEnabledConfigKey: "true",
		},
		aiAgent:           agentStub,
		deploymentEnabled: true,
	}
	reg := aitool.NewRegistry()
	New(host).RegisterAITools(reg)

	parentCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	parentCtx.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin, Language: "zh-CN"}
	tc := aitool.NewToolContext(parentCtx, nil, admin, "ses_actor_1", "conv_1", "", "run_1", "call_1", "", "model", nil)

	// 1. admin_test_ai_integration tests ai_provider directly via verified ActorContext without browser cookie
	testTool, _ := reg.Get("admin_test_ai_integration")
	prev, err := testTool.Preview(tc, json.RawMessage(`{"target":"ai_provider"}`))
	if err != nil || prev.Preview.TargetResource != "ai_provider" {
		t.Fatalf("preview ai_provider failed: %#v, err=%v", prev, err)
	}
	res, err := testTool.Execute(tc, json.RawMessage(`{"target":"ai_provider"}`))
	if err != nil {
		t.Fatalf("execute ai_provider failed: %v", err)
	}
	resMap := res.(map[string]any)
	if resMap["reachable"] != true || agentStub.lastActor.UserID != "usr_admin" || agentStub.lastActor.SessionID != "ses_actor_1" {
		t.Fatalf("unexpected provider test result/actor: %#v / %#v", resMap, agentStub.lastActor)
	}

	// 2. admin_test_ai_integration observability_source guides with hint when URL is missing
	if _, err := testTool.Preview(tc, json.RawMessage(`{"target":"observability_source","source":"prometheus"}`)); err == nil {
		t.Fatal("expected missing prometheus url to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "ai.observability.url_required" || te.Hint == "" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 3. Last-enabled AI model protection in mergeAndValidateAIModelPatch
	existing := model.AIModel{
		ID:                           "aimod_1",
		Name:                         "gpt-main",
		Enabled:                      true,
		MaxContextTokens:             128000,
		MaxOutputTokens:              8192,
		InputCreditsPerMillion:       decimal.NewFromInt(10),
		OutputCreditsPerMillion:      decimal.NewFromInt(30),
		CachedInputCreditsPerMillion: decimal.NewFromInt(2),
	}
	disable := false
	if _, _, err := mergeAndValidateAIModelPatch(existing, []model.AIModel{existing}, adminSaveAIModelInput{Enabled: &disable}); err == nil {
		t.Fatal("expected disabling last enabled model to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "ai.last_model_cannot_be_disabled" {
		t.Fatalf("unexpected error: %#v", err)
	}
}
