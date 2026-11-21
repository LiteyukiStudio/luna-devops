package projectapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
