package volume

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const quotaTestGiB = int64(1024 * 1024 * 1024)

func TestProjectVolumeQuotaMigrationUsesDurableTransactionalReservations(t *testing.T) {
	t.Parallel()
	up := strings.ToLower(readVolumeMigration(t, "000068_project_volume_quota_billing.up.sql"))
	for _, required := range []string{
		"create table project_volume_quota_usage",
		"create table project_volume_quota_reservations",
		"from projects where id = new.project_id for update",
		"project_volume_quota_exceeded",
		"project_volume_quota_config_invalid",
		"project_volume_quota_project_immutable",
		"errcode = 'pvr01'",
		"errcode = 'pvr02'",
		"errcode = 'pvr03'",
		"'storage.transfer_gib'",
	} {
		if !strings.Contains(up, required) {
			t.Errorf("quota migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{"pg_advisory", "pg_try_advisory"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("quota migration must not use process/advisory locking: %q", forbidden)
		}
	}
	if !strings.Contains(up, "if delta_bytes > 0 then\n        quota_limit_bytes := luna_project_volume_quota_limit_bytes()") {
		t.Fatal("quota configuration must be evaluated only for positive reservations so release and referenced-volume paths remain available")
	}

	down := strings.ToLower(readVolumeMigration(t, "000068_project_volume_quota_billing.down.sql"))
	for _, required := range []string{
		"drop trigger if exists trg_project_volumes_quota_update",
		"drop function if exists luna_sync_project_volume_quota",
		"drop table if exists project_volume_quota_reservations",
		"drop table if exists project_volume_quota_usage",
		"storage.transfer_gib",
	} {
		if !strings.Contains(down, required) {
			t.Errorf("quota rollback migration is missing %q", required)
		}
	}
}

func TestProjectVolumeQuotaPostgresLifecycleConcurrencyAndCancellation(t *testing.T) {
	db := openVolumeTestDB(t)
	installProjectVolumeTestSchema(t, db)
	installProjectVolumeQuotaTestSchema(t, db)
	if err := db.Exec(`
INSERT INTO projects(id, identifier, name) VALUES
  ('prj_quota', 'quota', 'Quota'),
  ('prj_quota_other', 'quota-other', 'Quota Other');
INSERT INTO runtime_clusters(id, name) VALUES ('rclu_quota', 'Quota');
INSERT INTO app_configs(key, value) VALUES ('storage.projectManagedCapacityLimitGiB', '10');`).Error; err != nil {
		t.Fatalf("seed quota parents and config: %v", err)
	}

	repository := NewGormRepository(db)
	type createResult struct {
		volume model.ProjectVolume
		err    error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			service := NewService(repository, quotaNoopDispatcher{})
			result, err := service.CreateProjectVolume(context.Background(), quotaCreateInput(index, 6))
			results <- createResult{volume: result.Volume, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	created := make([]model.ProjectVolume, 0, 1)
	quotaRejected := 0
	for result := range results {
		switch ErrorCode(result.err) {
		case "":
			if result.err != nil {
				t.Fatalf("unexpected concurrent create error: %v", result.err)
			}
			created = append(created, result.volume)
		case CodeQuotaExceeded:
			quotaRejected++
		default:
			t.Fatalf("unexpected concurrent create code %q: %v", ErrorCode(result.err), result.err)
		}
	}
	if len(created) != 1 || quotaRejected != 1 {
		t.Fatalf("concurrent reservations: created=%d quotaRejected=%d", len(created), quotaRejected)
	}
	volume := created[0]
	assertQuotaState(t, db, "prj_quota", volume.ID, 6*quotaTestGiB, 0, 6*quotaTestGiB)

	ready, err := repository.TransitionProjectVolume(context.Background(), "prj_quota", volume.ID,
		[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleReady, "", "")
	if err != nil {
		t.Fatalf("complete provision: %v", err)
	}
	assertQuotaState(t, db, "prj_quota", volume.ID, 6*quotaTestGiB, 6*quotaTestGiB, 0)

	service := NewService(repository, quotaNoopDispatcher{})
	overLimitCapacity := int64(11 * quotaTestGiB)
	overLimitRequest := "11Gi"
	if _, err = service.UpdateProjectVolume(context.Background(), "prj_quota", volume.ID, ready.Revision, UpdateProjectVolumeInput{
		ActorID: "usr_quota", CapacityBytes: &overLimitCapacity, CapacityRequest: &overLimitRequest,
	}); ErrorCode(err) != CodeQuotaExceeded {
		t.Fatalf("over-limit expansion error = %v (code %q)", err, ErrorCode(err))
	}
	afterRejected, err := service.GetProjectVolume(context.Background(), "prj_quota", volume.ID)
	if err != nil {
		t.Fatalf("read volume after rejected expansion: %v", err)
	}
	if afterRejected.CapacityBytes != 6*quotaTestGiB || afterRejected.Revision != ready.Revision {
		t.Fatalf("rejected expansion changed desired state: %#v", afterRejected)
	}

	if err := db.Model(&model.AppConfig{}).Where("key = ?", ProjectManagedCapacityLimitConfigKey).Update("value", "12").Error; err != nil {
		t.Fatalf("raise quota: %v", err)
	}
	expandedCapacity := int64(10 * quotaTestGiB)
	expandedRequest := "10Gi"
	_, err = service.UpdateProjectVolume(context.Background(), "prj_quota", volume.ID, ready.Revision, UpdateProjectVolumeInput{
		ActorID: "usr_quota", CapacityBytes: &expandedCapacity, CapacityRequest: &expandedRequest,
	})
	if err != nil {
		t.Fatalf("reserve expansion delta: %v", err)
	}
	assertQuotaState(t, db, "prj_quota", volume.ID, 10*quotaTestGiB, 6*quotaTestGiB, 4*quotaTestGiB)

	failed, err := service.SetProjectVolumeLifecycle(context.Background(), "prj_quota", volume.ID,
		[]string{model.ProjectVolumeLifecycleReady}, model.ProjectVolumeLifecycleError, "volume.expand_failed", "provider failure")
	if err != nil {
		t.Fatalf("fail expansion: %v", err)
	}
	assertQuotaState(t, db, "prj_quota", volume.ID, 6*quotaTestGiB, 6*quotaTestGiB, 0)

	_, err = service.RetryProjectVolumeOperation(context.Background(), "prj_quota", volume.ID, "usr_quota", failed.Revision)
	if err != nil {
		t.Fatalf("retry expansion: %v", err)
	}
	assertQuotaState(t, db, "prj_quota", volume.ID, 10*quotaTestGiB, 6*quotaTestGiB, 4*quotaTestGiB)
	completed, err := service.SetProjectVolumeLifecycle(context.Background(), "prj_quota", volume.ID,
		[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleReady, "", "")
	if err != nil {
		t.Fatalf("complete expansion retry: %v", err)
	}
	assertQuotaState(t, db, "prj_quota", volume.ID, 10*quotaTestGiB, 10*quotaTestGiB, 0)

	deleting, detached, err := service.RequestDeleteProjectVolume(context.Background(), DeleteProjectVolumeInput{
		ProjectID: "prj_quota", VolumeID: volume.ID, ActorID: "usr_quota", ExpectedRevision: completed.Revision, DataAction: "delete",
	})
	if err != nil || detached {
		t.Fatalf("request managed deletion: detached=%t err=%v", detached, err)
	}
	assertQuotaState(t, db, "prj_quota", volume.ID, 10*quotaTestGiB, 10*quotaTestGiB, 0)
	if _, err = service.CompleteProjectVolumeDeletion(context.Background(), "prj_quota", deleting.ID); err != nil {
		t.Fatalf("complete managed deletion: %v", err)
	}
	assertQuotaState(t, db, "prj_quota", "", 0, 0, 0)

	if err := db.Model(&model.AppConfig{}).Where("key = ?", ProjectManagedCapacityLimitConfigKey).Update("value", "invalid").Error; err != nil {
		t.Fatalf("write malformed quota for fail-closed test: %v", err)
	}
	referenced := quotaReferencedVolume("pvol_quota_ref", "prj_quota")
	if err := repository.CreateProjectVolume(context.Background(), &referenced); err != nil {
		t.Fatalf("referenced volume must not consume or read managed quota: %v", err)
	}
	assertStoredQuotaUsage(t, db, "prj_quota", 0, 0)
	if _, err = service.CreateProjectVolume(context.Background(), quotaCreateInput(3, 1)); ErrorCode(err) != CodeQuotaUnavailable {
		t.Fatalf("malformed managed quota error = %v (code %q)", err, ErrorCode(err))
	}

	if err := db.Model(&model.AppConfig{}).Where("key = ?", ProjectManagedCapacityLimitConfigKey).Update("value", "12").Error; err != nil {
		t.Fatalf("restore quota: %v", err)
	}
	projectMutation := db.Model(&model.ProjectVolume{}).Where("id = ?", referenced.ID).Update("project_id", "prj_quota_other").Error
	if ErrorCode(normalizeRepositoryError(projectMutation)) != CodeQuotaUnavailable {
		t.Fatalf("project mutation error = %v", projectMutation)
	}
	var persistedProjectID string
	if err := db.Model(&model.ProjectVolume{}).Select("project_id").Where("id = ?", referenced.ID).Scan(&persistedProjectID).Error; err != nil {
		t.Fatalf("read referenced volume project after rejected mutation: %v", err)
	}
	if persistedProjectID != "prj_quota" {
		t.Fatalf("project mutation persisted as %q", persistedProjectID)
	}

	locker := db.Begin()
	if locker.Error != nil {
		t.Fatalf("begin quota lock transaction: %v", locker.Error)
	}
	defer locker.Rollback()
	if err := locker.Exec(`SELECT id FROM projects WHERE id = 'prj_quota' FOR UPDATE`).Error; err != nil {
		t.Fatalf("lock durable project row: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = service.CreateProjectVolume(ctx, quotaCreateInput(4, 1))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked quota reservation error = %v, want context deadline exceeded", err)
	}
}

type quotaNoopDispatcher struct{}

func (quotaNoopDispatcher) DispatchVolumeOperation(context.Context, VolumeOperation) error {
	return nil
}

func quotaCreateInput(index int, capacityGiB int64) CreateProjectVolumeInput {
	return CreateProjectVolumeInput{
		ProjectID: "prj_quota", DisplayName: fmt.Sprintf("quota-volume-%d", index), ClusterID: "rclu_quota", Namespace: "quota-test",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		CapacityRequest: fmt.Sprintf("%dGi", capacityGiB), CapacityBytes: capacityGiB * quotaTestGiB, StorageClassName: "standard",
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		ActorID: "usr_quota", IdempotencyKey: fmt.Sprintf("quota-create-request-%08d", index),
	}
}

func quotaReferencedVolume(volumeID, projectID string) model.ProjectVolume {
	return model.ProjectVolume{
		ID: volumeID, ProjectID: projectID, DisplayName: volumeID, ClusterID: "rclu_quota", Namespace: "quota-test", ClaimName: volumeID,
		OwnershipMode: model.ProjectVolumeOwnershipReferenced, SourceKind: model.ProjectVolumeSourceExistingClaim,
		LifecycleState: model.ProjectVolumeLifecycleReady, PendingOperation: "", CapacityRequest: "100Gi", CapacityBytes: 100 * quotaTestGiB,
		StorageClassName: "standard", AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: "usr_quota", Revision: 1,
	}
}

func installProjectVolumeQuotaTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(readVolumeMigration(t, "000068_project_volume_quota_billing.up.sql")).Error; err != nil {
		t.Fatalf("install project volume quota schema: %v", err)
	}
}

func assertQuotaState(t *testing.T, db *gorm.DB, projectID, volumeID string, wantUsage, wantCommitted, wantPending int64) {
	t.Helper()
	snapshot, err := NewQuotaRepository(db).Get(context.Background(), projectID)
	if err != nil {
		t.Fatalf("read quota snapshot: %v", err)
	}
	if snapshot.ReservedBytes != wantUsage {
		t.Fatalf("quota usage = %d, want %d", snapshot.ReservedBytes, wantUsage)
	}
	if volumeID == "" {
		var reservationCount int64
		if err := db.Model(&model.ProjectVolumeQuotaReservation{}).Where("project_id = ?", projectID).Count(&reservationCount).Error; err != nil {
			t.Fatalf("count quota reservations: %v", err)
		}
		if reservationCount != 0 {
			t.Fatalf("quota reservation count = %d, want 0", reservationCount)
		}
		return
	}
	var reservation model.ProjectVolumeQuotaReservation
	if err := db.First(&reservation, "project_volume_id = ?", volumeID).Error; err != nil {
		t.Fatalf("read quota reservation: %v", err)
	}
	if reservation.CommittedBytes != wantCommitted || reservation.PendingBytes != wantPending {
		t.Fatalf("quota reservation = committed %d pending %d, want %d/%d", reservation.CommittedBytes, reservation.PendingBytes, wantCommitted, wantPending)
	}
}

func assertStoredQuotaUsage(t *testing.T, db *gorm.DB, projectID string, wantUsage, wantReservations int64) {
	t.Helper()
	var usage model.ProjectVolumeQuotaUsage
	if err := db.First(&usage, "project_id = ?", projectID).Error; err != nil {
		t.Fatalf("read stored quota usage: %v", err)
	}
	if usage.ReservedBytes != wantUsage {
		t.Fatalf("stored quota usage = %d, want %d", usage.ReservedBytes, wantUsage)
	}
	var reservations int64
	if err := db.Model(&model.ProjectVolumeQuotaReservation{}).Where("project_id = ?", projectID).Count(&reservations).Error; err != nil {
		t.Fatalf("count stored quota reservations: %v", err)
	}
	if reservations != wantReservations {
		t.Fatalf("stored quota reservations = %d, want %d", reservations, wantReservations)
	}
}
