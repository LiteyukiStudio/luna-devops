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
	"gorm.io/gorm/clause"
)

// 此测试仅使用 testdb 创建并销毁的独立 PostgreSQL 数据库。
func TestAICreditHoldAndAuthoritativeUsageSettlementPostgres(t *testing.T) {
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

	t.Run("active hold blocks racing debit", func(t *testing.T) {
		locked := make(chan struct{})
		continueHold := make(chan struct{})
		holdDone := make(chan error, 1)
		go func() {
			holdDone <- db.Transaction(func(tx *gorm.DB) error {
				var wallet model.UserWallet
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "user_id = ?", userID).Error; err != nil {
					return err
				}
				close(locked)
				<-continueHold
				return insertAIHold(tx, "aihold_racing", userID, "held", decimal.NewFromInt(80), nil)
			})
		}()
		<-locked
		debitDone := make(chan error, 1)
		go func() {
			debitDone <- service.debitUserUsages([]model.BillingUsageRecord{ordinaryUsage("busg_racing", decimal.NewFromInt(30))}, "test", "test", "system", userID)
		}()
		close(continueHold)
		if err := <-holdDone; err != nil {
			t.Fatalf("insert racing hold: %v", err)
		}
		if err := <-debitDone; !errors.Is(err, ErrReservedBalance) {
			t.Fatalf("racing debit error = %v, want ErrReservedBalance", err)
		}
		assertWalletBalance(t, db, userID, "100")
		if err := db.Exec("DELETE FROM ai.model_credit_holds WHERE id = 'aihold_racing'").Error; err != nil {
			t.Fatalf("delete test hold: %v", err)
		}
	})

	t.Run("settles only complete reported usage without double charging reasoning", func(t *testing.T) {
		actualCredits := decimal.NewFromInt(45)
		if err := insertAIHold(db, "aihold_settle", userID, "usage_recorded", decimal.NewFromInt(80), &actualCredits); err != nil {
			t.Fatalf("insert usage hold: %v", err)
		}
		if err := insertReportedUsage(db, "aiuse_settle", "aihold_settle", userID); err != nil {
			t.Fatalf("insert reported usage: %v", err)
		}
		settlementCtx, settlementSpan := otel.Tracer("luna-devops/tests").Start(t.Context(), "worker.ai_usage.settlement_request")
		settled, err := service.SettlePendingAIModelUsage(settlementCtx, 10)
		settlementSpan.End()
		if err != nil || settled != 1 {
			t.Fatalf("settle pending = (%d, %v), want (1, nil)", settled, err)
		}
		assertWalletBalance(t, db, userID, "55")

		var records []model.BillingUsageRecord
		if err := db.Where("resource_id = ?", "aiuse_settle").Order("meter").Find(&records).Error; err != nil {
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

		settled, err = service.SettlePendingAIModelUsage(t.Context(), 10)
		if err != nil || settled != 0 {
			t.Fatalf("idempotent settle = (%d, %v), want (0, nil)", settled, err)
		}
		assertWalletBalance(t, db, userID, "55")
	})

	t.Run("reconciliation hold never enters settlement", func(t *testing.T) {
		if err := insertAIHold(db, "aihold_unknown", userID, "reconciliation_required", decimal.NewFromInt(20), nil); err != nil {
			t.Fatalf("insert reconciliation hold: %v", err)
		}
		settled, err := service.SettlePendingAIModelUsage(t.Context(), 10)
		if err != nil || settled != 0 {
			t.Fatalf("settle unavailable usage = (%d, %v), want (0, nil)", settled, err)
		}
		assertWalletBalance(t, db, userID, "55")
	})

	t.Run("failed settlement keeps authoritative usage pending", func(t *testing.T) {
		actualCredits := decimal.NewFromInt(45)
		if err := insertAIHold(db, "aihold_failed", userID, "usage_recorded", decimal.NewFromInt(80), &actualCredits); err != nil {
			t.Fatalf("insert failed-settlement hold: %v", err)
		}
		if err := insertReportedUsage(db, "aiuse_failed", "aihold_failed", userID); err != nil {
			t.Fatalf("insert failed-settlement usage: %v", err)
		}
		blockedCredits := decimal.NewFromInt(50)
		if err := insertAIHold(db, "aihold_blocking", userID, "reconciliation_required", blockedCredits, nil); err != nil {
			t.Fatalf("insert blocking reconciliation hold: %v", err)
		}

		settlementCtx, settlementSpan := otel.Tracer("luna-devops/tests").Start(t.Context(), "worker.ai_usage.failed_settlement_request")
		settled, err := service.SettlePendingAIModelUsage(settlementCtx, 10)
		settlementSpan.End()
		if settled != 0 || !errors.Is(err, ErrAIUsageSettlementFailed) {
			t.Fatalf("failed settlement = (%d, %v), want (0, ErrAIUsageSettlementFailed)", settled, err)
		}
		var status string
		if err := db.Raw("SELECT settlement_status FROM ai.model_usages WHERE id = 'aiuse_failed'").Scan(&status).Error; err != nil {
			t.Fatalf("read failed usage: %v", err)
		}
		if status != "pending" {
			t.Fatalf("failed usage status = %s, want pending", status)
		}
	})
}

func insertAIHold(db *gorm.DB, holdID, userID, state string, maxRisk decimal.Decimal, actual *decimal.Decimal) error {
	return db.Exec(`INSERT INTO ai.model_credit_holds (
id, run_id, owner_user_id, operation, attempt, state, model_id, model_name,
max_context_tokens_snapshot, max_output_tokens_snapshot,
input_credits_per_million, output_credits_per_million, cached_input_credits_per_million,
max_risk_credits, actual_credits, expires_at
) VALUES (?, 'airun_ai_usage_pg', ?, 'assistant',
  (SELECT COALESCE(MAX(attempt), 0) + 1 FROM ai.model_credit_holds WHERE run_id = 'airun_ai_usage_pg' AND operation = 'assistant'),
  ?, 'aimod_ai_usage_pg', 'usage', 4096, 512, 2000000, 4000000, 1000000, ?, ?, now() + interval '1 hour')`,
		holdID, userID, state, maxRisk, actual).Error
}

func insertReportedUsage(db *gorm.DB, usageID, holdID, userID string) error {
	return db.Exec(`INSERT INTO ai.model_usages (
id, credit_hold_id, run_id, owner_user_id, operation, attempt, status, settlement_status,
model_id, model_name, max_context_tokens_snapshot, prompt_tokens, completion_tokens, total_tokens,
cached_prompt_tokens, cache_write_prompt_tokens, reasoning_completion_tokens, call_type
) SELECT ?, ?, run_id, ?, operation, attempt, 'reported', 'pending', model_id, model_name,
max_context_tokens_snapshot, 10, 8, 18, 4, 3, 5, 'stream'
FROM ai.model_credit_holds WHERE id = ?`, usageID, holdID, userID, holdID).Error
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
