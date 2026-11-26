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

func TestProjectScopedBillingAllowsAllMembersAndRedactsWalletBalance(t *testing.T) {
	fixture := newBillingPeriodFixture(t)
	db := fixture.handlers.db

	ownerB := model.User{ID: "usr_owner_b", Email: "owner-b@example.com", Name: "Owner B", Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash"}
	projAdmin := model.User{ID: "usr_proj_admin", Email: "proj-admin@example.com", Name: "Proj Admin", Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash"}
	developer := model.User{ID: "usr_proj_dev", Email: "proj-dev@example.com", Name: "Proj Dev", Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash"}
	viewer := model.User{ID: "usr_proj_viewer", Email: "proj-viewer@example.com", Name: "Proj Viewer", Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash"}
	outsider := model.User{ID: "usr_outsider", Email: "outsider@example.com", Name: "Outsider", Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash"}

	projectTwo := model.Project{ID: "prj_billing_two", Identifier: "billing-two", Name: "Billing Two", BillingOwnerUserID: ownerB.ID}
	projectUnrelated := model.Project{ID: "prj_unrelated", Identifier: "unrelated", Name: "Unrelated", BillingOwnerUserID: ownerB.ID}

	appWeb := model.Application{ID: "app_web", ProjectID: fixture.project.ID, Identifier: "web", Name: "Web App"}
	appWorker := model.Application{ID: "app_worker", ProjectID: fixture.project.ID, Identifier: "worker", Name: "Worker App"}
	targetProd := model.DeploymentTarget{ID: "dplt_web_prod", ProjectID: fixture.project.ID, ApplicationID: appWeb.ID, Name: "prod-env", Stage: "prod"}
	targetStaging := model.DeploymentTarget{ID: "dplt_web_staging", ProjectID: fixture.project.ID, ApplicationID: appWeb.ID, Name: "staging-env", Stage: "staging"}
	targetDev := model.DeploymentTarget{ID: "dplt_worker_dev", ProjectID: fixture.project.ID, ApplicationID: appWorker.ID, Name: "dev-env", Stage: "dev"}
	targetOther := model.DeploymentTarget{ID: "dplt_two_prod", ProjectID: projectTwo.ID, ApplicationID: "app_two", Name: "two-prod", Stage: "prod"}

	exOwner := model.User{ID: "usr_ex_owner", Email: "ex-owner@example.com", Name: "Ex Owner", Role: authz.PlatformRoleUser, Language: "zh-CN", Password: "hash"}
	projectExOwner := model.Project{ID: "prj_ex_owner", Identifier: "ex-owner-proj", Name: "Ex Owner Project", BillingOwnerUserID: exOwner.ID}

	members := []model.ProjectMember{
		{ID: "pm_owner_a", ProjectID: fixture.project.ID, UserID: fixture.user.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_owner_b", ProjectID: fixture.project.ID, UserID: ownerB.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_admin", ProjectID: fixture.project.ID, UserID: projAdmin.ID, Role: authz.ProjectRoleAdmin},
		{ID: "pm_dev", ProjectID: fixture.project.ID, UserID: developer.ID, Role: authz.ProjectRoleDeveloper},
		{ID: "pm_viewer", ProjectID: fixture.project.ID, UserID: viewer.ID, Role: authz.ProjectRoleViewer},
		{ID: "pm_dev_two", ProjectID: projectTwo.ID, UserID: developer.ID, Role: authz.ProjectRoleDeveloper},
	}

	for _, row := range []any{
		&ownerB, &projAdmin, &developer, &viewer, &outsider, &exOwner,
		&projectTwo, &projectUnrelated, &projectExOwner,
		&appWeb, &appWorker, &targetProd, &targetStaging, &targetDev, &targetOther,
		&model.UserWallet{ID: "wal_owner_b", UserID: ownerB.ID, BalanceCredits: decimal.RequireFromString("888")},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed row %T: %v", row, err)
		}
	}
	for i := range members {
		if err := db.Create(&members[i]).Error; err != nil {
			t.Fatalf("seed member %s: %v", members[i].ID, err)
		}
	}

	periodStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)
	recordCharge := func(id, projectID, appID, billedUserID, targetID, amount, balanceAfter string, offset time.Duration) {
		start := periodStart.Add(offset)
		end := start.Add(time.Hour)
		usage := model.BillingUsageRecord{
			ID: "busg_" + id, ProjectID: projectID, ApplicationID: appID, BilledUserID: billedUserID,
			Meter: billing.MeterRuntimeCPU, Quantity: decimal.NewFromInt(1), Unit: "vcpu_hour",
			AmountCredits: decimal.RequireFromString(amount), ResourceType: billing.ResourceTypeRuntime,
			ResourceID: targetID + ":window", PeriodStart: start, PeriodEnd: end, Status: "settled", SettledAt: &end, CreatedAt: end,
		}
		ledger := model.BillingLedgerEntry{
			ID: "bled_" + id, UserID: billedUserID, ProjectID: projectID, Type: "debit",
			AmountCredits: decimal.RequireFromString("-" + amount), BalanceAfterCredits: decimal.RequireFromString(balanceAfter),
			Reason: billing.ReasonRuntimeUsage, Meter: billing.MeterRuntimeCPU, UsageRecordID: usage.ID,
			ResourceType: billing.ResourceTypeRuntime, ResourceID: usage.ResourceID, CreatedAt: end,
		}
		if err := db.Create(&usage).Error; err != nil {
			t.Fatalf("create usage %s: %v", id, err)
		}
		if err := db.Create(&ledger).Error; err != nil {
			t.Fatalf("create ledger %s: %v", id, err)
		}
	}

	// Historical transfer: fixture.user paid 4, then ownerB paid 6 and 2.5 in fixture.project (total = 12.5)
	recordCharge("p1_prod_old_owner", fixture.project.ID, appWeb.ID, fixture.user.ID, targetProd.ID, "4", "496", time.Hour)
	recordCharge("p1_staging_new_owner", fixture.project.ID, appWeb.ID, ownerB.ID, targetStaging.ID, "6", "882", 2*time.Hour)
	recordCharge("p1_dev_new_owner", fixture.project.ID, appWorker.ID, ownerB.ID, targetDev.ID, "2.5", "879.5", 3*time.Hour)
	// Second project (total = 7) and unrelated project (total = 50)
	recordCharge("p2_prod", projectTwo.ID, "app_two", ownerB.ID, targetOther.ID, "7", "872.5", 4*time.Hour)
	recordCharge("p3_unrelated", projectUnrelated.ID, "app_three", ownerB.ID, "dplt_unrel", "50", "822.5", 5*time.Hour)

	// Personal AI charge on fixture.user must never appear in scopeMode=project
	aiLedger := model.BillingLedgerEntry{
		ID: "bled_ai_personal", UserID: fixture.user.ID, ProjectID: "", Type: "debit",
		AmountCredits: decimal.RequireFromString("-9"), BalanceAfterCredits: decimal.RequireFromString("487"),
		Reason: "ai.usage", Meter: "ai.prompt_tokens_1m", ResourceType: "ai_model_usage", ResourceID: "aimu_p",
		CreatedAt: periodStart.Add(6 * time.Hour),
	}
	if err := db.Create(&aiLedger).Error; err != nil {
		t.Fatalf("create ai ledger: %v", err)
	}

	// Non-debit wallet adjustment tagged with ProjectID must never leak into scopeMode=project ledger
	projectTaggedCredit := model.BillingLedgerEntry{
		ID: "bled_proj_credit", UserID: ownerB.ID, ProjectID: fixture.project.ID, Type: "credit",
		AmountCredits: decimal.RequireFromString("50"), BalanceAfterCredits: decimal.RequireFromString("938"),
		Reason: billing.ReasonManualRecharge, ResourceType: billing.ResourceTypeWallet, ResourceID: ownerB.ID,
		Description: "private admin note", CreatedAt: periodStart.Add(7 * time.Hour),
	}
	if err := db.Create(&projectTaggedCredit).Error; err != nil {
		t.Fatalf("create project-tagged credit: %v", err)
	}

	periodQuery := "periodStart=" + periodStart.Format(time.RFC3339) + "&periodEnd=" + periodEnd.Format(time.RFC3339)

	// 1. All member roles can read project billing across historical billing owners, with wallet balances redacted
	for _, memberUser := range []model.User{fixture.user, ownerB, projAdmin, developer, viewer, fixture.admin} {
		q := "?scopeMode=project&projectIds=" + fixture.project.ID + "&" + periodQuery
		sumRec := fixture.get(t, memberUser, "/api/v1/billing/summary"+q, fixture.handlers.domains.billing.GetBillingSummary)
		if sumRec.Code != http.StatusOK {
			t.Fatalf("role user %s summary status = %d body = %s", memberUser.ID, sumRec.Code, sumRec.Body.String())
		}
		var rawSum map[string]any
		if err := json.Unmarshal(sumRec.Body.Bytes(), &rawSum); err != nil {
			t.Fatalf("decode summary: %v", err)
		}
		if rawSum["balanceScope"] != billing.BalanceScopeProject {
			t.Fatalf("user %s balanceScope = %v, want project", memberUser.ID, rawSum["balanceScope"])
		}
		if _, hasStatus := rawSum["balanceStatus"]; hasStatus {
			t.Fatalf("user %s leaked balanceStatus: %v", memberUser.ID, rawSum["balanceStatus"])
		}
		if rawSum["balanceCredits"] != "0" || rawSum["availableCredits"] != "0" || rawSum["periodSpend"] != "12.5" {
			t.Fatalf("user %s summary values = %+v", memberUser.ID, rawSum)
		}

		tsRec := fixture.get(t, memberUser, "/api/v1/billing/spend-timeseries"+q+"&granularity=day", fixture.handlers.domains.billing.GetBillingSpendTimeseries)
		var ts billing.SpendTimeseries
		if tsRec.Code != http.StatusOK || json.Unmarshal(tsRec.Body.Bytes(), &ts) != nil || !ts.TotalCredits.Equal(decimal.RequireFromString("12.5")) {
			t.Fatalf("user %s timeseries status=%d total=%s body=%s", memberUser.ID, tsRec.Code, ts.TotalCredits, tsRec.Body.String())
		}

		ledgerRec := fixture.get(t, memberUser, "/api/v1/billing/ledger"+q, fixture.handlers.domains.billing.ListBillingLedgerEntries)
		var ledgerPage struct {
			Total int64 `json:"total"`
			Items []struct {
				UserID              string `json:"userId"`
				BalanceAfterCredits string `json:"balanceAfterCredits"`
			} `json:"items"`
		}
		if ledgerRec.Code != http.StatusOK || json.Unmarshal(ledgerRec.Body.Bytes(), &ledgerPage) != nil || ledgerPage.Total != 3 {
			t.Fatalf("user %s ledger status=%d body=%s", memberUser.ID, ledgerRec.Code, ledgerRec.Body.String())
		}
		for _, item := range ledgerPage.Items {
			if item.UserID != "" || item.BalanceAfterCredits != "0" {
				t.Fatalf("user %s leaked private wallet fields in project ledger: %+v", memberUser.ID, item)
			}
		}

		usageRec := fixture.get(t, memberUser, "/api/v1/billing/usage-records"+q, fixture.handlers.domains.billing.ListBillingUsageRecords)
		var usagePage struct {
			Total int64 `json:"total"`
			Items []struct {
				BilledUserID string `json:"billedUserId"`
			} `json:"items"`
		}
		if usageRec.Code != http.StatusOK || json.Unmarshal(usageRec.Body.Bytes(), &usagePage) != nil || usagePage.Total != 3 {
			t.Fatalf("user %s usage status=%d body=%s", memberUser.ID, usageRec.Code, usageRec.Body.String())
		}
		for _, item := range usagePage.Items {
			if item.BilledUserID != "" {
				t.Fatalf("user %s leaked billedUserId: %+v", memberUser.ID, item)
			}
		}
	}

	// Duplicate projectIds must deduplicate cleanly for admins without false 404
	if dupAdmin := fixture.get(t, fixture.admin, "/api/v1/billing/summary?scopeMode=project&projectIds="+fixture.project.ID+","+fixture.project.ID+"&"+periodQuery, fixture.handlers.domains.billing.GetBillingSummary); dupAdmin.Code != http.StatusOK {
		t.Fatalf("duplicate projectIds admin status = %d body = %s", dupAdmin.Code, dupAdmin.Body.String())
	}

	// Ex-member whose ID remains on BillingOwnerUserID without project_members row is denied in scopeMode=project
	if exOwnerRec := fixture.get(t, exOwner, "/api/v1/billing/summary?scopeMode=project&projectIds="+projectExOwner.ID+"&"+periodQuery, fixture.handlers.domains.billing.GetBillingSummary); exOwnerRec.Code != http.StatusForbidden {
		t.Fatalf("ex-member billingOwner status = %d, want 403", exOwnerRec.Code)
	}

	// Non-debit types and non-project categories are rejected in scopeMode=project
	if badLedgerType := fixture.get(t, viewer, "/api/v1/billing/ledger?scopeMode=project&type=credit&projectIds="+fixture.project.ID, fixture.handlers.domains.billing.ListBillingLedgerEntries); badLedgerType.Code != http.StatusBadRequest {
		t.Fatalf("scopeMode=project + type=credit status = %d, want 400", badLedgerType.Code)
	}
	if badLedgerCat := fixture.get(t, viewer, "/api/v1/billing/ledger?scopeMode=project&category=adjustment&projectIds="+fixture.project.ID, fixture.handlers.domains.billing.ListBillingLedgerEntries); badLedgerCat.Code != http.StatusBadRequest {
		t.Fatalf("scopeMode=project + category=adjustment status = %d, want 400", badLedgerCat.Code)
	}
	if badUsageMeter := fixture.get(t, viewer, "/api/v1/billing/usage-records?scopeMode=project&meter=ai.prompt_tokens_1m&projectIds="+fixture.project.ID, fixture.handlers.domains.billing.ListBillingUsageRecords); badUsageMeter.Code != http.StatusBadRequest {
		t.Fatalf("scopeMode=project + meter=ai.prompt_tokens_1m status = %d, want 400", badUsageMeter.Code)
	}

	// 2. Application and stage filters on deployment-spend match both items and total
	filteredSpendRec := fixture.get(t, viewer, "/api/v1/billing/deployment-spend?scopeMode=project&projectIds="+fixture.project.ID+"&applicationId="+appWeb.ID+"&stage=staging&"+periodQuery, fixture.handlers.domains.billing.ListBillingDeploymentSpend)
	var filteredSpend struct {
		Total int64 `json:"total"`
		Items []struct {
			ApplicationID         string `json:"applicationId"`
			DeploymentTargetStage string `json:"deploymentTargetStage"`
			AmountCredits         string `json:"amountCredits"`
		} `json:"items"`
	}
	if filteredSpendRec.Code != http.StatusOK || json.Unmarshal(filteredSpendRec.Body.Bytes(), &filteredSpend) != nil {
		t.Fatalf("filtered deployment-spend status=%d body=%s", filteredSpendRec.Code, filteredSpendRec.Body.String())
	}
	if filteredSpend.Total != 1 || len(filteredSpend.Items) != 1 || filteredSpend.Items[0].ApplicationID != appWeb.ID || filteredSpend.Items[0].DeploymentTargetStage != "staging" || filteredSpend.Items[0].AmountCredits != "6" {
		t.Fatalf("filtered deployment-spend = %+v", filteredSpend)
	}

	// 3. "All accessible projects" (empty projectIds in scopeMode=project) scopes strictly to user's projects
	devAllRec := fixture.get(t, developer, "/api/v1/billing/summary?scopeMode=project&"+periodQuery, fixture.handlers.domains.billing.GetBillingSummary)
	var devAll billing.ProjectBillingSummary
	if devAllRec.Code != http.StatusOK || json.Unmarshal(devAllRec.Body.Bytes(), &devAll) != nil || !devAll.PeriodSpend.Equal(decimal.RequireFromString("19.5")) {
		t.Fatalf("developer all-projects spend = %s (status %d), want 19.5 (12.5 + 7)", devAll.PeriodSpend, devAllRec.Code)
	}
	viewerAllRec := fixture.get(t, viewer, "/api/v1/billing/summary?scopeMode=project&"+periodQuery, fixture.handlers.domains.billing.GetBillingSummary)
	var viewerAll billing.ProjectBillingSummary
	if viewerAllRec.Code != http.StatusOK || json.Unmarshal(viewerAllRec.Body.Bytes(), &viewerAll) != nil || !viewerAll.PeriodSpend.Equal(decimal.RequireFromString("12.5")) {
		t.Fatalf("viewer all-projects spend = %s (status %d), want 12.5", viewerAll.PeriodSpend, viewerAllRec.Code)
	}

	// 4. Outsider with 0 projects gets 403 on explicit project and 0 on empty projectIds (fail-closed)
	if forbidden := fixture.get(t, outsider, "/api/v1/billing/summary?scopeMode=project&projectIds="+fixture.project.ID+"&"+periodQuery, fixture.handlers.domains.billing.GetBillingSummary); forbidden.Code != http.StatusForbidden {
		t.Fatalf("outsider explicit project status = %d, want 403", forbidden.Code)
	}
	outsiderAllRec := fixture.get(t, outsider, "/api/v1/billing/summary?scopeMode=project&"+periodQuery, fixture.handlers.domains.billing.GetBillingSummary)
	var outsiderAll billing.ProjectBillingSummary
	if outsiderAllRec.Code != http.StatusOK || json.Unmarshal(outsiderAllRec.Body.Bytes(), &outsiderAll) != nil || !outsiderAll.PeriodSpend.IsZero() {
		t.Fatalf("outsider all-projects spend = %s (status %d), want 0", outsiderAll.PeriodSpend, outsiderAllRec.Code)
	}
	outsiderSpendRec := fixture.get(t, outsider, "/api/v1/billing/deployment-spend?scopeMode=project&"+periodQuery, fixture.handlers.domains.billing.ListBillingDeploymentSpend)
	var outsiderSpend struct {
		Total int64 `json:"total"`
	}
	if outsiderSpendRec.Code != http.StatusOK || json.Unmarshal(outsiderSpendRec.Body.Bytes(), &outsiderSpend) != nil || outsiderSpend.Total != 0 {
		t.Fatalf("outsider deployment-spend = %d/%+v, want 0", outsiderSpendRec.Code, outsiderSpend)
	}

	// 5. Wallet mode (default) still restricts non-billingOwner members and guardrails reject invalid combinations
	if walletViewer := fixture.get(t, viewer, "/api/v1/billing/summary?projectIds="+fixture.project.ID+"&"+periodQuery, fixture.handlers.domains.billing.GetBillingSummary); walletViewer.Code != http.StatusForbidden {
		t.Fatalf("wallet mode viewer status = %d, want 403", walletViewer.Code)
	}
	if badMode := fixture.get(t, viewer, "/api/v1/billing/summary?scopeMode=bogus", fixture.handlers.domains.billing.GetBillingSummary); badMode.Code != http.StatusBadRequest {
		t.Fatalf("bad scopeMode status = %d, want 400", badMode.Code)
	}
	if badCombo := fixture.get(t, fixture.admin, "/api/v1/billing/summary?scopeMode=project&userId="+fixture.user.ID, fixture.handlers.domains.billing.GetBillingSummary); badCombo.Code != http.StatusBadRequest {
		t.Fatalf("scopeMode=project + userId status = %d, want 400", badCombo.Code)
	}
	if badAccountCombo := fixture.get(t, viewer, "/api/v1/billing/summary?scopeMode=project&accountScope=current", fixture.handlers.domains.billing.GetBillingSummary); badAccountCombo.Code != http.StatusBadRequest {
		t.Fatalf("scopeMode=project + accountScope status = %d, want 400", badAccountCombo.Code)
	}
}
