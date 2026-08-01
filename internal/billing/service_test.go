package billing

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDeploymentTargetStorageGiBSumsDataVolumes(t *testing.T) {
	target := model.DeploymentTarget{
		DataRetentionEnabled: true,
		DataCapacity:         "1Gi",
		DataVolumes:          `[{"name":"app1","mountPath":"/data/app1","capacity":"20Gi"},{"name":"app2","mountPath":"/data/app2","capacity":"40Gi"}]`,
	}
	got := deploymentTargetStorageGiB(target)
	if !got.Equal(decimalFromInt(60)) {
		t.Fatalf("storage GiB = %s", got)
	}
}

func TestDeploymentTargetStorageGiBFallsBackToPrimaryCapacity(t *testing.T) {
	target := model.DeploymentTarget{DataRetentionEnabled: true, DataCapacity: "5Gi"}
	got := deploymentTargetStorageGiB(target)
	if !got.Equal(decimalFromInt(5)) {
		t.Fatalf("storage GiB = %s", got)
	}
}

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

func TestDefaultRateRulesIncludeAIInputAndOutputTokens(t *testing.T) {
	rules := defaultRateRuleByMeter()
	input := rules[MeterAIInputTokens]
	if input.Unit != "1000_tokens" || !input.Enabled || !input.CreditsPerUnit.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("AI input token rule = %#v", input)
	}
	output := rules[MeterAIOutputTokens]
	if output.Unit != "1000_tokens" || !output.Enabled || !output.CreditsPerUnit.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("AI output token rule = %#v", output)
	}
}

func TestTokenBillingQuantityPreservesPartialThousands(t *testing.T) {
	if got := tokenBillingQuantity(1250); !got.Equal(decimal.RequireFromString("1.25")) {
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
		InputTokens: 2000, OutputTokens: 1000, OccurredAt: time.Now(),
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
	if usageCount != 2 || ledgerCount != 2 {
		t.Fatalf("usage count = %d, ledger count = %d", usageCount, ledgerCount)
	}
	if err := service.SettleAIModelUsage(input); !errors.Is(err, ErrAlreadySettled) {
		t.Fatalf("repeat settlement error = %v", err)
	}
	if err := db.Model(&model.BillingUsageRecord{}).Count(&usageCount).Error; err != nil {
		t.Fatalf("recount usage records: %v", err)
	}
	if usageCount != 2 {
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

func decimalFromInt(value int64) decimal.Decimal {
	return decimal.NewFromInt(value)
}

func openBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("billing_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
