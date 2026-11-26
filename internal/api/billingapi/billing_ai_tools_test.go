package billingapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type stubBillingHost struct {
	db     *gorm.DB
	user   model.User
	config map[string]string
}

func (s *stubBillingHost) DBFor(_ *gin.Context) *gorm.DB            { return s.db }
func (s *stubBillingHost) DBWithContext(_ context.Context) *gorm.DB { return s.db }
func (s *stubBillingHost) CurrentUser(_ *gin.Context) (model.User, bool) {
	return s.user, s.user.ID != ""
}
func (s *stubBillingHost) AuditWithContext(_, _, _ string, _ bool, _ string, _ context.Context) {}
func (s *stubBillingHost) ConfigValues(keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := s.config[k]; ok {
			out[k] = v
		}
	}
	return out
}
func (s *stubBillingHost) DefaultRuntimeClusterID(_ context.Context) string { return "" }
func (s *stubBillingHost) ObserveSystemComponentInstallations(_ context.Context, _ []model.SystemComponentInstallation) {
}
func (s *stubBillingHost) SystemComponentForBearerToken(_, _ string, _ context.Context) (model.SystemComponentInstallation, bool) {
	return model.SystemComponentInstallation{}, false
}

func TestBillingAIToolsRegisterWithExactSpecsVerificationsAndScopes(t *testing.T) {
	reg := aitool.NewRegistry()
	New(&stubBillingHost{}).RegisterAITools(reg)

	if reg.Len() != 5 {
		t.Fatalf("registered tools = %d, want 5", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences: %v", err)
	}

	cases := []struct {
		name                string
		domain              string
		tier                string
		effect              string
		adminOnly           bool
		personalProjectOmit bool
		preview             bool
		verificationOp      string
		verificationFamily  string
	}{
		{
			name: "ops_get_billing_summary", domain: aitool.DomainOps, tier: aitool.ApprovalTierL0,
			effect: aitool.EffectRead, personalProjectOmit: true,
		},
		{
			name: "ops_list_billing_records", domain: aitool.DomainOps, tier: aitool.ApprovalTierL0,
			effect: aitool.EffectRead, personalProjectOmit: true,
		},
		{
			name: "admin_inspect_billing", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL0,
			effect: aitool.EffectRead, adminOnly: true,
		},
		{
			name: "admin_update_billing_rate_rules", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2,
			effect: aitool.EffectWrite, adminOnly: true, preview: true,
			verificationOp: "admin_inspect_billing", verificationFamily: "billing_rate_rules",
		},
		{
			name: "admin_adjust_user_wallet", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2,
			effect: aitool.EffectWrite, adminOnly: true, preview: true,
			verificationOp: "admin_inspect_billing", verificationFamily: "resource",
		},
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
		if tool.AllowPersonalProjectOmit != tc.personalProjectOmit {
			t.Fatalf("%s AllowPersonalProjectOmit = %v, want %v",
				tc.name, tool.AllowPersonalProjectOmit, tc.personalProjectOmit)
		}
		if tool.HasPreview != tc.preview {
			t.Fatalf("%s HasPreview = %v, want %v", tc.name, tool.HasPreview, tc.preview)
		}
		if tc.verificationOp != "" {
			if len(tool.Catalog.Verification) != 1 ||
				tool.Catalog.Verification[0].OperationID != tc.verificationOp ||
				tool.Catalog.Verification[0].Family != tc.verificationFamily {
				t.Fatalf("%s verification = %#v", tc.name, tool.Catalog.Verification)
			}
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

	// Verify rate_rules staticArguments on admin_update_billing_rate_rules
	rateTool, _ := reg.Get("admin_update_billing_rate_rules")
	if rateTool.Catalog.Verification[0].StaticArguments["view"] != "rate_rules" {
		t.Fatalf("expected staticArguments.view=rate_rules, got %#v", rateTool.Catalog.Verification[0].StaticArguments)
	}
}

func TestBillingAIToolsPokaYokeValidations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := aitool.NewRegistry()
	New(&stubBillingHost{}).RegisterAITools(reg)

	parentCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	parentCtx.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}
	tc := aitool.NewToolContext(parentCtx, nil, admin, "ses_1", "conv_1", "", "run_1", "call_1", "idem_12345678", "model", nil)

	// 1. ops_list_billing_records rejects project + ai.* meter combination
	recordsTool, _ := reg.Get("ops_list_billing_records")
	if _, err := recordsTool.Execute(tc, json.RawMessage(`{
		"recordType": "usage",
		"project": "prj_demo",
		"meter": "ai.prompt_tokens_1m"
	}`)); err == nil {
		t.Fatal("expected project + ai.* meter to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "billing.ai_meter_not_project_scoped" || te.Hint == "" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 1b. ops_list_billing_records rejects project + non-debit ledgerType with self-repair hint
	if _, err := recordsTool.Execute(tc, json.RawMessage(`{
		"recordType": "ledger",
		"project": "prj_demo",
		"ledgerType": "credit"
	}`)); err == nil {
		t.Fatal("expected project + ledgerType=credit to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "billing.argument_invalid" || te.Hint == "" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 1c. summaryToMap omits misleading 0 wallet balances when balanceScope=project
	projMap := summaryToMap(billing.ProjectBillingSummary{
		BalanceScope: billing.BalanceScopeProject,
		PeriodSpend:  decimal.NewFromInt(12),
	})
	if projMap["walletBalanceRedacted"] != true {
		t.Fatalf("expected walletBalanceRedacted=true, got %#v", projMap)
	}
	for _, redactedKey := range []string{"balanceCredits", "availableCredits", "lowBalanceLimit", "balanceStatus"} {
		if _, exists := projMap[redactedKey]; exists {
			t.Fatalf("expected %s to be omitted in project scope, got %#v", redactedKey, projMap)
		}
	}

	// 2. admin_adjust_user_wallet rejects negative amount when type=credit with self-repair hint
	adjustTool, _ := reg.Get("admin_adjust_user_wallet")
	if _, err := adjustTool.Preview(tc, json.RawMessage(`{
		"user": "usr_target",
		"type": "credit",
		"amountCredits": "-50",
		"description": "deduct"
	}`)); err == nil {
		t.Fatal("expected negative credit to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "billing.transaction_invalid_amount" || te.Hint == "" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 3. admin_update_billing_rate_rules rejects noop rule patch
	rateTool, _ := reg.Get("admin_update_billing_rate_rules")
	if _, err := rateTool.Preview(tc, json.RawMessage(`{
		"rules": [{"meter": "build.cpu_vcpu_minute"}]
	}`)); err == nil {
		t.Fatal("expected noop rate rule patch to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "billing.rate_rule_noop" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 4. admin_inspect_billing requires user when view=user_summary and rejects user when view=platform_summary
	inspectTool, _ := reg.Get("admin_inspect_billing")
	if _, err := inspectTool.Execute(tc, json.RawMessage(`{"view":"user_summary"}`)); err == nil {
		t.Fatal("expected missing user on user_summary to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "billing.user_required" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if _, err := inspectTool.Execute(tc, json.RawMessage(`{"view":"platform_summary","user":"usr_1"}`)); err == nil {
		t.Fatal("expected user on platform_summary to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "billing.argument_invalid" {
		t.Fatalf("unexpected error: %#v", err)
	}
}
