package projectapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type stubProjectAIHost struct {
	db                *gorm.DB
	user              model.User
	convProjectID     string
	reconciledProjIDs []string
	cleanupQueued     []string
}

func (s *stubProjectAIHost) DBFor(*gin.Context) *gorm.DB { return s.db }
func (s *stubProjectAIHost) DBWithContext(ctx context.Context) *gorm.DB {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx)
}
func (s *stubProjectAIHost) CurrentUser(*gin.Context) (model.User, bool) {
	return s.user, s.user.ID != ""
}
func (s *stubProjectAIHost) EnsurePlatformSystemProject(_ model.User, _ context.Context) (model.Project, error) {
	return model.Project{ID: "prj_sys", Identifier: "luna-system", Name: "System", SystemKey: "platform"}, nil
}
func (s *stubProjectAIHost) AIConversationProjectID(*gin.Context) string { return s.convProjectID }
func (s *stubProjectAIHost) EnqueueResourceCleanup(_ context.Context, _, resourceID, _, _ string) bool {
	s.cleanupQueued = append(s.cleanupQueued, resourceID)
	return true
}
func (s *stubProjectAIHost) EnqueueNetworkPolicyProjectReconcile(_ context.Context, projectID string) bool {
	s.reconciledProjIDs = append(s.reconciledProjIDs, projectID)
	return true
}
func (s *stubProjectAIHost) AuditWithContext(string, string, string, bool, string, context.Context) {}
func (s *stubProjectAIHost) ProjectIDsForUser(_ context.Context, userID string) []string {
	var ids []string
	_ = s.db.Model(&model.ProjectMember{}).Where("user_id = ?", userID).Pluck("project_id", &ids).Error
	return ids
}
func (s *stubProjectAIHost) ProjectHasAnotherOwner(_ context.Context, projectID, memberID string) bool {
	var count int64
	_ = s.db.Model(&model.ProjectMember{}).
		Where("project_id = ? AND role = ? AND id <> ?", projectID, authz.ProjectRoleOwner, memberID).
		Count(&count).Error
	return count > 0
}
func (s *stubProjectAIHost) RequestUsesBearerToken(*gin.Context) bool { return false }
func (s *stubProjectAIHost) CurrentAccessTokenFromContext(*gin.Context) (model.AccessToken, bool) {
	return model.AccessToken{}, false
}
func (s *stubProjectAIHost) CurrentSessionFromCookie(*gin.Context) (model.UserSession, bool) {
	return model.UserSession{}, false
}
func (s *stubProjectAIHost) ContinuousAuthorizationInterval() time.Duration { return time.Minute }
func (s *stubProjectAIHost) ObserveGatewayRouteStatuses(_ context.Context, routes []model.GatewayRoute) []model.GatewayRoute {
	return routes
}
func (s *stubProjectAIHost) ObserveGatewayRouteDiagnostics(_ context.Context, routes []model.GatewayRoute) []model.GatewayRoute {
	return routes
}
func (s *stubProjectAIHost) VolumeTransferEnabled() bool { return true }

func newTestToolCtx(db *gorm.DB, user model.User, convProjectID string, locked any) *aitool.ToolContext {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools", nil)
	var lockedRaw json.RawMessage
	if locked != nil {
		lockedRaw, _ = json.Marshal(locked)
	}
	return aitool.NewToolContext(c, db, user, "ses_test", "conv_test", convProjectID, "run_test", "tc_test", "idem_test", "model", lockedRaw)
}

func TestProjectAIToolsCatalogRegistrationAndSchemas(t *testing.T) {
	reg := aitool.NewRegistry()
	New(&stubProjectAIHost{}).RegisterAITools(reg)

	if reg.Len() != 21 {
		t.Fatalf("expected 21 registered tools in projectapi, got %d", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	type expectedSpec struct {
		domain     string
		tier       string
		effect     string
		globalOnly bool
	}
	expected := map[string]expectedSpec{
		"runtime_list_hooks":                      {aitool.DomainRuntime, aitool.ApprovalTierL0, aitool.EffectRead, false},
		"runtime_list_hook_runs":                  {aitool.DomainRuntime, aitool.ApprovalTierL0, aitool.EffectRead, false},
		"runtime_save_hook":                       {aitool.DomainRuntime, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"runtime_delete_hook":                     {aitool.DomainRuntime, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_list_projects":                 {aitool.DomainWorkspace, aitool.ApprovalTierL0, aitool.EffectRead, false},
		"workspace_get_project":                   {aitool.DomainWorkspace, aitool.ApprovalTierL0, aitool.EffectRead, false},
		"workspace_list_members":                  {aitool.DomainWorkspace, aitool.ApprovalTierL0, aitool.EffectRead, false},
		"workspace_get_network_policy":            {aitool.DomainWorkspace, aitool.ApprovalTierL0, aitool.EffectRead, false},
		"workspace_list_network_shares":           {aitool.DomainWorkspace, aitool.ApprovalTierL0, aitool.EffectRead, false},
		"workspace_create_project":                {aitool.DomainWorkspace, aitool.ApprovalTierL1, aitool.EffectWrite, true},
		"workspace_update_project":                {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_pin_project":                   {aitool.DomainWorkspace, aitool.ApprovalTierL1, aitool.EffectWrite, false},
		"workspace_reorder_my_dashboard_projects": {aitool.DomainWorkspace, aitool.ApprovalTierL1, aitool.EffectWrite, true},
		"workspace_request_billing_transfer":      {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_delete_project":                {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectAsync, false},
		"workspace_save_member":                   {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_remove_member":                 {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_configure_network_policy":      {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_create_network_share":          {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_accept_network_share":          {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
		"workspace_delete_network_share":          {aitool.DomainWorkspace, aitool.ApprovalTierL2, aitool.EffectWrite, false},
	}

	for name, want := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != want.domain || cat.ApprovalTier != want.tier || cat.Effect != want.effect || cat.GlobalSessionOnly != want.globalOnly || cat.AdminOnly {
			t.Fatalf("tool %s metadata mismatch: got domain=%s tier=%s effect=%s globalOnly=%v adminOnly=%v",
				name, cat.Domain, cat.ApprovalTier, cat.Effect, cat.GlobalSessionOnly, cat.AdminOnly)
		}
		for _, sc := range cat.RequiredScopes {
			if authz.NormalizeOAuthScope(sc) != sc || !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleUser, sc) {
				t.Fatalf("tool %s has unauthorized tenant scope %q", name, sc)
			}
		}
		if props, ok := cat.InputSchema["properties"].(map[string]any); ok {
			if ps, hasPS := props["pageSize"].(map[string]any); hasPS {
				if ps["default"] != int64(20) || ps["maximum"] != int64(50) {
					t.Fatalf("tool %s pageSize bounds mismatch: %#v", name, ps)
				}
			}
		}
	}
}

func TestProjectAIToolsExecutionAndPreviewFlows(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.ProjectPin{},
				&model.ProjectHookConfig{},
				&model.HookRun{},
				&model.HookRunLog{},
				&model.DeploymentTargetHookBinding{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.BuildRun{},
				&model.Release{},
				&model.GatewayRoute{},
				&model.RepositoryBinding{},
				&model.ProjectVolume{},
				&model.RuntimeCluster{},
				&model.NetworkEgressException{},
				&model.NetworkEgressExceptionGrant{},
				&model.NetworkServiceShare{},
				&model.ScopedResourceProjectBinding{},
				&model.InboxMessage{},
				&model.InboxActionRequest{},
				&model.PlatformEvent{},
				&model.AuditLog{},
			)
		},
	})

	owner := model.User{ID: "usr_owner", Email: "owner@example.com", Name: "Owner", Role: authz.PlatformRoleUser}
	dev := model.User{ID: "usr_dev", Email: "dev@example.com", Name: "Dev", Role: authz.PlatformRoleUser}
	cand := model.User{ID: "usr_cand", Email: "cand@example.com", Name: "Candidate", Role: authz.PlatformRoleUser}
	if err := db.Create(&[]model.User{owner, dev, cand}).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}

	host := &stubProjectAIHost{db: db, user: owner}
	h := New(host)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	mustExec := func(user model.User, convProj string, toolName string, payload any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(payload)
		tool, ok := reg.Get(toolName)
		if !ok {
			t.Fatalf("tool %s missing", toolName)
		}
		var locked any
		if tool.HasPreview {
			ptc := newTestToolCtx(db, user, convProj, nil)
			pr, err := tool.Preview(ptc, raw)
			if err != nil {
				t.Fatalf("%s preview failed: %v", toolName, err)
			}
			locked = pr.LockedPayload
		}
		tc := newTestToolCtx(db, user, convProj, locked)
		res, err := tool.Execute(tc, raw)
		if err != nil {
			t.Fatalf("%s execute failed: %v", toolName, err)
		}
		encoded, _ := json.Marshal(res)
		var m map[string]any
		_ = json.Unmarshal(encoded, &m)
		return m
	}

	// 1. Create two projects via workspace_create_project
	prj1 := mustExec(owner, "", "workspace_create_project", map[string]any{
		"identifier": "alpha-svc",
		"name":       "Alpha Service",
	})
	prj1ID := prj1["projectId"].(string)
	prj2 := mustExec(owner, "", "workspace_create_project", map[string]any{
		"identifier": "beta-svc",
		"name":       "Beta Service",
	})
	prj2ID := prj2["projectId"].(string)

	// 2. Patch project preserving omitted fields + clearDescription
	mustExec(owner, "", "workspace_update_project", map[string]any{
		"project":             "alpha-svc",
		"description":         "Initial description",
		"maxConcurrentBuilds": 4,
	})
	patchedPrj := mustExec(owner, "", "workspace_update_project", map[string]any{
		"project":          prj1ID,
		"clearDescription": true,
	})
	if patchedPrj["description"] != "" || patchedPrj["maxConcurrentBuilds"] != float64(4) {
		t.Fatalf("expected clearDescription=true to clear description while keeping maxConcurrentBuilds=4, got %#v", patchedPrj)
	}

	// 3. Pin & Reorder dashboard projects + verify workspace_get_project does not mutate use_count
	mustExec(owner, "", "workspace_pin_project", map[string]any{"project": "alpha-svc", "pinned": true})
	reorderOut := mustExec(owner, "", "workspace_reorder_my_dashboard_projects", map[string]any{
		"projects": []string{"beta-svc", "alpha-svc"},
	})
	orderedIDs, _ := reorderOut["projectIds"].([]any)
	if len(orderedIDs) != 2 || orderedIDs[0] != prj2ID || orderedIDs[1] != prj1ID {
		t.Fatalf("unexpected projectIds order: %#v", reorderOut)
	}

	listOut := mustExec(owner, "", "workspace_list_projects", map[string]any{
		"sortBy":    "dashboardOrder",
		"sortOrder": "asc",
	})
	items := listOut["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["id"] != prj2ID || items[1].(map[string]any)["pinned"] != true {
		t.Fatalf("unexpected dashboardOrder list: %#v", items)
	}

	getTC := newTestToolCtx(db, owner, "", nil)
	getTool, _ := reg.Get("workspace_get_project")
	if _, err := getTool.Execute(getTC, json.RawMessage(`{"project":"alpha-svc","includeDiagnostics":true}`)); err != nil {
		t.Fatalf("workspace_get_project failed: %v", err)
	}
	if getTC.ResponseHeaders.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control: no-store on workspace_get_project")
	}
	var memberRow model.ProjectMember
	if err := db.First(&memberRow, "project_id = ? AND user_id = ?", prj1ID, owner.ID).Error; err != nil || memberRow.UseCount != 0 {
		t.Fatalf("expected workspace_get_project NOT to increment use_count, got %#v (%v)", memberRow, err)
	}

	// 4. Member management + strict role enum + billing transfer
	saveMemberTool, _ := reg.Get("workspace_save_member")
	if _, err := saveMemberTool.Preview(newTestToolCtx(db, owner, "", nil), json.RawMessage(`{"project":"alpha-svc","user":"dev@example.com","role":"bogus"}`)); err == nil {
		t.Fatalf("expected invalid role enum to be rejected")
	}
	addedMember := mustExec(owner, "", "workspace_save_member", map[string]any{
		"project": "alpha-svc",
		"user":    "dev@example.com",
		"role":    "developer",
	})
	if addedMember["userId"] != dev.ID || addedMember["projectId"] != prj1ID {
		t.Fatalf("unexpected added member: %#v", addedMember)
	}

	candOut := mustExec(owner, "", "workspace_list_members", map[string]any{
		"project":         "alpha-svc",
		"candidateSearch": "cand",
	})
	if len(candOut["candidates"].([]any)) != 1 {
		t.Fatalf("expected 1 candidate user, got %#v", candOut["candidates"])
	}

	transferOut := mustExec(owner, "", "workspace_request_billing_transfer", map[string]any{
		"project":    "alpha-svc",
		"targetUser": "dev@example.com",
	})
	if transferOut["recipientUserId"] != dev.ID {
		t.Fatalf("unexpected billing transfer response: %#v", transferOut)
	}
	prjWithPending := mustExec(owner, "", "workspace_get_project", map[string]any{"project": "alpha-svc"})
	if prjWithPending["pendingBillingTransfer"] == nil {
		t.Fatalf("expected pendingBillingTransfer in workspace_get_project, got %#v", prjWithPending)
	}

	removedMember := mustExec(owner, "", "workspace_remove_member", map[string]any{
		"project": "alpha-svc",
		"member":  "dev@example.com",
	})
	if removedMember["userId"] != dev.ID || removedMember["removed"] != true || removedMember["deleted"] != true || removedMember["projectId"] != prj1ID {
		t.Fatalf("unexpected workspace_remove_member response: %#v", removedMember)
	}

	// 5. Network policy R7 port preservation + warning when egressMode != allowlist
	npOut := mustExec(owner, "", "workspace_configure_network_policy", map[string]any{
		"project": "alpha-svc",
		"upsertAllowlistEntries": []map[string]any{
			{"cidr": "1.1.1.0/24", "ports": []int{443, 8443}},
		},
	})
	if npOut["warnings"] == nil {
		t.Fatalf("expected warnings when updating allowlist while egressMode=internet, got %#v", npOut)
	}
	// Upsert same CIDR omitting ports -> must preserve 443,8443!
	npOut2 := mustExec(owner, "", "workspace_configure_network_policy", map[string]any{
		"project":    "alpha-svc",
		"egressMode": "allowlist",
		"upsertAllowlistEntries": []map[string]any{
			{"cidr": "1.1.1.5/24"},
		},
	})
	if npOut2["egressAllowlist"] != "1.1.1.0/24 443,8443" {
		t.Fatalf("R7 violation: expected existing ports 443,8443 preserved when ports omitted, got %q", npOut2["egressAllowlist"])
	}

	// 6. Cross-project network shares
	cluster := model.RuntimeCluster{ID: "clu_shared", Name: "shared", Scope: "global", IsDefault: true, DeleteStatus: "active"}
	app := model.Application{ID: "app_a", ProjectID: prj1ID, Name: "API", Identifier: "api", DeleteStatus: "active"}
	target := model.DeploymentTarget{ID: "dplt_a", ProjectID: prj1ID, ApplicationID: app.ID, Name: "prod", Stage: "prod", KubernetesName: "api-prod", ClusterID: cluster.ID, DeleteStatus: "active"}
	_ = db.Create(&cluster).Error
	_ = db.Create(&app).Error
	_ = db.Create(&target).Error

	shareCreated := mustExec(owner, "", "workspace_create_network_share", map[string]any{
		"project":                   "alpha-svc",
		"target":                    "prod",
		"consumerProjectIdentifier": "beta-svc",
	})
	shareID := shareCreated["id"].(string)

	shareAccepted := mustExec(owner, "", "workspace_accept_network_share", map[string]any{
		"project": "beta-svc",
		"shareId": shareID,
	})
	if shareAccepted["status"] != NetworkShareStatusActive || shareAccepted["projectId"] != prj2ID {
		t.Fatalf("unexpected accepted share: %#v", shareAccepted)
	}

	shareDeleted := mustExec(owner, "", "workspace_delete_network_share", map[string]any{
		"project": "alpha-svc",
		"shareId": shareID,
	})
	if shareDeleted["deleted"] != true || shareDeleted["direction"] != "provided" || shareDeleted["projectId"] != prj1ID {
		t.Fatalf("unexpected deleted share: %#v", shareDeleted)
	}

	// 7. Runtime hooks lifecycle
	longScript := "echo hello\n" + string(make([]byte, 600))
	for i := range longScript {
		if longScript[i] == 0 {
			longScript = longScript[:i] + "x" + longScript[i+1:]
		}
	}
	hookCreated := mustExec(owner, "", "runtime_save_hook", map[string]any{
		"project":        "alpha-svc",
		"name":           "migrate-db",
		"script":         longScript,
		"shell":          "bash",
		"timeoutSeconds": 120,
	})
	hookID := hookCreated["id"].(string)
	_ = db.Create(&model.DeploymentTargetHookBinding{ID: "dthb_1", ProjectID: prj1ID, ApplicationID: app.ID, TargetID: target.ID, Phase: "preDeployment", HookConfigID: hookID}).Error

	// Patch hook name only -> preserves script, shell, timeoutSeconds
	hookPatched := mustExec(owner, "", "runtime_save_hook", map[string]any{
		"project": "alpha-svc",
		"hook":    "migrate-db",
		"name":    "migrate-db-v2",
	})
	if hookPatched["shell"] != "bash" || hookPatched["timeoutSeconds"] != float64(120) || hookPatched["script"] != longScript {
		t.Fatalf("hook patch wiped existing fields: %#v", hookPatched)
	}

	hooksList := mustExec(owner, "", "runtime_list_hooks", map[string]any{"project": "alpha-svc"})
	firstHookItem := hooksList["items"].([]any)[0].(map[string]any)
	if _, hasRawScript := firstHookItem["script"]; hasRawScript || firstHookItem["scriptTruncated"] != true {
		t.Fatalf("expected list mode to omit raw script and truncate preview: %#v", firstHookItem)
	}
	hookDetail := mustExec(owner, "", "runtime_list_hooks", map[string]any{"project": "alpha-svc", "hook": hookID})
	if hookDetail["script"] != longScript {
		t.Fatalf("expected detail mode to return full script")
	}

	_ = db.Create(&model.HookRun{ID: "hrun_1", ProjectID: prj1ID, HookConfigID: hookID, Name: "migrate-db-v2", Phase: "preDeployment", Status: "failed", ScriptSnapshot: "echo snap"}).Error
	_ = db.Create(&model.HookRunLog{ID: "hrlog_1", HookRunID: "hrun_1", ProjectID: prj1ID, Content: "line1\nline2\nfailed"}).Error

	runsList := mustExec(owner, "", "runtime_list_hook_runs", map[string]any{"project": "alpha-svc", "status": "failed"})
	if runsList["total"] != float64(1) {
		t.Fatalf("expected 1 failed hook run, got %#v", runsList)
	}
	runDetail := mustExec(owner, "", "runtime_list_hook_runs", map[string]any{"project": "alpha-svc", "hookRunId": "hrun_1", "logTailBytes": 6})
	if runDetail["log"] != "failed" || runDetail["logTruncated"] != true {
		t.Fatalf("expected UTF-8 tail truncated log='failed', got %#v", runDetail)
	}

	hookDel := mustExec(owner, "", "runtime_delete_hook", map[string]any{"project": "alpha-svc", "hook": hookID})
	if hookDel["deleted"] != true || hookDel["projectId"] != prj1ID {
		t.Fatalf("unexpected hook delete response: %#v", hookDel)
	}
	var remainingBindings int64
	_ = db.Model(&model.DeploymentTargetHookBinding{}).Where("hook_config_id = ?", hookID).Count(&remainingBindings).Error
	if remainingBindings != 0 {
		t.Fatalf("expected DeploymentTargetHookBinding rows to be cleaned up, got %d", remainingBindings)
	}

	// 8. Async project deletion
	delOut := mustExec(owner, "", "workspace_delete_project", map[string]any{"project": "beta-svc"})
	if delOut["deleteStatus"] != "deleting" || delOut["projectId"] != prj2ID || !slices.Contains(host.cleanupQueued, prj2ID) {
		t.Fatalf("unexpected workspace_delete_project result: %#v", delOut)
	}
}

func TestProjectAIToolsPurePatchAndValidationSemantics(t *testing.T) {
	// 1. Hook patch semantics & strict enum/bounds validation
	existingHook := model.ProjectHookConfig{
		ID:             "hook_1",
		ProjectID:      "prj_1",
		Name:           "orig-hook",
		Script:         "echo original",
		Shell:          "bash",
		TimeoutSeconds: 180,
		FailurePolicy:  "ignore",
	}
	newName := "renamed-hook"
	patchedHook, _, err := prepareHookSaveDraft(&existingHook, "usr_1", "prj_1", runtimeSaveHookInput{
		Project: "prj_1",
		Name:    &newName,
	})
	if err != nil {
		t.Fatalf("prepareHookSaveDraft failed: %v", err)
	}
	if patchedHook.Name != "renamed-hook" || patchedHook.Script != "echo original" || patchedHook.Shell != "bash" || patchedHook.TimeoutSeconds != 180 || patchedHook.FailurePolicy != "ignore" {
		t.Fatalf("omitted hook fields were not preserved: %#v", patchedHook)
	}

	badShell := "zsh"
	if _, _, err := prepareHookSaveDraft(&existingHook, "usr_1", "prj_1", runtimeSaveHookInput{Shell: &badShell}); err == nil {
		t.Fatalf("expected invalid shell zsh to be rejected instead of silently downgraded")
	}
	badTimeout := 9999
	if _, _, err := prepareHookSaveDraft(&existingHook, "usr_1", "prj_1", runtimeSaveHookInput{TimeoutSeconds: &badTimeout}); err == nil {
		t.Fatalf("expected timeoutSeconds > 3600 to be rejected")
	}

	// 2. Project patch semantics (omitted/empty description No-Op vs clearDescription: true)
	existingPrj := model.Project{
		ID:                  "prj_1",
		Identifier:          "alpha",
		Name:                "Alpha",
		Description:         "Keep me",
		MaxConcurrentBuilds: 5,
		WebConsoleEnabled:   false,
	}
	emptyStr := ""
	patchedPrj, _, _, err := prepareProjectUpdateDraft(existingPrj, workspaceUpdateProjectInput{
		Description: &emptyStr,
	})
	if err != nil || patchedPrj.Description != "Keep me" || patchedPrj.MaxConcurrentBuilds != 5 || patchedPrj.WebConsoleEnabled != false {
		t.Fatalf("expected empty description to act as No-Op preserving existing fields, got %#v (%v)", patchedPrj, err)
	}
	clearTrue := true
	clearedPrj, _, _, err := prepareProjectUpdateDraft(existingPrj, workspaceUpdateProjectInput{
		ClearDescription: &clearTrue,
	})
	if err != nil || clearedPrj.Description != "" {
		t.Fatalf("expected clearDescription=true to clear description, got %#v (%v)", clearedPrj, err)
	}

	// 3. Network policy R7 port preservation on existing CIDR + allPorts widening + warning on non-allowlist egressMode
	existingPolicyPrj := model.Project{
		ID:                     "prj_1",
		NetworkIngressMode:     "isolated",
		NetworkEgressMode:      "internet",
		NetworkEgressAllowlist: "1.1.1.0/24 443,8443",
	}
	_, egressMode, allowlistRaw, _, warnings, err := prepareNetworkPolicyPatch(existingPolicyPrj, workspaceConfigureNetworkPolicyInput{
		UpsertAllowlistEntries: []networkAllowlistEntryInput{
			{CIDR: "1.1.1.9/24"}, // omits ports -> must keep 443,8443!
		},
	})
	if err != nil || allowlistRaw != "1.1.1.0/24 443,8443" || egressMode != "internet" || len(warnings) == 0 {
		t.Fatalf("expected R7 port preservation and egressMode!=allowlist warning, got allowlist=%q warnings=%v err=%v", allowlistRaw, warnings, err)
	}

	allPortsTrue := true
	_, _, widenedRaw, _, _, err := prepareNetworkPolicyPatch(existingPolicyPrj, workspaceConfigureNetworkPolicyInput{
		UpsertAllowlistEntries: []networkAllowlistEntryInput{
			{CIDR: "1.1.1.0/24", AllPorts: &allPortsTrue},
		},
	})
	if err != nil || widenedRaw != "1.1.1.0/24" {
		t.Fatalf("expected allPorts=true to widen ports, got %q (%v)", widenedRaw, err)
	}

	// 4. DashboardOrder SQL clause places positive orders before default 0
	clause := projectListOrderClause("dashboardOrder", "asc")
	if clause != "case when coalesce(project_members.dashboard_order, 0) > 0 then 0 else 1 end asc, project_members.dashboard_order asc, projects.created_at desc, projects.id asc" {
		t.Fatalf("unexpected dashboardOrder clause: %s", clause)
	}
}
