package volume

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestGormRepositoryProjectVolumeContract(t *testing.T) {
	db := openVolumeTestDB(t)
	installProjectVolumeTestSchema(t, db)
	repository := NewGormRepository(db)
	service := NewService(repository)

	if err := db.Exec(`
INSERT INTO projects(id, identifier, name) VALUES ('prj_volume_test', 'volume-test', 'Volume Test');
INSERT INTO runtime_clusters(id, name) VALUES ('rclu_volume_test', 'Volume Test');
INSERT INTO applications(id, project_id, identifier, name) VALUES
  ('app_volume_a', 'prj_volume_test', 'volume-a', 'Volume A'),
  ('app_volume_b', 'prj_volume_test', 'volume-b', 'Volume B');
INSERT INTO deployment_targets(id, project_id, application_id, name, cluster_id, namespace)
VALUES
  ('dtgt_volume_a', 'prj_volume_test', 'app_volume_a', 'Volume A', 'rclu_volume_test', 'project-volume-test'),
  ('dtgt_volume_b', 'prj_volume_test', 'app_volume_b', 'Volume B', 'rclu_volume_test', 'project-volume-test');`).Error; err != nil {
		t.Fatalf("seed project volume parents: %v", err)
	}

	for index := 0; index < 105; index++ {
		projectVolume := postgresTestProjectVolume(index)
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatalf("create project volume %d: %v", index, err)
		}
	}

	t.Run("default pagination never returns every row", func(t *testing.T) {
		result, err := service.ListProjectVolumes(context.Background(), "prj_volume_test", ProjectVolumeListOptions{})
		if err != nil {
			t.Fatalf("list project volumes: %v", err)
		}
		if len(result.Items) != DefaultPageSize || result.Page != 1 || result.PageSize != DefaultPageSize || result.Total != 105 || result.TotalPages != 6 {
			t.Fatalf("unexpected default page: items=%d result=%#v", len(result.Items), result)
		}
	})

	t.Run("first consumer provisioning volume is available for reservation", func(t *testing.T) {
		projectVolume := postgresTestProjectVolume(500)
		projectVolume.DisplayName = "first-consumer-volume"
		projectVolume.LifecycleState = model.ProjectVolumeLifecycleProvisioning
		projectVolume.PendingOperation = OperationProvision
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatalf("create first-consumer volume: %v", err)
		}
		result, err := service.ListProjectVolumes(context.Background(), projectVolume.ProjectID, ProjectVolumeListOptions{
			Page: 1, PageSize: 20, SortBy: "displayName", SortOrder: "asc",
			Search: "first-consumer-volume", Availability: model.ProjectVolumeAvailabilityAvailable,
		})
		if err != nil {
			t.Fatalf("list attachable provisioning volume: %v", err)
		}
		if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != projectVolume.ID || result.Items[0].Availability != model.ProjectVolumeAvailabilityAvailable {
			t.Fatalf("attachable provisioning result=%#v", result)
		}
	})

	t.Run("maintenance scan is bounded and stable", func(t *testing.T) {
		cutoff := time.Now().UTC().Add(-time.Hour)
		if err := db.Model(&model.ProjectVolume{}).
			Where("id <> ?", "pvol_000").
			Updates(map[string]any{"lifecycle_state": model.ProjectVolumeLifecycleProvisioning, "pending_operation": OperationProvision, "updated_at": cutoff.Add(-time.Hour)}).Error; err != nil {
			t.Fatalf("mark stale project volumes: %v", err)
		}
		items, err := service.ListStaleProjectVolumeOperations(context.Background(), MaintenanceScanOptions{Cutoff: cutoff, Limit: 1000})
		if err != nil {
			t.Fatalf("list stale project volume operations: %v", err)
		}
		if len(items) != MaxPageSize || items[0].ID != "pvol_001" || items[len(items)-1].ID != "pvol_100" {
			t.Fatalf("maintenance scan is not bounded/stable: count=%d first=%q last=%q", len(items), items[0].ID, items[len(items)-1].ID)
		}
	})

	t.Run("snapshot source constraint rejects incomplete rows", func(t *testing.T) {
		projectVolume := postgresTestProjectVolume(200)
		projectVolume.SourceKind = model.ProjectVolumeSourceSnapshotRestore
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err == nil {
			t.Fatal("snapshot-restore volume without source snapshot was accepted")
		}
		projectVolume = postgresTestProjectVolume(201)
		projectVolume.SourceSnapshotName = "snapshot-not-allowed"
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err == nil {
			t.Fatal("non-snapshot volume with a source snapshot was accepted")
		}
	})

	t.Run("exclusive mount reservation is serialized across targets", func(t *testing.T) {
		inputs := []ReserveMountInput{
			{
				ProjectID: "prj_volume_test", ApplicationID: "app_volume_a", DeploymentTargetID: "dtgt_volume_a",
				SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: "pvol_000", LogicalName: "data", MountPath: "/data",
			},
			{
				ProjectID: "prj_volume_test", ApplicationID: "app_volume_b", DeploymentTargetID: "dtgt_volume_b",
				SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: "pvol_000", LogicalName: "data", MountPath: "/data",
			},
		}
		start := make(chan struct{})
		errorsByCall := make(chan error, len(inputs))
		var wait sync.WaitGroup
		for _, input := range inputs {
			input := input
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				_, err := service.ReserveDeploymentVolumeMount(context.Background(), input)
				errorsByCall <- err
			}()
		}
		close(start)
		wait.Wait()
		close(errorsByCall)
		succeeded := 0
		conflicted := 0
		for err := range errorsByCall {
			switch {
			case err == nil:
				succeeded++
			case ErrorCode(err) == CodeBindingConflict:
				conflicted++
			default:
				t.Fatalf("unexpected reservation error: %v (code %q)", err, ErrorCode(err))
			}
		}
		if succeeded != 1 || conflicted != 1 {
			t.Fatalf("reservations: succeeded=%d conflicted=%d", succeeded, conflicted)
		}
	})

	t.Run("failed exclusive mount keeps ownership until unbound", func(t *testing.T) {
		var mounts []model.DeploymentVolumeMount
		if err := db.Where("project_volume_id = ?", "pvol_000").Find(&mounts).Error; err != nil {
			t.Fatalf("list exclusive mounts: %v", err)
		}
		if len(mounts) != 1 {
			t.Fatalf("exclusive mounts = %d, want 1", len(mounts))
		}
		current := mounts[0]
		if _, err := service.FailDeploymentVolumeMount(context.Background(), current.ProjectID, current.ID, CodeClusterUnavailable, "cluster unavailable"); err != nil {
			t.Fatalf("mark exclusive mount failed: %v", err)
		}
		otherTargetID := "dtgt_volume_a"
		otherApplicationID := "app_volume_a"
		if current.DeploymentTargetID == otherTargetID {
			otherTargetID = "dtgt_volume_b"
			otherApplicationID = "app_volume_b"
		}
		_, err := service.ReserveDeploymentVolumeMount(context.Background(), ReserveMountInput{
			ProjectID: "prj_volume_test", ApplicationID: otherApplicationID, DeploymentTargetID: otherTargetID,
			SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: "pvol_000", LogicalName: "data", MountPath: "/data",
		})
		if ErrorCode(err) != CodeBindingConflict {
			t.Fatalf("reserve while failed mount owns RWO error = %v (code %q)", err, ErrorCode(err))
		}
		if _, err := service.BeginDeploymentVolumeUnbind(context.Background(), current.ProjectID, current.ID); err != nil {
			t.Fatalf("begin failed mount unbind: %v", err)
		}
		if err := service.CompleteDeploymentVolumeUnbind(context.Background(), current.ProjectID, current.ID); err != nil {
			t.Fatalf("complete failed mount unbind: %v", err)
		}
		if _, err := service.ReserveDeploymentVolumeMount(context.Background(), ReserveMountInput{
			ProjectID: "prj_volume_test", ApplicationID: otherApplicationID, DeploymentTargetID: otherTargetID,
			SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: "pvol_000", LogicalName: "data", MountPath: "/data",
		}); err != nil {
			t.Fatalf("reserve exclusive mount after unbind: %v", err)
		}
	})

	t.Run("release pending mount restores the existing relation", func(t *testing.T) {
		var before model.DeploymentVolumeMount
		if err := db.Where("project_volume_id = ?", "pvol_000").First(&before).Error; err != nil {
			t.Fatalf("find exclusive mount before restore: %v", err)
		}
		if _, err := service.BeginDeploymentVolumeUnbind(context.Background(), before.ProjectID, before.ID); err != nil {
			t.Fatalf("begin mount release: %v", err)
		}
		restored, err := service.RestoreReleasePendingDeploymentVolumeMount(context.Background(), before.ProjectID, before.ID)
		if err != nil {
			t.Fatalf("restore release-pending mount: %v", err)
		}
		if restored.ID != before.ID || restored.ActivationState != model.DeploymentVolumeActivationReserved {
			t.Fatalf("restored mount = %#v, before = %#v", restored, before)
		}
		var count int64
		if err := db.Model(&model.DeploymentVolumeMount{}).Where("project_volume_id = ?", "pvol_000").Count(&count).Error; err != nil {
			t.Fatalf("count exclusive mounts after restore: %v", err)
		}
		if count != 1 {
			t.Fatalf("exclusive mount count after restore = %d, want 1", count)
		}
	})

	t.Run("database row lock honors caller cancellation", func(t *testing.T) {
		locker := db.Begin()
		if locker.Error != nil {
			t.Fatalf("begin locker transaction: %v", locker.Error)
		}
		defer locker.Rollback()
		if err := locker.Exec(`SELECT id FROM project_volumes WHERE id = 'pvol_000' FOR UPDATE`).Error; err != nil {
			t.Fatalf("lock project volume: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := repository.Transaction(ctx, func(transaction Repository) error {
			_, lockErr := transaction.LockProjectVolume(ctx, "prj_volume_test", "pvol_000")
			return lockErr
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked repository operation error = %v, want context deadline exceeded", err)
		}
	})

}

func TestGormRepositoryVolumeTransferStreamClaimCAS(t *testing.T) {
	db := openVolumeTestDB(t)
	installProjectVolumeTestSchema(t, db)
	repository := NewGormRepository(db)
	service := NewService(repository)
	if err := db.Exec(`
INSERT INTO projects(id, identifier, name) VALUES ('prj_transfer_cas', 'transfer-cas', 'Transfer CAS');
INSERT INTO runtime_clusters(id, name) VALUES ('rclu_transfer_cas', 'Transfer CAS');`).Error; err != nil {
		t.Fatalf("seed transfer parents: %v", err)
	}
	projectVolume := model.ProjectVolume{
		ID: "pvol_transfer_cas", ProjectID: "prj_transfer_cas", DisplayName: "transfer-cas",
		ClusterID: "rclu_transfer_cas", Namespace: "project-transfer-cas", ClaimName: "transfer-cas",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		LifecycleState: model.ProjectVolumeLifecycleReady, CapacityRequest: "1Gi", CapacityBytes: 1024 * 1024 * 1024,
		StorageClassName: "standard", AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: "usr_transfer_cas", Revision: 1,
	}
	if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
		t.Fatalf("create transfer volume: %v", err)
	}
	transfer := model.VolumeTransfer{
		ID: "vtx_transfer_cas", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
		Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
		ConsistencyMode: model.VolumeTransferConsistencyLive, State: model.VolumeTransferStateReady,
		ActorID: "usr_transfer_cas", ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
		t.Fatalf("create ready transfer: %v", err)
	}

	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := service.ClaimVolumeTransferStream(context.Background(), transfer.ProjectID, transfer.ID, transfer.Direction)
			errorsByCall <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByCall)
	succeeded, conflicted := 0, 0
	for err := range errorsByCall {
		switch {
		case err == nil:
			succeeded++
		case ErrorCode(err) == CodeTransferStateConflict:
			conflicted++
		default:
			t.Fatalf("unexpected stream claim error: %v (code %q)", err, ErrorCode(err))
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("stream claims: succeeded=%d conflicted=%d", succeeded, conflicted)
	}

	completed, err := service.CompleteVolumeTransferStream(context.Background(), transfer.ProjectID, transfer.ID, TransferCompletion{
		ExpectedState: model.VolumeTransferStateStreaming, TransferredBytes: 42,
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("complete claimed stream: %v", err)
	}
	if completed.State != model.VolumeTransferStateSucceeded || completed.TransferredBytes != 42 {
		t.Fatalf("completed transfer = %#v", completed)
	}

	staleUpdatedAt := time.Now().UTC().Add(-time.Hour)
	stale := model.VolumeTransfer{
		ID: "vtx_stale_cas", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
		Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
		ConsistencyMode: model.VolumeTransferConsistencyLive, State: model.VolumeTransferStateStreaming,
		ActorID: "usr_transfer_cas", ExpiresAt: time.Now().UTC().Add(time.Hour), UpdatedAt: staleUpdatedAt,
	}
	if err := repository.CreateVolumeTransfer(context.Background(), &stale); err != nil {
		t.Fatalf("create stale streaming transfer: %v", err)
	}
	if err := db.Model(&model.VolumeTransfer{}).Where("id = ?", stale.ID).Update("updated_at", staleUpdatedAt).Error; err != nil {
		t.Fatalf("age stale transfer: %v", err)
	}
	start = make(chan struct{})
	errorsByCall = make(chan error, 2)
	wait = sync.WaitGroup{}
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, err := service.UpdateVolumeTransferProgress(context.Background(), stale.ProjectID, stale.ID, TransferProgress{Phase: "streaming"})
		errorsByCall <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := service.FailStaleVolumeTransfer(context.Background(), stale.ProjectID, stale.ID, time.Now().UTC().Add(-time.Minute), CodeTransferJobFailed, "stale stream")
		errorsByCall <- err
	}()
	close(start)
	wait.Wait()
	close(errorsByCall)
	succeeded, conflicted = 0, 0
	for err := range errorsByCall {
		switch {
		case err == nil:
			succeeded++
		case ErrorCode(err) == CodeTransferStateConflict || ErrorCode(err) == CodeTransferProgressInvalid:
			conflicted++
		default:
			t.Fatalf("unexpected heartbeat/stale CAS error: %v (code %q)", err, ErrorCode(err))
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("heartbeat/stale CAS: succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}
