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

func TestWebConsolePolicyMigrationIsExplicitlyIrreversible(t *testing.T) {
	downMigration, err := sqlmigrations.FS.ReadFile("000034_web_console_policy.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	downSQL := strings.ToLower(string(downMigration))
	if !strings.Contains(downSQL, "irreversible") || !strings.Contains(downSQL, "raise exception") {
		t.Fatalf("down migration must fail explicitly instead of dropping policy columns:\n%s", downSQL)
	}
	if strings.Contains(downSQL, "drop column") {
		t.Fatalf("down migration must not discard Web Console policy values:\n%s", downSQL)
	}
}

func TestWebConsolePolicyMigrationPreservesDisabledPoliciesOnFailedRollbackAndReapply(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("web_console_migration_test_%d", time.Now().UnixNano())
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
		CREATE TABLE projects (id text PRIMARY KEY);
		CREATE TABLE deployment_targets (id text PRIMARY KEY);
		INSERT INTO projects(id) VALUES ('prj_test');
		INSERT INTO deployment_targets(id) VALUES ('dplt_test');
	`).Error; err != nil {
		t.Fatalf("create migration prerequisites: %v", err)
	}
	upMigration, err := sqlmigrations.FS.ReadFile("000034_web_console_policy.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(upMigration)).Error; err != nil {
		t.Fatalf("apply Web Console policy migration: %v", err)
	}
	if err := db.Exec(`UPDATE projects SET web_console_enabled = false; UPDATE deployment_targets SET web_console_enabled = false`).Error; err != nil {
		t.Fatalf("disable Web Console policies: %v", err)
	}

	downMigration, err := sqlmigrations.FS.ReadFile("000034_web_console_policy.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(downMigration)).Error; err == nil {
		t.Fatal("expected the irreversible down migration to fail")
	}
	if err := db.Exec(string(upMigration)).Error; err != nil {
		t.Fatalf("reapply Web Console policy migration: %v", err)
	}

	var projectEnabled bool
	if err := db.Raw(`SELECT web_console_enabled FROM projects WHERE id = 'prj_test'`).Scan(&projectEnabled).Error; err != nil {
		t.Fatalf("read project policy: %v", err)
	}
	var targetEnabled bool
	if err := db.Raw(`SELECT web_console_enabled FROM deployment_targets WHERE id = 'dplt_test'`).Scan(&targetEnabled).Error; err != nil {
		t.Fatalf("read deployment policy: %v", err)
	}
	if projectEnabled || targetEnabled {
		t.Fatalf("disabled policies were not preserved: project=%v target=%v", projectEnabled, targetEnabled)
	}
}
