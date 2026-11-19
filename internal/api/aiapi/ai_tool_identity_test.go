package aiapi

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAIOperationScopesAllowed(t *testing.T) {
	if aiOperationScopesAllowed(authz.PlatformRoleAdmin, nil) {
		t.Fatal("empty requiredScopes must be rejected")
	}
	if aiOperationScopesAllowed(authz.PlatformRoleAdmin, []string{"unknown:scope"}) {
		t.Fatal("unregistered scope must be rejected even for platform admin")
	}
	if !aiOperationScopesAllowed(authz.PlatformRoleUser, []string{"web:read"}) {
		t.Fatal("web:read must be allowed for standard users")
	}
	if !aiOperationScopesAllowed(authz.PlatformRoleUser, []string{"project:read", "build:trigger"}) {
		t.Fatal("user-authorizable OAuth scopes must be allowed for standard users")
	}
	for _, adminScope := range []string{"user:manage", "retention:read", "retention:manage", "config:read", "agent-observability:read"} {
		if aiOperationScopesAllowed(authz.PlatformRoleUser, []string{adminScope}) {
			t.Fatalf("admin-only scope %q must be rejected for standard user", adminScope)
		}
		if !aiOperationScopesAllowed(authz.PlatformRoleAdmin, []string{adminScope}) {
			t.Fatalf("admin-only scope %q must be allowed for platform admin", adminScope)
		}
	}
}

func TestAIFacadeToolGatewayHardening(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := aitool.NewRegistry()

	var capturedReadProject string
	aitool.Register(reg, aitool.Spec[struct {
		Project    *string `json:"project,omitempty"`
		Visibility *string `json:"visibility,omitempty" enum:"related|all"`
	}, map[string]any]{
		Name:           "workspace_list_projects",
		Domain:         aitool.DomainWorkspace,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{"project:read"},
		Execute: func(_ *aitool.ToolContext, in struct {
			Project    *string `json:"project,omitempty"`
			Visibility *string `json:"visibility,omitempty" enum:"related|all"`
		}) (map[string]any, error) {
			if in.Project != nil {
				capturedReadProject = *in.Project
			} else {
				capturedReadProject = ""
			}
			return map[string]any{"items": []any{}, "injectedProject": capturedReadProject}, nil
		},
	})

	aitool.Register(reg, aitool.Spec[struct {
		URL string `json:"url"`
	}, map[string]any]{
		Name:           "ops_web_fetch",
		Domain:         aitool.DomainOps,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{"web:read"},
		Execute: func(_ *aitool.ToolContext, in struct {
			URL string `json:"url"`
		}) (map[string]any, error) {
			return map[string]any{"url": in.URL}, nil
		},
	})

	aitool.Register(reg, aitool.Spec[struct{}, map[string]any]{
		Name:           "admin_list_data_retention_catalog",
		Domain:         aitool.DomainAdmin,
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{"retention:read"},
		Execute: func(_ *aitool.ToolContext, _ struct{}) (map[string]any, error) {
			return map[string]any{"items": []any{}}, nil
		},
	})

	aitool.Register(reg, aitool.Spec[struct {
		Project string  `json:"project"`
		Stage   *string `json:"stage,omitempty"`
	}, map[string]any]{
		Name:           "ops_save_project_notification_rule",
		Domain:         aitool.DomainOps,
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{"notification:manage"},
		Verification:   []aitool.OperationVerification{{OperationID: "workspace_list_projects", Family: "resource"}},
		Execute: func(_ *aitool.ToolContext, in struct {
			Project string  `json:"project"`
			Stage   *string `json:"stage,omitempty"`
		}) (map[string]any, error) {
			return map[string]any{"id": "nrl_1", "projectId": in.Project}, nil
		},
	})

	var lockedSeen string
	aitool.Register(reg, aitool.Spec[struct {
		Name         string            `json:"name"`
		Scope        *string           `json:"scope,omitempty"`
		AddProjects  []string          `json:"addProjects,omitempty"`
		SetVariables map[string]string `json:"setVariables,omitempty"`
		UpsertGrants []struct {
			Project string `json:"project"`
		} `json:"upsertGrants,omitempty"`
	}, map[string]any]{
		Name:           "build_save_variable_set",
		Domain:         aitool.DomainBuild,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionBuildTrigger), string(authz.ActionSecretUpdate)},
		Verification:   []aitool.OperationVerification{{OperationID: "workspace_list_projects", Family: "resource"}},
		Preview: func(_ *aitool.ToolContext, in struct {
			Name         string            `json:"name"`
			Scope        *string           `json:"scope,omitempty"`
			AddProjects  []string          `json:"addProjects,omitempty"`
			SetVariables map[string]string `json:"setVariables,omitempty"`
			UpsertGrants []struct {
				Project string `json:"project"`
			} `json:"upsertGrants,omitempty"`
		}) (*aitool.PreviewResult, error) {
			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary: in.Name,
				},
				LockedPayload: json.RawMessage(`{"revision":7}`),
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in struct {
			Name         string            `json:"name"`
			Scope        *string           `json:"scope,omitempty"`
			AddProjects  []string          `json:"addProjects,omitempty"`
			SetVariables map[string]string `json:"setVariables,omitempty"`
			UpsertGrants []struct {
				Project string `json:"project"`
			} `json:"upsertGrants,omitempty"`
		}) (map[string]any, error) {
			lockedSeen = string(tc.LockedPayload)
			return map[string]any{"id": "bvs_1", "name": in.Name}, nil
		},
	})

	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences: %v", err)
	}

	state := &stubAIToolDBState{
		userRole:    authz.PlatformRoleUser,
		projectID:   "prj_bound",
		projectSlug: "bound-slug",
		toolStatus:  "running",
	}
	db := newStubGormDB(t, state)
	host := &stubAIToolHost{
		db:            db,
		reg:           reg,
		callbackToken: "primary-callback-token-32-bytes!!",
		previousToken: "previous-callback-token-32-bytes!",
	}
	handler := New(host)

	router := gin.New()
	router.POST("/internal/v1/ai/tools/preview", handler.PreviewAIToolInternal)
	router.POST("/internal/v1/ai/tools/execute", handler.ExecuteAIToolInternal)

	callGateway := func(endpoint, token, toolCallID, toolName, argsJSON, previewLock string) *httptest.ResponseRecorder {
		t.Helper()
		hash, hashErr := aitool.CanonicalJSONHash([]byte(argsJSON))
		state.mu.Lock()
		state.operationID = toolName
		if hashErr == nil {
			state.argumentsHash = hash
		} else {
			state.argumentsHash = ""
		}
		state.mu.Unlock()

		body := `{"toolName":"` + toolName + `","arguments":` + argsJSON + `}`
		req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(RunIDHeader, "airun_1")
		req.Header.Set(ToolCallIDHeader, toolCallID)
		if previewLock != "" {
			req.Header.Set("X-Luna-AI-Preview-Lock", previewLock)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1) Standard user invoking adminOnly tool -> 403 ai.tool_permission_denied
	rec := callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_admin", "admin_list_data_retention_catalog", `{}`, "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.tool_permission_denied") {
		t.Fatalf("expected 403 ai.tool_permission_denied, got %d %s", rec.Code, rec.Body.String())
	}

	// 2) Platform admin invoking adminOnly tool inside a project-bound conversation -> 403 ai.project_scope_mismatch
	state.mu.Lock()
	state.userRole = authz.PlatformRoleAdmin
	state.mu.Unlock()
	rec = callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_admin_proj", "admin_list_data_retention_catalog", `{}`, "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch for adminOnly in project conversation, got %d %s", rec.Code, rec.Body.String())
	}
	state.mu.Lock()
	state.userRole = authz.PlatformRoleUser
	state.mu.Unlock()

	// 3) L0 read tool with rotated previous callback token -> 200 OK, auto-injects optional project, writes ZERO AuditLogs
	rec = callGateway("/internal/v1/ai/tools/execute", host.previousToken, "tc_read", "workspace_list_projects", `{}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for L0 read tool with previous token, got %d %s", rec.Code, rec.Body.String())
	}
	if capturedReadProject != "prj_bound" {
		t.Fatalf("expected auto-injected project prj_bound, got %q", capturedReadProject)
	}
	if len(host.audits) != 0 {
		t.Fatalf("L0 read tool must write zero AuditLogs, got %#v", host.audits)
	}

	// 4) web:read L0 execute-effect tool -> 200 OK and writes 1 AuditLog
	rec = callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_web", "ops_web_fetch", `{"url":"https://example.com"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for ops_web_fetch, got %d %s", rec.Code, rec.Body.String())
	}
	if len(host.audits) != 1 || host.audits[0] != "ops_web_fetch:tc_web" {
		t.Fatalf("expected 1 AuditLog for ops_web_fetch, got %#v", host.audits)
	}

	// 5) Tampered wire arguments vs persisted arguments_hash -> 409 ai.tool_execution_binding_invalid
	state.mu.Lock()
	state.operationID = "ops_web_fetch"
	state.argumentsHash = "deadbeef"
	state.mu.Unlock()
	tamperedReq := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", strings.NewReader(`{"toolName":"ops_web_fetch","arguments":{"url":"https://evil.example"}}`))
	tamperedReq.Header.Set("Authorization", "Bearer "+host.callbackToken)
	tamperedReq.Header.Set("Content-Type", "application/json")
	tamperedReq.Header.Set(RunIDHeader, "airun_1")
	tamperedReq.Header.Set(ToolCallIDHeader, "tc_tampered")
	tamperedRec := httptest.NewRecorder()
	router.ServeHTTP(tamperedRec, tamperedReq)
	if tamperedRec.Code != http.StatusConflict || !strings.Contains(tamperedRec.Body.String(), "ai.tool_execution_binding_invalid") {
		t.Fatalf("expected 409 ai.tool_execution_binding_invalid on arguments_hash mismatch, got %d %s", tamperedRec.Code, tamperedRec.Body.String())
	}

	// 6) L1 approval & prod stage escalation to L2
	rec = callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_l1_unapproved", "ops_save_project_notification_rule", `{"project":"prj_bound","stage":"dev"}`, "")
	if rec.Code != http.StatusPreconditionRequired || !strings.Contains(rec.Body.String(), "ai.approval_required") {
		t.Fatalf("expected 428 for unapproved L1, got %d %s", rec.Code, rec.Body.String())
	}
	state.mu.Lock()
	state.approvalDecision = "remembered"
	state.rememberedApproval = false
	state.mu.Unlock()
	rec = callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_l1_forged", "ops_save_project_notification_rule", `{"project":"prj_bound","stage":"dev"}`, "")
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 for forged remembered approval, got %d %s", rec.Code, rec.Body.String())
	}
	state.mu.Lock()
	state.rememberedApproval = true
	state.mu.Unlock()
	rec = callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_l1_dev", "ops_save_project_notification_rule", `{"project":"bound-slug","stage":"dev"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for dev L1 with remembered grant and project slug, got %d %s", rec.Code, rec.Body.String())
	}
	rec = callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_l1_prod", "ops_save_project_notification_rule", `{"project":"prj_bound","stage":"prod"}`, "")
	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 when stage=prod escalates L1 to L2, got %d %s", rec.Code, rec.Body.String())
	}

	// 7) L2 preview -> preview_lock -> execute lifecycle + nested map safety + sandbox enforcement
	nestedArgs := `{"name":"ci-vars","scope":"project","addProjects":["prj_bound"],"setVariables":{"FIREBASE_PROJECT":"{\"projectId\":\"prj_other\"}","projectId":"prj_other"}}`
	state.mu.Lock()
	state.toolStatus = "proposed"
	state.approvalDecision = ""
	state.mu.Unlock()
	prevRec := callGateway("/internal/v1/ai/tools/preview", host.callbackToken, "tc_l2", "build_save_variable_set", nestedArgs, "")
	if prevRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from preview, got %d %s", prevRec.Code, prevRec.Body.String())
	}
	var prevPayload struct {
		ApprovalPreview aitool.ApprovalPreview `json:"approvalPreview"`
		PreviewLock     string                 `json:"previewLock"`
	}
	if err := json.Unmarshal(prevRec.Body.Bytes(), &prevPayload); err != nil || prevPayload.PreviewLock == "" {
		t.Fatalf("decode preview response: %v %#v", err, prevPayload)
	}

	state.mu.Lock()
	state.toolStatus = "running"
	state.approvalDecision = "approve"
	state.mu.Unlock()
	noLockRec := callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_l2", "build_save_variable_set", nestedArgs, "")
	if noLockRec.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428 when L2 execute omits X-Luna-AI-Preview-Lock, got %d %s", noLockRec.Code, noLockRec.Body.String())
	}
	badLockRec := callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_l2", "build_save_variable_set", nestedArgs, prevPayload.PreviewLock+"tampered")
	if badLockRec.Code != http.StatusConflict || !strings.Contains(badLockRec.Body.String(), "ai.preview_lock_invalid") {
		t.Fatalf("expected 409 ai.preview_lock_invalid on tampered lock, got %d %s", badLockRec.Code, badLockRec.Body.String())
	}
	okLockRec := callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_l2", "build_save_variable_set", nestedArgs, prevPayload.PreviewLock)
	if okLockRec.Code != http.StatusOK || lockedSeen != `{"revision":7}` {
		t.Fatalf("expected 200 and locked payload delivery, got %d %s (locked=%q)", okLockRec.Code, okLockRec.Body.String(), lockedSeen)
	}

	// 8) Sandbox rejections: visibility=all, scope=global, mismatched addProjects, mismatched upsertGrants, case-smuggled AddProjects
	for name, tcCase := range map[string]struct {
		tool string
		args string
	}{
		"visibility_all":       {tool: "workspace_list_projects", args: `{"visibility":"all"}`},
		"scope_global":         {tool: "build_save_variable_set", args: `{"name":"g","scope":"global"}`},
		"mismatched_add":       {tool: "build_save_variable_set", args: `{"name":"g","scope":"project","addProjects":["prj_other"]}`},
		"mismatched_grant":     {tool: "build_save_variable_set", args: `{"name":"g","upsertGrants":[{"project":"prj_other"}]}`},
		"case_smuggle_project": {tool: "build_save_variable_set", args: `{"name":"g","scope":"project","addProjects":["prj_bound"],"AddProjects":["prj_other"]}`},
	} {
		t.Run(name, func(t *testing.T) {
			r := callGateway("/internal/v1/ai/tools/execute", host.callbackToken, "tc_sbx_"+name, tcCase.tool, tcCase.args, "")
			if r.Code != http.StatusForbidden || !strings.Contains(r.Body.String(), "ai.project_scope_mismatch") {
				t.Fatalf("case %s expected 403 ai.project_scope_mismatch, got %d %s", name, r.Code, r.Body.String())
			}
		})
	}
}

type stubActorCtxKey struct{}

type stubAIToolHost struct {
	db            *gorm.DB
	reg           *aitool.Registry
	callbackToken string
	previousToken string
	audits        []string
}

func (h *stubAIToolHost) DBFor(*gin.Context) *gorm.DB                 { return h.db }
func (h *stubAIToolHost) CurrentUser(*gin.Context) (model.User, bool) { return model.User{}, false }
func (h *stubAIToolHost) CurrentSessionFromCookie(*gin.Context) (model.UserSession, bool) {
	return model.UserSession{}, false
}
func (h *stubAIToolHost) SetCurrentUser(ctx *gin.Context, user model.User) {
	ctx.Set("luna.devops.current_user", user)
}
func (h *stubAIToolHost) AuditWithContext(_ string, _ string, resource string, _ bool, _ string, _ context.Context) {
	h.audits = append(h.audits, resource)
}
func (h *stubAIToolHost) ConfigAvailable() bool { return true }
func (h *stubAIToolHost) ConfigValues([]string) map[string]string {
	return map[string]string{
		AssistantEnabledConfigKey: "true",
		AccessModeConfigKey:       "all_authenticated",
	}
}
func (h *stubAIToolHost) AllConfigValues() map[string]string           { return h.ConfigValues(nil) }
func (h *stubAIToolHost) ConfigDefinition(string) *ConfigDefinition    { return nil }
func (h *stubAIToolHost) ResolveSecret(context.Context, string) string { return "" }
func (h *stubAIToolHost) AIAgent() aiagent.Client                      { return nil }
func (h *stubAIToolHost) AIDeploymentEnabled() bool                    { return true }
func (h *stubAIToolHost) AIActorOverride(*gin.Context) (aiagent.ActorContext, string, bool, bool) {
	return aiagent.ActorContext{}, "", false, false
}
func (h *stubAIToolHost) FindProjectForCurrentUserByID(*gin.Context, string) (model.Project, bool) {
	return model.Project{}, false
}
func (h *stubAIToolHost) AIAgentCallbackServiceToken() string         { return h.callbackToken }
func (h *stubAIToolHost) AIAgentPreviousCallbackServiceToken() string { return h.previousToken }
func (h *stubAIToolHost) AIPlatformActor(ctx context.Context) (PlatformActor, bool) {
	actor, ok := ctx.Value(stubActorCtxKey{}).(PlatformActor)
	return actor, ok
}
func (h *stubAIToolHost) WithAIPlatformActor(ctx context.Context, actor PlatformActor) context.Context {
	return context.WithValue(ctx, stubActorCtxKey{}, actor)
}
func (h *stubAIToolHost) AIToolService() *aitool.Service   { return nil }
func (h *stubAIToolHost) AIToolRegistry() *aitool.Registry { return h.reg }
func (h *stubAIToolHost) AuthenticateMCPBearerToken(*gin.Context) (model.User, model.AccessToken, bool) {
	return model.User{}, model.AccessToken{}, false
}
func (h *stubAIToolHost) ExternalBaseURL(*gin.Context) string { return "https://devops.example.com" }
func (h *stubAIToolHost) MCPPreviewSigningSecret() []byte {
	return []byte("test-mcp-secret-32-bytes-minimum")
}
func (h *stubAIToolHost) AppVersion() string { return "0.1.0-test" }
func (h *stubAIToolHost) AuthorizeProject(*gin.Context, authz.Action) (model.User, model.Project, bool) {
	return model.User{}, model.Project{}, false
}
func (h *stubAIToolHost) AuthorizeProjectByID(*gin.Context, string, authz.Action) (model.User, model.Project, bool) {
	return model.User{}, model.Project{}, false
}
func (h *stubAIToolHost) AuthorizeProjectRef(*gin.Context, string, authz.Action) (model.User, model.Project, bool) {
	return model.User{}, model.Project{}, false
}
func (h *stubAIToolHost) MonitorProjectAuthorization(*gin.Context, context.Context, model.User, string, authz.Action, func()) (<-chan struct{}, bool) {
	return nil, false
}
func (h *stubAIToolHost) WriteContinuousAuthorizationRevoked(*gin.Context) {}

type stubAIToolDBState struct {
	mu                 sync.Mutex
	userRole           string
	projectID          string
	projectSlug        string
	operationID        string
	toolStatus         string
	inputMode          string
	argumentsHash      string
	approvalDecision   string
	rememberedApproval bool
}

type stubSQLConnector struct{ state *stubAIToolDBState }
type stubSQLDriver struct{}
type stubSQLConn struct{ state *stubAIToolDBState }
type stubSQLRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (c stubSQLConnector) Connect(context.Context) (driver.Conn, error) { return stubSQLConn(c), nil }
func (c stubSQLConnector) Driver() driver.Driver                        { return stubSQLDriver{} }
func (stubSQLDriver) Open(string) (driver.Conn, error)                  { return nil, driver.ErrSkip }
func (stubSQLConn) Prepare(string) (driver.Stmt, error)                 { return nil, driver.ErrSkip }
func (stubSQLConn) Close() error                                        { return nil }
func (stubSQLConn) Begin() (driver.Tx, error)                           { return nil, driver.ErrSkip }

func (c stubSQLConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	lower := strings.ToLower(query)
	switch {
	case strings.Contains(lower, "from ai.runs"):
		inputMode := c.state.inputMode
		if inputMode == "" {
			inputMode = "model"
		}
		toolStatus := c.state.toolStatus
		if toolStatus == "" {
			toolStatus = "running"
		}
		return &stubSQLRows{
			cols: []string{
				"owner_user_id", "actor_session_id", "conversation_id", "run_status",
				"conversation_owner_id", "conversation_project_id", "operation_id",
				"tool_status", "input_mode", "arguments_hash", "approval_decision", "remembered_approval",
			},
			rows: [][]driver.Value{{
				"usr_1", "ses_1", "aicnv_1", "running",
				"usr_1", c.state.projectID, c.state.operationID,
				toolStatus, inputMode, c.state.argumentsHash, c.state.approvalDecision, c.state.rememberedApproval,
			}},
		}, nil
	case strings.Contains(lower, `from "users"`):
		return &stubSQLRows{
			cols: []string{"id", "role", "disabled"},
			rows: [][]driver.Value{{"usr_1", c.state.userRole, false}},
		}, nil
	case strings.Contains(lower, `from "user_sessions"`):
		return &stubSQLRows{
			cols: []string{"id", "user_id", "expires_at"},
			rows: [][]driver.Value{{"ses_1", "usr_1", time.Now().Add(time.Hour)}},
		}, nil
	case strings.Contains(lower, `from "projects"`):
		if c.state.projectID == "" {
			return &stubSQLRows{cols: []string{"id"}, rows: nil}, nil
		}
		return &stubSQLRows{
			cols: []string{"id", "identifier"},
			rows: [][]driver.Value{{c.state.projectID, c.state.projectSlug}},
		}, nil
	default:
		return &stubSQLRows{cols: []string{"id"}, rows: nil}, nil
	}
}

func (r *stubSQLRows) Columns() []string { return r.cols }
func (r *stubSQLRows) Close() error      { return nil }
func (r *stubSQLRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func newStubGormDB(t *testing.T, state *stubAIToolDBState) *gorm.DB {
	t.Helper()
	sqlDB := sql.OpenDB(stubSQLConnector{state: state})
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(gormpostgres.New(gormpostgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open stub gorm db: %v", err)
	}
	return db
}
