package retention

import (
	"errors"
	"strings"
	"testing"
	"time"
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
		DatasetBuildLogs:              {"parent.status IN ('succeeded', 'failed', 'canceled', 'timeout')", "parent.id = logs.build_run_id", "parent.finished_at >= ? AND parent.finished_at < ?"},
		DatasetReleaseLogs:            {"parent.status IN ('succeeded', 'failed', 'canceled')", "parent.id = logs.release_id", "parent.finished_at >= ? AND parent.finished_at < ?"},
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

func TestExpiredAuthQueriesUseExpiry(t *testing.T) {
	authPlan := plans[DatasetExpiredAuthData]
	if len(authPlan.queries) != 5 {
		t.Fatalf("expired auth query count = %d, want 5", len(authPlan.queries))
	}
	sessionDeleteSQL := authPlan.queries[0].deleteSQL
	if !strings.Contains(sessionDeleteSQL, "session.expires_at <= ?") {
		t.Fatalf("session cleanup does not treat expiry equal to now as expired:\n%s", sessionDeleteSQL)
	}
	if !strings.Contains(authPlan.queries[1].countSQL, "email_registration_challenges") || !strings.Contains(authPlan.queries[1].deleteSQL, "expires_at <= ?") {
		t.Fatal("expired email registration challenges must be included in authentication cleanup")
	}
	if !strings.Contains(authPlan.queries[2].countSQL, "oauth_authorization_codes") || !strings.Contains(authPlan.queries[2].deleteSQL, "expires_at <= ?") {
		t.Fatal("expired OAuth authorization codes must be included in authentication cleanup")
	}
	if !strings.Contains(authPlan.queries[3].countSQL, "oauth_refresh_tokens") || !strings.Contains(authPlan.queries[3].deleteSQL, "expires_at <= ?") {
		t.Fatal("expired OAuth refresh tokens must be included in authentication cleanup")
	}
	if !strings.Contains(authPlan.queries[4].countSQL, "app.registration_source = 'dynamic'") || !strings.Contains(authPlan.queries[4].deleteSQL, "NOT EXISTS") {
		t.Fatal("abandoned dynamic OAuth applications must be included in authentication cleanup")
	}
}

func TestRuntimeObservationAndBillingRetentionSafetyGuards(t *testing.T) {
	obsPlan := plans[DatasetRuntimeObservations]
	if len(obsPlan.queries) != 1 || !obsPlan.queries[0].requireExpired {
		t.Fatalf("runtime observation plan = %#v, want 1 requireExpired query", obsPlan)
	}
	for _, fragment := range []string{
		"period_start >= ? AND period_start < ?",
		"period_start <= (?::timestamptz - interval '24 hours')",
	} {
		if !strings.Contains(obsPlan.queries[0].countSQL, fragment) || !strings.Contains(obsPlan.queries[0].deleteSQL, fragment) {
			t.Fatalf("runtime observation query missing %q", fragment)
		}
	}

	billingPlan := plans[DatasetBillingRecords]
	if len(billingPlan.queries) != 2 || !billingPlan.queries[0].requireExpired || !billingPlan.queries[1].requireExpired {
		t.Fatalf("billing records plan = %#v, want 2 requireExpired queries", billingPlan)
	}
	if !strings.Contains(billingPlan.queries[0].deleteSQL, "ledger.type = 'debit'") {
		t.Fatal("billing ledger cleanup must only delete debit entries and preserve credit/adjustment entries")
	}
	if !strings.Contains(billingPlan.queries[1].deleteSQL, "usage.status = 'settled'") || !strings.Contains(billingPlan.queries[1].deleteSQL, "volume_transfers") {
		t.Fatal("billing usage cleanup must enforce settled status and volume_transfers idempotency guard")
	}
}

var (
	startOfTestRange = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	endOfTestRange   = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	nowForTestRange  = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
)
