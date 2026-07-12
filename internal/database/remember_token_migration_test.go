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

func TestRememberTokenFamilyMigrationBackfillsBeforeNotNull(t *testing.T) {
	data, err := sqlmigrations.FS.ReadFile("000035_remember_token_families.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	backfill := strings.Index(sql, "UPDATE user_remember_tokens SET family_id = id")
	notNull := strings.Index(sql, "ALTER TABLE user_remember_tokens ALTER COLUMN family_id SET NOT NULL")
	if backfill < 0 || notNull < 0 || backfill >= notNull {
		t.Fatalf("family_id must be backfilled before adding NOT NULL:\n%s", sql)
	}
	for _, fragment := range []string{"consumed_at timestamptz", "revoked_at timestamptz", "remember_family_id text NOT NULL DEFAULT ''"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestRememberTokenFamilyMigrationUpgradesLegacyRowsInPostgres(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("remember_migration_test_%d", time.Now().UnixNano())
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

	if err := db.Exec(`
CREATE TABLE users (id text PRIMARY KEY);
CREATE TABLE user_sessions (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE user_remember_tokens (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO users(id) VALUES ('usr_test');
INSERT INTO user_sessions(id, user_id) VALUES ('ses_test', 'usr_test');
INSERT INTO user_remember_tokens(id, user_id, token_hash, expires_at)
VALUES ('rem_test', 'usr_test', 'hash', now() + interval '1 day');
`).Error; err != nil {
		t.Fatalf("create migration prerequisites: %v", err)
	}
	upMigration, err := sqlmigrations.FS.ReadFile("000035_remember_token_families.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(upMigration)).Error; err != nil {
		t.Fatalf("apply remember-token migration: %v", err)
	}
	var upgraded struct {
		FamilyID   string
		ConsumedAt *time.Time
		RevokedAt  *time.Time
	}
	if err := db.Raw(`SELECT family_id, consumed_at, revoked_at FROM user_remember_tokens WHERE id = 'rem_test'`).Scan(&upgraded).Error; err != nil {
		t.Fatalf("read upgraded remember token: %v", err)
	}
	if upgraded.FamilyID != "rem_test" || upgraded.ConsumedAt != nil || upgraded.RevokedAt != nil {
		t.Fatalf("upgraded remember token = %#v", upgraded)
	}

	downMigration, err := sqlmigrations.FS.ReadFile("000035_remember_token_families.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(downMigration)).Error; err != nil {
		t.Fatalf("roll back remember-token migration: %v", err)
	}
}
