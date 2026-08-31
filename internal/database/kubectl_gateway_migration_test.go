package database

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestKubectlGatewayMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{
			name: "000095_kubectl_gateway.up.sql",
			required: []string{
				"CREATE TABLE kube_access_bindings",
				"ADD COLUMN kube_gateway_enabled boolean NOT NULL DEFAULT false",
				"ADD COLUMN metadata jsonb",
				"ALTER COLUMN deployment_target_id DROP NOT NULL",
				"idx_runtime_observations_resource_period",
				"DROP COLUMN namespace",
			},
		},
		{
			name: "000095_kubectl_gateway.down.sql",
			required: []string{
				"ADD COLUMN namespace text NOT NULL DEFAULT ''",
				"DROP TABLE kube_access_bindings",
				"DROP COLUMN kube_gateway_enabled",
			},
		},
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

func TestKubectlGatewayMigrationRoundTrip(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "kubectl_gateway_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(94); err != nil {
		t.Fatalf("migrate isolated database to version 94: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 94)

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply kubectl gateway migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 95)
	for _, table := range []string{"kube_access_bindings", "runtime_observations"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("migration is missing table %s", table)
		}
	}
	if db.Migrator().HasColumn("deployment_targets", "namespace") {
		t.Fatal("deployment_targets.namespace remains after kubectl gateway migration")
	}
	if !db.Migrator().HasColumn("runtime_clusters", "kube_gateway_enabled") || !db.Migrator().HasColumn("audit_logs", "metadata") {
		t.Fatal("kubectl gateway migration is missing runtime cluster or audit columns")
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back kubectl gateway migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 94)
	if db.Migrator().HasTable("kube_access_bindings") || db.Migrator().HasColumn("runtime_clusters", "kube_gateway_enabled") {
		t.Fatal("kubectl gateway schema remains after rollback")
	}
	if !db.Migrator().HasColumn("deployment_targets", "namespace") {
		t.Fatal("rollback did not restore deployment_targets.namespace")
	}
}
