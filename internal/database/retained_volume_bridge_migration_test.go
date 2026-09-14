package database

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/golang-migrate/migrate/v4"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRetainedVolumeBridgeMigration(t *testing.T) {
	db, runner := openRetainedVolumeBridgeTestDatabase(t)
	if err := runner.Migrate(baselineMigrationVersion); err != nil {
		t.Fatalf("migrate retained bridge database to baseline: %v", err)
	}
	seedRetainedVolumeBridgeMigration(t, db)
	if err := runner.Migrate(latestMigrationVersion); err != nil {
		t.Fatalf("migrate retained bridge database to latest: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, latestMigrationVersion)
	assertRetainedVolumeBridgeMigration(t, db)
	assertRetainedVolumeBridgeFailures(t, db, runner)
}

func TestRetainedVolumeBridgeSkipsAmbiguousLegacyDefaultCluster(t *testing.T) {
	db, runner := openRetainedVolumeBridgeTestDatabase(t)
	if err := runner.Migrate(baselineMigrationVersion); err != nil {
		t.Fatalf("migrate retained bridge database to baseline: %v", err)
	}
	if err := db.Exec(`
INSERT INTO projects (id, identifier, name) VALUES
    ('prj_retained_fallback', 'retained-fallback', 'Retained Fallback');

INSERT INTO runtime_clusters (id, name, scope, is_default, delete_status, created_at) VALUES
    ('rclu_retained_later', 'Retained Later', 'global', false, 'active', '2026-02-01T00:00:00Z'),
    ('rclu_retained_earliest', 'Retained Earliest', 'global', false, 'active', '2026-01-01T00:00:00Z');

INSERT INTO retained_volumes (
    id, project_id, source_application_id, source_deployment_target_id,
    cluster_id, namespace, claim_name, capacity, storage_class_name,
    access_mode, volume_mode, status, retained_at
) VALUES (
    'rvol_default_fallback', 'prj_retained_fallback', 'app_removed', 'dplt_removed',
    '', 'luna-retained-fallback', 'fallback-data', '1Gi', 'standard',
    'ReadWriteOnce', 'Filesystem', 'retained', now()
)`).Error; err != nil {
		t.Fatalf("seed ambiguous default cluster fixture: %v", err)
	}
	if err := runner.Migrate(latestMigrationVersion); err != nil {
		t.Fatalf("migrate retained bridge database to latest: %v", err)
	}

	var count int64
	if err := db.Table("project_volumes").
		Where("claim_name = ? AND source_kind = ?", "fallback-data", model.ProjectVolumeSourceRetained).
		Count(&count).Error; err != nil {
		t.Fatalf("count ambiguous retained volume: %v", err)
	}
	if count != 0 {
		t.Fatalf("ambiguous retained volume count = %d, want 0", count)
	}
}

func TestRetainedVolumeBridgeDoesNotRecreateSoftDeletedVolumeAfterRollback(t *testing.T) {
	db, runner := openRetainedVolumeBridgeTestDatabase(t)
	if err := runner.Migrate(baselineMigrationVersion); err != nil {
		t.Fatalf("migrate retained bridge database to baseline: %v", err)
	}
	if err := db.Exec(`
INSERT INTO projects (id, identifier, name) VALUES
    ('prj_retained_replay', 'retained-replay', 'Retained Replay');

INSERT INTO runtime_clusters (id, name, scope, is_default, delete_status) VALUES
    ('rclu_retained_replay', 'Retained Replay', 'global', true, 'active');

INSERT INTO retained_volumes (
    id, project_id, source_application_id, source_deployment_target_id,
    cluster_id, namespace, claim_name, capacity, storage_class_name,
    access_mode, volume_mode, status, retained_at
) VALUES (
    'rvol_replay', 'prj_retained_replay', 'app_removed', 'dplt_removed',
    'rclu_retained_replay', 'luna-retained-replay', 'replay-data', '1Gi', 'standard',
    'ReadWriteOnce', 'Filesystem', 'retained', now()
)`).Error; err != nil {
		t.Fatalf("seed retained replay fixture: %v", err)
	}
	if err := runner.Migrate(latestMigrationVersion); err != nil {
		t.Fatalf("migrate retained replay database to latest: %v", err)
	}

	var migrated model.ProjectVolume
	if err := db.First(&migrated, "claim_name = ?", "replay-data").Error; err != nil {
		t.Fatalf("read migrated replay volume: %v", err)
	}
	if err := db.Delete(&migrated).Error; err != nil {
		t.Fatalf("soft-delete migrated replay volume: %v", err)
	}
	if err := runner.Migrate(baselineMigrationVersion); err != nil {
		t.Fatalf("roll back retained replay migration: %v", err)
	}
	if err := runner.Migrate(latestMigrationVersion); err != nil {
		t.Fatalf("reapply retained replay migration: %v", err)
	}

	var total int64
	if err := db.Unscoped().Model(&model.ProjectVolume{}).Where("id = ?", migrated.ID).Count(&total).Error; err != nil {
		t.Fatalf("count retained replay identities: %v", err)
	}
	if total != 1 {
		t.Fatalf("retained replay identity count = %d, want 1", total)
	}
	var active int64
	if err := db.Model(&model.ProjectVolume{}).Where("id = ?", migrated.ID).Count(&active).Error; err != nil {
		t.Fatalf("count active retained replay volumes: %v", err)
	}
	if active != 0 {
		t.Fatalf("active retained replay volume count = %d, want 0", active)
	}
	var reservations int64
	if err := db.Table("project_volume_quota_reservations").Where("project_volume_id = ?", migrated.ID).Count(&reservations).Error; err != nil {
		t.Fatalf("count retained replay quota reservations: %v", err)
	}
	if reservations != 0 {
		t.Fatalf("retained replay quota reservation count = %d, want 0", reservations)
	}
}

func openRetainedVolumeBridgeTestDatabase(t *testing.T) (*gorm.DB, *migrate.Migrate) {
	t.Helper()
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

	databaseName := fmt.Sprintf("luna_retained_bridge_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(databaseName, "luna_retained_bridge_test_") {
		t.Fatalf("refuse unsafe retained bridge test database name %q", databaseName)
	}
	if err := adminDB.Exec(`CREATE DATABASE "` + databaseName + `"`).Error; err != nil {
		t.Fatalf("create isolated retained bridge database: %v", err)
	}
	t.Cleanup(func() {
		if dropErr := adminDB.Exec(`DROP DATABASE IF EXISTS "` + databaseName + `" WITH (FORCE)`).Error; dropErr != nil {
			t.Errorf("drop isolated retained bridge database: %v", dropErr)
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
		t.Fatalf("open isolated retained bridge database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open isolated retained bridge SQL database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, openTestMigrationRunner(t, db)
}

func seedRetainedVolumeBridgeMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`
INSERT INTO projects (id, identifier, name, delete_status, deleted_at) VALUES
    ('prj_retained_bridge', 'retained-bridge', 'Retained Bridge', 'active', NULL),
    ('prj_retained_deleting', 'retained-deleting', 'Retained Deleting', 'deleting', NULL),
    ('prj_retained_deleted', 'retained-deleted', 'Retained Deleted', 'deleted', now());

INSERT INTO runtime_clusters (id, name, scope, owner_ref, is_default, delete_status, created_at, deleted_at) VALUES
    ('rclu_retained_bridge', 'Retained Bridge', 'project', 'prj_retained_bridge', false, 'active', '2026-01-01T00:00:00Z', NULL),
    ('rclu_retained_default', 'Retained Default', 'global', '', true, 'active', '2026-02-01T00:00:00Z', NULL),
    ('rclu_retained_deleted', 'Retained Deleted', 'global', '', false, 'deleted', '2026-01-01T00:00:00Z', now());

INSERT INTO app_configs (key, value)
VALUES ('storage.projectManagedCapacityLimitGiB', '1');

INSERT INTO project_volumes (
    id, project_id, display_name, cluster_id, namespace, claim_name,
    ownership_mode, source_kind, lifecycle_state, pending_operation,
    capacity_request, capacity_bytes, storage_class_name, access_mode, volume_mode, created_by
) VALUES (
    'pvol_existing_retained_claim', 'prj_retained_bridge', 'Existing claim',
    'rclu_retained_bridge', 'luna-retained-bridge', 'existing-data',
    'referenced', 'existing_claim', 'ready', '',
    '1Gi', 1073741824, 'standard', 'ReadWriteOnce', 'Filesystem', 'usr_bridge'
);

INSERT INTO retained_volumes (
    id, project_id, source_application_id, source_application_name,
    source_deployment_target_id, cluster_id, namespace, claim_name,
    capacity, storage_class_name, access_mode, volume_mode, status, retained_at
) VALUES
    ('rvol_binary', 'prj_retained_bridge', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_bridge', 'luna-retained-bridge', 'binary-data',
     '10Gi', 'standard', 'ReadWriteOnce', '', 'retained', now() - interval '3 hours'),
    ('rvol_decimal', 'prj_retained_bridge', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_bridge', 'luna-retained-bridge', 'decimal-data',
     '500M', 'standard', 'ReadWriteMany', 'Filesystem', 'retained', now() - interval '2 hours'),
    ('rvol_exponent', 'prj_retained_bridge', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_bridge', 'luna-retained-bridge', 'exponent-data',
     '1e9', 'standard', 'ReadOnlyMany', 'Block', 'retained', now() - interval '1 hour'),
    ('rvol_default_cluster', 'prj_retained_bridge', 'app_removed', 'Removed app',
     'dplt_removed', '', 'luna-retained-bridge', 'default-cluster-data',
     '2Gi', 'standard', 'ReadWriteOnce', 'Filesystem', 'retained', now()),
    ('rvol_existing', 'prj_retained_bridge', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_bridge', 'luna-retained-bridge', 'existing-data',
     '1Gi', 'standard', 'ReadWriteOnce', 'Filesystem', 'retained', now()),
    ('rvol_claimed', 'prj_retained_bridge', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_bridge', 'luna-retained-bridge', 'claimed-data',
     '1Gi', 'standard', 'ReadWriteOnce', 'Filesystem', 'claimed', now()),
    ('rvol_deleted_project', 'prj_retained_deleted', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_bridge', 'luna-retained-deleted', 'deleted-project-data',
     '1Gi', 'standard', 'ReadWriteOnce', 'Filesystem', 'retained', now()),
    ('rvol_deleting_project', 'prj_retained_deleting', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_bridge', 'luna-retained-deleting', 'deleting-project-data',
     '1Gi', 'standard', 'ReadWriteOnce', 'Filesystem', 'retained', now()),
    ('rvol_deleted_cluster', 'prj_retained_bridge', 'app_removed', 'Removed app',
     'dplt_removed', 'rclu_retained_deleted', 'luna-retained-bridge', 'deleted-cluster-data',
     '1Gi', 'standard', 'ReadWriteOnce', 'Filesystem', 'retained', now());`).Error; err != nil {
		t.Fatalf("seed retained volume bridge migration: %v", err)
	}
}

func assertRetainedVolumeBridgeMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	type migratedVolume struct {
		ID               string
		ClaimName        string
		ClusterID        string
		CapacityRequest  string
		CapacityBytes    int64
		VolumeMode       string
		SourceKind       string
		LifecycleState   string
		PendingOperation string
	}
	var volumes []migratedVolume
	if err := db.Table("project_volumes").
		Where("source_kind = ?", model.ProjectVolumeSourceRetained).
		Order("claim_name ASC").Find(&volumes).Error; err != nil {
		t.Fatalf("read migrated retained volumes: %v", err)
	}
	want := map[string]migratedVolume{
		"binary-data": {
			ID: "pvol_21de878d73796a7e390b5bc8", ClusterID: "rclu_retained_bridge", CapacityRequest: "10Gi",
			CapacityBytes: 10 * 1024 * 1024 * 1024, VolumeMode: model.ProjectVolumeModeFilesystem,
		},
		"default-cluster-data": {
			ID: "pvol_3de25c5618744ecb4c4f64bd", ClusterID: "rclu_retained_default", CapacityRequest: "2Gi",
			CapacityBytes: 2 * 1024 * 1024 * 1024, VolumeMode: model.ProjectVolumeModeFilesystem,
		},
		"decimal-data": {
			ID: "pvol_eceee71b3f1343583c927e44", ClusterID: "rclu_retained_bridge", CapacityRequest: "500M",
			CapacityBytes: 500_000_000, VolumeMode: model.ProjectVolumeModeFilesystem,
		},
		"exponent-data": {
			ID: "pvol_51dd1f7355aa419bd9865f2d", ClusterID: "rclu_retained_bridge", CapacityRequest: "1e9",
			CapacityBytes: 1_000_000_000, VolumeMode: model.ProjectVolumeModeBlock,
		},
	}
	if len(volumes) != len(want) {
		t.Fatalf("migrated retained volume count = %d, want %d: %#v", len(volumes), len(want), volumes)
	}
	for _, volume := range volumes {
		expected, ok := want[volume.ClaimName]
		if !ok || volume.ID != expected.ID || volume.ClusterID != expected.ClusterID || volume.CapacityRequest != expected.CapacityRequest ||
			volume.CapacityBytes != expected.CapacityBytes || volume.VolumeMode != expected.VolumeMode ||
			volume.SourceKind != model.ProjectVolumeSourceRetained ||
			volume.LifecycleState != model.ProjectVolumeLifecycleReady || volume.PendingOperation != "" {
			t.Fatalf("migrated retained volume = %#v, want %#v", volume, expected)
		}
	}

	for _, claimName := range []string{"existing-data", "claimed-data", "deleting-project-data", "deleted-project-data", "deleted-cluster-data"} {
		var count int64
		if err := db.Table("project_volumes").Where("claim_name = ? AND source_kind = ?", claimName, model.ProjectVolumeSourceRetained).Count(&count).Error; err != nil {
			t.Fatalf("count skipped retained claim %s: %v", claimName, err)
		}
		if count != 0 {
			t.Fatalf("retained claim %s from an ineligible source was migrated", claimName)
		}
	}

	const expectedCapacity = int64(12*1024*1024*1024 + 500_000_000 + 1_000_000_000)
	var usage struct{ ReservedBytes int64 }
	if err := db.Table("project_volume_quota_usage").Select("reserved_bytes").
		Where("project_id = ?", "prj_retained_bridge").Take(&usage).Error; err != nil {
		t.Fatalf("read migrated quota usage: %v", err)
	}
	if usage.ReservedBytes != expectedCapacity {
		t.Fatalf("migrated quota usage = %d, want %d", usage.ReservedBytes, expectedCapacity)
	}
	var reservationCount int64
	if err := db.Table("project_volume_quota_reservations").Where("project_id = ?", "prj_retained_bridge").Count(&reservationCount).Error; err != nil {
		t.Fatalf("count migrated quota reservations: %v", err)
	}
	if reservationCount != int64(len(want)) {
		t.Fatalf("migrated quota reservations = %d, want %d", reservationCount, len(want))
	}
}

func assertRetainedVolumeBridgeFailures(t *testing.T, db *gorm.DB, runner *migrate.Migrate) {
	t.Helper()
	if err := runner.Migrate(baselineMigrationVersion); err != nil {
		t.Fatalf("roll back retained volume bridge: %v", err)
	}
	if err := db.Exec(`
INSERT INTO retained_volumes (
    id, project_id, source_application_id, source_deployment_target_id,
    cluster_id, namespace, claim_name, capacity, storage_class_name,
    access_mode, volume_mode, status, retained_at
) VALUES (
    'rvol_invalid_capacity', 'prj_retained_bridge', 'app_removed', 'dplt_removed',
    'rclu_retained_bridge', 'luna-retained-bridge', 'invalid-data', 'not-a-quantity',
    'standard', 'ReadWriteOnce', 'Filesystem', 'retained', now()
)`).Error; err != nil {
		t.Fatalf("seed invalid retained capacity: %v", err)
	}
	if err := runner.Migrate(latestMigrationVersion); err == nil || !strings.Contains(strings.ToUpper(err.Error()), "PVR04") {
		t.Fatalf("invalid retained capacity migration error = %v, want SQLSTATE PVR04", err)
	}
	assertNoProjectVolumeClaim(t, db, "invalid-data")

	if err := runner.Force(baselineMigrationVersion); err != nil {
		t.Fatalf("restore clean baseline after capacity rejection: %v", err)
	}
	if err := db.Exec(`DELETE FROM retained_volumes WHERE id = 'rvol_invalid_capacity'`).Error; err != nil {
		t.Fatalf("remove invalid capacity fixture: %v", err)
	}
	if err := db.Exec(`
INSERT INTO project_volumes (
    id, project_id, display_name, cluster_id, namespace, claim_name,
    ownership_mode, source_kind, lifecycle_state, pending_operation,
    capacity_request, capacity_bytes, storage_class_name, access_mode, volume_mode, created_by
) VALUES (
    'pvol_trigger_display_owner', 'prj_retained_bridge', 'migrated-trigger-failure-0fcfc7991f66f93c6c68fc0e',
    'rclu_retained_bridge', 'luna-retained-bridge', 'trigger-collision-owner',
    'referenced', 'existing_claim', 'ready', '',
    '1Gi', 1073741824, 'standard', 'ReadWriteOnce', 'Filesystem', 'usr_bridge'
);
INSERT INTO retained_volumes (
    id, project_id, source_application_id, source_deployment_target_id,
    cluster_id, namespace, claim_name, capacity, storage_class_name,
    access_mode, volume_mode, status, retained_at
) VALUES (
    'rvol_trigger_rollback', 'prj_retained_bridge', 'app_removed', 'dplt_removed',
    'rclu_retained_bridge', 'luna-retained-bridge', 'trigger-failure', '1Gi',
    'standard', 'ReadWriteOnce', 'Filesystem', 'retained', now()
)`).Error; err != nil {
		t.Fatalf("seed post-disable migration failure: %v", err)
	}
	if err := runner.Migrate(latestMigrationVersion); err == nil || !strings.Contains(strings.ToLower(err.Error()), "idx_project_volumes_display_name_active") {
		t.Fatalf("project volume display-name collision migration error = %v, want idx_project_volumes_display_name_active", err)
	}
	var triggerState string
	if err := db.Raw(`SELECT tgenabled FROM pg_trigger WHERE tgname = 'trg_project_volumes_quota_insert'`).Scan(&triggerState).Error; err != nil {
		t.Fatalf("read quota trigger state after failed migration: %v", err)
	}
	if triggerState != "O" {
		t.Fatalf("quota insert trigger state after failed migration = %q, want O", triggerState)
	}
	assertNoProjectVolumeClaim(t, db, "trigger-failure")

	if err := runner.Force(baselineMigrationVersion); err != nil {
		t.Fatalf("restore clean baseline after post-disable rejection: %v", err)
	}
	if err := db.Exec(`
DELETE FROM retained_volumes WHERE id = 'rvol_trigger_rollback';
DELETE FROM project_volumes WHERE id = 'pvol_trigger_display_owner';`).Error; err != nil {
		t.Fatalf("remove trigger rollback fixtures: %v", err)
	}
	if err := runner.Migrate(latestMigrationVersion); err != nil {
		t.Fatalf("reapply retained volume bridge: %v", err)
	}
	assertRunnerMigrationVersion(t, runner, latestMigrationVersion)
}

func assertNoProjectVolumeClaim(t *testing.T, db *gorm.DB, claimName string) {
	t.Helper()
	var count int64
	if err := db.Table("project_volumes").Where("claim_name = ?", claimName).Count(&count).Error; err != nil {
		t.Fatalf("count project volume claim %s: %v", claimName, err)
	}
	if count != 0 {
		t.Fatalf("project volume claim %s was partially migrated", claimName)
	}
}
