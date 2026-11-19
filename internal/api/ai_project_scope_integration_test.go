package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAIProjectListVisibilityDirectExecutionPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-project-scope-integration-secret-0001")
	db := aiProjectScopeIntegrationDB(t)

	now := time.Now().UTC()
	admin := model.User{ID: "usr_ai_scope_admin", Email: "ai-scope-admin@example.test", Name: "AI Scope Admin", Role: authz.PlatformRoleAdmin}
	session := model.UserSession{
		ID: "ses_ai_scope_admin", UserID: admin.ID, TokenHash: "ai-project-scope-session-token", ExpiresAt: now.Add(time.Hour),
	}
	related := model.Project{
		ID: "prj_ai_scope_related", Identifier: "ai-scope-related", Name: "Related", DeleteStatus: "active",
	}
	unrelated := model.Project{
		ID: "prj_ai_scope_unrelated", Identifier: "ai-scope-unrelated", Name: "Unrelated", DeleteStatus: "active",
	}
	member := model.ProjectMember{ID: "prjm_ai_scope_admin", ProjectID: related.ID, UserID: admin.ID, Role: authz.ProjectRoleOwner}
	for _, value := range []any{&admin, &session, &related, &unrelated, &member} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	router := NewRouter(db, mustTestConfig(t))
	defaultItems := executeAIProjectList(t, db, router, admin, session, map[string]any{"page": 1, "pageSize": 20}, "")
	if !projectItemsContain(defaultItems, related.ID) || projectItemsContain(defaultItems, unrelated.ID) {
		t.Fatalf("default visibility items = %#v", defaultItems)
	}
	relatedItems := executeAIProjectList(t, db, router, admin, session, map[string]any{"visibility": "related", "page": 1, "pageSize": 20}, "")
	if !projectItemsContain(relatedItems, related.ID) || projectItemsContain(relatedItems, unrelated.ID) {
		t.Fatalf("related visibility items = %#v", relatedItems)
	}
	allItems := executeAIProjectList(t, db, router, admin, session, map[string]any{"visibility": "all", "page": 1, "pageSize": 20}, "")
	if !projectItemsContain(allItems, related.ID) || !projectItemsContain(allItems, unrelated.ID) {
		t.Fatalf("all visibility items = %#v", allItems)
	}

	// Single-project conversation restricts workspace_list_projects to the bound project,
	// and rejects visibility=all with 403 ai.project_scope_mismatch.
	boundItems := executeAIProjectList(t, db, router, admin, session, map[string]any{"page": 1, "pageSize": 20}, related.ID)
	if !projectItemsContain(boundItems, related.ID) || projectItemsContain(boundItems, unrelated.ID) {
		t.Fatalf("project-bound visibility items = %#v", boundItems)
	}
	rec := executeFacadeGatewayRaw(t, db, router, admin, session, related.ID, "bound_all", "workspace_list_projects", map[string]any{"visibility": "all"}, "", "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch for visibility=all in project conversation, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestAIProjectVolumePreviewExecuteAndReadbackPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-project-volume-integration-secret-01")
	db := aiProjectScopeIntegrationDB(t)

	now := time.Now().UTC()
	user := model.User{ID: "usr_ai_volume_owner", Email: "ai-volume-owner@example.test", Name: "AI Volume Owner", Role: authz.PlatformRoleAdmin}
	session := model.UserSession{ID: "ses_ai_volume_owner", UserID: user.ID, TokenHash: "ai-volume-session-token", ExpiresAt: now.Add(time.Hour)}
	project := model.Project{
		ID: "prj_ai_volume", Identifier: "ai-volume", Name: "AI Volume",
		KubernetesNamespace: "luna-ai-volume", BillingOwnerUserID: user.ID, DeleteStatus: "active",
	}
	member := model.ProjectMember{ID: "prjm_ai_volume_owner", ProjectID: project.ID, UserID: user.ID, Role: authz.ProjectRoleOwner}
	cluster := model.RuntimeCluster{ID: "rcl_ai_volume", Name: "AI Volume Cluster", Scope: "global", CreatedBy: user.ID}
	wallet := model.UserWallet{ID: "wlt_ai_volume_owner", UserID: user.ID, BalanceCredits: decimal.NewFromInt(100)}
	for _, value := range []any{&user, &session, &project, &member, &cluster, &wallet} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	handlers := NewHandlers(db, mustTestConfig(t))
	tasks := &volumeTaskEnqueuerStub{}
	handlers.volumes = volume.NewGormService(db, volumeOperationDispatcher{tasks: tasks})
	handlers.volumeClusters = nil
	handlers.aiToolRegistry = aitool.NewRegistry()
	handlers.domains = newDomainHandlers(handlers)

	router := gin.New()
	router.POST("/internal/v1/ai/tools/preview", handlers.domains.ai.PreviewAIToolInternal)
	router.POST("/internal/v1/ai/tools/execute", handlers.domains.ai.ExecuteAIToolInternal)

	keys := mustTestAIKeys(t)
	runID := "airun_ai_volume"
	conversationID := "aicnv_ai_volume"
	turnID := "aitrn_ai_volume"
	createToolCallID := "aitool_ai_volume_create"
	if err := db.Exec(`INSERT INTO ai.conversations (id, owner_user_id, project_id, title, status) VALUES (?, ?, ?, 'volume test', 'active')`, conversationID, user.ID, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES (?, ?, 1, 'running', 'create volume', ?)`, turnID, conversationID, runID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest, actor_session_id) VALUES (?, ?, ?, ?, 1, 'running', 'system-v4', 'test', ?)`, runID, user.ID, conversationID, turnID, session.ID).Error; err != nil {
		t.Fatal(err)
	}
	createArgs := `{"project":"ai-volume","displayName":"agent-smoke","cluster":"AI Volume Cluster","sourceType":"blank","capacity":"1Gi","storageClassName":"standard","accessMode":"ReadWriteOnce","volumeMode":"Filesystem"}`
	argsHash, err := aitool.CanonicalJSONHash([]byte(createArgs))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, arguments_hash, input_mode) VALUES (?, ?, 'runtime_create_volume', 'proposed', ?::jsonb, ?, 'model')`, createToolCallID, runID, createArgs, argsHash).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(`DELETE FROM ai.conversations WHERE id = ?`, conversationID).Error })

	previewBody := `{"toolName":"runtime_create_volume","arguments":` + createArgs + `}`
	previewReq := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/preview", bytes.NewBufferString(previewBody))
	previewReq.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	previewReq.Header.Set("Content-Type", "application/json")
	previewReq.Header.Set(aiapi.RunIDHeader, runID)
	previewReq.Header.Set(aiapi.ToolCallIDHeader, createToolCallID)
	previewRec := httptest.NewRecorder()
	router.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview volume = %d %s", previewRec.Code, previewRec.Body.String())
	}
	var previewRes struct {
		PreviewLock string `json:"previewLock"`
	}
	if err := json.Unmarshal(previewRec.Body.Bytes(), &previewRes); err != nil || previewRes.PreviewLock == "" {
		t.Fatalf("decode preview lock: %v %s", err, previewRec.Body.String())
	}

	if err := db.Exec(`UPDATE ai.tool_calls SET status = 'running', approval_decision = 'approve', preview_lock = ? WHERE id = ?`, previewRes.PreviewLock, createToolCallID).Error; err != nil {
		t.Fatal(err)
	}

	createRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", bytes.NewBufferString(previewBody))
	createRequest.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Idempotency-Key", createToolCallID)
	createRequest.Header.Set(aiapi.RunIDHeader, runID)
	createRequest.Header.Set(aiapi.ToolCallIDHeader, createToolCallID)
	createRequest.Header.Set("X-Luna-AI-Preview-Lock", previewRes.PreviewLock)
	createRecorder := httptest.NewRecorder()
	router.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusOK {
		t.Fatalf("create volume = %d %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	createdID, _ := created["id"].(string)
	if createdID == "" || created["projectId"] != project.ID || tasks.provision.VolumeID != createdID || tasks.provision.ProjectID != project.ID {
		t.Fatalf("created volume = %#v, task = %#v", created, tasks.provision)
	}

	getToolCallID := "aitool_ai_volume_get"
	getArgs := `{"project":"` + project.ID + `","volume":"agent-smoke"}`
	getHash, _ := aitool.CanonicalJSONHash([]byte(getArgs))
	if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, arguments_hash, input_mode) VALUES (?, ?, 'runtime_get_volume', 'running', ?::jsonb, ?, 'verification')`, getToolCallID, runID, getArgs, getHash).Error; err != nil {
		t.Fatal(err)
	}
	getRequest := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", bytes.NewBufferString(`{"toolName":"runtime_get_volume","arguments":`+getArgs+`}`))
	getRequest.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	getRequest.Header.Set("Content-Type", "application/json")
	getRequest.Header.Set(aiapi.RunIDHeader, runID)
	getRequest.Header.Set(aiapi.ToolCallIDHeader, getToolCallID)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("read volume = %d %s", getRecorder.Code, getRecorder.Body.String())
	}
	var readback map[string]any
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &readback); err != nil {
		t.Fatal(err)
	}
	obs, _ := readback["observation"].(map[string]any)
	if readback["id"] != createdID || readback["projectId"] != project.ID || readback["displayName"] != "agent-smoke" || obs["status"] != "unavailable" {
		t.Fatalf("authoritative readback = %#v", readback)
	}
	if getRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("readback cache policy = %q", getRecorder.Header().Get("Cache-Control"))
	}
}

func TestAIFetchWebPageDirectExecutionPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-web-tool-integration-secret-0001")
	db := aiProjectScopeIntegrationDB(t)

	now := time.Now().UTC()
	user := model.User{ID: "usr_ai_web_owner", Email: "ai-web-owner@example.test", Name: "AI Web Owner", Role: authz.PlatformRoleAdmin}
	session := model.UserSession{ID: "ses_ai_web_owner", UserID: user.ID, TokenHash: "ai-web-session-token", ExpiresAt: now.Add(time.Hour)}
	for _, value := range []any{&user, &session} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<html><head><title>Luna Tool Test</title></head><body><main>authoritative public page</main></body></html>`))
	}))
	defer target.Close()

	handlers := NewHandlers(db, mustTestConfig(t))
	handlers.aiTools = aitool.NewService(aitool.WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
		return security.AdminEgressPolicy(), nil
	}))
	handlers.aiToolRegistry = aitool.NewRegistry()
	handlers.domains = newDomainHandlers(handlers)

	router := gin.New()
	router.POST("/internal/v1/ai/tools/execute", handlers.domains.ai.ExecuteAIToolInternal)

	keys := mustTestAIKeys(t)
	conversationID := "aicnv_ai_web"
	turnID := "aitrn_ai_web"
	runID := "airun_ai_web"
	toolCallID := "aitool_ai_web_fetch"
	webArgs := `{"url":"` + target.URL + `","maxCharacters":2000}`
	webHash, _ := aitool.CanonicalJSONHash([]byte(webArgs))
	if err := db.Exec(`INSERT INTO ai.conversations (id, owner_user_id, title, status) VALUES (?, ?, 'web test', 'active')`, conversationID, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES (?, ?, 1, 'running', 'fetch web page', ?)`, turnID, conversationID, runID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest, actor_session_id) VALUES (?, ?, ?, ?, 1, 'running', 'system-v4', 'test', ?)`, runID, user.ID, conversationID, turnID, session.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, arguments_hash, input_mode) VALUES (?, ?, 'ops_web_fetch', 'running', ?::jsonb, ?, 'model')`, toolCallID, runID, webArgs, webHash).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(`DELETE FROM ai.conversations WHERE id = ?`, conversationID).Error })

	execute := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", bytes.NewBufferString(`{"toolName":"ops_web_fetch","arguments":`+webArgs+`}`))
	execute.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	execute.Header.Set("Content-Type", "application/json")
	execute.Header.Set(aiapi.RunIDHeader, runID)
	execute.Header.Set(aiapi.ToolCallIDHeader, toolCallID)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, execute)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ops_web_fetch = %d %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	dataMap := response
	if nested, ok := response["data"].(map[string]any); ok {
		dataMap = nested
	}
	if dataMap["title"] != "Luna Tool Test" || !strings.Contains(fmt.Sprint(dataMap["content"]), "authoritative public page") {
		t.Fatalf("web page response = %#v", response)
	}

	unbound := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", bytes.NewBufferString(`{"toolName":"ops_web_fetch","arguments":`+webArgs+`}`))
	unbound.Header.Set("Content-Type", "application/json")
	unboundRecorder := httptest.NewRecorder()
	router.ServeHTTP(unboundRecorder, unbound)
	if unboundRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unbound web request = %d %s", unboundRecorder.Code, unboundRecorder.Body.String())
	}
}

func TestAITaskStatusInternalWaitingExternalAndZeroAuditPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-task-status-integration-secret-0001")
	db := aiProjectScopeIntegrationDB(t)
	keys := mustTestAIKeys(t)

	now := time.Now().UTC()
	owner := model.User{ID: "usr_ai_wait_owner", Email: "ai-wait-owner@example.test", Name: "Wait Owner", Role: authz.PlatformRoleUser}
	session := model.UserSession{
		ID: "ses_ai_wait_owner", UserID: owner.ID, TokenHash: "ai-wait-owner-session-token", ExpiresAt: now.Add(time.Hour),
	}
	allowedProject := model.Project{ID: "prj_ai_wait_allowed", Identifier: "ai-wait-allowed", Name: "Allowed", DeleteStatus: "active"}
	forbiddenProject := model.Project{ID: "prj_ai_wait_forbidden", Identifier: "ai-wait-forbidden", Name: "Forbidden", DeleteStatus: "active"}
	membership := model.ProjectMember{ID: "prjm_ai_wait_owner", ProjectID: allowedProject.ID, UserID: owner.ID, Role: authz.ProjectRoleDeveloper}
	appRecord := model.Application{ID: "app_ai_wait", ProjectID: allowedProject.ID, Identifier: "web", Name: "Web", DeleteStatus: "active"}
	buildRun := model.BuildRun{
		ID:            "bldr_ai_wait_1",
		ProjectID:     allowedProject.ID,
		ApplicationID: appRecord.ID,
		Status:        "failed",
		FailureCode:   "build.dockerfile_step_failed",
		FinishedAt:    &now,
		UpdatedAt:     now,
	}
	otherBuild := model.BuildRun{
		ID:        "bldr_ai_wait_other",
		ProjectID: forbiddenProject.ID,
		Status:    "running",
		UpdatedAt: now,
	}
	for _, seed := range []any{&owner, &session, &allowedProject, &forbiddenProject, &membership, &appRecord, &buildRun, &otherBuild} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed %T: %v", seed, err)
		}
	}

	wakeAt := now.Add(5 * time.Second)
	if err := db.Exec(`INSERT INTO ai.conversations (id, owner_user_id, title, status, project_id) VALUES ('aicnv_wait_1', ?, 'wait test', 'active', ?)`, owner.ID, allowedProject.ID).Error; err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES ('aitrn_wait_1', 'aicnv_wait_1', 1, 'waiting_external', 'build and deploy', 'airun_wait_1')`).Error; err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, wake_at, prompt_version, tool_catalog_digest, actor_session_id) VALUES ('airun_wait_1', ?, 'aicnv_wait_1', 'aitrn_wait_1', 1, 'waiting_external', ?, 'system-v4', 'test', ?)`, owner.ID, wakeAt, session.ID).Error; err != nil {
		t.Fatalf("seed waiting_external run: %v", err)
	}

	var auditBefore int64
	if err := db.Model(&model.AuditLog{}).Count(&auditBefore).Error; err != nil {
		t.Fatalf("count audit logs before: %v", err)
	}

	router := NewRouter(db, mustTestConfig(t))

	// 1) Supports both canonical project ID and dual-mode project identifier, with zero AuditLogs
	for _, projRef := range []string{allowedProject.ID, allowedProject.Identifier} {
		req := httptest.NewRequest(http.MethodGet, "/internal/v1/ai/runs/airun_wait_1/tasks/"+projRef+"/build_run/"+buildRun.ID, nil)
		req.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for waiting_external task status (%s), got %d %s", projRef, rec.Code, rec.Body.String())
		}
		if gotCC := rec.Header().Get("Cache-Control"); gotCC != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", gotCC)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode task status response: %v", err)
		}
		if payload["taskStatus"] != "failed" || payload["terminal"] != true || payload["failureCode"] != "build.dockerfile_step_failed" {
			t.Fatalf("unexpected task status payload: %#v", payload)
		}
	}

	// 2) Cross-project poll is immediately blocked by sandbox-first check (403 ai.project_scope_mismatch)
	forbiddenReq := httptest.NewRequest(http.MethodGet, "/internal/v1/ai/runs/airun_wait_1/tasks/"+forbiddenProject.ID+"/build_run/"+otherBuild.ID, nil)
	forbiddenReq.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	forbiddenRec := httptest.NewRecorder()
	router.ServeHTTP(forbiddenRec, forbiddenReq)
	if forbiddenRec.Code != http.StatusForbidden || !strings.Contains(forbiddenRec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch for cross-project task poll, got %d %s", forbiddenRec.Code, forbiddenRec.Body.String())
	}

	var auditAfter int64
	if err := db.Model(&model.AuditLog{}).Count(&auditAfter).Error; err != nil {
		t.Fatalf("count audit logs after: %v", err)
	}
	if auditAfter != auditBefore {
		t.Fatalf("task status polling must not write AuditLog: before=%d after=%d", auditBefore, auditAfter)
	}
}

func executeAIProjectList(t *testing.T, db *gorm.DB, router http.Handler, user model.User, session model.UserSession, arguments map[string]any, conversationProjectID string) []any {
	t.Helper()
	visibility := "default"
	if value, exists := arguments["visibility"]; exists {
		visibility = fmt.Sprint(value)
	}
	suffix := fmt.Sprintf("%s_%s", visibility, strings.TrimPrefix(conversationProjectID, "prj_ai_scope_"))
	rec := executeFacadeGatewayRaw(t, db, router, user, session, conversationProjectID, suffix, "workspace_list_projects", arguments, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace_list_projects = %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	return response.Items
}

func executeFacadeGatewayRaw(
	t *testing.T,
	db *gorm.DB,
	router http.Handler,
	user model.User,
	session model.UserSession,
	conversationProjectID string,
	suffix string,
	toolName string,
	arguments map[string]any,
	approvalDecision string,
	previewLock string,
) *httptest.ResponseRecorder {
	t.Helper()
	keys := mustTestAIKeys(t)
	runID := "airun_gw_" + suffix
	toolCallID := "aitool_gw_" + suffix
	conversationID := "aicnv_gw_" + suffix
	turnID := "aitrn_gw_" + suffix
	var projectID any
	if conversationProjectID != "" {
		projectID = conversationProjectID
	}
	if err := db.Exec(`INSERT INTO ai.conversations (id, owner_user_id, title, status, project_id) VALUES (?, ?, 'gateway test', 'active', ?)`, conversationID, user.ID, projectID).Error; err != nil {
		t.Fatalf("seed AI conversation: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES (?, ?, 1, 'running', 'run tool', ?)`, turnID, conversationID, runID).Error; err != nil {
		t.Fatalf("seed AI turn: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest, actor_session_id) VALUES (?, ?, ?, ?, 1, 'running', 'system-v4', 'test', ?)`, runID, user.ID, conversationID, turnID, session.ID).Error; err != nil {
		t.Fatalf("seed AI run: %v", err)
	}
	argumentJSON, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("encode AI ToolCall arguments: %v", err)
	}
	argsHash, err := aitool.CanonicalJSONHash(argumentJSON)
	if err != nil {
		t.Fatalf("hash AI ToolCall arguments: %v", err)
	}
	var approvalVal any
	if approvalDecision != "" {
		approvalVal = approvalDecision
	}
	if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, arguments_hash, input_mode, approval_decision, preview_lock) VALUES (?, ?, ?, 'running', ?::jsonb, ?, 'model', ?, ?)`, toolCallID, runID, toolName, string(argumentJSON), argsHash, approvalVal, previewLock).Error; err != nil {
		t.Fatalf("seed AI ToolCall: %v", err)
	}
	t.Cleanup(func() { _ = db.Exec(`DELETE FROM ai.conversations WHERE id = ?`, conversationID).Error })

	body, err := json.Marshal(map[string]any{
		"toolName":  toolName,
		"arguments": json.RawMessage(argumentJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(aiapi.RunIDHeader, runID)
	req.Header.Set(aiapi.ToolCallIDHeader, toolCallID)
	if previewLock != "" {
		req.Header.Set("X-Luna-AI-Preview-Lock", previewLock)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func projectItemsContain(items []any, projectID string) bool {
	for _, item := range items {
		value, _ := item.(map[string]any)
		if value["id"] == projectID {
			return true
		}
	}
	return false
}

func TestAIToolExecutionIdentityScopeNestedBodyAndAuditPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-identity-hardening-secret-000001")
	db := aiProjectScopeIntegrationDB(t)
	keys := mustTestAIKeys(t)

	now := time.Now().UTC()
	normalUser := model.User{ID: "usr_ai_norm", Email: "ai-norm@example.test", Name: "Normal User", Role: authz.PlatformRoleUser}
	normalSession := model.UserSession{ID: "ses_ai_norm", UserID: normalUser.ID, TokenHash: "ai-norm-session-token", ExpiresAt: now.Add(time.Hour)}
	boundProject := model.Project{ID: "prj_ai_norm_bound", Identifier: "ai-norm-bound", Name: "Bound", DeleteStatus: "active"}
	otherProject := model.Project{ID: "prj_ai_norm_other", Identifier: "ai-norm-other", Name: "Other", DeleteStatus: "active"}
	member := model.ProjectMember{ID: "prjm_ai_norm", ProjectID: boundProject.ID, UserID: normalUser.ID, Role: authz.ProjectRoleOwner}
	for _, seed := range []any{&normalUser, &normalSession, &boundProject, &otherProject, &member} {
		if err := db.Create(seed).Error; err != nil {
			t.Fatalf("seed %T: %v", seed, err)
		}
	}

	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte("public guide"))
	}))
	defer target.Close()

	handlers := NewHandlers(db, mustTestConfig(t))
	handlers.aiTools = aitool.NewService(aitool.WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
		return security.AdminEgressPolicy(), nil
	}))
	handlers.aiToolRegistry = aitool.NewRegistry()
	handlers.domains = newDomainHandlers(handlers)

	router := gin.New()
	router.POST("/internal/v1/ai/tools/preview", handlers.domains.ai.PreviewAIToolInternal)
	router.POST("/internal/v1/ai/tools/execute", handlers.domains.ai.ExecuteAIToolInternal)

	conversationID := "aicnv_ai_norm"
	turnID := "aitrn_ai_norm"
	runID := "airun_ai_norm"
	if err := db.Exec(`INSERT INTO ai.conversations (id, owner_user_id, title, status, project_id) VALUES (?, ?, 'hardening test', 'active', ?)`, conversationID, normalUser.ID, boundProject.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES (?, ?, 1, 'running', 'test tools', ?)`, turnID, conversationID, runID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest, actor_session_id) VALUES (?, ?, ?, ?, 1, 'running', 'system-v4', 'test', ?)`, runID, normalUser.ID, conversationID, turnID, normalSession.ID).Error; err != nil {
		t.Fatal(err)
	}

	callTool := func(endpoint, id, toolName, argsJSON, status, approval, lock string) *httptest.ResponseRecorder {
		t.Helper()
		hash, err := aitool.CanonicalJSONHash([]byte(argsJSON))
		if err != nil {
			t.Fatalf("hash %s: %v", id, err)
		}
		var approvalValue any
		if approval != "" {
			approvalValue = approval
		}
		if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, arguments_hash, input_mode, approval_decision, preview_lock) VALUES (?, ?, ?, ?, ?::jsonb, ?, 'model', ?, ?) ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, approval_decision = EXCLUDED.approval_decision, preview_lock = EXCLUDED.preview_lock`, id, runID, toolName, status, argsJSON, hash, approvalValue, lock).Error; err != nil {
			t.Fatalf("seed tool_call %s: %v", id, err)
		}
		req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"toolName":"`+toolName+`","arguments":`+argsJSON+`}`))
		req.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(aiapi.RunIDHeader, runID)
		req.Header.Set(aiapi.ToolCallIDHeader, id)
		if lock != "" {
			req.Header.Set("X-Luna-AI-Preview-Lock", lock)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1) Normal user calling admin-only tool -> 403 ai.tool_permission_denied
	adminRec := callTool("/internal/v1/ai/tools/execute", "aitool_admin_scope", "admin_preview_data_retention", `{}`, "running", "", "")
	if adminRec.Code != http.StatusForbidden || !strings.Contains(adminRec.Body.String(), "ai.tool_permission_denied") {
		t.Fatalf("expected 403 ai.tool_permission_denied, got %d %s", adminRec.Code, adminRec.Body.String())
	}

	// 2) Normal user calling web:read tool -> 200 OK AND writes AuditLog (effect=execute)
	var auditBefore int64
	if err := db.Model(&model.AuditLog{}).Where("action = ?", "ai.tool.execute").Count(&auditBefore).Error; err != nil {
		t.Fatal(err)
	}
	webRec := callTool("/internal/v1/ai/tools/execute", "aitool_web_read", "ops_web_fetch", `{"url":"`+target.URL+`"}`, "running", "", "")
	if webRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for ops_web_fetch, got %d %s", webRec.Code, webRec.Body.String())
	}
	var auditAfterWeb int64
	if err := db.Model(&model.AuditLog{}).Where("action = ?", "ai.tool.execute").Count(&auditAfterWeb).Error; err != nil {
		t.Fatal(err)
	}
	if auditAfterWeb != auditBefore+1 {
		t.Fatalf("ops_web_fetch must write AuditLog: before=%d after=%d", auditBefore, auditAfterWeb)
	}

	// 3) Read-only L0 tool call -> 200 OK AND must NOT write AuditLog
	getRec := callTool("/internal/v1/ai/tools/execute", "aitool_get_proj", "workspace_get_project", `{"project":"ai-norm-bound"}`, "running", "", "")
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for workspace_get_project, got %d %s", getRec.Code, getRec.Body.String())
	}
	var auditAfterGet int64
	if err := db.Model(&model.AuditLog{}).Where("action = ?", "ai.tool.execute").Count(&auditAfterGet).Error; err != nil {
		t.Fatal(err)
	}
	if auditAfterGet != auditAfterWeb {
		t.Fatalf("L0 read tool call must NOT write AuditLog: before=%d after=%d", auditAfterWeb, auditAfterGet)
	}

	// 4) L2 build_save_variable_set rejects remembered decision, succeeds with preview + approve, and allows nested user payload projectId
	nestedBody := `{"name":"ci-vars","scope":"project","addProjects":["` + boundProject.ID + `"],"setVariables":[{"key":"FIREBASE_CONFIG","value":"{\"projectId\":\"` + otherProject.ID + `\"}"},{"key":"projectId","value":"` + otherProject.ID + `"}],"enabled":true}`
	l2RememberedRec := callTool("/internal/v1/ai/tools/execute", "aitool_l2_forbid_remembered", "build_save_variable_set", nestedBody, "running", "remembered", "")
	if l2RememberedRec.Code != http.StatusPreconditionRequired || !strings.Contains(l2RememberedRec.Body.String(), "ai.approval_required") {
		t.Fatalf("expected 428 ai.approval_required for L2 build_save_variable_set with remembered, got %d %s", l2RememberedRec.Code, l2RememberedRec.Body.String())
	}

	prevRec := callTool("/internal/v1/ai/tools/preview", "aitool_nested_ok", "build_save_variable_set", nestedBody, "proposed", "", "")
	if prevRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from build_save_variable_set preview, got %d %s", prevRec.Code, prevRec.Body.String())
	}
	var prevRes struct {
		PreviewLock string `json:"previewLock"`
	}
	if err := json.Unmarshal(prevRec.Body.Bytes(), &prevRes); err != nil || prevRes.PreviewLock == "" {
		t.Fatalf("decode preview lock: %v %s", err, prevRec.Body.String())
	}
	nestedRec := callTool("/internal/v1/ai/tools/execute", "aitool_nested_ok", "build_save_variable_set", nestedBody, "running", "approve", prevRes.PreviewLock)
	if nestedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for build_save_variable_set with nested dict, got %d %s", nestedRec.Code, nestedRec.Body.String())
	}

	// 5) Sandbox blocks mismatched addProjects, scope=global, and case-smuggled AddProjects
	for name, badBody := range map[string]string{
		"mismatched_add": `{"name":"bad-vars","scope":"project","addProjects":["` + otherProject.ID + `"],"setVariables":[{"key":"K","value":"V"}]}`,
		"global_scope":   `{"name":"global-vars","scope":"global","setVariables":[{"key":"K","value":"V"}]}`,
		"case_smuggle":   `{"name":"smuggle-vars","scope":"project","addProjects":["` + boundProject.ID + `"],"AddProjects":["` + otherProject.ID + `"],"setVariables":[{"key":"K","value":"V"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := callTool("/internal/v1/ai/tools/preview", "aitool_bad_"+name, "build_save_variable_set", badBody, "proposed", "", "")
			if r.Code != http.StatusForbidden || !strings.Contains(r.Body.String(), "ai.project_scope_mismatch") {
				t.Fatalf("case %s expected 403 ai.project_scope_mismatch, got %d %s", name, r.Code, r.Body.String())
			}
		})
	}
}

func aiProjectScopeIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	databaseName := fmt.Sprintf("luna_ai_tool_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(databaseName, "luna_ai_tool_test_") {
		t.Fatalf("refuse unsafe integration database name %q", databaseName)
	}
	if err := adminDB.Exec(`CREATE DATABASE "` + databaseName + `"`).Error; err != nil {
		t.Fatalf("create isolated integration database: %v", err)
	}
	var db *gorm.DB
	t.Cleanup(func() {
		if db != nil {
			if sqlDB, dbErr := db.DB(); dbErr == nil {
				_ = sqlDB.Close()
			}
		}
		if dropErr := adminDB.Exec(`DROP DATABASE IF EXISTS "` + databaseName + `" WITH (FORCE)`).Error; dropErr != nil {
			t.Errorf("drop isolated integration database: %v", dropErr)
		}
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	parsedURL.Path = "/" + databaseName
	parsedURL.RawPath = ""
	query := parsedURL.Query()
	query.Del("search_path")
	parsedURL.RawQuery = query.Encode()
	db, err = gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated integration database: %v", err)
	}
	if err := database.MigrateContext(context.Background(), db); err != nil {
		t.Fatalf("migrate isolated integration database: %v", err)
	}
	return db
}
