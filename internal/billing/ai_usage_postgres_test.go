package billing

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
)

// 此测试仅使用 testdb 创建并销毁的独立 PostgreSQL 数据库。
func TestAuthoritativeAIUsageSettlementPostgres(t *testing.T) {
	if os.Getenv("OTEL_SMOKE") == "true" {
		startupConfig, err := sharedconfig.LoadTelemetry()
		if err != nil {
			t.Fatalf("load OTel smoke configuration: %v", err)
		}
		runtime, err := telemetry.Setup(t.Context(), telemetry.ServiceConfig{
			ServiceName:        "luna-worker-ai-usage-smoke",
			ServiceVersion:     "test",
			Endpoint:           startupConfig.Endpoint,
			Headers:            startupConfig.Headers,
			ResourceAttributes: startupConfig.ResourceAttributes,
			LogFormat:          startupConfig.LogFormat,
			LogColor:           startupConfig.LogColor,
			LogLevel:           startupConfig.LogLevel,
			NoColor:            startupConfig.NoColor,
		})
		if err != nil {
			t.Fatalf("setup OTel smoke runtime: %v", err)
		}
		t.Cleanup(func() {
			if shutdownErr := runtime.Shutdown(context.Background()); shutdownErr != nil {
				t.Errorf("shutdown OTel smoke runtime: %v", shutdownErr)
			}
		})
	}
	db := openIsolatedAIUsageDB(t)
	service := Service{DB: db}
	userID := "usr_ai_usage_pg"
	seedAIUsageUserRun(t, db, userID, decimal.NewFromInt(100))

	t.Run("unsettled AI usage is reflected in summary and guards manual negative adjustment without blocking postpaid debit", func(t *testing.T) {
		if err := insertReportedUsage(db, "aiuse_pending_summary", userID, 10, 8, 18); err != nil {
			t.Fatalf("insert pending reported usage: %v", err)
		}
		summary, err := service.Summary(SummaryInput{UserIDs: []string{userID}})
		if err != nil {
			t.Fatalf("read user billing summary: %v", err)
		}
		if summary.PendingSpend.String() != "45" || summary.AvailableCredits.String() != "55" {
			t.Fatalf("summary = %+v, want pending=45 available=55", summary)
		}
		projectSummary, err := service.Summary(SummaryInput{UserIDs: []string{userID}, ProjectIDs: []string{"prj_isolated"}})
		if err != nil {
			t.Fatalf("read project billing summary: %v", err)
		}
		if !projectSummary.PendingSpend.IsZero() {
			t.Fatalf("project summary pending spend = %s, want 0 (AI costs never attach to projects)", projectSummary.PendingSpend)
		}

		if _, err := service.ApplyWalletTransaction(WalletTransactionInput{
			UserID:        userID,
			Type:          "adjustment",
			AmountCredits: decimal.NewFromInt(-60),
			ActorID:       "usr_admin",
		}); !errors.Is(err, ErrReservedBalance) {
			t.Fatalf("negative manual adjustment error = %v, want ErrReservedBalance", err)
		}

		if err := service.debitUserUsages([]model.BillingUsageRecord{ordinaryUsage("busg_postpaid", decimal.NewFromInt(30))}, "runtime.usage", "Runtime usage", "system", userID); err != nil {
			t.Fatalf("postpaid debit should not be blocked by pending AI usage: %v", err)
		}
		assertWalletBalance(t, db, userID, "70")
	})

	t.Run("batch settles multiple usages per user in one wallet update and allows single-turn overdraft", func(t *testing.T) {
		// Current wallet = 70; aiuse_pending_summary costs 45, aiuse_overdraft costs 45 => total 90 => balance becomes -20.
		if err := insertReportedUsage(db, "aiuse_overdraft", userID, 10, 20, 40); err != nil {
			t.Fatalf("insert second reported usage (total_tokens > prompt + completion): %v", err)
		}
		// Delete conversation before Worker settles to prove FK decoupling preserves immutable ai.model_usages.
		if err := db.Exec("DELETE FROM ai.conversations WHERE id = 'aicnv_ai_usage_pg'").Error; err != nil {
			t.Fatalf("delete conversation with reported usage: %v", err)
		}

		settlementCtx, settlementSpan := otel.Tracer("luna-devops/tests").Start(t.Context(), "worker.ai_usage.settlement_request")
		settled, err := service.SettlePendingAIModelUsage(settlementCtx, 10)
		settlementSpan.End()
		if err != nil || settled != 2 {
			t.Fatalf("settle pending = (%d, %v), want (2, nil)", settled, err)
		}
		// aiuse_pending_summary = 45, aiuse_overdraft = (3*2 + 7*1 + 20*4) = 93 => 70 - 45 - 93 = -68
		assertWalletBalance(t, db, userID, "-68")

		var records []model.BillingUsageRecord
		if err := db.Where("resource_id = ?", "aiuse_pending_summary").Order("meter").Find(&records).Error; err != nil {
			t.Fatalf("list usage records: %v", err)
		}
		if len(records) != 4 {
			t.Fatalf("usage record count = %d, want 4", len(records))
		}
		amounts := map[string]string{}
		for _, record := range records {
			if record.ProjectID != "" {
				t.Fatalf("AI usage project_id = %q, want empty", record.ProjectID)
			}
			amounts[record.Meter] = record.AmountCredits.String()
		}
		want := map[string]string{
			MeterAIPromptTokens: "6", MeterAICompletionTokens: "32",
			MeterAICachedPromptTokens: "4", MeterAICacheWritePromptTokens: "3",
		}
		for meter, amount := range want {
			if amounts[meter] != amount {
				t.Fatalf("%s amount = %s, want %s", meter, amounts[meter], amount)
			}
		}

		summary, err := service.Summary(SummaryInput{UserIDs: []string{userID}})
		if err != nil {
			t.Fatalf("read post-overdraft summary: %v", err)
		}
		if !summary.PendingSpend.IsZero() || summary.AvailableCredits.String() != "-68" || summary.BalanceStatus != "insufficient" {
			t.Fatalf("post-overdraft summary = %+v, want pending=0 available=-68 status=insufficient", summary)
		}

		settled, err = service.SettlePendingAIModelUsage(t.Context(), 10)
		if err != nil || settled != 0 {
			t.Fatalf("idempotent settle = (%d, %v), want (0, nil)", settled, err)
		}
	})

	t.Run("malformed usage row stays pending without blocking valid sibling usage", func(t *testing.T) {
		if err := insertReportedUsageWithModel(db, "aiuse_invalid_model", userID, "", 10, 8, 18); err != nil {
			t.Fatalf("insert malformed model usage: %v", err)
		}
		if err := insertReportedUsage(db, "aiuse_valid_sibling", userID, 10, 8, 18); err != nil {
			t.Fatalf("insert valid sibling usage: %v", err)
		}

		settled, err := service.SettlePendingAIModelUsage(t.Context(), 10)
		if settled != 1 || !errors.Is(err, ErrAIUsageSettlementFailed) {
			t.Fatalf("partial settlement = (%d, %v), want (1, ErrAIUsageSettlementFailed)", settled, err)
		}
		var invalidStatus, validStatus string
		if err := db.Raw("SELECT settlement_status FROM ai.model_usages WHERE id = 'aiuse_invalid_model'").Scan(&invalidStatus).Error; err != nil {
			t.Fatalf("read invalid usage status: %v", err)
		}
		if err := db.Raw("SELECT settlement_status FROM ai.model_usages WHERE id = 'aiuse_valid_sibling'").Scan(&validStatus).Error; err != nil {
			t.Fatalf("read valid usage status: %v", err)
		}
		if invalidStatus != "pending" || validStatus != "settled" {
			t.Fatalf("statuses = (%s, %s), want (pending, settled)", invalidStatus, validStatus)
		}
		assertWalletBalance(t, db, userID, "-113")
	})

	t.Run("zero-token and partial-meter AI usages skip zero-quantity ledger rows while marking settlement complete", func(t *testing.T) {
		if err := db.Exec(`INSERT INTO ai.model_usages (
			id, run_id, owner_user_id, operation, attempt, status, settlement_status,
			model_id, model_name, max_context_tokens_snapshot, max_output_tokens_snapshot,
			input_credits_per_million, output_credits_per_million, cached_input_credits_per_million,
			prompt_tokens, completion_tokens, total_tokens,
			cached_prompt_tokens, cache_write_prompt_tokens, reasoning_completion_tokens, call_type
		) VALUES
			('aiuse_zero_tokens', 'airun_ai_usage_pg', ?, 'assistant', 99, 'reported', 'pending', 'aimod_ai_usage_pg', 'usage', 4096, 512, 2000000, 4000000, 1000000, 0, 0, 0, 0, 0, 0, 'stream'),
			('aiuse_no_cache', 'airun_ai_usage_pg', ?, 'assistant', 100, 'reported', 'pending', 'aimod_ai_usage_pg', 'usage', 4096, 512, 2000000, 4000000, 1000000, 5, 2, 7, 0, 0, 0, 'stream')`,
			userID, userID,
		).Error; err != nil {
			t.Fatalf("insert zero/no-cache usages: %v", err)
		}
		// Remove the malformed row from the previous subtest so batch settlement returns nil error.
		if err := db.Exec("DELETE FROM ai.model_usages WHERE id = 'aiuse_invalid_model'").Error; err != nil {
			t.Fatalf("cleanup invalid model usage: %v", err)
		}

		settled, err := service.SettlePendingAIModelUsage(t.Context(), 10)
		if err != nil || settled != 2 {
			t.Fatalf("settle zero/no-cache = (%d, %v), want (2, nil)", settled, err)
		}
		var zeroStatus string
		if err := db.Raw("SELECT settlement_status FROM ai.model_usages WHERE id = 'aiuse_zero_tokens'").Scan(&zeroStatus).Error; err != nil {
			t.Fatal(err)
		}
		if zeroStatus != "settled" {
			t.Fatalf("zero-token settlement_status = %q, want settled", zeroStatus)
		}
		var zeroRecordCount, noCacheRecordCount int64
		if err := db.Model(&model.BillingUsageRecord{}).Where("resource_id = 'aiuse_zero_tokens'").Count(&zeroRecordCount).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&model.BillingUsageRecord{}).Where("resource_id = 'aiuse_no_cache'").Count(&noCacheRecordCount).Error; err != nil {
			t.Fatal(err)
		}
		if zeroRecordCount != 0 || noCacheRecordCount != 2 {
			t.Fatalf("record counts = (%d, %d), want (0, 2)", zeroRecordCount, noCacheRecordCount)
		}
		assertWalletBalance(t, db, userID, "-131")
	})
}

func insertReportedUsage(db *gorm.DB, usageID, userID string, promptTokens, completionTokens, totalTokens int64) error {
	return insertReportedUsageWithModel(db, usageID, userID, "usage", promptTokens, completionTokens, totalTokens)
}

func insertReportedUsageWithModel(db *gorm.DB, usageID, userID, modelName string, promptTokens, completionTokens, totalTokens int64) error {
	return db.Exec(`INSERT INTO ai.model_usages (
id, run_id, owner_user_id, operation, attempt, status, settlement_status,
model_id, model_name, max_context_tokens_snapshot, max_output_tokens_snapshot,
input_credits_per_million, output_credits_per_million, cached_input_credits_per_million,
prompt_tokens, completion_tokens, total_tokens,
cached_prompt_tokens, cache_write_prompt_tokens, reasoning_completion_tokens, call_type
) VALUES (?, 'airun_ai_usage_pg', ?, 'assistant',
  (SELECT COALESCE(MAX(attempt), 0) + 1 FROM ai.model_usages WHERE run_id = 'airun_ai_usage_pg' AND operation = 'assistant'),
  'reported', 'pending', 'aimod_ai_usage_pg', ?, 4096, 512, 2000000, 4000000, 1000000,
  ?, ?, ?, 4, 3, 5, 'stream')`,
		usageID, userID, modelName, promptTokens, completionTokens, totalTokens).Error
}

func seedAIUsageUserRun(t *testing.T, db *gorm.DB, userID string, balance decimal.Decimal) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO users(id, email, name) VALUES (?, ?, 'Usage Test')", []any{userID, userID + "@example.test"}},
		{"INSERT INTO user_wallets(id, user_id, balance_credits) VALUES ('wlt_ai_usage_pg', ?, ?)", []any{userID, balance}},
		{"INSERT INTO ai.conversations(id, owner_user_id, title) VALUES ('aicnv_ai_usage_pg', ?, 'Usage')", []any{userID}},
		{"INSERT INTO ai.turns(id, conversation_id, turn_index, status, input, selected_run_id) VALUES ('aitrn_ai_usage_pg', 'aicnv_ai_usage_pg', 0, 'queued', 'usage', 'airun_ai_usage_pg')", nil},
		{`INSERT INTO ai.runs(id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest,
model_id, model_name, input_credits_per_million, output_credits_per_million,
cached_input_credits_per_million, max_context_tokens, max_output_tokens, actor_session_id)
VALUES ('airun_ai_usage_pg', ?, 'aicnv_ai_usage_pg', 'aitrn_ai_usage_pg', 0, 'running', 'test', 'test',
'aimod_ai_usage_pg', 'usage', 2000000, 4000000, 1000000, 4096, 512, 'ses_ai_usage_pg')`, []any{userID}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed usage database: %v", err)
		}
	}
}

func ordinaryUsage(id string, amount decimal.Decimal) model.BillingUsageRecord {
	now := time.Now()
	return model.BillingUsageRecord{
		ID: id, ProjectID: "", Meter: "test.ordinary", Quantity: decimal.NewFromInt(1), Unit: "test",
		AmountCredits: amount, ResourceType: "ordinary", ResourceID: id,
		PeriodStart: now, PeriodEnd: now.Add(time.Second), Status: "settled", SettledAt: &now,
	}
}

func assertWalletBalance(t *testing.T, db *gorm.DB, userID, want string) {
	t.Helper()
	var wallet model.UserWallet
	if err := db.First(&wallet, "user_id = ?", userID).Error; err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	if wallet.BalanceCredits.String() != want {
		t.Fatalf("wallet balance = %s, want %s", wallet.BalanceCredits, want)
	}
}

func openIsolatedAIUsageDB(t *testing.T) *gorm.DB {
	return testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_ai_usage_test",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})
}
