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
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAIProjectListScopeDirectExecutionPostgres(t *testing.T) {
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
		ID: "prj_ai_scope_related", Identifier: "ai-scope-related", Name: "Related", NamespaceStrategy: "project", DeleteStatus: "active",
	}
	unrelated := model.Project{
		ID: "prj_ai_scope_unrelated", Identifier: "ai-scope-unrelated", Name: "Unrelated", NamespaceStrategy: "project", DeleteStatus: "active",
	}
	member := model.ProjectMember{ID: "prjm_ai_scope_admin", ProjectID: related.ID, UserID: admin.ID, Role: authz.ProjectRoleOwner}
	for _, value := range []any{&admin, &session, &related, &unrelated, &member} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	router := NewRouter(db)
	relatedItems := executeAIProjectList(t, db, router, admin, session, map[string]any{"scope": "related", "page": 1, "pageSize": 20}, "")
	if !projectItemsContain(relatedItems, related.ID) || projectItemsContain(relatedItems, unrelated.ID) {
		t.Fatalf("related scope items = %#v", relatedItems)
	}
	allItems := executeAIProjectList(t, db, router, admin, session, map[string]any{"scope": "all", "page": 1, "pageSize": 20}, "")
	if !projectItemsContain(allItems, related.ID) || !projectItemsContain(allItems, unrelated.ID) {
		t.Fatalf("all scope items = %#v", allItems)
	}
	boundItems := executeAIProjectList(t, db, router, admin, session, map[string]any{"scope": "all", "page": 1, "pageSize": 20}, related.ID)
	if !projectItemsContain(boundItems, related.ID) || projectItemsContain(boundItems, unrelated.ID) {
		t.Fatalf("project-bound scope items = %#v", boundItems)
	}
}

func TestAIProjectVolumeDirectExecutionAndAuthoritativeReadbackPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-project-volume-integration-secret-01")
	db := aiProjectScopeIntegrationDB(t)

	now := time.Now().UTC()
	user := model.User{ID: "usr_ai_volume_owner", Email: "ai-volume-owner@example.test", Name: "AI Volume Owner", Role: authz.PlatformRoleAdmin}
	session := model.UserSession{ID: "ses_ai_volume_owner", UserID: user.ID, TokenHash: "ai-volume-session-token", ExpiresAt: now.Add(time.Hour)}
	project := model.Project{
		ID: "prj_ai_volume", Identifier: "ai-volume", Name: "AI Volume", NamespaceStrategy: "project",
		KubernetesNamespace: "luna-ai-volume", BillingOwnerUserID: user.ID, DeleteStatus: "active",
	}
	member := model.ProjectMember{ID: "prjm_ai_volume_owner", ProjectID: project.ID, UserID: user.ID, Role: authz.ProjectRoleOwner}
	cluster := model.RuntimeCluster{ID: "rcl_ai_volume", Name: "AI Volume Cluster", Type: "kubernetes", Scope: "global", CreatedBy: user.ID}
	wallet := model.UserWallet{ID: "wlt_ai_volume_owner", UserID: user.ID, BalanceCredits: decimal.NewFromInt(100)}
	for _, value := range []any{&user, &session, &project, &member, &cluster, &wallet} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}

	handlers := NewHandlers(db)
	tasks := &volumeTaskEnqueuerStub{}
	handlers.volumes = volume.NewGormService(db, volumeOperationDispatcher{tasks: tasks})
	// The readback deliberately exercises the public unavailable observation
	// contract without requiring a test Kubernetes credential.
	handlers.volumeClusters = nil
	router := gin.New()
	router.Use(handlers.aiToolExecutionIdentityMiddleware())
	router.POST("/api/v1/projects/:projectId/volumes", handlers.CreateProjectVolume)
	router.GET("/api/v1/projects/:projectId/volumes/:volumeId", handlers.GetProjectVolume)

	keys, err := aiagent.LoadInternalKeys()
	if err != nil {
		t.Fatal(err)
	}
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
	createArguments := `{"projectId":"` + project.ID + `","displayName":"agent-smoke","clusterId":"` + cluster.ID + `","capacity":"1Gi","storageClassName":"standard","accessMode":"ReadWriteOnce","volumeMode":"Filesystem","source":{"type":"blank"}}`
	if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, input_mode, approval_decision) VALUES (?, ?, 'createProjectVolume', 'running', ?::jsonb, 'model', 'approve')`, createToolCallID, runID, createArguments).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(`DELETE FROM ai.conversations WHERE id = ?`, conversationID).Error })

	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/volumes", bytes.NewBufferString(`{"displayName":"agent-smoke","clusterId":"`+cluster.ID+`","capacity":"1Gi","storageClassName":"standard","accessMode":"ReadWriteOnce","volumeMode":"Filesystem","source":{"type":"blank"}}`))
	createRequest.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.Header.Set("Idempotency-Key", createToolCallID)
	createRequest.Header.Set(aiRunIDHeader, runID)
	createRequest.Header.Set(aiToolCallIDHeader, createToolCallID)
	createRecorder := httptest.NewRecorder()
	router.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusAccepted {
		t.Fatalf("create volume = %d %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created projectVolumeResponse
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || tasks.provision.VolumeID != created.ID || tasks.provision.ProjectID != project.ID {
		t.Fatalf("created volume = %#v, task = %#v", created, tasks.provision)
	}

	getToolCallID := "aitool_ai_volume_get"
	if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, input_mode) VALUES (?, ?, 'getProjectVolume', 'running', ?::jsonb, 'model')`, getToolCallID, runID, `{"projectId":"`+project.ID+`","volumeId":"`+created.ID+`"}`).Error; err != nil {
		t.Fatal(err)
	}
	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/volumes/"+created.ID, nil)
	getRequest.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	getRequest.Header.Set(aiRunIDHeader, runID)
	getRequest.Header.Set(aiToolCallIDHeader, getToolCallID)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, getRequest)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("read volume = %d %s", getRecorder.Code, getRecorder.Body.String())
	}
	var readback projectVolumeDetailResponse
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &readback); err != nil {
		t.Fatal(err)
	}
	if readback.ID != created.ID || readback.ProjectID != project.ID || readback.DisplayName != "agent-smoke" || readback.Observation.Status != "unavailable" {
		t.Fatalf("authoritative readback = %#v", readback)
	}
	if getRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("readback cache policy = %q", getRecorder.Header().Get("Cache-Control"))
	}
}

func executeAIProjectList(t *testing.T, db *gorm.DB, router http.Handler, user model.User, session model.UserSession, arguments map[string]any, conversationProjectID string) []any {
	t.Helper()
	keys, err := aiagent.LoadInternalKeys()
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%s_%s", arguments["scope"], strings.TrimPrefix(conversationProjectID, "prj_ai_scope_"))
	runID := "airun_scope_" + suffix
	toolCallID := "aitool_scope_" + suffix
	conversationID := "aicnv_scope_" + suffix
	turnID := "aitrn_scope_" + suffix
	var projectID any
	if conversationProjectID != "" {
		projectID = conversationProjectID
	}
	if err := db.Exec(`INSERT INTO ai.conversations (id, owner_user_id, title, status, project_id) VALUES (?, ?, 'scope test', 'active', ?)`, conversationID, user.ID, projectID).Error; err != nil {
		t.Fatalf("seed AI conversation: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES (?, ?, 1, 'running', 'list projects', ?)`, turnID, conversationID, runID).Error; err != nil {
		t.Fatalf("seed AI turn: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest, actor_session_id) VALUES (?, ?, ?, ?, 1, 'running', 'system-v4', 'test', ?)`, runID, user.ID, conversationID, turnID, session.ID).Error; err != nil {
		t.Fatalf("seed AI run: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, input_mode) VALUES (?, ?, 'listProjects', 'running', ?::jsonb, 'model')`, toolCallID, runID, `{"scope":"`+fmt.Sprint(arguments["scope"])+`"}`).Error; err != nil {
		t.Fatalf("seed AI ToolCall: %v", err)
	}
	t.Cleanup(func() { _ = db.Exec(`DELETE FROM ai.conversations WHERE id = ?`, conversationID).Error })

	target := fmt.Sprintf("/api/v1/projects?scope=%s&page=1&pageSize=20", url.QueryEscape(fmt.Sprint(arguments["scope"])))
	execute := httptest.NewRequest(http.MethodGet, target, nil)
	execute.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	execute.Header.Set(aiRunIDHeader, runID)
	execute.Header.Set(aiToolCallIDHeader, toolCallID)
	executeRecorder := httptest.NewRecorder()
	router.ServeHTTP(executeRecorder, execute)
	if executeRecorder.Code != http.StatusOK {
		t.Fatalf("tool execution = %d %s", executeRecorder.Code, executeRecorder.Body.String())
	}
	var response struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(executeRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	return response.Items
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
