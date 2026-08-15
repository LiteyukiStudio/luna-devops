package worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
	"github.com/hibiken/asynq"
)

const (
	volumeTransferJobPollInterval  = 2 * time.Second
	volumeTransferCallbackTTL      = 3 * time.Hour
	volumeTransferCleanupDeadline  = 45 * time.Second
	volumeTransferCreationLeaseTTL = 2 * time.Minute
	volumeTransferObjectLeaseTTL   = 2 * time.Minute
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
	if volume.IsVolumeTransferTerminal(transfer.State) && transfer.JobCreatedAt == nil &&
		transfer.CreationLeaseExpiresAt != nil && transfer.CreationLeaseExpiresAt.After(time.Now().UTC()) {
		// A terminal transition (most commonly cancellation) may race a Worker
		// that is still creating deterministic Job prerequisites. Wait for its
		// fenced lease to expire; its heartbeat observes the terminal state and
		// cancels provider calls before cleanup is allowed to release assets.
		return safeVolumeTaskCode(volume.CodeClusterUnavailable)
	}
	projectVolume, err := service.GetProjectVolume(ctx, payload.ProjectID, payload.VolumeID)
	if err != nil {
		if volume.ErrorCode(err) == volume.CodeNotFound && transfer.State == model.VolumeTransferStateCancelled {
			return r.cleanupCancelledTransfer(ctx, service, transfer, model.ProjectVolume{})
		}
		return safeVolumeTaskError(err)
	}
	if transfer.ExecutionCleanupCompletedAt != nil &&
		(transfer.State == model.VolumeTransferStateSucceeded || transfer.State == model.VolumeTransferStateFailed || transfer.State == model.VolumeTransferStateExpired) {
		return nil
	}
	jobProvider, err := r.volumeTransferJobProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		if volume.IsVolumeTransferTerminal(transfer.State) {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
		return r.failVolumeTransfer(ctx, service, transfer, projectVolume, volume.CodeClusterUnavailable, err)
	}

	switch transfer.State {
	case model.VolumeTransferStateSucceeded:
		_, err = r.completeVolumeTransferExecutionCleanup(ctx, service, jobProvider, volumeTransferSnapshotCleanupFor(transfer), projectVolume, transfer)
		return safeVolumeCleanupResult(err)
	case model.VolumeTransferStateFailed, model.VolumeTransferStateExpired:
		_, err = r.completeVolumeTransferExecutionCleanup(ctx, service, jobProvider, volumeTransferSnapshotCleanupFor(transfer), projectVolume, transfer)
		return safeVolumeCleanupResult(err)
	case model.VolumeTransferStateCancelled:
		return r.cleanupCancelledTransferWithProvider(ctx, service, jobProvider, projectVolume, transfer)
	case model.VolumeTransferStateQueued, model.VolumeTransferStateRunning:
	default:
		return safeVolumeTaskCode(volume.CodeTransferStateConflict)
	}

	claimName := projectVolume.ClaimName
	var snapshotCleanup *volumeTransferSnapshotCleanup
	leaseOwner := id.New("vlease")
	executionClaimed := false
	replacingExecution := false
	if transfer.State == model.VolumeTransferStateRunning {
		if transfer.JobSucceededAt != nil {
			return r.finalizeSuccessfulVolumeTransfer(ctx, service, jobProvider, projectVolume, transfer, volumeTransferSnapshotCleanupFor(transfer))
		}
		observation, observeErr := jobProvider.ObserveVolumeTransferJob(ctx, projectVolume.Namespace, transfer.ID)
		if observeErr != nil {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
		switch observation.State {
		case "pending", "running":
			transfer, err = r.confirmObservedVolumeTransferJob(ctx, service, transfer)
			if err != nil {
				return safeVolumeTaskError(err)
			}
			return r.waitForVolumeTransferJob(ctx, service, jobProvider, projectVolume, transfer, volumeTransferSnapshotCleanupFor(transfer))
		case "succeeded", "failed":
			transfer, err = r.confirmObservedVolumeTransferJob(ctx, service, transfer)
			if err != nil {
				return safeVolumeTaskError(err)
			}
			return r.finishObservedVolumeTransferJob(ctx, service, jobProvider, projectVolume, transfer, volumeTransferSnapshotCleanupFor(transfer), observation)
		case "not_found":
			if transfer.CompletionReportedAt != nil {
				return r.failVolumeTransferWithoutJobAuthority(ctx, service, jobProvider, projectVolume, transfer,
					volumeTransferSnapshotCleanupFor(transfer),
					errors.New("volume transfer completion was reported without an authoritative Job success observation"))
			}
			if transfer.Direction == model.VolumeTransferDirectionExport {
				if transfer.JobCreatedAt != nil {
					return r.failVolumeTransferWithoutJobAuthority(ctx, service, jobProvider, projectVolume, transfer,
						volumeTransferSnapshotCleanupFor(transfer),
						errors.New("volume export Job disappeared after its creation was confirmed"))
				}
				_, totalParts, partsErr := service.ListVolumeTransferParts(ctx, transfer.ID, 1, 1)
				if partsErr != nil {
					return safeVolumeTaskError(partsErr)
				}
				if totalParts > 0 {
					return r.failVolumeTransferWithoutJobAuthority(ctx, service, jobProvider, projectVolume, transfer,
						volumeTransferSnapshotCleanupFor(transfer),
						errors.New("volume export Job disappeared after committing multipart content"))
				}
			}
			claimed, claimErr := service.ClaimVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID,
				transfer.State, leaseOwner, time.Now().UTC().Add(volumeTransferCreationLeaseTTL))
			if claimErr != nil {
				return retryVolumeTransferExecutionClaim(claimErr)
			}
			transfer = claimed
			executionClaimed = true
			replacingExecution = true
		default:
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
	}

	if !executionClaimed {
		claimed, claimErr := service.ClaimVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID,
			transfer.State, leaseOwner, time.Now().UTC().Add(volumeTransferCreationLeaseTTL))
		if claimErr != nil {
			return retryVolumeTransferExecutionClaim(claimErr)
		}
		transfer = claimed
	}
	creationCtx, stopLeaseHeartbeat := maintainVolumeTransferExecutionLease(ctx, service, transfer, leaseOwner,
		volumeTransferCreationLeaseTTL/3)
	creationStage := ""
	creationErr := func() error {
		if replacingExecution {
			creationStage = "cleanup"
			// The persisted raw callback token is intentionally unavailable after
			// a Worker restart. Only the fenced lease owner may remove orphaned
			// prerequisites and rotate the hash for a replacement Job.
			if cleanupErr := r.cleanupVolumeTransferResourcesFenced(creationCtx, jobProvider, volumeTransferSnapshotCleanupFor(transfer), projectVolume, transfer); cleanupErr != nil {
				return cleanupErr
			}
		}
		creationStage = "claim"
		claimName, snapshotCleanup, err = r.prepareVolumeTransferClaim(creationCtx, service, projectVolume, transfer)
		if err != nil {
			return err
		}
		creationStage = "token"
		rawToken, tokenHash, tokenErr := newVolumeTransferCallbackToken()
		if tokenErr != nil {
			return tokenErr
		}
		defer clear(rawToken)
		creationStage = "prepare"
		prepared, prepareErr := service.PrepareVolumeTransferExecution(creationCtx, transfer.ProjectID, transfer.ID, transfer.State,
			leaseOwner, transfer.ExecutionGeneration, tokenHash, time.Now().UTC().Add(volumeTransferCallbackTTL))
		if prepareErr != nil {
			return prepareErr
		}
		transfer = prepared
		exportedAt := transfer.CreatedAt
		if transfer.StartedAt != nil {
			exportedAt = *transfer.StartedAt
		}
		creationStage = "create"
		chunkExpectedBytes := transfer.ExpectedBytes
		if transfer.Direction == model.VolumeTransferDirectionExport {
			chunkExpectedBytes = r.volumeTransferMaxBytes
		}
		_, createErr := jobProvider.CreateVolumeTransferJob(creationCtx, kubeprovider.VolumeTransferJobSpec{
			TransferID: transfer.ID, ProjectID: transfer.ProjectID, ProjectVolumeID: transfer.ProjectVolumeID,
			Namespace: projectVolume.Namespace, ClaimName: claimName, Direction: transfer.Direction,
			Format: transfer.Format, VolumeMode: projectVolume.VolumeMode, ConsistencyMode: transfer.ConsistencyMode,
			CallbackBaseURL: r.volumeTransferCallbackURL, CallbackToken: rawToken, Image: r.volumeTransferJobImage,
			CapacityBytes: projectVolume.CapacityBytes, ExpectedBytes: transfer.ExpectedBytes,
			ExpectedSHA256: transfer.SHA256, ExportedAt: exportedAt,
			ChunkSize: volumetransfer.RequiredChunkSize(chunkExpectedBytes),
		})
		return createErr
	}()
	leaseErr := stopLeaseHeartbeat()
	if leaseErr != nil {
		return retryVolumeTransferExecutionClaim(leaseErr)
	}
	if creationErr != nil {
		switch creationStage {
		case "claim", "create":
			return r.failOrRetryVolumeTransfer(ctx, service, jobProvider, transfer, projectVolume, snapshotCleanup, creationErr)
		case "token":
			return r.failAndCleanupVolumeTransfer(ctx, service, jobProvider, transfer, projectVolume, snapshotCleanup,
				volume.CodeTransferJobFailed, creationErr)
		default:
			return safeVolumeTaskError(creationErr)
		}
	}
	transfer, err = service.ConfirmVolumeTransferJobCreated(ctx, transfer.ProjectID, transfer.ID, transfer.ExecutionGeneration)
	if err != nil {
		return safeVolumeTaskError(err)
	}
	return r.waitForVolumeTransferJob(ctx, service, jobProvider, projectVolume, transfer, snapshotCleanup)
}

func (r *Runner) confirmObservedVolumeTransferJob(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer) (model.VolumeTransfer, error) {
	if transfer.JobCreatedAt != nil {
		return transfer, nil
	}
	return service.ConfirmVolumeTransferJobCreated(ctx, transfer.ProjectID, transfer.ID, transfer.ExecutionGeneration)
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

// maintainVolumeTransferExecutionLease keeps provider preparation fenced even
// when snapshot readiness or Kubernetes API calls outlive the initial lease.
// Losing the lease cancels the in-flight provider context so a stale attempt
// cannot continue mutating resources owned by a newer generation.
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
	return &volumeTransferSnapshotCleanup{
		claimName: idResourceName("vtx-export", transfer.ID), snapshotName: idResourceName("vtx-snapshot", transfer.ID),
	}
}

func (r *Runner) prepareVolumeTransferClaim(ctx context.Context, service volumeWorkerService, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) (string, *volumeTransferSnapshotCleanup, error) {
	provider, err := r.projectVolumeProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return "", nil, err
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		if transfer.State == model.VolumeTransferStateQueued {
			_, observeErr := provider.ObserveProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName)
			switch {
			case observeErr == nil:
				if err := provider.DeleteProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName, projectVolume.ProjectID, projectVolume.ID); err != nil {
					return "", nil, err
				}
				return "", nil, &volume.DomainError{Code: volume.CodeDeletionPending, Message: "partial import volume cleanup is pending"}
			case !errors.Is(observeErr, kubeprovider.ErrProjectVolumeClaimNotFound):
				return "", nil, observeErr
			}
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
		return projectVolume.ClaimName, nil, nil
	}

	cleanup := volumeTransferSnapshotCleanupFor(transfer)
	snapshotName := cleanup.snapshotName
	claimName := cleanup.claimName
	observation, err := provider.CreateVolumeSnapshot(ctx, kubeprovider.ProjectVolumeSnapshotSpec{
		ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID, Namespace: projectVolume.Namespace,
		Name: snapshotName, SourceClaimName: projectVolume.ClaimName,
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
		case <-time.After(volumeTransferJobPollInterval):
		}
		observation, err = provider.ObserveVolumeSnapshot(ctx, projectVolume.Namespace, snapshotName)
		if err != nil {
			return "", nil, err
		}
	}
	_, err = provider.CreateProjectVolumeClaim(ctx, kubeprovider.ProjectVolumeClaimSpec{
		ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID, Namespace: projectVolume.Namespace,
		ClaimName: claimName, Capacity: projectVolume.CapacityRequest, StorageClassName: projectVolume.StorageClassName,
		AccessMode: projectVolume.AccessMode, VolumeMode: projectVolume.VolumeMode,
		SourceSnapshotName: snapshotName, SourceSnapshotAPI: "snapshot.storage.k8s.io", SourceSnapshotKind: "VolumeSnapshot",
	})
	if err != nil {
		return "", nil, err
	}
	return claimName, cleanup, nil
}

func (r *Runner) waitForVolumeTransferJob(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferJobProvider, projectVolume model.ProjectVolume, transfer model.VolumeTransfer, snapshot *volumeTransferSnapshotCleanup) error {
	ticker := time.NewTicker(volumeTransferJobPollInterval)
	defer ticker.Stop()
	for {
		current, err := service.GetVolumeTransfer(ctx, transfer.ProjectID, transfer.ID)
		if err != nil {
			return safeVolumeTaskError(err)
		}
		if current.State == model.VolumeTransferStateCancelled {
			return r.cleanupCancelledTransferWithProvider(ctx, service, provider, projectVolume, current)
		}
		if current.State == model.VolumeTransferStateSucceeded || current.State == model.VolumeTransferStateFailed {
			_, cleanupErr := r.completeVolumeTransferExecutionCleanup(ctx, service, provider, snapshot, projectVolume, current)
			return safeVolumeCleanupResult(cleanupErr)
		}
		if current.JobSucceededAt != nil {
			return r.finalizeSuccessfulVolumeTransfer(ctx, service, provider, projectVolume, current, snapshot)
		}
		observation, err := provider.ObserveVolumeTransferJob(ctx, projectVolume.Namespace, transfer.ID)
		if err != nil {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
		switch observation.State {
		case "succeeded", "failed", "not_found":
			return r.finishObservedVolumeTransferJob(ctx, service, provider, projectVolume, current, snapshot, observation)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) finishObservedVolumeTransferJob(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferJobProvider, projectVolume model.ProjectVolume, transfer model.VolumeTransfer, snapshot *volumeTransferSnapshotCleanup, observation kubeprovider.VolumeTransferJobObservation) error {
	current, err := service.GetVolumeTransfer(ctx, transfer.ProjectID, transfer.ID)
	if err != nil {
		return safeVolumeTaskError(err)
	}
	if current.State == model.VolumeTransferStateCancelled {
		return r.cleanupCancelledTransferWithProvider(ctx, service, provider, projectVolume, current)
	}
	if current.State == model.VolumeTransferStateSucceeded {
		_, cleanupErr := r.completeVolumeTransferExecutionCleanup(ctx, service, provider, snapshot, projectVolume, current)
		return safeVolumeCleanupResult(cleanupErr)
	}
	if observation.State == "succeeded" {
		return r.finalizeSuccessfulVolumeTransfer(ctx, service, provider, projectVolume, current, snapshot)
	}
	if current.State == model.VolumeTransferStateFailed {
		_, cleanupErr := r.completeVolumeTransferExecutionCleanup(ctx, service, provider, snapshot, projectVolume, current)
		return safeVolumeCleanupResult(cleanupErr)
	}
	if observation.State == "not_found" {
		if current.CompletionReportedAt != nil {
			return r.failVolumeTransferWithoutJobAuthority(ctx, service, provider, projectVolume, current, snapshot,
				errors.New("volume transfer completion was reported without an authoritative Job success observation"))
		}
		return r.failOrRetryVolumeTransfer(ctx, service, provider, current, projectVolume, snapshot,
			&volume.DomainError{Code: volume.CodeClusterUnavailable, Message: "volume transfer job was not found"})
	}
	code := volume.CodeTransferJobFailed
	return r.failAndCleanupVolumeTransfer(ctx, service, provider, current, projectVolume, snapshot, code,
		stableVolumeTransferFailure(observation.Reason))
}

func (r *Runner) failVolumeTransferWithoutJobAuthority(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferJobProvider, projectVolume model.ProjectVolume, transfer model.VolumeTransfer, snapshot *volumeTransferSnapshotCleanup, cause error) error {
	return r.failAndCleanupVolumeTransfer(ctx, service, provider, transfer, projectVolume, snapshot,
		volume.CodeTransferJobFailed, cause)
}

func (r *Runner) finalizeSuccessfulVolumeTransfer(ctx context.Context, service volumeWorkerService, jobProvider kubeprovider.VolumeTransferJobProvider, projectVolume model.ProjectVolume, transfer model.VolumeTransfer, snapshot *volumeTransferSnapshotCleanup) error {
	if transfer.JobSucceededAt == nil {
		marked, err := service.MarkVolumeTransferJobSucceeded(ctx, transfer.ProjectID, transfer.ID)
		if err != nil {
			return safeVolumeTaskError(err)
		}
		transfer = marked
	}
	if transfer.CompletionReportedAt == nil {
		return r.failAndCleanupVolumeTransfer(ctx, service, jobProvider, transfer, projectVolume, snapshot,
			volume.CodeTransferCompletionMissing,
			errors.New("volume transfer Job succeeded without an authenticated completion report"))
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		if err := r.verifyImportedVolumeClaim(ctx, projectVolume); err != nil {
			code, permanent := projectVolumeProviderError(err)
			if !permanent {
				return safeVolumeTaskCode(code)
			}
			return r.failAndCleanupVolumeTransfer(ctx, service, jobProvider, transfer, projectVolume, snapshot, code, err)
		}
	}
	finalized, err := service.FinalizeVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID)
	if err != nil {
		if volume.ErrorCode(err) == volume.CodeTransferStateConflict || volume.ErrorCode(err) == volume.CodeStateConflict {
			current, getErr := service.GetVolumeTransfer(ctx, transfer.ProjectID, transfer.ID)
			if getErr == nil {
				switch current.State {
				case model.VolumeTransferStateSucceeded, model.VolumeTransferStateFailed:
					_, cleanupErr := r.completeVolumeTransferExecutionCleanup(ctx, service, jobProvider, snapshot, projectVolume, current)
					return safeVolumeCleanupResult(cleanupErr)
				case model.VolumeTransferStateCancelled:
					return r.cleanupCancelledTransferWithProvider(ctx, service, jobProvider, projectVolume, current)
				case model.VolumeTransferStateRunning:
					return r.failAndCleanupVolumeTransfer(ctx, service, jobProvider, current, projectVolume, snapshot,
						volume.ErrorCode(err), err)
				}
			}
		}
		return safeVolumeTaskError(err)
	}
	_, cleanupErr := r.completeVolumeTransferExecutionCleanup(ctx, service, jobProvider, snapshot, projectVolume, finalized)
	return safeVolumeCleanupResult(cleanupErr)
}

func (r *Runner) verifyImportedVolumeClaim(ctx context.Context, projectVolume model.ProjectVolume) error {
	provider, err := r.projectVolumeProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return err
	}
	inspection, err := provider.InspectExistingProjectVolumeClaim(ctx, kubeprovider.ExistingProjectVolumeClaimSpec{
		ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID,
		Namespace: projectVolume.Namespace, ClaimName: projectVolume.ClaimName,
		ExpectedCapacity: projectVolume.CapacityRequest, ExpectedStorageClassName: projectVolume.StorageClassName,
		ExpectedAccessMode: projectVolume.AccessMode, ExpectedVolumeMode: projectVolume.VolumeMode,
	})
	if err != nil {
		return err
	}
	if !inspection.Observation.Exists {
		return kubeprovider.ErrProjectVolumeClaimNotFound
	}
	if inspection.ManagedBy != kubeprovider.ManagedByValue || inspection.ProjectID != projectVolume.ProjectID || inspection.ProjectVolumeID != projectVolume.ID {
		return kubeprovider.ErrProjectVolumeOwnershipConflict
	}
	return nil
}

func (r *Runner) failOrRetryVolumeTransfer(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferJobProvider, transfer model.VolumeTransfer, projectVolume model.ProjectVolume, snapshot *volumeTransferSnapshotCleanup, cause error) error {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
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

func (r *Runner) failVolumeTransfer(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer, projectVolume model.ProjectVolume, code string, cause error) error {
	diagnostic := "volume transfer execution failed"
	if cause != nil {
		diagnostic = cause.Error()
	}
	_, err := service.FailVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID, code, diagnostic)
	if err != nil {
		return safeVolumeTaskError(err)
	}
	return skipVolumeTask(code)
}

func (r *Runner) failAndCleanupVolumeTransfer(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferJobProvider, transfer model.VolumeTransfer, projectVolume model.ProjectVolume, snapshot *volumeTransferSnapshotCleanup, code string, cause error) error {
	failure := r.failVolumeTransfer(ctx, service, transfer, projectVolume, code, cause)
	if !errors.Is(failure, asynq.SkipRetry) {
		return failure
	}
	transfer.State = model.VolumeTransferStateFailed
	if _, cleanupErr := r.completeVolumeTransferExecutionCleanup(ctx, service, provider, snapshot, projectVolume, transfer); cleanupErr != nil {
		return safeVolumeTaskError(cleanupErr)
	}
	return failure
}

func (r *Runner) completeVolumeTransferExecutionCleanup(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferJobProvider, snapshot *volumeTransferSnapshotCleanup, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) (model.VolumeTransfer, error) {
	if transfer.ExecutionCleanupCompletedAt != nil {
		return transfer, nil
	}
	if err := r.cleanupVolumeTransferResources(ctx, provider, snapshot, projectVolume, transfer); err != nil {
		return model.VolumeTransfer{}, err
	}
	return service.MarkVolumeTransferExecutionCleanupCompleted(ctx, transfer.ProjectID, transfer.ID)
}

func (r *Runner) cleanupVolumeTransferResources(ctx context.Context, provider kubeprovider.VolumeTransferJobProvider, snapshot *volumeTransferSnapshotCleanup, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) error {
	// Terminal cleanup must survive cancellation of the Asynq attempt while
	// retaining trace values. It is bounded and never used for the data path.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), volumeTransferCleanupDeadline)
	defer cancel()
	return r.cleanupVolumeTransferResourcesWithContext(cleanupCtx, provider, snapshot, projectVolume, transfer)
}

func (r *Runner) cleanupVolumeTransferResourcesFenced(ctx context.Context, provider kubeprovider.VolumeTransferJobProvider, snapshot *volumeTransferSnapshotCleanup, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) error {
	cleanupCtx, cancel := context.WithTimeout(ctx, volumeTransferCleanupDeadline)
	defer cancel()
	return r.cleanupVolumeTransferResourcesWithContext(cleanupCtx, provider, snapshot, projectVolume, transfer)
}

func (r *Runner) cleanupVolumeTransferResourcesWithContext(cleanupCtx context.Context, provider kubeprovider.VolumeTransferJobProvider, snapshot *volumeTransferSnapshotCleanup, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) error {
	if err := provider.CleanupVolumeTransferJob(cleanupCtx, projectVolume.Namespace, transfer.ID); err != nil {
		return err
	}
	if snapshot == nil {
		return nil
	}
	volumeProvider, err := r.projectVolumeProvider(cleanupCtx, projectVolume.ClusterID)
	if err != nil {
		return err
	}
	for {
		err = volumeProvider.DeleteProjectVolumeClaim(cleanupCtx, projectVolume.Namespace, snapshot.claimName, projectVolume.ProjectID, projectVolume.ID)
		if err == nil || errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound) {
			break
		}
		if !errors.Is(err, kubeprovider.ErrProjectVolumeClaimInUse) {
			return err
		}
		select {
		case <-cleanupCtx.Done():
			return cleanupCtx.Err()
		case <-time.After(time.Second):
		}
	}
	return volumeProvider.DeleteVolumeSnapshot(cleanupCtx, projectVolume.Namespace, snapshot.snapshotName, projectVolume.ProjectID, projectVolume.ID)
}

func safeVolumeCleanupResult(err error) error {
	if err == nil {
		return nil
	}
	return safeVolumeTaskError(err)
}

func newVolumeTransferCallbackToken() ([]byte, string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, "", err
	}
	raw := []byte(base64.RawURLEncoding.EncodeToString(random))
	clear(random)
	digest := sha256.Sum256(raw)
	return raw, hex.EncodeToString(digest[:]), nil
}

func (r *Runner) cleanupCancelledTransfer(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer, projectVolume model.ProjectVolume) error {
	if projectVolume.ID == "" {
		if transfer.ExecutionCleanupCompletedAt == nil {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
		return r.cleanupVolumeTransferObject(ctx, service, transfer)
	}
	provider, err := r.volumeTransferJobProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return safeVolumeTaskCode(volume.CodeClusterUnavailable)
	}
	return r.cleanupCancelledTransferWithProvider(ctx, service, provider, projectVolume, transfer)
}

func (r *Runner) cleanupCancelledTransferWithProvider(ctx context.Context, service volumeWorkerService, provider kubeprovider.VolumeTransferJobProvider, projectVolume model.ProjectVolume, transfer model.VolumeTransfer) error {
	if transfer.ExecutionCleanupCompletedAt == nil {
		if err := provider.CancelVolumeTransferJob(ctx, projectVolume.Namespace, transfer.ID); err != nil {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
		// Cancellation is not durably complete until the Job, callback Secret,
		// NetworkPolicy and snapshot-scoped resources are gone. The marker is
		// written only after all provider cleanup succeeds, so stale reconciliation
		// can safely retry from the same durable cancelled state.
		cleaned, err := r.completeVolumeTransferExecutionCleanup(ctx, service, provider,
			volumeTransferSnapshotCleanupFor(transfer), projectVolume, transfer)
		if err != nil {
			return safeVolumeTaskError(err)
		}
		transfer = cleaned
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		volumeProvider, err := r.projectVolumeProvider(ctx, projectVolume.ClusterID)
		if err != nil {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
		if err := volumeProvider.DeleteProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName, projectVolume.ProjectID, projectVolume.ID); err != nil && !errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound) {
			return safeVolumeTaskCode(volume.CodeClusterUnavailable)
		}
	}
	if err := r.cleanupVolumeTransferObject(ctx, service, transfer); err != nil {
		return err
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		if _, err := service.CompleteCancelledVolumeImport(ctx, projectVolume.ProjectID, projectVolume.ID, transfer.ID); err != nil {
			if volume.ErrorCode(err) == volume.CodeNotFound {
				return nil
			}
			return safeVolumeTaskError(err)
		}
	}
	return nil
}

func (r *Runner) cleanupVolumeTransferObject(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer) error {
	if transfer.ObjectDeletedAt != nil || !transfer.ObjectOwned || strings.TrimSpace(transfer.ObjectKey) == "" {
		return nil
	}
	if volume.IsVolumeTransferTerminal(transfer.State) && transfer.ExecutionCleanupCompletedAt == nil {
		return safeVolumeTaskCode(volume.CodeDeletionPending)
	}
	if r.volumeTransferStore == nil {
		return safeVolumeTaskCode(volume.CodeTransferStoreUnavailable)
	}
	leaseToken := id.New("voclease")
	claimed, err := service.ClaimVolumeTransferObjectCleanup(ctx, transfer.ProjectID, transfer.ID,
		leaseToken, time.Now().UTC().Add(volumeTransferObjectLeaseTTL))
	if err != nil {
		return safeVolumeTaskError(err)
	}
	if claimed.ObjectDeletedAt != nil || !claimed.ObjectOwned || strings.TrimSpace(claimed.ObjectKey) == "" {
		return nil
	}
	cleanupCtx, stopLeaseHeartbeat := maintainVolumeTransferObjectCleanupLease(ctx, service, claimed, leaseToken,
		volumeTransferObjectLeaseTTL/3)
	completed := false
	defer func() {
		if completed {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = service.ReleaseVolumeTransferObjectCleanup(releaseCtx, claimed.ProjectID, claimed.ID, leaseToken)
	}()
	if claimed.MultipartUploadID != "" {
		if err := r.volumeTransferStore.AbortMultipart(cleanupCtx, claimed.ObjectKey, claimed.MultipartUploadID); err != nil {
			_ = stopLeaseHeartbeat()
			return safeVolumeTaskCode(volume.CodeTransferStoreUnavailable)
		}
	}
	if err := r.volumeTransferStore.Delete(cleanupCtx, claimed.ObjectKey); err != nil {
		_ = stopLeaseHeartbeat()
		return safeVolumeTaskCode(volume.CodeTransferStoreUnavailable)
	}
	if err := stopLeaseHeartbeat(); err != nil {
		return safeVolumeTaskError(err)
	}
	if _, err := service.CompleteVolumeTransferObjectCleanup(ctx, claimed.ProjectID, claimed.ID, leaseToken, time.Now().UTC()); err != nil {
		return safeVolumeTaskError(err)
	}
	completed = true
	return nil
}

func maintainVolumeTransferObjectCleanupLease(ctx context.Context, service volumeWorkerService, transfer model.VolumeTransfer, leaseToken string, renewInterval time.Duration) (context.Context, func() error) {
	if renewInterval <= 0 {
		renewInterval = volumeTransferObjectLeaseTTL / 3
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
				_, err := service.RenewVolumeTransferObjectCleanup(heartbeatCtx, transfer.ProjectID, transfer.ID,
					leaseToken, time.Now().UTC().Add(volumeTransferObjectLeaseTTL))
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

func stableVolumeTransferFailure(reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "job_failed"
	}
	return fmt.Errorf("volume transfer job failed: %s", reason)
}
