package database

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMigrateBootstrapsFreshPostgresSchema(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}

	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("fresh_database_migration_test_%d", time.Now().UnixNano())
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
		t.Fatalf("open fresh integration schema: %v", err)
	}
	defer func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate fresh database: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("repeat migration after fresh bootstrap: %v", err)
	}

	assertFreshMigrationState(t, db)
}

func assertFreshMigrationState(t *testing.T, db *gorm.DB) {
	t.Helper()

	var migrationState struct {
		Version uint
		Dirty   bool
	}
	if err := db.Raw(`SELECT version, dirty FROM schema_migrations`).Scan(&migrationState).Error; err != nil {
		t.Fatalf("read migration state: %v", err)
	}
	if migrationState.Dirty {
		t.Fatalf("fresh database migration is dirty at version %d", migrationState.Version)
	}
	latestVersion := latestEmbeddedMigrationVersion(t)
	if migrationState.Version != latestVersion {
		t.Fatalf("migration version = %d, want %d", migrationState.Version, latestVersion)
	}

	for _, table := range []string{
		"billing_rate_rules",
		"billing_usage_records",
		"billing_ledger_entries",
		"user_wallets",
	} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("fresh database is missing table %s", table)
		}
	}
	for _, expected := range []struct {
		table  string
		column string
	}{
		{table: "billing_usage_records", column: "billed_user_id"},
		{table: "billing_ledger_entries", column: "idempotency_key"},
		{table: "billing_ledger_entries", column: "user_id"},
	} {
		if !db.Migrator().HasColumn(expected.table, expected.column) {
			t.Fatalf("fresh database is missing %s.%s", expected.table, expected.column)
		}
	}

	var defaultRuleCount int64
	if err := db.Table("billing_rate_rules").Count(&defaultRuleCount).Error; err != nil {
		t.Fatalf("count default billing rules: %v", err)
	}
	if defaultRuleCount == 0 {
		t.Fatal("fresh database did not seed default billing rules")
	}

}

func latestEmbeddedMigrationVersion(t *testing.T) uint {
	t.Helper()

	entries, err := sqlmigrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var latest uint64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		prefix, _, found := strings.Cut(name, "_")
		if !found {
			t.Fatalf("invalid migration filename %q", name)
		}
		version, parseErr := strconv.ParseUint(prefix, 10, 64)
		if parseErr != nil {
			t.Fatalf("parse migration version from %q: %v", name, parseErr)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		t.Fatal("no embedded up migrations found")
	}
	return uint(latest)
}
