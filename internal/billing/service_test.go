package billing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func TestDefaultRateRulesPreferGatewayTrafficOverRequestBilling(t *testing.T) {
	rules := defaultRateRuleByMeter()
	traffic, ok := rules["gateway.egress_gib"]
	if !ok {
		t.Fatal("expected gateway traffic billing rule")
	}
	if !traffic.Enabled {
		t.Fatal("expected gateway traffic billing to be enabled by default")
	}
	if traffic.Unit != "gib" {
		t.Fatalf("gateway traffic unit = %q", traffic.Unit)
	}
	requests, ok := rules["gateway.requests_1000"]
	if !ok {
		t.Fatal("expected gateway request count rule")
	}
	if requests.Enabled {
		t.Fatal("expected request count billing to be disabled by default")
	}
	if !requests.CreditsPerUnit.Equal(decimal.Zero) {
		t.Fatalf("request count price = %s", requests.CreditsPerUnit)
	}
}

func TestDefaultRateRulesExcludeAIModelTokenRates(t *testing.T) {
	rules := defaultRateRuleByMeter()
	for _, meter := range []string{MeterAIPromptTokens, MeterAICompletionTokens, MeterAICachedPromptTokens, MeterAICacheWritePromptTokens} {
		if _, ok := rules[meter]; ok {
			t.Fatalf("AI model token rate %q must not be a generic default rate", meter)
		}
	}
}

func TestTokenBillingQuantityPreservesPartialMillions(t *testing.T) {
	if got := tokenBillingQuantity(1_250_000); !got.Equal(decimal.RequireFromString("1.25")) {
		t.Fatalf("token billing quantity = %s", got)
	}
}

func TestSettleAIModelUsageBillsRunOwnerOnce(t *testing.T) {
	db := openBillingTestDB(t)
	if err := db.AutoMigrate(&model.UserWallet{}, &model.BillingRateRule{}, &model.BillingUsageRecord{}, &model.BillingLedgerEntry{}); err != nil {
		t.Fatalf("migrate billing tables: %v", err)
	}
	service := Service{DB: db}
	input := AIModelUsageInput{
		EventID: "aievt_usage", RunID: "airun_usage", UserID: "usr_owner",
		ModelID: "aimod_test", ModelName: "test-model",
		PromptTokens: 2_000_000, CompletionTokens: 1_000_000, TotalTokens: 3_000_000, OccurredAt: time.Now(),
		Pricing: AIModelPricingSnapshot{
			InputCreditsPerMillion: decimal.NewFromInt(1), OutputCreditsPerMillion: decimal.NewFromInt(4),
		},
	}
	if err := service.SettleAIModelUsage(input); err != nil {
		t.Fatalf("settle AI model usage: %v", err)
	}
	wallet, err := service.EnsureWallet(input.UserID)
	if err != nil {
		t.Fatalf("load wallet: %v", err)
	}
	if !wallet.BalanceCredits.Equal(decimal.NewFromInt(-6)) {
		t.Fatalf("wallet balance = %s", wallet.BalanceCredits)
	}
	var usageCount, ledgerCount int64
	if err := db.Model(&model.BillingUsageRecord{}).Count(&usageCount).Error; err != nil {
		t.Fatalf("count usage records: %v", err)
	}
	if err := db.Model(&model.BillingLedgerEntry{}).Count(&ledgerCount).Error; err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	if usageCount != 4 || ledgerCount != 4 {
		t.Fatalf("usage count = %d, ledger count = %d", usageCount, ledgerCount)
	}
	// AI 费用只归属个人，不关联项目空间：所有记录的 project_id 必须为空
	var nonEmptyProject int64
	if err := db.Model(&model.BillingUsageRecord{}).Where("meter LIKE ? AND project_id <> ''", "ai.%").Count(&nonEmptyProject).Error; err != nil {
		t.Fatalf("count project-bound AI usage: %v", err)
	}
	if nonEmptyProject != 0 {
		t.Fatalf("AI usage must not bind to a project space, got %d records", nonEmptyProject)
	}
	var ledgerWithProject int64
	if err := db.Model(&model.BillingLedgerEntry{}).Where("reason = ? AND project_id <> ''", ReasonAIUsage).Count(&ledgerWithProject).Error; err != nil {
		t.Fatalf("count project-bound AI ledger: %v", err)
	}
	if ledgerWithProject != 0 {
		t.Fatalf("AI ledger must not bind to a project space, got %d entries", ledgerWithProject)
	}
	if err := service.SettleAIModelUsage(input); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("repeat settlement error = %v", err)
	}
	if err := db.Model(&model.BillingUsageRecord{}).Count(&usageCount).Error; err != nil {
		t.Fatalf("recount usage records: %v", err)
	}
	if usageCount != 4 {
		t.Fatalf("usage count after replay = %d", usageCount)
	}
}

func TestGatewayTrafficUsageResourceIDUsesMinuteWindow(t *testing.T) {
	periodStart := time.Date(2026, 6, 21, 10, 5, 30, 0, time.FixedZone("CST", 8*3600))
	got := gatewayTrafficUsageResourceID("gwr_demo", periodStart)
	if got != "gwr_demo:202606210205" {
		t.Fatalf("resource id = %q", got)
	}
}

func TestProjectVolumeStorageUsageResourceIDUsesVolumeAndHour(t *testing.T) {
	periodStart := time.Date(2026, 8, 15, 18, 42, 0, 0, time.FixedZone("CST", 8*3600))
	if got := projectVolumeStorageUsageResourceID("pvol_demo", periodStart); got != "pvol_demo:2026081510" {
		t.Fatalf("resource id = %q", got)
	}
}

func TestSettleRuntimeTargetAggregationCreatesOneHourlyBatchAndIsIdempotent(t *testing.T) {
	db := openBillingTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.Project{}, &model.UserWallet{}, &model.BillingRateRule{},
		&model.BillingUsageRecord{}, &model.BillingLedgerEntry{},
	); err != nil {
		t.Fatalf("migrate runtime billing tables: %v", err)
	}
	user := model.User{ID: "usr_runtime_owner", Email: "runtime-owner@example.invalid", Name: "Runtime Owner", Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create runtime billing owner: %v", err)
	}
	project := model.Project{ID: "prj_runtime", Identifier: "runtime", Name: "Runtime", BillingOwnerUserID: user.ID}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create runtime billing project: %v", err)
	}
	service := Service{DB: db}
	if err := service.EnsureDefaultRateRules(); err != nil {
		t.Fatalf("seed runtime billing rates: %v", err)
	}
	periodStart := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	input := RuntimeAggregatedUsageInput{
		Context: t.Context(), ProjectID: project.ID, ApplicationID: "app_runtime",
		DeploymentTargetID: "dplt_runtime", EnvironmentID: "env_runtime",
		PeriodStart: periodStart, PeriodEnd: periodStart.Add(time.Hour),
		CPUCoreHours: decimal.RequireFromString("0.25"), MemoryGiBHours: decimal.RequireFromString("0.5"),
		CPURequestFloorCoreHours: decimal.RequireFromString("0.1"), MemoryRequestFloorGiBHours: decimal.RequireFromString("0.4"),
		CPUActualObservedCoreHours: decimal.RequireFromString("0.2"), MemoryActualObservedGiBHours: decimal.RequireFromString("0.3"),
		SampleCount: 42, MetricsSampleCount: 40, ObservedDurationSeconds: 2520, ExpectedDurationSeconds: 3600,
		ClusterResourcePolicySnapshot: []map[string]int{{"cpuRequestPercent": 10, "memoryRequestPercent": 25, "cpuLimitPercent": 100, "memoryLimitPercent": 100}},
		ActorID:                       "system",
	}
	if err := service.SettleRuntimeTargetAggregation(input); err != nil {
		t.Fatalf("settle runtime aggregation: %v", err)
	}
	if err := service.SettleRuntimeTargetAggregation(input); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("repeat runtime aggregation error = %v", err)
	}
	var usages []model.BillingUsageRecord
	if err := db.Where("resource_type = ?", ResourceTypeRuntime).Order("meter ASC").Find(&usages).Error; err != nil {
		t.Fatalf("load runtime usage records: %v", err)
	}
	if len(usages) != 2 {
		t.Fatalf("runtime usage count = %d, want 2", len(usages))
	}
	for _, usage := range usages {
		if usage.ResourceID != "dplt_runtime:2026082212" || !usage.PeriodStart.Equal(periodStart) || !usage.PeriodEnd.Equal(periodStart.Add(time.Hour)) {
			t.Fatalf("runtime usage window = %#v", usage)
		}
		var metadata map[string]any
		if err := json.Unmarshal([]byte(usage.Metadata), &metadata); err != nil {
			t.Fatalf("decode runtime usage metadata: %v", err)
		}
		if metadata["formula"] != "max_request_actual" || metadata["sampleCount"] != float64(42) || metadata["coverageRatio"] != "0.7" {
			t.Fatalf("runtime usage metadata = %#v", metadata)
		}
	}
	var ledgerCount int64
	if err := db.Model(&model.BillingLedgerEntry{}).Where("resource_type = ?", ResourceTypeRuntime).Count(&ledgerCount).Error; err != nil {
		t.Fatalf("count runtime ledger entries: %v", err)
	}
	if ledgerCount != 2 {
		t.Fatalf("runtime ledger count = %d, want 2", ledgerCount)
	}
}

func TestReferencedProjectVolumeIsNotBilledAsManagedStorage(t *testing.T) {
	service := Service{}
	err := service.SettleProjectVolumeStorageWindow(context.Background(), ProjectVolumeStorageUsageInput{
		Volume: model.ProjectVolume{
			ID: "pvol_ref", ProjectID: "prj_demo", OwnershipMode: model.ProjectVolumeOwnershipReferenced,
		},
		ObservedCapacityBytes: 10 * 1024 * 1024 * 1024,
		PeriodStart:           time.Now().Add(-time.Hour),
		PeriodEnd:             time.Now(),
	})
	if err != nil {
		t.Fatalf("referenced volume settlement error = %v", err)
	}
}

func TestManagedProjectVolumeStorageUsesObservedCapacityAsTheOnlyAuthority(t *testing.T) {
	db := openBillingTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.Project{}, &model.UserWallet{}, &model.BillingRateRule{},
		&model.BillingUsageRecord{}, &model.BillingLedgerEntry{},
	); err != nil {
		t.Fatalf("migrate authoritative storage billing tables: %v", err)
	}
	user := model.User{ID: "usr_storage_owner", Email: "storage-owner@example.invalid", Name: "Storage Owner", Role: "user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create storage billing owner: %v", err)
	}
	project := model.Project{ID: "prj_storage", Identifier: "storage", Name: "Storage", BillingOwnerUserID: user.ID}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create storage billing project: %v", err)
	}
	service := Service{DB: db}
	if err := service.EnsureDefaultRateRules(); err != nil {
		t.Fatalf("seed storage billing rate: %v", err)
	}
	periodStart := time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)
	err := service.SettleProjectVolumeStorageWindow(context.Background(), ProjectVolumeStorageUsageInput{
		Volume: model.ProjectVolume{
			ID: "pvol_authoritative", ProjectID: project.ID, OwnershipMode: model.ProjectVolumeOwnershipManaged,
		},
		ObservedCapacityBytes: 12 * 1024 * 1024 * 1024,
		PeriodStart:           periodStart,
		PeriodEnd:             periodStart.Add(time.Hour),
		ActorID:               "system",
	})
	if err != nil {
		t.Fatalf("settle authoritative project volume storage: %v", err)
	}
	var usage model.BillingUsageRecord
	if err := db.First(&usage, "meter = ?", "storage.gib_day").Error; err != nil {
		t.Fatalf("load authoritative storage usage: %v", err)
	}
	if usage.ResourceType != ResourceTypeStorage || usage.ResourceID != projectVolumeStorageUsageResourceID("pvol_authoritative", periodStart) || usage.ApplicationID != "" {
		t.Fatalf("storage usage authority = %#v", usage)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(usage.Metadata), &metadata); err != nil {
		t.Fatalf("decode storage usage metadata: %v", err)
	}
	if metadata["projectVolumeId"] != "pvol_authoritative" || metadata["capacityGiB"] != "12" || metadata["deploymentTargetId"] != "" {
		t.Fatalf("storage usage metadata = %#v", metadata)
	}
}

func TestProjectVolumeStorageSettlementPropagatesCancellationToRateLookup(t *testing.T) {
	db := openBillingTestDB(t)
	if err := db.AutoMigrate(&model.BillingRateRule{}); err != nil {
		t.Fatalf("migrate storage rate table: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	periodStart := time.Now().UTC().Add(-time.Hour)
	err := (Service{DB: db}).SettleProjectVolumeStorageWindow(ctx, ProjectVolumeStorageUsageInput{
		Volume: model.ProjectVolume{
			ID: "pvol_cancelled", ProjectID: "prj_cancelled", OwnershipMode: model.ProjectVolumeOwnershipManaged,
		},
		ObservedCapacityBytes: 1024 * 1024 * 1024,
		PeriodStart:           periodStart,
		PeriodEnd:             periodStart.Add(time.Hour),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled storage settlement error = %v, want context cancellation", err)
	}
}

func openBillingTestDB(t *testing.T) *gorm.DB {
	return testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "billing_test",
		Migrate: func(db *gorm.DB) error {
			return db.Exec(`
CREATE SCHEMA ai;
CREATE TABLE ai.model_credit_holds (
    id text PRIMARY KEY,
    owner_user_id text NOT NULL,
    state text NOT NULL,
    max_risk_credits numeric NOT NULL DEFAULT 0,
    actual_credits numeric
)`).Error
		},
	})
}
