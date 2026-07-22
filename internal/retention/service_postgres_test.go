package retention

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCleanupEnforcesTerminalParentsInPostgres(t *testing.T) {
	db := openRetentionTestDB(t)
	if err := db.Exec(`
CREATE TABLE platform_events (id text PRIMARY KEY, occurred_at timestamptz NOT NULL);
CREATE TABLE notification_deliveries (id text PRIMARY KEY, status text NOT NULL, finished_at timestamptz);
CREATE TABLE worker_task_events (id text PRIMARY KEY, created_at timestamptz NOT NULL);
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
		{"INSERT INTO worker_task_events(id, created_at) VALUES (?, ?)", []any{"task_old", old}},
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
		DatasetWorkerTaskEvents,
		DatasetBuildLogs,
		DatasetReleaseLogs,
		DatasetHookRunLogs,
	}
	results, err := NewService(db).Cleanup(t.Context(), datasets, now.Add(-48*time.Hour), now, now)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	wantMatched := map[string]int64{
		DatasetPlatformEvents: 1, DatasetNotificationDeliveries: 2, DatasetWorkerTaskEvents: 1,
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

func TestExpiredAuthCleanupPreservesFutureAndBlockedRelationsInPostgres(t *testing.T) {
	db := openRetentionTestDB(t)
	if err := db.Exec(`
CREATE TABLE user_sessions (id text PRIMARY KEY, expires_at timestamptz NOT NULL);
CREATE TABLE step_up_assertions (
    id text PRIMARY KEY,
    session_id text NOT NULL REFERENCES user_sessions(id) ON DELETE CASCADE,
    idle_expires_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL
);
CREATE TABLE user_remember_tokens (id text PRIMARY KEY, expires_at timestamptz NOT NULL);
CREATE TABLE email_registration_challenges (id text PRIMARY KEY, expires_at timestamptz NOT NULL);
`).Error; err != nil {
		t.Fatalf("create auth retention tables: %v", err)
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)
	future := now.Add(24 * time.Hour)
	if err := db.Exec("INSERT INTO user_sessions(id, expires_at) VALUES (?, ?), (?, ?), (?, ?)",
		"session_old", old, "session_blocked", old, "session_future", future).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO step_up_assertions(id, session_id, idle_expires_at, absolute_expires_at) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
		"assertion_old", "session_old", old, old, "assertion_future", "session_blocked", future, future).Error; err != nil {
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
	if len(preview) != 1 || preview[0].Matched != 4 || preview[0].Deleted != 0 {
		t.Fatalf("auth preview = %#v, want 4 matched", preview)
	}
	results, err := service.Cleanup(t.Context(), []string{DatasetExpiredAuthData}, start, end, now)
	if err != nil {
		t.Fatalf("cleanup auth data: %v", err)
	}
	if len(results) != 1 || results[0].Matched != 4 || results[0].Deleted != 4 {
		t.Fatalf("auth cleanup = %#v, want 4 matched/deleted", results)
	}

	assertRowCount(t, db, "step_up_assertions", 1)
	assertRowCount(t, db, "user_sessions", 2)
	assertRowCount(t, db, "user_remember_tokens", 1)
	assertRowCount(t, db, "email_registration_challenges", 1)
	assertIDExists(t, db, "step_up_assertions", "assertion_future")
	assertIDExists(t, db, "user_sessions", "session_blocked")
	assertIDExists(t, db, "user_sessions", "session_future")
	assertIDExists(t, db, "user_remember_tokens", "remember_future")
	assertIDExists(t, db, "email_registration_challenges", "registration_future")
}

func TestRunAutomaticReadsConfigsAndHonorsZeroInPostgres(t *testing.T) {
	db := openRetentionTestDB(t)
	if err := db.Exec(`
CREATE TABLE app_configs (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE worker_task_events (id text PRIMARY KEY, created_at timestamptz NOT NULL);
`).Error; err != nil {
		t.Fatalf("create automatic retention tables: %v", err)
	}
	for _, dataset := range catalog {
		value := "0"
		if dataset.Key == DatasetWorkerTaskEvents {
			value = "30"
		}
		if err := db.Exec("INSERT INTO app_configs(key, value) VALUES (?, ?)", dataset.ConfigKey, value).Error; err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	if err := db.Exec("INSERT INTO worker_task_events(id, created_at) VALUES (?, ?), (?, ?)",
		"task_old", now.AddDate(0, 0, -31), "task_recent", now.AddDate(0, 0, -29)).Error; err != nil {
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
		if result.Dataset != DatasetWorkerTaskEvents || result.Matched != 1 || result.Deleted != 1 {
			t.Fatalf("worker automatic result = %#v", result)
		}
	}
	assertRowCount(t, db, "worker_task_events", 1)
	assertIDExists(t, db, "worker_task_events", "task_recent")
}

func openRetentionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}

	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("retention_test_%d", time.Now().UnixNano())
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
