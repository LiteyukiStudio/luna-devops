package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestListBillingRateRulesAllowsAuthenticatedUser(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.BillingRateRule{}); err != nil {
		t.Fatalf("migrate billing rate rules: %v", err)
	}
	user := model.User{
		ID: "usr_billing_reader", Email: "billing-reader@example.com", Name: "Billing reader",
		Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create billing reader: %v", err)
	}
	plainSessionToken := "sess_billing_reader"
	session := model.UserSession{
		ID: "ses_billing_reader", UserID: user.ID, TokenHash: hashToken(plainSessionToken),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create billing reader session: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/billing/rate-rules", nil)
	ctx.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: plainSessionToken})

	(&Handlers{db: db, mode: "production"}).ListBillingRateRules(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var rules []model.BillingRateRule
	if err := json.Unmarshal(recorder.Body.Bytes(), &rules); err != nil {
		t.Fatalf("decode rate rules: %v", err)
	}
	if len(rules) != 9 {
		t.Fatalf("rate rule count = %d, want 9", len(rules))
	}
	if rules[0].Meter != "ai.input_tokens_1000" {
		t.Fatalf("first meter = %q, want sorted meter list", rules[0].Meter)
	}
}

func TestUpdateBillingRateRulesKeepsAdministratorPermission(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/billing/rate-rules",
		strings.NewReader(`{"rules":[{"meter":"storage.gib_day","creditsPerUnit":"2","enabled":true}]}`),
	)
	ctx.Set(currentUserContextKey, model.User{ID: "usr_billing_reader", Role: authz.PlatformRoleUser, Language: "zh-CN"})

	(&Handlers{mode: "production"}).UpdateBillingRateRules(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode forbidden response: %v", err)
	}
	if response["code"] != "config.admin.required" {
		t.Fatalf("error code = %v, want config.admin.required", response["code"])
	}
}
