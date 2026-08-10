package database

import (
	"context"
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
	adminSQLDB, err := adminDB.DB()
	if err != nil {
		t.Fatalf("open integration database connection: %v", err)
	}
	t.Cleanup(func() { _ = adminSQLDB.Close() })

	type sourceDatabaseState struct {
		HasMigrationTable bool
		HasAISchema       bool
	}
	readSourceState := func() sourceDatabaseState {
		var state sourceDatabaseState
		if stateErr := adminDB.Raw(`SELECT
  to_regclass('schema_migrations') IS NOT NULL AS has_migration_table,
  to_regnamespace('ai') IS NOT NULL AS has_ai_schema`).Scan(&state).Error; stateErr != nil {
			t.Fatalf("inspect source integration database: %v", stateErr)
		}
		return state
	}
	sourceState := readSourceState()
	t.Cleanup(func() {
		if currentState := readSourceState(); currentState != sourceState {
			t.Errorf("migration test changed source database state: before=%+v after=%+v", sourceState, currentState)
		}
	})

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	databaseName := fmt.Sprintf("luna_migration_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(databaseName, "luna_migration_test_") {
		t.Fatalf("refuse unsafe migration test database name %q", databaseName)
	}
	if err := adminDB.Exec(`CREATE DATABASE "` + databaseName + `"`).Error; err != nil {
		t.Fatalf("create isolated migration test database: %v", err)
	}
	t.Cleanup(func() {
		if dropErr := adminDB.Exec(`DROP DATABASE IF EXISTS "` + databaseName + `" WITH (FORCE)`).Error; dropErr != nil {
			t.Errorf("drop isolated migration test database: %v", dropErr)
		}
	})

	parsedURL.Path = "/" + databaseName
	parsedURL.RawPath = ""
	query := parsedURL.Query()
	query.Del("search_path")
	parsedURL.RawQuery = query.Encode()
	testDB, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated migration test database: %v", err)
	}
	defer func() {
		if sqlDB, dbErr := testDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := MigrateContext(context.Background(), testDB); err != nil {
		t.Fatalf("migrate fresh database: %v", err)
	}
	if err := MigrateContext(context.Background(), testDB); err != nil {
		t.Fatalf("repeat migration after fresh bootstrap: %v", err)
	}

	assertFreshMigrationState(t, testDB)
	assertActiveDeploymentStageUniqueness(t, testDB)
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
		"service_bindings",
		"project_topology_edges",
		"oauth_applications",
		"oauth_grants",
		"oauth_authorization_codes",
		"oauth_refresh_tokens",
		"auth_registration_settings",
		"email_registration_challenges",
		"ai.ui_actions",
		"ai.conversation_summaries",
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
		{table: "access_tokens", column: "oauth_application_id"},
		{table: "access_tokens", column: "oauth_grant_id"},
		{table: "auth_registration_settings", column: "allow_oidc_registration"},
		{table: "projects", column: "identifier"},
		{table: "projects", column: "kubernetes_namespace"},
		{table: "applications", column: "identifier"},
		{table: "deployment_targets", column: "kubernetes_name"},
		{table: "ai.runs", column: "client_instance_id"},
		{table: "ai.runs", column: "next_item_position"},
		{table: "ai.runs", column: "next_event_sequence"},
		{table: "ai.items", column: "revision"},
	} {
		if !db.Migrator().HasColumn(expected.table, expected.column) {
			t.Fatalf("fresh database is missing %s.%s", expected.table, expected.column)
		}
	}
	for _, table := range []string{
		"o_auth_applications",
		"o_auth_grants",
		"o_auth_authorization_codes",
		"o_auth_refresh_tokens",
	} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("fresh database contains legacy OAuth table %s", table)
		}
	}
	for _, column := range []string{"o_auth_application_id", "o_auth_grant_id"} {
		if db.Migrator().HasColumn("access_tokens", column) {
			t.Fatalf("fresh database contains legacy access_tokens.%s", column)
		}
	}
	for _, obsolete := range []struct {
		table  string
		column string
	}{
		{table: "runtime_clusters", column: "status"},
		{table: "runtime_clusters", column: "last_checked_at"},
		{table: "service_bindings", column: "last_check_status"},
		{table: "service_bindings", column: "last_checked_at"},
		{table: "gateway_routes", column: "certificate_status"},
		{table: "gateway_routes", column: "certificate_message"},
		{table: "gateway_routes", column: "certificate_not_after"},
		{table: "gateway_routes", column: "certificate_issuer_kind"},
		{table: "gateway_routes", column: "certificate_issuer_name"},
		{table: "gateway_routes", column: "dns_status"},
		{table: "gateway_routes", column: "status"},
		{table: "git_accounts", column: "status"},
		{table: "repository_bindings", column: "webhook_status"},
		{table: "notification_channels", column: "last_delivery_status"},
		{table: "notification_channels", column: "last_delivery_error"},
		{table: "notification_channels", column: "last_delivered_at"},
		{table: "notification_rules", column: "last_matched_event_id"},
	} {
		if db.Migrator().HasColumn(obsolete.table, obsolete.column) {
			t.Fatalf("fresh database contains persisted live observation %s.%s", obsolete.table, obsolete.column)
		}
	}
	if !db.Migrator().HasColumn("repository_bindings", "webhook_enabled") {
		t.Fatal("fresh database is missing desired-state repository_bindings.webhook_enabled")
	}
	if db.Migrator().HasColumn("users", "auth_type") {
		t.Fatal("fresh database contains obsolete users.auth_type")
	}
	for _, table := range []string{"projects", "applications"} {
		if db.Migrator().HasColumn(table, "slug") {
			t.Fatalf("fresh database contains obsolete %s.slug", table)
		}
	}
	var activeStageIndex string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'idx_deployment_targets_application_stage_active'`).Scan(&activeStageIndex).Error; err != nil {
		t.Fatalf("read deployment target active stage index: %v", err)
	}
	if !strings.Contains(activeStageIndex, "UNIQUE INDEX") || !strings.Contains(activeStageIndex, "WHERE (deleted_at IS NULL)") {
		t.Fatalf("deployment target active stage index is missing or invalid: %q", activeStageIndex)
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

func assertActiveDeploymentStageUniqueness(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now()
	if err := db.Exec(`INSERT INTO projects (id, identifier, kubernetes_namespace, name, namespace_strategy, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"prj_stage_test", "stage-test", "luna-stage-test", "Stage Test", "project", now, now).Error; err != nil {
		t.Fatalf("insert stage test project: %v", err)
	}
	if err := db.Exec(`INSERT INTO applications (id, project_id, identifier, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"app_stage_test", "prj_stage_test", "api", "API", now, now).Error; err != nil {
		t.Fatalf("insert stage test application: %v", err)
	}
	insertTarget := func(id string) error {
		return db.Exec(`INSERT INTO deployment_targets (id, project_id, application_id, environment_id, name, stage, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, "prj_stage_test", "app_stage_test", "", id, "prod", now, now).Error
	}
	if err := insertTarget("dplt_stage_first"); err != nil {
		t.Fatalf("insert first active deployment stage: %v", err)
	}
	if err := insertTarget("dplt_stage_duplicate"); err == nil {
		t.Fatal("duplicate active deployment stage unexpectedly succeeded")
	}
	if err := db.Exec(`UPDATE deployment_targets SET deleted_at = ? WHERE id = ?`, now, "dplt_stage_first").Error; err != nil {
		t.Fatalf("soft delete first deployment stage: %v", err)
	}
	if err := insertTarget("dplt_stage_reused"); err != nil {
		t.Fatalf("reuse deployment stage after soft deletion: %v", err)
	}
}
