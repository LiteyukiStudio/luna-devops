package database

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestRemoveUnusedBuilderScaffoldingMigration(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "builder_scaffolding_removal_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(102); err != nil {
		t.Fatalf("migrate isolated database to version 102: %v", err)
	}

	removed := map[string][]string{
		"build_jobs":         {"type", "builder_id", "lease_token", "lease_until", "last_heartbeat_at"},
		"build_runs":         {"build_labels", "cache_config", "cpu_core_seconds", "memory_mb_seconds", "credit_cost"},
		"deployment_targets": {"build_labels"},
	}
	removedIndexes := map[string][]string{
		"build_jobs": {"idx_build_jobs_builder_id", "idx_build_jobs_lease_token", "idx_build_jobs_lease_until", "idx_build_jobs_last_heartbeat_at"},
	}
	assertBuilderScaffoldingSchema(t, db, removed, removedIndexes, true)
	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply unused builder scaffolding migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 103)
	assertBuilderScaffoldingSchema(t, db, removed, removedIndexes, false)

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back unused builder scaffolding migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 102)
	assertBuilderScaffoldingSchema(t, db, removed, removedIndexes, true)

	if err := runner.Steps(1); err != nil {
		t.Fatalf("reapply unused builder scaffolding migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 103)
	assertBuilderScaffoldingSchema(t, db, removed, removedIndexes, false)
}

func assertBuilderScaffoldingSchema(t *testing.T, db *gorm.DB, columns map[string][]string, indexes map[string][]string, present bool) {
	t.Helper()
	for table, names := range columns {
		for _, name := range names {
			if got := db.Migrator().HasColumn(table, name); got != present {
				t.Errorf("%s.%s presence = %t, want %t", table, name, got, present)
			}
		}
	}
	for table, names := range indexes {
		for _, name := range names {
			if got := db.Migrator().HasIndex(table, name); got != present {
				t.Errorf("%s index %s presence = %t, want %t", table, name, got, present)
			}
		}
	}
}
