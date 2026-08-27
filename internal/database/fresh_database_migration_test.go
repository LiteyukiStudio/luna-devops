package database

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	preReleaseBaselineVersion     = 67
	retiredAIDeliveryStateVersion = 87
)

func TestEmbeddedMigrationsStartAtPreReleaseBaseline(t *testing.T) {
	entries, err := sqlmigrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	var firstVersion uint64
	baselineFiles := map[string]bool{
		"000067_baseline.up.sql":   false,
		"000067_baseline.down.sql": false,
	}
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := baselineFiles[name]; ok {
			baselineFiles[name] = true
		}
		if entry.IsDir() || (!strings.HasSuffix(name, ".up.sql") && !strings.HasSuffix(name, ".down.sql")) {
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
		if firstVersion == 0 || version < firstVersion {
			firstVersion = version
		}
	}
	if firstVersion != preReleaseBaselineVersion {
		t.Fatalf("first embedded migration version = %d, want %d", firstVersion, preReleaseBaselineVersion)
	}
	for name, found := range baselineFiles {
		if !found {
			t.Fatalf("pre-release baseline is missing %s", name)
		}
	}
}

func TestMigrateBootstrapsFreshPostgresSchema(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}

	adminDB, err := gorm.Open(gormpostgres.Open(databaseURL), &gorm.Config{})
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
	testDB, err := gorm.Open(gormpostgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated migration test database: %v", err)
	}
	defer func() {
		if sqlDB, dbErr := testDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	}()

	runner := openTestMigrationRunner(t, testDB)
	if err := runner.Migrate(preReleaseBaselineVersion); err != nil {
		t.Fatalf("migrate database to pre-release baseline: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, preReleaseBaselineVersion)
	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back pre-release baseline: %v", err)
	}
	if testDB.Migrator().HasTable("users") {
		t.Fatal("baseline rollback retained owned public tables")
	}
	var hasAISchema bool
	if err := testDB.Raw(`SELECT to_regnamespace('ai') IS NOT NULL`).Scan(&hasAISchema).Error; err != nil {
		t.Fatalf("inspect AI schema after baseline rollback: %v", err)
	}
	if hasAISchema {
		t.Fatal("baseline rollback retained the owned AI schema")
	}
	if err := runner.Migrate(preReleaseBaselineVersion); err != nil {
		t.Fatalf("reapply pre-release baseline: %v", err)
	}
	if err := runner.Migrate(retiredAIDeliveryStateVersion); err != nil {
		t.Fatalf("migrate database to retired AI delivery state: %v", err)
	}
	seedRetiredAIDeliveryState(t, testDB)
	if err := MigrateContext(context.Background(), testDB); err != nil {
		t.Fatalf("migrate fresh database: %v", err)
	}
	assertRetiredAIDeliveryStateRemoved(t, testDB)
	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back retired AI delivery state migration: %v", err)
	}
	assertRetiredAIDeliveryStateSchemaRestored(t, testDB)
	if err := MigrateContext(context.Background(), testDB); err != nil {
		t.Fatalf("reapply retired AI delivery state migration: %v", err)
	}
	assertRetiredAIDeliveryStateRemoved(t, testDB)
	if err := MigrateContext(context.Background(), testDB); err != nil {
		t.Fatalf("repeat migration after fresh bootstrap: %v", err)
	}

	assertFreshMigrationState(t, testDB)
	assertStableModelMigrationCoverage(t, testDB)
	assertActiveDeploymentStageUniqueness(t, testDB)
	assertDirtyMigrationFailsClosed(t, testDB)
}

func seedRetiredAIDeliveryState(t *testing.T, db *gorm.DB) {
	t.Helper()

	statements := []string{
		`INSERT INTO users (id, email, name, brand_color_preset) VALUES ('usr_retired_theme', 'retired-theme@example.test', 'Retired Theme', 'ruby')`,
		`INSERT INTO app_configs (key, value) VALUES ('site.brandColorPreset', 'plum') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		`ALTER TABLE ai.runs ADD COLUMN IF NOT EXISTS client_instance_id text`,
		`ALTER TABLE ai.tool_calls DROP CONSTRAINT IF EXISTS tool_calls_approval_decision_check`,
		`ALTER TABLE ai.tool_calls ADD CONSTRAINT tool_calls_approval_decision_check CHECK (approval_decision IN ('approve', 'approve_always'))`,
		`CREATE TABLE ai.tool_approval_exemptions (
  user_id text NOT NULL,
  operation_id text NOT NULL,
  source_tool_call_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, operation_id)
)`,
		`CREATE TABLE ai.ui_actions (
  id text PRIMARY KEY,
  run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
  tool_call_id text NOT NULL UNIQUE,
  client_instance_id text NOT NULL,
  action jsonb NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed', 'expired')),
  attempts integer NOT NULL DEFAULT 1 CHECK (attempts > 0),
  expires_at timestamptz NOT NULL,
  acknowledged_at timestamptz,
  actual_path text,
  error_code text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
)`,
		`CREATE INDEX ai_ui_actions_pending_client_idx ON ai.ui_actions (client_instance_id, created_at) WHERE status = 'pending'`,
		`INSERT INTO ai.conversations (id, owner_user_id, title) VALUES ('conv_retired_delivery', 'usr_retired_delivery', 'Retired delivery state')`,
		`INSERT INTO ai.turns (id, conversation_id, turn_index, status, input, selected_run_id) VALUES ('turn_retired_delivery', 'conv_retired_delivery', 1, 'completed', 'test', 'run_retired_delivery')`,
		`INSERT INTO ai.runs (id, owner_user_id, conversation_id, turn_id, run_index, status, prompt_version, tool_catalog_digest, actor_session_id, client_instance_id) VALUES ('run_retired_delivery', 'usr_retired_delivery', 'conv_retired_delivery', 'turn_retired_delivery', 1, 'completed', 'v1', 'digest', 'ses_retired_delivery', 'client_retired_delivery')`,
		`INSERT INTO ai.tool_calls (id, run_id, operation_id, status, arguments, approval_decision) VALUES ('tool_retired_delivery', 'run_retired_delivery', 'restartRelease', 'succeeded', '{}'::jsonb, 'approve_always')`,
		`INSERT INTO ai.tool_approval_exemptions (user_id, operation_id, source_tool_call_id) VALUES ('usr_retired_delivery', 'restartRelease', 'tool_retired_delivery')`,
		`INSERT INTO ai.ui_actions (id, run_id, tool_call_id, client_instance_id, action, expires_at) VALUES ('uia_retired_delivery', 'run_retired_delivery', 'tool_retired_delivery', 'client_retired_delivery', '{"type":"navigate"}'::jsonb, now() + interval '5 minutes')`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed retired AI delivery state with %q: %v", statement, err)
		}
	}
}

func assertRetiredAIDeliveryStateRemoved(t *testing.T, db *gorm.DB) {
	t.Helper()

	for _, table := range []string{"ai.ui_actions", "ai.tool_approval_exemptions"} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("retired AI delivery table still exists: %s", table)
		}
	}
	if db.Migrator().HasColumn("ai.runs", "client_instance_id") {
		t.Fatal("retired ai.runs.client_instance_id still exists")
	}
	var decision string
	if err := db.Raw(`SELECT approval_decision FROM ai.tool_calls WHERE id = 'tool_retired_delivery'`).Scan(&decision).Error; err != nil {
		t.Fatalf("read migrated approval decision: %v", err)
	}
	if decision != "approve" {
		t.Fatalf("migrated approval decision = %q, want approve", decision)
	}
	var userPreset string
	if err := db.Raw(`SELECT brand_color_preset FROM users WHERE id = 'usr_retired_theme'`).Scan(&userPreset).Error; err != nil {
		t.Fatalf("read migrated user theme: %v", err)
	}
	if userPreset != "" {
		t.Fatalf("retired user theme = %q, want inherited empty preference", userPreset)
	}
	var sitePreset string
	if err := db.Raw(`SELECT value FROM app_configs WHERE key = 'site.brandColorPreset'`).Scan(&sitePreset).Error; err != nil {
		t.Fatalf("read migrated site theme: %v", err)
	}
	if sitePreset != "blue" {
		t.Fatalf("retired site theme = %q, want blue", sitePreset)
	}
}

func assertRetiredAIDeliveryStateSchemaRestored(t *testing.T, db *gorm.DB) {
	t.Helper()

	for _, table := range []string{"ai.ui_actions", "ai.tool_approval_exemptions"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("rollback did not restore retired AI delivery table %s", table)
		}
	}
	if !db.Migrator().HasColumn("ai.runs", "client_instance_id") {
		t.Fatal("rollback did not restore ai.runs.client_instance_id")
	}
	if err := db.Exec(`UPDATE ai.tool_calls SET approval_decision = 'approve_always' WHERE id = 'tool_retired_delivery'`).Error; err != nil {
		t.Fatalf("rollback did not restore approve_always constraint: %v", err)
	}
}

func TestMigrateRejectsUnversionedNonEmptySchema(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}

	adminDB, err := gorm.Open(gormpostgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schemaName := fmt.Sprintf("unversioned_schema_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schemaName)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(gormpostgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open unversioned integration schema: %v", err)
	}
	if err := db.Exec(`CREATE TABLE legacy_data (id text PRIMARY KEY)`).Error; err != nil {
		t.Fatalf("create unversioned table: %v", err)
	}

	err = MigrateContext(context.Background(), db)
	if !errors.Is(err, errUnversionedNonEmptySchema) {
		t.Fatalf("unversioned non-empty schema error = %v, want %v", err, errUnversionedNonEmptySchema)
	}
	if db.Migrator().HasTable("schema_migrations") {
		t.Fatal("failed preflight unexpectedly created schema_migrations")
	}
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
		"inbox_messages",
		"inbox_action_requests",
		"project_volume_quota_usage",
		"project_volume_quota_reservations",
		"ai.conversation_summaries",
		"ai.model_credit_holds",
		"ai.model_usages",
	} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("fresh database is missing table %s", table)
		}
	}
	for _, table := range []string{"ai.ui_actions", "ai.tool_approval_exemptions"} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("fresh database still contains retired table %s", table)
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
		{table: "user_remember_tokens", column: "family_id"},
		{table: "user_remember_tokens", column: "consumed_at"},
		{table: "user_remember_tokens", column: "revoked_at"},
		{table: "user_sessions", column: "remember_family_id"},
		{table: "user_sessions", column: "primary_authenticated_at"},
		{table: "projects", column: "identifier"},
		{table: "projects", column: "kubernetes_namespace"},
		{table: "projects", column: "web_console_enabled"},
		{table: "applications", column: "identifier"},
		{table: "deployment_targets", column: "kubernetes_name"},
		{table: "deployment_targets", column: "web_console_enabled"},
		{table: "inbox_messages", column: "recipient_user_id"},
		{table: "inbox_messages", column: "params_json"},
		{table: "inbox_action_requests", column: "row_version"},
		{table: "volume_transfers", column: "logical_bytes"},
		{table: "volume_transfers", column: "data_sha256"},
		{table: "volume_transfers", column: "execution_generation"},
		{table: "volume_transfers", column: "creation_lease_owner"},
		{table: "volume_transfers", column: "creation_lease_expires_at"},
		{table: "volume_transfers", column: "job_created_at"},
		{table: "ai.runs", column: "next_item_position"},
		{table: "ai.runs", column: "next_event_sequence"},
		{table: "ai.runs", column: "max_context_tokens"},
		{table: "ai.runs", column: "max_output_tokens"},
		{table: "ai.runs", column: "actor_session_id"},
		{table: "ai.runs", column: "row_version"},
		{table: "ai.tool_calls", column: "input_mode"},
		{table: "ai.tool_calls", column: "approval_decision"},
		{table: "ai.tool_calls", column: "row_version"},
		{table: "ai_models", column: "max_context_tokens"},
		{table: "ai_models", column: "max_output_tokens"},
		{table: "ai.items", column: "revision"},
	} {
		if !db.Migrator().HasColumn(expected.table, expected.column) {
			t.Fatalf("fresh database is missing %s.%s", expected.table, expected.column)
		}
	}
	if db.Migrator().HasColumn("ai.runs", "graph_version") {
		t.Fatal("fresh database contains obsolete ai.runs.graph_version")
	}
	for _, obsolete := range []struct {
		table  string
		column string
	}{
		{table: "applications", column: "source_type"},
		{table: "applications", column: "repository_url"},
		{table: "applications", column: "image_reference"},
		{table: "applications", column: "git_account_id"},
		{table: "applications", column: "service_port"},
		{table: "deployment_targets", column: "build_config_id"},
	} {
		if db.Migrator().HasColumn(obsolete.table, obsolete.column) {
			t.Fatalf("fresh database contains obsolete %s.%s", obsolete.table, obsolete.column)
		}
	}
	for _, column := range []string{"client_instance_id", "run_actor_grant_ciphertext", "lease_owner", "lease_expires_at", "heartbeat_at"} {
		if db.Migrator().HasColumn("ai.runs", column) {
			t.Fatalf("fresh database contains obsolete ai.runs.%s", column)
		}
	}
	var leaseFunctionCount int64
	if err := db.Raw(`SELECT count(*)
		FROM pg_proc AS procedure
		JOIN pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
		WHERE namespace.nspname = 'ai'
		  AND procedure.proname IN ('claim_next_run', 'renew_run_lease', 'release_run_lease')`).Scan(&leaseFunctionCount).Error; err != nil {
		t.Fatalf("inspect retired AI lease functions: %v", err)
	}
	if leaseFunctionCount != 0 {
		t.Fatalf("fresh database contains %d retired AI lease functions", leaseFunctionCount)
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
	for _, expected := range []struct {
		name      string
		fragments []string
	}{
		{
			name:      "idx_deployment_targets_application_stage_active",
			fragments: []string{"UNIQUE INDEX", "WHERE (deleted_at IS NULL)"},
		},
		{
			name:      "idx_inbox_messages_dedup_key",
			fragments: []string{"UNIQUE INDEX", "WHERE (dedup_key IS NOT NULL)"},
		},
		{name: "idx_platform_events_retention"},
		{name: "idx_notification_deliveries_retention_terminal", fragments: []string{"WHERE (status = ANY"}},
		{name: "idx_build_runs_retention_terminal"},
		{name: "idx_release_logs_retention_parent"},
		{name: "idx_releases_retention_terminal"},
		{name: "idx_hook_run_logs_retention_parent"},
		{name: "idx_hook_runs_retention_terminal"},
		{name: "idx_user_sessions_retention_expiry"},
		{name: "idx_user_remember_tokens_retention_expiry"},
	} {
		var definition string
		if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`, expected.name).Scan(&definition).Error; err != nil {
			t.Fatalf("read index %s: %v", expected.name, err)
		}
		if definition == "" {
			t.Fatalf("fresh database is missing index %s", expected.name)
		}
		for _, fragment := range expected.fragments {
			if !strings.Contains(definition, fragment) {
				t.Fatalf("index %s is missing %q: %q", expected.name, fragment, definition)
			}
		}
	}

	var defaultRuleCount int64
	if err := db.Table("billing_rate_rules").Count(&defaultRuleCount).Error; err != nil {
		t.Fatalf("count default billing rules: %v", err)
	}
	if defaultRuleCount == 0 {
		t.Fatal("fresh database did not seed default billing rules")
	}
	var transferRate struct {
		Enabled        bool
		CreditsPerUnit string
	}
	if err := db.Table("billing_rate_rules").Select("enabled, credits_per_unit::text AS credits_per_unit").Where("meter = ?", "storage.transfer_gib").Scan(&transferRate).Error; err != nil {
		t.Fatalf("read default volume transfer billing rule: %v", err)
	}
	if transferRate.Enabled || transferRate.CreditsPerUnit != "0.00000000" {
		t.Fatalf("default volume transfer billing rule = %#v", transferRate)
	}

}

func assertStableModelMigrationCoverage(t *testing.T, db *gorm.DB) {
	t.Helper()

	models := []any{
		&model.User{},
		&model.UserSession{},
		&model.UserRememberToken{},
		&model.OAuthApplication{},
		&model.OAuthGrant{},
		&model.OAuthAuthorizationCode{},
		&model.OAuthRefreshToken{},
		&model.OAuthDeviceAuthorization{},
		&model.AuthProvider{},
		&model.ExternalIdentity{},
		&model.AuthAdmissionPolicy{},
		&model.AuthRegistrationSettings{},
		&model.EmailRegistrationChallenge{},
		&model.Project{},
		&model.ProjectMember{},
		&model.ProjectPin{},
		&model.ProjectVolumeQuotaUsage{},
		&model.ProjectVolumeQuotaReservation{},
		&model.UserWallet{},
		&model.ProjectHookConfig{},
		&model.HookRun{},
		&model.HookRunLog{},
		&model.AccessToken{},
		&model.AuditLog{},
		&model.SecretValue{},
		&model.ScopedResourceProjectBinding{},
		&model.Application{},
		&model.RetainedVolume{},
		&model.ServiceBinding{},
		&model.ProjectTopologyEdge{},
		&model.AppTemplateInstallation{},
		&model.SystemComponentInstallation{},
		&model.GitProvider{},
		&model.GitAccount{},
		&model.RepositoryBinding{},
		&model.ArtifactRegistry{},
		&model.RegistryCredential{},
		&model.ContainerImage{},
		&model.DeploymentTargetHookBinding{},
		&model.BuildVariableSet{},
		&model.BuildEnvironmentConfig{},
		&model.BuildRun{},
		&model.BuildJob{},
		&model.BuildLog{},
		&model.BillingRateRule{},
		&model.BillingUsageRecord{},
		&model.BillingLedgerEntry{},
		&model.RuntimeCluster{},
		&model.Release{},
		&model.ReleaseLog{},
		&model.ProjectRuntimeConfigSet{},
		&model.DeploymentTarget{},
		&model.GatewayRoute{},
		&model.NotificationChannel{},
		&model.NotificationTemplate{},
		&model.NotificationRule{},
		&model.NotificationDelivery{},
		&model.PlatformEvent{},
		&model.InboxActionRequest{},
		&model.InboxMessage{},
		&model.AppConfig{},
	}

	for _, value := range models {
		parsed, err := schema.Parse(value, &sync.Map{}, db.NamingStrategy)
		if err != nil {
			t.Fatalf("parse model schema %T: %v", value, err)
		}
		if !db.Migrator().HasTable(value) {
			t.Errorf("versioned migrations are missing table %s for %T", parsed.Table, value)
			continue
		}
		for _, field := range parsed.Fields {
			if field.DBName == "" || field.IgnoreMigration {
				continue
			}
			if !db.Migrator().HasColumn(value, field.DBName) {
				t.Errorf("versioned migrations are missing column %s.%s for %T", parsed.Table, field.DBName, value)
			}
		}
	}
}

func assertDirtyMigrationFailsClosed(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.Exec(`UPDATE schema_migrations SET dirty = true`).Error; err != nil {
		t.Fatalf("mark migration state dirty: %v", err)
	}
	err := MigrateContext(context.Background(), db)
	var dirtyErr migrate.ErrDirty
	if !errors.As(err, &dirtyErr) {
		t.Fatalf("migration with dirty state error = %v, want migrate.ErrDirty", err)
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

func openTestMigrationRunner(t *testing.T, db *gorm.DB) *migrate.Migrate {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open migration test SQL database: %v", err)
	}
	sourceDriver, err := iofs.New(sqlmigrations.FS, ".")
	if err != nil {
		t.Fatalf("open embedded migrations: %v", err)
	}
	databaseDriver, err := migratepostgres.WithInstance(sqlDB, &migratepostgres.Config{})
	if err != nil {
		t.Fatalf("open migration test database driver: %v", err)
	}
	runner, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		t.Fatalf("create migration test runner: %v", err)
	}
	return runner
}

func assertRunnerMigrationVersion(t *testing.T, runner *migrate.Migrate, expected uint) {
	t.Helper()
	version, dirty, err := runner.Version()
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != expected || dirty {
		t.Fatalf("migration version = %d dirty=%t, want %d clean", version, dirty, expected)
	}
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
