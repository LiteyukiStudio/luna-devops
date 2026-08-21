package database

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestRememberTokenFamilyMigrationUpgradesLegacyRowsInPostgres(t *testing.T) {
	db := testdb.Open(t, testdb.Options{SchemaPrefix: "remember_migration_test"})

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
