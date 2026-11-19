package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestDiagnosticsEndpointsEnforceScopesNoStoreAndAIToolIdentityPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv(aiagent.InternalSecretEnvironment, "ai-diagnostics-integration-secret-00001")
	db := aiProjectScopeIntegrationDB(t)

	now := time.Now().UTC()
	owner := model.User{ID: "usr_diag_owner", Email: "diag-owner@example.test", Name: "Diag Owner", Role: authz.PlatformRoleUser}
	outsider := model.User{ID: "usr_diag_outsider", Email: "diag-outsider@example.test", Name: "Outsider", Role: authz.PlatformRoleUser}
	session := model.UserSession{
		ID: "ses_diag_owner", UserID: owner.ID, TokenHash: "diag-owner-session-hash", ExpiresAt: now.Add(time.Hour),
	}
	project := model.Project{ID: "prj_diag_it", Identifier: "diag-it", Name: "Diag IT", DeleteStatus: "active"}
	otherProject := model.Project{ID: "prj_diag_other", Identifier: "diag-other", Name: "Diag Other", DeleteStatus: "active"}
	member := model.ProjectMember{ID: "prjm_diag_owner", ProjectID: project.ID, UserID: owner.ID, Role: authz.ProjectRoleDeveloper}
	otherMember := model.ProjectMember{ID: "prjm_diag_other", ProjectID: otherProject.ID, UserID: owner.ID, Role: authz.ProjectRoleDeveloper}
	app := model.Application{ID: "app_diag_it", ProjectID: project.ID, Identifier: "api-svc", Name: "API Service", DeleteStatus: "active"}
	cluster := model.RuntimeCluster{
		ID: "cls_diag_default", Name: "Default Cluster", Scope: "global", IsDefault: true,
		DeleteStatus: "active", GatewayPublicScheme: "https", GatewayPublicPort: 443,
	}
	target := model.DeploymentTarget{
		ID: "tgt_diag_it", ProjectID: project.ID, ApplicationID: app.ID, ClusterID: cluster.ID,
		Name: "API Prod", Stage: "prod", KubernetesName: "api-svc-prod",
		WorkloadType: "Deployment", Replicas: 2, Enabled: true,
		ImageRef: "ghcr.io/liteyuki/api:v1.2.0",
		EnvVars:  `[{"key":"PUBLIC_ENV","value":"1","valueMode":"public"}]`,
	}
	failedBuild := model.BuildRun{
		ID: "bld_diag_it", ProjectID: project.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		Status: "failed", FailureCode: "build.step_failed", TriggerType: "manual", SourceBranch: "main",
		CreatedAt: now.Add(-10 * time.Minute), FinishedAt: &now,
	}
	failedRelease := model.Release{
		ID: "rel_diag_it", ProjectID: project.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		ImageRef: "ghcr.io/liteyuki/api:v1.2.0", Type: "deploy", Status: "failed",
		FailureCode: "release.rollout_timeout", Revision: 3, Message: "rollout timed out",
		CreatedAt: now.Add(-5 * time.Minute), FinishedAt: &now,
	}
	route := model.GatewayRoute{
		ID: "grt_diag_it", ProjectID: project.ID, ApplicationID: app.ID, DeploymentTargetID: target.ID,
		Host: "api.example.test", Path: "/", ServicePort: 8080, TLSMode: "http-challenge", Enabled: true, DeleteStatus: "active",
	}

	rawLimitedToken := "luna_pat_limited_project_only_0001"
	limitedPAT := model.AccessToken{
		ID:        "pat_diag_limited",
		UserID:    owner.ID,
		Name:      "limited",
		Scope:     "project:read",
		TokenHash: transportapi.HashToken(rawLimitedToken),
		Source:    "personal",
	}
	rawFullReadToken := "luna_pat_full_diag_read_000000002"
	fullReadPAT := model.AccessToken{
		ID:        "pat_diag_full",
		UserID:    owner.ID,
		Name:      "full-read",
		Scope:     "project:read build:read deployment:read gateway:read",
		TokenHash: transportapi.HashToken(rawFullReadToken),
		Source:    "personal",
	}
	rawOutsiderSession := "diag-outsider-session-raw"
	outsiderSession := model.UserSession{
		ID: "ses_diag_outsider", UserID: outsider.ID, TokenHash: transportapi.HashToken(rawOutsiderSession), ExpiresAt: now.Add(time.Hour),
	}

	for _, record := range []any{
		&owner, &outsider, &session, &outsiderSession, &project, &otherProject,
		&member, &otherMember, &app, &cluster, &target, &failedBuild, &failedRelease, &route,
		&limitedPAT, &fullReadPAT,
	} {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("seed %T: %v", record, err)
		}
	}

	router := NewRouter(db, mustTestConfig(t))

	// 1. PAT missing build:read / deployment:read / gateway:read must be rejected with 403
	limitedReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/diagnostics", nil)
	limitedReq.Header.Set("Authorization", "Bearer "+rawLimitedToken)
	limitedRec := httptest.NewRecorder()
	router.ServeHTTP(limitedRec, limitedReq)
	if limitedRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for PAT missing build/deployment/gateway read scopes, got %d: %s", limitedRec.Code, limitedRec.Body.String())
	}

	// 2. Non-member user must be rejected with 403
	outsiderReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/diagnostics", nil)
	outsiderReq.AddCookie(&http.Cookie{Name: sessionCookieName, Value: rawOutsiderSession})
	outsiderRec := httptest.NewRecorder()
	router.ServeHTTP(outsiderRec, outsiderReq)
	if outsiderRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %d: %s", outsiderRec.Code, outsiderRec.Body.String())
	}

	// 3. Full read PAT succeeds with Cache-Control: no-store and honest live observation
	projectDiagReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/diagnostics", nil)
	projectDiagReq.Header.Set("Authorization", "Bearer "+rawFullReadToken)
	projectDiagRec := httptest.NewRecorder()
	router.ServeHTTP(projectDiagRec, projectDiagReq)
	if projectDiagRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for getProjectDiagnostics, got %d: %s", projectDiagRec.Code, projectDiagRec.Body.String())
	}
	if cc := projectDiagRec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("expected Cache-Control: no-store, got %q", cc)
	}
	var projectDiagBody map[string]any
	if err := json.Unmarshal(projectDiagRec.Body.Bytes(), &projectDiagBody); err != nil {
		t.Fatalf("decode project diagnostics: %v", err)
	}
	buildsObj, _ := projectDiagBody["builds"].(map[string]any)
	releasesObj, _ := projectDiagBody["releases"].(map[string]any)
	if buildsObj["failedCount"] != float64(1) || releasesObj["failedCount"] != float64(1) {
		t.Fatalf("unexpected failed counts in project diagnostics: %#v", projectDiagBody)
	}

	// 4. Deployment target diagnostics via AI L0 tool execution (no approval required) + project-bound isolation
	keys, err := aiagent.DeriveInternalKeys("ai-diagnostics-integration-secret-00001")
	if err != nil {
		t.Fatalf("derive keys: %v", err)
	}
	convID := "aicnv_diag_bound"
	turnID := "aitrn_diag_bound"
	runID := "airun_diag_bound"
	if err := db.Exec(
		`INSERT INTO ai.conversations (id, owner_user_id, title, status, project_id) VALUES (?, ?, 'Diag', 'active', ?)`,
		convID, owner.ID, project.ID,
	).Error; err != nil {
		t.Fatalf("insert ai conversation: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES (?, ?, 1, 'running', '排查', ?)`,
		turnID, convID, runID,
	).Error; err != nil {
		t.Fatalf("insert ai turn: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest, actor_session_id) VALUES (?, ?, ?, ?, 1, 'running', 'system-v4', 'test', ?)`,
		runID, owner.ID, convID, turnID, session.ID,
	).Error; err != nil {
		t.Fatalf("insert ai run: %v", err)
	}
	diagArgs := `{"project":"` + project.ID + `","application":"` + app.ID + `","target":"` + target.ID + `"}`
	diagHash, err := aitool.CanonicalJSONHash([]byte(diagArgs))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(
		`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, arguments_hash, input_mode) VALUES ('aitool_diag_l0', ?, 'runtime_get_target_diagnostics', 'running', ?::jsonb, ?, 'model')`,
		runID, diagArgs, diagHash,
	).Error; err != nil {
		t.Fatalf("insert ai tool_call: %v", err)
	}

	aiTargetReq := httptest.NewRequest(
		http.MethodPost,
		"/internal/v1/ai/tools/execute",
		strings.NewReader(`{"toolName":"runtime_get_target_diagnostics","arguments":`+diagArgs+`}`),
	)
	aiTargetReq.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	aiTargetReq.Header.Set("Content-Type", "application/json")
	aiTargetReq.Header.Set(aiapi.RunIDHeader, runID)
	aiTargetReq.Header.Set(aiapi.ToolCallIDHeader, "aitool_diag_l0")
	aiTargetRec := httptest.NewRecorder()
	router.ServeHTTP(aiTargetRec, aiTargetReq)
	if aiTargetRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for L0 AI runtime_get_target_diagnostics without approval, got %d: %s", aiTargetRec.Code, aiTargetRec.Body.String())
	}
	if cc := aiTargetRec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("expected Cache-Control: no-store on target diagnostics, got %q", cc)
	}
	if strings.Contains(aiTargetRec.Body.String(), "PUBLIC_ENV") {
		t.Fatalf("target diagnostics must not dump raw envVars: %s", aiTargetRec.Body.String())
	}
	var targetDiagBody map[string]any
	if err := json.Unmarshal(aiTargetRec.Body.Bytes(), &targetDiagBody); err != nil {
		t.Fatalf("decode target diagnostics: %v", err)
	}
	podsObj, _ := targetDiagBody["pods"].(map[string]any)
	eventsObj, _ := targetDiagBody["events"].(map[string]any)
	if podsObj["status"] != "unavailable" || eventsObj["status"] != "unavailable" {
		t.Fatalf("expected unavailable pod/event observations when kubeconfig is not configured, got pods=%#v events=%#v", podsObj, eventsObj)
	}
	routesArr, _ := targetDiagBody["accessRoutes"].([]any)
	if len(routesArr) != 1 {
		t.Fatalf("expected 1 accessRoute in target diagnostics, got %#v", targetDiagBody["accessRoutes"])
	}
	firstRoute, _ := routesArr[0].(map[string]any)
	if firstRoute["accessUrl"] != "https://api.example.test" || firstRoute["certificateStatus"] != "unavailable" {
		t.Fatalf("expected populated accessUrl and live certificateStatus on target diagnostics route, got %#v", firstRoute)
	}

	// Cross-project request in project-bound conversation must be rejected with 403 ai.project_scope_mismatch
	crossArgs := `{"project":"` + otherProject.ID + `","includeDiagnostics":true}`
	crossHash, _ := aitool.CanonicalJSONHash([]byte(crossArgs))
	if err := db.Exec(
		`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, arguments_hash, input_mode) VALUES ('aitool_diag_cross', ?, 'workspace_get_project', 'running', ?::jsonb, ?, 'model')`,
		runID, crossArgs, crossHash,
	).Error; err != nil {
		t.Fatalf("insert cross-project ai tool_call: %v", err)
	}
	crossReq := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute", strings.NewReader(`{"toolName":"workspace_get_project","arguments":`+crossArgs+`}`))
	crossReq.Header.Set("Authorization", "Bearer "+keys.CallbackServiceToken)
	crossReq.Header.Set("Content-Type", "application/json")
	crossReq.Header.Set(aiapi.RunIDHeader, runID)
	crossReq.Header.Set(aiapi.ToolCallIDHeader, "aitool_diag_cross")
	crossRec := httptest.NewRecorder()
	router.ServeHTTP(crossRec, crossReq)
	if crossRec.Code != http.StatusForbidden || !strings.Contains(crossRec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch on cross-project diagnostics, got %d: %s", crossRec.Code, crossRec.Body.String())
	}
}
