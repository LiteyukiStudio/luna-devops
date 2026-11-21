package aiapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestExecuteHeadlessAIToolInternalEnforcesL0ReadOnlyAndProjectSandbox(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := aitool.NewRegistry()
	var capturedProject string
	aitool.Register(reg, aitool.Spec[struct {
		Project *string `json:"project,omitempty"`
	}, map[string]any]{
		Name:           "workspace_list_projects",
		Domain:         aitool.DomainWorkspace,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{"project:read"},
		Execute: func(tc *aitool.ToolContext, in struct {
			Project *string `json:"project,omitempty"`
		}) (map[string]any, error) {
			if in.Project != nil {
				capturedProject = *in.Project
			} else {
				capturedProject = ""
			}
			return map[string]any{"user": tc.User.ID, "project": capturedProject}, nil
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

	aitool.Register(reg, aitool.Spec[struct {
		Project string `json:"project"`
	}, map[string]any]{
		Name:           "ops_save_project_notification_rule",
		Domain:         aitool.DomainOps,
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{"notification:manage"},
		Verification:   []aitool.OperationVerification{{OperationID: "workspace_list_projects", Family: "resource"}},
		Execute: func(_ *aitool.ToolContext, in struct {
			Project string `json:"project"`
		}) (map[string]any, error) {
			return map[string]any{"project": in.Project}, nil
		},
	})

	state := &stubAIToolDBState{
		userRole:    authz.PlatformRoleUser,
		projectID:   "prj_bound",
		projectSlug: "bound-slug",
	}
	host := &stubAIToolHost{
		db:            newStubGormDB(t, state),
		reg:           reg,
		callbackToken: "primary-callback-token-32-bytes!!",
		previousToken: "previous-callback-token-32-bytes!",
	}
	handler := New(host)
	router := gin.New()
	router.POST("/internal/v1/ai/tools/execute-headless", handler.ExecuteHeadlessAIToolInternal)

	callHeadless := func(token, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/execute-headless", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(RunIDHeader, "airun_hl_1")
		req.Header.Set(ToolCallIDHeader, "tc_hl_1")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. Missing internal service token -> 401
	rec := callHeadless("", `{"actorUserId":"usr_1","projectId":"prj_bound","toolName":"workspace_list_projects","arguments":{}}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without service token, got %d %s", rec.Code, rec.Body.String())
	}

	// 2. L0 read tool auto-injects bound projectId -> 200 OK (and also accepts rotated previous callback token)
	rec = callHeadless(host.callbackToken, `{"actorUserId":"usr_1","projectId":"prj_bound","toolName":"workspace_list_projects","arguments":{}}`)
	if rec.Code != http.StatusOK || capturedProject != "prj_bound" {
		t.Fatalf("expected 200 with injected prj_bound, got %d %s (captured=%q)", rec.Code, rec.Body.String(), capturedProject)
	}
	rec = callHeadless(host.previousToken, `{"actorUserId":"usr_1","projectId":"prj_bound","toolName":"workspace_list_projects","arguments":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with rotated previous callback token, got %d %s", rec.Code, rec.Body.String())
	}

	// 3. Cross-project escape rejected -> 403 ai.project_scope_mismatch
	rec = callHeadless(host.callbackToken, `{"actorUserId":"usr_1","projectId":"prj_bound","toolName":"workspace_list_projects","arguments":{"project":"prj_other"}}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch, got %d %s", rec.Code, rec.Body.String())
	}

	// 4. L0 non-read effect rejected -> 403 ai.headless_tier_exceeded
	rec = callHeadless(host.callbackToken, `{"actorUserId":"usr_1","projectId":"prj_bound","toolName":"ops_web_fetch","arguments":{"url":"https://example.com"}}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.headless_tier_exceeded") {
		t.Fatalf("expected 403 ai.headless_tier_exceeded for execute-effect tool, got %d %s", rec.Code, rec.Body.String())
	}

	// 5. L1 write tool rejected -> 403 ai.headless_tier_exceeded
	rec = callHeadless(host.callbackToken, `{"actorUserId":"usr_1","projectId":"prj_bound","toolName":"ops_save_project_notification_rule","arguments":{"project":"prj_bound"}}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai.headless_tier_exceeded") {
		t.Fatalf("expected 403 ai.headless_tier_exceeded for L1 tool, got %d %s", rec.Code, rec.Body.String())
	}
}

type headlessTestHost struct {
	*stubAIToolHost
	agentClient aiagent.Client
}

func (h *headlessTestHost) AIAgent() aiagent.Client { return h.agentClient }

func TestRunHeadlessInferenceForUserFastFailsOnEmptyWallet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	agentCalled := false
	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		agentCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer agentSrv.Close()

	agentClient, err := aiagent.NewHTTPClient(
		agentSrv.URL,
		"service-token-32-bytes-minimum!!",
		"actor-signing-key-32-bytes-min!!",
	)
	if err != nil {
		t.Fatal(err)
	}

	state := &stubAIToolDBState{userRole: authz.PlatformRoleUser}
	host := &headlessTestHost{
		stubAIToolHost: &stubAIToolHost{
			db:            newStubGormDB(t, state),
			callbackToken: "primary-callback-token-32-bytes!!",
		},
		agentClient: agentClient,
	}
	handler := New(host)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)

	// stubSQLConn returns empty rows for user_wallets -> GORM ErrRecordNotFound -> ErrAIWalletBalanceInsufficient -> 422!
	resp, ok := handler.RunHeadlessInferenceForUser(
		ctx,
		model.User{ID: "usr_1", Role: authz.PlatformRoleUser, Language: "zh-CN"},
		aiagent.HeadlessInferenceRequest{
			ActorUserID: "usr_1",
			ProjectID:   "prj_1",
			TaskType:    aiagent.HeadlessTaskRepoConfigInfer,
			MaxToolTier: aiagent.HeadlessToolTierNone,
		},
	)
	if ok || resp != nil {
		t.Fatalf("expected fast-fail when wallet is missing/empty, got resp=%+v", resp)
	}
	if agentCalled {
		t.Fatal("expected wallet preflight to block before calling luna-agent")
	}
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "ai.wallet_balance_insufficient") {
		t.Fatalf("expected 422 ai.wallet_balance_insufficient, got %d %s", rec.Code, rec.Body.String())
	}

	// Disabled user fails closed (401) before wallet check or agent call
	disabledRec := httptest.NewRecorder()
	disabledCtx, _ := gin.CreateTestContext(disabledRec)
	disabledCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
	_, ok = handler.RunHeadlessInferenceForUser(
		disabledCtx,
		model.User{ID: "usr_1", Role: authz.PlatformRoleUser, Disabled: true},
		aiagent.HeadlessInferenceRequest{TaskType: aiagent.HeadlessTaskRepoConfigInfer},
	)
	if ok || disabledRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for disabled user, got ok=%v status=%d body=%s", ok, disabledRec.Code, disabledRec.Body.String())
	}
}
