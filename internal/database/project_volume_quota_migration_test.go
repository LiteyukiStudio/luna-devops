package database

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestProjectVolumeQuotaMigrationUpgradesVersion67(t *testing.T) {
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(gormpostgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if adminSQL, dbErr := adminDB.DB(); dbErr == nil {
		t.Cleanup(func() { _ = adminSQL.Close() })
	}

	databaseName := fmt.Sprintf("luna_volume_quota_upgrade_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(databaseName, "luna_volume_quota_upgrade_test_") {
		t.Fatalf("refuse unsafe migration test database name %q", databaseName)
	}
	if err := adminDB.Exec(`CREATE DATABASE "` + databaseName + `"`).Error; err != nil {
		t.Fatalf("create isolated migration database: %v", err)
	}
	t.Cleanup(func() {
		if dropErr := adminDB.Exec(`DROP DATABASE IF EXISTS "` + databaseName + `" WITH (FORCE)`).Error; dropErr != nil {
			t.Errorf("drop isolated migration database: %v", dropErr)
		}
	})

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	parsedURL.Path = "/" + databaseName
	parsedURL.RawPath = ""
	query := parsedURL.Query()
	query.Del("search_path")
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(gormpostgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated migration database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open isolated migration SQL database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	sourceDriver, err := iofs.New(sqlmigrations.FS, ".")
	if err != nil {
		t.Fatalf("open embedded migrations: %v", err)
	}
	databaseDriver, err := migratepostgres.WithInstance(sqlDB, &migratepostgres.Config{})
	if err != nil {
		t.Fatalf("open migration database driver: %v", err)
	}
	runner, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		t.Fatalf("create migration runner: %v", err)
	}
	if err := runner.Migrate(67); err != nil {
		t.Fatalf("migrate isolated database to version 67: %v", err)
	}
	assertMigrationVersion(t, runner, 67)

	if err := db.Exec(`
INSERT INTO projects(id, identifier, name) VALUES ('prj_quota_upgrade', 'quota-upgrade', 'Quota Upgrade');
INSERT INTO runtime_clusters(id, name) VALUES ('rclu_quota_upgrade', 'Quota Upgrade');
INSERT INTO project_volumes(
    id, project_id, display_name, cluster_id, namespace, claim_name,
    ownership_mode, source_kind, lifecycle_state, pending_operation,
    capacity_request, capacity_bytes, storage_class_name, access_mode, volume_mode, created_by
) VALUES
    ('pvol_upgrade_ready', 'prj_quota_upgrade', 'Ready managed', 'rclu_quota_upgrade', 'quota-upgrade', 'ready-managed',
     'managed', 'blank', 'ready', '', '3Gi', 3221225472, 'standard', 'ReadWriteOnce', 'Filesystem', 'usr_upgrade'),
    ('pvol_upgrade_pending', 'prj_quota_upgrade', 'Pending managed', 'rclu_quota_upgrade', 'quota-upgrade', 'pending-managed',
     'managed', 'blank', 'provisioning', 'provision', '2Gi', 2147483648, 'standard', 'ReadWriteOnce', 'Filesystem', 'usr_upgrade'),
    ('pvol_upgrade_failed_initial', 'prj_quota_upgrade', 'Failed initial managed', 'rclu_quota_upgrade', 'quota-upgrade', 'failed-managed',
     'managed', 'blank', 'error', 'provision', '4Gi', 4294967296, 'standard', 'ReadWriteOnce', 'Filesystem', 'usr_upgrade'),
    ('pvol_upgrade_referenced', 'prj_quota_upgrade', 'Referenced', 'rclu_quota_upgrade', 'quota-upgrade', 'referenced',
     'referenced', 'existing_claim', 'ready', '', '100Gi', 107374182400, 'standard', 'ReadWriteOnce', 'Filesystem', 'usr_upgrade');`).Error; err != nil {
		t.Fatalf("seed version 67 project volumes: %v", err)
	}

	if err := runner.Migrate(68); err != nil {
		t.Fatalf("upgrade isolated database from 67 to 68: %v", err)
	}
	assertMigrationVersion(t, runner, 68)
	for _, table := range []string{"project_volume_quota_usage", "project_volume_quota_reservations"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("version 68 is missing %s", table)
		}
	}

	var aggregate struct{ ReservedBytes int64 }
	if err := db.Table("project_volume_quota_usage").Select("reserved_bytes").Where("project_id = ?", "prj_quota_upgrade").Scan(&aggregate).Error; err != nil {
		t.Fatalf("read upgraded quota aggregate: %v", err)
	}
	if aggregate.ReservedBytes != 5*1024*1024*1024 {
		t.Fatalf("upgraded quota aggregate = %d, want 5Gi", aggregate.ReservedBytes)
	}
	type reservation struct {
		ProjectVolumeID string
		CommittedBytes  int64
		PendingBytes    int64
	}
	var reservations []reservation
	if err := db.Table("project_volume_quota_reservations").Order("project_volume_id ASC").Find(&reservations).Error; err != nil {
		t.Fatalf("read upgraded quota reservations: %v", err)
	}
	if len(reservations) != 2 ||
		reservations[0].ProjectVolumeID != "pvol_upgrade_pending" || reservations[0].CommittedBytes != 0 || reservations[0].PendingBytes != 2*1024*1024*1024 ||
		reservations[1].ProjectVolumeID != "pvol_upgrade_ready" || reservations[1].CommittedBytes != 3*1024*1024*1024 || reservations[1].PendingBytes != 0 {
		t.Fatalf("upgraded quota reservations = %#v", reservations)
	}

	var limitBytes int64
	if err := db.Raw(`SELECT luna_project_volume_quota_limit_bytes()`).Scan(&limitBytes).Error; err != nil {
		t.Fatalf("read default quota limit: %v", err)
	}
	if limitBytes != 0 {
		t.Fatalf("default quota limit = %d, want unlimited (0)", limitBytes)
	}
	var transferRate struct {
		Enabled        bool
		CreditsPerUnit string
	}
	if err := db.Table("billing_rate_rules").Select("enabled, credits_per_unit::text AS credits_per_unit").Where("meter = ?", "storage.transfer_gib").Scan(&transferRate).Error; err != nil {
		t.Fatalf("read transfer rate: %v", err)
	}
	if transferRate.Enabled || transferRate.CreditsPerUnit != "0.00000000" {
		t.Fatalf("default transfer rate = %#v", transferRate)
	}

	if err := db.Exec(`INSERT INTO app_configs(key, value, updated_at) VALUES ('storage.projectManagedCapacityLimitGiB', '4', now())`).Error; err != nil {
		t.Fatalf("lower quota below historical usage: %v", err)
	}
	insertErr := db.Exec(`
INSERT INTO project_volumes(
    id, project_id, display_name, cluster_id, namespace, claim_name,
    ownership_mode, source_kind, lifecycle_state, pending_operation,
    capacity_request, capacity_bytes, storage_class_name, access_mode, volume_mode, created_by
) VALUES (
    'pvol_upgrade_over_limit', 'prj_quota_upgrade', 'Over limit', 'rclu_quota_upgrade', 'quota-upgrade', 'over-limit',
    'managed', 'blank', 'provisioning', 'provision', '1Gi', 1073741824, 'standard', 'ReadWriteOnce', 'Filesystem', 'usr_upgrade'
)`).Error
	if insertErr == nil || !strings.Contains(strings.ToLower(insertErr.Error()), "sqlstate pvr01") {
		t.Fatalf("historical over-limit insert error = %v, want SQLSTATE PVR01", insertErr)
	}
	if err := db.Table("project_volume_quota_usage").Select("reserved_bytes").Where("project_id = ?", "prj_quota_upgrade").Scan(&aggregate).Error; err != nil {
		t.Fatalf("read quota aggregate after rejected insert: %v", err)
	}
	if aggregate.ReservedBytes != 5*1024*1024*1024 {
		t.Fatalf("historical over-limit usage was mutated to %d", aggregate.ReservedBytes)
	}

	if err := db.Exec(`
INSERT INTO volume_transfers(
    id, project_id, project_volume_id, direction, format, consistency_mode, state,
    object_key, expected_bytes, transferred_bytes, sha256, actor_id, expires_at, finished_at
) VALUES (
    'vtx_manifest_upgrade', 'prj_quota_upgrade', 'pvol_upgrade_ready', 'export', 'raw_zst', 'snapshot', 'succeeded',
    'opaque/manifest-upgrade', 128, 128, repeat('a', 64), 'usr_upgrade', now() + interval '1 hour', now()
)`).Error; err != nil {
		t.Fatalf("seed version 68 block transfer: %v", err)
	}

	if err := runner.Migrate(69); err != nil {
		t.Fatalf("upgrade isolated database from 68 to 69: %v", err)
	}
	assertMigrationVersion(t, runner, 69)
	var manifestMetadata struct {
		LogicalBytes int64
		DataSHA256   string
	}
	if err := db.Table("volume_transfers").Select("logical_bytes, data_sha256").Where("id = ?", "vtx_manifest_upgrade").Scan(&manifestMetadata).Error; err != nil {
		t.Fatalf("read upgraded block transfer metadata: %v", err)
	}
	if manifestMetadata.LogicalBytes != 0 || manifestMetadata.DataSHA256 != "" {
		t.Fatalf("legacy block transfer metadata = %#v, want unavailable defaults", manifestMetadata)
	}
	if err := db.Table("volume_transfers").Where("id = ?", "vtx_manifest_upgrade").Updates(map[string]any{
		"logical_bytes": int64(4096),
		"data_sha256":   strings.Repeat("b", 64),
	}).Error; err != nil {
		t.Fatalf("persist valid block manifest metadata: %v", err)
	}
	if err := db.Table("volume_transfers").Where("id = ?", "vtx_manifest_upgrade").Update("data_sha256", "").Error; err == nil {
		t.Fatal("accepted logical bytes without the server-observed digest")
	}

	if err := runner.Migrate(70); err != nil {
		t.Fatalf("upgrade isolated database from 69 to 70: %v", err)
	}
	assertMigrationVersion(t, runner, 70)
	var completionEvidence struct {
		CompletionReportedAt        *time.Time
		JobSucceededAt              *time.Time
		ExecutionCleanupCompletedAt *time.Time
		FinishedAt                  *time.Time
	}
	if err := db.Table("volume_transfers").
		Select("completion_reported_at, job_succeeded_at, execution_cleanup_completed_at, finished_at").
		Where("id = ?", "vtx_manifest_upgrade").Scan(&completionEvidence).Error; err != nil {
		t.Fatalf("read upgraded transfer completion evidence: %v", err)
	}
	if completionEvidence.CompletionReportedAt == nil || completionEvidence.JobSucceededAt == nil || completionEvidence.ExecutionCleanupCompletedAt == nil || completionEvidence.FinishedAt == nil ||
		!completionEvidence.CompletionReportedAt.Equal(*completionEvidence.FinishedAt) || !completionEvidence.JobSucceededAt.Equal(*completionEvidence.FinishedAt) {
		t.Fatalf("legacy success evidence was not backfilled from finished_at: %#v", completionEvidence)
	}
	if !completionEvidence.ExecutionCleanupCompletedAt.Equal(*completionEvidence.FinishedAt) {
		t.Fatalf("legacy execution cleanup marker was not backfilled from finished_at: %#v", completionEvidence)
	}
	if err := db.Exec(`
INSERT INTO volume_transfers(
    id, project_id, project_volume_id, direction, format, consistency_mode, state,
    object_key, actor_id, expires_at
) VALUES (
    'vtx_missing_completion_evidence', 'prj_quota_upgrade', 'pvol_upgrade_pending', 'export', 'tar_gz', 'unmounted', 'running',
    'opaque/missing-completion-evidence', 'usr_upgrade', now() + interval '1 hour'
)`).Error; err != nil {
		t.Fatalf("seed transfer without completion evidence: %v", err)
	}
	if err := db.Table("volume_transfers").Where("id = ?", "vtx_missing_completion_evidence").Update("state", "succeeded").Error; err == nil {
		t.Fatal("version 70 accepted succeeded transfer without callback and Job evidence")
	}

	if err := runner.Migrate(69); err != nil {
		t.Fatalf("roll back isolated database from 70 to 69: %v", err)
	}
	assertMigrationVersion(t, runner, 69)
	if db.Migrator().HasColumn("volume_transfers", "completion_reported_at") || db.Migrator().HasColumn("volume_transfers", "job_succeeded_at") ||
		db.Migrator().HasColumn("volume_transfers", "execution_cleanup_completed_at") {
		t.Fatal("version 70 down migration retained completion evidence columns")
	}
	if err := runner.Migrate(70); err != nil {
		t.Fatalf("reapply isolated database version 70: %v", err)
	}
	assertMigrationVersion(t, runner, 70)
}

func assertMigrationVersion(t *testing.T, runner *migrate.Migrate, expected uint) {
	t.Helper()
	version, dirty, err := runner.Version()
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if dirty || version != expected {
		t.Fatalf("migration version = %d dirty=%t, want %d clean", version, dirty, expected)
	}
}
