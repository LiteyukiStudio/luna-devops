package platformapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type stubPlatformHost struct {
	db      *gorm.DB
	user    model.User
	allowed map[string]model.Project
}

func (s *stubPlatformHost) Config() sharedconfig.APIConfig { return sharedconfig.APIConfig{} }
func (s *stubPlatformHost) DBFor(_ *gin.Context) *gorm.DB  { return s.db }
func (s *stubPlatformHost) DBWithContext(_ context.Context) *gorm.DB {
	return s.db
}
func (s *stubPlatformHost) CurrentUser(_ *gin.Context) (model.User, bool) {
	return s.user, s.user.ID != ""
}
func (s *stubPlatformHost) RequestAllowsScope(_ *gin.Context, _ string) bool { return true }
func (s *stubPlatformHost) AIConversationProjectID(_ *gin.Context) string    { return "" }
func (s *stubPlatformHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, _ authz.Action) (model.User, model.Project, bool) {
	if proj, ok := s.allowed[projectID]; ok {
		return s.user, proj, true
	}
	writeErrorCode(ctx, http.StatusForbidden, "project.forbidden", "forbidden")
	return model.User{}, model.Project{}, false
}
func (s *stubPlatformHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, action authz.Action) (model.User, model.Project, bool) {
	return s.AuthorizeProjectByID(ctx, projectRef, action)
}
func (s *stubPlatformHost) ResolveListVisibility(_ *gin.Context, _ model.User) (projectservice.ListVisibility, bool) {
	return projectservice.ListVisibilityRelated, true
}
func (s *stubPlatformHost) ProjectIDsForUser(_ context.Context, _ string) []string { return nil }
func (s *stubPlatformHost) EnsurePlatformSystemProject(_ model.User, _ context.Context) (model.Project, error) {
	return model.Project{ID: "prj_sys", Identifier: "luna-system"}, nil
}
func (s *stubPlatformHost) ObserveRuntimeClusters(_ context.Context, _ []model.RuntimeCluster) {}
func (s *stubPlatformHost) DashboardRegistryAvailable(_ context.Context, _ model.User, _ model.ArtifactRegistry) bool {
	return true
}
func (s *stubPlatformHost) RequirePlatformAdmin(_ *gin.Context) bool {
	return authz.IsPlatformAdmin(s.user.Role)
}
func (s *stubPlatformHost) AuditWithContext(_, _, _ string, _ bool, _ string, _ context.Context) {}
func (s *stubPlatformHost) AllowRate(_ context.Context, _ string, _ int, _ time.Duration) (bool, error) {
	return true, nil
}

func newDryRunPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "postgres://test:test@127.0.0.1:1/test?sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true, DryRun: true})
	if err != nil {
		t.Fatalf("open dry-run postgres: %v", err)
	}
	return db
}

func TestPlatformAIToolsRegisterWithExactSpecsAndScopes(t *testing.T) {
	reg := aitool.NewRegistry()
	h := New(&stubPlatformHost{})
	h.RegisterAITools(reg)

	if reg.Len() != 4 {
		t.Fatalf("registered tools = %d, want 4", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences: %v", err)
	}

	cases := []struct {
		name      string
		domain    string
		tier      string
		effect    string
		adminOnly bool
		preview   bool
	}{
		{name: "ops_search_global", domain: aitool.DomainOps, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead, adminOnly: false, preview: false},
		{name: "ops_list_events", domain: aitool.DomainOps, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead, adminOnly: false, preview: false},
		{name: "admin_preview_data_retention", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL0, effect: aitool.EffectRead, adminOnly: true, preview: false},
		{name: "admin_execute_data_retention_cleanup", domain: aitool.DomainAdmin, tier: aitool.ApprovalTierL2, effect: aitool.EffectExecute, adminOnly: true, preview: true},
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
		if len(tool.Catalog.SensitivePaths) != 0 {
			t.Fatalf("%s unexpected sensitivePaths: %v", tc.name, tool.Catalog.SensitivePaths)
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

func TestPlatformAIToolsValidationAndSandboxGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newDryRunPostgresDB(t)
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}
	tenant := model.User{ID: "usr_tenant", Role: authz.PlatformRoleUser}

	host := &stubPlatformHost{db: db, user: admin}
	reg := aitool.NewRegistry()
	New(host).RegisterAITools(reg)

	parentCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	parentCtx.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", nil)

	// 1. admin_preview_data_retention catalog mode returns 6 datasets
	previewTool, _ := reg.Get("admin_preview_data_retention")
	tcAdmin := aitool.NewToolContext(parentCtx, db, admin, "ses_1", "conv_1", "", "run_1", "call_1", "", "model", nil)
	catOut, err := previewTool.Execute(tcAdmin, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("admin_preview_data_retention catalog failed: %v", err)
	}
	catMap := catOut.(map[string]any)
	if catMap["mode"] != "catalog" {
		t.Fatalf("unexpected mode: %#v", catMap)
	}

	// 2. admin_preview_data_retention rejects non-admin and invalid range
	tcTenant := aitool.NewToolContext(parentCtx, db, tenant, "ses_2", "conv_2", "", "run_2", "call_2", "", "model", nil)
	if _, err := previewTool.Execute(tcTenant, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected non-admin to be rejected")
	}
	if _, err := previewTool.Execute(tcAdmin, json.RawMessage(`{"datasets":["platform_events"],"startAt":"2026-07-02T00:00:00Z","endAt":"2026-07-01T00:00:00Z"}`)); err == nil {
		t.Fatal("expected inverted range to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "retention.invalid_range" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 3. ops_search_global rejects visibility=all for tenant and for project-bound admin
	searchTool, _ := reg.Get("ops_search_global")
	if _, err := searchTool.Execute(tcTenant, json.RawMessage(`{"visibility":"all","query":"demo"}`)); err == nil {
		t.Fatal("expected visibility=all for tenant to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "auth.forbidden" {
		t.Fatalf("unexpected error: %#v", err)
	}
	boundTC := aitool.NewToolContext(parentCtx, db, admin, "ses_1", "conv_1", "prj_bound", "run_1", "call_3", "", "model", nil)
	if _, err := searchTool.Execute(boundTC, json.RawMessage(`{"visibility":"all","query":"demo"}`)); err == nil {
		t.Fatal("expected visibility=all in project-bound session to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "ai.project_scope_mismatch" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 4. ops_list_events requires project when filtering by application
	eventsTool, _ := reg.Get("ops_list_events")
	if _, err := eventsTool.Execute(tcAdmin, json.RawMessage(`{"application":"app_1"}`)); err == nil {
		t.Fatal("expected application without project to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "project.required" {
		t.Fatalf("unexpected error: %#v", err)
	}

	// 5. admin_execute_data_retention_cleanup preview validates admin role and range before DB query
	cleanupTool, _ := reg.Get("admin_execute_data_retention_cleanup")
	if _, err := cleanupTool.Preview(tcTenant, json.RawMessage(`{
		"datasets": ["platform_events"],
		"startAt": "2025-01-01T00:00:00Z",
		"endAt": "2025-02-01T00:00:00Z"
	}`)); err == nil {
		t.Fatal("expected non-admin preview to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "config.admin.required" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if _, err := cleanupTool.Preview(tcAdmin, json.RawMessage(`{
		"datasets": ["platform_events"],
		"startAt": "2025-02-01T00:00:00Z",
		"endAt": "2025-01-01T00:00:00Z"
	}`)); err == nil {
		t.Fatal("expected inverted range preview to fail")
	} else if te, ok := aitool.AsToolError(err); !ok || te.Code != "retention.invalid_range" {
		t.Fatalf("unexpected error: %#v", err)
	}
}
