package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	volumeReconcileStaleAfter = 15 * time.Minute
	volumeMaintenanceBatch    = 100
)

type volumeWorkerService interface {
	GetProjectVolume(context.Context, string, string) (model.ProjectVolume, error)
	GetProjectVolumeForMaintenance(context.Context, string) (model.ProjectVolume, error)
	SetProjectVolumeLifecycle(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error)
	CompleteProjectVolumeDeletion(context.Context, string, string) (model.ProjectVolume, error)
	ListStaleProjectVolumeOperations(context.Context, volume.MaintenanceScanOptions) ([]model.ProjectVolume, error)
	ListStaleVolumeTransferOperations(context.Context, volume.MaintenanceScanOptions) ([]model.VolumeTransfer, error)
	ListExpiredVolumeTransfers(context.Context, time.Time, int) ([]model.VolumeTransfer, error)
	GetVolumeTransferForMaintenance(context.Context, string) (model.VolumeTransfer, error)
	ExpireVolumeTransfer(context.Context, string, string, time.Time) (model.VolumeTransfer, error)
	GetVolumeTransfer(context.Context, string, string) (model.VolumeTransfer, error)
	ClaimVolumeTransferExecution(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error)
	RenewVolumeTransferExecutionLease(context.Context, string, string, string, int64, time.Time) (model.VolumeTransfer, error)
	ConfirmVolumeTransferJobCreated(context.Context, string, string, int64) (model.VolumeTransfer, error)
	MarkVolumeTransferReady(context.Context, string, string, int64) (model.VolumeTransfer, error)
	FailStaleVolumeTransfer(context.Context, string, string, time.Time, string, string) (model.VolumeTransfer, error)
	FailVolumeTransferExecution(context.Context, string, string, string, string) (model.VolumeTransfer, error)
	MarkVolumeTransferExecutionCleanupCompleted(context.Context, string, string) (model.VolumeTransfer, error)
	TransitionVolumeTransfer(context.Context, string, string, string, string, string) (model.VolumeTransfer, error)
	CompleteCancelledVolumeImport(context.Context, string, string, string) (model.ProjectVolume, error)
	ListDeploymentTargetMounts(context.Context, string, string) ([]model.DeploymentVolumeMount, error)
	ActivateDeploymentVolumeMount(context.Context, string, string) (model.DeploymentVolumeMount, error)
	FailDeploymentVolumeMount(context.Context, string, string, string, string) (model.DeploymentVolumeMount, error)
	BeginDeploymentVolumeUnbind(context.Context, string, string) (model.DeploymentVolumeMount, error)
	CompleteDeploymentVolumeUnbind(context.Context, string, string) error
}

type volumeTaskEnqueuer interface {
	EnqueueVolumeProvision(context.Context, tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error)
	EnqueueVolumeImport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error)
	EnqueueVolumeExport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error)
	EnqueueVolumeDelete(context.Context, tasks.VolumeDeletePayload) (*asynq.TaskInfo, error)
}

func (r *Runner) handleVolumeProvision(ctx context.Context, task *asynq.Task) error {
	var payload tasks.VolumeProvisionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return skipVolumeTask(volume.CodeInvalidInput)
	}
	payload.ProjectID = strings.TrimSpace(payload.ProjectID)
	payload.VolumeID = strings.TrimSpace(payload.VolumeID)
	payload.Operation = strings.ToLower(strings.TrimSpace(payload.Operation))
	if payload.Operation == "" {
		payload.Operation = tasks.VolumeOperationProvision
	}
	if payload.ProjectID == "" || payload.VolumeID == "" ||
		(payload.Operation != tasks.VolumeOperationProvision && payload.Operation != tasks.VolumeOperationExpand) {
		return skipVolumeTask(volume.CodeInvalidInput)
	}

	service, err := r.projectVolumeService()
	if err != nil {
		return err
	}
	projectVolume, err := service.GetProjectVolume(ctx, payload.ProjectID, payload.VolumeID)
	if err != nil {
		if volume.ErrorCode(err) == volume.CodeNotFound {
			return nil
		}
		return safeVolumeTaskError(err)
	}
	if projectVolume.LifecycleState == model.ProjectVolumeLifecycleReady && projectVolume.PendingOperation == "" {
		return nil
	}
	if projectVolume.PendingOperation != payload.Operation {
		// A newer operation superseded this idempotent task.
		return nil
	}

	provider, err := r.projectVolumeProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return r.finishProjectVolumeAttempt(ctx, service, projectVolume, err)
	}
	err = r.applyProjectVolumeOperation(ctx, provider, projectVolume, payload.Operation)
	if err != nil {
		return r.finishProjectVolumeAttempt(ctx, service, projectVolume, err)
	}
	from := []string{projectVolume.LifecycleState}
	if _, err = service.SetProjectVolumeLifecycle(ctx, projectVolume.ProjectID, projectVolume.ID, from,
		model.ProjectVolumeLifecycleReady, "", ""); err != nil {
		if volume.ErrorCode(err) == volume.CodeStateConflict {
			current, currentErr := service.GetProjectVolume(ctx, projectVolume.ProjectID, projectVolume.ID)
			if currentErr == nil && current.LifecycleState == model.ProjectVolumeLifecycleReady && current.PendingOperation == "" {
				return nil
			}
		}
		return safeVolumeTaskError(err)
	}
	return nil
}

func (r *Runner) applyProjectVolumeOperation(ctx context.Context, provider kubeprovider.ProjectVolumeProvider, projectVolume model.ProjectVolume, operation string) error {
	if operation == tasks.VolumeOperationExpand {
		_, err := provider.ExpandProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName,
			projectVolume.ProjectID, projectVolume.ID, projectVolume.CapacityRequest)
		return err
	}

	existingSpec := kubeprovider.ExistingProjectVolumeClaimSpec{
		ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID,
		Namespace: projectVolume.Namespace, ClaimName: projectVolume.ClaimName,
		ExpectedCapacity: projectVolume.CapacityRequest, ExpectedStorageClassName: projectVolume.StorageClassName,
		ExpectedAccessMode: projectVolume.AccessMode, ExpectedVolumeMode: projectVolume.VolumeMode,
	}
	if projectVolume.SourceKind == model.ProjectVolumeSourceExistingClaim {
		if projectVolume.OwnershipMode == model.ProjectVolumeOwnershipManaged {
			_, err := provider.AdoptExistingProjectVolumeClaim(ctx, existingSpec)
			return err
		}
		inspection, err := provider.InspectExistingProjectVolumeClaim(ctx, existingSpec)
		if err != nil {
			return err
		}
		return ensureReferencedClaimMatches(projectVolume, inspection)
	}
	if projectVolume.SourceKind == model.ProjectVolumeSourceArchiveImport {
		return errors.New("archive imports must be provisioned by the transfer workflow")
	}
	_, err := provider.CreateProjectVolumeClaim(ctx, kubeprovider.ProjectVolumeClaimSpec{
		ProjectID:          projectVolume.ProjectID,
		VolumeID:           projectVolume.ID,
		Namespace:          projectVolume.Namespace,
		ClaimName:          projectVolume.ClaimName,
		Capacity:           projectVolume.CapacityRequest,
		StorageClassName:   projectVolume.StorageClassName,
		AccessMode:         projectVolume.AccessMode,
		VolumeMode:         projectVolume.VolumeMode,
		SourceSnapshotName: projectVolume.SourceSnapshotName,
		SourceSnapshotAPI:  "snapshot.storage.k8s.io",
		SourceSnapshotKind: "VolumeSnapshot",
	})
	return err
}

func ensureReferencedClaimMatches(projectVolume model.ProjectVolume, inspection kubeprovider.ExistingProjectVolumeClaimInspection) error {
	if inspection.ProjectID != "" && inspection.ProjectID != projectVolume.ProjectID {
		return kubeprovider.ErrProjectVolumeOwnershipConflict
	}
	if inspection.ProjectVolumeID != "" && inspection.ProjectVolumeID != projectVolume.ID {
		return kubeprovider.ErrProjectVolumeOwnershipConflict
	}
	observation := inspection.Observation
	if observation.StorageClassName != projectVolume.StorageClassName || observation.VolumeMode != projectVolume.VolumeMode ||
		!containsString(observation.AccessModes, projectVolume.AccessMode) {
		return kubeprovider.ErrProjectVolumeSpecConflict
	}
	actual, actualErr := resource.ParseQuantity(observation.Capacity)
	expected, expectedErr := resource.ParseQuantity(projectVolume.CapacityRequest)
	if actualErr != nil || expectedErr != nil || actual.Cmp(expected) < 0 {
		return kubeprovider.ErrProjectVolumeSpecConflict
	}
	return nil
}

func (r *Runner) handleVolumeDelete(ctx context.Context, task *asynq.Task) error {
	var payload tasks.VolumeDeletePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return skipVolumeTask(volume.CodeInvalidInput)
	}
	payload.ProjectID = strings.TrimSpace(payload.ProjectID)
	payload.VolumeID = strings.TrimSpace(payload.VolumeID)
	if payload.ProjectID == "" || payload.VolumeID == "" {
		return skipVolumeTask(volume.CodeInvalidInput)
	}
	service, err := r.projectVolumeService()
	if err != nil {
		return err
	}
	projectVolume, err := service.GetProjectVolume(ctx, payload.ProjectID, payload.VolumeID)
	if err != nil {
		if volume.ErrorCode(err) == volume.CodeNotFound {
			return nil
		}
		return safeVolumeTaskError(err)
	}
	if projectVolume.LifecycleState != model.ProjectVolumeLifecycleDeleting || projectVolume.PendingOperation != volume.OperationDelete {
		return nil
	}
	provider, err := r.projectVolumeProvider(ctx, projectVolume.ClusterID)
	if err != nil {
		return r.finishProjectVolumeAttempt(ctx, service, projectVolume, err)
	}
	err = provider.DeleteProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName, projectVolume.ProjectID, projectVolume.ID)
	if err != nil && !errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound) {
		return r.finishProjectVolumeAttempt(ctx, service, projectVolume, err)
	}
	if err == nil {
		_, observeErr := provider.ObserveProjectVolumeClaim(ctx, projectVolume.Namespace, projectVolume.ClaimName)
		switch {
		case observeErr == nil:
			return safeVolumeTaskCode(volume.CodeDeletionPending)
		case errors.Is(observeErr, kubeprovider.ErrProjectVolumeClaimNotFound):
		case observeErr != nil:
			return r.finishProjectVolumeAttempt(ctx, service, projectVolume, observeErr)
		}
	}
	if _, err = service.CompleteProjectVolumeDeletion(ctx, projectVolume.ProjectID, projectVolume.ID); err != nil {
		if volume.ErrorCode(err) == volume.CodeNotFound {
			return nil
		}
		return safeVolumeTaskError(err)
	}
	return nil
}

func (r *Runner) handleVolumeReconcile(ctx context.Context, task *asynq.Task) error {
	var payload tasks.VolumeReconcilePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return skipVolumeTask(volume.CodeInvalidInput)
	}
	service, err := r.projectVolumeService()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-volumeReconcileStaleAfter)
	volumes := make([]model.ProjectVolume, 0, volumeMaintenanceBatch)
	if strings.TrimSpace(payload.VolumeID) != "" {
		item, getErr := service.GetProjectVolumeForMaintenance(ctx, strings.TrimSpace(payload.VolumeID))
		if getErr != nil {
			if volume.ErrorCode(getErr) == volume.CodeNotFound {
				return nil
			}
			return safeVolumeTaskError(getErr)
		}
		volumes = append(volumes, item)
	} else {
		volumes, err = service.ListStaleProjectVolumeOperations(ctx, volume.MaintenanceScanOptions{Cutoff: cutoff, Limit: volumeMaintenanceBatch})
		if err != nil {
			return safeVolumeTaskError(err)
		}
	}
	for _, item := range volumes {
		if err := r.requeueProjectVolume(ctx, item); err != nil {
			return safeVolumeTaskError(err)
		}
	}
	if strings.TrimSpace(payload.VolumeID) != "" {
		return nil
	}
	transfers, err := service.ListStaleVolumeTransferOperations(ctx, volume.MaintenanceScanOptions{Cutoff: cutoff, Limit: volumeMaintenanceBatch})
	if err != nil {
		return safeVolumeTaskError(err)
	}
	for _, transfer := range transfers {
		if transfer.State == model.VolumeTransferStateStreaming {
			if err := r.failStaleStreamingVolumeTransfer(ctx, service, transfer, cutoff); err != nil {
				return safeVolumeTaskError(err)
			}
			continue
		}
		if err := r.requeueVolumeTransfer(ctx, transfer); err != nil {
			return safeVolumeTaskError(err)
		}
	}
	return nil
}

func (r *Runner) handleVolumeTransferCleanup(ctx context.Context, task *asynq.Task) error {
	var payload tasks.VolumeTransferCleanupPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return skipVolumeTask(volume.CodeInvalidInput)
	}
	service, err := r.projectVolumeService()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	wantedTransferID := strings.TrimSpace(payload.TransferID)
	var items []model.VolumeTransfer
	if wantedTransferID != "" {
		transfer, lookupErr := service.GetVolumeTransferForMaintenance(ctx, wantedTransferID)
		if lookupErr != nil {
			if volume.ErrorCode(lookupErr) == volume.CodeTransferNotFound || volume.ErrorCode(lookupErr) == volume.CodeNotFound {
				return nil
			}
			return safeVolumeTaskError(lookupErr)
		}
		if !volume.IsVolumeTransferTerminal(transfer.State) && transfer.ExpiresAt.After(now) {
			return nil
		}
		items = []model.VolumeTransfer{transfer}
	} else {
		items, err = service.ListExpiredVolumeTransfers(ctx, now, volumeMaintenanceBatch)
		if err != nil {
			return safeVolumeTaskError(err)
		}
	}
	for _, transfer := range items {
		if !volume.IsVolumeTransferTerminal(transfer.State) && transfer.ExpiresAt.After(now) {
			continue
		}
		if transfer.State == model.VolumeTransferStateCreated || transfer.State == model.VolumeTransferStatePreparing || transfer.State == model.VolumeTransferStateReady {
			transfer, err = service.ExpireVolumeTransfer(ctx, transfer.ProjectID, transfer.ID, now)
			if err != nil {
				if volume.ErrorCode(err) == volume.CodeTransferStateConflict {
					continue
				}
				return safeVolumeTaskError(err)
			}
			if transfer.Direction == model.VolumeTransferDirectionImport && strings.TrimSpace(transfer.ProjectVolumeID) != "" {
				_, lifecycleErr := service.SetProjectVolumeLifecycle(ctx, transfer.ProjectID, transfer.ProjectVolumeID,
					[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleError,
					volume.CodeTransferExpired, "volume import upload expired")
				if lifecycleErr != nil && volume.ErrorCode(lifecycleErr) != volume.CodeStateConflict && volume.ErrorCode(lifecycleErr) != volume.CodeNotFound {
					return safeVolumeTaskError(lifecycleErr)
				}
			}
		}
		if volume.IsVolumeTransferTerminal(transfer.State) && transfer.ExecutionCleanupCompletedAt == nil {
			projectVolume, getErr := service.GetProjectVolume(ctx, transfer.ProjectID, transfer.ProjectVolumeID)
			if getErr != nil {
				return safeVolumeTaskError(getErr)
			}
			jobProvider, providerErr := r.volumeTransferProvider(ctx, projectVolume.ClusterID)
			if providerErr != nil {
				return safeVolumeTaskCode(volume.CodeClusterUnavailable)
			}
			transfer, err = r.completeVolumeTransferExecutionCleanup(ctx, service, jobProvider,
				volumeTransferSnapshotCleanupFor(transfer), projectVolume, transfer)
			if err != nil {
				return safeVolumeTaskError(err)
			}
		}
	}
	return nil
}

func (r *Runner) requeueProjectVolume(ctx context.Context, item model.ProjectVolume) error {
	if r.volumeTaskEnqueuer == nil {
		return errors.New("volume task enqueuer is not configured")
	}
	switch {
	case item.LifecycleState == model.ProjectVolumeLifecycleDeleting && item.PendingOperation == volume.OperationDelete:
		_, err := r.volumeTaskEnqueuer.EnqueueVolumeDelete(ctx, tasks.VolumeDeletePayload{
			ProjectID: item.ProjectID, VolumeID: item.ID, ActorID: "system",
		})
		return err
	case item.LifecycleState == model.ProjectVolumeLifecycleProvisioning &&
		(item.PendingOperation == volume.OperationProvision || item.PendingOperation == volume.OperationExpand):
		_, err := r.volumeTaskEnqueuer.EnqueueVolumeProvision(ctx, tasks.VolumeProvisionPayload{
			ProjectID: item.ProjectID, VolumeID: item.ID, ActorID: "system", Operation: item.PendingOperation,
		})
		return err
	default:
		return nil
	}
}

func (r *Runner) requeueVolumeTransfer(ctx context.Context, transfer model.VolumeTransfer) error {
	if r.volumeTaskEnqueuer == nil {
		return errors.New("volume task enqueuer is not configured")
	}
	payload := tasks.VolumeTransferPayload{
		ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID, ActorID: transfer.ActorID,
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		_, err := r.volumeTaskEnqueuer.EnqueueVolumeImport(ctx, payload)
		return err
	}
	if transfer.Direction == model.VolumeTransferDirectionExport {
		_, err := r.volumeTaskEnqueuer.EnqueueVolumeExport(ctx, payload)
		return err
	}
	return skipVolumeTask(volume.CodeTransferFormatMismatch)
}

func (r *Runner) projectVolumeService() (volumeWorkerService, error) {
	if r == nil || r.volumeService == nil {
		return nil, errors.New("volume service is not configured")
	}
	return r.volumeService, nil
}

func (r *Runner) finishProjectVolumeAttempt(ctx context.Context, service volumeWorkerService, projectVolume model.ProjectVolume, cause error) error {
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	code, permanent := projectVolumeProviderError(cause)
	if permanent || volumeTaskAttemptExhausted(ctx) {
		_, transitionErr := service.SetProjectVolumeLifecycle(ctx, projectVolume.ProjectID, projectVolume.ID,
			[]string{projectVolume.LifecycleState}, model.ProjectVolumeLifecycleError, code, cause.Error())
		if transitionErr != nil && volume.ErrorCode(transitionErr) != volume.CodeStateConflict {
			return safeVolumeTaskError(transitionErr)
		}
	}
	if permanent {
		return skipVolumeTask(code)
	}
	return safeVolumeTaskCode(code)
}

func projectVolumeProviderError(err error) (string, bool) {
	switch {
	case errors.Is(err, kubeprovider.ErrInvalidProjectVolumeSpec):
		return volume.CodeInvalidInput, true
	case errors.Is(err, kubeprovider.ErrProjectVolumeClaimNotFound):
		return volume.CodeClaimNotFound, true
	case errors.Is(err, kubeprovider.ErrProjectVolumeOwnershipConflict):
		return volume.CodeOwnershipConflict, true
	case errors.Is(err, kubeprovider.ErrProjectVolumeSpecConflict):
		return volume.CodeClaimConflict, true
	case errors.Is(err, kubeprovider.ErrVolumeCapacityShrinkForbidden):
		return volume.CodeCapacityShrinkForbidden, true
	case errors.Is(err, kubeprovider.ErrVolumeExpansionUnsupported):
		return volume.CodeExpansionUnsupported, true
	case errors.Is(err, kubeprovider.ErrVolumeSnapshotUnsupported):
		return volume.CodeSnapshotUnsupported, true
	case errors.Is(err, kubeprovider.ErrVolumeSnapshotNotFound):
		return volume.CodeSnapshotNotFound, true
	case errors.Is(err, kubeprovider.ErrProjectVolumeClaimInUse):
		return volume.CodeInUse, false
	default:
		return volume.CodeClusterUnavailable, false
	}
}

func volumeTaskAttemptExhausted(ctx context.Context) bool {
	retry, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxRetryOK := asynq.GetMaxRetry(ctx)
	return retryOK && maxRetryOK && retry >= maxRetry
}

func safeVolumeTaskError(err error) error {
	if code := volume.ErrorCode(err); code != "" {
		return safeVolumeTaskCode(code)
	}
	return safeVolumeTaskCode(volume.CodeClusterUnavailable)
}

func safeVolumeTaskCode(code string) error {
	return errors.New(strings.TrimSpace(code))
}

func skipVolumeTask(code string) error {
	return fmt.Errorf("%w: %s", asynq.SkipRetry, strings.TrimSpace(code))
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
