package retention

import (
	"errors"
	"strings"
	"testing"
	"time"

	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestValidateManualInput(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	plans, err := selectPlans([]string{DatasetBuildLogs, DatasetBuildLogs, DatasetReleaseLogs})
	if err != nil {
		t.Fatalf("validate input: %v", err)
	}
	if len(plans) != 2 || plans[0].dataset.Key != DatasetBuildLogs || plans[1].dataset.Key != DatasetReleaseLogs {
		t.Fatalf("validated plans = %#v", plans)
	}

	service := NewService(newDryRunDB(t))
	for name, test := range map[string]struct {
		start   time.Time
		end     time.Time
		wantErr error
	}{
		"equal range":    {start: start, end: start, wantErr: ErrInvalidRange},
		"reversed range": {start: end, end: start, wantErr: ErrInvalidRange},
	} {
		t.Run(name, func(t *testing.T) {
			_, validateErr := validateRequest(service, []string{DatasetBuildLogs}, test.start, test.end)
			if !errors.Is(validateErr, test.wantErr) {
				t.Fatalf("error = %v, want %v", validateErr, test.wantErr)
			}
		})
	}
	if _, err := selectPlans([]string{"build_logs; DROP TABLE build_runs"}); !errors.Is(err, ErrUnknownDataset) {
		t.Fatalf("unknown dataset error = %v, want %v", err, ErrUnknownDataset)
	}
	if _, err := selectPlans(nil); !errors.Is(err, ErrNoDatasets) {
		t.Fatalf("empty dataset error = %v, want %v", err, ErrNoDatasets)
	}
}

func TestParseRetentionDaysBounds(t *testing.T) {
	for _, value := range []string{"0", " 30 ", "3650"} {
		if _, err := parseRetentionDays(value); err != nil {
			t.Fatalf("parseRetentionDays(%q): %v", value, err)
		}
	}
	for _, value := range []string{"-1", "3651", "invalid"} {
		if _, err := parseRetentionDays(value); !errors.Is(err, ErrInvalidRetentionDays) {
			t.Fatalf("parseRetentionDays(%q) error = %v, want %v", value, err, ErrInvalidRetentionDays)
		}
	}
}

func TestStandardQueriesContainTerminalGuardsAndHalfOpenRange(t *testing.T) {
	tests := map[string][]string{
		DatasetNotificationDeliveries: {"status IN ('succeeded', 'failed')", "finished_at >= ? AND finished_at < ?"},
		DatasetBuildLogs:              {"parent.status IN ('succeeded', 'failed', 'canceled', 'lost', 'timeout')", "parent.id = logs.build_run_id", "parent.finished_at >= ? AND parent.finished_at < ?"},
		DatasetReleaseLogs:            {"parent.status IN ('succeeded', 'failed')", "parent.id = logs.release_id", "parent.finished_at >= ? AND parent.finished_at < ?"},
		DatasetHookRunLogs:            {"parent.status IN ('succeeded', 'failed')", "parent.id = logs.hook_run_id", "parent.finished_at >= ? AND parent.finished_at < ?"},
	}
	for dataset, fragments := range tests {
		t.Run(dataset, func(t *testing.T) {
			query := plans[dataset].queries[0]
			for _, fragment := range fragments {
				if !strings.Contains(query.countSQL, fragment) || !strings.Contains(query.deleteSQL, fragment) {
					t.Fatalf("queries for %s do not contain %q:\n%s\n%s", dataset, fragment, query.countSQL, query.deleteSQL)
				}
			}
			if !strings.Contains(query.deleteSQL, "LIMIT 1000") || !strings.Contains(query.deleteSQL, "ORDER BY") {
				t.Fatalf("delete query is not deterministic and bounded:\n%s", query.deleteSQL)
			}
		})
	}
}

func TestExpiredAuthQueriesUseExpiryAndProtectSessionChildren(t *testing.T) {
	authPlan := plans[DatasetExpiredAuthData]
	if len(authPlan.queries) != 4 {
		t.Fatalf("expired auth query count = %d, want 4", len(authPlan.queries))
	}
	assertionCountSQL := authPlan.queries[0].countSQL
	if strings.Count(assertionCountSQL, "LEAST(idle_expires_at, absolute_expires_at)") != 3 {
		t.Fatalf("assertion query does not apply range and now to effective expiry:\n%s", assertionCountSQL)
	}
	if !strings.Contains(assertionCountSQL, "<= ?") {
		t.Fatalf("assertion query does not treat expiry equal to now as expired:\n%s", assertionCountSQL)
	}
	sessionCountSQL := authPlan.queries[1].countSQL
	sessionDeleteSQL := authPlan.queries[1].deleteSQL
	if !strings.Contains(sessionCountSQL, "NOT EXISTS") || !strings.Contains(sessionCountSQL, "AND NOT (") {
		t.Fatalf("session preview does not account for assertion eligibility:\n%s", sessionCountSQL)
	}
	if !strings.Contains(sessionDeleteSQL, "NOT EXISTS") || !strings.Contains(sessionDeleteSQL, "AND NOT (") {
		t.Fatalf("session cleanup does not preserve assertions outside the requested range:\n%s", sessionDeleteSQL)
	}
	if !strings.Contains(sessionDeleteSQL, "session.expires_at <= ?") {
		t.Fatalf("session cleanup does not treat expiry equal to now as expired:\n%s", sessionDeleteSQL)
	}
	if authPlan.queries[1].windowCount != 2 || len(authPlan.queries[1].rangeArgs(startOfTestRange, endOfTestRange, nowForTestRange)) != 6 {
		t.Fatal("session relation guard must receive the same range for session and assertion predicates")
	}
	if !strings.Contains(authPlan.queries[3].countSQL, "email_registration_challenges") || !strings.Contains(authPlan.queries[3].deleteSQL, "expires_at <= ?") {
		t.Fatal("expired email registration challenges must be included in authentication cleanup")
	}
}

var (
	startOfTestRange = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	endOfTestRange   = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	nowForTestRange  = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
)

func TestRetentionIndexMigrationCoversAllDatasets(t *testing.T) {
	up, err := sqlmigrations.FS.ReadFile("000037_data_retention_indexes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := sqlmigrations.FS.ReadFile("000037_data_retention_indexes.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := string(up)
	downSQL := string(down)
	for _, index := range []string{
		"idx_platform_events_retention",
		"idx_notification_deliveries_retention_terminal",
		"idx_worker_task_events_retention",
		"idx_build_runs_retention_terminal",
		"idx_release_logs_retention_parent",
		"idx_releases_retention_terminal",
		"idx_hook_run_logs_retention_parent",
		"idx_hook_runs_retention_terminal",
		"idx_step_up_assertions_retention_expiry",
		"idx_user_sessions_retention_expiry",
		"idx_user_remember_tokens_retention_expiry",
	} {
		if !strings.Contains(upSQL, "CREATE INDEX IF NOT EXISTS "+index) {
			t.Fatalf("up migration is missing %s", index)
		}
		if !strings.Contains(downSQL, "DROP INDEX IF EXISTS "+index) {
			t.Fatalf("down migration is missing %s", index)
		}
	}
	if !strings.Contains(upSQL, "WHERE status IN ('succeeded', 'failed')") ||
		!strings.Contains(upSQL, "LEAST(idle_expires_at, absolute_expires_at)") {
		t.Fatal("migration is missing terminal notification or effective assertion expiry index")
	}
}
