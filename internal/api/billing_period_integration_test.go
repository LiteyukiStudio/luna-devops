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
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type billingPeriodFixture struct {
	handlers *Handlers
	user     model.User
	admin    model.User
	project  model.Project
}

func newBillingPeriodFixture(t *testing.T) billingPeriodFixture {
	t.Helper()
	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "billing_period_test",
		Migrate: func(db *gorm.DB) error {
			if err := db.Exec(`
CREATE SCHEMA ai;
CREATE TABLE ai.model_usages (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL,
    status text NOT NULL DEFAULT 'reported',
    settlement_status text NOT NULL DEFAULT 'pending',
    prompt_tokens bigint NOT NULL DEFAULT 0,
    completion_tokens bigint NOT NULL DEFAULT 0,
    total_tokens bigint NOT NULL DEFAULT 0,
    cached_prompt_tokens bigint,
    cache_write_prompt_tokens bigint,
    reasoning_completion_tokens bigint,
    input_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    output_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    cached_input_credits_per_million numeric(24,8) NOT NULL DEFAULT 0
)`).Error; err != nil {
				return err
			}
			return db.AutoMigrate(
				&model.User{}, &model.Project{}, &model.ProjectMember{},
				&model.UserWallet{}, &model.BillingUsageRecord{}, &model.BillingLedgerEntry{},
				&model.Application{}, &model.DeploymentTarget{}, &model.BuildRun{}, &model.GatewayRoute{},
			)
		},
	})
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

func TestBillingSpendTimeseriesZeroFillsAndMatchesSummary(t *testing.T) {
	fixture := newBillingPeriodFixture(t)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load Asia/Shanghai: %v", err)
	}
	dayStart := time.Date(2026, 11, 22, 0, 0, 0, 0, shanghai)
	periodEnd := dayStart.Add(48 * time.Hour)

	fixture.addDebit(t, "rt_h1", billing.ResourceTypeRuntime, "dplt_billing_period:h1", billing.MeterRuntimeCPU, billing.ReasonRuntimeUsage, "2.5",
		dayStart.Add(2*time.Hour), dayStart.Add(3*time.Hour), dayStart.Add(3*time.Hour+5*time.Minute))
	fixture.addDebit(t, "build_d2", billing.ResourceTypeBuildRun, "bld_d2", "build.cpu_vcpu_minute", billing.ReasonBuildUsage, "1.25",
		dayStart.Add(26*time.Hour), dayStart.Add(26*time.Hour+10*time.Minute), dayStart.Add(26*time.Hour+12*time.Minute))

	// Personal AI debit (empty ProjectID) + credit recharge (must be excluded from spend)
	aiUsage := model.BillingUsageRecord{
		ID: "busg_ai_1", ProjectID: "", BilledUserID: fixture.user.ID, Meter: "ai.completion_tokens_1m",
		Quantity: decimal.NewFromInt(1), Unit: "1M tokens", AmountCredits: decimal.RequireFromString("6.25"),
		ResourceType: "ai_model_usage", ResourceID: "aimu_1",
		PeriodStart: dayStart.Add(23 * time.Hour), PeriodEnd: dayStart.Add(23*time.Hour + time.Minute),
		Status: "settled", CreatedAt: dayStart.Add(23*time.Hour + time.Minute),
	}
	aiLedger := model.BillingLedgerEntry{
		ID: "bled_ai_1", UserID: fixture.user.ID, ProjectID: "", Type: "debit",
		AmountCredits: decimal.RequireFromString("-6.25"), Reason: "ai.usage", Meter: "ai.completion_tokens_1m",
		UsageRecordID: aiUsage.ID, ResourceType: "ai_model_usage", ResourceID: "aimu_1", CreatedAt: dayStart.Add(23*time.Hour + time.Minute),
	}
	creditLedger := model.BillingLedgerEntry{
		ID: "bled_recharge_1", UserID: fixture.user.ID, ProjectID: "", Type: "credit",
		AmountCredits: decimal.RequireFromString("100"), Reason: "billing.recharge", CreatedAt: dayStart.Add(5 * time.Hour),
	}
	for _, row := range []any{&aiUsage, &aiLedger, &creditLedger} {
		if err := fixture.handlers.db.Create(row).Error; err != nil {
			t.Fatalf("seed row %T: %v", row, err)
		}
	}

	baseQuery := "?periodStart=" + dayStart.UTC().Format(time.RFC3339) +
		"&periodEnd=" + periodEnd.UTC().Format(time.RFC3339) +
		"&timeZone=Asia%2FShanghai"

	summaryRec := fixture.get(t, fixture.user, "/api/v1/billing/summary"+baseQuery, fixture.handlers.domains.billing.GetBillingSummary)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("summary status = %d body = %s", summaryRec.Code, summaryRec.Body.String())
	}
	var summary billing.ProjectBillingSummary
	if err := json.Unmarshal(summaryRec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}

	for _, tc := range []struct {
		granularity string
		wantBuckets int
	}{
		{granularity: "hour", wantBuckets: 48},
		{granularity: "day", wantBuckets: 2},
		{granularity: "month", wantBuckets: 1},
	} {
		t.Run(tc.granularity, func(t *testing.T) {
			rec := fixture.get(t, fixture.user, "/api/v1/billing/spend-timeseries"+baseQuery+"&granularity="+tc.granularity, fixture.handlers.domains.billing.GetBillingSpendTimeseries)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			var series billing.SpendTimeseries
			if err := json.Unmarshal(rec.Body.Bytes(), &series); err != nil {
				t.Fatalf("decode timeseries: %v", err)
			}
			if len(series.Buckets) != tc.wantBuckets {
				t.Fatalf("buckets = %d, want %d", len(series.Buckets), tc.wantBuckets)
			}
			if !series.TotalCredits.Equal(summary.PeriodSpend) || !series.TotalCredits.Equal(decimal.RequireFromString("10")) {
				t.Fatalf("totalCredits = %s, summary.PeriodSpend = %s, want 10", series.TotalCredits, summary.PeriodSpend)
			}
			bucketSum := decimal.Zero
			for _, b := range series.Buckets {
				bucketSum = bucketSum.Add(b.TotalCredits)
			}
			if !bucketSum.Equal(series.TotalCredits) {
				t.Fatalf("bucketSum = %s, want %s", bucketSum, series.TotalCredits)
			}
		})
	}

	// Project filter excludes personal AI debits (3.75 instead of 10)
	projectRec := fixture.get(t, fixture.user, "/api/v1/billing/spend-timeseries"+baseQuery+"&granularity=day&projectIds="+fixture.project.ID, fixture.handlers.domains.billing.GetBillingSpendTimeseries)
	if projectRec.Code != http.StatusOK {
		t.Fatalf("project-scoped status = %d body = %s", projectRec.Code, projectRec.Body.String())
	}
	var projectSeries billing.SpendTimeseries
	if err := json.Unmarshal(projectRec.Body.Bytes(), &projectSeries); err != nil {
		t.Fatalf("decode project series: %v", err)
	}
	if !projectSeries.TotalCredits.Equal(decimal.RequireFromString("3.75")) {
		t.Fatalf("project totalCredits = %s, want 3.75", projectSeries.TotalCredits)
	}

	// Guardrails: invalid granularity & wide range for hourly
	badGran := fixture.get(t, fixture.user, "/api/v1/billing/spend-timeseries"+baseQuery+"&granularity=minute", fixture.handlers.domains.billing.GetBillingSpendTimeseries)
	if badGran.Code != http.StatusBadRequest {
		t.Fatalf("invalid granularity status = %d, want 400", badGran.Code)
	}
	wideQuery := "?periodStart=2026-01-01T00:00:00Z&periodEnd=2027-01-01T00:00:00Z&granularity=hour"
	tooWide := fixture.get(t, fixture.user, "/api/v1/billing/spend-timeseries"+wideQuery, fixture.handlers.domains.billing.GetBillingSpendTimeseries)
	if tooWide.Code != http.StatusBadRequest {
		t.Fatalf("too wide hourly status = %d, want 400", tooWide.Code)
	}

	// Unified ledger entries carry joined usageQuantity/usageUnit and support category filtering
	aiLedgerRec := fixture.get(t, fixture.user, "/api/v1/billing/ledger"+baseQuery+"&category=ai", fixture.handlers.domains.billing.ListBillingLedgerEntries)
	if aiLedgerRec.Code != http.StatusOK {
		t.Fatalf("ai category ledger status = %d body = %s", aiLedgerRec.Code, aiLedgerRec.Body.String())
	}
	var aiLedgerPage struct {
		Total int64 `json:"total"`
		Items []struct {
			ID            string `json:"id"`
			UsageQuantity string `json:"usageQuantity"`
			UsageUnit     string `json:"usageUnit"`
		} `json:"items"`
	}
	if err := json.Unmarshal(aiLedgerRec.Body.Bytes(), &aiLedgerPage); err != nil {
		t.Fatalf("decode ai ledger page: %v", err)
	}
	if aiLedgerPage.Total != 1 || len(aiLedgerPage.Items) != 1 || aiLedgerPage.Items[0].ID != "bled_ai_1" ||
		aiLedgerPage.Items[0].UsageQuantity != "1" || aiLedgerPage.Items[0].UsageUnit != "1M tokens" {
		t.Fatalf("ai ledger page = %+v", aiLedgerPage)
	}

	creditLedgerRec := fixture.get(t, fixture.user, "/api/v1/billing/ledger"+baseQuery+"&category=adjustment&type=credit", fixture.handlers.domains.billing.ListBillingLedgerEntries)
	if creditLedgerRec.Code != http.StatusOK {
		t.Fatalf("credit adjustment ledger status = %d body = %s", creditLedgerRec.Code, creditLedgerRec.Body.String())
	}
	var creditLedgerPage struct {
		Total int64 `json:"total"`
		Items []struct {
			ID            string `json:"id"`
			UsageQuantity string `json:"usageQuantity"`
			UsageUnit     string `json:"usageUnit"`
		} `json:"items"`
	}
	if err := json.Unmarshal(creditLedgerRec.Body.Bytes(), &creditLedgerPage); err != nil {
		t.Fatalf("decode credit ledger page: %v", err)
	}
	if creditLedgerPage.Total != 1 || len(creditLedgerPage.Items) != 1 || creditLedgerPage.Items[0].ID != "bled_recharge_1" ||
		creditLedgerPage.Items[0].UsageQuantity != "" || creditLedgerPage.Items[0].UsageUnit != "" {
		t.Fatalf("credit ledger page = %+v", creditLedgerPage)
	}

	invalidCategoryRec := fixture.get(t, fixture.user, "/api/v1/billing/ledger"+baseQuery+"&category=bogus", fixture.handlers.domains.billing.ListBillingLedgerEntries)
	if invalidCategoryRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid category status = %d, want 400", invalidCategoryRec.Code)
	}
}
