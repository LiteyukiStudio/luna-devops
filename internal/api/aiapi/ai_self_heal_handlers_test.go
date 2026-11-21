package aiapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type selfHealPostgresHost struct {
	*stubAIToolHost
	pgDB        *gorm.DB
	currentUser model.User
	hasSession  bool
	audits      []string
}

func (h *selfHealPostgresHost) DBFor(ctx *gin.Context) *gorm.DB {
	if ctx != nil && ctx.Request != nil {
		return h.pgDB.WithContext(ctx.Request.Context())
	}
	return h.pgDB
}

func (h *selfHealPostgresHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.pgDB.WithContext(ctx)
}

func (h *selfHealPostgresHost) CurrentUser(*gin.Context) (model.User, bool) {
	return h.currentUser, true
}

func (h *selfHealPostgresHost) CurrentSessionFromCookie(*gin.Context) (model.UserSession, bool) {
	if !h.hasSession {
		return model.UserSession{}, false
	}
	return model.UserSession{ID: "ses_" + h.currentUser.ID, UserID: h.currentUser.ID}, true
}

func (h *selfHealPostgresHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, _ authz.Action) (model.User, model.Project, bool) {
	var p model.Project
	if err := h.pgDB.First(&p, "id = ?", projectID).Error; err != nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return model.User{}, model.Project{}, false
	}
	if !authz.IsPlatformAdmin(h.currentUser.Role) {
		var cnt int64
		h.pgDB.Model(&model.ProjectMember{}).Where("project_id = ? AND user_id = ?", projectID, h.currentUser.ID).Count(&cnt)
		if cnt == 0 {
			ctx.AbortWithStatus(http.StatusForbidden)
			return model.User{}, model.Project{}, false
		}
	}
	return h.currentUser, p, true
}

func (h *selfHealPostgresHost) AuditWithContext(userID, action, resource string, success bool, _ string, _ context.Context) {
	if success {
		h.audits = append(h.audits, userID+":"+action+":"+resource)
	}
}

func TestRuntimeSelfHealCrossUserPreviewLockDriftAndAutoHealPostgres(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_ai_self_heal_api",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})

	if err := db.Create(&model.AppConfig{Key: "ai.assistant.enabled", Value: "true"}).Error; err != nil {
		t.Fatal(err)
	}

	owner := model.User{ID: "usr_owner", Email: "owner@example.test", Name: "Owner", Role: authz.PlatformRoleUser}
	dev := model.User{ID: "usr_dev", Email: "dev@example.test", Name: "Dev", Role: authz.PlatformRoleUser}
	viewer := model.User{ID: "usr_viewer", Email: "viewer@example.test", Name: "Viewer", Role: authz.PlatformRoleUser}
	for _, u := range []model.User{owner, dev, viewer} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}

	project := model.Project{ID: "prj_sh", Name: "SelfHeal", Identifier: "self-heal", DeleteStatus: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	for _, m := range []model.ProjectMember{
		{ID: "pm_1", ProjectID: project.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_2", ProjectID: project.ID, UserID: dev.ID, Role: authz.ProjectRoleDeveloper},
		{ID: "pm_3", ProjectID: project.ID, UserID: viewer.ID, Role: authz.ProjectRoleViewer},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}

	app := model.Application{ID: "app_sh", ProjectID: project.ID, Name: "API", Identifier: "api", DeleteStatus: "active"}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	target := model.DeploymentTarget{
		ID:            "dplt_sh_prod",
		ProjectID:     project.ID,
		ApplicationID: app.ID,
		Name:          "prod",
		Stage:         "prod",
		WorkloadType:  "Deployment",
		Replicas:      2,
		Enabled:       true,
		DeleteStatus:  "active",
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	var savedTarget model.DeploymentTarget
	if err := db.First(&savedTarget, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}

	baseRel := model.Release{
		ID:                 "rel_sh_1",
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Revision:           1,
		Status:             "succeeded",
		ImageRef:           "registry.example.test/api:v1",
		CreatedAt:          time.Now().UTC().Add(-10 * time.Minute),
	}
	if err := db.Create(&baseRel).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	incident := model.RuntimeAlertIncident{
		ID:                    "rai_sh_1",
		ProjectID:             project.ID,
		ApplicationID:         app.ID,
		DeploymentTargetID:    target.ID,
		ResourceID:            target.ID,
		IncidentKind:          "runtime.pod_oom_killed",
		Status:                "active",
		Fingerprint:           "oom:api",
		ConsecutiveFailures:   2,
		FirstDetectedAt:       now,
		LastObservedAt:        now,
		SummaryJSON:           "{}",
		RuleNotificationsJSON: "{}",
		InboxCoalesceKey:      "runtime_incident:rai_sh_1",
	}
	if err := db.Create(&incident).Error; err != nil {
		t.Fatal(err)
	}

	reg := aitool.NewRegistry()
	var executedUsers []string
	aitool.Register(reg, aitool.Spec[struct {
		Project     string `json:"project"`
		Application string `json:"application"`
		Target      string `json:"target"`
	}, map[string]any]{
		Name:           "delivery_restart_target",
		Domain:         aitool.DomainDelivery,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionDeploymentRestart)},
		Verification:   []aitool.OperationVerification{{OperationID: "delivery_restart_target", Family: "resource"}},
		Preview: func(_ *aitool.ToolContext, in struct {
			Project     string `json:"project"`
			Application string `json:"application"`
			Target      string `json:"target"`
		}) (*aitool.PreviewResult, error) {
			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        "Rolling restart " + in.Target,
					TargetResource: in.Target,
					Changes:        []aitool.PreviewChange{{Field: "restartedAt", Before: "none", After: "now"}},
				},
				LockedPayload: map[string]string{"target": in.Target},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in struct {
			Project     string `json:"project"`
			Application string `json:"application"`
			Target      string `json:"target"`
		}) (map[string]any, error) {
			executedUsers = append(executedUsers, tc.User.ID)
			return map[string]any{"restarted": in.Target}, nil
		},
	})
	aitool.Register(reg, aitool.Spec[struct {
		Project     string `json:"project"`
		Application string `json:"application"`
		Target      string `json:"target"`
		Replicas    int    `json:"replicas"`
	}, map[string]any]{
		Name:           "delivery_scale_target",
		Domain:         aitool.DomainDelivery,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionDeploymentUpdate)},
		Verification:   []aitool.OperationVerification{{OperationID: "delivery_scale_target", Family: "resource"}},
		Preview: func(_ *aitool.ToolContext, _ struct {
			Project     string `json:"project"`
			Application string `json:"application"`
			Target      string `json:"target"`
			Replicas    int    `json:"replicas"`
		}) (*aitool.PreviewResult, error) {
			return &aitool.PreviewResult{}, nil
		},
		Execute: func(_ *aitool.ToolContext, _ struct {
			Project     string `json:"project"`
			Application string `json:"application"`
			Target      string `json:"target"`
			Replicas    int    `json:"replicas"`
		}) (map[string]any, error) {
			return map[string]any{}, nil
		},
	})
	aitool.Register(reg, aitool.Spec[struct {
		Project     string `json:"project"`
		Application string `json:"application"`
		Target      string `json:"target"`
		ImageRef    string `json:"imageRef,omitempty"`
	}, map[string]any]{
		Name:           "delivery_deploy_target",
		Domain:         aitool.DomainDelivery,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionDeploymentRelease)},
		Verification:   []aitool.OperationVerification{{OperationID: "delivery_deploy_target", Family: "release"}},
		Preview: func(_ *aitool.ToolContext, _ struct {
			Project     string `json:"project"`
			Application string `json:"application"`
			Target      string `json:"target"`
			ImageRef    string `json:"imageRef,omitempty"`
		}) (*aitool.PreviewResult, error) {
			return &aitool.PreviewResult{}, nil
		},
		Execute: func(_ *aitool.ToolContext, _ struct {
			Project     string `json:"project"`
			Application string `json:"application"`
			Target      string `json:"target"`
			ImageRef    string `json:"imageRef,omitempty"`
		}) (map[string]any, error) {
			return map[string]any{}, nil
		},
	})

	host := &selfHealPostgresHost{
		stubAIToolHost: &stubAIToolHost{
			db:            db,
			reg:           reg,
			callbackToken: "cb_secret_token",
		},
		pgDB:        db,
		currentUser: owner,
		hasSession:  true,
	}
	handler := New(host)
	router := gin.New()
	router.POST("/api/v1/inbox/action-requests/:requestId/self-heal-preview", handler.PreviewInboxSelfHealAction)
	router.POST("/internal/v1/ai/tools/auto-heal", handler.ExecuteAutoHealInternal)

	inboxSvc := inbox.NewService(db)
	expires := now.Add(24 * time.Hour)
	payload := map[string]any{
		"incidentId":              incident.ID,
		"applicationId":           app.ID,
		"deploymentTargetId":      target.ID,
		"toolName":                "delivery_restart_target",
		"arguments":               map[string]any{"project": project.ID, "application": app.ID, "target": target.ID},
		"expectedSummary":         "Restart prod target",
		"baselineReleaseId":       baseRel.ID,
		"baselineTargetUpdatedAt": savedTarget.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"createdAt":               now.Format(time.RFC3339),
	}
	ownerReq, err := inboxSvc.CreateActionRequest(context.Background(), inbox.CreateActionRequestInput{
		Type:            inbox.ActionRequestTypeRuntimeSelfHeal,
		RequesterUserID: owner.ID,
		RecipientUserID: owner.ID,
		ProjectID:       project.ID,
		ResourceType:    "deployment_target",
		ResourceID:      target.ID,
		Payload:         payload,
		ExpiresAt:       &expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	devReq, err := inboxSvc.CreateActionRequest(context.Background(), inbox.CreateActionRequestInput{
		Type:            inbox.ActionRequestTypeRuntimeSelfHeal,
		RequesterUserID: owner.ID,
		RecipientUserID: dev.ID,
		ProjectID:       project.ID,
		ResourceType:    "deployment_target",
		ResourceID:      target.ID,
		Payload:         payload,
		ExpiresAt:       &expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	viewerReq, err := inboxSvc.CreateActionRequest(context.Background(), inbox.CreateActionRequestInput{
		Type:            inbox.ActionRequestTypeRuntimeSelfHeal,
		RequesterUserID: owner.ID,
		RecipientUserID: viewer.ID,
		ProjectID:       project.ID,
		ResourceType:    "deployment_target",
		ResourceID:      target.ID,
		Payload:         payload,
		ExpiresAt:       &expires,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Viewer cannot preview or execute runtime self-heal -> 403 / ErrSelfHealForbidden!
	host.currentUser = viewer
	viewerPrevRec := httptest.NewRecorder()
	viewerPrevHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/inbox/action-requests/"+viewerReq.ID+"/self-heal-preview", nil)
	router.ServeHTTP(viewerPrevRec, viewerPrevHTTP)
	if viewerPrevRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer preview, got %d %s", viewerPrevRec.Code, viewerPrevRec.Body.String())
	}
	viewerGin, _ := gin.CreateTestContext(httptest.NewRecorder())
	viewerGin.Request = httptest.NewRequest(http.MethodPost, "/decision", nil)
	viewerCtx := inbox.WithDecisionMeta(context.Background(), inbox.DecisionMeta{RequestCarrier: viewerGin})
	if _, err := handler.DecideRuntimeSelfHealInboxAction(viewerCtx, db, viewer, viewerReq, "accept", viewerReq.RowVersion); !errors.Is(err, inbox.ErrSelfHealForbidden) {
		t.Fatalf("expected ErrSelfHealForbidden for viewer decision, got %v", err)
	}

	// 2. Owner previews and gets an owner-bound previewLock; Dev MUST NOT be able to reuse Owner's previewLock!
	host.currentUser = owner
	ownerPrevRec := httptest.NewRecorder()
	ownerPrevHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/inbox/action-requests/"+ownerReq.ID+"/self-heal-preview", nil)
	router.ServeHTTP(ownerPrevRec, ownerPrevHTTP)
	if ownerPrevRec.Code != http.StatusOK {
		t.Fatalf("expected 200 owner preview, got %d %s", ownerPrevRec.Code, ownerPrevRec.Body.String())
	}
	var ownerPrevOut InboxSelfHealPreviewOutput
	if err := json.Unmarshal(ownerPrevRec.Body.Bytes(), &ownerPrevOut); err != nil || ownerPrevOut.PreviewLock == "" {
		t.Fatalf("decode owner preview: %v %+v", err, ownerPrevOut)
	}

	host.currentUser = dev
	devGinCrossLock, _ := gin.CreateTestContext(httptest.NewRecorder())
	devGinCrossLock.Request = httptest.NewRequest(http.MethodPost, "/decision", nil)
	crossLockCtx := inbox.WithDecisionMeta(context.Background(), inbox.DecisionMeta{
		RequestCarrier: devGinCrossLock,
		PreviewLock:    ownerPrevOut.PreviewLock,
	})
	_, crossErr := handler.DecideRuntimeSelfHealInboxAction(crossLockCtx, db, dev, devReq, "accept", devReq.RowVersion)
	if te, ok := aitool.AsToolError(crossErr); !ok || te.Code != "ai.preview_lock_mismatch" {
		t.Fatalf("expected ai.preview_lock_mismatch when Dev reuses Owner's previewLock, got %v", crossErr)
	}

	// 3. Simulate state drift (a newer release rel_sh_2 was created after diagnosis!).
	// Dev clicking Confirm without re-preview MUST get ErrSelfHealStateDrifted!
	newerRel := model.Release{
		ID:                 "rel_sh_2",
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Revision:           2,
		Status:             "succeeded",
		ImageRef:           "registry.example.test/api:v2",
		CreatedAt:          time.Now().UTC(),
	}
	if err := db.Create(&newerRel).Error; err != nil {
		t.Fatal(err)
	}

	devGinNoLock, _ := gin.CreateTestContext(httptest.NewRecorder())
	devGinNoLock.Request = httptest.NewRequest(http.MethodPost, "/decision", nil)
	driftCtx := inbox.WithDecisionMeta(context.Background(), inbox.DecisionMeta{RequestCarrier: devGinNoLock})
	if _, err := handler.DecideRuntimeSelfHealInboxAction(driftCtx, db, dev, devReq, "accept", devReq.RowVersion); !errors.Is(err, inbox.ErrSelfHealStateDrifted) {
		t.Fatalf("expected ErrSelfHealStateDrifted when newer release exists, got %v", err)
	}

	// 4. Dev runs live re-preview under Dev's own identity -> gets stateDrifted=true + Dev-bound previewLock -> confirms successfully!
	devPrevRec := httptest.NewRecorder()
	devPrevHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/inbox/action-requests/"+devReq.ID+"/self-heal-preview", nil)
	router.ServeHTTP(devPrevRec, devPrevHTTP)
	if devPrevRec.Code != http.StatusOK {
		t.Fatalf("expected 200 dev re-preview, got %d %s", devPrevRec.Code, devPrevRec.Body.String())
	}
	var devPrevOut InboxSelfHealPreviewOutput
	if err := json.Unmarshal(devPrevRec.Body.Bytes(), &devPrevOut); err != nil {
		t.Fatal(err)
	}
	if !devPrevOut.StateDrifted || devPrevOut.DriftReason != "newer_release_created" || devPrevOut.PreviewLock == "" {
		t.Fatalf("expected drifted dev preview output, got %+v", devPrevOut)
	}

	devGinConfirm, _ := gin.CreateTestContext(httptest.NewRecorder())
	devGinConfirm.Request = httptest.NewRequest(http.MethodPost, "/decision", nil)
	confirmCtx := inbox.WithDecisionMeta(context.Background(), inbox.DecisionMeta{
		RequestCarrier: devGinConfirm,
		PreviewLock:    devPrevOut.PreviewLock,
	})
	notified, confirmErr := handler.DecideRuntimeSelfHealInboxAction(confirmCtx, db, dev, devReq, "accept", devReq.RowVersion)
	if confirmErr != nil {
		t.Fatalf("expected dev confirmation with fresh previewLock to succeed, got %v", confirmErr)
	}
	if len(executedUsers) != 1 || executedUsers[0] != dev.ID {
		t.Fatalf("expected tool executed once as %s, got %v", dev.ID, executedUsers)
	}
	if len(notified) < 2 {
		t.Fatalf("expected sibling recipients notified, got %v", notified)
	}

	// Sibling Owner request for same incident MUST now be completed so Owner cannot double-execute!
	var refreshedOwnerReq model.InboxActionRequest
	if err := db.First(&refreshedOwnerReq, "id = ?", ownerReq.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshedOwnerReq.Status != inbox.ActionRequestStatusCompleted {
		t.Fatalf("expected sibling owner request marked completed, got %s", refreshedOwnerReq.Status)
	}

	// 5. Internal auto-heal endpoint enforces auto_l1 vs prod/L2 and executes under auto_l2 + AuditLog!
	autoCfg := model.ProjectAIAutomationConfig{
		ProjectID:                   project.ID,
		RuntimeDiagnosisEnabled:     true,
		RuntimeBillingUserID:        owner.ID,
		RuntimeCooldownSeconds:      300,
		RuntimeDailyLimit:           20,
		SelfHealMode:                model.AISelfHealModeAutoL1,
		StageSelfHealModesJSON:      "{}",
		AllowedAutoHealToolsJSON:    `["delivery_restart_target"]`,
		MaxConsecutiveAutoHeals:     2,
		CircuitBreakerWindowSeconds: 1800,
	}
	if err := db.Create(&autoCfg).Error; err != nil {
		t.Fatal(err)
	}

	// auto_l1 on prod target -> 403 ai.self_heal_not_permitted (EffectiveSelfHealModeForStage downgrades prod auto_l1 to suggest_only)
	autoBodyL1 := `{"actorUserId":"usr_owner","projectId":"prj_sh","incidentId":"rai_sh_1","selfHealMode":"auto_l1","toolName":"delivery_restart_target","arguments":{"application":"app_sh","target":"dplt_sh_prod"}}`
	autoRecL1 := httptest.NewRecorder()
	autoReqL1 := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/auto-heal", strings.NewReader(autoBodyL1))
	autoReqL1.Header.Set("Content-Type", "application/json")
	autoReqL1.Header.Set("Authorization", "Bearer cb_secret_token")
	router.ServeHTTP(autoRecL1, autoReqL1)
	if autoRecL1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for auto_l1 on prod L2 tool, got %d %s", autoRecL1.Code, autoRecL1.Body.String())
	}

	// Upgrade project config to auto_l2 -> succeeds + writes AuditLog!
	if err := db.Model(&model.ProjectAIAutomationConfig{}).Where("project_id = ?", project.ID).Update("self_heal_mode", model.AISelfHealModeAutoL2).Error; err != nil {
		t.Fatal(err)
	}
	autoBodyL2 := `{"actorUserId":"usr_owner","projectId":"prj_sh","incidentId":"rai_sh_1","selfHealMode":"auto_l2","toolName":"delivery_restart_target","arguments":{"application":"app_sh","target":"dplt_sh_prod"}}`
	autoRecL2 := httptest.NewRecorder()
	autoReqL2 := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/auto-heal", strings.NewReader(autoBodyL2))
	autoReqL2.Header.Set("Content-Type", "application/json")
	autoReqL2.Header.Set("Authorization", "Bearer cb_secret_token")
	router.ServeHTTP(autoRecL2, autoReqL2)
	if autoRecL2.Code != http.StatusOK {
		t.Fatalf("expected 200 for auto_l2 internal auto-heal, got %d %s", autoRecL2.Code, autoRecL2.Body.String())
	}
	if len(executedUsers) != 2 || executedUsers[1] != owner.ID {
		t.Fatalf("expected auto-heal executed as owner, got %v", executedUsers)
	}
	foundAutoAudit := false
	for _, entry := range host.audits {
		if strings.Contains(entry, "usr_owner:ai.self_heal.auto_execute.delivery_restart_target:prj_sh") {
			foundAutoAudit = true
		}
	}
	if !foundAutoAudit {
		t.Fatalf("expected AuditLog entry for auto-heal, got %v", host.audits)
	}

	// 6. Adversarial checks: cross-target escape, zero-replica scale DoS, arbitrary imageRef RCE, and resolved incident!
	if err := db.Model(&model.ProjectAIAutomationConfig{}).Where("project_id = ?", project.ID).Update("allowed_auto_heal_tools_json", "[]").Error; err != nil {
		t.Fatal(err)
	}

	escapeTargetBody := `{"actorUserId":"usr_owner","projectId":"prj_sh","incidentId":"rai_sh_1","selfHealMode":"auto_l2","toolName":"delivery_restart_target","arguments":{"application":"app_sh","target":"dplt_other"}}`
	escapeRec := httptest.NewRecorder()
	escapeReq := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/auto-heal", strings.NewReader(escapeTargetBody))
	escapeReq.Header.Set("Content-Type", "application/json")
	escapeReq.Header.Set("Authorization", "Bearer cb_secret_token")
	router.ServeHTTP(escapeRec, escapeReq)
	if escapeRec.Code != http.StatusForbidden || !strings.Contains(escapeRec.Body.String(), "ai.project_scope_mismatch") {
		t.Fatalf("expected 403 ai.project_scope_mismatch for cross-target auto-heal, got %d %s", escapeRec.Code, escapeRec.Body.String())
	}

	zeroScaleBody := `{"actorUserId":"usr_owner","projectId":"prj_sh","incidentId":"rai_sh_1","selfHealMode":"auto_l2","toolName":"delivery_scale_target","arguments":{"application":"app_sh","target":"dplt_sh_prod","replicas":0}}`
	zeroScaleRec := httptest.NewRecorder()
	zeroScaleReq := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/auto-heal", strings.NewReader(zeroScaleBody))
	zeroScaleReq.Header.Set("Content-Type", "application/json")
	zeroScaleReq.Header.Set("Authorization", "Bearer cb_secret_token")
	router.ServeHTTP(zeroScaleRec, zeroScaleReq)
	if zeroScaleRec.Code != http.StatusForbidden || !strings.Contains(zeroScaleRec.Body.String(), "ai.self_heal_not_permitted") {
		t.Fatalf("expected 403 ai.self_heal_not_permitted for replicas=0 scale, got %d %s", zeroScaleRec.Code, zeroScaleRec.Body.String())
	}

	injectImgBody := `{"actorUserId":"usr_owner","projectId":"prj_sh","incidentId":"rai_sh_1","selfHealMode":"auto_l2","toolName":"delivery_deploy_target","arguments":{"application":"app_sh","target":"dplt_sh_prod","imageRef":"docker.io/attacker/exfil:latest"}}`
	injectImgRec := httptest.NewRecorder()
	injectImgReq := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/auto-heal", strings.NewReader(injectImgBody))
	injectImgReq.Header.Set("Content-Type", "application/json")
	injectImgReq.Header.Set("Authorization", "Bearer cb_secret_token")
	router.ServeHTTP(injectImgRec, injectImgReq)
	if injectImgRec.Code != http.StatusForbidden || !strings.Contains(injectImgRec.Body.String(), "ai.self_heal_not_permitted") {
		t.Fatalf("expected 403 ai.self_heal_not_permitted for arbitrary imageRef override, got %d %s", injectImgRec.Code, injectImgRec.Body.String())
	}

	// Mark incident resolved -> auto-heal MUST reject with 409 ai.incident_not_active!
	if err := db.Model(&model.RuntimeAlertIncident{}).Where("id = ?", incident.ID).Update("status", "resolved").Error; err != nil {
		t.Fatal(err)
	}
	resolvedRec := httptest.NewRecorder()
	resolvedReq := httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/auto-heal", strings.NewReader(autoBodyL2))
	resolvedReq.Header.Set("Content-Type", "application/json")
	resolvedReq.Header.Set("Authorization", "Bearer cb_secret_token")
	router.ServeHTTP(resolvedRec, resolvedReq)
	if resolvedRec.Code != http.StatusConflict || !strings.Contains(resolvedRec.Body.String(), "ai.incident_not_active") {
		t.Fatalf("expected 409 ai.incident_not_active for resolved incident, got %d %s", resolvedRec.Code, resolvedRec.Body.String())
	}
}

func TestValidateSafeRuntimeHealArgumentsAutoScaling(t *testing.T) {
	validArgs := map[string]any{
		"project":       "prj_1",
		"application":   "app_1",
		"target":        "dplt_1",
		"cpuRequest":    "500m",
		"memoryRequest": "1Gi",
		"autoScaling": map[string]any{
			"enabled":     true,
			"minReplicas": float64(2),
			"maxReplicas": float64(6),
		},
	}
	if err := validateSafeRuntimeHealArguments("delivery_update_target_runtime", validArgs); err != nil {
		t.Fatalf("expected valid autoScaling + cpuRequest/memoryRequest to pass, got %v", err)
	}

	zeroMinArgs := map[string]any{
		"project":     "prj_1",
		"application": "app_1",
		"target":      "dplt_1",
		"autoScaling": map[string]any{
			"minReplicas": float64(0),
		},
	}
	if err := validateSafeRuntimeHealArguments("delivery_update_target_runtime", zeroMinArgs); err == nil {
		t.Fatalf("expected minReplicas=0 to be rejected")
	}

	forbiddenFieldArgs := map[string]any{
		"project":     "prj_1",
		"application": "app_1",
		"target":      "dplt_1",
		"imageRef":    "docker.io/evil/img:latest",
	}
	if err := validateSafeRuntimeHealArguments("delivery_update_target_runtime", forbiddenFieldArgs); err == nil {
		t.Fatalf("expected imageRef on delivery_update_target_runtime to be rejected")
	}
}
