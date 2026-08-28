package database

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestPlatformMailCooldownMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{
			name: "000090_platform_mail_personal_email_cooldown.up.sql",
			required: []string{
				"ADD COLUMN personal_email_cooldown_seconds integer DEFAULT 60 NOT NULL",
				"CHECK (personal_email_cooldown_seconds BETWEEN 0 AND 3600)",
			},
		},
		{
			name: "000090_platform_mail_personal_email_cooldown.down.sql",
			required: []string{
				"DROP CONSTRAINT IF EXISTS platform_mail_settings_personal_email_cooldown_seconds_range",
				"DROP COLUMN personal_email_cooldown_seconds",
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

func TestPlatformMailCooldownMigrationRoundTripPostgres(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "platform_mail_cooldown_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(89); err != nil {
		t.Fatalf("migrate isolated database to version 89: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 89)

	if err := db.Exec(`
INSERT INTO platform_mail_settings (id, host, from_address)
VALUES ('default', 'smtp.example.com', 'noreply@example.com')
ON CONFLICT (id) DO UPDATE
SET host = EXCLUDED.host,
    from_address = EXCLUDED.from_address`).Error; err != nil {
		t.Fatalf("seed version 89 mail settings: %v", err)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply platform mail cooldown migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 90)
	if !db.Migrator().HasColumn("platform_mail_settings", "personal_email_cooldown_seconds") {
		t.Fatal("platform_mail_settings.personal_email_cooldown_seconds is missing")
	}

	var cooldown int
	if err := db.Table("platform_mail_settings").
		Select("personal_email_cooldown_seconds").
		Where("id = ?", "default").
		Scan(&cooldown).Error; err != nil {
		t.Fatalf("read default personal email cooldown: %v", err)
	}
	if cooldown != 60 {
		t.Fatalf("default personal email cooldown = %d, want 60", cooldown)
	}
	for _, invalid := range []int{-1, 3601} {
		if err := db.Table("platform_mail_settings").
			Where("id = ?", "default").
			Update("personal_email_cooldown_seconds", invalid).Error; err == nil {
			t.Fatalf("database accepted personal email cooldown %d", invalid)
		}
	}
	if err := db.Table("platform_mail_settings").
		Where("id = ?", "default").
		Update("personal_email_cooldown_seconds", 0).Error; err != nil {
		t.Fatalf("database rejected disabled personal email cooldown: %v", err)
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back platform mail cooldown migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 89)
	if db.Migrator().HasColumn("platform_mail_settings", "personal_email_cooldown_seconds") {
		t.Fatal("personal email cooldown column remains after rollback")
	}
}
