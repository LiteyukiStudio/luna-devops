package database

import (
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/testdb"
	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"gorm.io/gorm"
)

func TestKubectlCompatibilityRemovalMigrationContract(t *testing.T) {
	for _, test := range []struct {
		name     string
		required []string
	}{
		{
			name: "000095_runtime_cluster_delete_lifecycle.up.sql",
			required: []string{
				"ADD COLUMN delete_status",
				"ADD COLUMN metadata jsonb",
				"DROP COLUMN namespace",
			},
		},
		{
			name: "000096_remove_kubectl_compatibility.up.sql",
			required: []string{
				"WHERE source = 'kubeconfig'",
				"DROP TABLE IF EXISTS kube_access_bindings",
				"DROP COLUMN IF EXISTS kube_gateway_enabled",
				"DROP COLUMN IF EXISTS management_source",
				"CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_observations_target_period",
				"part.value NOT LIKE 'kube:%'",
			},
		},
		{
			name:     "000096_remove_kubectl_compatibility.down.sql",
			required: []string{"Intentionally irreversible"},
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
			if test.name == "000095_runtime_cluster_delete_lifecycle.up.sql" {
				for _, retired := range []string{"kube_access_bindings", "kube_gateway_", "management_source", "resource_uid"} {
					if strings.Contains(string(content), retired) {
						t.Errorf("rewritten migration still creates retired schema %q", retired)
					}
				}
			}
		})
	}
}

func TestKubectlCompatibilityRemovalMigrationCleansLegacySchemaAndScopes(t *testing.T) {
	db := testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "kubectl_compatibility_removal_test"})
	runner := openTestMigrationRunner(t, db)
	if err := runner.Migrate(95); err != nil {
		t.Fatalf("migrate isolated database to version 95: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 95)
	assertRewrittenVersion95Schema(t, db)
	installLegacyKubectlCompatibilitySchema(t, db)
	if !db.Migrator().HasTable("kube_access_bindings") || !db.Migrator().HasColumn("runtime_clusters", "kube_gateway_enabled") {
		t.Fatal("legacy compatibility schema fixture was not installed")
	}

	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO users (id, email, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"usr_removed_kubectl", "removed-kubectl@example.test", "Removed Kubectl", "admin", now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_applications (id, owner_user_id, name, client_id, client_secret_hash, redirect_uris, allowed_scopes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"oapp_removed_kubectl", "usr_removed_kubectl", "Removed Kubectl App", "removed-kubectl-app", "secret-hash", `[]`, "user:read kube:read,volume:read", now, now).Error; err != nil {
		t.Fatalf("insert OAuth application: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_grants (id, application_id, user_id, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"ogrt_removed_kubectl", "oapp_removed_kubectl", "usr_removed_kubectl", "user:read,kube:write", now, now).Error; err != nil {
		t.Fatalf("insert OAuth grant: %v", err)
	}
	if err := db.Exec(`INSERT INTO access_tokens (id, user_id, name, scope, token_hash, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"tok_removed_personal", "usr_removed_kubectl", "Personal", "user:read,kube:read,volume:read", "personal-token-hash", "personal", now, now).Error; err != nil {
		t.Fatalf("insert personal access token: %v", err)
	}
	if err := db.Exec(`INSERT INTO access_tokens (id, user_id, name, scope, token_hash, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"tok_removed_kubeconfig", "usr_removed_kubectl", "Generated", "kube:read,kube:connect", "generated-token-hash", "kubeconfig", now, now).Error; err != nil {
		t.Fatalf("insert generated access token: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_refresh_tokens (id, application_id, grant_id, family_id, user_id, token_hash, scope, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ortk_removed_kubectl", "oapp_removed_kubectl", "ogrt_removed_kubectl", "family-removed-kubectl", "usr_removed_kubectl", "refresh-token-hash", "user:read,kube:connect", now.Add(time.Hour), now, now).Error; err != nil {
		t.Fatalf("insert OAuth refresh token: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_authorization_codes (id, application_id, grant_id, user_id, code_hash, redirect_uri, scope, code_challenge, code_challenge_method, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ocod_removed_kubectl", "oapp_removed_kubectl", "ogrt_removed_kubectl", "usr_removed_kubectl", "authorization-code-hash", "https://example.test/callback", "user:read,kube:read", strings.Repeat("a", 43), "S256", now.Add(time.Minute), now).Error; err != nil {
		t.Fatalf("insert OAuth authorization code: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_device_authorizations (id, application_id, grant_id, user_id, device_code_hash, user_code_hash, scope, status, interval_seconds, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"odev_removed_kubectl", "oapp_removed_kubectl", "ogrt_removed_kubectl", "usr_removed_kubectl", "device-code-hash", "user-code-hash", "user:read,kube:write", "approved", 5, now.Add(time.Minute), now, now).Error; err != nil {
		t.Fatalf("insert OAuth device authorization: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_applications (id, owner_user_id, name, client_id, client_secret_hash, redirect_uris, allowed_scopes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"oapp_kube_only", "usr_removed_kubectl", "Retired Only App", "retired-only-app", "secret-hash", `[]`, "kube:read,kube:write", now, now).Error; err != nil {
		t.Fatalf("insert kube-only OAuth application: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_grants (id, application_id, user_id, scope, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"ogrt_kube_only", "oapp_kube_only", "usr_removed_kubectl", "kube:read", now, now).Error; err != nil {
		t.Fatalf("insert kube-only OAuth grant: %v", err)
	}
	if err := db.Exec(`INSERT INTO access_tokens (id, user_id, name, scope, token_hash, source, oauth_application_id, oauth_grant_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"tok_kube_only", "usr_removed_kubectl", "Retired Only", "kube:read", "kube-only-token-hash", "oauth", "oapp_kube_only", "ogrt_kube_only", now, now).Error; err != nil {
		t.Fatalf("insert kube-only access token: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_refresh_tokens (id, application_id, grant_id, family_id, user_id, token_hash, scope, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ortk_kube_only", "oapp_kube_only", "ogrt_kube_only", "family-kube-only", "usr_removed_kubectl", "kube-only-refresh-hash", "kube:read", now.Add(time.Hour), now, now).Error; err != nil {
		t.Fatalf("insert kube-only OAuth refresh token: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_authorization_codes (id, application_id, grant_id, user_id, code_hash, redirect_uri, scope, code_challenge, code_challenge_method, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ocod_kube_only", "oapp_kube_only", "ogrt_kube_only", "usr_removed_kubectl", "kube-only-code-hash", "https://example.test/callback", "kube:read", strings.Repeat("b", 43), "S256", now.Add(time.Minute), now).Error; err != nil {
		t.Fatalf("insert kube-only OAuth authorization code: %v", err)
	}
	if err := db.Exec(`INSERT INTO oauth_device_authorizations (id, application_id, grant_id, user_id, device_code_hash, user_code_hash, scope, status, interval_seconds, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"odev_kube_only", "oapp_kube_only", "ogrt_kube_only", "usr_removed_kubectl", "kube-only-device-hash", "kube-only-user-code-hash", "kube:write", "pending", 5, now.Add(time.Minute), now, now).Error; err != nil {
		t.Fatalf("insert kube-only OAuth device authorization: %v", err)
	}

	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply kubectl compatibility removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 96)
	assertKubectlCompatibilityRemoved(t, db)

	if err := runner.Steps(-1); err != nil {
		t.Fatalf("step over irreversible kubectl compatibility removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 95)
	assertKubectlCompatibilityRemoved(t, db)
	if err := runner.Steps(1); err != nil {
		t.Fatalf("reapply kubectl compatibility removal migration: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 96)
	assertKubectlCompatibilityRemoved(t, db)

	if err := runner.Steps(-2); err != nil {
		t.Fatalf("roll back removal and rewritten version 95 migrations: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, 94)
	assertVersion94AfterRetiredCompatibilityRollback(t, db)
}

func assertRewrittenVersion95Schema(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db.Migrator().HasTable("kube_access_bindings") {
		t.Fatal("rewritten version 95 created retired kube access bindings")
	}
	for _, column := range []string{"kube_gateway_enabled", "kube_gateway_extra_resource_rules", "kube_gateway_drain_until", "kube_gateway_cleanup_completed_at"} {
		if db.Migrator().HasColumn("runtime_clusters", column) {
			t.Fatalf("rewritten version 95 created retired runtime_clusters.%s", column)
		}
	}
	for _, column := range []string{"management_source", "resource_kind", "resource_uid", "application_id"} {
		if db.Migrator().HasColumn("runtime_observations", column) {
			t.Fatalf("rewritten version 95 created retired runtime_observations.%s", column)
		}
	}
	if !db.Migrator().HasColumn("runtime_clusters", "delete_status") || !db.Migrator().HasColumn("audit_logs", "metadata") {
		t.Fatal("rewritten version 95 omitted shared runtime deletion or audit fields")
	}
	if db.Migrator().HasColumn("deployment_targets", "namespace") {
		t.Fatal("rewritten version 95 retained obsolete deployment_targets.namespace")
	}
}

func installLegacyKubectlCompatibilitySchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE kube_access_bindings (
    id text PRIMARY KEY,
    access_token_id text NOT NULL REFERENCES access_tokens(id) ON DELETE CASCADE,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    runtime_cluster_id text NOT NULL REFERENCES runtime_clusters(id) ON DELETE CASCADE,
    application_id text REFERENCES applications(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
)`,
		`CREATE INDEX idx_kube_access_bindings_access_token_id ON kube_access_bindings(access_token_id)`,
		`CREATE INDEX idx_kube_access_bindings_project_id ON kube_access_bindings(project_id)`,
		`CREATE INDEX idx_kube_access_bindings_runtime_cluster_id ON kube_access_bindings(runtime_cluster_id)`,
		`CREATE INDEX idx_kube_access_bindings_application_id ON kube_access_bindings(application_id)`,
		`CREATE UNIQUE INDEX idx_kube_access_bindings_context
    ON kube_access_bindings(access_token_id, project_id, runtime_cluster_id, COALESCE(application_id, ''))`,
		`ALTER TABLE runtime_clusters
    ADD COLUMN kube_gateway_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN kube_gateway_extra_resource_rules jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN kube_gateway_drain_until timestamptz,
    ADD COLUMN kube_gateway_cleanup_completed_at timestamptz`,
		`ALTER TABLE runtime_observations
    DROP CONSTRAINT runtime_observations_target_period_unique,
    ALTER COLUMN deployment_target_id DROP NOT NULL,
    ADD COLUMN management_source text NOT NULL DEFAULT 'platform',
    ADD COLUMN resource_kind text NOT NULL DEFAULT 'Deployment',
    ADD COLUMN resource_uid text NOT NULL DEFAULT '',
    ADD COLUMN application_id text REFERENCES applications(id) ON DELETE SET NULL,
    ADD CONSTRAINT runtime_observations_management_source_check
        CHECK (management_source IN ('platform', 'kubectl'))`,
		`CREATE UNIQUE INDEX idx_runtime_observations_resource_period
    ON runtime_observations(runtime_cluster_id, project_id, resource_uid, period_start)`,
		`CREATE INDEX idx_runtime_observations_management_source ON runtime_observations(management_source)`,
		`CREATE INDEX idx_runtime_observations_resource_kind ON runtime_observations(resource_kind)`,
		`CREATE INDEX idx_runtime_observations_resource_uid ON runtime_observations(resource_uid)`,
		`CREATE INDEX idx_runtime_observations_application_id ON runtime_observations(application_id)`,
		`INSERT INTO runtime_observations (
    id, deployment_target_id, period_start, period_end,
    desired_replicas, updated_replicas, ready_replicas, available_replicas,
    effective_cpu_request, effective_memory_request, workload_created_at,
    status, observation_code, observed_at, management_source, resource_kind, resource_uid
) VALUES (
    'obs_retired_kubectl', NULL, now() - interval '1 minute', now(),
    0, 0, 0, 0, '', '', now() - interval '1 minute',
    'unknown', 'legacy', now(), 'kubectl', 'Deployment', 'legacy-workload'
)`,
		`INSERT INTO deployment_targets (
    id, project_id, application_id, name, stage, cluster_id
) VALUES (
    'dplt_observation_duplicate', 'prj_observation_duplicate',
    'app_observation_duplicate', 'Observation duplicate', 'prod', 'clu_old'
)`,
		`INSERT INTO runtime_observations (
    id, deployment_target_id, runtime_cluster_id, project_id, period_start, period_end,
    desired_replicas, updated_replicas, ready_replicas, available_replicas,
    effective_cpu_request, effective_memory_request, workload_created_at,
    status, observation_code, observed_at, created_at, updated_at,
    management_source, resource_kind, resource_uid
) VALUES
(
    'obs_duplicate_older', 'dplt_observation_duplicate', 'clu_old', 'prj_observation_duplicate',
    '2026-09-02 12:00:00+00', '2026-09-02 12:01:00+00',
    1, 1, 1, 1, '100m', '128Mi', '2026-09-02 11:00:00+00',
    'ready', 'ready', '2026-09-02 12:00:10+00',
    '2026-09-02 12:00:10+00', '2026-09-02 12:00:10+00',
    'platform', 'Deployment', 'resource-before-cluster-move'
),
(
    'obs_duplicate_newest', 'dplt_observation_duplicate', 'clu_new', 'prj_observation_duplicate',
    '2026-09-02 12:00:00+00', '2026-09-02 12:01:00+00',
    1, 1, 1, 1, '100m', '128Mi', '2026-09-02 11:00:00+00',
    'ready', 'ready', '2026-09-02 12:00:20+00',
    '2026-09-02 12:00:20+00', '2026-09-02 12:00:20+00',
    'platform', 'Deployment', 'resource-after-cluster-move'
)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("install legacy compatibility schema with %q: %v", statement, err)
		}
	}
}

func assertKubectlCompatibilityRemoved(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db.Migrator().HasTable("kube_access_bindings") {
		t.Fatal("retired kube access binding table remains")
	}
	for _, column := range []string{
		"kube_gateway_enabled",
		"kube_gateway_extra_resource_rules",
		"kube_gateway_drain_until",
		"kube_gateway_cleanup_completed_at",
	} {
		if db.Migrator().HasColumn("runtime_clusters", column) {
			t.Fatalf("retired runtime_clusters.%s remains", column)
		}
	}
	for _, column := range []string{"management_source", "resource_kind", "resource_uid", "application_id"} {
		if db.Migrator().HasColumn("runtime_observations", column) {
			t.Fatalf("retired runtime_observations.%s remains", column)
		}
	}
	if !db.Migrator().HasColumn("runtime_clusters", "delete_status") || !db.Migrator().HasColumn("audit_logs", "metadata") {
		t.Fatal("removal discarded shared runtime deletion or audit fields")
	}

	var nullable string
	if err := db.Raw(`SELECT is_nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'runtime_observations' AND column_name = 'deployment_target_id'`).Scan(&nullable).Error; err != nil {
		t.Fatalf("inspect runtime observation target nullability: %v", err)
	}
	if nullable != "NO" {
		t.Fatalf("runtime_observations.deployment_target_id is_nullable = %q, want NO", nullable)
	}
	var uniqueIndexCount int64
	if err := db.Raw(`SELECT count(*)
FROM pg_indexes
WHERE schemaname = current_schema()
  AND tablename = 'runtime_observations'
  AND indexdef LIKE '%UNIQUE INDEX%'
  AND indexdef LIKE '%(deployment_target_id, period_start)%'`).Scan(&uniqueIndexCount).Error; err != nil {
		t.Fatalf("inspect runtime observation unique index: %v", err)
	}
	if uniqueIndexCount != 1 {
		t.Fatalf("runtime observation target-period unique index count = %d, want 1", uniqueIndexCount)
	}
	var retainedObservationIDs []string
	if err := db.Table("runtime_observations").Where("deployment_target_id = ?", "dplt_observation_duplicate").Order("id").Pluck("id", &retainedObservationIDs).Error; err != nil {
		t.Fatalf("read deduplicated runtime observations: %v", err)
	}
	if len(retainedObservationIDs) != 1 || retainedObservationIDs[0] != "obs_duplicate_newest" {
		t.Fatalf("deduplicated runtime observations = %v, want [obs_duplicate_newest]", retainedObservationIDs)
	}

	var generatedTokenCount int64
	if err := db.Table("access_tokens").Where("source = ?", "kubeconfig").Count(&generatedTokenCount).Error; err != nil {
		t.Fatalf("count retired generated access tokens: %v", err)
	}
	if generatedTokenCount != 0 {
		t.Fatalf("retired generated access token count = %d, want 0", generatedTokenCount)
	}
	assertMigrationStringValue(t, db, "access_tokens", "id", "tok_removed_personal", "scope", "user:read,volume:read")
	assertMigrationStringValue(t, db, "oauth_applications", "id", "oapp_removed_kubectl", "allowed_scopes", "user:read,volume:read")
	assertMigrationStringValue(t, db, "oauth_grants", "id", "ogrt_removed_kubectl", "scope", "user:read")
	assertMigrationStringValue(t, db, "oauth_refresh_tokens", "id", "ortk_removed_kubectl", "scope", "user:read")
	assertMigrationStringValue(t, db, "oauth_authorization_codes", "id", "ocod_removed_kubectl", "scope", "user:read")
	assertMigrationStringValue(t, db, "oauth_device_authorizations", "id", "odev_removed_kubectl", "scope", "user:read")
	for _, retired := range []struct {
		table string
		id    string
	}{
		{table: "access_tokens", id: "tok_kube_only"},
		{table: "oauth_grants", id: "ogrt_kube_only"},
		{table: "oauth_refresh_tokens", id: "ortk_kube_only"},
		{table: "oauth_authorization_codes", id: "ocod_kube_only"},
		{table: "oauth_device_authorizations", id: "odev_kube_only"},
	} {
		var count int64
		if err := db.Table(retired.table).Where("id = ?", retired.id).Count(&count).Error; err != nil {
			t.Fatalf("count retired %s row: %v", retired.table, err)
		}
		if count != 0 {
			t.Fatalf("retired %s row %s remains", retired.table, retired.id)
		}
	}
	var revokedAt *time.Time
	if err := db.Table("oauth_applications").Select("revoked_at").Where("id = ?", "oapp_kube_only").Scan(&revokedAt).Error; err != nil {
		t.Fatalf("read kube-only OAuth application revocation: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("kube-only OAuth application was not revoked")
	}
}

func assertVersion94AfterRetiredCompatibilityRollback(t *testing.T, db *gorm.DB) {
	t.Helper()
	if !db.Migrator().HasColumn("deployment_targets", "namespace") {
		t.Fatal("version 94 rollback did not restore deployment_targets.namespace")
	}
	for _, field := range []struct {
		table  string
		column string
	}{
		{table: "runtime_clusters", column: "delete_status"},
		{table: "audit_logs", column: "metadata"},
	} {
		if db.Migrator().HasColumn(field.table, field.column) {
			t.Fatalf("version 94 rollback retained %s.%s", field.table, field.column)
		}
	}
	if db.Migrator().HasTable("kube_access_bindings") {
		t.Fatal("rollback reintroduced retired kube access bindings")
	}
	for _, column := range []string{"kube_gateway_enabled", "kube_gateway_extra_resource_rules", "kube_gateway_drain_until", "kube_gateway_cleanup_completed_at"} {
		if db.Migrator().HasColumn("runtime_clusters", column) {
			t.Fatalf("rollback reintroduced retired runtime_clusters.%s", column)
		}
	}
	var constraintCount int64
	if err := db.Raw(`SELECT count(*)
FROM pg_constraint
WHERE connamespace = current_schema()::regnamespace
  AND conrelid = 'runtime_observations'::regclass
  AND conname = 'runtime_observations_target_period_unique'`).Scan(&constraintCount).Error; err != nil {
		t.Fatalf("inspect restored runtime observation constraint: %v", err)
	}
	if constraintCount != 1 {
		t.Fatalf("version 94 target-period constraint count = %d, want 1", constraintCount)
	}
}

func assertMigrationStringValue(t *testing.T, db *gorm.DB, table, keyColumn, keyValue, valueColumn, want string) {
	t.Helper()
	var row map[string]any
	if err := db.Table(table).Select(valueColumn).Where(keyColumn+" = ?", keyValue).Take(&row).Error; err != nil {
		t.Fatalf("read %s.%s for %s: %v", table, valueColumn, keyValue, err)
	}
	if got, _ := row[valueColumn].(string); got != want {
		t.Fatalf("%s.%s for %s = %q, want %q", table, valueColumn, keyValue, got, want)
	}
}
