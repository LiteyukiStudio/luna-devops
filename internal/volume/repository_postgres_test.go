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
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestGormRepositoryProjectVolumeContract(t *testing.T) {
	db := openVolumeTestDB(t)
	installProjectVolumeTestSchema(t, db)
	repository := NewGormRepository(db)
	service := NewService(repository)

	if err := db.Exec(`
INSERT INTO projects(id) VALUES ('prj_volume_test');
INSERT INTO runtime_clusters(id) VALUES ('rclu_volume_test');
INSERT INTO applications(id) VALUES ('app_volume_a'), ('app_volume_b');
INSERT INTO deployment_targets(id, project_id, application_id, cluster_id, namespace)
VALUES
  ('dtgt_volume_a', 'prj_volume_test', 'app_volume_a', 'rclu_volume_test', 'project-volume-test'),
  ('dtgt_volume_b', 'prj_volume_test', 'app_volume_b', 'rclu_volume_test', 'project-volume-test');`).Error; err != nil {
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

	t.Run("same transfer offset uses a durable lease without holding the database lock", func(t *testing.T) {
		transfer := model.VolumeTransfer{
			ID: "vtx_concurrent_part", ProjectID: "prj_volume_test", ProjectVolumeID: "pvol_000",
			Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateUploading,
			ObjectKey: "transfers/concurrent-part", MultipartUploadID: "upload-concurrent-part",
			ExpectedBytes: 4, ActorID: "usr_volume_test", ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("seed concurrent volume transfer: %v", err)
		}
		firstEntered := make(chan struct{})
		releaseFirst := make(chan struct{})
		firstResult := make(chan error, 1)
		go func() {
			_, _, err := service.WriteVolumeTransferPart(context.Background(), transfer.ProjectID, transfer.ID, model.VolumeTransferPart{
				Offset: 0, Size: 4, SHA256: strings.Repeat("a", 64),
			}, func(context.Context, int) (string, error) {
				close(firstEntered)
				<-releaseFirst
				return "etag-first", nil
			})
			firstResult <- err
		}()
		<-firstEntered
		queryCtx, cancelQuery := context.WithTimeout(context.Background(), time.Second)
		defer cancelQuery()
		if _, err := repository.GetVolumeTransfer(queryCtx, transfer.ProjectID, transfer.ID); err != nil {
			t.Fatalf("query transfer while S3 writer is blocked: %v", err)
		}

		secondWriterCalled := make(chan struct{}, 1)
		secondResult := make(chan error, 1)
		go func() {
			_, _, err := service.WriteVolumeTransferPart(context.Background(), transfer.ProjectID, transfer.ID, model.VolumeTransferPart{
				Offset: 0, Size: 4, SHA256: strings.Repeat("b", 64),
			}, func(context.Context, int) (string, error) {
				secondWriterCalled <- struct{}{}
				return "etag-second", nil
			})
			secondResult <- err
		}()
		select {
		case <-secondWriterCalled:
			t.Fatal("conflicting second writer reached the object store")
		case err := <-secondResult:
			if ErrorCode(err) != CodeTransferPartConflict {
				t.Fatalf("second transfer part code=%q err=%v", ErrorCode(err), err)
			}
		case <-time.After(time.Second):
			t.Fatal("conflicting second write waited for blocked object-store I/O")
		}
		close(releaseFirst)
		if err := <-firstResult; err != nil {
			t.Fatalf("first transfer part write: %v", err)
		}
		select {
		case <-secondWriterCalled:
			t.Fatal("conflicting second writer reached the object store")
		default:
		}
	})

	t.Run("completion report stays running and import finalization is atomic", func(t *testing.T) {
		checksum := strings.Repeat("a", 64)
		projectVolume := postgresTestProjectVolume(301)
		projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
		projectVolume.LifecycleState = model.ProjectVolumeLifecycleProvisioning
		projectVolume.PendingOperation = OperationImport
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatalf("seed atomic import volume: %v", err)
		}
		transfer := model.VolumeTransfer{
			ID: "vtx_atomic_import", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateRunning,
			ObjectKey: "transfers/atomic-import", ExpectedBytes: 4096, SHA256: checksum,
			ActorID: "usr_volume_test", ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("seed atomic import transfer: %v", err)
		}

		reported, err := service.ReportVolumeTransferCompletion(context.Background(), projectVolume.ProjectID, transfer.ID, TransferCompletion{
			ExpectedState: model.VolumeTransferStateRunning, TransferredBytes: transfer.ExpectedBytes, SHA256: checksum,
		})
		if err != nil || reported.State != model.VolumeTransferStateRunning || reported.CompletionReportedAt == nil || reported.FinishedAt != nil {
			t.Fatalf("completion report exposed success: transfer=%#v err=%v", reported, err)
		}
		marked, err := service.MarkVolumeTransferJobSucceeded(context.Background(), projectVolume.ProjectID, transfer.ID)
		if err != nil || marked.State != model.VolumeTransferStateRunning || marked.JobSucceededAt == nil {
			t.Fatalf("mark Job succeeded transfer=%#v err=%v", marked, err)
		}
		finalized, err := service.FinalizeVolumeTransferExecution(context.Background(), projectVolume.ProjectID, transfer.ID)
		if err != nil || finalized.State != model.VolumeTransferStateSucceeded || finalized.FinishedAt == nil {
			t.Fatalf("finalize import transfer=%#v err=%v", finalized, err)
		}
		ready, err := repository.GetProjectVolume(context.Background(), projectVolume.ProjectID, projectVolume.ID)
		if err != nil || ready.LifecycleState != model.ProjectVolumeLifecycleReady || ready.PendingOperation != "" {
			t.Fatalf("atomic import volume=%#v err=%v", ready, err)
		}
	})

	t.Run("failed import finalization rolls back transfer success", func(t *testing.T) {
		checksum := strings.Repeat("b", 64)
		projectVolume := postgresTestProjectVolume(302)
		projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
		projectVolume.LifecycleState = model.ProjectVolumeLifecycleError
		projectVolume.PendingOperation = OperationImport
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatalf("seed rollback import volume: %v", err)
		}
		now := time.Now().UTC()
		transfer := model.VolumeTransfer{
			ID: "vtx_atomic_rollback", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateRunning,
			ObjectKey: "transfers/atomic-rollback", ExpectedBytes: 4096, TransferredBytes: 4096, SHA256: checksum,
			CompletionReportedAt: &now, JobSucceededAt: &now,
			ActorID: "usr_volume_test", ExpiresAt: now.Add(time.Hour),
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("seed rollback transfer: %v", err)
		}
		if _, err := service.FinalizeVolumeTransferExecution(context.Background(), projectVolume.ProjectID, transfer.ID); ErrorCode(err) != CodeStateConflict {
			t.Fatalf("invalid import finalization code=%q err=%v", ErrorCode(err), err)
		}
		persisted, err := repository.GetVolumeTransfer(context.Background(), projectVolume.ProjectID, transfer.ID)
		if err != nil || persisted.State != model.VolumeTransferStateRunning || persisted.FinishedAt != nil {
			t.Fatalf("failed finalization persisted transfer=%#v err=%v", persisted, err)
		}
	})

	t.Run("import failure is atomic across transfer and project volume", func(t *testing.T) {
		projectVolume := postgresTestProjectVolume(303)
		projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
		projectVolume.LifecycleState = model.ProjectVolumeLifecycleReady
		projectVolume.PendingOperation = ""
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatalf("seed atomic failure volume: %v", err)
		}
		transfer := model.VolumeTransfer{
			ID: "vtx_atomic_failure", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateRunning,
			ObjectKey: "transfers/atomic-failure", ActorID: "usr_volume_test", ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("seed atomic failure transfer: %v", err)
		}
		if _, err := service.FailVolumeTransferExecution(context.Background(), projectVolume.ProjectID, transfer.ID,
			CodeTransferJobFailed, "provider failed"); ErrorCode(err) != CodeStateConflict {
			t.Fatalf("invalid import failure code=%q err=%v", ErrorCode(err), err)
		}
		persisted, err := repository.GetVolumeTransfer(context.Background(), projectVolume.ProjectID, transfer.ID)
		if err != nil || persisted.State != model.VolumeTransferStateRunning || persisted.FinishedAt != nil {
			t.Fatalf("failed atomic failure changed transfer=%#v err=%v", persisted, err)
		}
		if err := db.Model(&model.ProjectVolume{}).Where("id = ?", projectVolume.ID).Updates(map[string]any{
			"lifecycle_state":   model.ProjectVolumeLifecycleProvisioning,
			"pending_operation": OperationImport,
		}).Error; err != nil {
			t.Fatalf("restore provisional import state: %v", err)
		}
		failed, err := service.FailVolumeTransferExecution(context.Background(), projectVolume.ProjectID, transfer.ID,
			CodeTransferJobFailed, "provider failed")
		if err != nil || failed.State != model.VolumeTransferStateFailed || failed.FinishedAt == nil {
			t.Fatalf("atomic import failure=%#v err=%v", failed, err)
		}
		errorVolume, err := repository.GetProjectVolume(context.Background(), projectVolume.ProjectID, projectVolume.ID)
		if err != nil || errorVolume.LifecycleState != model.ProjectVolumeLifecycleError || errorVolume.PendingOperation != OperationImport || errorVolume.LastErrorCode != CodeTransferJobFailed {
			t.Fatalf("atomic import error volume=%#v err=%v", errorVolume, err)
		}
		if replay, replayErr := service.FailVolumeTransferExecution(context.Background(), projectVolume.ProjectID, transfer.ID,
			CodeTransferJobFailed, "provider failed"); replayErr != nil || replay.State != model.VolumeTransferStateFailed {
			t.Fatalf("atomic failure replay=%#v err=%v", replay, replayErr)
		}
	})

	t.Run("terminal cleanup marker blocks deletion and object expiry until durable", func(t *testing.T) {
		projectVolume := postgresTestProjectVolume(304)
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatalf("seed cleanup blocker volume: %v", err)
		}
		now := time.Now().UTC()
		transfer := model.VolumeTransfer{
			ID: "vtx_cleanup_pending", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateSucceeded,
			ObjectKey: "transfers/cleanup-pending", ActorID: "usr_volume_test", ExpiresAt: now.Add(-time.Hour),
			CompletionReportedAt: &now, JobSucceededAt: &now, FinishedAt: &now,
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("seed cleanup-pending transfer: %v", err)
		}
		if count, err := repository.CountActiveTransfers(context.Background(), projectVolume.ID); err != nil || count != 1 {
			t.Fatalf("cleanup-pending blocker count=%d err=%v", count, err)
		}
		if items, err := repository.ListExpiredVolumeTransferObjects(context.Background(), now, 100); err != nil || containsVolumeTransfer(items, transfer.ID) {
			t.Fatalf("cleanup-pending object became eligible items=%#v err=%v", items, err)
		}
		if items, err := repository.ListStaleVolumeTransfers(context.Background(), now.Add(time.Hour), 100); err != nil || !containsVolumeTransfer(items, transfer.ID) {
			t.Fatalf("cleanup-pending terminal transfer not reconciled items=%#v err=%v", items, err)
		}
		if deleted, err := repository.MarkVolumeTransferObjectDeleted(context.Background(), transfer.ProjectID, transfer.ID, now); err != nil || deleted {
			t.Fatalf("cleanup-pending object deletion deleted=%t err=%v", deleted, err)
		}
		marked, err := service.MarkVolumeTransferExecutionCleanupCompleted(context.Background(), projectVolume.ProjectID, transfer.ID)
		if err != nil || marked.ExecutionCleanupCompletedAt == nil {
			t.Fatalf("mark execution cleanup transfer=%#v err=%v", marked, err)
		}
		if count, err := repository.CountActiveTransfers(context.Background(), projectVolume.ID); err != nil || count != 0 {
			t.Fatalf("completed cleanup blocker count=%d err=%v", count, err)
		}
		if items, err := repository.ListExpiredVolumeTransferObjects(context.Background(), now, 100); err != nil || !containsVolumeTransfer(items, transfer.ID) {
			t.Fatalf("cleaned object not eligible items=%#v err=%v", items, err)
		}
		if items, err := repository.ListStaleVolumeTransfers(context.Background(), now.Add(time.Hour), 100); err != nil || containsVolumeTransfer(items, transfer.ID) {
			t.Fatalf("completed terminal cleanup remained stale items=%#v err=%v", items, err)
		}
		if replay, replayErr := service.MarkVolumeTransferExecutionCleanupCompleted(context.Background(), projectVolume.ProjectID, transfer.ID); replayErr != nil || replay.ExecutionCleanupCompletedAt == nil {
			t.Fatalf("cleanup marker replay=%#v err=%v", replay, replayErr)
		}
	})

	t.Run("execution lease fences concurrent creation and expired lease takeover", func(t *testing.T) {
		transfer := model.VolumeTransfer{
			ID: "vtx_execution_lease", ProjectID: "prj_volume_test", ProjectVolumeID: "pvol_001",
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateQueued,
			ObjectKey: "transfers/execution-lease", ActorID: "usr_volume_test", ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("seed execution lease transfer: %v", err)
		}
		first, err := service.ClaimVolumeTransferExecution(context.Background(), transfer.ProjectID, transfer.ID,
			model.VolumeTransferStateQueued, "worker-attempt-one", time.Now().UTC().Add(time.Minute))
		if err != nil || first.ExecutionGeneration != 1 || first.CreationLeaseOwner != "worker-attempt-one" {
			t.Fatalf("first execution claim=%#v err=%v", first, err)
		}
		renewed, err := service.RenewVolumeTransferExecutionLease(context.Background(), transfer.ProjectID, transfer.ID,
			"worker-attempt-one", first.ExecutionGeneration, time.Now().UTC().Add(2*time.Minute))
		if err != nil || renewed.CreationLeaseExpiresAt == nil || !renewed.CreationLeaseExpiresAt.After(*first.CreationLeaseExpiresAt) {
			t.Fatalf("execution lease renewal=%#v err=%v", renewed, err)
		}
		if _, err := service.ClaimVolumeTransferExecution(context.Background(), transfer.ProjectID, transfer.ID,
			model.VolumeTransferStateQueued, "worker-attempt-two", time.Now().UTC().Add(time.Minute)); ErrorCode(err) != CodeTransferStateConflict {
			t.Fatalf("active lease contender code=%q err=%v", ErrorCode(err), err)
		}
		if err := db.Model(&model.VolumeTransfer{}).Where("id = ?", transfer.ID).
			Update("creation_lease_expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
			t.Fatalf("expire crashed execution lease: %v", err)
		}
		second, err := service.ClaimVolumeTransferExecution(context.Background(), transfer.ProjectID, transfer.ID,
			model.VolumeTransferStateQueued, "worker-attempt-two", time.Now().UTC().Add(time.Minute))
		if err != nil || second.ExecutionGeneration != 2 || second.CreationLeaseOwner != "worker-attempt-two" {
			t.Fatalf("expired execution takeover=%#v err=%v", second, err)
		}
		if _, err := service.RenewVolumeTransferExecutionLease(context.Background(), transfer.ProjectID, transfer.ID,
			"worker-attempt-one", first.ExecutionGeneration, time.Now().UTC().Add(time.Minute)); ErrorCode(err) != CodeTransferStateConflict {
			t.Fatalf("stale execution renewal code=%q err=%v", ErrorCode(err), err)
		}
		checksum := strings.Repeat("d", 64)
		if _, err := service.PrepareVolumeTransferExecution(context.Background(), transfer.ProjectID, transfer.ID,
			model.VolumeTransferStateQueued, "worker-attempt-one", first.ExecutionGeneration, checksum, time.Now().UTC().Add(time.Hour)); ErrorCode(err) != CodeTransferStateConflict {
			t.Fatalf("stale generation prepare code=%q err=%v", ErrorCode(err), err)
		}
		prepared, err := service.PrepareVolumeTransferExecution(context.Background(), transfer.ProjectID, transfer.ID,
			model.VolumeTransferStateQueued, "worker-attempt-two", second.ExecutionGeneration, checksum, time.Now().UTC().Add(time.Hour))
		if err != nil || prepared.State != model.VolumeTransferStateRunning || prepared.CallbackTokenHash != checksum || prepared.ExecutionGeneration != 2 {
			t.Fatalf("takeover prepare=%#v err=%v", prepared, err)
		}
		confirmed, err := service.ConfirmVolumeTransferJobCreated(context.Background(), transfer.ProjectID, transfer.ID, prepared.ExecutionGeneration)
		if err != nil || confirmed.JobCreatedAt == nil || confirmed.CreationLeaseOwner != "" || confirmed.CreationLeaseExpiresAt != nil {
			t.Fatalf("Job creation confirmation=%#v err=%v", confirmed, err)
		}
		if replay, replayErr := service.ConfirmVolumeTransferJobCreated(context.Background(), transfer.ProjectID, transfer.ID, prepared.ExecutionGeneration); replayErr != nil || replay.JobCreatedAt == nil {
			t.Fatalf("Job creation confirmation replay=%#v err=%v", replay, replayErr)
		}
		failed, err := service.FailVolumeTransferExecution(context.Background(), transfer.ProjectID, transfer.ID,
			CodeTransferJobFailed, "test execution complete")
		if err != nil || failed.State != model.VolumeTransferStateFailed {
			t.Fatalf("terminalize execution lease fixture=%#v err=%v", failed, err)
		}
		if _, err := service.MarkVolumeTransferExecutionCleanupCompleted(context.Background(), transfer.ProjectID, transfer.ID); err != nil {
			t.Fatalf("clean execution lease fixture: %v", err)
		}
	})

	t.Run("database rejects success without callback and Job evidence", func(t *testing.T) {
		transfer := model.VolumeTransfer{
			ID: "vtx_missing_success_evidence", ProjectID: "prj_volume_test", ProjectVolumeID: "pvol_001",
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateRunning,
			ObjectKey: "transfers/missing-success-evidence", ActorID: "usr_volume_test", ExpiresAt: time.Now().UTC().Add(time.Hour),
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("seed transfer without success evidence: %v", err)
		}
		if err := db.Model(&model.VolumeTransfer{}).Where("id = ?", transfer.ID).
			Update("state", model.VolumeTransferStateSucceeded).Error; err == nil {
			t.Fatal("database accepted succeeded transfer without completion and Job evidence")
		}
		persisted, err := repository.GetVolumeTransfer(context.Background(), transfer.ProjectID, transfer.ID)
		if err != nil || persisted.State != model.VolumeTransferStateRunning {
			t.Fatalf("constraint failure changed transfer=%#v err=%v", persisted, err)
		}
	})

	t.Run("retry and cleanup atomically select one object owner", func(t *testing.T) {
		projectVolume := postgresTestProjectVolume(305)
		projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
		projectVolume.LifecycleState = model.ProjectVolumeLifecycleError
		projectVolume.PendingOperation = OperationImport
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatalf("seed retry project volume: %v", err)
		}
		now := time.Now().UTC()
		original := model.VolumeTransfer{
			ID: "vtx_object_owner_race", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateFailed,
			ObjectKey: "transfers/object-owner-race", ObjectOwned: true, SourceFilename: "archive.tar.gz",
			ExpectedBytes: 4096, SHA256: strings.Repeat("d", 64), ActorID: "usr_volume_test",
			ExpiresAt: now.Add(time.Hour), ExecutionCleanupCompletedAt: &now,
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &original); err != nil {
			t.Fatalf("seed retry transfer: %v", err)
		}
		serviceWithDispatcher := NewService(repository, &dispatcherStub{})
		input := CreateVolumeTransferInput{
			ProjectID: original.ProjectID, ProjectVolumeID: original.ProjectVolumeID,
			Direction: original.Direction, Format: original.Format, ConsistencyMode: original.ConsistencyMode,
			ObjectKey: original.ObjectKey, SourceFilename: original.SourceFilename,
			ExpectedBytes: original.ExpectedBytes, SHA256: original.SHA256, ActorID: original.ActorID,
			ExpiresAt: now.Add(2 * time.Hour), IdempotencyKey: "retry-owner-race", VerifiedObject: true,
		}
		start := make(chan struct{})
		cleanupResult := make(chan model.VolumeTransfer, 1)
		cleanupErr := make(chan error, 1)
		retryResult := make(chan model.VolumeTransfer, 1)
		retryErr := make(chan error, 1)
		go func() {
			<-start
			claimed, err := serviceWithDispatcher.ClaimVolumeTransferObjectCleanup(context.Background(), original.ProjectID, original.ID,
				"cleanup-lease-race", time.Now().UTC().Add(time.Minute))
			cleanupResult <- claimed
			cleanupErr <- err
		}()
		go func() {
			<-start
			retried, err := serviceWithDispatcher.RetryVolumeImportTransfer(context.Background(), original.ID, input)
			retryResult <- retried
			retryErr <- err
		}()
		close(start)
		claimed, claimErr := <-cleanupResult, <-cleanupErr
		retried, retryCallErr := <-retryResult, <-retryErr
		cleanupWon := claimErr == nil && claimed.ObjectOwned && claimed.ObjectCleanupStartedAt != nil
		retryWon := retryCallErr == nil && retried.ID != "" && retried.ObjectOwned
		if cleanupWon == retryWon {
			t.Fatalf("cleanupWon=%t retryWon=%t claim=%#v claimErr=%v retry=%#v retryErr=%v", cleanupWon, retryWon, claimed, claimErr, retried, retryCallErr)
		}
		var ownerCount int64
		if err := db.Model(&model.VolumeTransfer{}).Where("object_key = ? AND object_owned = true", original.ObjectKey).Count(&ownerCount).Error; err != nil || ownerCount != 1 {
			t.Fatalf("object owner count=%d err=%v", ownerCount, err)
		}
		if retryWon {
			persistedOriginal, err := repository.GetVolumeTransfer(context.Background(), original.ProjectID, original.ID)
			if err != nil || persistedOriginal.ObjectOwned || persistedOriginal.ObjectCleanupStartedAt != nil {
				t.Fatalf("original ownership after retry=%#v err=%v", persistedOriginal, err)
			}
		}
	})

	t.Run("terminal retry replay has no project volume side effects", func(t *testing.T) {
		projectVolume := postgresTestProjectVolume(307)
		projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
		projectVolume.LifecycleState = model.ProjectVolumeLifecycleError
		projectVolume.PendingOperation = OperationImport
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		original := model.VolumeTransfer{
			ID: "vtx_terminal_retry_original", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateFailed,
			ObjectKey: "transfers/terminal-retry-replay", ObjectOwned: true, SourceFilename: "archive.tar.gz",
			ExpectedBytes: 4096, SHA256: strings.Repeat("f", 64), ActorID: "usr_volume_test",
			ExpiresAt: now.Add(time.Hour), ExecutionCleanupCompletedAt: &now,
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &original); err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&model.VolumeTransfer{}).Where("id = ?", original.ID).Update("object_owned", false).Error; err != nil {
			t.Fatal(err)
		}
		input := CreateVolumeTransferInput{
			ProjectID: original.ProjectID, ProjectVolumeID: original.ProjectVolumeID,
			Direction: original.Direction, Format: original.Format, ConsistencyMode: original.ConsistencyMode,
			ObjectKey: original.ObjectKey, SourceFilename: original.SourceFilename,
			ExpectedBytes: original.ExpectedBytes, SHA256: original.SHA256, ActorID: original.ActorID,
			ExpiresAt: now.Add(2 * time.Hour), IdempotencyKey: "terminal-retry-replay", VerifiedObject: true,
		}
		existing := original
		existing.ID = idempotentVolumeTransferID(input)
		existing.ObjectOwned = true
		existing.ExpiresAt = now.Add(30 * time.Minute)
		if err := repository.CreateVolumeTransfer(context.Background(), &existing); err != nil {
			t.Fatal(err)
		}
		dispatcher := &dispatcherStub{}
		serviceWithDispatcher := NewService(repository, dispatcher)
		replayed, err := serviceWithDispatcher.RetryVolumeImportTransfer(context.Background(), original.ID, input)
		if err != nil || replayed.ID != existing.ID || !replayed.ExpiresAt.Equal(existing.ExpiresAt) {
			t.Fatalf("terminal retry replay=%#v err=%v", replayed, err)
		}
		persistedVolume, err := repository.GetProjectVolume(context.Background(), projectVolume.ProjectID, projectVolume.ID)
		if err != nil || persistedVolume.LifecycleState != model.ProjectVolumeLifecycleError || persistedVolume.PendingOperation != OperationImport {
			t.Fatalf("terminal retry replay changed project volume=%#v err=%v", persistedVolume, err)
		}
		if dispatcher.operation.TransferID != "" {
			t.Fatalf("terminal retry replay dispatched operation=%#v", dispatcher.operation)
		}
		var transferCount int64
		if err := db.Model(&model.VolumeTransfer{}).Where("object_key = ?", original.ObjectKey).Count(&transferCount).Error; err != nil || transferCount != 2 {
			t.Fatalf("terminal retry replay transfer count=%d err=%v", transferCount, err)
		}
	})

	t.Run("cleanup tombstone permanently fences retry after a lost delete response", func(t *testing.T) {
		projectVolume := postgresTestProjectVolume(306)
		projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
		projectVolume.LifecycleState = model.ProjectVolumeLifecycleError
		projectVolume.PendingOperation = OperationImport
		if err := repository.CreateProjectVolume(context.Background(), &projectVolume); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		original := model.VolumeTransfer{
			ID: "vtx_object_delete_lost", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
			ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateFailed,
			ObjectKey: "transfers/object-delete-lost", ObjectOwned: true, SourceFilename: "archive.tar.gz",
			ExpectedBytes: 4096, SHA256: strings.Repeat("e", 64), ActorID: "usr_volume_test",
			ExpiresAt: now.Add(time.Hour), ExecutionCleanupCompletedAt: &now,
		}
		if err := repository.CreateVolumeTransfer(context.Background(), &original); err != nil {
			t.Fatal(err)
		}
		serviceWithDispatcher := NewService(repository, &dispatcherStub{})
		claimed, err := serviceWithDispatcher.ClaimVolumeTransferObjectCleanup(context.Background(), original.ProjectID, original.ID,
			"cleanup-lease-lost", now.Add(time.Minute))
		if err != nil || claimed.ObjectCleanupStartedAt == nil {
			t.Fatalf("initial cleanup claim=%#v err=%v", claimed, err)
		}
		if err := db.Model(&model.VolumeTransfer{}).Where("id = ?", original.ID).
			Update("object_cleanup_lease_expires_at", now.Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		_, err = serviceWithDispatcher.RetryVolumeImportTransfer(context.Background(), original.ID, CreateVolumeTransferInput{
			ProjectID: original.ProjectID, ProjectVolumeID: original.ProjectVolumeID,
			Direction: original.Direction, Format: original.Format, ConsistencyMode: original.ConsistencyMode,
			ObjectKey: original.ObjectKey, SourceFilename: original.SourceFilename,
			ExpectedBytes: original.ExpectedBytes, SHA256: original.SHA256, ActorID: original.ActorID,
			ExpiresAt: now.Add(2 * time.Hour), IdempotencyKey: "retry-after-lost-delete", VerifiedObject: true,
		})
		if ErrorCode(err) != CodeTransferExpired {
			t.Fatalf("retry after cleanup tombstone code=%q err=%v", ErrorCode(err), err)
		}
		reclaimed, err := serviceWithDispatcher.ClaimVolumeTransferObjectCleanup(context.Background(), original.ProjectID, original.ID,
			"cleanup-lease-recover", now.Add(2*time.Minute))
		if err != nil || reclaimed.ObjectCleanupLeaseToken != "cleanup-lease-recover" || reclaimed.ObjectCleanupStartedAt == nil {
			t.Fatalf("stale cleanup reclaim=%#v err=%v", reclaimed, err)
		}
	})
}

func containsVolumeTransfer(items []model.VolumeTransfer, transferID string) bool {
	for _, item := range items {
		if item.ID == transferID {
			return true
		}
	}
	return false
}

func postgresTestProjectVolume(index int) model.ProjectVolume {
	return model.ProjectVolume{
		ID: fmt.Sprintf("pvol_%03d", index), ProjectID: "prj_volume_test", DisplayName: fmt.Sprintf("volume-%03d", index),
		ClusterID: "rclu_volume_test", Namespace: "project-volume-test", ClaimName: fmt.Sprintf("claim-%03d", index),
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		LifecycleState: model.ProjectVolumeLifecycleReady, PendingOperation: "", CapacityRequest: "1Gi", CapacityBytes: 1024 * 1024 * 1024,
		StorageClassName: "standard", AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: "usr_volume_test", Revision: 1,
	}
}

func installProjectVolumeTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	parents := `
CREATE TABLE projects (id text PRIMARY KEY);
CREATE TABLE runtime_clusters (id text PRIMARY KEY);
CREATE TABLE applications (id text PRIMARY KEY);
CREATE TABLE deployment_targets (
  id text PRIMARY KEY,
  project_id text NOT NULL,
  application_id text NOT NULL,
  cluster_id text NOT NULL,
  namespace text NOT NULL,
  deleted_at timestamptz
);`
	schemaSQL := parents +
		readVolumeMigration(t, "000066_project_volume_center.up.sql") +
		readVolumeMigration(t, "000069_volume_transfer_block_manifest.up.sql") +
		readVolumeMigration(t, "000070_volume_transfer_completion_state.up.sql") +
		readVolumeMigration(t, "000071_volume_transfer_part_leases.up.sql") +
		readVolumeMigration(t, "000072_volume_transfer_execution_leases.up.sql") +
		readVolumeMigration(t, "000073_volume_transfer_object_ownership.up.sql")
	for _, statement := range splitVolumeMigrationStatements(stripVolumeMigrationLineComments(schemaSQL)) {
		if statement = strings.TrimSpace(statement); statement != "" {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatalf("install project volume schema statement %q: %v", statement, err)
			}
		}
	}
}

func stripVolumeMigrationLineComments(sql string) string {
	lines := strings.Split(sql, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

func splitVolumeMigrationStatements(sql string) []string {
	statements := make([]string, 0)
	var current strings.Builder
	var singleQuoted, doubleQuoted bool
	for index := 0; index < len(sql); index++ {
		character := sql[index]
		switch character {
		case '\'':
			current.WriteByte(character)
			if !doubleQuoted {
				if singleQuoted && index+1 < len(sql) && sql[index+1] == '\'' {
					current.WriteByte(sql[index+1])
					index++
					continue
				}
				singleQuoted = !singleQuoted
			}
		case '"':
			current.WriteByte(character)
			if !singleQuoted {
				doubleQuoted = !doubleQuoted
			}
		case ';':
			if singleQuoted || doubleQuoted {
				current.WriteByte(character)
				continue
			}
			statements = append(statements, current.String())
			current.Reset()
		default:
			current.WriteByte(character)
		}
	}
	if trailing := strings.TrimSpace(current.String()); trailing != "" {
		statements = append(statements, trailing)
	}
	return statements
}

func openVolumeTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{SchemaPrefix: "volume_test"})
}
