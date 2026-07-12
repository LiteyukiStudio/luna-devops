package database

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMFAMigrationBackfillsLegacyAssertionBeforeDroppingExpiresAt(t *testing.T) {
	data, err := sqlmigrations.FS.ReadFile("000033_mfa_step_up.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	backfill := strings.Index(sql, "idle_expires_at = COALESCE(idle_expires_at, expires_at)")
	setNotNull := strings.Index(sql, "ALTER TABLE step_up_assertions ALTER COLUMN idle_expires_at SET NOT NULL")
	dropLegacy := strings.Index(sql, "ALTER TABLE step_up_assertions DROP COLUMN IF EXISTS expires_at")
	if backfill < 0 || setNotNull < 0 || dropLegacy < 0 {
		t.Fatalf("migration does not contain the expected legacy upgrade stages:\n%s", sql)
	}
	if !(backfill < setNotNull && setNotNull < dropLegacy) {
		t.Fatalf("legacy expires_at must be backfilled before constraints and removal")
	}
	if !strings.Contains(sql, "last_totp_counter bigint") {
		t.Fatal("MFA migration must persist the last accepted TOTP counter")
	}
	downData, err := sqlmigrations.FS.ReadFile("000033_mfa_step_up.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	downSQL := string(downData)
	deleteSecrets := strings.Index(downSQL, "DELETE FROM secret_values WHERE resource LIKE 'mfa:%:totp'")
	dropConfigs := strings.Index(downSQL, "DROP TABLE IF EXISTS user_mfa_configs")
	if deleteSecrets < 0 || dropConfigs < 0 || deleteSecrets > dropConfigs {
		t.Fatal("MFA secret rows must be deleted before MFA tables are dropped")
	}
}

func TestMFAMigrationUpgradesAndRollsBackLegacyAssertionInPostgres(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("mfa_migration_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration schema: %v", err)
	}
	defer func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := db.Exec(`CREATE TABLE users (id text PRIMARY KEY); CREATE TABLE user_sessions (id text PRIMARY KEY); CREATE TABLE secret_values (id text PRIMARY KEY, resource text NOT NULL); INSERT INTO users(id) VALUES ('usr_test'); INSERT INTO user_sessions(id) VALUES ('ses_test')`).Error; err != nil {
		t.Fatalf("create migration prerequisites: %v", err)
	}
	legacyMigration, err := sqlmigrations.FS.ReadFile("000023_step_up_assertions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(legacyMigration)).Error; err != nil {
		t.Fatalf("apply legacy migration: %v", err)
	}
	legacyExpiry := time.Now().UTC().Truncate(time.Second).Add(10 * time.Minute)
	if err := db.Exec(`INSERT INTO step_up_assertions(id, user_id, session_id, purpose, expires_at) VALUES (?, ?, ?, ?, ?)`, "mfaas_test", "usr_test", "ses_test", "runtime_exec", legacyExpiry).Error; err != nil {
		t.Fatalf("insert legacy assertion: %v", err)
	}

	upMigration, err := sqlmigrations.FS.ReadFile("000033_mfa_step_up.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(upMigration)).Error; err != nil {
		t.Fatalf("apply MFA migration: %v", err)
	}
	var upgraded struct {
		VerifiedAt        time.Time
		LastActivityAt    time.Time
		IdleExpiresAt     time.Time
		AbsoluteExpiresAt time.Time
	}
	if err := db.Raw(`SELECT verified_at, last_activity_at, idle_expires_at, absolute_expires_at FROM step_up_assertions WHERE id = ?`, "mfaas_test").Scan(&upgraded).Error; err != nil {
		t.Fatalf("read upgraded assertion: %v", err)
	}
	if !upgraded.IdleExpiresAt.Equal(legacyExpiry) || !upgraded.AbsoluteExpiresAt.Equal(legacyExpiry) {
		t.Fatalf("legacy expiry was not preserved: %#v", upgraded)
	}
	if upgraded.VerifiedAt.IsZero() || upgraded.LastActivityAt.IsZero() {
		t.Fatalf("legacy activity timestamps were not backfilled: %#v", upgraded)
	}
	if err := db.Exec(`INSERT INTO secret_values(id, resource) VALUES ('sec_mfa', 'mfa:usr_test:totp'), ('sec_other', 'registry:usr_test')`).Error; err != nil {
		t.Fatalf("insert migration secrets: %v", err)
	}

	downMigration, err := sqlmigrations.FS.ReadFile("000033_mfa_step_up.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(downMigration)).Error; err != nil {
		t.Fatalf("roll back MFA migration: %v", err)
	}
	var rolledBackExpiry time.Time
	if err := db.Raw(`SELECT expires_at FROM step_up_assertions WHERE id = ?`, "mfaas_test").Scan(&rolledBackExpiry).Error; err != nil {
		t.Fatalf("read rolled-back assertion: %v", err)
	}
	if !rolledBackExpiry.Equal(legacyExpiry) {
		t.Fatalf("rollback expiry = %s, want %s", rolledBackExpiry, legacyExpiry)
	}
	var mfaSecretCount, otherSecretCount int64
	if err := db.Raw(`SELECT count(*) FROM secret_values WHERE id = 'sec_mfa'`).Scan(&mfaSecretCount).Error; err != nil || mfaSecretCount != 0 {
		t.Fatalf("MFA secret cleanup count=%d err=%v", mfaSecretCount, err)
	}
	if err := db.Raw(`SELECT count(*) FROM secret_values WHERE id = 'sec_other'`).Scan(&otherSecretCount).Error; err != nil || otherSecretCount != 1 {
		t.Fatalf("unrelated secret cleanup count=%d err=%v", otherSecretCount, err)
	}
}
