package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestSystemConfigAIToolsRegisterMaskSecretsAndEnforceLinkageGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := aitool.NewRegistry()
	h := &Handlers{
		configs: &configCache{
			values: map[string]string{
				aiapi.AssistantEnabledConfigKey: "true",
				"ai.provider.base_url":          "https://api.openai.com/v1",
				"ai.provider.api_key":           "secret-id:super-secret-ciphertext-must-never-leak",
				"site.title":                    "Luna DevOps",
			},
		},
	}
	h.registerConfigAITools(reg)

	if reg.Len() != 2 {
		t.Fatalf("registered tools = %d, want 2", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences: %v", err)
	}

	getTool, ok := reg.Get("admin_get_system_configs")
	if !ok || getTool.Catalog.ApprovalTier != aitool.ApprovalTierL0 || !getTool.Catalog.AdminOnly {
		t.Fatalf("unexpected admin_get_system_configs: %#v", getTool)
	}
	updateTool, ok := reg.Get("admin_update_system_configs")
	if !ok || updateTool.Catalog.ApprovalTier != aitool.ApprovalTierL2 || !updateTool.Catalog.AdminOnly || !updateTool.HasPreview {
		t.Fatalf("unexpected admin_update_system_configs: %#v", updateTool)
	}
	if !slices.Equal(updateTool.Catalog.SensitivePaths, []string{"secretActions.*.value"}) {
		t.Fatalf("sensitivePaths = %v, want [secretActions.*.value]", updateTool.Catalog.SensitivePaths)
	}

	for _, tool := range []*aitool.RegisteredTool{getTool, updateTool} {
		for _, sc := range tool.Catalog.RequiredScopes {
			if norm := authz.NormalizeOAuthScope(sc); norm != sc || !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleAdmin, sc) {
				t.Fatalf("%s invalid requiredScope %q", tool.Catalog.OperationID, sc)
			}
		}
	}

	parentCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	parentCtx.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}
	tc := aitool.NewToolContext(parentCtx, nil, admin, "ses_1", "conv_1", "", "run_1", "call_1", "", "model", nil)

	// 1. admin_get_system_configs returns all 49 definitions and never leaks secret ciphertext/plaintext
	out, err := getTool.Execute(tc, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("admin_get_system_configs failed: %v", err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "super-secret-ciphertext-must-never-leak") {
		t.Fatalf("secret reference leaked in admin_get_system_configs output: %s", string(encoded))
	}
	outMap := out.(map[string]any)
	if outMap["total"] != 49 {
		t.Fatalf("total configs = %v, want 49", outMap["total"])
	}
	secretStatus := outMap["secretStatus"].(map[string]bool)
	if !secretStatus["ai.provider.api_key"] || secretStatus["ai.web.proxy_pool"] {
		t.Fatalf("unexpected secretStatus: %#v", secretStatus)
	}

	// 2. admin_update_system_configs isolates secret keys from setConfigs with R11 hint
	if _, err := updateTool.Preview(tc, json.RawMessage(`{
		"setConfigs": {"ai.provider.api_key": "sk-plaintext"}
	}`)); err == nil {
		t.Fatal("expected secret key in setConfigs to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "config.secret_key_isolated" || te.Hint == "" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 3. admin_update_system_configs blocks clearing ai.provider.api_key or ai.provider.base_url while ai.assistant.enabled == true
	if _, err := updateTool.Preview(tc, json.RawMessage(`{
		"secretActions": [{"key": "ai.provider.api_key", "operation": "clear"}]
	}`)); err == nil {
		t.Fatal("expected clearing ai.provider.api_key while assistant enabled to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "ai.config_in_use" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if _, err := updateTool.Preview(tc, json.RawMessage(`{
		"clearConfigKeys": ["ai.provider.base_url"]
	}`)); err == nil {
		t.Fatal("expected clearing ai.provider.base_url while assistant enabled to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "ai.config_in_use" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 4. Preview succeeds and records sensitiveKeys without leaking secret value in PreviewChange
	prev, err := updateTool.Preview(tc, json.RawMessage(`{
		"setConfigs": {"site.title": "Luna Cloud"},
		"secretActions": [{"key": "ai.observability.prometheus_token", "operation": "set", "value": "prom-secret-val"}]
	}`))
	if err != nil {
		t.Fatalf("preview valid update failed: %v", err)
	}
	prevJSON, _ := json.Marshal(prev.Preview)
	if strings.Contains(string(prevJSON), "prom-secret-val") {
		t.Fatalf("preview leaked secret plaintext: %s", string(prevJSON))
	}
	if !slices.Equal(prev.Preview.SensitiveKeys, []string{"ai.observability.prometheus_token"}) {
		t.Fatalf("unexpected preview sensitiveKeys: %v", prev.Preview.SensitiveKeys)
	}
}
