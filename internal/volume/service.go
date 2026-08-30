package volume

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
)

type Service struct {
	repository     Repository
	dispatcher     OperationDispatcher
	claimInspector ExistingClaimInspector
}

func NewService(repository Repository, dispatchers ...OperationDispatcher) *Service {
	if repository == nil {
		panic("volume repository is required")
	}
	service := &Service{repository: repository}
	if len(dispatchers) > 0 {
		service.dispatcher = dispatchers[0]
	}
	return service
}

func NewGormService(db *gorm.DB, dispatchers ...OperationDispatcher) *Service {
	if db == nil {
		panic("volume database is required")
	}
	return NewService(NewGormRepository(db), dispatchers...)
}

func NewServiceWithDependencies(repository Repository, dispatcher OperationDispatcher, claimInspector ExistingClaimInspector) *Service {
	service := NewService(repository, dispatcher)
	service.claimInspector = claimInspector
	return service
}

func (service *Service) WithExistingClaimInspector(inspector ExistingClaimInspector) *Service {
	service.claimInspector = inspector
	return service
}

func (service *Service) ListProjectVolumes(ctx context.Context, projectID string, options ProjectVolumeListOptions) (result ProjectVolumeListResult, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "list")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectVolumeListResult{}, newDomainError(CodeInvalidInput, "project id is required")
	}
	options, err = normalizeProjectVolumeListOptions(options)
	if err != nil {
		return ProjectVolumeListResult{}, err
	}
	return service.repository.ListProjectVolumes(ctx, projectID, options)
}

func (service *Service) GetProjectVolume(ctx context.Context, projectID, volumeID string) (result model.ProjectVolume, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "get")
	defer func() { end(err) }()
	result, err = service.repository.GetProjectVolume(ctx, strings.TrimSpace(projectID), strings.TrimSpace(volumeID))
	return result, err
}

// GetProjectVolumeForMaintenance resolves a globally unique volume ID for an
// authenticated internal maintenance task. Request-facing callers must use
// GetProjectVolume so project scope remains explicit.
func (service *Service) GetProjectVolumeForMaintenance(ctx context.Context, volumeID string) (result model.ProjectVolume, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "maintenance.get")
	defer func() { end(err) }()
	volumeID = strings.TrimSpace(volumeID)
	if volumeID == "" {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "volume id is required")
	}
	result, err = service.repository.GetProjectVolumeForMaintenance(ctx, volumeID)
	return result, err
}

func (service *Service) GetProjectVolumeDetail(ctx context.Context, projectID, volumeID string) (ProjectVolumeDetail, error) {
	return service.GetProjectVolumeDetailPage(ctx, projectID, volumeID, 1, DefaultPageSize, 1, DefaultPageSize)
}

func (service *Service) GetProjectVolumeDetailPage(ctx context.Context, projectID, volumeID string, bindingPage, bindingPageSize, transferPage, transferPageSize int) (result ProjectVolumeDetail, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "detail")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	volumeID = strings.TrimSpace(volumeID)
	if projectID == "" || volumeID == "" {
		return ProjectVolumeDetail{}, newDomainError(CodeInvalidInput, "project id and volume id are required")
	}
	bindingPage, bindingPageSize = normalizeRepositoryPage(bindingPage, bindingPageSize)
	transferPage, transferPageSize = normalizeRepositoryPage(transferPage, transferPageSize)
	result.Volume, err = service.repository.GetProjectVolume(ctx, projectID, volumeID)
	if err != nil {
		return ProjectVolumeDetail{}, normalizeRepositoryError(err)
	}
	result.Bindings, result.BindingTotal, err = service.repository.ListProjectVolumeMounts(ctx, projectID, volumeID, nil, bindingPage, bindingPageSize)
	if err != nil {
		return ProjectVolumeDetail{}, err
	}
	transfers, err := service.repository.ListVolumeTransfers(ctx, projectID, VolumeTransferListOptions{
		Page: transferPage, PageSize: transferPageSize, SortBy: "createdAt", SortOrder: "desc", VolumeID: volumeID,
	})
	if err != nil {
		return ProjectVolumeDetail{}, err
	}
	result.BindingPage = bindingPage
	result.BindingPageSize = bindingPageSize
	result.BindingTotalPages = pageCount(result.BindingTotal, bindingPageSize)
	result.Transfers = transfers.Items
	result.TransferPage = transfers.Page
	result.TransferPageSize = transfers.PageSize
	result.TransferTotal = transfers.Total
	result.TransferTotalPages = transfers.TotalPages
	return result, nil
}

func (service *Service) PreviewProjectVolumeDeletion(ctx context.Context, projectID, volumeID string) (result ProjectVolumeDeletionPreview, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "delete.preview")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	volumeID = strings.TrimSpace(volumeID)
	if projectID == "" || volumeID == "" {
		return ProjectVolumeDeletionPreview{}, newDomainError(CodeInvalidInput, "project id and volume id are required")
	}
	result.Volume, err = service.repository.GetProjectVolume(ctx, projectID, volumeID)
	if err != nil {
		return ProjectVolumeDeletionPreview{}, normalizeRepositoryError(err)
	}
	result.BlockingBindings, result.BlockingBindingCount, err = service.repository.ListProjectVolumeMounts(ctx, projectID, volumeID, []string{
		model.DeploymentVolumeActivationReserved,
		model.DeploymentVolumeActivationActive,
		model.DeploymentVolumeActivationReleasePending,
		model.DeploymentVolumeActivationError,
	}, 1, MaxPageSize)
	if err != nil {
		return ProjectVolumeDeletionPreview{}, err
	}
	result.BlockingTransfers, result.BlockingTransferCount, err = service.repository.ListBlockingVolumeTransfers(ctx, projectID, volumeID, 1, MaxPageSize)
	if err != nil {
		return ProjectVolumeDeletionPreview{}, err
	}
	if result.Volume.OwnershipMode == model.ProjectVolumeOwnershipReferenced {
		result.RequiredDataAction = "detach"
	} else {
		result.RequiredDataAction = "delete"
	}
	result.CanDelete = result.BlockingBindingCount == 0 && result.BlockingTransferCount == 0
	return result, nil
}

func (service *Service) CreateProjectVolume(ctx context.Context, input CreateProjectVolumeInput) (result CreateProjectVolumeResult, err error) {
	input = normalizeCreateProjectVolumeInput(input)
	metricStartedAt := time.Now()
	sourceKindAttribute := input.SourceKind
	if !validSourceKind(sourceKindAttribute) {
		sourceKindAttribute = "invalid"
	}
	ctx, end := telemetry.StartOperation(ctx, "volume", "create",
		attribute.String("volume.source_kind", sourceKindAttribute),
	)
	defer func() {
		end(err)
		recordVolumeOperationMetrics(ctx, "create", sourceKindAttribute, metricStartedAt, err)
	}()
	if err = validateCreateProjectVolumeInput(input); err != nil {
		return CreateProjectVolumeResult{}, err
	}

	keyHash := hashValue(input.IdempotencyKey)
	requestHash, err := hashCreateProjectVolumeRequest(input)
	if err != nil {
		return CreateProjectVolumeResult{}, err
	}
	if existing, findErr := service.repository.FindProjectVolumeByIdempotency(ctx, input.ProjectID, keyHash); findErr == nil {
		return service.replayProjectVolumeCreate(ctx, input, existing, requestHash)
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) && ErrorCode(findErr) != CodeNotFound {
		return CreateProjectVolumeResult{}, normalizeRepositoryError(findErr)
	}

	volumeID := id.New("pvol")
	if input.SourceKind == model.ProjectVolumeSourceExistingClaim {
		input, err = service.inspectExistingClaim(ctx, volumeID, input)
		if err != nil {
			return CreateProjectVolumeResult{}, err
		}
	}
	claimName := input.ClaimName
	if claimName == "" {
		claimName = generatedClaimName(volumeID)
	}
	volume := model.ProjectVolume{
		ID:                       volumeID,
		ProjectID:                input.ProjectID,
		DisplayName:              input.DisplayName,
		ClusterID:                input.ClusterID,
		Namespace:                input.Namespace,
		ClaimName:                claimName,
		OwnershipMode:            input.OwnershipMode,
		SourceKind:               input.SourceKind,
		SourceSnapshotName:       input.SourceSnapshotName,
		LifecycleState:           model.ProjectVolumeLifecycleProvisioning,
		PendingOperation:         OperationProvision,
		CapacityRequest:          input.CapacityRequest,
		CapacityBytes:            input.CapacityBytes,
		StorageClassName:         input.StorageClassName,
		AccessMode:               input.AccessMode,
		VolumeMode:               input.VolumeMode,
		SourceApplicationID:      optionalString(input.SourceApplicationID),
		SourceApplicationName:    input.SourceApplicationName,
		SourceDeploymentTargetID: optionalString(input.SourceDeploymentTargetID),
		CreatedBy:                input.ActorID,
		Revision:                 1,
		IdempotencyKeyHash:       keyHash,
		IdempotencyRequestHash:   requestHash,
	}
	if input.SourceKind == model.ProjectVolumeSourceArchiveImport {
		volume.PendingOperation = OperationImport
	}

	err = service.repository.Transaction(ctx, func(repository Repository) error {
		existing, findErr := repository.FindProjectVolumeByIdempotency(ctx, input.ProjectID, keyHash)
		if findErr == nil {
			if existing.IdempotencyRequestHash != requestHash {
				return newDomainError(CodeIdempotencyConflict, "idempotency key was used for a different project volume request")
			}
			result = CreateProjectVolumeResult{Volume: existing, Replayed: true}
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) && ErrorCode(findErr) != CodeNotFound {
			return findErr
		}
		if createErr := repository.CreateProjectVolume(ctx, &volume); createErr != nil {
			return normalizeRepositoryError(createErr)
		}
		result = CreateProjectVolumeResult{Volume: volume}
		return nil
	})
	if err != nil {
		// Resolve a concurrent insert of the same idempotency key into a replay.
		if ErrorCode(err) == CodeIdempotencyConflict {
			existing, findErr := service.repository.FindProjectVolumeByIdempotency(ctx, input.ProjectID, keyHash)
			if findErr == nil && existing.IdempotencyRequestHash == requestHash {
				return service.replayProjectVolumeCreate(ctx, input, existing, requestHash)
			}
		}
		return CreateProjectVolumeResult{}, err
	}
	if result.Replayed {
		return service.replayProjectVolumeCreate(ctx, input, result.Volume, requestHash)
	}
	if input.SourceKind == model.ProjectVolumeSourceArchiveImport {
		return result, nil
	}
	if err = service.dispatch(ctx, VolumeOperation{
		Kind: OperationProvision, ProjectID: input.ProjectID, VolumeID: result.Volume.ID, ActorID: input.ActorID,
	}); err != nil {
		_, _ = service.repository.TransitionProjectVolume(ctx, input.ProjectID, result.Volume.ID,
			[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleError, CodeTaskEnqueueFailed, err.Error())
		return result, err
	}
	return result, nil
}

func (service *Service) replayProjectVolumeCreate(ctx context.Context, input CreateProjectVolumeInput, existing model.ProjectVolume, requestHash string) (CreateProjectVolumeResult, error) {
	if existing.IdempotencyRequestHash != requestHash {
		return CreateProjectVolumeResult{}, newDomainError(CodeIdempotencyConflict, "idempotency key was used for a different project volume request")
	}
	result := CreateProjectVolumeResult{Volume: existing, Replayed: true}
	if existing.LifecycleState != model.ProjectVolumeLifecycleError ||
		existing.LastErrorCode != CodeTaskEnqueueFailed || existing.PendingOperation != OperationProvision {
		return result, nil
	}

	retrying, err := service.RetryProjectVolumeOperation(ctx, input.ProjectID, existing.ID, input.ActorID, existing.Revision)
	if err != nil {
		if ErrorCode(err) == CodeRevisionConflict || ErrorCode(err) == CodeStateConflict {
			current, findErr := service.repository.FindProjectVolumeByIdempotency(ctx, input.ProjectID, hashValue(input.IdempotencyKey))
			if findErr == nil && current.IdempotencyRequestHash == requestHash {
				result.Volume = current
				if current.LifecycleState == model.ProjectVolumeLifecycleError &&
					current.LastErrorCode == CodeTaskEnqueueFailed && current.PendingOperation == OperationProvision {
					return result, newDomainError(CodeTaskEnqueueFailed, "volume operation could not be queued")
				}
				return result, nil
			}
		}
		result.Volume = retrying
		return result, err
	}
	result.Volume = retrying
	return result, nil
}

func (service *Service) UpdateProjectVolume(ctx context.Context, projectID, volumeID string, expectedRevision int64, input UpdateProjectVolumeInput) (result model.ProjectVolume, err error) {
	metricStartedAt := time.Now()
	metricSourceKind := "unknown"
	ctx, end := telemetry.StartOperation(ctx, "volume", "update")
	defer func() {
		end(err)
		recordVolumeOperationMetrics(ctx, "update", metricSourceKind, metricStartedAt, err)
	}()
	projectID = strings.TrimSpace(projectID)
	volumeID = strings.TrimSpace(volumeID)
	if projectID == "" || volumeID == "" || expectedRevision < 1 {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "project id, volume id, and a positive revision are required")
	}
	input.ActorID = strings.TrimSpace(input.ActorID)
	if input.ActorID == "" {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "actor id is required")
	}
	if input.DisplayName == nil && input.CapacityBytes == nil && input.CapacityRequest == nil {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "at least one project volume field must be updated")
	}
	if (input.CapacityBytes == nil) != (input.CapacityRequest == nil) {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "capacity and normalized capacity bytes must be updated together")
	}

	capacityChanged := false
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		volume, lockErr := repository.LockProjectVolume(ctx, projectID, volumeID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		metricSourceKind = volume.SourceKind
		if volume.Revision != expectedRevision {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		if input.DisplayName != nil {
			name := strings.TrimSpace(*input.DisplayName)
			if !validDisplayName(name) {
				return newDomainError(CodeInvalidInput, "project volume display name is invalid")
			}
			volume.DisplayName = name
		}
		if input.CapacityBytes != nil {
			if volume.OwnershipMode != model.ProjectVolumeOwnershipManaged {
				return newDomainError(CodeOwnershipConflict, "referenced project volumes cannot be expanded by the platform")
			}
			if *input.CapacityBytes < volume.CapacityBytes {
				return newDomainError(CodeCapacityShrinkForbidden, "project volume capacity cannot be reduced")
			}
			if *input.CapacityBytes <= 0 || strings.TrimSpace(*input.CapacityRequest) == "" {
				return newDomainError(CodeInvalidInput, "project volume capacity is invalid")
			}
			capacityChanged = *input.CapacityBytes > volume.CapacityBytes
			volume.CapacityBytes = *input.CapacityBytes
			volume.CapacityRequest = strings.TrimSpace(*input.CapacityRequest)
			if capacityChanged {
				volume.PendingOperation = OperationExpand
			}
		}
		updated, updateErr := repository.UpdateProjectVolume(ctx, &volume, expectedRevision)
		if updateErr != nil {
			return normalizeRepositoryError(updateErr)
		}
		if !updated {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		result, updateErr = repository.GetProjectVolume(ctx, projectID, volumeID)
		return normalizeRepositoryError(updateErr)
	})
	if err != nil {
		return model.ProjectVolume{}, err
	}
	if capacityChanged {
		if err = service.dispatch(ctx, VolumeOperation{Kind: OperationExpand, ProjectID: projectID, VolumeID: volumeID, ActorID: input.ActorID}); err != nil {
			_, _ = service.repository.TransitionProjectVolume(ctx, projectID, volumeID,
				[]string{model.ProjectVolumeLifecycleReady}, model.ProjectVolumeLifecycleError, CodeTaskEnqueueFailed, err.Error())
			return result, err
		}
	}
	return result, nil
}

func (service *Service) RequestDeleteProjectVolume(ctx context.Context, input DeleteProjectVolumeInput) (result model.ProjectVolume, detached bool, err error) {
	metricStartedAt := time.Now()
	metricSourceKind := "unknown"
	ctx, end := telemetry.StartOperation(ctx, "volume", "delete")
	defer func() {
		end(err)
		recordVolumeOperationMetrics(ctx, "delete", metricSourceKind, metricStartedAt, err)
	}()
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.VolumeID = strings.TrimSpace(input.VolumeID)
	input.ActorID = strings.TrimSpace(input.ActorID)
	input.DataAction = strings.ToLower(strings.TrimSpace(input.DataAction))
	if input.ProjectID == "" || input.VolumeID == "" || input.ActorID == "" || input.ExpectedRevision < 1 {
		return model.ProjectVolume{}, false, newDomainError(CodeInvalidInput, "project id, volume id, actor id, and revision are required")
	}

	err = service.repository.Transaction(ctx, func(repository Repository) error {
		volume, lockErr := repository.LockProjectVolume(ctx, input.ProjectID, input.VolumeID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		metricSourceKind = volume.SourceKind
		if volume.Revision != input.ExpectedRevision {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		if volume.OwnershipMode == model.ProjectVolumeOwnershipManaged && input.DataAction != "delete" {
			return newDomainError(CodeOwnershipConflict, "managed project volumes require dataAction=delete")
		}
		if volume.OwnershipMode == model.ProjectVolumeOwnershipReferenced && input.DataAction != "detach" {
			return newDomainError(CodeOwnershipConflict, "referenced project volumes require dataAction=detach")
		}
		mountCount, countErr := repository.CountBlockingMounts(ctx, volume.ID)
		if countErr != nil {
			return countErr
		}
		transferCount, countErr := repository.CountActiveTransfers(ctx, volume.ID)
		if countErr != nil {
			return countErr
		}
		if mountCount > 0 || transferCount > 0 {
			return newDomainError(CodeInUse, "project volume has active mounts or transfers")
		}
		if volume.OwnershipMode == model.ProjectVolumeOwnershipReferenced {
			deleted, deleteErr := repository.SoftDeleteProjectVolume(ctx, volume.ProjectID, volume.ID, volume.Revision)
			if deleteErr != nil {
				return normalizeRepositoryError(deleteErr)
			}
			if !deleted {
				return newDomainError(CodeRevisionConflict, "project volume revision changed")
			}
			volume.Revision++
			result = volume
			detached = true
			return nil
		}
		if !CanTransitionProjectVolume(volume.LifecycleState, model.ProjectVolumeLifecycleDeleting) {
			return newDomainError(CodeStateConflict, "project volume cannot enter deleting state")
		}
		volume.LifecycleState = model.ProjectVolumeLifecycleDeleting
		volume.PendingOperation = OperationDelete
		volume.LastErrorCode = ""
		volume.LastErrorMessage = ""
		updated, updateErr := repository.UpdateProjectVolume(ctx, &volume, volume.Revision)
		if updateErr != nil {
			return normalizeRepositoryError(updateErr)
		}
		if !updated {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		result, updateErr = repository.GetProjectVolume(ctx, input.ProjectID, input.VolumeID)
		return normalizeRepositoryError(updateErr)
	})
	if err != nil || detached {
		return result, detached, err
	}
	if err = service.dispatch(ctx, VolumeOperation{
		Kind: OperationDelete, ProjectID: input.ProjectID, VolumeID: input.VolumeID, ActorID: input.ActorID,
	}); err != nil {
		_, _ = service.repository.TransitionProjectVolume(ctx, input.ProjectID, input.VolumeID,
			[]string{model.ProjectVolumeLifecycleDeleting}, model.ProjectVolumeLifecycleError, CodeTaskEnqueueFailed, err.Error())
		return result, false, err
	}
	return result, false, nil
}

func (service *Service) RetryProjectVolumeOperation(ctx context.Context, projectID, volumeID, actorID string, expectedRevision int64) (result model.ProjectVolume, err error) {
	metricStartedAt := time.Now()
	metricSourceKind := "unknown"
	ctx, end := telemetry.StartOperation(ctx, "volume", "retry")
	defer func() {
		end(err)
		recordVolumeOperationMetrics(ctx, "retry", metricSourceKind, metricStartedAt, err)
	}()
	projectID = strings.TrimSpace(projectID)
	volumeID = strings.TrimSpace(volumeID)
	actorID = strings.TrimSpace(actorID)
	if projectID == "" || volumeID == "" || actorID == "" || expectedRevision < 1 {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "project id, volume id, actor id, and revision are required")
	}
	operation := ""
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		volume, lockErr := repository.LockProjectVolume(ctx, projectID, volumeID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		metricSourceKind = volume.SourceKind
		if volume.Revision != expectedRevision {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		if volume.LifecycleState != model.ProjectVolumeLifecycleError {
			return newDomainError(CodeStateConflict, "only failed project volume operations can be retried")
		}
		operation = volume.PendingOperation
		switch operation {
		case OperationProvision, OperationExpand:
			volume.LifecycleState = model.ProjectVolumeLifecycleProvisioning
		case OperationDelete:
			volume.LifecycleState = model.ProjectVolumeLifecycleDeleting
		default:
			return newDomainError(CodeStateConflict, "the failed project volume operation must be retried through its transfer")
		}
		volume.LastErrorCode = ""
		volume.LastErrorMessage = ""
		updated, updateErr := repository.UpdateProjectVolume(ctx, &volume, expectedRevision)
		if updateErr != nil {
			return normalizeRepositoryError(updateErr)
		}
		if !updated {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		result, updateErr = repository.GetProjectVolume(ctx, projectID, volumeID)
		return normalizeRepositoryError(updateErr)
	})
	if err != nil {
		return model.ProjectVolume{}, err
	}
	if err = service.dispatch(ctx, VolumeOperation{Kind: operation, ProjectID: projectID, VolumeID: volumeID, ActorID: actorID}); err != nil {
		failed, transitionErr := service.repository.TransitionProjectVolume(ctx, projectID, volumeID,
			[]string{result.LifecycleState}, model.ProjectVolumeLifecycleError, CodeTaskEnqueueFailed, err.Error())
		if transitionErr == nil {
			result = failed
		}
		return result, err
	}
	return result, nil
}

func (service *Service) CompleteProjectVolumeDeletion(ctx context.Context, projectID, volumeID string) (result model.ProjectVolume, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "delete")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	volumeID = strings.TrimSpace(volumeID)
	if projectID == "" || volumeID == "" {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "project id and volume id are required")
	}
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		volume, lockErr := repository.LockProjectVolume(ctx, projectID, volumeID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		if volume.OwnershipMode != model.ProjectVolumeOwnershipManaged ||
			volume.LifecycleState != model.ProjectVolumeLifecycleDeleting || volume.PendingOperation != OperationDelete {
			return newDomainError(CodeStateConflict, "project volume is not awaiting managed data deletion")
		}
		mountCount, countErr := repository.CountBlockingMounts(ctx, volume.ID)
		if countErr != nil {
			return countErr
		}
		transferCount, countErr := repository.CountActiveTransfers(ctx, volume.ID)
		if countErr != nil {
			return countErr
		}
		if mountCount > 0 || transferCount > 0 {
			return newDomainError(CodeInUse, "project volume has active mounts or transfers")
		}
		deleted, deleteErr := repository.SoftDeleteProjectVolume(ctx, projectID, volumeID, volume.Revision)
		if deleteErr != nil {
			return normalizeRepositoryError(deleteErr)
		}
		if !deleted {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		volume.Revision++
		result = volume
		return nil
	})
	return result, err
}

func (service *Service) SetProjectVolumeLifecycle(ctx context.Context, projectID, volumeID string, from []string, to, errorCode, internalMessage string) (result model.ProjectVolume, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "state_transition")
	defer func() { end(err) }()
	if len(from) == 0 || !allProjectVolumeTransitionsAllowed(from, to) {
		return model.ProjectVolume{}, newDomainError(CodeStateConflict, "project volume lifecycle transition is not allowed")
	}
	result, err = service.repository.TransitionProjectVolume(ctx, strings.TrimSpace(projectID), strings.TrimSpace(volumeID), from, to,
		strings.TrimSpace(errorCode), internalMessage)
	return result, err
}

func (service *Service) ReserveDeploymentVolumeMount(ctx context.Context, input ReserveMountInput) (result model.DeploymentVolumeMount, err error) {
	metricStartedAt := time.Now()
	metricSourceKind := "empty_dir"
	ctx, end := telemetry.StartOperation(ctx, "volume", "bind")
	defer func() {
		end(err)
		recordVolumeOperationMetrics(ctx, "bind", metricSourceKind, metricStartedAt, err)
	}()
	input = normalizeReserveMountInput(input)
	if err = validateReserveMountInput(input); err != nil {
		return model.DeploymentVolumeMount{}, err
	}

	err = service.repository.Transaction(ctx, func(repository Repository) error {
		target, lockErr := repository.LockDeploymentTarget(ctx, input.ProjectID, input.DeploymentTargetID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		if target.ApplicationID != input.ApplicationID {
			return newDomainError(CodeInvalidInput, "deployment target does not belong to the application")
		}
		existing, listErr := repository.ListDeploymentTargetMounts(ctx, input.ProjectID, input.DeploymentTargetID)
		if listErr != nil {
			return listErr
		}
		if mountConflicts(existing, input) {
			return newDomainError(CodeBindingConflict, "deployment volume path or logical name conflicts with an existing mount")
		}

		mount := model.DeploymentVolumeMount{
			ID:                 id.New("dvmt"),
			ProjectID:          input.ProjectID,
			ApplicationID:      input.ApplicationID,
			DeploymentTargetID: input.DeploymentTargetID,
			SourceType:         input.SourceType,
			LogicalName:        input.LogicalName,
			ReadOnly:           input.ReadOnly,
			ActivationState:    model.DeploymentVolumeActivationReserved,
			EmptyDirMedium:     input.EmptyDirMedium,
			EmptyDirSizeLimit:  input.EmptyDirSizeLimit,
			MountPath:          optionalString(input.MountPath),
			DevicePath:         optionalString(input.DevicePath),
		}
		if input.SourceType == model.DeploymentVolumeSourceProjectVolume {
			volume, volumeErr := repository.LockProjectVolume(ctx, input.ProjectID, input.ProjectVolumeID)
			if volumeErr != nil {
				return normalizeRepositoryError(volumeErr)
			}
			metricSourceKind = volume.SourceKind
			if !CanAttachProjectVolume(volume) {
				return newDomainError(CodeStateConflict, "project volume is not attachable")
			}
			if volume.ClusterID != target.ClusterID || (strings.TrimSpace(target.Namespace) != "" && volume.Namespace != target.Namespace) {
				return newDomainError(CodeIncompatibleCluster, "project volume and deployment target must use the same cluster and namespace")
			}
			if err := applyVolumeMountPolicy(&mount, volume); err != nil {
				return err
			}
			mount.ProjectVolumeID = optionalString(volume.ID)
		}
		if createErr := repository.CreateDeploymentVolumeMount(ctx, &mount); createErr != nil {
			return normalizeRepositoryError(createErr)
		}
		result = mount
		return nil
	})
	return result, err
}

func (service *Service) ActivateDeploymentVolumeMount(ctx context.Context, projectID, mountID string) (model.DeploymentVolumeMount, error) {
	return service.transitionMount(ctx, "bind", projectID, mountID,
		[]string{model.DeploymentVolumeActivationReserved, model.DeploymentVolumeActivationError},
		model.DeploymentVolumeActivationActive, "", "")
}

func (service *Service) FailDeploymentVolumeMount(ctx context.Context, projectID, mountID, errorCode, internalMessage string) (model.DeploymentVolumeMount, error) {
	return service.transitionMount(ctx, "bind", projectID, mountID,
		[]string{model.DeploymentVolumeActivationReserved, model.DeploymentVolumeActivationActive},
		model.DeploymentVolumeActivationError, strings.TrimSpace(errorCode), internalMessage)
}

func (service *Service) BeginDeploymentVolumeUnbind(ctx context.Context, projectID, mountID string) (model.DeploymentVolumeMount, error) {
	return service.transitionMount(ctx, "unbind", projectID, mountID,
		[]string{model.DeploymentVolumeActivationReserved, model.DeploymentVolumeActivationActive, model.DeploymentVolumeActivationError},
		model.DeploymentVolumeActivationReleasePending, "", "")
}

func (service *Service) CompleteDeploymentVolumeUnbind(ctx context.Context, projectID, mountID string) (err error) {
	metricStartedAt := time.Now()
	ctx, end := telemetry.StartOperation(ctx, "volume", "unbind")
	defer func() {
		end(err)
		recordVolumeOperationMetrics(ctx, "unbind", "unknown", metricStartedAt, err)
	}()
	deleted, err := service.repository.DeleteDeploymentVolumeMount(ctx, strings.TrimSpace(projectID), strings.TrimSpace(mountID),
		[]string{model.DeploymentVolumeActivationReleasePending})
	if err != nil {
		return normalizeRepositoryError(err)
	}
	if !deleted {
		return newDomainError(CodeStateConflict, "deployment volume mount is not awaiting release")
	}
	return nil
}

func (service *Service) transitionMount(ctx context.Context, operation, projectID, mountID string, from []string, to, errorCode, internalMessage string) (result model.DeploymentVolumeMount, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", operation)
	defer func() { end(err) }()
	result, err = service.repository.TransitionDeploymentVolumeMount(ctx, strings.TrimSpace(projectID), strings.TrimSpace(mountID), from, to, errorCode, internalMessage)
	return result, err
}

func (service *Service) ListVolumeTransfers(ctx context.Context, projectID string, options VolumeTransferListOptions) (result VolumeTransferListResult, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.list")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return VolumeTransferListResult{}, newDomainError(CodeInvalidInput, "project id is required")
	}
	options, err = normalizeVolumeTransferListOptions(options)
	if err != nil {
		return VolumeTransferListResult{}, err
	}
	return service.repository.ListVolumeTransfers(ctx, projectID, options)
}

func (service *Service) GetVolumeTransfer(ctx context.Context, projectID, transferID string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.get")
	defer func() { end(err) }()
	result, err = service.repository.GetVolumeTransfer(ctx, strings.TrimSpace(projectID), strings.TrimSpace(transferID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.VolumeTransfer{}, newDomainError(CodeTransferNotFound, "volume transfer was not found")
	}
	return result, err
}

// GetVolumeTransferForMaintenance resolves a transfer without accepting a
// caller-supplied project identifier. It is intentionally reserved for
// bounded Worker and maintenance paths.
func (service *Service) GetVolumeTransferForMaintenance(ctx context.Context, transferID string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.get_internal")
	defer func() { end(err) }()
	transferID = strings.TrimSpace(transferID)
	if transferID == "" {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer id is required")
	}
	result, err = service.repository.GetVolumeTransferForMaintenance(ctx, transferID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.VolumeTransfer{}, newDomainError(CodeTransferNotFound, "volume transfer was not found")
	}
	return result, err
}

func (service *Service) CreateVolumeTransfer(ctx context.Context, input CreateVolumeTransferInput) (result model.VolumeTransfer, err error) {
	input = normalizeCreateVolumeTransferInput(input)
	directionAttribute := input.Direction
	if !validTransferDirection(directionAttribute) {
		directionAttribute = "invalid"
	}
	formatAttribute := input.Format
	if !oneOf(formatAttribute, model.VolumeTransferFormatTarGZ, model.VolumeTransferFormatRawZST) {
		formatAttribute = "invalid"
	}
	operationName := directionAttribute
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer."+operationName,
		attribute.String("volume.transfer.direction", directionAttribute),
		attribute.String("volume.transfer.format", formatAttribute),
	)
	defer func() { end(err) }()
	if err = validateCreateVolumeTransferInput(input); err != nil {
		return model.VolumeTransfer{}, err
	}

	created := false
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		volume, lockErr := repository.LockProjectVolume(ctx, input.ProjectID, input.ProjectVolumeID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		if transferErr := validateVolumeForTransfer(volume, input); transferErr != nil {
			return transferErr
		}
		transferID := id.New("vtx")
		if input.IdempotencyKey != "" {
			transferID = idempotentVolumeTransferID(input)
			existing, findErr := repository.GetVolumeTransfer(ctx, input.ProjectID, transferID)
			if findErr == nil {
				if !sameVolumeTransferRequest(existing, input) {
					return newDomainError(CodeIdempotencyConflict, "volume transfer idempotency key was used for a different request")
				}
				result = existing
				return nil
			}
			if !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return findErr
			}
		}
		transfer := model.VolumeTransfer{
			ID:              transferID,
			ProjectID:       input.ProjectID,
			ProjectVolumeID: input.ProjectVolumeID,
			Direction:       input.Direction,
			Format:          input.Format,
			ConsistencyMode: input.ConsistencyMode,
			State:           model.VolumeTransferStatePreparing,
			SourceFilename:  input.SourceFilename,
			ExpectedBytes:   input.ExpectedBytes,
			ActorID:         input.ActorID,
			ExpiresAt:       input.ExpiresAt,
		}
		if createErr := repository.CreateVolumeTransfer(ctx, &transfer); createErr != nil {
			return normalizeRepositoryError(createErr)
		}
		created = true
		result = transfer
		return nil
	})
	if err != nil && input.IdempotencyKey != "" {
		existing, findErr := service.repository.GetVolumeTransfer(ctx, input.ProjectID, idempotentVolumeTransferID(input))
		if findErr == nil && sameVolumeTransferRequest(existing, input) {
			result = existing
			err = nil
			created = false
		}
	}
	if err != nil || !created || result.State != model.VolumeTransferStatePreparing {
		return result, err
	}
	if err = service.dispatch(ctx, VolumeOperation{
		Kind: result.Direction, ProjectID: result.ProjectID, VolumeID: result.ProjectVolumeID, TransferID: result.ID, ActorID: result.ActorID,
	}); err != nil {
		_, _ = service.FailVolumeTransferExecution(ctx, result.ProjectID, result.ID, CodeTaskEnqueueFailed, err.Error())
		return result, err
	}
	return result, nil
}

func (service *Service) TransitionVolumeTransfer(ctx context.Context, projectID, transferID, to, errorCode, internalMessage string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.transition")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	to = strings.TrimSpace(to)
	if to == model.VolumeTransferStateSucceeded {
		return model.VolumeTransfer{}, newDomainError(CodeTransferStateConflict, "successful volume transfers must use authoritative finalization")
	}
	var shouldDispatch bool
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		transfer, lockErr := repository.LockVolumeTransfer(ctx, projectID, transferID)
		if lockErr != nil {
			if errors.Is(lockErr, gorm.ErrRecordNotFound) {
				return newDomainError(CodeTransferNotFound, "volume transfer was not found")
			}
			return lockErr
		}
		if !CanTransitionVolumeTransfer(transfer.State, to) {
			return newDomainError(CodeTransferStateConflict, "volume transfer state transition is not allowed")
		}
		result, lockErr = repository.TransitionVolumeTransfer(ctx, projectID, transferID, transfer.State, to, strings.TrimSpace(errorCode), internalMessage)
		shouldDispatch = to == model.VolumeTransferStatePreparing || to == model.VolumeTransferStateCancelled
		return lockErr
	})
	if err != nil || !shouldDispatch {
		return result, err
	}
	if err = service.dispatch(ctx, VolumeOperation{
		Kind: result.Direction, ProjectID: result.ProjectID, VolumeID: result.ProjectVolumeID, TransferID: result.ID, ActorID: result.ActorID,
	}); err != nil {
		if result.State != model.VolumeTransferStateCancelled {
			_, _ = service.FailVolumeTransferExecution(ctx, result.ProjectID, result.ID, CodeTaskEnqueueFailed, err.Error())
		}
		return result, err
	}
	return result, nil
}

func (service *Service) ClaimVolumeTransferExecution(ctx context.Context, projectID, transferID, expectedState, leaseOwner string, leaseExpiresAt time.Time) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.claim_preparation")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	leaseOwner = strings.TrimSpace(leaseOwner)
	now := timeNowUTC()
	if projectID == "" || transferID == "" || expectedState != model.VolumeTransferStatePreparing || len(leaseOwner) < 8 || len(leaseOwner) > 128 || !leaseExpiresAt.After(now) {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer preparation lease is invalid")
	}
	return service.repository.ClaimVolumeTransferExecution(ctx, projectID, transferID, expectedState, leaseOwner, now, leaseExpiresAt)
}

func (service *Service) RenewVolumeTransferExecutionLease(ctx context.Context, projectID, transferID, leaseOwner string, generation int64, leaseExpiresAt time.Time) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.renew_preparation_lease")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	leaseOwner = strings.TrimSpace(leaseOwner)
	now := timeNowUTC()
	if projectID == "" || transferID == "" || len(leaseOwner) < 8 || len(leaseOwner) > 128 || generation < 1 || !leaseExpiresAt.After(now) {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer preparation lease is invalid")
	}
	return service.repository.RenewVolumeTransferExecutionLease(ctx, projectID, transferID, leaseOwner, generation, now, leaseExpiresAt)
}

func (service *Service) ConfirmVolumeTransferJobCreated(ctx context.Context, projectID, transferID string, generation int64) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.confirm_runtime")
	defer func() { end(err) }()
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(transferID) == "" || generation < 1 {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer runtime identity is invalid")
	}
	return service.repository.ConfirmVolumeTransferJobCreated(ctx, strings.TrimSpace(projectID), strings.TrimSpace(transferID), generation)
}

func (service *Service) MarkVolumeTransferReady(ctx context.Context, projectID, transferID string, generation int64) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.ready")
	defer func() { end(err) }()
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(transferID) == "" || generation < 1 {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer ready identity is invalid")
	}
	return service.repository.MarkVolumeTransferReady(ctx, strings.TrimSpace(projectID), strings.TrimSpace(transferID), generation)
}

func (service *Service) ClaimVolumeTransferStream(ctx context.Context, projectID, transferID, direction string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.claim_stream")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	direction = strings.TrimSpace(direction)
	if projectID == "" || transferID == "" || !validTransferDirection(direction) {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer stream identity is invalid")
	}
	return service.repository.ClaimVolumeTransferStream(ctx, projectID, transferID, direction)
}

func (service *Service) CompleteVolumeTransferStream(ctx context.Context, projectID, transferID string, completion TransferCompletion) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.complete_stream")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	completion.SHA256 = strings.ToLower(strings.TrimSpace(completion.SHA256))
	completion.DataSHA256 = strings.ToLower(strings.TrimSpace(completion.DataSHA256))
	if projectID == "" || transferID == "" || completion.ExpectedState != model.VolumeTransferStateStreaming || completion.TransferredBytes < 1 || completion.ProcessedFiles < 0 || !validSHA256(completion.SHA256) {
		return model.VolumeTransfer{}, newDomainError(CodeTransferChecksumInvalid, "volume transfer completion is invalid")
	}
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		transfer, lockErr := repository.LockVolumeTransfer(ctx, projectID, transferID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		if transfer.State != model.VolumeTransferStateStreaming {
			return newDomainError(CodeTransferStateConflict, "volume transfer is not streaming")
		}
		if transfer.Direction == model.VolumeTransferDirectionImport && transfer.ExpectedBytes != completion.TransferredBytes {
			return newDomainError(CodeTransferChecksumMismatch, "volume import content length does not match its prepared transfer")
		}
		if transfer.Format == model.VolumeTransferFormatRawZST {
			if completion.LogicalBytes < 1 || !validSHA256(completion.DataSHA256) {
				return newDomainError(CodeTransferChecksumInvalid, "block transfer data digest is required")
			}
			projectVolume, volumeErr := repository.LockProjectVolume(ctx, projectID, transfer.ProjectVolumeID)
			if volumeErr != nil {
				return normalizeRepositoryError(volumeErr)
			}
			if projectVolume.VolumeMode != model.ProjectVolumeModeBlock || completion.LogicalBytes > projectVolume.CapacityBytes {
				return newDomainError(CodeTransferCapacityExceeded, "block transfer exceeds volume capacity")
			}
		} else if completion.LogicalBytes != 0 || completion.DataSHA256 != "" {
			return newDomainError(CodeTransferChecksumInvalid, "filesystem transfer cannot report a block digest")
		}
		result, lockErr = repository.CompleteVolumeTransferStream(ctx, projectID, transferID, completion)
		if lockErr != nil {
			return lockErr
		}
		if transfer.Direction == model.VolumeTransferDirectionImport {
			projectVolume, volumeErr := repository.LockProjectVolume(ctx, projectID, transfer.ProjectVolumeID)
			if volumeErr != nil {
				return normalizeRepositoryError(volumeErr)
			}
			if projectVolume.LifecycleState != model.ProjectVolumeLifecycleProvisioning || projectVolume.PendingOperation != OperationImport {
				return newDomainError(CodeStateConflict, "import project volume is not awaiting completion")
			}
			_, volumeErr = repository.TransitionProjectVolume(ctx, projectID, projectVolume.ID, []string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleReady, "", "")
			return volumeErr
		}
		return nil
	})
	if err == nil {
		service.dispatchTerminalCleanup(ctx, result)
	}
	return result, err
}

func (service *Service) FailVolumeTransferExecution(ctx context.Context, projectID, transferID, errorCode, internalMessage string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.fail_execution")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	errorCode = strings.TrimSpace(errorCode)
	internalMessage = strings.TrimSpace(internalMessage)
	if projectID == "" || transferID == "" || errorCode == "" || len(errorCode) > 128 {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer failure is invalid")
	}
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		transfer, lockErr := repository.LockVolumeTransfer(ctx, projectID, transferID)
		if lockErr != nil {
			if errors.Is(lockErr, gorm.ErrRecordNotFound) {
				return newDomainError(CodeTransferNotFound, "volume transfer was not found")
			}
			return lockErr
		}
		alreadyFailed := transfer.State == model.VolumeTransferStateFailed
		if alreadyFailed {
			if transfer.LastErrorCode != errorCode {
				return newDomainError(CodeTransferStateConflict, "volume transfer failure differs from the committed result")
			}
			if transfer.LastErrorMessage != "" {
				internalMessage = transfer.LastErrorMessage
			}
		} else if !CanTransitionVolumeTransfer(transfer.State, model.VolumeTransferStateFailed) {
			return newDomainError(CodeTransferStateConflict, "volume transfer cannot enter the failed state")
		}

		var projectVolume model.ProjectVolume
		transitionVolume := false
		if transfer.Direction == model.VolumeTransferDirectionImport {
			projectVolume, lockErr = repository.LockProjectVolume(ctx, projectID, transfer.ProjectVolumeID)
			if lockErr != nil {
				return normalizeRepositoryError(lockErr)
			}
			if projectVolume.SourceKind != model.ProjectVolumeSourceArchiveImport {
				return newDomainError(CodeStateConflict, "import project volume source changed")
			}
			switch {
			case projectVolume.LifecycleState == model.ProjectVolumeLifecycleProvisioning && projectVolume.PendingOperation == OperationImport:
				transitionVolume = true
			case projectVolume.LifecycleState == model.ProjectVolumeLifecycleError && projectVolume.PendingOperation == OperationImport:
			case projectVolume.LifecycleState == model.ProjectVolumeLifecycleDeleting:
				return newDomainError(CodeStateConflict, "import project volume is being deleted")
			default:
				return newDomainError(CodeStateConflict, "import project volume is not awaiting transfer completion")
			}
		}
		if !alreadyFailed {
			result, lockErr = repository.TransitionVolumeTransfer(ctx, projectID, transferID, transfer.State,
				model.VolumeTransferStateFailed, errorCode, internalMessage)
			if lockErr != nil {
				return lockErr
			}
		} else {
			result = transfer
		}
		if transitionVolume {
			if _, lockErr = repository.TransitionProjectVolume(ctx, projectID, projectVolume.ID,
				[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleError,
				errorCode, internalMessage); lockErr != nil {
				return lockErr
			}
		}
		return nil
	})
	if err == nil {
		service.dispatchTerminalCleanup(ctx, result)
	}
	return result, err
}

// FailStaleVolumeTransfer atomically lets the stream heartbeat or the stale
// recovery worker win. A zero-row compare-and-swap is returned as a stable
// state conflict so the worker must not clean a live transfer Pod.
func (service *Service) FailStaleVolumeTransfer(ctx context.Context, projectID, transferID string, cutoff time.Time, errorCode, internalMessage string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.fail_stale")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	errorCode = strings.TrimSpace(errorCode)
	internalMessage = strings.TrimSpace(internalMessage)
	if projectID == "" || transferID == "" || cutoff.IsZero() || errorCode == "" || len(errorCode) > 128 {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "stale volume transfer failure is invalid")
	}
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		var failErr error
		result, failErr = repository.FailStaleVolumeTransfer(ctx, projectID, transferID, cutoff, errorCode, internalMessage)
		if failErr != nil {
			return failErr
		}
		if result.Direction != model.VolumeTransferDirectionImport {
			return nil
		}
		projectVolume, volumeErr := repository.LockProjectVolume(ctx, projectID, result.ProjectVolumeID)
		if volumeErr != nil {
			return normalizeRepositoryError(volumeErr)
		}
		if projectVolume.LifecycleState == model.ProjectVolumeLifecycleProvisioning && projectVolume.PendingOperation == OperationImport {
			_, volumeErr = repository.TransitionProjectVolume(ctx, projectID, projectVolume.ID,
				[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleError, errorCode, internalMessage)
		}
		return volumeErr
	})
	return result, err
}

// MarkVolumeTransferExecutionCleanupCompleted persists proof that all
// execution-scoped Kubernetes resources were removed.
func (service *Service) MarkVolumeTransferExecutionCleanupCompleted(ctx context.Context, projectID, transferID string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.mark_execution_cleanup")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	if projectID == "" || transferID == "" {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "project id and volume transfer id are required")
	}
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		transfer, lockErr := repository.LockVolumeTransfer(ctx, projectID, transferID)
		if lockErr != nil {
			if errors.Is(lockErr, gorm.ErrRecordNotFound) {
				return newDomainError(CodeTransferNotFound, "volume transfer was not found")
			}
			return lockErr
		}
		if transfer.ExecutionCleanupCompletedAt != nil {
			if !IsVolumeTransferTerminal(transfer.State) {
				return newDomainError(CodeTransferStateConflict, "active volume transfer has an execution cleanup marker")
			}
			result = transfer
			return nil
		}
		if !IsVolumeTransferTerminal(transfer.State) {
			return newDomainError(CodeTransferStateConflict, "volume transfer execution is not terminal")
		}
		result, lockErr = repository.MarkVolumeTransferExecutionCleanupCompleted(ctx, projectID, transferID)
		return lockErr
	})
	return result, err
}

// CompleteCancelledVolumeImport removes the platform asset only after the
// Worker has cancelled and cleaned the import Job and its partial PVC. It does
// not delete Kubernetes resources itself and therefore must never be called
// before that authoritative cleanup has completed.
func (service *Service) CompleteCancelledVolumeImport(ctx context.Context, projectID, volumeID, transferID string) (result model.ProjectVolume, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.cancel_import_complete")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	volumeID = strings.TrimSpace(volumeID)
	transferID = strings.TrimSpace(transferID)
	if projectID == "" || volumeID == "" || transferID == "" {
		return model.ProjectVolume{}, newDomainError(CodeInvalidInput, "project id, volume id, and transfer id are required")
	}
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		transfer, lockErr := repository.LockVolumeTransfer(ctx, projectID, transferID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		if transfer.Direction != model.VolumeTransferDirectionImport || transfer.State != model.VolumeTransferStateCancelled || transfer.ProjectVolumeID != volumeID {
			return newDomainError(CodeTransferStateConflict, "cancelled import transfer state changed")
		}
		if transfer.ExecutionCleanupCompletedAt == nil {
			return newDomainError(CodeDeletionPending, "cancelled import execution cleanup is pending")
		}
		projectVolume, lockErr := repository.LockProjectVolume(ctx, projectID, volumeID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		if projectVolume.SourceKind != model.ProjectVolumeSourceArchiveImport ||
			(projectVolume.LifecycleState != model.ProjectVolumeLifecycleProvisioning && projectVolume.LifecycleState != model.ProjectVolumeLifecycleError) {
			return newDomainError(CodeStateConflict, "cancelled import project volume state changed")
		}
		mountCount, countErr := repository.CountBlockingMounts(ctx, volumeID)
		if countErr != nil {
			return countErr
		}
		activeTransfers, countErr := repository.CountActiveTransfers(ctx, volumeID)
		if countErr != nil {
			return countErr
		}
		if mountCount != 0 || activeTransfers != 0 {
			return newDomainError(CodeInUse, "cancelled import project volume is still in use")
		}
		deleted, deleteErr := repository.SoftDeleteProjectVolume(ctx, projectID, volumeID, projectVolume.Revision)
		if deleteErr != nil {
			return normalizeRepositoryError(deleteErr)
		}
		if !deleted {
			return newDomainError(CodeRevisionConflict, "project volume revision changed")
		}
		projectVolume.Revision++
		result = projectVolume
		return nil
	})
	return result, err
}

func (service *Service) UpdateVolumeTransferProgress(ctx context.Context, projectID, transferID string, progress TransferProgress) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.progress")
	defer func() { end(err) }()
	progress.Phase = strings.TrimSpace(progress.Phase)
	if progress.TransferredBytes < 0 || progress.ProcessedFiles < 0 || len(progress.Phase) > 128 {
		return model.VolumeTransfer{}, newDomainError(CodeTransferProgressInvalid, "volume transfer progress is invalid")
	}
	result, err = service.repository.UpdateVolumeTransferProgress(ctx, strings.TrimSpace(projectID), strings.TrimSpace(transferID), progress)
	return result, normalizeRepositoryError(err)
}

func (service *Service) ListStaleProjectVolumeOperations(ctx context.Context, options MaintenanceScanOptions) ([]model.ProjectVolume, error) {
	if options.Cutoff.IsZero() {
		return nil, newDomainError(CodeInvalidInput, "maintenance cutoff is required")
	}
	return service.repository.ListStaleProjectVolumes(ctx, options.Cutoff, normalizeMaintenanceLimit(options.Limit))
}

func (service *Service) ListStaleVolumeTransferOperations(ctx context.Context, options MaintenanceScanOptions) ([]model.VolumeTransfer, error) {
	if options.Cutoff.IsZero() {
		return nil, newDomainError(CodeInvalidInput, "maintenance cutoff is required")
	}
	return service.repository.ListStaleVolumeTransfers(ctx, options.Cutoff, normalizeMaintenanceLimit(options.Limit))
}

func (service *Service) ListExpiredVolumeTransfers(ctx context.Context, now time.Time, limit int) ([]model.VolumeTransfer, error) {
	if now.IsZero() {
		return nil, newDomainError(CodeInvalidInput, "volume transfer expiry time is required")
	}
	return service.repository.ListExpiredVolumeTransfers(ctx, now, normalizeMaintenanceLimit(limit))
}

func (service *Service) ExpireVolumeTransfer(ctx context.Context, projectID, transferID string, now time.Time) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer.expire")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	transferID = strings.TrimSpace(transferID)
	if projectID == "" || transferID == "" || now.IsZero() {
		return model.VolumeTransfer{}, newDomainError(CodeInvalidInput, "volume transfer expiry identity is invalid")
	}
	err = service.repository.Transaction(ctx, func(repository Repository) error {
		transfer, lockErr := repository.LockVolumeTransfer(ctx, projectID, transferID)
		if lockErr != nil {
			return normalizeRepositoryError(lockErr)
		}
		if transfer.State == model.VolumeTransferStateExpired {
			result = transfer
			return nil
		}
		if transfer.ExpiresAt.After(now) || !oneOf(transfer.State, model.VolumeTransferStateCreated, model.VolumeTransferStatePreparing, model.VolumeTransferStateReady) {
			return newDomainError(CodeTransferStateConflict, "volume transfer is not eligible for expiry")
		}
		result, lockErr = repository.TransitionVolumeTransfer(ctx, projectID, transferID, transfer.State,
			model.VolumeTransferStateExpired, CodeTransferExpired, "direct transfer session expired before streaming")
		return lockErr
	})
	return result, err
}

func (service *Service) dispatch(ctx context.Context, operation VolumeOperation) error {
	if service.dispatcher == nil {
		return newDomainError(CodeTaskEnqueueFailed, "volume operation dispatcher is unavailable")
	}
	if err := service.dispatcher.DispatchVolumeOperation(ctx, operation); err != nil {
		return newDomainError(CodeTaskEnqueueFailed, "volume operation could not be queued", err)
	}
	return nil
}

func (service *Service) dispatchTerminalCleanup(ctx context.Context, transfer model.VolumeTransfer) {
	if service == nil || service.dispatcher == nil || transfer.ID == "" || !IsVolumeTransferTerminal(transfer.State) {
		return
	}
	dispatchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := service.dispatcher.DispatchVolumeOperation(dispatchCtx, VolumeOperation{
		Kind: OperationCleanup, ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID,
		TransferID: transfer.ID, ActorID: transfer.ActorID,
	}); err != nil {
		telemetry.LogError(dispatchCtx, "Volume transfer targeted cleanup enqueue failed",
			"volume_transfer.cleanup.enqueue_failed", "volume_transfer.cleanup.enqueue",
			"task.enqueue.failed", err,
			slog.String("resource.type", "volume_transfer"),
			slog.String("resource.id", transfer.ID))
	}
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func idempotentVolumeTransferID(input CreateVolumeTransferInput) string {
	fingerprint := hashValue(strings.Join([]string{
		input.ProjectID, input.ActorID, input.Direction, input.IdempotencyKey,
	}, "\x00"))
	return "vtx_" + fingerprint[:24]
}

func sameVolumeTransferRequest(existing model.VolumeTransfer, input CreateVolumeTransferInput) bool {
	return existing.ProjectID == input.ProjectID &&
		existing.ProjectVolumeID == input.ProjectVolumeID &&
		existing.Direction == input.Direction &&
		existing.Format == input.Format &&
		existing.ConsistencyMode == input.ConsistencyMode &&
		existing.SourceFilename == input.SourceFilename &&
		existing.ExpectedBytes == input.ExpectedBytes &&
		existing.ActorID == input.ActorID &&
		existing.ExpiresAt.UTC().Truncate(time.Microsecond).Equal(input.ExpiresAt.UTC().Truncate(time.Microsecond))
}

func hashCreateProjectVolumeRequest(input CreateProjectVolumeInput) (string, error) {
	canonical := struct {
		ProjectID                string `json:"projectId"`
		DisplayName              string `json:"displayName"`
		ClusterID                string `json:"clusterId"`
		Namespace                string `json:"namespace"`
		ClaimName                string `json:"claimName"`
		OwnershipMode            string `json:"ownershipMode"`
		SourceKind               string `json:"sourceKind"`
		SourceSnapshotName       string `json:"sourceSnapshotName"`
		CapacityRequest          string `json:"capacityRequest"`
		CapacityBytes            int64  `json:"capacityBytes"`
		StorageClassName         string `json:"storageClassName"`
		AccessMode               string `json:"accessMode"`
		VolumeMode               string `json:"volumeMode"`
		SourceApplicationID      string `json:"sourceApplicationId"`
		SourceApplicationName    string `json:"sourceApplicationName"`
		SourceDeploymentTargetID string `json:"sourceDeploymentTargetId"`
		ActorID                  string `json:"actorId"`
	}{
		ProjectID: input.ProjectID, DisplayName: input.DisplayName, ClusterID: input.ClusterID,
		Namespace: input.Namespace, ClaimName: input.ClaimName, OwnershipMode: input.OwnershipMode,
		SourceKind: input.SourceKind, SourceSnapshotName: input.SourceSnapshotName,
		CapacityRequest: input.CapacityRequest, CapacityBytes: input.CapacityBytes,
		StorageClassName: input.StorageClassName, AccessMode: input.AccessMode, VolumeMode: input.VolumeMode,
		SourceApplicationID: input.SourceApplicationID, SourceApplicationName: input.SourceApplicationName,
		SourceDeploymentTargetID: input.SourceDeploymentTargetID, ActorID: input.ActorID,
	}
	if input.SourceKind == model.ProjectVolumeSourceExistingClaim {
		// Existing-claim specifications are authoritative Kubernetes state, not
		// client input. Exclude them from the request fingerprint so a replay is
		// stable even if the claim has since expanded.
		canonical.CapacityRequest = ""
		canonical.CapacityBytes = 0
		canonical.StorageClassName = ""
		canonical.AccessMode = ""
		canonical.VolumeMode = ""
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode project volume idempotency input: %w", err)
	}
	return hashValue(string(encoded)), nil
}

func generatedClaimName(volumeID string) string {
	suffix := strings.TrimPrefix(volumeID, "pvol_")
	if len(suffix) > 16 {
		suffix = suffix[:16]
	}
	return "luna-pvol-" + suffix
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func allProjectVolumeTransitionsAllowed(from []string, to string) bool {
	for _, state := range from {
		if !CanTransitionProjectVolume(state, to) {
			return false
		}
	}
	return true
}

func pageCount(total int64, pageSize int) int {
	if total == 0 || pageSize <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func (service *Service) inspectExistingClaim(ctx context.Context, volumeID string, input CreateProjectVolumeInput) (CreateProjectVolumeInput, error) {
	if service.claimInspector == nil {
		return CreateProjectVolumeInput{}, newDomainError(CodeClusterUnavailable, "existing claim inspection is unavailable")
	}
	inspection, err := service.claimInspector.InspectExistingClaim(ctx, ExistingClaimInspectionInput{
		ProjectID: input.ProjectID,
		VolumeID:  volumeID,
		ClusterID: input.ClusterID,
		Namespace: input.Namespace,
		ClaimName: input.ClaimName,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return CreateProjectVolumeInput{}, err
		case errors.Is(err, ErrExistingClaimNotFound):
			return CreateProjectVolumeInput{}, newDomainError(CodeClaimNotFound, "existing project volume claim was not found", err)
		case errors.Is(err, ErrExistingClaimOwnershipConflict):
			return CreateProjectVolumeInput{}, newDomainError(CodeOwnershipConflict, "existing project volume claim belongs to another project", err)
		case errors.Is(err, ErrExistingClaimSpecConflict):
			return CreateProjectVolumeInput{}, newDomainError(CodeClaimSpecConflict, "existing project volume claim specification is incompatible", err)
		default:
			if ErrorCode(err) != "" {
				return CreateProjectVolumeInput{}, err
			}
			return CreateProjectVolumeInput{}, newDomainError(CodeClusterUnavailable, "existing claim could not be inspected", err)
		}
	}
	if inspection.OwnerProjectID != "" && inspection.OwnerProjectID != input.ProjectID {
		return CreateProjectVolumeInput{}, newDomainError(CodeOwnershipConflict, "existing project volume claim belongs to another project")
	}
	if inspection.OwnerVolumeID != "" {
		return CreateProjectVolumeInput{}, newDomainError(CodeOwnershipConflict, "existing project volume claim is already registered")
	}
	if input.OwnershipMode == model.ProjectVolumeOwnershipManaged {
		if inspection.ActivePodReferences > 0 {
			return CreateProjectVolumeInput{}, newDomainError(CodeOwnershipConflict, "an active workload still references the claim")
		}
		if inspection.ManagedBy != "" && inspection.ManagedBy != "luna-devops" {
			return CreateProjectVolumeInput{}, newDomainError(CodeOwnershipConflict, "existing project volume claim is managed by another controller")
		}
	}
	if inspection.CapacityBytes <= 0 || strings.TrimSpace(inspection.CapacityRequest) == "" ||
		!validAccessMode(inspection.AccessMode) || !validVolumeMode(inspection.VolumeMode) {
		return CreateProjectVolumeInput{}, newDomainError(CodeClaimSpecConflict, "existing project volume claim has an unsupported specification")
	}
	input.CapacityRequest = strings.TrimSpace(inspection.CapacityRequest)
	input.CapacityBytes = inspection.CapacityBytes
	input.StorageClassName = strings.TrimSpace(inspection.StorageClassName)
	input.AccessMode = strings.TrimSpace(inspection.AccessMode)
	input.VolumeMode = strings.TrimSpace(inspection.VolumeMode)
	return input, nil
}
