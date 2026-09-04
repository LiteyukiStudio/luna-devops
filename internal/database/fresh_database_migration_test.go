package database

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
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

const baselineMigrationVersion = 103

func TestEmbeddedMigrationsContainSingleBaseline(t *testing.T) {
	entries, err := sqlmigrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	baselineFiles := map[string]bool{
		"000103_baseline.up.sql":   false,
		"000103_baseline.down.sql": false,
	}
	migrationCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".up.sql") && !strings.HasSuffix(name, ".down.sql")) {
			continue
		}
		migrationCount++
		if _, ok := baselineFiles[name]; !ok {
			t.Fatalf("unexpected embedded migration %s", name)
		}
		baselineFiles[name] = true
	}
	if migrationCount != len(baselineFiles) {
		t.Fatalf("embedded migration file count = %d, want %d", migrationCount, len(baselineFiles))
	}
	for name, found := range baselineFiles {
		if !found {
			t.Fatalf("baseline is missing %s", name)
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
	if err := runner.Migrate(baselineMigrationVersion); err != nil {
		t.Fatalf("migrate database to baseline: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, baselineMigrationVersion)
	if err := testDB.Exec(`CREATE TABLE public.migration_down_sentinel (id text PRIMARY KEY)`).Error; err != nil {
		t.Fatalf("create rollback sentinel table: %v", err)
	}
	if err := testDB.Exec(`CREATE VIEW public.migration_down_external_view AS SELECT id FROM public.users`).Error; err != nil {
		t.Fatalf("create external view that depends on baseline: %v", err)
	}
	if err := runner.Steps(-1); err == nil {
		t.Fatal("baseline rollback unexpectedly deleted an external dependent view")
	}
	var dependentViewPreserved bool
	if err := testDB.Raw(`SELECT to_regclass('public.migration_down_external_view') IS NOT NULL`).Scan(&dependentViewPreserved).Error; err != nil {
		t.Fatalf("inspect external dependent view after rejected rollback: %v", err)
	}
	if !dependentViewPreserved || !testDB.Migrator().HasTable("users") {
		t.Fatal("rejected baseline rollback did not preserve external and baseline objects atomically")
	}
	var rejectedRollbackState struct {
		Version int
		Dirty   bool
	}
	if err := testDB.Raw(`SELECT version, dirty FROM schema_migrations`).Scan(&rejectedRollbackState).Error; err != nil {
		t.Fatalf("read rejected rollback migration state: %v", err)
	}
	if rejectedRollbackState.Version != -1 || !rejectedRollbackState.Dirty {
		t.Fatalf("rejected rollback migration state = %+v, want version -1 dirty", rejectedRollbackState)
	}
	if err := testDB.Exec(`DROP VIEW public.migration_down_external_view`).Error; err != nil {
		t.Fatalf("remove external dependent view: %v", err)
	}
	if err := runner.Force(baselineMigrationVersion); err != nil {
		t.Fatalf("restore clean baseline version after expected rollback rejection: %v", err)
	}
	if err := runner.Steps(-1); err != nil {
		t.Fatalf("roll back baseline: %v", err)
	}
	var remainingPublicTables int64
	if err := testDB.Raw(`SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename NOT IN ('schema_migrations', 'migration_down_sentinel')`).Scan(&remainingPublicTables).Error; err != nil {
		t.Fatalf("inspect public tables after baseline rollback: %v", err)
	}
	if remainingPublicTables != 0 {
		t.Fatalf("baseline rollback retained %d owned public tables", remainingPublicTables)
	}
	if !testDB.Migrator().HasTable("migration_down_sentinel") {
		t.Fatal("baseline rollback removed an unrelated external table")
	}
	var hasAISchema bool
	if err := testDB.Raw(`SELECT to_regnamespace('ai') IS NOT NULL`).Scan(&hasAISchema).Error; err != nil {
		t.Fatalf("inspect AI schema after baseline rollback: %v", err)
	}
	if hasAISchema {
		t.Fatal("baseline rollback retained the owned AI schema")
	}
	var remainingFunctions int64
	if err := testDB.Raw(`SELECT count(*)
FROM pg_proc AS procedure
JOIN pg_namespace AS namespace ON namespace.oid = procedure.pronamespace
WHERE namespace.nspname = current_schema()
  AND procedure.proname LIKE 'luna_%'`).Scan(&remainingFunctions).Error; err != nil {
		t.Fatalf("inspect functions after baseline rollback: %v", err)
	}
	if remainingFunctions != 0 {
		t.Fatalf("baseline rollback retained %d Luna functions", remainingFunctions)
	}
	if err := runner.Migrate(baselineMigrationVersion); err != nil {
		t.Fatalf("reapply baseline: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, baselineMigrationVersion)
	if err := MigrateContext(context.Background(), testDB); err != nil {
		t.Fatalf("repeat migration after baseline bootstrap: %v", err)
	}

	assertFreshMigrationState(t, testDB)
	assertStableModelMigrationCoverage(t, testDB)
	assertActiveDeploymentStageUniqueness(t, testDB)
	assertDirtyMigrationFailsClosed(t, testDB)
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
	if migrationState.Version != baselineMigrationVersion {
		t.Fatalf("migration version = %d, want %d", migrationState.Version, baselineMigrationVersion)
	}
	assertSlimmingMigrationRemovals(t, db)
	assertBaselineConstraintsAndDefaults(t, db)
	assertBaselineSeedData(t, db)

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
		"platform_mail_settings",
		"email_registration_challenges",
		"user_notification_preferences",
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
	for _, table := range []string{"ai.ui_actions", "ai.tool_approval_exemptions", "kube_access_bindings"} {
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
		{table: "access_tokens", column: "oauth_family_id"},
		{table: "access_tokens", column: "oauth_grant_id"},
		{table: "oauth_refresh_tokens", column: "family_id"},
		{table: "runtime_clusters", column: "delete_status"},
		{table: "audit_logs", column: "metadata"},
		{table: "auth_registration_settings", column: "allow_oidc_registration"},
		{table: "platform_mail_settings", column: "personal_email_cooldown_seconds"},
		{table: "notification_channels", column: "owner_user_id"},
		{table: "notification_deliveries", column: "recipient_user_id"},
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
		{table: "auth_registration_settings", column: "smtp_host"},
		{table: "auth_registration_settings", column: "smtp_password_ref"},
		{table: "applications", column: "source_type"},
		{table: "applications", column: "repository_url"},
		{table: "applications", column: "image_reference"},
		{table: "applications", column: "git_account_id"},
		{table: "applications", column: "service_port"},
		{table: "applications", column: "data_retention_mode"},
		{table: "deployment_targets", column: "build_config_id"},
		{table: "deployment_targets", column: "service_port"},
		{table: "deployment_targets", column: "runtime_config_set_ids"},
		{table: "deployment_targets", column: "config_refs"},
		{table: "deployment_targets", column: "namespace"},
		{table: "runtime_clusters", column: "kube_gateway_enabled"},
		{table: "runtime_clusters", column: "kube_gateway_extra_resource_rules"},
		{table: "runtime_clusters", column: "kube_gateway_drain_until"},
		{table: "runtime_clusters", column: "kube_gateway_cleanup_completed_at"},
		{table: "runtime_observations", column: "management_source"},
		{table: "runtime_observations", column: "resource_kind"},
		{table: "runtime_observations", column: "resource_uid"},
		{table: "runtime_observations", column: "application_id"},
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
		{name: "idx_runtime_observations_target_period", fragments: []string{"UNIQUE INDEX"}},
		{name: "idx_runtime_observations_cluster_period"},
		{name: "idx_runtime_observations_project_period"},
		{name: "idx_access_tokens_oauth_family_id", fragments: []string{"WHERE (oauth_family_id <> ''::text)"}},
		{name: "idx_oauth_refresh_tokens_family_id"},
		{name: "idx_notification_channels_owner_user_id"},
		{name: "idx_notification_deliveries_event_channel_recipient", fragments: []string{"UNIQUE INDEX"}},
		{name: "idx_notification_deliveries_recipient_user_id"},
		{name: "idx_platform_events_notification_fanout_status"},
		{name: "idx_platform_events_resource_owner_user_id"},
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
}

func assertBaselineConstraintsAndDefaults(t *testing.T, db *gorm.DB) {
	t.Helper()

	constraintNames := []string{
		"chk_platform_events_notification_fanout_status",
		"platform_mail_settings_personal_email_cooldown_seconds_range",
		"runtime_clusters_cpu_request_percent_range",
		"runtime_clusters_memory_request_percent_range",
		"runtime_clusters_cpu_limit_percent_range",
		"runtime_clusters_memory_limit_percent_range",
		"runtime_clusters_cpu_policy_order",
		"runtime_clusters_memory_policy_order",
		"runtime_observations_policy_range",
		"runtime_observations_usage_nonnegative",
	}
	var constraintCount int64
	if err := db.Raw(`SELECT count(*)
FROM pg_constraint
WHERE connamespace = current_schema()::regnamespace
  AND conname IN ?`, constraintNames).Scan(&constraintCount).Error; err != nil {
		t.Fatalf("inspect baseline constraints: %v", err)
	}
	if constraintCount != int64(len(constraintNames)) {
		t.Fatalf("baseline constraint count = %d, want %d", constraintCount, len(constraintNames))
	}

	if err := db.Exec(`INSERT INTO runtime_clusters (id, name) VALUES ('rcl_baseline_defaults', 'Baseline Defaults')`).Error; err != nil {
		t.Fatalf("insert runtime cluster with baseline defaults: %v", err)
	}
	var policy struct {
		CPURequestPercent    int
		MemoryRequestPercent int
		CPULimitPercent      int
		MemoryLimitPercent   int
	}
	if err := db.Table("runtime_clusters").Where("id = ?", "rcl_baseline_defaults").Take(&policy).Error; err != nil {
		t.Fatalf("read runtime cluster baseline defaults: %v", err)
	}
	if policy.CPURequestPercent != 10 || policy.MemoryRequestPercent != 25 || policy.CPULimitPercent != 100 || policy.MemoryLimitPercent != 100 {
		t.Fatalf("runtime cluster policy defaults = %#v", policy)
	}
	if err := db.Table("runtime_clusters").Where("id = ?", "rcl_baseline_defaults").Updates(map[string]any{
		"cpu_request_percent": 50,
		"cpu_limit_percent":   40,
	}).Error; err == nil {
		t.Fatal("database accepted a CPU request above its limit")
	}

	if err := db.Exec(`INSERT INTO platform_mail_settings (id) VALUES ('baseline_defaults')`).Error; err != nil {
		t.Fatalf("insert platform mail settings with baseline defaults: %v", err)
	}
	var cooldown int
	if err := db.Table("platform_mail_settings").Select("personal_email_cooldown_seconds").Where("id = ?", "baseline_defaults").Scan(&cooldown).Error; err != nil {
		t.Fatalf("read personal email cooldown default: %v", err)
	}
	if cooldown != 60 {
		t.Fatalf("personal email cooldown default = %d, want 60", cooldown)
	}
	if err := db.Table("platform_mail_settings").Where("id = ?", "baseline_defaults").Update("personal_email_cooldown_seconds", 3601).Error; err == nil {
		t.Fatal("database accepted an out-of-range personal email cooldown")
	}

	if err := db.Exec(`INSERT INTO platform_events (id, type, category, severity, status, occurred_at) VALUES ('evt_baseline', 'build.failed', 'build', 'error', 'failed', now())`).Error; err != nil {
		t.Fatalf("insert baseline platform event: %v", err)
	}
	if err := db.Table("platform_events").Where("id = ?", "evt_baseline").Update("notification_fanout_status", "invalid").Error; err == nil {
		t.Fatal("database accepted an invalid notification fanout status")
	}

	insertDelivery := func(id, recipient string) error {
		return db.Exec(`INSERT INTO notification_deliveries (id, event_id, event_type, channel_id, adapter_kind, recipient_user_id, queued_at) VALUES (?, 'evt_baseline', 'build.failed', 'notification:user-email', 'smtp', ?, now())`, id, recipient).Error
	}
	if err := insertDelivery("ndl_baseline_first", "usr_first"); err != nil {
		t.Fatalf("insert first baseline notification delivery: %v", err)
	}
	if err := insertDelivery("ndl_baseline_second", "usr_second"); err != nil {
		t.Fatalf("insert delivery for another recipient: %v", err)
	}
	if err := insertDelivery("ndl_baseline_duplicate", "usr_first"); err == nil {
		t.Fatal("database accepted a duplicate event, channel, and recipient delivery")
	}
}

func assertBaselineSeedData(t *testing.T, db *gorm.DB) {
	t.Helper()

	var oauthApplicationCount int64
	if err := db.Table("oauth_applications").Count(&oauthApplicationCount).Error; err != nil {
		t.Fatalf("count built-in OAuth applications: %v", err)
	}
	if oauthApplicationCount != 1 {
		t.Fatalf("built-in OAuth application count = %d, want 1", oauthApplicationCount)
	}
	var cliApplication struct {
		ClientID                string
		AllowedScopes           string
		AccessTokenLifetimeDays int
	}
	if err := db.Table("oauth_applications").Where("id = ?", "oapp_luna_cli").Take(&cliApplication).Error; err != nil {
		t.Fatalf("read built-in Luna CLI OAuth application: %v", err)
	}
	if cliApplication.ClientID != "luna-cli" || cliApplication.AllowedScopes != "*" || cliApplication.AccessTokenLifetimeDays != 1 {
		t.Fatalf("built-in Luna CLI OAuth application = %#v", cliApplication)
	}

	var appConfigCount int64
	if err := db.Table("app_configs").Count(&appConfigCount).Error; err != nil {
		t.Fatalf("count baseline app configs: %v", err)
	}
	if appConfigCount != 1 {
		t.Fatalf("baseline app config count = %d, want 1", appConfigCount)
	}
	var aiAccessMode string
	if err := db.Table("app_configs").Select("value").Where("key = ?", "ai.access.mode").Scan(&aiAccessMode).Error; err != nil {
		t.Fatalf("read default AI access mode: %v", err)
	}
	if aiAccessMode != "all_authenticated" {
		t.Fatalf("default AI access mode = %q, want all_authenticated", aiAccessMode)
	}

	type billingRateSeed struct {
		Meter          string
		Unit           string
		CreditsPerUnit string
		Enabled        bool
		Description    string
	}
	var rates []billingRateSeed
	if err := db.Table("billing_rate_rules").Select("meter, unit, credits_per_unit::text AS credits_per_unit, enabled, description").Order("meter ASC").Scan(&rates).Error; err != nil {
		t.Fatalf("read default billing rates: %v", err)
	}
	wantRates := []billingRateSeed{
		{Meter: "build.cpu_vcpu_minute", Unit: "vcpu_minute", CreditsPerUnit: "10.00000000", Enabled: true, Description: "Build CPU usage"},
		{Meter: "build.memory_gib_minute", Unit: "gib_minute", CreditsPerUnit: "2.00000000", Enabled: true, Description: "Build memory usage"},
		{Meter: "gateway.egress_gib", Unit: "gib", CreditsPerUnit: "1.00000000", Enabled: true, Description: "Gateway response egress traffic"},
		{Meter: "gateway.requests_1000", Unit: "1000_requests", CreditsPerUnit: "0.00000000", Enabled: false, Description: "Gateway request count"},
		{Meter: "runtime.cpu_vcpu_hour", Unit: "vcpu_hour", CreditsPerUnit: "30.00000000", Enabled: true, Description: "Runtime CPU usage"},
		{Meter: "runtime.memory_gib_hour", Unit: "gib_hour", CreditsPerUnit: "6.00000000", Enabled: true, Description: "Runtime memory usage"},
		{Meter: "storage.gib_day", Unit: "gib_day", CreditsPerUnit: "1.00000000", Enabled: true, Description: "Persistent storage usage"},
		{Meter: "storage.transfer_gib", Unit: "gib", CreditsPerUnit: "0.00000000", Enabled: false, Description: "Volume transfer bytes"},
	}
	if len(rates) != len(wantRates) {
		t.Fatalf("default billing rate count = %d, want %d", len(rates), len(wantRates))
	}
	for index := range wantRates {
		if rates[index] != wantRates[index] {
			t.Fatalf("default billing rate %d = %#v, want %#v", index, rates[index], wantRates[index])
		}
	}
}

func assertSlimmingMigrationRemovals(t *testing.T, db *gorm.DB) {
	t.Helper()

	for _, obsolete := range []struct {
		table  string
		column string
	}{
		{table: "projects", column: "namespace_strategy"},
		{table: "runtime_clusters", column: "type"},
		{table: "runtime_clusters", column: "gateway_provider"},
		{table: "runtime_clusters", column: "gateway_root_domain"},
		{table: "deployment_targets", column: "environment_id"},
		{table: "gateway_routes", column: "environment_id"},
		{table: "hook_runs", column: "environment_id"},
		{table: "releases", column: "environment_id"},
		{table: "deployment_targets", column: "build_labels"},
		{table: "build_runs", column: "build_labels"},
		{table: "build_runs", column: "cache_config"},
		{table: "build_runs", column: "cpu_core_seconds"},
		{table: "build_runs", column: "memory_mb_seconds"},
		{table: "build_runs", column: "credit_cost"},
		{table: "build_jobs", column: "type"},
		{table: "build_jobs", column: "builder_id"},
		{table: "build_jobs", column: "lease_token"},
		{table: "build_jobs", column: "lease_until"},
		{table: "build_jobs", column: "last_heartbeat_at"},
	} {
		if db.Migrator().HasColumn(obsolete.table, obsolete.column) {
			t.Fatalf("fresh database contains slimming-migration column %s.%s", obsolete.table, obsolete.column)
		}
	}

	for _, obsolete := range []struct {
		table string
		name  string
	}{
		{table: "deployment_targets", name: "idx_deployment_targets_app_env_name_active"},
		{table: "deployment_targets", name: "idx_deployment_targets_environment_id"},
		{table: "gateway_routes", name: "idx_gateway_routes_environment_id"},
		{table: "hook_runs", name: "idx_hook_runs_environment_id"},
		{table: "releases", name: "idx_releases_environment_id"},
		{table: "build_jobs", name: "idx_build_jobs_builder_id"},
		{table: "build_jobs", name: "idx_build_jobs_lease_token"},
		{table: "build_jobs", name: "idx_build_jobs_lease_until"},
		{table: "build_jobs", name: "idx_build_jobs_last_heartbeat_at"},
	} {
		if db.Migrator().HasIndex(obsolete.table, obsolete.name) {
			t.Fatalf("fresh database contains slimming-migration index %s", obsolete.name)
		}
	}

	obsoleteConfigKeys := []string{
		"gateway.rootDomain",
		"gateway.publicScheme",
		"billing.freeQuotaCredits",
		"billing.overdueGracePeriodHours",
		"billing.allowNegativeBalance",
		"ai.runtime.context_input_k_tokens",
		"ai.context.summary_input_k_tokens",
	}
	var obsoleteConfigCount int64
	if err := db.Table("app_configs").Where("key IN ?", obsoleteConfigKeys).Count(&obsoleteConfigCount).Error; err != nil {
		t.Fatalf("count obsolete app configs: %v", err)
	}
	if obsoleteConfigCount != 0 {
		t.Fatalf("fresh database contains %d obsolete app configs", obsoleteConfigCount)
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
		&model.PlatformMailSettings{},
		&model.EmailRegistrationChallenge{},
		&model.UserNotificationPreference{},
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
		&model.RuntimeObservation{},
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
	if err := db.Exec(`INSERT INTO projects (id, identifier, kubernetes_namespace, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"prj_stage_test", "stage-test", "luna-stage-test", "Stage Test", now, now).Error; err != nil {
		t.Fatalf("insert stage test project: %v", err)
	}
	if err := db.Exec(`INSERT INTO applications (id, project_id, identifier, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"app_stage_test", "prj_stage_test", "api", "API", now, now).Error; err != nil {
		t.Fatalf("insert stage test application: %v", err)
	}
	insertTarget := func(id string) error {
		return db.Exec(`INSERT INTO deployment_targets (id, project_id, application_id, name, stage, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, "prj_stage_test", "app_stage_test", id, "prod", now, now).Error
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
