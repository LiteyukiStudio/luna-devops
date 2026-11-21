package aiapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type proposalTestHost struct {
	*stubAIToolHost
	currentUser model.User
	project     model.Project
	hasSession  bool
}

func (h *proposalTestHost) CurrentSessionFromCookie(*gin.Context) (model.UserSession, bool) {
	if !h.hasSession {
		return model.UserSession{}, false
	}
	return model.UserSession{ID: "ses_" + h.currentUser.ID, UserID: h.currentUser.ID}, true
}

func (h *proposalTestHost) AuthorizeProject(*gin.Context, authz.Action) (model.User, model.Project, bool) {
	return h.currentUser, h.project, true
}

func TestProjectAIActionProposalPreviewLockAllowlistAndSandbox(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := aitool.NewRegistry()
	var executedBy string
	var executedProject string
	aitool.Register(reg, aitool.Spec[struct {
		Project string `json:"project"`
		Target  string `json:"target"`
	}, map[string]any]{
		Name:           "delivery_restart_target",
		Domain:         aitool.DomainDelivery,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{"deployment:restart"},
		Verification:   []aitool.OperationVerification{{OperationID: "delivery_restart_target", Family: "resource"}},
		Preview: func(_ *aitool.ToolContext, in struct {
			Project string `json:"project"`
			Target  string `json:"target"`
		}) (*aitool.PreviewResult, error) {
			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        "Restart " + in.Target,
					TargetResource: in.Target,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in struct {
			Project string `json:"project"`
			Target  string `json:"target"`
		}) (map[string]any, error) {
			executedBy = tc.User.ID
			executedProject = in.Project
			return map[string]any{"restarted": in.Target}, nil
		},
	})
	aitool.Register(reg, aitool.Spec[struct {
		Project string `json:"project"`
	}, map[string]any]{
		Name:           "workspace_delete_project",
		Domain:         aitool.DomainWorkspace,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{"project:delete"},
		Verification:   []aitool.OperationVerification{{OperationID: "workspace_delete_project", Family: "resource"}},
		Preview: func(_ *aitool.ToolContext, _ struct {
			Project string `json:"project"`
		}) (*aitool.PreviewResult, error) {
			return &aitool.PreviewResult{}, nil
		},
		Execute: func(_ *aitool.ToolContext, _ struct {
			Project string `json:"project"`
		}) (map[string]any, error) {
			return map[string]any{}, nil
		},
	})

	state := &stubAIToolDBState{
		userRole:    authz.PlatformRoleUser,
		projectID:   "prj_prop",
		projectSlug: "prop",
	}
	host := &proposalTestHost{
		stubAIToolHost: &stubAIToolHost{
			db:  newStubGormDB(t, state),
			reg: reg,
		},
		currentUser: model.User{ID: "usr_alice", Role: authz.PlatformRoleUser},
		project:     model.Project{ID: "prj_prop", Identifier: "prop"},
		hasSession:  false,
	}
	handler := New(host)
	router := gin.New()
	router.POST("/api/v1/projects/:projectId/ai-action-proposals/preview", handler.PreviewProjectAIActionProposal)
	router.POST("/api/v1/projects/:projectId/ai-action-proposals/execute", handler.ExecuteProjectAIActionProposal)

	// 1. Without cookie session (e.g., PAT) -> 401 auth.session.missing
	noSesReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_prop/ai-action-proposals/preview", strings.NewReader(`{"toolName":"delivery_restart_target","arguments":{"target":"dt_1"}}`))
	noSesReq.Header.Set("Content-Type", "application/json")
	noSesRec := httptest.NewRecorder()
	router.ServeHTTP(noSesRec, noSesReq)
	if noSesRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without browser session, got %d %s", noSesRec.Code, noSesRec.Body.String())
	}

	host.hasSession = true

	// 2. Non-remediation tool (workspace_delete_project) -> 403 ai.tool_not_available
	badToolReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_prop/ai-action-proposals/preview", strings.NewReader(`{"toolName":"workspace_delete_project","arguments":{"project":"prj_prop"}}`))
	badToolReq.Header.Set("Content-Type", "application/json")
	badToolRec := httptest.NewRecorder()
	router.ServeHTTP(badToolRec, badToolReq)
	if badToolRec.Code != http.StatusForbidden || !strings.Contains(badToolRec.Body.String(), "ai.tool_not_available") {
		t.Fatalf("expected 403 ai.tool_not_available for non-remediation tool, got %d %s", badToolRec.Code, badToolRec.Body.String())
	}

	// 3. Cross-project escape attempt ("project":"prj_other") -> 403 ai.project_scope_mismatch
	escapeReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_prop/ai-action-proposals/preview", strings.NewReader(`{"toolName":"delivery_restart_target","arguments":{"project":"prj_other","target":"dt_1"}}`))
	escapeReq.Header.Set("Content-Type", "application/json")
	escapeRec := httptest.NewRecorder()
	router.ServeHTTP(escapeRec, escapeReq)
	if escapeRec.Code != http.StatusForbidden || !strings.Contains(escapeRec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch for cross-project argument, got %d %s", escapeRec.Code, escapeRec.Body.String())
	}

	// 4. Alice previews omitting project -> auto-bound to prj_prop + 200 OK + previewLock
	prevReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_prop/ai-action-proposals/preview", strings.NewReader(`{"toolName":"delivery_restart_target","arguments":{"target":"dt_1"}}`))
	prevReq.Header.Set("Content-Type", "application/json")
	prevRec := httptest.NewRecorder()
	router.ServeHTTP(prevRec, prevReq)
	if prevRec.Code != http.StatusOK {
		t.Fatalf("expected 200 preview, got %d %s", prevRec.Code, prevRec.Body.String())
	}
	var prevOut AIActionProposalPreviewOutput
	if err := json.Unmarshal(prevRec.Body.Bytes(), &prevOut); err != nil || prevOut.PreviewLock == "" {
		t.Fatalf("decode preview output: %v %+v", err, prevOut)
	}

	// 5. Bob tries to execute using Alice's previewLock -> 409 ai.preview_lock_mismatch!
	host.currentUser = model.User{ID: "usr_bob", Role: authz.PlatformRoleUser}
	execBody, _ := json.Marshal(map[string]any{
		"toolName":    "delivery_restart_target",
		"arguments":   map[string]any{"target": "dt_1"},
		"previewLock": prevOut.PreviewLock,
	})
	bobRec := httptest.NewRecorder()
	bobReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_prop/ai-action-proposals/execute", strings.NewReader(string(execBody)))
	bobReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(bobRec, bobReq)
	if bobRec.Code != http.StatusConflict || !strings.Contains(bobRec.Body.String(), "ai.preview_lock_mismatch") {
		t.Fatalf("expected 409 ai.preview_lock_mismatch when reusing another user's previewLock, got %d %s", bobRec.Code, bobRec.Body.String())
	}

	// 6. Alice executes with her own previewLock -> 200 OK + bound project + writes AuditLog!
	host.currentUser = model.User{ID: "usr_alice", Role: authz.PlatformRoleUser}
	aliceRec := httptest.NewRecorder()
	aliceReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_prop/ai-action-proposals/execute", strings.NewReader(string(execBody)))
	aliceReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(aliceRec, aliceReq)
	if aliceRec.Code != http.StatusOK || executedBy != "usr_alice" || executedProject != "prj_prop" {
		t.Fatalf("expected 200 executed by usr_alice in prj_prop, got %d %s (executedBy=%q proj=%q)", aliceRec.Code, aliceRec.Body.String(), executedBy, executedProject)
	}
	if len(host.audits) != 1 {
		t.Fatalf("expected 1 AuditLog entry on proposal execution, got %v", host.audits)
	}
}
