package database

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestRemoveSingleValueStrategyFieldsMigration(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "single_value_strategy_removal_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(99); err != nil {
		t.Fatalf("migrate isolated database to version 99: %v", err)
	}
	if err := db.Exec(`INSERT INTO projects (id, identifier, kubernetes_namespace, name, namespace_strategy) VALUES (?, ?, ?, ?, ?)`,
		"prj_strategy_removal", "strategy-removal", "luna-strategy-removal", "Strategy Removal", "project").Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := db.Exec(`INSERT INTO runtime_clusters (id, name, type, gateway_provider) VALUES (?, ?, ?, ?)`,
		"rcl_strategy_removal", "Strategy Removal", "k3s", "gateway-api").Error; err != nil {
		t.Fatalf("insert runtime cluster: %v", err)
	}

	removed := map[string][]string{
		"projects":         {"namespace_strategy"},
		"runtime_clusters": {"type", "gateway_provider"},
	}
	assertSingleValueStrategySchema(t, db, removed, true)

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply single-value strategy removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 100)
	assertSingleValueStrategySchema(t, db, removed, false)

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back single-value strategy removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 99)
	assertSingleValueStrategySchema(t, db, removed, true)
	var restored struct {
		Type            string
		GatewayProvider string
	}
	if err := db.Table("runtime_clusters").Select("type, gateway_provider").Where("id = ?", "rcl_strategy_removal").Scan(&restored).Error; err != nil {
		t.Fatalf("read restored runtime cluster strategy fields: %v", err)
	}
	if restored.Type != "kubernetes" || restored.GatewayProvider != "gateway-api" {
		t.Fatalf("restored runtime cluster strategy fields = %#v", restored)
	}
	var restoredNamespaceStrategy string
	if err := db.Table("projects").Select("namespace_strategy").Where("id = ?", "prj_strategy_removal").Scan(&restoredNamespaceStrategy).Error; err != nil {
		t.Fatalf("read restored project namespace strategy: %v", err)
	}
	if restoredNamespaceStrategy != "project" {
		t.Fatalf("restored project namespace strategy = %q, want project", restoredNamespaceStrategy)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("reapply single-value strategy removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 100)
	assertSingleValueStrategySchema(t, db, removed, false)
}

func assertSingleValueStrategySchema(t *testing.T, db *gorm.DB, columns map[string][]string, present bool) {
	t.Helper()
	for table, names := range columns {
		for _, name := range names {
			if got := db.Migrator().HasColumn(table, name); got != present {
				t.Errorf("%s.%s presence = %t, want %t", table, name, got, present)
			}
		}
	}
}
