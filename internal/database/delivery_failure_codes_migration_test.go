package database

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"gorm.io/gorm"
)

// assertDeliveryFailureCodesMigration verifies that 000111 converts legacy
// lost builds into the timeout state with a stable code and that rolling back
// restores the previous state set without failure code columns.
func assertDeliveryFailureCodesMigration(t *testing.T, db *gorm.DB, runner *migrate.Migrate) {
	t.Helper()
	const failureCodeVersion = 111
	if err := runner.Migrate(failureCodeVersion - 1); err != nil {
		t.Fatalf("roll back to version before delivery failure codes: %v", err)
	}
	seed := []string{
		`INSERT INTO build_runs (id, project_id, status) VALUES
			('bldr_fc_lost', 'prj_fc', 'lost'),
			('bldr_fc_canceled', 'prj_fc', 'canceled'),
			('bldr_fc_failed', 'prj_fc', 'failed')`,
		`INSERT INTO build_jobs (id, build_run_id, project_id, status) VALUES
			('bldj_fc_lost', 'bldr_fc_lost', 'prj_fc', 'lost')`,
		`INSERT INTO releases (id, project_id, application_id, image_ref, status) VALUES
			('rel_fc_running', 'prj_fc', 'app_fc', 'registry.example.test/app:1', 'running')`,
	}
	for _, statement := range seed {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed delivery failure code fixture: %v", err)
		}
	}
	if err := runner.Migrate(failureCodeVersion); err != nil {
		t.Fatalf("apply delivery failure codes migration: %v", err)
	}
	type stateRow struct {
		ID          string
		Status      string
		FailureCode string
	}
	readStates := func(table string) map[string]stateRow {
		var rows []stateRow
		if err := db.Raw(`SELECT id, status, failure_code FROM ` + table + ` WHERE project_id = 'prj_fc'`).Scan(&rows).Error; err != nil {
			t.Fatalf("read migrated %s: %v", table, err)
		}
		states := map[string]stateRow{}
		for _, row := range rows {
			states[row.ID] = row
		}
		return states
	}
	runs := readStates("build_runs")
	if got := runs["bldr_fc_lost"]; got.Status != "timeout" || got.FailureCode != "timeout" {
		t.Fatalf("migrated lost run = %+v, want timeout/timeout", got)
	}
	if got := runs["bldr_fc_canceled"]; got.Status != "canceled" || got.FailureCode != "canceled_by_user" {
		t.Fatalf("migrated canceled run = %+v, want canceled/canceled_by_user", got)
	}
	if got := runs["bldr_fc_failed"]; got.Status != "failed" || got.FailureCode != "" {
		t.Fatalf("migrated failed run = %+v, want failed without code", got)
	}
	if got := readStates("build_jobs")["bldj_fc_lost"]; got.Status != "timeout" || got.FailureCode != "timeout" {
		t.Fatalf("migrated lost job = %+v, want timeout/timeout", got)
	}
	if got := readStates("releases")["rel_fc_running"]; got.Status != "running" || got.FailureCode != "" {
		t.Fatalf("migrated release = %+v, want running without code", got)
	}
	if err := db.Exec(`UPDATE releases SET status = 'canceled', failure_code = 'canceled_by_user' WHERE id = 'rel_fc_running'`).Error; err != nil {
		t.Fatalf("cancel release fixture: %v", err)
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back delivery failure codes migration: %v", err)
	}
	for _, table := range []string{"build_runs", "build_jobs", "releases"} {
		if db.Migrator().HasColumn(table, "failure_code") {
			t.Fatalf("delivery failure codes rollback retained %s.failure_code", table)
		}
	}
	var restored []struct {
		ID     string
		Status string
	}
	if err := db.Raw(`SELECT id, status FROM build_runs WHERE id = 'bldr_fc_lost'
		UNION ALL SELECT id, status FROM releases WHERE id = 'rel_fc_running'`).Scan(&restored).Error; err != nil {
		t.Fatalf("read rolled back states: %v", err)
	}
	for _, row := range restored {
		if (row.ID == "bldr_fc_lost" && row.Status != "lost") || (row.ID == "rel_fc_running" && row.Status != "failed") {
			t.Fatalf("rolled back state %s = %q", row.ID, row.Status)
		}
	}
	if err := runner.Migrate(latestMigrationVersion); err != nil {
		t.Fatalf("restore latest migrations after delivery failure code round trip: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, latestMigrationVersion)
	for _, statement := range []string{
		`DELETE FROM build_jobs WHERE project_id = 'prj_fc'`,
		`DELETE FROM build_runs WHERE project_id = 'prj_fc'`,
		`DELETE FROM releases WHERE project_id = 'prj_fc'`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("clean delivery failure code fixture: %v", err)
		}
	}
}
