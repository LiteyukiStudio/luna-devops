package database

import (
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestRemoveEnvironmentDomainMigration(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "environment_domain_removal_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(101); err != nil {
		t.Fatalf("migrate isolated database to version 101: %v", err)
	}

	now := time.Now()
	if err := db.Exec(`INSERT INTO projects (id, identifier, kubernetes_namespace, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"prj_environment_removal", "environment-removal", "luna-environment-removal", "Environment Removal", now, now).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := db.Exec(`INSERT INTO applications (id, project_id, identifier, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"app_environment_removal", "prj_environment_removal", "api", "API", now, now).Error; err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if err := db.Exec(`INSERT INTO deployment_targets (id, project_id, application_id, environment_id, name, stage, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"dplt_environment_removal", "prj_environment_removal", "app_environment_removal", "legacy_environment", "Production", "prod", now, now).Error; err != nil {
		t.Fatalf("insert deployment target: %v", err)
	}
	if err := db.Exec(`INSERT INTO gateway_routes (id, project_id, application_id, environment_id, deployment_target_id, host, domain_suffix, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"gwr_environment_removal", "prj_environment_removal", "app_environment_removal", "different_environment", "dplt_environment_removal", "api.example.test", "example.test", now, now).Error; err != nil {
		t.Fatalf("insert mismatched gateway route: %v", err)
	}
	if err := db.Exec(`INSERT INTO hook_runs (id, project_id, application_id, environment_id, deployment_target_id, name, phase, script_snapshot, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"hkr_environment_removal", "prj_environment_removal", "app_environment_removal", "legacy_environment", "missing_target", "Deploy Hook", "pre_deploy", "true", now, now).Error; err != nil {
		t.Fatalf("insert hook run with missing target: %v", err)
	}
	if err := db.Exec(`INSERT INTO releases (id, project_id, application_id, environment_id, deployment_target_id, image_ref, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"rel_environment_removal", "prj_environment_removal", "app_environment_removal", "legacy_environment", "", "registry.example.test/api:latest", now, now).Error; err != nil {
		t.Fatalf("insert release without target: %v", err)
	}

	removedColumns := map[string]string{
		"deployment_targets": "environment_id",
		"gateway_routes":     "environment_id",
		"hook_runs":          "environment_id",
		"releases":           "environment_id",
	}
	removedIndexes := map[string][]string{
		"deployment_targets": {"idx_deployment_targets_app_env_name_active", "idx_deployment_targets_environment_id"},
		"gateway_routes":     {"idx_gateway_routes_environment_id"},
		"hook_runs":          {"idx_hook_runs_environment_id"},
		"releases":           {"idx_releases_environment_id"},
	}

	err := runner.Steps(1)
	if err == nil || !strings.Contains(err.Error(), "without an equivalent deployment target association") {
		t.Fatalf("apply migration with invalid child associations error = %v", err)
	}
	assertEnvironmentDomainSchema(t, db, removedColumns, removedIndexes, true)
	version, dirty, versionErr := runner.Version()
	if versionErr != nil {
		t.Fatalf("read failed migration version: %v", versionErr)
	}
	if version != 102 || !dirty {
		t.Fatalf("failed migration version = %d dirty=%t, want 102 dirty", version, dirty)
	}

	for table := range map[string]struct{}{"gateway_routes": {}, "hook_runs": {}, "releases": {}} {
		if err := db.Table(table).Where("id IN ?", []string{"gwr_environment_removal", "hkr_environment_removal", "rel_environment_removal"}).Updates(map[string]any{
			"deployment_target_id": "dplt_environment_removal",
			"environment_id":       "legacy_environment",
		}).Error; err != nil {
			t.Fatalf("repair %s association: %v", table, err)
		}
	}
	if err := runner.Force(101); err != nil {
		t.Fatalf("reset failed migration version: %v", err)
	}
	if err := runner.Steps(1); err != nil {
		t.Fatalf("retry environment domain removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 102)
	assertEnvironmentDomainSchema(t, db, removedColumns, removedIndexes, false)

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back environment domain removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 101)
	assertEnvironmentDomainSchema(t, db, removedColumns, removedIndexes, true)
	for table, id := range map[string]string{
		"deployment_targets": "dplt_environment_removal",
		"gateway_routes":     "gwr_environment_removal",
		"hook_runs":          "hkr_environment_removal",
		"releases":           "rel_environment_removal",
	} {
		assertRestoredEnvironmentID(t, db, table, id, "dplt_environment_removal")
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("reapply environment domain removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 102)
	assertEnvironmentDomainSchema(t, db, removedColumns, removedIndexes, false)
}

func assertEnvironmentDomainSchema(t *testing.T, db *gorm.DB, columns map[string]string, indexes map[string][]string, present bool) {
	t.Helper()
	for table, column := range columns {
		if got := db.Migrator().HasColumn(table, column); got != present {
			t.Errorf("%s.%s presence = %t, want %t", table, column, got, present)
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

func assertRestoredEnvironmentID(t *testing.T, db *gorm.DB, table string, id string, want string) {
	t.Helper()
	var got string
	if err := db.Table(table).Select("environment_id").Where("id = ?", id).Scan(&got).Error; err != nil {
		t.Fatalf("read restored environment identifier from %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("restored environment identifier from %s = %q, want %q", table, got, want)
	}
}
