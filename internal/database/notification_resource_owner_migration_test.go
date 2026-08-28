package database

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestNotificationResourceOwnerMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{
			name: "000091_notification_resource_owner_visibility.up.sql",
			required: []string{
				"ADD COLUMN resource_owner_user_id text DEFAULT ''::text NOT NULL",
				"CREATE INDEX idx_platform_events_resource_owner_user_id",
			},
		},
		{
			name: "000091_notification_resource_owner_visibility.down.sql",
			required: []string{
				"DROP INDEX IF EXISTS idx_platform_events_resource_owner_user_id",
				"DROP COLUMN resource_owner_user_id",
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

func TestNotificationResourceOwnerMigrationRoundTripPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "notification_resource_owner_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(90); err != nil {
		t.Fatalf("migrate isolated database to version 90: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 90)

	if err := db.Exec(`
INSERT INTO platform_events (
  id, type, category, severity, status, actor_id, occurred_at, created_at
) VALUES (
  'evt_resource_owner_existing', 'build.failed', 'build', 'error', 'failed', 'usr_actor', now(), now()
)`).Error; err != nil {
		t.Fatalf("seed version 90 platform event: %v", err)
	}
	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply notification resource owner migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 91)
	if !db.Migrator().HasColumn("platform_events", "resource_owner_user_id") {
		t.Fatal("platform_events.resource_owner_user_id is missing")
	}
	if !db.Migrator().HasIndex("platform_events", "idx_platform_events_resource_owner_user_id") {
		t.Fatal("platform event resource owner index is missing")
	}
	var ownerUserID string
	if err := db.Table("platform_events").Select("resource_owner_user_id").Where("id = ?", "evt_resource_owner_existing").Scan(&ownerUserID).Error; err != nil {
		t.Fatalf("read migrated resource owner: %v", err)
	}
	if ownerUserID != "" {
		t.Fatalf("existing event resource owner = %q, want empty", ownerUserID)
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back notification resource owner migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 90)
	if db.Migrator().HasColumn("platform_events", "resource_owner_user_id") {
		t.Fatal("resource owner column remains after rollback")
	}
}
