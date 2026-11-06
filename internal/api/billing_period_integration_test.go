package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type billingPeriodFixture struct {
	handlers *Handlers
	user     model.User
	admin    model.User
	project  model.Project
}

func newBillingPeriodFixture(t *testing.T) billingPeriodFixture {
	t.Helper()
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(
		&model.UserWallet{}, &model.BillingUsageRecord{}, &model.BillingLedgerEntry{},
		&model.Application{}, &model.DeploymentTarget{}, &model.BuildRun{}, &model.GatewayRoute{},
	); err != nil {
		t.Fatalf("migrate billing period models: %v", err)
	}
	fixture := billingPeriodFixture{
		user:  model.User{ID: "usr_billing_period", Email: "billing-period@example.com", Name: "Billing period", Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash"},
		admin: model.User{ID: "usr_billing_admin", Email: "billing-admin@example.com", Name: "Billing admin", Role: authz.PlatformRoleAdmin, Language: "zh-CN", Password: "hash"},
	}
	fixture.project = model.Project{ID: "prj_billing_period", Identifier: "billing-period", Name: "Billing period", BillingOwnerUserID: fixture.user.ID}
	for _, value := range []any{&fixture.user, &fixture.admin, &fixture.project,
		&model.DeploymentTarget{ID: "dplt_billing_period", ProjectID: fixture.project.ID, ApplicationID: "app_billing_period", Name: "web", Stage: "production"},
		&model.UserWallet{ID: "wal_billing_period", UserID: fixture.user.ID, BalanceCredits: decimal.RequireFromString("500")},
		&model.UserWallet{ID: "wal_billing_admin", UserID: fixture.admin.ID, BalanceCredits: decimal.Zero},
	} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("create billing period fixture %T: %v", value, err)
		}
	}
	fixture.handlers = &Handlers{db: db, mode: "production", configs: &configCache{values: map[string]string{}}}
	fixture.handlers.domains = newDomainHandlers(fixture.handlers)
	return fixture
}

// addDebit creates a settled usage record and its debit ledger entry; the
// ledger is posted at settledAt, while usage occurs at periodStart.
func (f billingPeriodFixture) addDebit(t *testing.T, id, resourceType, resourceID, meter, reason string, amount string, periodStart, periodEnd, settledAt time.Time) {
	t.Helper()
	usage := model.BillingUsageRecord{
		ID: "busg_" + id, ProjectID: f.project.ID, BilledUserID: f.user.ID, Meter: meter, Quantity: decimal.NewFromInt(1), Unit: "unit",
		AmountCredits: decimal.RequireFromString(amount), ResourceType: resourceType, ResourceID: resourceID,
		PeriodStart: periodStart, PeriodEnd: periodEnd, Status: "settled", SettledAt: &settledAt, CreatedAt: settledAt,
	}
	ledger := model.BillingLedgerEntry{
		ID: "bled_" + id, UserID: f.user.ID, ProjectID: f.project.ID, Type: "debit", AmountCredits: decimal.RequireFromString("-" + amount),
		Reason: reason, Meter: meter, UsageRecordID: usage.ID, ResourceType: resourceType, ResourceID: resourceID, CreatedAt: settledAt,
	}
	if err := f.handlers.db.Create(&usage).Error; err != nil {
		t.Fatalf("create usage %s: %v", id, err)
	}
	if err := f.handlers.db.Create(&ledger).Error; err != nil {
		t.Fatalf("create ledger %s: %v", id, err)
	}
}

func (f billingPeriodFixture) get(t *testing.T, user model.User, target string, handler func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Set(currentUserContextKey, user)
	handler(ctx)
	return recorder
}

func TestBillingSummaryCountsUsageByOccurrenceInCallerTimeZone(t *testing.T) {
	fixture := newBillingPeriodFixture(t)
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	dayStart, _ := billing.DayAndMonthStart(time.Now().In(location))
	// Usage that happened before local midnight but was settled after it
	// belongs to yesterday; usage after local midnight belongs to today.
	fixture.addDebit(t, "yesterday", billing.ResourceTypeRuntime, "dplt_billing_period:1", billing.MeterRuntimeCPU, billing.ReasonRuntimeUsage, "3",
		dayStart.Add(-time.Hour), dayStart, dayStart.Add(5*time.Minute))
	fixture.addDebit(t, "today", billing.ResourceTypeRuntime, "dplt_billing_period:2", billing.MeterRuntimeCPU, billing.ReasonRuntimeUsage, "2",
		dayStart, dayStart.Add(time.Hour), dayStart.Add(time.Hour+5*time.Minute))

	recorder := fixture.get(t, fixture.user, "/api/v1/billing/summary?timeZone=Asia%2FShanghai", fixture.handlers.domains.billing.GetBillingSummary)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var summary billing.ProjectBillingSummary
	if err := json.Unmarshal(recorder.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if !summary.TodaySpend.Equal(decimal.NewFromInt(2)) {
		t.Fatalf("todaySpend = %s, want 2 (usage occurrence, not settlement time)", summary.TodaySpend)
	}
	if summary.BalanceScope != billing.BalanceScopeAccount || summary.BalanceStatus != "ok" || !summary.BalanceCredits.Equal(decimal.NewFromInt(500)) {
		t.Fatalf("account balance = %s %s %s", summary.BalanceScope, summary.BalanceStatus, summary.BalanceCredits)
	}

	invalid := fixture.get(t, fixture.user, "/api/v1/billing/summary?timeZone=Mars%2FOlympus", fixture.handlers.domains.billing.GetBillingSummary)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid time zone status = %d, body = %s", invalid.Code, invalid.Body.String())
	}
}

func TestBillingSummaryPlatformTotalHasNoBalanceStatus(t *testing.T) {
	fixture := newBillingPeriodFixture(t)
	for _, test := range []struct {
		name       string
		target     string
		wantScope  string
		wantStatus string
		wantTotal  int64
	}{
		{name: "all wallets", target: "/api/v1/billing/summary", wantScope: billing.BalanceScopePlatformTotal, wantStatus: "", wantTotal: 500},
		{name: "selected user", target: "/api/v1/billing/summary?userId=usr_billing_period", wantScope: billing.BalanceScopeAccount, wantStatus: "ok", wantTotal: 500},
		{name: "current account", target: "/api/v1/billing/summary?accountScope=current", wantScope: billing.BalanceScopeAccount, wantStatus: "insufficient", wantTotal: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := fixture.get(t, fixture.admin, test.target, fixture.handlers.domains.billing.GetBillingSummary)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode summary: %v", err)
			}
			status, hasStatus := body["balanceStatus"]
			if body["balanceScope"] != test.wantScope || (test.wantStatus == "" && hasStatus) || (test.wantStatus != "" && status != test.wantStatus) {
				t.Fatalf("summary scope/status = %v/%v", body["balanceScope"], status)
			}
			if got, err := decimal.NewFromString(body["balanceCredits"].(string)); err != nil || !got.Equal(decimal.NewFromInt(test.wantTotal)) {
				t.Fatalf("balanceCredits = %v, want %d", body["balanceCredits"], test.wantTotal)
			}
		})
	}
}

func TestBillingDeploymentSpendGroupsStorageAndFiltersByOccurrence(t *testing.T) {
	fixture := newBillingPeriodFixture(t)
	periodStart := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)
	fixture.addDebit(t, "runtime", billing.ResourceTypeRuntime, "dplt_billing_period:2026090101", billing.MeterRuntimeCPU, billing.ReasonRuntimeUsage, "4",
		periodStart.Add(time.Hour), periodStart.Add(2*time.Hour), periodStart.Add(2*time.Hour))
	fixture.addDebit(t, "storage", billing.ResourceTypeStorage, "pvol_billing_period:2026090101", "storage.gib_day", billing.ReasonStorageUsage, "1",
		periodStart.Add(time.Hour), periodStart.Add(2*time.Hour), periodStart.Add(2*time.Hour))
	fixture.addDebit(t, "transfer", billing.ResourceTypeTransfer, "vtx_billing_period", billing.MeterStorageTransferGiB, billing.ReasonTransferUsage, "0.5",
		periodStart.Add(3*time.Hour), periodStart.Add(4*time.Hour), periodStart.Add(4*time.Hour))
	// Started before the period and ended inside it: occurs in August.
	fixture.addDebit(t, "overlap", billing.ResourceTypeRuntime, "dplt_billing_period:2026083123", billing.MeterRuntimeCPU, billing.ReasonRuntimeUsage, "8",
		periodStart.Add(-30*time.Minute), periodStart.Add(30*time.Minute), periodStart.Add(time.Hour))

	query := "?periodStart=" + periodStart.Format(time.RFC3339) + "&periodEnd=" + periodEnd.Format(time.RFC3339)
	recorder := fixture.get(t, fixture.user, "/api/v1/billing/deployment-spend"+query, fixture.handlers.domains.billing.ListBillingDeploymentSpend)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var page struct {
		Items []struct {
			DeploymentTargetID string `json:"deploymentTargetId"`
			SpendGroup         string `json:"spendGroup"`
			AmountCredits      string `json:"amountCredits"`
			StorageCredits     string `json:"storageCredits"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode deployment spend: %v", err)
	}
	got := map[string]string{}
	for _, item := range page.Items {
		got[item.SpendGroup+"|"+item.DeploymentTargetID] = item.AmountCredits
	}
	want := map[string]string{"deployment_target|dplt_billing_period": "4", "project_storage|": "1.5"}
	if page.Total != int64(len(want)) || len(got) != len(want) {
		t.Fatalf("deployment spend rows = %#v (total %d), want %#v", got, page.Total, want)
	}
	for key, amount := range want {
		value, err := decimal.NewFromString(got[key])
		if err != nil || !value.Equal(decimal.RequireFromString(amount)) {
			t.Fatalf("deployment spend %s = %q, want %s (rows %#v)", key, got[key], amount, got)
		}
	}

	usage := fixture.get(t, fixture.user, "/api/v1/billing/usage-records"+query, fixture.handlers.domains.billing.ListBillingUsageRecords)
	ledger := fixture.get(t, fixture.user, "/api/v1/billing/ledger"+query, fixture.handlers.domains.billing.ListBillingLedgerEntries)
	for name, response := range map[string]*httptest.ResponseRecorder{"usage": usage, "ledger": ledger} {
		var listed struct {
			Total int64 `json:"total"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &listed) != nil || listed.Total != 3 {
			t.Fatalf("%s list status = %d total = %d body = %s", name, response.Code, listed.Total, response.Body.String())
		}
	}
}
