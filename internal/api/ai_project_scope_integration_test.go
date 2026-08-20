package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAIProjectListScopeDelegationPostgres(t *testing.T) {
	db := aiProjectScopeIntegrationDB(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-project-scope-integration-secret-0001")

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
	relatedItems := executeAIProjectList(t, router, admin, session, map[string]any{"scope": "related", "page": 1, "pageSize": 20})
	if !projectItemsContain(relatedItems, related.ID) || projectItemsContain(relatedItems, unrelated.ID) {
		t.Fatalf("related scope items = %#v", relatedItems)
	}
	allItems := executeAIProjectList(t, router, admin, session, map[string]any{"scope": "all", "page": 1, "pageSize": 20})
	if !projectItemsContain(allItems, related.ID) || !projectItemsContain(allItems, unrelated.ID) {
		t.Fatalf("all scope items = %#v", allItems)
	}
}

func executeAIProjectList(t *testing.T, router http.Handler, user model.User, session model.UserSession, arguments map[string]any) []any {
	t.Helper()
	keys, err := aiagent.LoadInternalKeys()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	runID := fmt.Sprintf("airun_scope_%s", arguments["scope"])
	toolCallID := fmt.Sprintf("aitool_scope_%s", arguments["scope"])
	grant, err := aiagent.SignRunActorGrant(aiagent.RunActorGrant{
		Audience: "luna-ai-run-grant", Purpose: "agent_delegation_exchange",
		RunID: runID, ConversationID: "aicnv_scope", UserID: user.ID, SessionID: session.ID,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(),
	}, keys.RunActorGrantSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	argumentsHash := hashAICanonicalArguments(string(canonical))
	exchangePayload, _ := json.Marshal(map[string]any{
		"runActorGrant": grant, "runId": runID, "toolCallId": toolCallID,
		"operationId": "listProjects", "requestedScopes": []string{"project:read"},
		"argumentsHash": argumentsHash, "approvalGranted": false,
	})
	exchange := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/delegations/exchange", bytes.NewReader(exchangePayload))
	exchange.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	exchange.Header.Set("Content-Type", "application/json")
	exchangeRecorder := httptest.NewRecorder()
	router.ServeHTTP(exchangeRecorder, exchange)
	if exchangeRecorder.Code != http.StatusOK {
		t.Fatalf("delegation exchange = %d %s", exchangeRecorder.Code, exchangeRecorder.Body.String())
	}
	var exchangeResponse struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(exchangeRecorder.Body.Bytes(), &exchangeResponse); err != nil || exchangeResponse.AccessToken == "" {
		t.Fatalf("delegation response = %s, error = %v", exchangeRecorder.Body.String(), err)
	}

	executePayload, _ := json.Marshal(map[string]any{"argumentsCanonical": string(canonical)})
	execute := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/listProjects/execute", bytes.NewReader(executePayload))
	execute.Header.Set("Authorization", "Bearer "+exchangeResponse.AccessToken)
	execute.Header.Set("Content-Type", "application/json")
	executeRecorder := httptest.NewRecorder()
	router.ServeHTTP(executeRecorder, execute)
	if executeRecorder.Code != http.StatusOK {
		t.Fatalf("tool execution = %d %s", executeRecorder.Code, executeRecorder.Body.String())
	}
	var response struct {
		Result struct {
			Items []any `json:"items"`
		} `json:"result"`
	}
	if err := json.Unmarshal(executeRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	return response.Result.Items
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
	schemaName := fmt.Sprintf("ai_project_scope_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schemaName)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration schema: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.UserSession{}, &model.Project{}, &model.ProjectMember{},
		&model.ProjectPin{}, &model.AuditLog{}, &model.AppConfig{},
	); err != nil {
		t.Fatalf("migrate integration schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
