package retention

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestCleanupEnforcesTerminalParentsInPostgres(t *testing.T) {
	db := openRetentionTestDB(t)
	if err := db.Exec(`
CREATE TABLE platform_events (id text PRIMARY KEY, occurred_at timestamptz NOT NULL);
CREATE TABLE notification_deliveries (id text PRIMARY KEY, status text NOT NULL, finished_at timestamptz);
CREATE TABLE build_runs (id text PRIMARY KEY, status text NOT NULL, finished_at timestamptz);
CREATE TABLE build_logs (id text PRIMARY KEY, build_run_id text NOT NULL, created_at timestamptz NOT NULL);
CREATE TABLE releases (id text PRIMARY KEY, status text NOT NULL, finished_at timestamptz);
CREATE TABLE release_logs (id text PRIMARY KEY, release_id text NOT NULL, created_at timestamptz NOT NULL);
CREATE TABLE hook_runs (id text PRIMARY KEY, status text NOT NULL, finished_at timestamptz);
CREATE TABLE hook_run_logs (id text PRIMARY KEY, hook_run_id text NOT NULL, created_at timestamptz NOT NULL);
`).Error; err != nil {
		t.Fatalf("create retention tables: %v", err)
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	old := now.Add(-36 * time.Hour)
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO platform_events(id, occurred_at) VALUES (?, ?)", []any{"event_old", old}},
		{"INSERT INTO notification_deliveries(id, status, finished_at) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)", []any{"delivery_ok", "succeeded", old, "delivery_failed", "failed", old, "delivery_pending", "pending", old}},
		{"INSERT INTO build_runs(id, status, finished_at) VALUES (?, ?, ?), (?, ?, ?)", []any{"build_done", "succeeded", old, "build_running", "running", old}},
		{"INSERT INTO build_logs(id, build_run_id, created_at) VALUES (?, ?, ?), (?, ?, ?)", []any{"build_log_done", "build_done", old, "build_log_running", "build_running", old}},
		{"INSERT INTO releases(id, status, finished_at) VALUES (?, ?, ?), (?, ?, ?)", []any{"release_done", "failed", old, "release_pending", "pending", old}},
		{"INSERT INTO release_logs(id, release_id, created_at) VALUES (?, ?, ?), (?, ?, ?)", []any{"release_log_done", "release_done", old, "release_log_pending", "release_pending", old}},
		{"INSERT INTO hook_runs(id, status, finished_at) VALUES (?, ?, ?), (?, ?, ?)", []any{"hook_done", "succeeded", old, "hook_queued", "queued", old}},
		{"INSERT INTO hook_run_logs(id, hook_run_id, created_at) VALUES (?, ?, ?), (?, ?, ?)", []any{"hook_log_done", "hook_done", old, "hook_log_queued", "hook_queued", old}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("seed retention data: %v", err)
		}
	}

	datasets := []string{
		DatasetPlatformEvents,
		DatasetNotificationDeliveries,
		DatasetBuildLogs,
		DatasetReleaseLogs,
		DatasetHookRunLogs,
	}
	results, err := NewService(db).Cleanup(t.Context(), datasets, now.Add(-48*time.Hour), now, now)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	wantMatched := map[string]int64{
		DatasetPlatformEvents: 1, DatasetNotificationDeliveries: 2,
		DatasetBuildLogs: 1, DatasetReleaseLogs: 1, DatasetHookRunLogs: 1,
	}
	for _, result := range results {
		if result.Matched != wantMatched[result.Dataset] || result.Deleted != wantMatched[result.Dataset] {
			t.Fatalf("result for %s = %#v, want matched/deleted %d", result.Dataset, result, wantMatched[result.Dataset])
		}
	}

	assertRowCount(t, db, "notification_deliveries", 1)
	assertRowCount(t, db, "build_logs", 1)
	assertRowCount(t, db, "release_logs", 1)
	assertRowCount(t, db, "hook_run_logs", 1)
	assertRowCount(t, db, "build_runs", 2)
	assertRowCount(t, db, "releases", 2)
	assertRowCount(t, db, "hook_runs", 2)
}

func TestExpiredAuthCleanupPreservesFutureRecordsInPostgres(t *testing.T) {
	db := openRetentionTestDB(t)
	if err := db.Exec(`
CREATE TABLE user_sessions (id text PRIMARY KEY, expires_at timestamptz NOT NULL);
CREATE TABLE email_registration_challenges (id text PRIMARY KEY, expires_at timestamptz NOT NULL);
CREATE TABLE oauth_applications (id text PRIMARY KEY, registration_source text NOT NULL, created_at timestamptz NOT NULL);
CREATE TABLE oauth_grants (id text PRIMARY KEY, application_id text NOT NULL);
CREATE TABLE access_tokens (id text PRIMARY KEY, oauth_application_id text NOT NULL);
CREATE TABLE oauth_authorization_codes (id text PRIMARY KEY, application_id text NOT NULL, expires_at timestamptz NOT NULL);
CREATE TABLE oauth_refresh_tokens (id text PRIMARY KEY, application_id text NOT NULL, expires_at timestamptz NOT NULL);
`).Error; err != nil {
		t.Fatalf("create auth retention tables: %v", err)
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)
	recent := now.Add(-2 * time.Hour)
	future := now.Add(24 * time.Hour)
	if err := db.Exec("INSERT INTO user_sessions(id, expires_at) VALUES (?, ?), (?, ?)",
		"session_old", old, "session_future", future).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO email_registration_challenges(id, expires_at) VALUES (?, ?), (?, ?)",
		"registration_old", old, "registration_future", future).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(
		"INSERT INTO oauth_applications(id, registration_source, created_at) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?), (?, ?, ?)",
		"oapp_dyn_abandoned", "dynamic", old,
		"oapp_dyn_with_grant", "dynamic", old,
		"oapp_dyn_recent", "dynamic", recent,
		"oapp_manual", "manual", old,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO oauth_grants(id, application_id) VALUES (?, ?)", "ogrt_active", "oapp_dyn_with_grant").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO oauth_authorization_codes(id, application_id, expires_at) VALUES (?, ?, ?), (?, ?, ?)",
		"ocod_old", "oapp_manual", old, "ocod_future", "oapp_manual", future).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO oauth_refresh_tokens(id, application_id, expires_at) VALUES (?, ?, ?), (?, ?, ?)",
		"ortk_old", "oapp_manual", old, "ortk_future", "oapp_manual", future).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(db)
	start := now.AddDate(0, 0, -60)
	end := now.AddDate(0, 0, 2)
	preview, err := service.Preview(t.Context(), []string{DatasetExpiredAuthData}, start, end, now)
	if err != nil {
		t.Fatalf("preview auth cleanup: %v", err)
	}
	if len(preview) != 1 || preview[0].Matched != 5 || preview[0].Deleted != 0 {
		t.Fatalf("auth preview = %#v, want 5 matched", preview)
	}
	results, err := service.Cleanup(t.Context(), []string{DatasetExpiredAuthData}, start, end, now)
	if err != nil {
		t.Fatalf("cleanup auth data: %v", err)
	}
	if len(results) != 1 || results[0].Matched != 5 || results[0].Deleted != 5 {
		t.Fatalf("auth cleanup = %#v, want 5 matched/deleted", results)
	}

	assertRowCount(t, db, "user_sessions", 1)
	assertRowCount(t, db, "email_registration_challenges", 1)
	assertRowCount(t, db, "oauth_authorization_codes", 1)
	assertRowCount(t, db, "oauth_refresh_tokens", 1)
	assertRowCount(t, db, "oauth_applications", 3)
	assertIDExists(t, db, "user_sessions", "session_future")
	assertIDExists(t, db, "email_registration_challenges", "registration_future")
	assertIDExists(t, db, "oauth_authorization_codes", "ocod_future")
	assertIDExists(t, db, "oauth_refresh_tokens", "ortk_future")
	assertIDExists(t, db, "oauth_applications", "oapp_dyn_with_grant")
	assertIDExists(t, db, "oauth_applications", "oapp_dyn_recent")
	assertIDExists(t, db, "oauth_applications", "oapp_manual")
}

func TestRunAutomaticReadsConfigsAndHonorsZeroInPostgres(t *testing.T) {
	db := openRetentionTestDB(t)
	if err := db.Exec(`
CREATE TABLE app_configs (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE platform_events (id text PRIMARY KEY, occurred_at timestamptz NOT NULL);
`).Error; err != nil {
		t.Fatalf("create automatic retention tables: %v", err)
	}
	for _, dataset := range catalog {
		value := "0"
		if dataset.Key == DatasetPlatformEvents {
			value = "90"
		}
		if err := db.Exec("INSERT INTO app_configs(key, value) VALUES (?, ?)", dataset.ConfigKey, value).Error; err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	if err := db.Exec("INSERT INTO platform_events(id, occurred_at) VALUES (?, ?), (?, ?)",
		"event_old", now.AddDate(0, 0, -91), "event_recent", now.AddDate(0, 0, -89)).Error; err != nil {
		t.Fatal(err)
	}
	results, err := NewService(db).RunAutomatic(t.Context(), now)
	if err != nil {
		t.Fatalf("run automatic retention: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("automatic results = %d, want one enabled dataset", len(results))
	}
	for _, result := range results {
		if result.Dataset != DatasetPlatformEvents || result.Matched != 1 || result.Deleted != 1 {
			t.Fatalf("platform event automatic result = %#v", result)
		}
	}
	assertRowCount(t, db, "platform_events", 1)
	assertIDExists(t, db, "platform_events", "event_recent")
}

func TestRuntimeObservationsAndBillingRecordsCleanupInPostgres(t *testing.T) {
	db := openRetentionTestDB(t)
	if err := db.Exec(`
CREATE TABLE runtime_observations (id text PRIMARY KEY, period_start timestamptz NOT NULL);
CREATE TABLE user_wallets (id text PRIMARY KEY, user_id text NOT NULL, balance_credits numeric NOT NULL);
CREATE TABLE volume_transfers (id text PRIMARY KEY);
CREATE TABLE billing_usage_records (
    id text PRIMARY KEY,
    status text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    period_start timestamptz NOT NULL
);
CREATE TABLE billing_ledger_entries (
    id text PRIMARY KEY,
    type text NOT NULL,
    usage_record_id text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    created_at timestamptz NOT NULL
);
`).Error; err != nil {
		t.Fatalf("create billing/observation retention tables: %v", err)
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	expired := now.AddDate(0, 0, -100)
	withinLookback := now.Add(-6 * time.Hour)

	if err := db.Exec(
		"INSERT INTO runtime_observations(id, period_start) VALUES (?, ?), (?, ?)",
		"robs_expired", expired,
		"robs_recent", withinLookback,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO user_wallets(id, user_id, balance_credits) VALUES ('wlt_1', 'usr_1', 42.5)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO volume_transfers(id) VALUES ('vtx_active')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO billing_usage_records(id, status, resource_type, resource_id, period_start) VALUES
		('busg_expired_settled', 'settled', 'runtime_target', 'tgt_1:2026040100', ?),
		('busg_expired_pending', 'pending', 'ai_model_request', 'ai_1', ?),
		('busg_recent_settled', 'settled', 'runtime_target', 'tgt_1:2026071406', ?),
		('busg_transfer_active', 'settled', 'volume_transfer', 'vtx_active', ?),
		('busg_transfer_deleted', 'settled', 'volume_transfer', 'vtx_gone', ?)`,
		expired, expired, withinLookback, expired, expired,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO billing_ledger_entries(id, type, usage_record_id, resource_type, resource_id, created_at) VALUES
		('bled_expired_debit', 'debit', 'busg_expired_settled', 'runtime_target', 'tgt_1:2026040100', ?),
		('bled_expired_credit', 'credit', '', 'user_wallet', 'usr_1', ?),
		('bled_expired_adjustment', 'adjustment', '', 'user_wallet', 'usr_1', ?),
		('bled_recent_debit', 'debit', 'busg_recent_settled', 'runtime_target', 'tgt_1:2026071406', ?),
		('bled_transfer_active', 'debit', 'busg_transfer_active', 'volume_transfer', 'vtx_active', ?),
		('bled_transfer_deleted', 'debit', 'busg_transfer_deleted', 'volume_transfer', 'vtx_gone', ?)`,
		expired, expired, expired, withinLookback, expired, expired,
	).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(db)
	start := now.AddDate(0, 0, -120)
	end := now.Add(time.Hour)
	datasets := []string{DatasetRuntimeObservations, DatasetBillingRecords}

	preview, err := service.Preview(t.Context(), datasets, start, end, now)
	if err != nil {
		t.Fatalf("preview cleanup: %v", err)
	}
	if len(preview) != 2 || preview[0].Matched != 1 || preview[1].Matched != 4 {
		t.Fatalf("preview = %#v, want [1, 4]", preview)
	}

	results, err := service.Cleanup(t.Context(), datasets, start, end, now)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(results) != 2 || results[0].Matched != 1 || results[0].Deleted != 1 || results[1].Matched != 4 || results[1].Deleted != 4 {
		t.Fatalf("cleanup results = %#v, want [1/1, 4/4]", results)
	}

	assertRowCount(t, db, "runtime_observations", 1)
	assertIDExists(t, db, "runtime_observations", "robs_recent")
	assertRowCount(t, db, "billing_usage_records", 3)
	assertIDExists(t, db, "billing_usage_records", "busg_expired_pending")
	assertIDExists(t, db, "billing_usage_records", "busg_recent_settled")
	assertIDExists(t, db, "billing_usage_records", "busg_transfer_active")
	assertRowCount(t, db, "billing_ledger_entries", 4)
	assertIDExists(t, db, "billing_ledger_entries", "bled_expired_credit")
	assertIDExists(t, db, "billing_ledger_entries", "bled_expired_adjustment")
	assertIDExists(t, db, "billing_ledger_entries", "bled_recent_debit")
	assertIDExists(t, db, "billing_ledger_entries", "bled_transfer_active")

	var balance float64
	if err := db.Raw("SELECT balance_credits FROM user_wallets WHERE id = 'wlt_1'").Scan(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance != 42.5 {
		t.Fatalf("wallet balance = %v, want 42.5", balance)
	}
}

func TestAIRunEventsCleanupPreservesActiveRunsAndConversationHistoryInPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "retention_ai_test"})
	if err := db.Exec(`
CREATE TABLE app_configs (key text PRIMARY KEY, value text NOT NULL);
CREATE SCHEMA ai;
CREATE TABLE ai.runs (
    id text PRIMARY KEY,
    conversation_id text NOT NULL,
    status text NOT NULL,
    created_at timestamptz NOT NULL,
    completed_at timestamptz
);
CREATE TABLE ai.run_events (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    event_sequence bigint NOT NULL,
    type text NOT NULL,
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL
);
CREATE TABLE ai.items (
    id text PRIMARY KEY,
    conversation_id text NOT NULL,
    run_id text,
    kind text NOT NULL,
    content jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL
);
CREATE TABLE ai.conversation_summaries (
    id text PRIMARY KEY,
    conversation_id text NOT NULL,
    summary_text text NOT NULL,
    created_at timestamptz NOT NULL
);
`).Error; err != nil {
		t.Fatalf("create AI retention tables: %v", err)
	}
	for _, dataset := range catalog {
		value := "0"
		if dataset.Key == DatasetAIRunEvents {
			value = "14"
		}
		if err := db.Exec("INSERT INTO app_configs(key, value) VALUES (?, ?)", dataset.ConfigKey, value).Error; err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	expired := now.AddDate(0, 0, -20)
	withinRetention := now.AddDate(0, 0, -5)
	justCompleted := now.Add(-10 * time.Minute)

	runs := []struct {
		id          string
		status      string
		createdAt   time.Time
		completedAt any
	}{
		// All 5 terminal statuses: old events must be deleted, even if completed_at is NULL.
		{"run_completed_old", "completed", expired, expired},
		{"run_failed_old", "failed", expired, expired},
		{"run_canceled_old", "canceled", expired, expired},
		{"run_expired_old", "expired", expired, expired},
		{"run_interrupted_old", "interrupted", expired, nil},
		// Terminal run within 14-day retention window: events must be preserved.
		{"run_completed_recent", "completed", withinRetention, withinRetention},
		// Long-waiting run started 20d ago and just completed 10m ago: old events must NOT be torn/deleted.
		{"run_resumed_just_completed", "completed", expired, justCompleted},
		// All 5 non-terminal statuses: old events must NEVER be deleted.
		{"run_queued_old", "queued", expired, nil},
		{"run_running_old", "running", expired, nil},
		{"run_waiting_approval_old", "waiting_approval", expired, nil},
		{"run_waiting_input_old", "waiting_input", expired, nil},
		{"run_waiting_external_old", "waiting_external", expired, nil},
	}
	for _, run := range runs {
		if err := db.Exec(
			"INSERT INTO ai.runs(id, conversation_id, status, created_at, completed_at) VALUES (?, 'conv_1', ?, ?, ?)",
			run.id, run.status, run.createdAt, run.completedAt,
		).Error; err != nil {
			t.Fatalf("insert run %s: %v", run.id, err)
		}
	}

	// Seed 1,001 events on run_completed_old plus 1 event each on the other 4 terminal old runs
	// (= 1,005 eligible events across >1 batch of CleanupBatchSize=1000).
	if err := db.Exec(`
		INSERT INTO ai.run_events(id, run_id, event_sequence, type, created_at)
		SELECT 'evt_batch_' || seq::text, 'run_completed_old', seq, 'item.delta', ?
		FROM generate_series(1, 1001) AS seq
	`, expired).Error; err != nil {
		t.Fatalf("seed batched terminal events: %v", err)
	}
	extraEvents := []struct {
		id        string
		runID     string
		seq       int64
		createdAt time.Time
	}{
		{"evt_failed_old", "run_failed_old", 1, expired},
		{"evt_canceled_old", "run_canceled_old", 1, expired},
		{"evt_expired_old", "run_expired_old", 1, expired},
		{"evt_interrupted_old", "run_interrupted_old", 1, expired},
		{"evt_recent_preserved", "run_completed_recent", 1, withinRetention},
		{"evt_resumed_early_preserved", "run_resumed_just_completed", 1, expired},
		{"evt_resumed_final_preserved", "run_resumed_just_completed", 2, justCompleted},
		// Non-terminal runs with old events: all 5 must be preserved.
		{"evt_queued_preserved", "run_queued_old", 1, expired},
		{"evt_running_preserved", "run_running_old", 1, expired},
		{"evt_waiting_approval_preserved", "run_waiting_approval_old", 1, expired},
		{"evt_waiting_input_preserved", "run_waiting_input_old", 1, expired},
		{"evt_waiting_external_preserved", "run_waiting_external_old", 1, expired},
	}
	for _, evt := range extraEvents {
		if err := db.Exec(
			"INSERT INTO ai.run_events(id, run_id, event_sequence, type, created_at) VALUES (?, ?, ?, 'run.started', ?)",
			evt.id, evt.runID, evt.seq, evt.createdAt,
		).Error; err != nil {
			t.Fatalf("insert event %s: %v", evt.id, err)
		}
	}

	// Seed conversation items and summary on the expired completed run to prove conversation history remains intact.
	if err := db.Exec(`
		INSERT INTO ai.items(id, conversation_id, run_id, kind, created_at) VALUES
			('item_user_old', 'conv_1', NULL, 'user_message', ?),
			('item_assistant_old', 'conv_1', 'run_completed_old', 'assistant_message', ?)
	`, expired, expired).Error; err != nil {
		t.Fatalf("seed items: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO ai.conversation_summaries(id, conversation_id, summary_text, created_at) VALUES
			('sum_conv_1', 'conv_1', 'Preserved conversation summary', ?)
	`, expired).Error; err != nil {
		t.Fatalf("seed conversation summary: %v", err)
	}

	service := NewService(db)
	cutoff := now.AddDate(0, 0, -14)
	preview, err := service.Preview(t.Context(), []string{DatasetAIRunEvents}, time.Time{}, cutoff, now)
	if err != nil {
		t.Fatalf("preview ai_run_events: %v", err)
	}
	if len(preview) != 1 || preview[0].Matched != 1005 || preview[0].Deleted != 0 {
		t.Fatalf("preview = %#v, want 1005 matched / 0 deleted", preview)
	}

	results, err := service.RunAutomatic(t.Context(), now)
	if err != nil {
		t.Fatalf("run automatic ai_run_events cleanup: %v", err)
	}
	if len(results) != 1 || results[0].Dataset != DatasetAIRunEvents || results[0].Matched != 1005 || results[0].Deleted != 1005 {
		t.Fatalf("automatic results = %#v, want ai_run_events 1005/1005", results)
	}

	// Manual cleanup with end = now + 1h must still protect runs completed within the 1-hour grace window,
	// while cleaning runs completed 5 days ago.
	manualResults, err := service.Cleanup(t.Context(), []string{DatasetAIRunEvents}, now.AddDate(0, 0, -30), now.Add(time.Hour), now)
	if err != nil {
		t.Fatalf("manual ai_run_events cleanup: %v", err)
	}
	if len(manualResults) != 1 || manualResults[0].Matched != 1 || manualResults[0].Deleted != 1 {
		t.Fatalf("manual cleanup results = %#v, want 1/1 (only run_completed_recent)", manualResults)
	}

	// Remaining events: 2 events on run_resumed_just_completed (within 1h grace) + 5 old non-terminal events = 7.
	assertRowCount(t, db, "ai.run_events", 7)
	for _, preservedID := range []string{
		"evt_resumed_early_preserved",
		"evt_resumed_final_preserved",
		"evt_queued_preserved",
		"evt_running_preserved",
		"evt_waiting_approval_preserved",
		"evt_waiting_input_preserved",
		"evt_waiting_external_preserved",
	} {
		assertIDExists(t, db, "ai.run_events", preservedID)
	}
	// All runs, items, and conversation summaries are untouched.
	assertRowCount(t, db, "ai.runs", int64(len(runs)))
	assertRowCount(t, db, "ai.items", 2)
	assertIDExists(t, db, "ai.items", "item_user_old")
	assertIDExists(t, db, "ai.items", "item_assistant_old")
	assertRowCount(t, db, "ai.conversation_summaries", 1)
	assertIDExists(t, db, "ai.conversation_summaries", "sum_conv_1")
}

func openRetentionTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{SchemaPrefix: "retention_test"})
}

func assertRowCount(t *testing.T, db *gorm.DB, table string, want int64) {
	t.Helper()
	var count int64
	if err := db.Table(table).Count(&count).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func assertIDExists(t *testing.T, db *gorm.DB, table, id string) {
	t.Helper()
	var count int64
	if err := db.Table(table).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("find %s %s: %v", table, id, err)
	}
	if count != 1 {
		t.Fatalf("expected %s %s to remain", table, id)
	}
}
