package database

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"gorm.io/gorm"
)

func TestRuntimeResourcePolicyMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{name: "000085_runtime_resource_policy.up.sql", required: []string{
			"cpu_request_percent integer NOT NULL DEFAULT 10", "memory_request_percent integer NOT NULL DEFAULT 25",
			"cpu_limit_percent integer NOT NULL DEFAULT 100", "memory_limit_percent integer NOT NULL DEFAULT 100",
			"effective_cpu_request", "effective_memory_request", "cpu_usage_milli", "memory_usage_bytes",
			"metrics_available", "pod_count", "container_count", "runtime_cluster_id", "project_id",
		}},
		{name: "000085_runtime_resource_policy.down.sql", required: []string{
			"ADD COLUMN cpu_request", "ADD COLUMN memory_request", "DROP COLUMN runtime_cluster_id",
			"DROP COLUMN cpu_request_percent", "DROP COLUMN memory_limit_percent",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, err := sqlmigrations.FS.ReadFile(test.name)
			if err != nil {
				t.Fatalf("read migration: %v", err)
			}
			for _, required := range test.required {
				if !strings.Contains(string(content), required) {
					t.Errorf("migration is missing %q", required)
				}
			}
		})
	}
}

func TestRuntimeResourcePolicyMigrationRoundTripPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "runtime_policy_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(84); err != nil {
		t.Fatalf("migrate isolated database to version 84: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 84)

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply runtime resource policy migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 85)
	assertRuntimeResourcePolicySchema(t, db, true)

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back runtime resource policy migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 84)
	assertRuntimeResourcePolicySchema(t, db, false)

	if err := runner.Steps(1); err != nil {
		t.Fatalf("reapply runtime resource policy migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 85)
	assertRuntimeResourcePolicySchema(t, db, true)
}

func assertRuntimeResourcePolicySchema(t *testing.T, db *gorm.DB, migrated bool) {
	t.Helper()
	for _, column := range []string{"cpu_request_percent", "memory_request_percent", "cpu_limit_percent", "memory_limit_percent"} {
		if got := db.Migrator().HasColumn("runtime_clusters", column); got != migrated {
			t.Fatalf("runtime_clusters.%s present = %t, want %t", column, got, migrated)
		}
	}
	for _, column := range []string{"runtime_cluster_id", "project_id", "effective_cpu_request", "effective_memory_request", "cpu_usage_milli", "memory_usage_bytes", "metrics_available", "pod_count", "container_count"} {
		if got := db.Migrator().HasColumn("runtime_observations", column); got != migrated {
			t.Fatalf("runtime_observations.%s present = %t, want %t", column, got, migrated)
		}
	}
	for _, column := range []string{"cpu_request", "memory_request"} {
		if got := db.Migrator().HasColumn("runtime_observations", column); got == migrated {
			t.Fatalf("runtime_observations.%s present = %t after migrated=%t", column, got, migrated)
		}
	}
}
