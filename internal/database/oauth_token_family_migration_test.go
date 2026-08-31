package database

import (
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestOAuthTokenFamilyMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{
			name: "000094_oauth_token_families.up.sql",
			required: []string{
				"ADD COLUMN oauth_family_id text DEFAULT ''::text NOT NULL",
				"SET oauth_family_id = oauth_grant_id",
				"WHERE oauth_family_id <> ''",
				"ADD COLUMN family_id text DEFAULT ''::text NOT NULL",
				"SET family_id = grant_id",
				"ALTER COLUMN grant_id DROP NOT NULL",
			},
		},
		{
			name: "000094_oauth_token_families.down.sql",
			required: []string{
				"INSERT INTO oauth_grants",
				"consumed_at = COALESCE(consumed_at, now())",
				"ALTER COLUMN grant_id SET NOT NULL",
				"DROP COLUMN family_id",
				"DROP COLUMN oauth_family_id",
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

func TestOAuthTokenFamilyMigrationRoundTripPreservesCredentialsAndCodes(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "oauth_token_family_migration_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(93); err != nil {
		t.Fatalf("migrate isolated database to version 93: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 93)

	now := time.Now()
	if err := db.Exec(`INSERT INTO users (id, email, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"usr_oauth_family", "oauth-family@example.com", "OAuth Family", "admin", now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_applications (id, name, client_id, client_secret_hash, redirect_uris, allowed_scopes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"oapp_oauth_family", "OAuth Family App", "oauth-family-app", "secret-hash", `["https://example.com/callback"]`, "user:read,volume:export", now, now).Error; err != nil {
		t.Fatalf("insert OAuth application: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_grants (id, application_id, user_id, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"ogrt_oauth_family", "oapp_oauth_family", "usr_oauth_family", "user:read", now, now).Error; err != nil {
		t.Fatalf("insert OAuth grant: %v", err)
	}
	if err := db.Exec(`INSERT INTO access_tokens (id, user_id, name, scope, token_hash, source, oauth_application_id, oauth_grant_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"tok_oauth_family", "usr_oauth_family", "OAuth Family App", "user:read", "access-token-hash", "oauth", "oapp_oauth_family", "ogrt_oauth_family", now, now).Error; err != nil {
		t.Fatalf("insert OAuth access token: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_refresh_tokens (id, application_id, grant_id, user_id, token_hash, scope, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ortk_oauth_family", "oapp_oauth_family", "ogrt_oauth_family", "usr_oauth_family", "refresh-token-hash", "user:read", now.Add(time.Hour), now, now).Error; err != nil {
		t.Fatalf("insert OAuth refresh token: %v", err)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply OAuth token family migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 94)
	var accessFamilyID string
	if err := db.Table("access_tokens").Select("oauth_family_id").Where("id = ?", "tok_oauth_family").Scan(&accessFamilyID).Error; err != nil {
		t.Fatalf("read migrated access token family: %v", err)
	}
	var refreshFamilyID string
	if err := db.Table("oauth_refresh_tokens").Select("family_id").Where("id = ?", "ortk_oauth_family").Scan(&refreshFamilyID).Error; err != nil {
		t.Fatalf("read migrated refresh token family: %v", err)
	}
	if accessFamilyID != "ogrt_oauth_family" || refreshFamilyID != "ogrt_oauth_family" {
		t.Fatalf("migrated families: access=%q refresh=%q", accessFamilyID, refreshFamilyID)
	}
	var accessFamilyIndex string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'idx_access_tokens_oauth_family_id'`).Scan(&accessFamilyIndex).Error; err != nil {
		t.Fatalf("read access token family index: %v", err)
	}
	if !strings.Contains(accessFamilyIndex, "WHERE (oauth_family_id <> ''::text)") {
		t.Fatalf("access token family index is not partial: %q", accessFamilyIndex)
	}
	for index, scope := range []string{"user:read", "user:read,volume:export"} {
		if err := db.Exec(`INSERT INTO oauth_authorization_codes (id, application_id, grant_id, user_id, code_hash, redirect_uri, scope, code_challenge, code_challenge_method, expires_at, created_at) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"ocod_pending_"+string(rune('a'+index)), "oapp_oauth_family", "usr_oauth_family", "pending-code-hash-"+string(rune('a'+index)),
			"https://example.com/callback", scope, strings.Repeat("a", 43), "S256", now.Add(time.Minute), now).Error; err != nil {
			t.Fatalf("insert pending authorization code %d: %v", index, err)
		}
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back OAuth token family migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 93)
	if db.Migrator().HasColumn("access_tokens", "oauth_family_id") || db.Migrator().HasColumn("oauth_refresh_tokens", "family_id") {
		t.Fatal("family columns remain after rollback")
	}
	var codeCount int64
	if err := db.Table("oauth_authorization_codes").Where("id LIKE ?", "ocod_pending_%").Count(&codeCount).Error; err != nil {
		t.Fatalf("count rolled-back authorization codes: %v", err)
	}
	if codeCount != 2 {
		t.Fatalf("authorization code count = %d, want 2", codeCount)
	}
	var unsafeCodeCount int64
	if err := db.Table("oauth_authorization_codes").Where("id LIKE ? AND (grant_id IS NULL OR consumed_at IS NULL)", "ocod_pending_%").Count(&unsafeCodeCount).Error; err != nil {
		t.Fatalf("inspect rolled-back authorization codes: %v", err)
	}
	if unsafeCodeCount != 0 {
		t.Fatalf("unsafe rolled-back authorization code count = %d", unsafeCodeCount)
	}
	var grantIDNullable string
	if err := db.Raw(`SELECT is_nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'oauth_authorization_codes' AND column_name = 'grant_id'`).Scan(&grantIDNullable).Error; err != nil {
		t.Fatalf("inspect rolled-back grant_id nullability: %v", err)
	}
	if grantIDNullable != "NO" {
		t.Fatalf("rolled-back grant_id is_nullable = %q, want NO", grantIDNullable)
	}
	var tombstoneCount int64
	if err := db.Table("oauth_grants").Where("id LIKE ? AND revoked_at IS NOT NULL", "ogrt_rollback_%").Count(&tombstoneCount).Error; err != nil {
		t.Fatalf("count rollback grant tombstones: %v", err)
	}
	if tombstoneCount != 2 {
		t.Fatalf("rollback grant tombstone count = %d, want 2", tombstoneCount)
	}
	for table, id := range map[string]string{"access_tokens": "tok_oauth_family", "oauth_refresh_tokens": "ortk_oauth_family"} {
		var count int64
		if err := db.Table(table).Where("id = ?", id).Count(&count).Error; err != nil {
			t.Fatalf("count preserved %s row: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("preserved %s row count = %d, want 1", table, count)
		}
	}
}
