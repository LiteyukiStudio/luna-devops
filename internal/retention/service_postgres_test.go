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
CREATE TABLE user_remember_tokens (id text PRIMARY KEY, expires_at timestamptz NOT NULL);
CREATE TABLE email_registration_challenges (id text PRIMARY KEY, expires_at timestamptz NOT NULL);
`).Error; err != nil {
		t.Fatalf("create auth retention tables: %v", err)
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)
	future := now.Add(24 * time.Hour)
	if err := db.Exec("INSERT INTO user_sessions(id, expires_at) VALUES (?, ?), (?, ?)",
		"session_old", old, "session_future", future).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO user_remember_tokens(id, expires_at) VALUES (?, ?), (?, ?)",
		"remember_old", old, "remember_future", future).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO email_registration_challenges(id, expires_at) VALUES (?, ?), (?, ?)",
		"registration_old", old, "registration_future", future).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(db)
	start := now.AddDate(0, 0, -60)
	end := now.AddDate(0, 0, 2)
	preview, err := service.Preview(t.Context(), []string{DatasetExpiredAuthData}, start, end, now)
	if err != nil {
		t.Fatalf("preview auth cleanup: %v", err)
	}
	if len(preview) != 1 || preview[0].Matched != 3 || preview[0].Deleted != 0 {
		t.Fatalf("auth preview = %#v, want 3 matched", preview)
	}
	results, err := service.Cleanup(t.Context(), []string{DatasetExpiredAuthData}, start, end, now)
	if err != nil {
		t.Fatalf("cleanup auth data: %v", err)
	}
	if len(results) != 1 || results[0].Matched != 3 || results[0].Deleted != 3 {
		t.Fatalf("auth cleanup = %#v, want 3 matched/deleted", results)
	}

	assertRowCount(t, db, "user_sessions", 1)
	assertRowCount(t, db, "user_remember_tokens", 1)
	assertRowCount(t, db, "email_registration_challenges", 1)
	assertIDExists(t, db, "user_sessions", "session_future")
	assertIDExists(t, db, "user_remember_tokens", "remember_future")
	assertIDExists(t, db, "email_registration_challenges", "registration_future")
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
