package database

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestWebConsolePolicyMigrationPreservesDisabledPoliciesOnFailedRollbackAndReapply(t *testing.T) {
	db := testdb.Open(t, testdb.Options{SchemaPrefix: "web_console_migration_test"})

	if err := db.Exec(`
		CREATE TABLE projects (id text PRIMARY KEY);
		CREATE TABLE deployment_targets (id text PRIMARY KEY);
		INSERT INTO projects(id) VALUES ('prj_test');
		INSERT INTO deployment_targets(id) VALUES ('dplt_test');
	`).Error; err != nil {
		t.Fatalf("create migration prerequisites: %v", err)
	}
	upMigration, err := sqlmigrations.FS.ReadFile("000034_web_console_policy.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(upMigration)).Error; err != nil {
		t.Fatalf("apply Web Console policy migration: %v", err)
	}
	if err := db.Exec(`UPDATE projects SET web_console_enabled = false; UPDATE deployment_targets SET web_console_enabled = false`).Error; err != nil {
		t.Fatalf("disable Web Console policies: %v", err)
	}

	downMigration, err := sqlmigrations.FS.ReadFile("000034_web_console_policy.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(downMigration)).Error; err == nil {
		t.Fatal("expected the irreversible down migration to fail")
	}
	if err := db.Exec(string(upMigration)).Error; err != nil {
		t.Fatalf("reapply Web Console policy migration: %v", err)
	}

	var projectEnabled bool
	if err := db.Raw(`SELECT web_console_enabled FROM projects WHERE id = 'prj_test'`).Scan(&projectEnabled).Error; err != nil {
		t.Fatalf("read project policy: %v", err)
	}
	var targetEnabled bool
	if err := db.Raw(`SELECT web_console_enabled FROM deployment_targets WHERE id = 'dplt_test'`).Scan(&targetEnabled).Error; err != nil {
		t.Fatalf("read deployment policy: %v", err)
	}
	if projectEnabled || targetEnabled {
		t.Fatalf("disabled policies were not preserved: project=%v target=%v", projectEnabled, targetEnabled)
	}
}
