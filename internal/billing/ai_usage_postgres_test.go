package billing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// These tests create and force-drop a luna_billing_budget_test_* database.
// They never use schemas or tables from AUTH_TEST_DATABASE_URL itself.
func TestAIReservationWalletAndSettlementPostgres(t *testing.T) {
	db := openIsolatedBillingBudgetDB(t)
	service := Service{DB: db}
	userID := "usr_budget_pg"
	seedBudgetUserRun(t, db, userID, decimal.NewFromInt(100))

	t.Run("hold blocks a racing ordinary debit and negative adjustment", func(t *testing.T) {
		locked := make(chan struct{})
		continueReservation := make(chan struct{})
		reservationDone := make(chan error, 1)
		go func() {
			reservationDone <- db.Transaction(func(tx *gorm.DB) error {
				var wallet model.UserWallet
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&wallet, "user_id = ?", userID).Error; err != nil {
					return err
				}
				close(locked)
				<-continueReservation
				return insertBudgetReservation(tx, "aibgt_racing", userID, "reserved", decimal.NewFromInt(80))
			})
		}()
		<-locked
		debitDone := make(chan error, 1)
		go func() {
			usage := budgetTestUsage("busg_racing", "ordinary", decimal.NewFromInt(30))
			debitDone <- service.debitUserUsages([]model.BillingUsageRecord{usage}, "test", "test", "system", userID)
		}()
		close(continueReservation)
		if err := <-reservationDone; err != nil {
			t.Fatalf("insert racing reservation: %v", err)
		}
		if err := <-debitDone; !errors.Is(err, ErrReservedBalance) {
			t.Fatalf("racing ordinary debit error = %v, want ErrReservedBalance", err)
		}
		_, err := service.ApplyWalletTransaction(WalletTransactionInput{
			UserID: userID, Type: "adjustment", AmountCredits: decimal.NewFromInt(-30), ActorID: "admin",
		})
		if !errors.Is(err, ErrReservedBalance) {
			t.Fatalf("negative adjustment error = %v, want ErrReservedBalance", err)
		}
		assertWalletBalance(t, db, userID, "100")
		if err := db.Exec("DELETE FROM ai.model_budget_reservations WHERE id = 'aibgt_racing'").Error; err != nil {
			t.Fatalf("delete test reservation: %v", err)
		}
	})

	t.Run("settlement excludes its own hold and is four-price idempotent", func(t *testing.T) {
		if err := insertBudgetReservation(db, "aibgt_settle", userID, "confirmed", decimal.NewFromInt(45)); err != nil {
			t.Fatalf("insert confirmed reservation: %v", err)
		}
		settled, err := service.SettlePendingAIModelUsage(t.Context(), 10)
		if err != nil || settled != 1 {
			t.Fatalf("settle pending = (%d, %v), want (1, nil)", settled, err)
		}
		assertWalletBalance(t, db, userID, "55")

		var records []model.BillingUsageRecord
		if err := db.Where("resource_id = ?", "aibgt_settle").Order("meter").Find(&records).Error; err != nil {
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
			MeterAIInputTokens: "12", MeterAIOutputTokens: "20",
			MeterAICachedInputTokens: "4", MeterAICachedOutputTokens: "9",
		}
		for meter, amount := range want {
			if amounts[meter] != amount {
				t.Fatalf("%s amount = %s, want %s", meter, amounts[meter], amount)
			}
		}

		if err := db.Exec("UPDATE ai.model_budget_reservations SET state = 'confirmed' WHERE id = ?", "aibgt_settle").Error; err != nil {
			t.Fatalf("restore confirmed state: %v", err)
		}
		settled, err = service.SettlePendingAIModelUsage(t.Context(), 10)
		if err != nil || settled != 1 {
			t.Fatalf("idempotent settle = (%d, %v), want (1, nil)", settled, err)
		}
		assertWalletBalance(t, db, userID, "55")
		var count int64
		if err := db.Model(&model.BillingUsageRecord{}).Where("resource_id = ?", "aibgt_settle").Count(&count).Error; err != nil || count != 4 {
			t.Fatalf("idempotent usage count = %d, err=%v, want 4", count, err)
		}
	})

	t.Run("settlement failure is stable and does not expose the reservation id", func(t *testing.T) {
		const reservationID = "aibgt_high_cardinality_secret_marker"
		if err := insertBudgetReservation(db, reservationID, userID, "confirmed", decimal.NewFromInt(45)); err != nil {
			t.Fatalf("insert failing reservation: %v", err)
		}
		const constraint = "billing_usage_records_budget_test_reject"
		if err := db.Exec(`ALTER TABLE billing_usage_records ADD CONSTRAINT ` + constraint +
			` CHECK (resource_id <> '` + reservationID + `')`).Error; err != nil {
			t.Fatalf("install isolated settlement failure constraint: %v", err)
		}
		defer func() {
			if err := db.Exec(`ALTER TABLE billing_usage_records DROP CONSTRAINT IF EXISTS ` + constraint).Error; err != nil {
				t.Errorf("remove isolated settlement failure constraint: %v", err)
			}
		}()
		settled, err := service.SettlePendingAIModelUsage(t.Context(), 10)
		if settled != 0 || !errors.Is(err, ErrAIUsageSettlementFailed) {
			t.Fatalf("failed settlement = (%d, %v), want stable settlement failure", settled, err)
		}
		if strings.Contains(err.Error(), reservationID) {
			t.Fatalf("settlement error exposed reservation id: %v", err)
		}
	})
}

func insertBudgetReservation(db *gorm.DB, reservationID, userID, state string, credits decimal.Decimal) error {
	confirmed := state == "confirmed" || state == "settled"
	query := `INSERT INTO ai.model_budget_reservations (
id, run_id, owner_user_id, operation, state, model_id, model_name,
input_credits_per_million, output_credits_per_million,
cached_input_credits_per_million, cached_output_credits_per_million,
reserved_tokens, reserved_input_tokens, reserved_output_tokens, reserved_credits,
confirmed_tokens, confirmed_credits, input_tokens, output_tokens,
cached_input_tokens, cached_output_tokens, expires_at
) VALUES (?, 'airun_budget_pg', ?, 'assistant', ?, 'aimod_budget_pg', 'budget',
2000000, 4000000, 1000000, 3000000,
18, 10, 8, ?, ?, ?, ?, ?, ?, ?, now() + interval '1 hour')`
	if confirmed {
		return db.Exec(query, reservationID, userID, state, credits, 18, credits, 10, 8, 4, 3).Error
	}
	return db.Exec(query, reservationID, userID, state, credits, nil, nil, nil, nil, nil, nil).Error
}

func seedBudgetUserRun(t *testing.T, db *gorm.DB, userID string, balance decimal.Decimal) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO users(id, email, name) VALUES (?, ?, 'Budget Test')", []any{userID, userID + "@example.test"}},
		{"INSERT INTO user_wallets(id, user_id, balance_credits) VALUES ('wlt_budget_pg', ?, ?)", []any{userID, balance}},
		{"INSERT INTO ai.conversations(id, owner_user_id, title) VALUES ('aicnv_budget_pg', ?, 'Budget')", []any{userID}},
		{"INSERT INTO ai.turns(id, conversation_id, turn_index, status, input, selected_run_id) VALUES ('aitrn_budget_pg', 'aicnv_budget_pg', 0, 'queued', 'budget', 'airun_budget_pg')", nil},
		{`INSERT INTO ai.runs(id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest,
model_id, model_name, input_credits_per_million, output_credits_per_million,
cached_input_credits_per_million, cached_output_credits_per_million,
max_context_tokens, max_output_tokens, total_token_budget, total_credit_budget, actor_session_id)
VALUES ('airun_budget_pg', ?, 'aicnv_budget_pg', 'aitrn_budget_pg', 0, 'running', 'test', 'test',
'aimod_budget_pg', 'budget', 2000000, 4000000, 1000000, 3000000, 4096, 512, 10000, 1000, 'ses_budget_pg')`, []any{userID}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed budget database: %v", err)
		}
	}
}

func budgetTestUsage(id, resourceID string, amount decimal.Decimal) model.BillingUsageRecord {
	now := time.Now()
	return model.BillingUsageRecord{
		ID: id, ProjectID: "", Meter: "test.ordinary", Quantity: decimal.NewFromInt(1), Unit: "test",
		AmountCredits: amount, ResourceType: "ordinary", ResourceID: resourceID,
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

func openIsolatedBillingBudgetDB(t *testing.T) *gorm.DB {
	return testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "luna_billing_budget_test",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})
}
