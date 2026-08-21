package database

import (
	"database/sql"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestSessionPrimaryAuthenticationMigrationPreservesNullLegacyValueInPostgres(t *testing.T) {
	db := testdb.Open(t, testdb.Options{SchemaPrefix: "session_primary_auth_migration_test"})

	if err := db.Exec(`
CREATE TABLE user_sessions (
  id text PRIMARY KEY,
  created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO user_sessions(id) VALUES ('ses_legacy');
`).Error; err != nil {
		t.Fatalf("create migration prerequisite: %v", err)
	}
	upMigration, err := sqlmigrations.FS.ReadFile("000036_session_primary_authentication.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(upMigration)).Error; err != nil {
		t.Fatalf("apply session primary-authentication migration: %v", err)
	}
	var primaryAuthenticatedAt sql.NullTime
	if err := db.Raw(`SELECT primary_authenticated_at FROM user_sessions WHERE id = 'ses_legacy'`).Scan(&primaryAuthenticatedAt).Error; err != nil {
		t.Fatalf("read upgraded legacy session: %v", err)
	}
	if primaryAuthenticatedAt.Valid {
		t.Fatalf("legacy session was incorrectly marked fresh: %v", primaryAuthenticatedAt)
	}

	downMigration, err := sqlmigrations.FS.ReadFile("000036_session_primary_authentication.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(downMigration)).Error; err != nil {
		t.Fatalf("roll back session primary-authentication migration: %v", err)
	}
}
