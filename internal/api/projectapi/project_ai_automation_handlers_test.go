package projectapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type aiAutomationTestHost struct {
	*stubProjectAIHost
	hasSession bool
	audits     int
}

func (h *aiAutomationTestHost) CurrentSessionFromCookie(*gin.Context) (model.UserSession, bool) {
	if !h.hasSession {
		return model.UserSession{}, false
	}
	return model.UserSession{ID: "ses_" + h.user.ID, UserID: h.user.ID}, true
}

func (h *aiAutomationTestHost) AuditWithContext(string, string, string, bool, string, context.Context) {
	h.audits++
}

func TestProjectAIAutomationConfigWalletConsentAndSessionGuardsPostgres(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_proj_ai_auto_cfg",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})

	alice := model.User{ID: "usr_alice", Email: "alice@example.test", Name: "Alice", Role: authz.PlatformRoleUser}
	bob := model.User{ID: "usr_bob", Email: "bob@example.test", Name: "Bob", Role: authz.PlatformRoleUser}
	for _, u := range []model.User{alice, bob} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	proj := model.Project{ID: "prj_auto", Name: "Auto", Identifier: "auto", DeleteStatus: "active"}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	for _, u := range []model.User{alice, bob} {
		if err := db.Create(&model.ProjectMember{ID: "pm_" + u.ID, ProjectID: proj.ID, UserID: u.ID, Role: authz.ProjectRoleOwner}).Error; err != nil {
			t.Fatal(err)
		}
	}

	host := &aiAutomationTestHost{
		stubProjectAIHost: &stubProjectAIHost{db: db, user: alice},
		hasSession:        false,
	}
	handler := New(host)
	router := gin.New()
	router.GET("/api/v1/projects/:projectId/ai-automation-config", handler.GetProjectAIAutomationConfig)
	router.PUT("/api/v1/projects/:projectId/ai-automation-config", handler.UpdateProjectAIAutomationConfig)

	// 1. Binding personal wallet without browser session (e.g., PAT) -> 401 auth.session.missing
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/prj_auto/ai-automation-config", strings.NewReader(`{"deliveryDiagnosisEnabled":true,"deliveryDailyLimit":10,"deliveryCooldownSeconds":600}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "auth.session.missing") {
		t.Fatalf("expected 401 auth.session.missing without cookie session, got %d %s", rec.Code, rec.Body.String())
	}

	// 2. Alice binds her own wallet via browser session -> 200 OK + AuditLog
	host.hasSession = true
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/projects/prj_auto/ai-automation-config", strings.NewReader(`{"deliveryDiagnosisEnabled":true,"deliveryDailyLimit":10,"deliveryCooldownSeconds":600}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || host.audits != 1 {
		t.Fatalf("expected 200 OK + audit when Alice binds her own wallet, got %d %s (audits=%d)", rec.Code, rec.Body.String(), host.audits)
	}

	// 3. Co-manager Bob tries to raise dailyLimit=500 on Alice's bound wallet without rebinding to Bob -> 403 ai_automation.billing_user_must_be_self!
	host.user = bob
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/projects/prj_auto/ai-automation-config", strings.NewReader(`{"deliveryDailyLimit":500}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai_automation.billing_user_must_be_self") {
		t.Fatalf("expected 403 when co-manager escalates spend on Alice's wallet, got %d %s", rec.Code, rec.Body.String())
	}

	// 4. Co-manager Bob CAN disable automatic diagnosis (reducing spend to zero) without taking over Alice's wallet -> 200 OK
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/projects/prj_auto/ai-automation-config", strings.NewReader(`{"deliveryDiagnosisEnabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when co-manager disables automatic diagnosis, got %d %s", rec.Code, rec.Body.String())
	}

	// 5. Co-manager Bob tries to re-enable diagnosis while still pointing at Alice's wallet -> 403!
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/projects/prj_auto/ai-automation-config", strings.NewReader(`{"deliveryDiagnosisEnabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "ai_automation.billing_user_must_be_self") {
		t.Fatalf("expected 403 when co-manager re-enables diagnosis on Alice's wallet, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestListProjectAIDiagnosesPaginationAndFilteringPostgres(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_proj_ai_diag_list",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})

	alice := model.User{ID: "usr_diag_a", Email: "diag@example.test", Name: "Alice", Role: authz.PlatformRoleUser}
	if err := db.Create(&alice).Error; err != nil {
		t.Fatal(err)
	}
	proj := model.Project{ID: "prj_diag_hist", Name: "Hist", Identifier: "hist", DeleteStatus: "active"}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ProjectMember{ID: "pm_diag_a", ProjectID: proj.ID, UserID: alice.ID, Role: authz.ProjectRoleOwner}).Error; err != nil {
		t.Fatal(err)
	}
	app := model.Application{ID: "app_diag_1", ProjectID: proj.ID, Name: "WebApp", Identifier: "webapp", DeleteStatus: "active"}
	if err := db.Create(&app).Error; err != nil {
		t.Fatal(err)
	}
	target := model.DeploymentTarget{ID: "dplt_diag_1", ProjectID: proj.ID, ApplicationID: app.ID, Name: "Production", Stage: "prod", WorkloadType: "Deployment", Enabled: true, DeleteStatus: "active"}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	inc := model.RuntimeAlertIncident{
		ID:                    "rai_diag_1",
		ProjectID:             proj.ID,
		ApplicationID:         app.ID,
		DeploymentTargetID:    target.ID,
		ResourceID:            target.ID,
		IncidentKind:          "runtime.pod_oom_killed",
		Status:                "active",
		Fingerprint:           "oom",
		ConsecutiveFailures:   2,
		FirstDetectedAt:       time.Now().UTC(),
		LastObservedAt:        time.Now().UTC(),
		SummaryJSON:           "{}",
		RuleNotificationsJSON: "{}",
		InboxCoalesceKey:      "runtime_incident:rai_diag_1",
	}
	if err := db.Create(&inc).Error; err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 11, 22, 6, 0, 0, 0, time.UTC)
	if err := db.Create(&model.AIDeliveryDiagnosis{
		ID:                 "aidg_hist_1",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		SubjectType:        model.AIDeliverySubjectBuildRun,
		SubjectID:          "bldr_hist_1",
		BilledUserID:       alice.ID,
		Status:             model.AIDiagnosisStatusSucceeded,
		FailureCode:        "job_failed",
		RootCauseSummary:   "Dockerfile typo nginxx",
		Confidence:         "high",
		SuggestionsJSON:    `["Fix FROM nginx"]`,
		CreatedAt:          base,
		UpdatedAt:          base,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AIRuntimeIncidentDiagnosis{
		ID:                 "airtd_hist_2",
		ProjectID:          proj.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		IncidentID:         inc.ID,
		BilledUserID:       alice.ID,
		Status:             model.AIDiagnosisStatusRunning,
		AnomalyType:        "runtime.pod_oom_killed",
		AutoHealMode:       model.AISelfHealModeAutoL2,
		AutoHealStatus:     model.AIAutoHealStatusNone,
		AutoHealSummary:    "in_progress",
		SuggestionsJSON:    "[]",
		CreatedAt:          base.Add(1 * time.Minute),
		UpdatedAt:          base.Add(1 * time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}

	host := &aiAutomationTestHost{
		stubProjectAIHost: &stubProjectAIHost{db: db, user: alice},
		hasSession:        true,
	}
	handler := New(host)
	router := gin.New()
	router.GET("/api/v1/projects/:projectId/ai-automation-diagnoses", handler.ListProjectAIDiagnoses)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_diag_hist/ai-automation-diagnoses?page=1&pageSize=10", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d %s", rec.Code, rec.Body.String())
	}

	var page struct {
		Items []ProjectAIDiagnosisRecordResponse `json:"items"`
		Total int64                              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("expected 2 items, got total=%d items=%+v", page.Total, page.Items)
	}
	// Newest first (runtime row created at base + 1m)
	if page.Items[0].ID != "airtd_hist_2" || page.Items[0].AutoHealSummary != "" {
		t.Fatalf("expected airtd_hist_2 first with internal 'in_progress' stripped, got %+v", page.Items[0])
	}
	if page.Items[1].ID != "aidg_hist_1" || page.Items[1].ApplicationName != "WebApp" || !strings.Contains(page.Items[1].DeepLink, "#buildRunId=bldr_hist_1") {
		t.Fatalf("unexpected delivery diagnosis record: %+v", page.Items[1])
	}

	// Filter by category=delivery & status=succeeded
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_diag_hist/ai-automation-diagnoses?category=delivery&status=succeeded", nil)
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on filtered list, got %d %s", rec2.Code, rec2.Body.String())
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].ID != "aidg_hist_1" {
		t.Fatalf("expected single delivery record, got %+v", page)
	}
}
