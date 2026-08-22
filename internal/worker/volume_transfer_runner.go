package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
)

const (
	volumeTransferPodPollInterval  = 2 * time.Second
	volumeTransferCleanupDeadline  = 45 * time.Second
	volumeTransferCreationLeaseTTL = 2 * time.Minute
	volumeTransferPodReadyDeadline = 15 * time.Minute
)

func (r *Runner) handleVolumeImport(ctx context.Context, task *asynq.Task) error {
	return r.handleVolumeTransfer(ctx, task, model.VolumeTransferDirectionImport)
}

func (r *Runner) handleVolumeExport(ctx context.Context, task *asynq.Task) error {
	return r.handleVolumeTransfer(ctx, task, model.VolumeTransferDirectionExport)
}

func (r *Runner) handleVolumeTransfer(ctx context.Context, task *asynq.Task, expectedDirection string) error {
	var payload tasks.VolumeTransferPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return skipVolumeTask(volume.CodeInvalidInput)
	}
	payload.ProjectID = strings.TrimSpace(payload.ProjectID)
	payload.VolumeID = strings.TrimSpace(payload.VolumeID)
	payload.TransferID = strings.TrimSpace(payload.TransferID)
	if payload.ProjectID == "" || payload.VolumeID == "" || payload.TransferID == "" {
		return skipVolumeTask(volume.CodeInvalidInput)
	}
	service, err := r.projectVolumeService()
	if err != nil {
		return err
	}
	transfer, err := service.GetVolumeTransfer(ctx, payload.ProjectID, payload.TransferID)
	if err != nil {
		if volume.ErrorCode(err) == volume.CodeTransferNotFound {
			return nil
		}
		return safeVolumeTaskError(err)
	}
	if transfer.ProjectVolumeID != payload.VolumeID || transfer.Direction != expectedDirection {
		return skipVolumeTask(volume.CodeTransferStateConflict)
	}
	projectVolume, err := service.GetProjectVolume(ctx, payload.ProjectID, payload.VolumeID)
	if err != nil {
		if volume.ErrorCode(err) == volume.CodeNotFound && transfer.State == model.VolumeTransferStateCancelled {
			return nil
		}
		return safeVolumeTaskError(err)
	}
	provider, err := r.volumeTransferProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		if volume.IsVolumeTransferTerminal(transfer.State) {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
		return r.failVolumeTransfer(ctx, service, transfer, volume.CodeClusterUnavailable, err)
	}

	switch transfer.State {
	case model.VolumeTransferStateReady, model.VolumeTransferStateStreaming:
		return nil
	case model.VolumeTransferStateSucceeded, model.VolumeTransferStateFailed, model.VolumeTransferStateExpired:
		_, err = r.completeVolumeTransferExecutionCleanup(ctx, service, provider, volumeTransferSnapshotCleanupFor(transfer), projectVolume, transfer)
		return safeVolumeCleanupResult(err)
	case model.VolumeTransferStateCancelled:
		return r.cleanupCancelledTransferWithProvider(ctx, service, provider, projectVolume, transfer)
	case model.VolumeTransferStatePreparing:
	default:
		return safeVolumeTaskCode(volume.CodeTransferStateConflict)
	}

	leaseOwner := id.New("vlease")
	claimed, err := service.ClaimVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID,
		model.VolumeTransferStatePreparing, leaseOwner, time.Now().UTC().Add(volumeTransferCreationLeaseTTL))
	if err != nil {
		return retryVolumeTransferExecutionClaim(err)
	}
	transfer = claimed
	preparationCtx, stopLeaseHeartbeat := maintainVolumeTransferExecutionLease(ctx, service, transfer, leaseOwner, volumeTransferCreationLeaseTTL/3)
	claimName, snapshotCleanup, err := r.prepareVolumeTransferClaim(preparationCtx, projectVolume, transfer)
	if err == nil {
		exportedAt := transfer.CreatedAt
		if transfer.StartedAt != nil {
			exportedAt = *transfer.StartedAt
		}
		_, err = provider.PrepareVolumeTransfer(preparationCtx, kubeprovider.VolumeTransferSpec{
			TransferID: transfer.ID, ProjectID: transfer.ProjectID, ProjectVolumeID: transfer.ProjectVolumeID,
			Namespace: projectVolume.Namespace, ClaimName: claimName, Direction: transfer.Direction,
			Format: transfer.Format, VolumeMode: projectVolume.VolumeMode, ConsistencyMode: transfer.ConsistencyMode,
			Image: r.volumeTransferJobImage, CapacityBytes: projectVolume.CapacityBytes,
			MaxArchiveBytes: r.volumeTransferMaxBytes,
			ExpectedBytes:   transfer.ExpectedBytes, ExpectedSHA256: transfer.SHA256, ExportedAt: exportedAt,
		})
	}
	if err == nil {
		transfer, err = service.ConfirmVolumeTransferJobCreated(preparationCtx, transfer.ProjectID, transfer.ID, transfer.ExecutionGeneration)
	}
	if err == nil {
		err = r.waitForVolumeTransferPodReady(preparationCtx, service, provider, projectVolume, transfer)
	}
	leaseErr := stopLeaseHeartbeat()
	if err == nil {
		err = leaseErr
	}
	if err != nil {
		return r.failOrRetryVolumeTransfer(ctx, service, provider, transfer, projectVolume, snapshotCleanup, err)
	}
	return nil
}

func (r *Runner) waitForVolumeTransferPodReady(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferProvider, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) error {
	readyCtx, cancel := context.WithTimeout(ctx, volumeTransferPodReadyDeadline)
	defer cancel()
	ticker := time.NewTicker(volumeTransferPodPollInterval)
	defer ticker.Stop()
	for {
		current, err := service.GetVolumeTransfer(readyCtx, transfer.ProjectID, transfer.ID)
		if err != nil {
			return err
		}
		if current.State == model.VolumeTransferStateCancelled {
			return context.Canceled
		}
		observation, err := provider.ObserveVolumeTransfer(readyCtx, projectVolume.Namespace, transfer.ID)
		if err != nil {
			return err
		}
		switch observation.State {
		case "ready":
			_, err = service.MarkVolumeTransferReady(readyCtx, transfer.ProjectID, transfer.ID, transfer.ExecutionGeneration)
			return err
		case "failed", "not_found":
			return fmt.Errorf("volume transfer pod unavailable: %s", observation.Reason)
		}
		select {
		case <-readyCtx.Done():
			return readyCtx.Err()
		case <-ticker.C:
		}
	}
}

func retryVolumeTransferExecutionClaim(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if volume.ErrorCode(err) == volume.CodeTransferStateConflict {
		return safeVolumeTaskCode(volume.CodeClusterUnavailable)
	}
	return safeVolumeTaskError(err)
}

func maintainVolumeTransferExecutionLease(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer, leaseOwner string, renewInterval time.Duration) (context.Context, func() error) {
	if renewInterval <= 0 {
		renewInterval = volumeTransferCreationLeaseTTL / 3
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(renewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				done <- nil
				return
			case <-ticker.C:
				_, err := service.RenewVolumeTransferExecutionLease(heartbeatCtx, transfer.ProjectID, transfer.ID,
					leaseOwner, transfer.ExecutionGeneration, time.Now().UTC().Add(volumeTransferCreationLeaseTTL))
				if err != nil {
					if heartbeatCtx.Err() != nil {
						done <- nil
						return
					}
					done <- err
					cancel()
					return
				}
			}
		}
	}()
	var once sync.Once
	var heartbeatErr error
	return heartbeatCtx, func() error {
		once.Do(func() {
			cancel()
			heartbeatErr = <-done
		})
		return heartbeatErr
	}
}

type volumeTransferSnapshotCleanup struct {
	claimName    string
	snapshotName string
}

func volumeTransferSnapshotCleanupFor(transfer model.VolumeTransfer) *volumeTransferSnapshotCleanup {
	if transfer.Direction != model.VolumeTransferDirectionExport || transfer.ConsistencyMode != model.VolumeTransferConsistencySnapshot {
		return nil
	}
	return &volumeTransferSnapshotCleanup{claimName: idResourceName("vtx-export", transfer.ID), snapshotName: idResourceName("vtx-snapshot", transfer.ID)}
}

func (r *Runner) prepareVolumeTransferClaim(ctx context.Context, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) (string, *volumeTransferSnapshotCleanup, error) {
	provider, err := r.projectVolumeProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return "", nil, err
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		_, observeErr := provider.ObserveProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName)
		switch {
		case observeErr == nil:
			if err := provider.DeleteProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName, projectVolume.ProjectID, projectVolume.ID); err != nil && !errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound) {
				return "", nil, err
			}
			for {
				_, observeErr = provider.ObserveProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName)
				if errors.Is(observeErr, kubeprovider.ErrProjectVolumeClaimNotFound) {
					break
				}
				if observeErr != nil {
					return "", nil, observeErr
				}
				select {
				case <-ctx.Done():
					return "", nil, ctx.Err()
				case <-time.After(volumeTransferPodPollInterval):
				}
			}
		case !errors.Is(observeErr, kubeprovider.ErrProjectVolumeClaimNotFound):
			return "", nil, observeErr
		}
		_, err = provider.CreateProjectVolumeClaim(ctx, kubeprovider.ProjectVolumeClaimSpec{
			ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID, Namespace: projectVolume.Namespace,
			ClaimName: projectVolume.ClaimName, Capacity: projectVolume.CapacityRequest,
			StorageClassName: projectVolume.StorageClassName, AccessMode: projectVolume.AccessMode, VolumeMode: projectVolume.VolumeMode,
		})
		return projectVolume.ClaimName, nil, err
	}
	if transfer.ConsistencyMode != model.VolumeTransferConsistencySnapshot {
		if transfer.ConsistencyMode == model.VolumeTransferConsistencyUnmounted &&
			(projectVolume.BindingSummary.Active != 0 || projectVolume.BindingSummary.Reserved != 0) {
			return "", nil, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "project volume is mounted"}
		}
		if projectVolume.VolumeMode == "Block" && transfer.ConsistencyMode == model.VolumeTransferConsistencyLive {
			return "", nil, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "block volume live export is unsafe"}
		}
		return projectVolume.ClaimName, nil, nil
	}
	cleanup := volumeTransferSnapshotCleanupFor(transfer)
	observation, err := provider.CreateVolumeSnapshot(ctx, kubeprovider.ProjectVolumeSnapshotSpec{
		ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID, Namespace: projectVolume.Namespace,
		Name: cleanup.snapshotName, SourceClaimName: projectVolume.ClaimName,
		ManagedClaim: projectVolume.OwnershipMode == model.ProjectVolumeOwnershipManaged,
	})
	if err != nil {
		return "", nil, err
	}
	for !observation.ReadyToUse {
		if observation.ErrorCode != "" {
			return "", nil, &volume.DomainError{Code: volume.CodeTransferJobFailed, Message: "volume snapshot failed"}
		}
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(volumeTransferPodPollInterval):
		}
		observation, err = provider.ObserveVolumeSnapshot(ctx, projectVolume.Namespace, cleanup.snapshotName)
		if err != nil {
			return "", nil, err
		}
	}
	_, err = provider.CreateProjectVolumeClaim(ctx, kubeprovider.ProjectVolumeClaimSpec{
		ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID, Namespace: projectVolume.Namespace,
		ClaimName: cleanup.claimName, Capacity: projectVolume.CapacityRequest, StorageClassName: projectVolume.StorageClassName,
		AccessMode: projectVolume.AccessMode, VolumeMode: projectVolume.VolumeMode,
		SourceSnapshotName: cleanup.snapshotName, SourceSnapshotAPI: "snapshot.storage.k8s.io", SourceSnapshotKind: "VolumeSnapshot",
	})
	return cleanup.claimName, cleanup, err
}

func (r *Runner) failOrRetryVolumeTransfer(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferProvider, transfer model.VolumeTransfer, projectVolume model.ProjectVolume, snapshot *volumeTransferSnapshotCleanup, cause error) error {
	if errors.Is(cause, context.Canceled) && ctx.Err() != nil {
		return ctx.Err()
	}
	code, permanent := projectVolumeProviderError(cause)
	if domainCode := volume.ErrorCode(cause); domainCode != "" {
		code = domainCode
		permanent = domainCode != volume.CodeClusterUnavailable && domainCode != volume.CodeDeletionPending
	}
	if permanent || volumeTaskAttemptExhausted(ctx) {
		return r.failAndCleanupVolumeTransfer(ctx, service, provider, transfer, projectVolume, snapshot, code, cause)
	}
	return safeVolumeTaskCode(code)
}

func (r *Runner) failStaleStreamingVolumeTransfer(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer, cutoff time.Time) error {
	failed, err := service.FailStaleVolumeTransfer(ctx, transfer.ProjectID, transfer.ID, cutoff,
		volume.CodeTransferJobFailed, "volume transfer stream heartbeat expired")
	if err != nil {
		if volume.ErrorCode(err) == volume.CodeTransferStateConflict {
			// A concurrent heartbeat or terminal transition won the CAS. Its owner
			// remains responsible for the stream and cleanup.
			return nil
		}
		return err
	}
	projectVolume, err := service.GetProjectVolume(ctx, failed.ProjectID, failed.ProjectVolumeID)
	if err != nil {
		return err
	}
	provider, err := r.volumeTransferProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return err
	}
	_, err = r.completeVolumeTransferExecutionCleanup(ctx, service, provider, volumeTransferSnapshotCleanupFor(failed), projectVolume, failed)
	return err
}

func (r *Runner) failVolumeTransfer(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer, code string, cause error) error {
	diagnostic := "volume transfer preparation failed"
	if cause != nil {
		diagnostic = cause.Error()
	}
	_, err := service.FailVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID, code, diagnostic)
	if err != nil {
		return safeVolumeTaskError(err)
	}
	return skipVolumeTask(code)
}

func (r *Runner) failAndCleanupVolumeTransfer(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferProvider, transfer model.VolumeTransfer, projectVolume model.ProjectVolume, snapshot *volumeTransferSnapshotCleanup, code string, cause error) error {
	failure := r.failVolumeTransfer(ctx, service, transfer, code, cause)
	transfer.State = model.VolumeTransferStateFailed
	if _, cleanupErr := r.completeVolumeTransferExecutionCleanup(ctx, service, provider, snapshot, projectVolume, transfer); cleanupErr != nil {
		return safeVolumeTaskError(cleanupErr)
	}
	return failure
}

func (r *Runner) completeVolumeTransferExecutionCleanup(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferProvider, snapshot *volumeTransferSnapshotCleanup, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) (model.VolumeTransfer, error) {
	if transfer.ExecutionCleanupCompletedAt != nil {
		return transfer, nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), volumeTransferCleanupDeadline)
	defer cancel()
	if err := provider.CleanupVolumeTransfer(cleanupCtx, projectVolume.Namespace, transfer.ID); err != nil {
		return model.VolumeTransfer{}, err
	}
	if snapshot != nil {
		volumeProvider, err := r.projectVolumeProvider(cleanupCtx, projectVolume.ClusterID)
		if err != nil {
			return model.VolumeTransfer{}, err
		}
		if err := volumeProvider.DeleteProjectVolumeClaim(cleanupCtx, projectVolume.Namespace, snapshot.claimName, projectVolume.ProjectID, projectVolume.ID); err != nil && !errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound) {
			return model.VolumeTransfer{}, err
		}
		if err := volumeProvider.DeleteVolumeSnapshot(cleanupCtx, projectVolume.Namespace, snapshot.snapshotName, projectVolume.ProjectID, projectVolume.ID); err != nil {
			return model.VolumeTransfer{}, err
		}
	}
	if transfer.Direction == model.VolumeTransferDirectionImport && transfer.State != model.VolumeTransferStateSucceeded {
		volumeProvider, err := r.projectVolumeProvider(cleanupCtx, projectVolume.ClusterID)
		if err != nil {
			return model.VolumeTransfer{}, err
		}
		if err := volumeProvider.DeleteProjectVolumeClaim(cleanupCtx, projectVolume.Namespace, projectVolume.ClaimName, projectVolume.ProjectID, projectVolume.ID); err != nil && !errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound) {
			return model.VolumeTransfer{}, err
		}
	}
	return service.MarkVolumeTransferExecutionCleanupCompleted(ctx, transfer.ProjectID, transfer.ID)
}

func safeVolumeCleanupResult(err error) error {
	if err == nil {
		return nil
	}
	return safeVolumeTaskError(err)
}

func (r *Runner) cleanupCancelledTransfer(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer, projectVolume model.ProjectVolume) error {
	if projectVolume.ID == "" {
		return nil
	}
	provider, err := r.volumeTransferProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return safeVolumeTaskCode(volume.CodeClusterUnavailable)
	}
	return r.cleanupCancelledTransferWithProvider(ctx, service, provider, projectVolume, transfer)
}

func (r *Runner) cleanupCancelledTransferWithProvider(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferProvider, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) error {
	if err := provider.CancelVolumeTransfer(ctx, projectVolume.Namespace, transfer.ID); err != nil {
		return safeVolumeTaskCode(volume.CodeClusterUnavailable)
	}
	cleaned, err := r.completeVolumeTransferExecutionCleanup(ctx, service, provider, volumeTransferSnapshotCleanupFor(transfer), projectVolume, transfer)
	if err != nil {
		return safeVolumeTaskError(err)
	}
	transfer = cleaned
	if transfer.Direction != model.VolumeTransferDirectionImport {
		return nil
	}
	_, err = service.CompleteCancelledVolumeImport(ctx, projectVolume.ProjectID, projectVolume.ID, transfer.ID)
	if volume.ErrorCode(err) == volume.CodeNotFound {
		return nil
	}
	return safeVolumeTaskError(err)
}
