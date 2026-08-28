package database

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"gorm.io/gorm"
)

func TestPersonalNotificationsAndPlatformMailMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{name: "000089_personal_notifications_and_platform_mail.up.sql", required: []string{
			"CREATE TABLE platform_mail_settings", "FROM auth_registration_settings",
			"CREATE TABLE user_notification_preferences", "email_enabled boolean DEFAULT true",
			"ADD COLUMN owner_user_id", "ADD COLUMN recipient_user_id",
			"idx_notification_deliveries_event_channel_recipient",
		}},
		{name: "000089_personal_notifications_and_platform_mail.down.sql", required: []string{
			"ADD COLUMN smtp_host", "smtp_password_ref", "FROM platform_mail_settings",
			"DELETE FROM notification_deliveries", "DELETE FROM notification_channels",
			"DROP TABLE user_notification_preferences", "DROP COLUMN owner_user_id",
			"DROP COLUMN recipient_user_id", "idx_notification_deliveries_event_channel",
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

func TestPersonalNotificationsAndPlatformMailMigrationRoundTripPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "personal_notifications_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(88); err != nil {
		t.Fatalf("migrate isolated database to version 88: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 88)

	if err := db.Exec(`
INSERT INTO auth_registration_settings (
  id, allow_email_registration, allow_oidc_registration, allow_external_identity_password,
  smtp_host, smtp_port, smtp_security, smtp_username, smtp_password_ref,
  smtp_from_address, smtp_from_name
) VALUES (
  'default', true, true, false,
  'smtp.old.example', 465, 'tls', 'mailer', 'secret:old',
  'noreply@old.example', 'Old Sender'
)`).Error; err != nil {
		t.Fatalf("seed version 88 registration SMTP settings: %v", err)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply personal notification migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 89)
	assertPersonalNotificationSchema(t, db, true)

	var mail struct {
		Host        string
		Port        int
		Security    string
		Username    string
		PasswordRef string
		FromAddress string
		FromName    string
	}
	if err := db.Table("platform_mail_settings").Where("id = ?", "default").Take(&mail).Error; err != nil {
		t.Fatalf("read migrated platform mail settings: %v", err)
	}
	if mail.Host != "smtp.old.example" || mail.Port != 465 || mail.Security != "tls" ||
		mail.Username != "mailer" || mail.PasswordRef != "secret:old" ||
		mail.FromAddress != "noreply@old.example" || mail.FromName != "Old Sender" {
		t.Fatalf("migrated platform mail settings = %#v", mail)
	}

	if err := db.Exec(`INSERT INTO user_notification_preferences(user_id) VALUES ('usr_default')`).Error; err != nil {
		t.Fatalf("insert default notification preference: %v", err)
	}
	var preference struct {
		EmailEnabled   bool
		EventTypesJSON string
	}
	if err := db.Table("user_notification_preferences").
		Select("email_enabled, event_types_json::text AS event_types_json").
		Where("user_id = ?", "usr_default").Take(&preference).Error; err != nil {
		t.Fatalf("read default notification preference: %v", err)
	}
	if !preference.EmailEnabled || !strings.Contains(preference.EventTypesJSON, "build.failed") {
		t.Fatalf("default notification preference = %#v", preference)
	}

	insertDelivery := func(id, recipient string) error {
		return db.Exec(`INSERT INTO notification_deliveries(
  id, event_id, event_type, channel_id, adapter_kind, recipient_user_id
) VALUES (?, 'evt_personal', 'build.failed', 'notification:user-email', 'smtp', ?)`, id, recipient).Error
	}
	if err := insertDelivery("ndl_first", "usr_first"); err != nil {
		t.Fatalf("insert first recipient delivery: %v", err)
	}
	if err := insertDelivery("ndl_second", "usr_second"); err != nil {
		t.Fatalf("same event and channel must allow a different recipient: %v", err)
	}
	if err := insertDelivery("ndl_duplicate", "usr_first"); err == nil {
		t.Fatal("same event, channel, and recipient unexpectedly bypassed delivery uniqueness")
	}
	if err := db.Exec(`INSERT INTO notification_channels(
  id, owner_user_id, name, adapter_kind, config_json, secret_refs_json
) VALUES (
  'nch_personal', 'usr_first', 'personal webhook', 'webhook', '{}', '{}'
)`).Error; err != nil {
		t.Fatalf("insert personal channel before rollback: %v", err)
	}
	if err := db.Table("platform_mail_settings").Where("id = ?", "default").Updates(map[string]any{
		"host": "smtp.new.example", "password_ref": "secret:new", "from_address": "noreply@new.example",
	}).Error; err != nil {
		t.Fatalf("update migrated platform mail settings: %v", err)
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back personal notification migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 88)
	assertPersonalNotificationSchema(t, db, false)
	for table, ids := range map[string][]string{
		"notification_deliveries": {"ndl_first", "ndl_second"},
		"notification_channels":   {"nch_personal"},
	} {
		var count int64
		if err := db.Table(table).Where("id in ?", ids).Count(&count).Error; err != nil {
			t.Fatalf("count rolled-back %s rows: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("rolled-back personal rows remain in %s: %d", table, count)
		}
	}

	var restored struct {
		SMTPHost        string
		SMTPPasswordRef string
		SMTPFromAddress string
	}
	if err := db.Table("auth_registration_settings").Where("id = ?", "default").Take(&restored).Error; err != nil {
		t.Fatalf("read restored registration SMTP settings: %v", err)
	}
	if restored.SMTPHost != "smtp.new.example" || restored.SMTPPasswordRef != "secret:new" || restored.SMTPFromAddress != "noreply@new.example" {
		t.Fatalf("restored registration SMTP settings = %#v", restored)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("reapply personal notification migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 89)
	assertPersonalNotificationSchema(t, db, true)
}

func assertPersonalNotificationSchema(t *testing.T, db *gorm.DB, migrated bool) {
	t.Helper()
	for _, table := range []string{"platform_mail_settings", "user_notification_preferences"} {
		if got := db.Migrator().HasTable(table); got != migrated {
			t.Fatalf("table %s present = %t, want %t", table, got, migrated)
		}
	}
	for _, column := range []string{"owner_user_id"} {
		if got := db.Migrator().HasColumn("notification_channels", column); got != migrated {
			t.Fatalf("notification_channels.%s present = %t, want %t", column, got, migrated)
		}
	}
	for _, column := range []string{"recipient_user_id"} {
		if got := db.Migrator().HasColumn("notification_deliveries", column); got != migrated {
			t.Fatalf("notification_deliveries.%s present = %t, want %t", column, got, migrated)
		}
	}
	for _, column := range []string{"smtp_host", "smtp_password_ref", "smtp_from_address"} {
		if got := db.Migrator().HasColumn("auth_registration_settings", column); got == migrated {
			t.Fatalf("auth_registration_settings.%s present = %t after migrated=%t", column, got, migrated)
		}
	}
}
