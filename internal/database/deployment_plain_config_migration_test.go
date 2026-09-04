package database

import (
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestUnifyDeploymentPlainConfigMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{
			name: "000093_unify_deployment_plain_config.up.sql",
			required: []string{
				"env_vars::jsonb",
				"config_refs::jsonb",
				"DROP COLUMN config_refs",
			},
		},
		{
			name:     "000093_unify_deployment_plain_config.down.sql",
			required: []string{"ADD COLUMN config_refs text DEFAULT ''::text NOT NULL"},
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

func TestUnifyDeploymentPlainConfigMigrationPreservesPrecedence(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "deployment_plain_config_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(92); err != nil {
		t.Fatalf("migrate isolated database to version 92: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 92)

	now := time.Now()
	if err := db.Exec(`INSERT INTO projects (id, identifier, kubernetes_namespace, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"prj_plain_config", "plain-config", "luna-plain-config", "Plain Config", now, now).Error; err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if err := db.Exec(`INSERT INTO applications (id, project_id, identifier, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"app_plain_config", "prj_plain_config", "api", "API", now, now).Error; err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if err := db.Exec(`INSERT INTO deployment_targets (id, project_id, application_id, name, stage, env_vars, config_refs, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"dplt_plain_config", "prj_plain_config", "app_plain_config", "Production", "prod", `{"KEEP":"plain","OVERRIDE":"plain"}`, `{"OVERRIDE":"config-map","MIGRATED":"true"}`, now, now).Error; err != nil {
		t.Fatalf("insert deployment target: %v", err)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply plain config migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 93)
	if db.Migrator().HasColumn("deployment_targets", "config_refs") {
		t.Fatal("deployment_targets.config_refs remains after migration")
	}
	var encoded string
	if err := db.Table("deployment_targets").Select("env_vars").Where("id = ?", "dplt_plain_config").Scan(&encoded).Error; err != nil {
		t.Fatalf("read migrated plain config: %v", err)
	}
	values, err := runtimeconfig.DecodeKeyValue(encoded)
	if err != nil {
		t.Fatalf("decode migrated plain config: %v", err)
	}
	if values["KEEP"] != "plain" || values["OVERRIDE"] != "config-map" || values["MIGRATED"] != "true" {
		t.Fatalf("migrated plain config = %#v", values)
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back plain config migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 92)
	if !db.Migrator().HasColumn("deployment_targets", "config_refs") {
		t.Fatal("rollback did not restore deployment_targets.config_refs")
	}
	var restored struct {
		ConfigRefs string
		EnvVars    string
	}
	if err := db.Table("deployment_targets").Where("id = ?", "dplt_plain_config").Scan(&restored).Error; err != nil {
		t.Fatalf("read rolled back plain config: %v", err)
	}
	if restored.ConfigRefs != "" {
		t.Fatalf("restored config_refs = %q, want empty", restored.ConfigRefs)
	}
	values, err = runtimeconfig.DecodeKeyValue(restored.EnvVars)
	if err != nil || values["OVERRIDE"] != "config-map" {
		t.Fatalf("rollback lost merged plain config: values=%#v err=%v", values, err)
	}
}
