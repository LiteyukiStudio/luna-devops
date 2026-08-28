package database

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestNotificationFanoutOutboxMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{
			name: "000092_notification_fanout_outbox.up.sql",
			required: []string{
				"ADD COLUMN notification_fanout_status text DEFAULT ''::text NOT NULL",
				"ADD COLUMN fanout_traceparent text DEFAULT ''::text NOT NULL",
				"ADD COLUMN fanout_tracestate text DEFAULT ''::text NOT NULL",
				"CREATE INDEX idx_platform_events_notification_fanout_status",
				"ADD COLUMN traceparent text DEFAULT ''::text NOT NULL",
				"ADD COLUMN tracestate text DEFAULT ''::text NOT NULL",
			},
		},
		{
			name: "000092_notification_fanout_outbox.down.sql",
			required: []string{
				"DROP COLUMN tracestate",
				"DROP COLUMN traceparent",
				"DROP INDEX IF EXISTS idx_platform_events_notification_fanout_status",
				"DROP COLUMN notification_fanout_status",
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

func TestNotificationFanoutOutboxMigrationRoundTripPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "notification_fanout_outbox_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(91); err != nil {
		t.Fatalf("migrate isolated database to version 91: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 91)

	if err := db.Exec(`
INSERT INTO platform_events (
  id, type, category, severity, status, actor_id, occurred_at, created_at
) VALUES (
  'evt_fanout_existing', 'build.failed', 'build', 'error', 'failed', 'usr_actor', now(), now()
);
INSERT INTO notification_deliveries (
  id, event_id, event_type, channel_id, adapter_kind, recipient_user_id, queued_at, created_at, updated_at
) VALUES (
  'ndl_fanout_existing', 'evt_fanout_existing', 'build.failed', 'notification:user-email', 'smtp', 'usr_actor', now(), now(), now()
)`).Error; err != nil {
		t.Fatalf("seed version 91 notification records: %v", err)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply notification fanout outbox migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 92)
	for _, column := range []string{"notification_fanout_status", "fanout_traceparent", "fanout_tracestate"} {
		if !db.Migrator().HasColumn("platform_events", column) {
			t.Fatalf("platform_events.%s is missing", column)
		}
	}
	for _, column := range []string{"traceparent", "tracestate"} {
		if !db.Migrator().HasColumn("notification_deliveries", column) {
			t.Fatalf("notification_deliveries.%s is missing", column)
		}
	}
	if !db.Migrator().HasIndex("platform_events", "idx_platform_events_notification_fanout_status") {
		t.Fatal("platform event fanout status index is missing")
	}

	var existing struct {
		NotificationFanoutStatus string
		FanoutTraceparent        string
		FanoutTracestate         string
	}
	if err := db.Table("platform_events").Where("id = ?", "evt_fanout_existing").Scan(&existing).Error; err != nil {
		t.Fatalf("read migrated platform event defaults: %v", err)
	}
	if existing.NotificationFanoutStatus != "" || existing.FanoutTraceparent != "" || existing.FanoutTracestate != "" {
		t.Fatalf("existing platform event fanout values = %#v, want empty defaults", existing)
	}
	if err := db.Table("platform_events").Where("id = ?", "evt_fanout_existing").Update("notification_fanout_status", "invalid").Error; err == nil {
		t.Fatal("database accepted invalid notification fanout status")
	}
	if err := db.Table("platform_events").Where("id = ?", "evt_fanout_existing").Updates(map[string]any{
		"notification_fanout_status": "pending",
		"fanout_traceparent":         "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01",
		"fanout_tracestate":          "vendor=value",
	}).Error; err != nil {
		t.Fatalf("write notification fanout state: %v", err)
	}
	if err := db.Table("notification_deliveries").Where("id = ?", "ndl_fanout_existing").Updates(map[string]any{
		"traceparent": "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01",
		"tracestate":  "vendor=value",
	}).Error; err != nil {
		t.Fatalf("write delivery trace context: %v", err)
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back notification fanout outbox migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 91)
	for _, column := range []string{"notification_fanout_status", "fanout_traceparent", "fanout_tracestate"} {
		if db.Migrator().HasColumn("platform_events", column) {
			t.Fatalf("platform_events.%s remains after rollback", column)
		}
	}
	for _, column := range []string{"traceparent", "tracestate"} {
		if db.Migrator().HasColumn("notification_deliveries", column) {
			t.Fatalf("notification_deliveries.%s remains after rollback", column)
		}
	}
}
