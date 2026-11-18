package aiapi

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

func TestAIRequestMatchesConversationProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	queryOperation := aitool.OpenAPIOperation{
		Parameters:  []aitool.OpenAPIParameter{{InputName: "projectId", WireName: "projectId", In: "query"}},
		InputSchema: map[string]any{"properties": map[string]any{"projectId": map[string]any{"type": "string"}}},
	}
	for name, target := range map[string]string{
		"matching":   "/api/v1/resources?projectId=prj_bound",
		"mismatched": "/api/v1/resources?projectId=prj_other",
		"missing":    "/api/v1/resources",
	} {
		t.Run(name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", target, nil)
			got := aiRequestMatchesConversationProject(ctx, queryOperation, "prj_bound")
			if got != (name == "matching") {
				t.Fatalf("match = %v", got)
			}
		})
	}

	bodyOperation := aitool.OpenAPIOperation{
		RequestBody: true,
		InputSchema: map[string]any{
			"properties": map[string]any{
				"body": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"projectId": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/resources", strings.NewReader(`{"projectId":"prj_bound","env":{"FIREBASE_CONFIG":{"projectId":"my-ext-proj"}}}`))
	if !aiRequestMatchesConversationProject(ctx, bodyOperation, "prj_bound") {
		t.Fatal("matching top-level body project was rejected due to nested user dictionary")
	}
	restored, err := io.ReadAll(ctx.Request.Body)
	if err != nil || !strings.Contains(string(restored), "my-ext-proj") {
		t.Fatalf("request body was not restored: %q, %v", restored, err)
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/resources", strings.NewReader(`{"projectId":"prj_other"}`))
	if aiRequestMatchesConversationProject(ctx, bodyOperation, "prj_bound") {
		t.Fatal("mismatched top-level body project was accepted")
	}

	pathOperation := aitool.OpenAPIOperation{
		RequestBody: true,
		Parameters:  []aitool.OpenAPIParameter{{InputName: "projectId", WireName: "projectId", In: "path", Required: true}},
		InputSchema: map[string]any{
			"properties": map[string]any{
				"projectId": map[string]any{"type": "string"},
				"body": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"env": map[string]any{"type": "object"},
					},
				},
			},
		},
	}
	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_bound"}}
	ctx.Request = httptest.NewRequest("PUT", "/api/v1/projects/prj_bound/env", strings.NewReader(`{"env":{"projectId":"other"}}`))
	if !aiRequestMatchesConversationProject(ctx, pathOperation, "prj_bound") {
		t.Fatal("path-scoped request with nested user dictionary projectId was falsely rejected")
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/resources", strings.NewReader(`{"projectId":"prj_other"}`))
	if !aiRequestMatchesConversationProject(ctx, bodyOperation, "") {
		t.Fatal("unbound conversation should defer project authorization to the business handler")
	}

	nestedSchemaOperation := aitool.OpenAPIOperation{
		RequestBody: true,
		InputSchema: map[string]any{
			"properties": map[string]any{
				"body": map[string]any{
					"type": "object",
					"allOf": []any{
						map[string]any{
							"type": "object",
							"properties": map[string]any{
								"target": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"projectId": map[string]any{"type": "string"},
									},
								},
								"batches": map[string]any{
									"type": "array",
									"items": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"projectIds": map[string]any{
												"type":  "array",
												"items": map[string]any{"type": "string"},
											},
										},
									},
								},
								"labels": map[string]any{
									"type":                 "object",
									"additionalProperties": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, payload := range map[string]struct {
		body string
		want bool
	}{
		"nested_matching_with_freeform_map": {
			body: `{"target":{"projectId":"prj_bound"},"batches":[{"projectIds":["prj_bound"]}],"labels":{"projectId":"unrelated-label"}}`,
			want: true,
		},
		"nested_mismatched_target": {
			body: `{"target":{"projectId":"prj_other"},"batches":[{"projectIds":["prj_bound"]}]}`,
			want: false,
		},
		"empty_project_ids_array_rejected": {
			body: `{"target":{"projectId":"prj_bound"},"batches":[{"projectIds":[]}]}`,
			want: false,
		},
		"empty_string_project_id_rejected": {
			body: `{"target":{"projectId":""},"batches":[{"projectIds":["prj_bound"]}]}`,
			want: false,
		},
		"non_string_project_ids_element_rejected": {
			body: `{"target":{"projectId":"prj_bound"},"batches":[{"projectIds":[123]}]}`,
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			testCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
			testCtx.Request = httptest.NewRequest("POST", "/api/v1/batch", strings.NewReader(payload.body))
			if got := aiRequestMatchesConversationProject(testCtx, nestedSchemaOperation, "prj_bound"); got != payload.want {
				t.Fatalf("aiRequestMatchesConversationProject(%s) = %v, want %v", name, got, payload.want)
			}
		})
	}
}

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
	operations, err := aitool.PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range operations {
		if !aiOperationScopesAllowed(authz.PlatformRoleAdmin, op.RequiredScopes) {
			t.Fatalf("platform catalog operation %q has invalid requiredScopes %v", op.OperationID, op.RequiredScopes)
		}
	}
}

func TestAIToolExecutionIdentityMiddlewareHardening(t *testing.T) {
	gin.SetMode(gin.TestMode)
	state := &stubAIToolDBState{
		userRole:    authz.PlatformRoleUser,
		projectID:   "prj_bound",
		operationID: "getProject",
	}
	db := newStubGormDB(t, state)
	host := &stubAIToolHost{
		db:            db,
		callbackToken: "primary-callback-token-32-bytes!!",
		previousToken: "previous-callback-token-32-bytes!",
	}
	handler := New(host)

	router := gin.New()
	router.Use(handler.AIToolExecutionIdentityMiddleware())
	router.GET("/api/v1/data-retention/catalog", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"ok": true}) })
	router.POST("/api/v1/ai-tools/fetch-web-page", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"ok": true}) })
	router.GET("/api/v1/projects/:projectId", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"ok": true}) })
	router.POST("/api/v1/build/variable-sets", func(ctx *gin.Context) { ctx.JSON(http.StatusCreated, gin.H{"ok": true}) })
	router.POST("/api/v1/projects/:projectId/notifications/rules", func(ctx *gin.Context) { ctx.JSON(http.StatusCreated, gin.H{"ok": true}) })
	router.POST("/api/v1/notification-channels", func(ctx *gin.Context) { ctx.JSON(http.StatusCreated, gin.H{"ok": true}) })
	router.GET("/api/v1/build/variable-sets", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"ok": true}) })
	router.POST("/api/v1/projects/:projectId/app-templates/:templateId/install", func(ctx *gin.Context) { ctx.JSON(http.StatusCreated, gin.H{"ok": true}) })

	call := func(method, target, body, token, toolCallID, operationID string) *httptest.ResponseRecorder {
		t.Helper()
		state.mu.Lock()
		state.operationID = operationID
		state.mu.Unlock()
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, target, reader)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(RunIDHeader, "airun_1")
		req.Header.Set(ToolCallIDHeader, toolCallID)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1) Standard user invoking admin-scoped operation -> 403 ai.tool_permission_denied
	rec := call(http.MethodGet, "/api/v1/data-retention/catalog", "", host.callbackToken, "aitool_admin", "listDataRetentionCatalog")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.tool_permission_denied") {
		t.Fatalf("expected 403 ai.tool_permission_denied, got %d %s", rec.Code, rec.Body.String())
	}

	// 2) Standard user invoking web:read operation -> 200 OK and writes 1 AuditLog (POST)
	rec = call(http.MethodPost, "/api/v1/ai-tools/fetch-web-page", `{"url":"https://example.com"}`, host.callbackToken, "aitool_web", "fetchWebPage")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for web:read tool, got %d %s", rec.Code, rec.Body.String())
	}
	if len(host.audits) != 1 || host.audits[0] != "fetchWebPage:aitool_web" {
		t.Fatalf("expected 1 AuditLog after POST tool, got %#v", host.audits)
	}

	// 3) Read-only GET operation (using rotated previous callback token) -> 200 OK and writes ZERO AuditLogs
	rec = call(http.MethodGet, "/api/v1/projects/prj_bound", "", host.previousToken, "aitool_get", "getProject")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET getProject with previous callback token, got %d %s", rec.Code, rec.Body.String())
	}
	if len(host.audits) != 1 {
		t.Fatalf("GET tool must not write AuditLog, got %#v", host.audits)
	}

	// 4) L1 write without approval -> 428 ai.approval_required; forged remembered without DB grant -> 428; valid remembered -> 201
	rec = call(http.MethodPost, "/api/v1/projects/prj_bound/notifications/rules", `{"name":"Crash Rule","eventTypes":["release.failed"],"channelIds":["nch_1"],"filter":{"severities":["error"]}}`, host.callbackToken, "aitool_unapproved_l1", "createProjectNotificationRule")
	if rec.Code != http.StatusPreconditionRequired || !strings.Contains(rec.Body.String(), "ai.approval_required") {
		t.Fatalf("expected 428 ai.approval_required for unapproved L1 write, got %d %s", rec.Code, rec.Body.String())
	}
	state.mu.Lock()
	state.approvalDecision = "remembered"
	state.rememberedApproval = false
	state.mu.Unlock()
	rec = call(http.MethodPost, "/api/v1/projects/prj_bound/notifications/rules", `{"name":"Crash Rule","eventTypes":["release.failed"],"channelIds":["nch_1"],"filter":{"severities":["error"]}}`, host.callbackToken, "aitool_forged_remembered", "createProjectNotificationRule")
	if rec.Code != http.StatusPreconditionRequired || !strings.Contains(rec.Body.String(), "ai.approval_required") {
		t.Fatalf("expected 428 when approval_decision=remembered lacks DB grant, got %d %s", rec.Code, rec.Body.String())
	}
	state.mu.Lock()
	state.rememberedApproval = true
	state.mu.Unlock()

	// 4b) L1 installAppTemplate with stage=dev accepts remembered approval, while stage=prod escalates to L2 and rejects remembered approval
	rec = call(http.MethodPost, "/api/v1/projects/prj_bound/app-templates/redis/install", `{"applicationName":"Redis","applicationIdentifier":"redis","deploymentName":"Redis dev","stage":"dev","clusterId":"clu_1"}`, host.callbackToken, "aitool_install_dev", "installAppTemplate")
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for dev installAppTemplate with remembered approval, got %d %s", rec.Code, rec.Body.String())
	}
	if len(host.audits) != 2 {
		t.Fatalf("expected 2 AuditLogs after approved POST tool, got %#v", host.audits)
	}
	rec = call(http.MethodPost, "/api/v1/projects/prj_bound/app-templates/redis/install", `{"applicationName":"Redis","applicationIdentifier":"redis","deploymentName":"Redis prod","stage":"prod","clusterId":"clu_1"}`, host.callbackToken, "aitool_install_prod_remembered", "installAppTemplate")
	if rec.Code != http.StatusPreconditionRequired || !strings.Contains(rec.Body.String(), "ai.approval_required") {
		t.Fatalf("expected 428 ai.approval_required when stage=prod escalates L1 to L2, got %d %s", rec.Code, rec.Body.String())
	}

	// 4c) L2 createBuildVariableSet / createScopedNotificationChannel rejects remembered and requires explicit approve
	rec = call(http.MethodPost, "/api/v1/build/variable-sets", `{"name":"vars","scope":"project","projectIds":["prj_bound"],"variables":{"projectId":"other"}}`, host.callbackToken, "aitool_l2_remembered", "createBuildVariableSet")
	if rec.Code != http.StatusPreconditionRequired || !strings.Contains(rec.Body.String(), "ai.approval_required") {
		t.Fatalf("expected 428 ai.approval_required for L2 with remembered decision, got %d %s", rec.Code, rec.Body.String())
	}
	state.mu.Lock()
	state.approvalDecision = "approve"
	state.mu.Unlock()
	rec = call(http.MethodPost, "/api/v1/build/variable-sets", `{"name":"vars","scope":"project","projectIds":["prj_bound"],"variables":{"projectId":"other"}}`, host.callbackToken, "aitool_nested", "createBuildVariableSet")
	if rec.Code != http.StatusCreated {
		t.Fatalf("nested projectId falsely rejected: %d %s", rec.Code, rec.Body.String())
	}

	// 5) Request JSON with mismatched top-level body.projectIds or omitted projectIds -> 403 ai.project_scope_mismatch
	rec = call(http.MethodPost, "/api/v1/build/variable-sets", `{"name":"vars","scope":"project","projectIds":["prj_other"]}`, host.callbackToken, "aitool_mismatch", "createBuildVariableSet")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch, got %d %s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/v1/build/variable-sets", `{"name":"vars","scope":"global","projectId":"prj_bound"}`, host.callbackToken, "aitool_global_bypass", "createBuildVariableSet")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch when omitting projectIds, got %d %s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/v1/build/variable-sets", `{"name":"vars","scope":"user","projectIds":["prj_bound"]}`, host.callbackToken, "aitool_user_scope_bypass", "createBuildVariableSet")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch when scope=user carries forged projectIds, got %d %s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/v1/build/variable-sets", `{"name":"vars","scope":"project","projectIds":["prj_bound"],"ProjectIds":["prj_other"]}`, host.callbackToken, "aitool_case_smuggle", "createBuildVariableSet")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch on case-variant ProjectIds smuggling, got %d %s", rec.Code, rec.Body.String())
	}

	// 6) Project-bound notification rule omitting optional single-enum filter.scope ("projects") -> 201 Created; mismatched filter.projectIds -> 403
	rec = call(http.MethodPost, "/api/v1/projects/prj_bound/notifications/rules", `{"name":"Crash Rule","eventTypes":["release.failed"],"channelIds":["nch_1"],"filter":{"severities":["error"]}}`, host.callbackToken, "aitool_proj_rule_ok", "createProjectNotificationRule")
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for createProjectNotificationRule with omitted filter.scope, got %d %s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/v1/projects/prj_bound/notifications/rules", `{"name":"Crash Rule","eventTypes":["release.failed"],"channelIds":["nch_1"],"filter":{"projectIds":["prj_other"]}}`, host.callbackToken, "aitool_proj_rule_mismatch", "createProjectNotificationRule")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch on mismatched filter.projectIds, got %d %s", rec.Code, rec.Body.String())
	}

	// 7) Scoped notification channel enforces scope=project + matching projectIds; global channel endpoints require approval and exclude fake project fields
	state.mu.Lock()
	state.approvalDecision = "approve"
	state.mu.Unlock()
	rec = call(http.MethodPost, "/api/v1/notification-channels", `{"name":"Proj Hook","scope":"project","projectIds":["prj_bound"],"adapterKind":"webhook"}`, host.callbackToken, "aitool_scoped_ch_ok", "createScopedNotificationChannel")
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for createScopedNotificationChannel with matching project scope, got %d %s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/v1/notification-channels", `{"name":"Global Hook","scope":"global","projectIds":["prj_bound"],"adapterKind":"webhook"}`, host.callbackToken, "aitool_scoped_ch_global", "createScopedNotificationChannel")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch for scope=global in project conversation, got %d %s", rec.Code, rec.Body.String())
	}
	rec = call(http.MethodPost, "/api/v1/notification-channels", `{"name":"Missing Scope Hook","projectIds":["prj_bound"],"adapterKind":"webhook"}`, host.callbackToken, "aitool_scoped_ch_missing_scope", "createScopedNotificationChannel")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch when multi-scope channel omits scope, got %d %s", rec.Code, rec.Body.String())
	}

	// 8) Authoritative verification GET readbacks without optional query projectId (e.g. after 204 DELETE) are allowed, while model calls without projectId stay rejected
	rec = call(http.MethodGet, "/api/v1/build/variable-sets?page=1&pageSize=100", "", host.callbackToken, "aitool_list_model", "listBuildVariableSets")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 for model GET listBuildVariableSets omitting projectId, got %d %s", rec.Code, rec.Body.String())
	}
	state.mu.Lock()
	state.inputMode = "verification"
	state.mu.Unlock()
	rec = call(http.MethodGet, "/api/v1/build/variable-sets?page=1&pageSize=100", "", host.callbackToken, "aitool_list_verify", "listBuildVariableSets")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for verification GET listBuildVariableSets readback, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestOpenAPIPathToGin(t *testing.T) {
	if got := openAPIPathToGin("/api/v1/projects/{projectId}/volumes/{volumeId}"); got != "/api/v1/projects/:projectId/volumes/:volumeId" {
		t.Fatalf("path = %q", got)
	}
}

type stubActorCtxKey struct{}

type stubAIToolHost struct {
	db            *gorm.DB
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
func (h *stubAIToolHost) AIToolService() *aitool.Service { return nil }
func (h *stubAIToolHost) AuthorizeProject(*gin.Context, authz.Action) (model.User, model.Project, bool) {
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
	operationID        string
	inputMode          string
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
		return &stubSQLRows{
			cols: []string{
				"owner_user_id", "actor_session_id", "conversation_id", "run_status",
				"conversation_owner_id", "conversation_project_id", "operation_id",
				"tool_status", "input_mode", "approval_decision", "remembered_approval",
			},
			rows: [][]driver.Value{{
				"usr_1", "ses_1", "aicnv_1", "running",
				"usr_1", c.state.projectID, c.state.operationID,
				"running", inputMode, c.state.approvalDecision, c.state.rememberedApproval,
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
