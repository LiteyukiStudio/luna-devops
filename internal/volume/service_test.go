package volume

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type contextKey string

type repositoryStub struct {
	Repository
	transactionMu       sync.Mutex
	listOptions         ProjectVolumeListOptions
	findVolume          model.ProjectVolume
	findErr             error
	createdVolume       model.ProjectVolume
	createErr           error
	contextValue        any
	contextTraceID      trace.TraceID
	transitionTo        string
	transitionErrorCode string
	mountTransitionFrom []string
	mountTransitionTo   string
	lockedVolume        model.ProjectVolume
	lockedTransfer      model.VolumeTransfer
	createdTransfer     model.VolumeTransfer
	transferByID        model.VolumeTransfer
	transferGetErr      error
	transferTo          string
	transferErrorCode   string
	preparedState       string
	claimedLeaseOwner   string
	claimedLeaseExpiry  time.Time
	renewedLeaseExpiry  time.Time
	preparedTokenHash   string
	preparedExpiry      time.Time
	completedBytes      int64
	completedSHA256     string
	completionReported  bool
	jobSucceeded        bool
	finalized           bool
	cleanupCompleted    bool
	maintenanceLimits   []int
	blockingMounts      int64
	activeTransfers     int64
	softDeleted         bool
	objectDeleted       bool
	transferParts       []model.VolumeTransferPart
	completePartErr     error
}

func (repository *repositoryStub) Transaction(ctx context.Context, fn func(Repository) error) error {
	repository.transactionMu.Lock()
	defer repository.transactionMu.Unlock()
	repository.captureContext(ctx)
	return fn(repository)
}

func (repository *repositoryStub) ListProjectVolumes(ctx context.Context, _ string, options ProjectVolumeListOptions) (ProjectVolumeListResult, error) {
	repository.captureContext(ctx)
	repository.listOptions = options
	return ProjectVolumeListResult{
		Items: []model.ProjectVolume{}, Page: options.Page, PageSize: options.PageSize,
		SortBy: options.SortBy, SortOrder: options.SortOrder,
	}, nil
}

func (repository *repositoryStub) FindProjectVolumeByIdempotency(ctx context.Context, _, _ string) (model.ProjectVolume, error) {
	repository.captureContext(ctx)
	return repository.findVolume, repository.findErr
}

func (repository *repositoryStub) GetProjectVolumeForMaintenance(ctx context.Context, _ string) (model.ProjectVolume, error) {
	repository.captureContext(ctx)
	return repository.lockedVolume, repository.findErr
}

func (repository *repositoryStub) CreateProjectVolume(ctx context.Context, volume *model.ProjectVolume) error {
	repository.captureContext(ctx)
	repository.createdVolume = *volume
	return repository.createErr
}

func (repository *repositoryStub) TransitionProjectVolume(ctx context.Context, projectID, volumeID string, _ []string, to, errorCode, _ string) (model.ProjectVolume, error) {
	repository.captureContext(ctx)
	repository.transitionTo = to
	repository.transitionErrorCode = errorCode
	result := repository.lockedVolume
	result.ID = volumeID
	result.ProjectID = projectID
	result.LifecycleState = to
	if to == model.ProjectVolumeLifecycleReady {
		result.PendingOperation = ""
	}
	result.LastErrorCode = errorCode
	repository.lockedVolume = result
	return result, nil
}

func (repository *repositoryStub) TransitionDeploymentVolumeMount(ctx context.Context, projectID, mountID string, from []string, to, _, _ string) (model.DeploymentVolumeMount, error) {
	repository.captureContext(ctx)
	repository.mountTransitionFrom = append([]string(nil), from...)
	repository.mountTransitionTo = to
	return model.DeploymentVolumeMount{ID: mountID, ProjectID: projectID, ActivationState: to}, nil
}

func (repository *repositoryStub) LockProjectVolume(ctx context.Context, _, _ string) (model.ProjectVolume, error) {
	repository.captureContext(ctx)
	return repository.lockedVolume, nil
}

func (repository *repositoryStub) CountBlockingMounts(ctx context.Context, _ string) (int64, error) {
	repository.captureContext(ctx)
	return repository.blockingMounts, nil
}

func (repository *repositoryStub) CountActiveTransfers(ctx context.Context, _ string) (int64, error) {
	repository.captureContext(ctx)
	return repository.activeTransfers, nil
}

func (repository *repositoryStub) SoftDeleteProjectVolume(ctx context.Context, _, _ string, _ int64) (bool, error) {
	repository.captureContext(ctx)
	repository.softDeleted = true
	return true, nil
}

func (repository *repositoryStub) CreateVolumeTransfer(ctx context.Context, transfer *model.VolumeTransfer) error {
	repository.captureContext(ctx)
	repository.createdTransfer = *transfer
	repository.transferByID = *transfer
	repository.lockedTransfer = *transfer
	return nil
}

func (repository *repositoryStub) GetVolumeTransfer(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	if repository.transferGetErr != nil {
		return model.VolumeTransfer{}, repository.transferGetErr
	}
	if repository.transferByID.ID == "" || repository.transferByID.ID != transferID || repository.transferByID.ProjectID != projectID {
		return model.VolumeTransfer{}, gorm.ErrRecordNotFound
	}
	return repository.transferByID, nil
}

func (repository *repositoryStub) LockVolumeTransfer(ctx context.Context, _, _ string) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	return repository.lockedTransfer, nil
}

func (repository *repositoryStub) TransitionVolumeTransfer(ctx context.Context, projectID, transferID, _, to, errorCode, message string) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.transferTo = to
	repository.transferErrorCode = errorCode
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	result.State = to
	result.LastErrorCode = errorCode
	result.LastErrorMessage = message
	repository.lockedTransfer = result
	return result, nil
}

func (repository *repositoryStub) CompleteVolumeTransferUpload(ctx context.Context, projectID, transferID, _ string, expectedBytes int64, checksum string) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.completedBytes = expectedBytes
	repository.completedSHA256 = checksum
	return model.VolumeTransfer{ID: transferID, ProjectID: projectID, State: model.VolumeTransferStateQueued, ExpectedBytes: expectedBytes, SHA256: checksum}, nil
}

func (repository *repositoryStub) ClaimVolumeTransferExecution(ctx context.Context, projectID, transferID, expectedState, leaseOwner string, claimedAt, leaseExpiresAt time.Time) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.claimedLeaseOwner = leaseOwner
	repository.claimedLeaseExpiry = leaseExpiresAt
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	result.State = expectedState
	result.ExecutionGeneration++
	result.CreationLeaseOwner = leaseOwner
	result.CreationLeaseExpiresAt = &leaseExpiresAt
	result.JobCreatedAt = nil
	repository.lockedTransfer = result
	return result, nil
}

func (repository *repositoryStub) RenewVolumeTransferExecutionLease(ctx context.Context, projectID, transferID, leaseOwner string, generation int64, renewedAt, leaseExpiresAt time.Time) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.renewedLeaseExpiry = leaseExpiresAt
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	result.ExecutionGeneration = generation
	result.CreationLeaseOwner = leaseOwner
	result.CreationLeaseExpiresAt = &leaseExpiresAt
	repository.lockedTransfer = result
	return result, nil
}

func (repository *repositoryStub) PrepareVolumeTransferExecution(ctx context.Context, projectID, transferID, expectedState, leaseOwner string, generation int64, tokenHash string, expiresAt time.Time) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.preparedState = expectedState
	repository.preparedTokenHash = tokenHash
	repository.preparedExpiry = expiresAt
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	result.State = model.VolumeTransferStateRunning
	result.ExecutionGeneration = generation
	result.CreationLeaseOwner = leaseOwner
	result.CallbackTokenHash = tokenHash
	result.CallbackTokenExpiresAt = &expiresAt
	repository.lockedTransfer = result
	return result, nil
}

func (repository *repositoryStub) ConfirmVolumeTransferJobCreated(ctx context.Context, projectID, transferID string, generation int64) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	result.ExecutionGeneration = generation
	now := timeNowUTC()
	result.JobCreatedAt = &now
	result.CreationLeaseOwner = ""
	result.CreationLeaseExpiresAt = nil
	repository.lockedTransfer = result
	return result, nil
}

func (repository *repositoryStub) ReportVolumeTransferCompletion(ctx context.Context, projectID, transferID string, completion TransferCompletion) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.completedBytes = completion.TransferredBytes
	repository.completedSHA256 = completion.SHA256
	repository.completionReported = true
	now := timeNowUTC()
	return model.VolumeTransfer{
		ID: transferID, ProjectID: projectID, ProjectVolumeID: repository.lockedTransfer.ProjectVolumeID,
		Direction: repository.lockedTransfer.Direction, Format: repository.lockedTransfer.Format,
		State: model.VolumeTransferStateRunning, CompletionReportedAt: &now,
		TransferredBytes: completion.TransferredBytes, SHA256: completion.SHA256,
		LogicalBytes: completion.LogicalBytes, DataSHA256: completion.DataSHA256,
	}, nil
}

func (repository *repositoryStub) MarkVolumeTransferJobSucceeded(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.jobSucceeded = true
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	now := timeNowUTC()
	result.JobSucceededAt = &now
	return result, nil
}

func (repository *repositoryStub) FinalizeVolumeTransferExecution(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.finalized = true
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	result.State = model.VolumeTransferStateSucceeded
	return result, nil
}

func (repository *repositoryStub) MarkVolumeTransferExecutionCleanupCompleted(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.cleanupCompleted = true
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	now := timeNowUTC()
	result.ExecutionCleanupCompletedAt = &now
	repository.lockedTransfer = result
	return result, nil
}

func (repository *repositoryStub) VolumeTransferUploadOffset(ctx context.Context, _ string) (int64, error) {
	repository.captureContext(ctx)
	var offset int64
	for _, part := range repository.transferParts {
		if part.State != model.VolumeTransferPartStateCompleted {
			continue
		}
		if end := part.Offset + part.Size; end > offset {
			offset = end
		}
	}
	return offset, nil
}

func (repository *repositoryStub) NextVolumeTransferPartNumber(ctx context.Context, _ string) (int, error) {
	repository.captureContext(ctx)
	return len(repository.transferParts) + 1, nil
}

func (repository *repositoryStub) GetVolumeTransferPartByOffset(ctx context.Context, _ string, offset int64) (model.VolumeTransferPart, error) {
	repository.captureContext(ctx)
	for _, part := range repository.transferParts {
		if part.Offset == offset {
			return part, nil
		}
	}
	return model.VolumeTransferPart{}, gorm.ErrRecordNotFound
}

func (repository *repositoryStub) CreateVolumeTransferPart(ctx context.Context, part *model.VolumeTransferPart) (bool, model.VolumeTransferPart, error) {
	repository.captureContext(ctx)
	for _, existing := range repository.transferParts {
		if existing.PartNumber == part.PartNumber {
			return false, existing, nil
		}
	}
	repository.transferParts = append(repository.transferParts, *part)
	return true, *part, nil
}

func (repository *repositoryStub) TakeOverVolumeTransferPart(ctx context.Context, transferID string, partNumber int, expectedLeaseToken, leaseToken string, leaseExpiresAt time.Time) (bool, model.VolumeTransferPart, error) {
	repository.captureContext(ctx)
	for index := range repository.transferParts {
		part := &repository.transferParts[index]
		if part.TransferID == transferID && part.PartNumber == partNumber && part.State == model.VolumeTransferPartStateReserved && part.LeaseToken == expectedLeaseToken {
			part.LeaseToken = leaseToken
			part.LeaseExpiresAt = &leaseExpiresAt
			return true, *part, nil
		}
	}
	return false, model.VolumeTransferPart{}, nil
}

func (repository *repositoryStub) CompleteVolumeTransferPart(ctx context.Context, transferID string, partNumber int, leaseToken, etag string) (bool, model.VolumeTransferPart, error) {
	repository.captureContext(ctx)
	if repository.completePartErr != nil {
		return false, model.VolumeTransferPart{}, repository.completePartErr
	}
	for index := range repository.transferParts {
		part := &repository.transferParts[index]
		if part.TransferID != transferID || part.PartNumber != partNumber {
			continue
		}
		if part.State == model.VolumeTransferPartStateReserved && part.LeaseToken == leaseToken {
			part.State = model.VolumeTransferPartStateCompleted
			part.ETag = etag
			part.LeaseToken = ""
			part.LeaseExpiresAt = nil
			return true, *part, nil
		}
		return false, *part, nil
	}
	return false, model.VolumeTransferPart{}, gorm.ErrRecordNotFound
}

func (repository *repositoryStub) ExpireVolumeTransferPartLease(ctx context.Context, transferID string, partNumber int, leaseToken string, expiredAt time.Time) (bool, error) {
	repository.captureContext(ctx)
	for index := range repository.transferParts {
		part := &repository.transferParts[index]
		if part.TransferID == transferID && part.PartNumber == partNumber && part.State == model.VolumeTransferPartStateReserved && part.LeaseToken == leaseToken {
			part.LeaseExpiresAt = &expiredAt
			return true, nil
		}
	}
	return false, nil
}

func (repository *repositoryStub) ListStaleProjectVolumes(ctx context.Context, _ time.Time, limit int) ([]model.ProjectVolume, error) {
	repository.captureContext(ctx)
	repository.maintenanceLimits = append(repository.maintenanceLimits, limit)
	return []model.ProjectVolume{}, nil
}

func (repository *repositoryStub) ListStaleVolumeTransfers(ctx context.Context, _ time.Time, limit int) ([]model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.maintenanceLimits = append(repository.maintenanceLimits, limit)
	return []model.VolumeTransfer{}, nil
}

func (repository *repositoryStub) ListExpiredVolumeTransferObjects(ctx context.Context, _ time.Time, limit int) ([]model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	repository.maintenanceLimits = append(repository.maintenanceLimits, limit)
	return []model.VolumeTransfer{}, nil
}

func (repository *repositoryStub) MarkVolumeTransferObjectDeleted(ctx context.Context, _, _ string, _ time.Time) (bool, error) {
	repository.captureContext(ctx)
	repository.objectDeleted = true
	return true, nil
}

func (repository *repositoryStub) captureContext(ctx context.Context) {
	repository.contextValue = ctx.Value(contextKey("request"))
	repository.contextTraceID = trace.SpanContextFromContext(ctx).TraceID()
}

type dispatcherStub struct {
	operation      VolumeOperation
	contextValue   any
	contextTraceID trace.TraceID
	err            error
}

func (dispatcher *dispatcherStub) DispatchVolumeOperation(ctx context.Context, operation VolumeOperation) error {
	dispatcher.operation = operation
	dispatcher.contextValue = ctx.Value(contextKey("request"))
	dispatcher.contextTraceID = trace.SpanContextFromContext(ctx).TraceID()
	return dispatcher.err
}

type inspectorStub struct {
	inspection     ExistingClaimInspection
	input          ExistingClaimInspectionInput
	contextValue   any
	contextTraceID trace.TraceID
	err            error
	waitForCancel  bool
	called         bool
}

func (inspector *inspectorStub) InspectExistingClaim(ctx context.Context, input ExistingClaimInspectionInput) (ExistingClaimInspection, error) {
	inspector.called = true
	inspector.input = input
	inspector.contextValue = ctx.Value(contextKey("request"))
	inspector.contextTraceID = trace.SpanContextFromContext(ctx).TraceID()
	if inspector.waitForCancel {
		<-ctx.Done()
		return ExistingClaimInspection{}, ctx.Err()
	}
	return inspector.inspection, inspector.err
}

func TestListProjectVolumesAlwaysNormalizesPagination(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{}
	service := NewService(repository)
	result, err := service.ListProjectVolumes(context.Background(), "prj_demo", ProjectVolumeListOptions{Page: -2, PageSize: 1000})
	if err != nil {
		t.Fatalf("list project volumes: %v", err)
	}
	if result.Page != 1 || result.PageSize != MaxPageSize || result.SortBy != "createdAt" || result.SortOrder != "desc" {
		t.Fatalf("unexpected normalized list result: %#v", result)
	}
	if repository.listOptions != (ProjectVolumeListOptions{Page: 1, PageSize: MaxPageSize, SortBy: "createdAt", SortOrder: "desc"}) {
		t.Fatalf("repository options = %#v", repository.listOptions)
	}
}

func TestListProjectVolumesRejectsUnknownSort(t *testing.T) {
	t.Parallel()
	_, err := NewService(&repositoryStub{}).ListProjectVolumes(context.Background(), "prj_demo", ProjectVolumeListOptions{SortBy: "claimName"})
	if ErrorCode(err) != CodePaginationSortByInvalid {
		t.Fatalf("error code = %q, err=%v", ErrorCode(err), err)
	}
}

func TestVolumeTransferSortWhitelistIncludesTransferredBytes(t *testing.T) {
	t.Parallel()
	options, err := normalizeVolumeTransferListOptions(VolumeTransferListOptions{SortBy: "transferredBytes", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("normalize public transferredBytes sort: %v", err)
	}
	if options.SortBy != "transferredBytes" || volumeTransferSortColumns[options.SortBy] != "transferred_bytes" {
		t.Fatalf("normalized options=%#v column=%q", options, volumeTransferSortColumns[options.SortBy])
	}
}

func TestCreateProjectVolumePropagatesContextAndTrace(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{findErr: gorm.ErrRecordNotFound}
	dispatcher := &dispatcherStub{}
	service := NewService(repository, dispatcher)
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	ctx := trace.ContextWithSpanContext(context.WithValue(context.Background(), contextKey("request"), "ctx-value"), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	result, err := service.CreateProjectVolume(ctx, validBlankVolumeInput())
	if err != nil {
		t.Fatalf("create project volume: %v", err)
	}
	if result.Replayed || result.Volume.ID == "" || repository.createdVolume.ID != result.Volume.ID {
		t.Fatalf("unexpected create result: %#v", result)
	}
	if repository.contextValue != "ctx-value" || dispatcher.contextValue != "ctx-value" {
		t.Fatalf("context value did not reach repository/dispatcher: repository=%v dispatcher=%v", repository.contextValue, dispatcher.contextValue)
	}
	if repository.contextTraceID != traceID || dispatcher.contextTraceID != traceID {
		t.Fatalf("trace context was truncated: repository=%s dispatcher=%s", repository.contextTraceID, dispatcher.contextTraceID)
	}
	if dispatcher.operation.Kind != OperationProvision || dispatcher.operation.VolumeID != result.Volume.ID {
		t.Fatalf("unexpected dispatched operation: %#v", dispatcher.operation)
	}
	if repository.createdVolume.IdempotencyKeyHash == validBlankVolumeInput().IdempotencyKey || len(repository.createdVolume.IdempotencyKeyHash) != 64 {
		t.Fatalf("idempotency key was not hashed: %q", repository.createdVolume.IdempotencyKeyHash)
	}
}

func TestExistingClaimUsesAuthoritativeInspection(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{findErr: gorm.ErrRecordNotFound}
	inspector := &inspectorStub{inspection: ExistingClaimInspection{
		CapacityRequest: "20Gi", CapacityBytes: 20 * 1024 * 1024 * 1024,
		StorageClassName: "fast", AccessMode: model.ProjectVolumeAccessReadWriteOnce,
		VolumeMode: model.ProjectVolumeModeFilesystem,
	}}
	service := NewServiceWithDependencies(repository, &dispatcherStub{}, inspector)
	input := validBlankVolumeInput()
	input.SourceKind = model.ProjectVolumeSourceExistingClaim
	input.OwnershipMode = model.ProjectVolumeOwnershipReferenced
	input.ClaimName = "external-data"
	input.CapacityRequest = "999Ti"
	input.CapacityBytes = 999
	input.StorageClassName = "client-controlled"
	input.AccessMode = model.ProjectVolumeAccessReadWriteMany
	input.VolumeMode = model.ProjectVolumeModeBlock
	ctx := context.WithValue(context.Background(), contextKey("request"), "inspect")
	result, err := service.CreateProjectVolume(ctx, input)
	if err != nil {
		t.Fatalf("create referenced project volume: %v", err)
	}
	created := repository.createdVolume
	if created.CapacityRequest != "20Gi" || created.CapacityBytes != 20*1024*1024*1024 || created.StorageClassName != "fast" ||
		created.AccessMode != model.ProjectVolumeAccessReadWriteOnce || created.VolumeMode != model.ProjectVolumeModeFilesystem {
		t.Fatalf("client-controlled claim specification was persisted: %#v", created)
	}
	if inspector.input.VolumeID != result.Volume.ID || inspector.input.ClusterID != input.ClusterID || inspector.contextValue != "inspect" {
		t.Fatalf("unexpected inspector call: input=%#v context=%v", inspector.input, inspector.contextValue)
	}
}

func TestExistingClaimInspectionHonorsCancellation(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{findErr: gorm.ErrRecordNotFound}
	inspector := &inspectorStub{waitForCancel: true}
	service := NewServiceWithDependencies(repository, nil, inspector)
	input := validBlankVolumeInput()
	input.SourceKind = model.ProjectVolumeSourceExistingClaim
	input.OwnershipMode = model.ProjectVolumeOwnershipReferenced
	input.ClaimName = "external-data"
	input.CapacityRequest = ""
	input.CapacityBytes = 0
	input.StorageClassName = ""
	input.AccessMode = ""
	input.VolumeMode = ""
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.CreateProjectVolume(ctx, input)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("create error = %v, want context.Canceled", err)
	}
}

func TestIdempotencyReplaySkipsInspectionAndDispatch(t *testing.T) {
	t.Parallel()
	input := validBlankVolumeInput()
	input.SourceKind = model.ProjectVolumeSourceExistingClaim
	input.OwnershipMode = model.ProjectVolumeOwnershipReferenced
	input.ClaimName = "external-data"
	input.CapacityRequest = ""
	input.CapacityBytes = 0
	input.StorageClassName = ""
	input.AccessMode = ""
	input.VolumeMode = ""
	requestHash, err := hashCreateProjectVolumeRequest(normalizeCreateProjectVolumeInput(input))
	if err != nil {
		t.Fatalf("hash request: %v", err)
	}
	existing := model.ProjectVolume{ID: "pvol_existing", ProjectID: input.ProjectID, IdempotencyRequestHash: requestHash}
	repository := &repositoryStub{findVolume: existing}
	inspector := &inspectorStub{}
	dispatcher := &dispatcherStub{}
	result, err := NewServiceWithDependencies(repository, dispatcher, inspector).CreateProjectVolume(context.Background(), input)
	if err != nil {
		t.Fatalf("replay project volume create: %v", err)
	}
	if !result.Replayed || result.Volume.ID != existing.ID || inspector.called || dispatcher.operation.Kind != "" {
		t.Fatalf("unexpected replay behavior: result=%#v inspector=%t operation=%#v", result, inspector.called, dispatcher.operation)
	}
}

func TestSnapshotRestoreRequiresAndPersistsSourceName(t *testing.T) {
	t.Parallel()
	input := validBlankVolumeInput()
	input.SourceKind = model.ProjectVolumeSourceSnapshotRestore
	service := NewService(&repositoryStub{findErr: gorm.ErrRecordNotFound}, &dispatcherStub{})
	if _, err := service.CreateProjectVolume(context.Background(), input); ErrorCode(err) != CodeInvalidInput {
		t.Fatalf("missing snapshot name error code = %q, err=%v", ErrorCode(err), err)
	}
	repository := &repositoryStub{findErr: gorm.ErrRecordNotFound}
	input.SourceSnapshotName = "nightly-2026-08-15"
	if _, err := NewService(repository, &dispatcherStub{}).CreateProjectVolume(context.Background(), input); err != nil {
		t.Fatalf("create snapshot restore volume: %v", err)
	}
	if repository.createdVolume.SourceSnapshotName != input.SourceSnapshotName {
		t.Fatalf("source snapshot name = %q", repository.createdVolume.SourceSnapshotName)
	}
}

func TestMutatingCreateRequiresOperationDispatcher(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{findErr: gorm.ErrRecordNotFound}
	_, err := NewService(repository).CreateProjectVolume(context.Background(), validBlankVolumeInput())
	if ErrorCode(err) != CodeTaskEnqueueFailed {
		t.Fatalf("error code = %q, err=%v", ErrorCode(err), err)
	}
	if repository.transitionTo != model.ProjectVolumeLifecycleError || repository.transitionErrorCode != CodeTaskEnqueueFailed {
		t.Fatalf("failed enqueue did not move the durable record to error: to=%q code=%q", repository.transitionTo, repository.transitionErrorCode)
	}
}

func TestExportTransferRequiresOperationDispatcher(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedVolume: model.ProjectVolume{
		ID: "pvol_demo", ProjectID: "prj_demo", LifecycleState: model.ProjectVolumeLifecycleReady,
		VolumeMode: model.ProjectVolumeModeFilesystem,
	}}
	_, err := NewService(repository).CreateVolumeTransfer(context.Background(), CreateVolumeTransferInput{
		ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo", Direction: model.VolumeTransferDirectionExport,
		Format: model.VolumeTransferFormatTarGZ, ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		ActorID: "usr_demo", ExpiresAt: timeNowUTC().Add(time.Hour),
	})
	if ErrorCode(err) != CodeTaskEnqueueFailed {
		t.Fatalf("error code = %q, err=%v", ErrorCode(err), err)
	}
	if repository.createdTransfer.State != model.VolumeTransferStateQueued || repository.transferTo != model.VolumeTransferStateFailed {
		t.Fatalf("export transfer was left without a worker: created=%q transitioned=%q", repository.createdTransfer.State, repository.transferTo)
	}
}

func TestVerifiedImportRetryUsesHashedIdempotentTransferIdentity(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedVolume: model.ProjectVolume{
		ID: "pvol_demo", ProjectID: "prj_demo", SourceKind: model.ProjectVolumeSourceArchiveImport,
		LifecycleState: model.ProjectVolumeLifecycleProvisioning, VolumeMode: model.ProjectVolumeModeFilesystem,
	}}
	dispatcher := &dispatcherStub{}
	service := NewService(repository, dispatcher)
	input := CreateVolumeTransferInput{
		ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo", Direction: model.VolumeTransferDirectionImport,
		Format: model.VolumeTransferFormatTarGZ, ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		ObjectKey: "transfers/import-retry", ExpectedBytes: 4096,
		SHA256:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ActorID: "usr_demo", ExpiresAt: timeNowUTC().Add(time.Hour),
		IdempotencyKey: "retry-import-demo", VerifiedObject: true,
	}
	first, err := service.CreateVolumeTransfer(context.Background(), input)
	if err != nil || first.ID == "" || dispatcher.operation.TransferID != first.ID {
		t.Fatalf("first retry transfer=%#v operation=%#v err=%v", first, dispatcher.operation, err)
	}
	if strings.Contains(first.ID, input.IdempotencyKey) || first.ID != idempotentVolumeTransferID(input) {
		t.Fatalf("transfer id does not use an opaque deterministic hash: %q", first.ID)
	}

	dispatcher.operation = VolumeOperation{}
	replayed, err := service.CreateVolumeTransfer(context.Background(), input)
	if err != nil || replayed.ID != first.ID || dispatcher.operation.TransferID != "" {
		t.Fatalf("replayed transfer=%#v operation=%#v err=%v", replayed, dispatcher.operation, err)
	}

	changed := input
	changed.ExpectedBytes++
	if _, err = service.CreateVolumeTransfer(context.Background(), changed); ErrorCode(err) != CodeIdempotencyConflict {
		t.Fatalf("changed retry code=%q err=%v", ErrorCode(err), err)
	}
}

func TestVerifiedImportRetryReplaysTerminalResultWithoutComparingGeneratedExpiry(t *testing.T) {
	t.Parallel()
	original := model.VolumeTransfer{
		ID: "vtx_original", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
		Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
		ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		State:           model.VolumeTransferStateSucceeded, ObjectKey: "transfers/import-object",
		SourceFilename: "archive.tar.gz", ExpectedBytes: 4096,
		SHA256:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ActorID: "usr_demo", ExpiresAt: timeNowUTC().Add(time.Hour),
	}
	input := CreateVolumeTransferInput{
		ProjectID: original.ProjectID, ProjectVolumeID: original.ProjectVolumeID,
		Direction: original.Direction, Format: original.Format, ConsistencyMode: original.ConsistencyMode,
		ObjectKey: original.ObjectKey, SourceFilename: original.SourceFilename,
		ExpectedBytes: original.ExpectedBytes, SHA256: original.SHA256, ActorID: original.ActorID,
		ExpiresAt: timeNowUTC().Add(24 * time.Hour), IdempotencyKey: "terminal-replay-key", VerifiedObject: true,
	}
	existing := original
	existing.ID = idempotentVolumeTransferID(input)
	existing.ExpiresAt = timeNowUTC().Add(30 * time.Minute)
	repository := &repositoryStub{
		lockedVolume: model.ProjectVolume{
			ID: original.ProjectVolumeID, ProjectID: original.ProjectID,
			SourceKind:       model.ProjectVolumeSourceArchiveImport,
			LifecycleState:   model.ProjectVolumeLifecycleError,
			PendingOperation: OperationImport, VolumeMode: model.ProjectVolumeModeFilesystem,
		},
		lockedTransfer: original,
		transferByID:   existing,
	}
	dispatcher := &dispatcherStub{}
	service := NewService(repository, dispatcher)

	replayed, err := service.RetryVolumeImportTransfer(context.Background(), original.ID, input)
	if err != nil || replayed.ID != existing.ID || replayed.ExpiresAt != existing.ExpiresAt {
		t.Fatalf("replayed transfer=%#v err=%v", replayed, err)
	}
	if repository.createdTransfer.ID != "" {
		t.Fatalf("terminal replay created another transfer: %#v", repository.createdTransfer)
	}
	if repository.lockedVolume.LifecycleState != model.ProjectVolumeLifecycleError || repository.transitionTo != "" {
		t.Fatalf("terminal replay changed project volume: volume=%#v transition=%q", repository.lockedVolume, repository.transitionTo)
	}
	if dispatcher.operation.TransferID != "" {
		t.Fatalf("terminal replay dispatched another operation: %#v", dispatcher.operation)
	}

	changedBody := input
	changedBody.ExpectedBytes++
	if _, err := service.RetryVolumeImportTransfer(context.Background(), original.ID, changedBody); ErrorCode(err) != CodeIdempotencyConflict {
		t.Fatalf("changed body code=%q err=%v", ErrorCode(err), err)
	}

	repository.lockedTransfer = original
	repository.lockedTransfer.ID = "vtx_other_original"
	repository.lockedTransfer.ObjectKey = "transfers/other-object"
	if _, err := service.RetryVolumeImportTransfer(context.Background(), repository.lockedTransfer.ID, input); ErrorCode(err) != CodeIdempotencyConflict {
		t.Fatalf("changed original code=%q err=%v", ErrorCode(err), err)
	}
}

func TestUpdateProjectVolumeRequiresActor(t *testing.T) {
	t.Parallel()
	name := "renamed"
	_, err := NewService(&repositoryStub{}).UpdateProjectVolume(context.Background(), "prj_demo", "pvol_demo", 1, UpdateProjectVolumeInput{DisplayName: &name})
	if ErrorCode(err) != CodeInvalidInput {
		t.Fatalf("error code = %q, err=%v", ErrorCode(err), err)
	}
}

func TestTransferPartRangeRejectsIntegerOverflow(t *testing.T) {
	t.Parallel()
	if _, ok := safeTransferPartEnd(math.MaxInt64-4, 8); ok {
		t.Fatal("overflowing transfer part range was accepted")
	}
	if end, ok := safeTransferPartEnd(64, 32); !ok || end != 96 {
		t.Fatalf("valid transfer part range = %d, %t", end, ok)
	}
}

func TestReadyTransitionClearsPendingOperation(t *testing.T) {
	t.Parallel()
	updates := projectVolumeTransitionUpdates(model.ProjectVolumeLifecycleReady, "", "", timeNowUTC())
	if pending, exists := updates["pending_operation"]; !exists || pending != "" {
		t.Fatalf("ready transition pending_operation = %#v, exists=%t", pending, exists)
	}
	errorUpdates := projectVolumeTransitionUpdates(model.ProjectVolumeLifecycleError, "volume.failed", "internal", timeNowUTC())
	if _, exists := errorUpdates["pending_operation"]; exists {
		t.Fatal("error transition must retain the pending operation for retry")
	}
}

func TestMaintenanceReadsPropagateContextAndCapBatchSize(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{}
	service := NewService(repository)
	ctx := context.WithValue(context.Background(), contextKey("request"), "maintenance")
	cutoff := timeNowUTC().Add(-time.Hour)
	if _, err := service.ListStaleProjectVolumeOperations(ctx, MaintenanceScanOptions{Cutoff: cutoff, Limit: 1000}); err != nil {
		t.Fatalf("list stale project volume operations: %v", err)
	}
	if _, err := service.ListStaleVolumeTransferOperations(ctx, MaintenanceScanOptions{Cutoff: cutoff, Limit: 1000}); err != nil {
		t.Fatalf("list stale volume transfer operations: %v", err)
	}
	if _, err := service.ListExpiredVolumeTransferObjects(ctx, timeNowUTC(), 1000); err != nil {
		t.Fatalf("list expired volume transfer objects: %v", err)
	}
	if len(repository.maintenanceLimits) != 3 {
		t.Fatalf("maintenance scans = %d", len(repository.maintenanceLimits))
	}
	for _, limit := range repository.maintenanceLimits {
		if limit != MaxPageSize {
			t.Fatalf("maintenance limit = %d", limit)
		}
	}
	if repository.contextValue != "maintenance" {
		t.Fatalf("maintenance context value = %v", repository.contextValue)
	}
}

func TestGetProjectVolumeForMaintenancePropagatesContext(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedVolume: model.ProjectVolume{ID: "pvol_demo", ProjectID: "prj_demo"}}
	ctx := context.WithValue(context.Background(), contextKey("request"), "maintenance-get")
	result, err := NewService(repository).GetProjectVolumeForMaintenance(ctx, "pvol_demo")
	if err != nil {
		t.Fatalf("get project volume for maintenance: %v", err)
	}
	if result.ID != "pvol_demo" || repository.contextValue != "maintenance-get" {
		t.Fatalf("maintenance get result=%#v context=%v", result, repository.contextValue)
	}
}

func TestCompleteProjectVolumeDeletionChecksAndSoftDeletes(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedVolume: model.ProjectVolume{
		ID: "pvol_demo", ProjectID: "prj_demo", OwnershipMode: model.ProjectVolumeOwnershipManaged,
		LifecycleState: model.ProjectVolumeLifecycleDeleting, PendingOperation: OperationDelete, Revision: 4,
	}}
	result, err := NewService(repository).CompleteProjectVolumeDeletion(context.Background(), "prj_demo", "pvol_demo")
	if err != nil {
		t.Fatalf("complete project volume deletion: %v", err)
	}
	if !repository.softDeleted || result.Revision != 5 {
		t.Fatalf("soft delete=%t result=%#v", repository.softDeleted, result)
	}
}

func TestExpireVolumeTransferUsesStableTerminalCode(t *testing.T) {
	t.Parallel()
	now := timeNowUTC()
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: "prj_demo", State: model.VolumeTransferStateUploading, ExpiresAt: now.Add(-time.Minute),
	}}
	result, err := NewService(repository).ExpireVolumeTransfer(context.Background(), "prj_demo", "vtx_demo", now)
	if err != nil {
		t.Fatalf("expire volume transfer: %v", err)
	}
	if repository.transferTo != model.VolumeTransferStateExpired || repository.transferErrorCode != CodeTransferExpired || result.State != model.VolumeTransferStateExpired {
		t.Fatalf("transfer transition=%q code=%q result=%#v", repository.transferTo, repository.transferErrorCode, result)
	}
}

func TestWorkerStableErrorCodes(t *testing.T) {
	t.Parallel()
	if CodeSnapshotNotFound != "volume.snapshot_not_found" {
		t.Fatalf("snapshot-not-found code = %q", CodeSnapshotNotFound)
	}
	if CodeDeletionPending != "volume.deletion_pending" {
		t.Fatalf("deletion-pending code = %q", CodeDeletionPending)
	}
	if CodeTransferCompletionMissing != "volume_transfer.completion_missing" {
		t.Fatalf("completion-missing code = %q", CodeTransferCompletionMissing)
	}
}

func TestPrepareVolumeTransferExecutionUsesCallerContextAndStateCAS(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: "prj_demo", State: model.VolumeTransferStateQueued,
	}}
	service := NewService(repository)
	expiresAt := timeNowUTC().Add(time.Hour)
	leaseOwner := "attempt-transfer-prepare"
	tokenHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ctx := context.WithValue(context.Background(), contextKey("request"), "transfer-prepare")
	claimed, err := service.ClaimVolumeTransferExecution(ctx, "prj_demo", "vtx_demo", model.VolumeTransferStateQueued, leaseOwner, timeNowUTC().Add(time.Minute))
	if err != nil || claimed.ExecutionGeneration != 1 || claimed.CreationLeaseOwner != leaseOwner {
		t.Fatalf("claim volume transfer execution: transfer=%#v err=%v", claimed, err)
	}
	result, err := service.PrepareVolumeTransferExecution(ctx, "prj_demo", "vtx_demo", model.VolumeTransferStateQueued,
		leaseOwner, claimed.ExecutionGeneration, tokenHash, expiresAt)
	if err != nil {
		t.Fatalf("prepare volume transfer execution: %v", err)
	}
	if result.State != model.VolumeTransferStateRunning || repository.preparedState != model.VolumeTransferStateQueued || repository.preparedTokenHash != tokenHash || !repository.preparedExpiry.Equal(expiresAt) {
		t.Fatalf("unexpected prepare result=%#v repository=%#v", result, repository)
	}
	if repository.contextValue != "transfer-prepare" {
		t.Fatalf("prepare context value=%v", repository.contextValue)
	}
	renewed, err := service.RenewVolumeTransferExecutionLease(ctx, "prj_demo", "vtx_demo", leaseOwner,
		claimed.ExecutionGeneration, timeNowUTC().Add(2*time.Minute))
	if err != nil || renewed.State != model.VolumeTransferStateRunning || repository.renewedLeaseExpiry.IsZero() {
		t.Fatalf("renew queued-to-running execution lease: transfer=%#v err=%v", renewed, err)
	}
}

func TestReportVolumeTransferCompletionRejectsUnverifiedImportResultAndStaysRunning(t *testing.T) {
	t.Parallel()
	verifiedSHA := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
		Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateRunning,
		ExpectedBytes: 4096, SHA256: verifiedSHA,
	}}
	service := NewService(repository)
	if _, err := service.ReportVolumeTransferCompletion(context.Background(), "prj_demo", "vtx_demo", TransferCompletion{
		ExpectedState: model.VolumeTransferStateRunning, TransferredBytes: 4095, SHA256: verifiedSHA,
	}); ErrorCode(err) != CodeTransferChecksumMismatch {
		t.Fatalf("length mismatch code=%q err=%v", ErrorCode(err), err)
	}
	if repository.completedBytes != 0 {
		t.Fatalf("unverified completion reached repository: %d", repository.completedBytes)
	}
	result, err := service.ReportVolumeTransferCompletion(context.Background(), "prj_demo", "vtx_demo", TransferCompletion{
		ExpectedState: model.VolumeTransferStateRunning, TransferredBytes: 4096, SHA256: verifiedSHA,
	})
	if err != nil || result.State != model.VolumeTransferStateRunning || result.CompletionReportedAt == nil ||
		repository.completedBytes != 4096 || !repository.completionReported || repository.transitionTo != "" {
		t.Fatalf("verified completion result=%#v bytes=%d volumeState=%q err=%v", result, repository.completedBytes, repository.transitionTo, err)
	}
}

func TestReportRawVolumeTransferPersistsServerObservedDigestAndRejectsDifferentReplay(t *testing.T) {
	t.Parallel()
	archiveSHA := strings.Repeat("a", 64)
	dataSHA := strings.Repeat("b", 64)
	completion := TransferCompletion{
		ExpectedState: model.VolumeTransferStateRunning, TransferredBytes: 1024,
		SHA256: archiveSHA, LogicalBytes: 4096, DataSHA256: dataSHA,
	}
	repository := &repositoryStub{
		lockedVolume: model.ProjectVolume{ID: "pvol_demo", ProjectID: "prj_demo", VolumeMode: model.ProjectVolumeModeBlock, CapacityBytes: 8192},
		lockedTransfer: model.VolumeTransfer{
			ID: "vtx_demo", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatRawZST,
			State: model.VolumeTransferStateRunning,
		},
	}
	service := NewService(repository)
	result, err := service.ReportVolumeTransferCompletion(context.Background(), "prj_demo", "vtx_demo", completion)
	if err != nil || result.LogicalBytes != completion.LogicalBytes || result.DataSHA256 != dataSHA {
		t.Fatalf("raw completion result=%#v err=%v", result, err)
	}

	repository.lockedTransfer = result
	different := completion
	different.DataSHA256 = strings.Repeat("c", 64)
	if _, err := service.ReportVolumeTransferCompletion(context.Background(), "prj_demo", "vtx_demo", different); ErrorCode(err) != CodeTransferStateConflict {
		t.Fatalf("different replay code=%q err=%v", ErrorCode(err), err)
	}
}

func TestFinalizeImportAtomicallyRequiresBothEvidenceMarkersAndPromotesVolume(t *testing.T) {
	t.Parallel()
	now := timeNowUTC()
	repository := &repositoryStub{
		lockedVolume: model.ProjectVolume{
			ID: "pvol_demo", ProjectID: "prj_demo", SourceKind: model.ProjectVolumeSourceArchiveImport,
			LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationImport,
		},
		lockedTransfer: model.VolumeTransfer{
			ID: "vtx_demo", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
			Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateRunning,
			CompletionReportedAt: &now, JobSucceededAt: &now,
		},
	}
	result, err := NewService(repository).FinalizeVolumeTransferExecution(context.Background(), "prj_demo", "vtx_demo")
	if err != nil || result.State != model.VolumeTransferStateSucceeded || !repository.finalized || repository.transitionTo != model.ProjectVolumeLifecycleReady {
		t.Fatalf("finalize result=%#v finalized=%t volumeState=%q err=%v", result, repository.finalized, repository.transitionTo, err)
	}

	repository.finalized = false
	repository.transitionTo = ""
	repository.lockedTransfer.JobSucceededAt = nil
	if _, err = NewService(repository).FinalizeVolumeTransferExecution(context.Background(), "prj_demo", "vtx_demo"); ErrorCode(err) != CodeTransferStateConflict || repository.finalized || repository.transitionTo != "" {
		t.Fatalf("missing marker code=%q finalized=%t volumeState=%q err=%v", ErrorCode(err), repository.finalized, repository.transitionTo, err)
	}
}

func TestFailImportAtomicallyUsesCallerTraceAndIsIdempotent(t *testing.T) {
	t.Parallel()
	traceID := trace.TraceID{8, 6, 7, 5, 3, 0, 9}
	spanID := trace.SpanID{1, 2, 3, 4}
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true,
	}))
	repository := &repositoryStub{
		lockedVolume: model.ProjectVolume{
			ID: "pvol_demo", ProjectID: "prj_demo", SourceKind: model.ProjectVolumeSourceArchiveImport,
			LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationImport,
		},
		lockedTransfer: model.VolumeTransfer{
			ID: "vtx_demo", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
			Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateRunning,
		},
	}
	service := NewService(repository)
	result, err := service.FailVolumeTransferExecution(ctx, "prj_demo", "vtx_demo", CodeTransferJobFailed, "trusted worker diagnostic")
	if err != nil || result.State != model.VolumeTransferStateFailed || repository.lockedVolume.LifecycleState != model.ProjectVolumeLifecycleError ||
		repository.lockedVolume.PendingOperation != OperationImport || repository.lockedVolume.LastErrorCode != CodeTransferJobFailed {
		t.Fatalf("atomic failure result=%#v volume=%#v err=%v", result, repository.lockedVolume, err)
	}
	if repository.contextTraceID != traceID {
		t.Fatalf("failure transaction trace=%s, want %s", repository.contextTraceID, traceID)
	}
	if replay, replayErr := service.FailVolumeTransferExecution(ctx, "prj_demo", "vtx_demo", CodeTransferJobFailed, "trusted worker diagnostic"); replayErr != nil || replay.State != model.VolumeTransferStateFailed {
		t.Fatalf("idempotent failure replay=%#v err=%v", replay, replayErr)
	}
}

func TestExecutionCleanupMarkerRequiresTerminalTransferAndIsIdempotent(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: "prj_demo", State: model.VolumeTransferStateFailed,
	}}
	service := NewService(repository)
	marked, err := service.MarkVolumeTransferExecutionCleanupCompleted(context.Background(), "prj_demo", "vtx_demo")
	if err != nil || marked.ExecutionCleanupCompletedAt == nil || !repository.cleanupCompleted {
		t.Fatalf("cleanup marker=%#v persisted=%t err=%v", marked, repository.cleanupCompleted, err)
	}
	if replay, replayErr := service.MarkVolumeTransferExecutionCleanupCompleted(context.Background(), "prj_demo", "vtx_demo"); replayErr != nil || replay.ExecutionCleanupCompletedAt == nil {
		t.Fatalf("cleanup marker replay=%#v err=%v", replay, replayErr)
	}

	active := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_active", ProjectID: "prj_demo", State: model.VolumeTransferStateRunning,
	}}
	if _, err = NewService(active).MarkVolumeTransferExecutionCleanupCompleted(context.Background(), "prj_demo", "vtx_active"); ErrorCode(err) != CodeTransferStateConflict || active.cleanupCompleted {
		t.Fatalf("active cleanup marker code=%q persisted=%t err=%v", ErrorCode(err), active.cleanupCompleted, err)
	}
}

func TestVolumeTransferObjectDeletionRequiresExecutionCleanupProof(t *testing.T) {
	t.Parallel()
	deletedAt := timeNowUTC()
	pending := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_pending", ProjectID: "prj_demo", State: model.VolumeTransferStateFailed,
	}}
	if _, err := NewService(pending).MarkVolumeTransferObjectDeleted(context.Background(), "prj_demo", "vtx_pending", deletedAt); ErrorCode(err) != CodeDeletionPending || pending.objectDeleted {
		t.Fatalf("cleanup-pending object deletion code=%q deleted=%t err=%v", ErrorCode(err), pending.objectDeleted, err)
	}

	cleanupCompletedAt := deletedAt.Add(-time.Second)
	cleaned := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_cleaned", ProjectID: "prj_demo", State: model.VolumeTransferStateFailed,
		ExecutionCleanupCompletedAt: &cleanupCompletedAt,
	}}
	result, err := NewService(cleaned).MarkVolumeTransferObjectDeleted(context.Background(), "prj_demo", "vtx_cleaned", deletedAt)
	if err != nil || !cleaned.objectDeleted || result.ObjectDeletedAt == nil || !result.ObjectDeletedAt.Equal(deletedAt) {
		t.Fatalf("cleaned object deletion result=%#v deleted=%t err=%v", result, cleaned.objectDeleted, err)
	}
}

func TestGenericTransferTransitionCannotBypassAuthoritativeFinalization(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: "prj_demo", State: model.VolumeTransferStateRunning,
	}}
	if _, err := NewService(repository).TransitionVolumeTransfer(context.Background(), "prj_demo", "vtx_demo", model.VolumeTransferStateSucceeded, "", ""); ErrorCode(err) != CodeTransferStateConflict || repository.transferTo != "" {
		t.Fatalf("generic success code=%q repositoryTo=%q err=%v", ErrorCode(err), repository.transferTo, err)
	}
}

func TestCancelVolumeTransferDispatchesWorkerCleanupAndCanRetryEnqueue(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo", ActorID: "usr_demo",
		Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateRunning,
	}}
	dispatcher := &dispatcherStub{}
	service := NewService(repository, dispatcher)
	result, err := service.TransitionVolumeTransfer(context.Background(), "prj_demo", "vtx_demo", model.VolumeTransferStateCancelled, "", "")
	if err != nil || result.State != model.VolumeTransferStateCancelled || dispatcher.operation.Kind != model.VolumeTransferDirectionImport || dispatcher.operation.TransferID != "vtx_demo" {
		t.Fatalf("cancel result=%#v operation=%#v err=%v", result, dispatcher.operation, err)
	}

	// Retrying cancellation is intentionally idempotent and re-dispatches the
	// cleanup task if the first queue write was unavailable.
	repository.lockedTransfer.State = model.VolumeTransferStateCancelled
	dispatcher.operation = VolumeOperation{}
	if _, err = service.TransitionVolumeTransfer(context.Background(), "prj_demo", "vtx_demo", model.VolumeTransferStateCancelled, "", ""); err != nil || dispatcher.operation.TransferID != "vtx_demo" {
		t.Fatalf("cancel redispatch operation=%#v err=%v", dispatcher.operation, err)
	}
}

func TestCompleteCancelledVolumeImportOnlyRemovesCleanedArchiveAsset(t *testing.T) {
	t.Parallel()
	cleanupCompletedAt := timeNowUTC()
	repository := &repositoryStub{
		lockedTransfer: model.VolumeTransfer{
			ID: "vtx_demo", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
			Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateCancelled,
			ExecutionCleanupCompletedAt: &cleanupCompletedAt,
		},
		lockedVolume: model.ProjectVolume{
			ID: "pvol_demo", ProjectID: "prj_demo", SourceKind: model.ProjectVolumeSourceArchiveImport,
			LifecycleState: model.ProjectVolumeLifecycleProvisioning, Revision: 3,
		},
	}
	result, err := NewService(repository).CompleteCancelledVolumeImport(context.Background(), "prj_demo", "pvol_demo", "vtx_demo")
	if err != nil || !repository.softDeleted || result.Revision != 4 {
		t.Fatalf("complete cancelled import result=%#v deleted=%t err=%v", result, repository.softDeleted, err)
	}

	cleanupPendingTransfer := repository.lockedTransfer
	cleanupPendingTransfer.ExecutionCleanupCompletedAt = nil
	cleanupPending := &repositoryStub{lockedTransfer: cleanupPendingTransfer, lockedVolume: repository.lockedVolume}
	if _, err = NewService(cleanupPending).CompleteCancelledVolumeImport(context.Background(), "prj_demo", "pvol_demo", "vtx_demo"); ErrorCode(err) != CodeDeletionPending || cleanupPending.softDeleted {
		t.Fatalf("cleanup-pending cancellation code=%q deleted=%t err=%v", ErrorCode(err), cleanupPending.softDeleted, err)
	}

	blocked := &repositoryStub{lockedTransfer: repository.lockedTransfer, lockedVolume: repository.lockedVolume, blockingMounts: 1}
	if _, err = NewService(blocked).CompleteCancelledVolumeImport(context.Background(), "prj_demo", "pvol_demo", "vtx_demo"); ErrorCode(err) != CodeInUse || blocked.softDeleted {
		t.Fatalf("blocked cancellation code=%q deleted=%t err=%v", ErrorCode(err), blocked.softDeleted, err)
	}
}

func TestWriteVolumeTransferPartSerializesSameOffsetAcrossReplicas(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: "prj_demo", State: model.VolumeTransferStateUploading, ExpectedBytes: 4,
	}}
	service := NewService(repository)
	type writeResult struct{ err error }
	results := make(chan writeResult, 2)
	start := make(chan struct{})
	var writerMu sync.Mutex
	writerCalls := 0
	for _, checksum := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		checksum := checksum
		go func() {
			<-start
			_, _, err := service.WriteVolumeTransferPart(context.Background(), "prj_demo", "vtx_demo", model.VolumeTransferPart{
				Offset: 0, Size: 4, SHA256: checksum,
			}, func(context.Context, int) (string, error) {
				writerMu.Lock()
				writerCalls++
				writerMu.Unlock()
				return "etag-" + checksum[:8], nil
			})
			results <- writeResult{err: err}
		}()
	}
	close(start)
	succeeded := 0
	conflicted := 0
	for range 2 {
		result := <-results
		switch ErrorCode(result.err) {
		case "":
			succeeded++
		case CodeTransferOffsetMismatch, CodeTransferPartConflict:
			conflicted++
		default:
			t.Fatalf("unexpected concurrent part error: %v", result.err)
		}
	}
	writerMu.Lock()
	defer writerMu.Unlock()
	if succeeded != 1 || conflicted != 1 || writerCalls != 1 || len(repository.transferParts) != 1 {
		t.Fatalf("same-offset writes succeeded=%d conflicted=%d writerCalls=%d parts=%d", succeeded, conflicted, writerCalls, len(repository.transferParts))
	}
}

func TestWriteVolumeTransferPartLeavesDatabaseAvailableDuringObjectStoreWrite(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
		ID: "vtx_network", ProjectID: "prj_demo", State: model.VolumeTransferStateUploading, ExpectedBytes: 4,
	}}
	service := NewService(repository)
	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		_, _, err := service.WriteVolumeTransferPart(context.Background(), "prj_demo", "vtx_network", model.VolumeTransferPart{
			Offset: 0, Size: 4, SHA256: strings.Repeat("a", 64),
		}, func(context.Context, int) (string, error) {
			close(writeEntered)
			<-releaseWrite
			return "etag-first", nil
		})
		firstResult <- err
	}()
	<-writeEntered

	queryDone := make(chan error, 1)
	go func() {
		queryDone <- repository.Transaction(context.Background(), func(Repository) error { return nil })
	}()
	select {
	case err := <-queryDone:
		if err != nil {
			t.Fatalf("concurrent database query: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("object-store write retained the repository transaction")
	}

	secondWriterCalled := false
	_, _, err := service.WriteVolumeTransferPart(context.Background(), "prj_demo", "vtx_network", model.VolumeTransferPart{
		Offset: 0, Size: 4, SHA256: strings.Repeat("a", 64),
	}, func(context.Context, int) (string, error) {
		secondWriterCalled = true
		return "etag-second", nil
	})
	if ErrorCode(err) != CodeTransferPartInProgress || secondWriterCalled {
		t.Fatalf("same-content concurrent retry code=%q writerCalled=%t err=%v", ErrorCode(err), secondWriterCalled, err)
	}
	_, _, err = service.WriteVolumeTransferPart(context.Background(), "prj_demo", "vtx_network", model.VolumeTransferPart{
		Offset: 0, Size: 4, SHA256: strings.Repeat("b", 64),
	}, func(context.Context, int) (string, error) { return "etag-conflict", nil })
	if ErrorCode(err) != CodeTransferPartConflict {
		t.Fatalf("different-content concurrent retry code=%q err=%v", ErrorCode(err), err)
	}
	close(releaseWrite)
	if err := <-firstResult; err != nil {
		t.Fatalf("first write: %v", err)
	}
}

func TestPreflightVolumeTransferPartRejectsActiveLeaseWithoutWriterTransaction(t *testing.T) {
	t.Parallel()
	leaseExpiresAt := timeNowUTC().Add(time.Minute)
	transfer := model.VolumeTransfer{
		ID: "vtx_preflight", ProjectID: "prj_demo", State: model.VolumeTransferStateUploading, ExpectedBytes: 4,
	}
	repository := &repositoryStub{
		lockedTransfer: transfer,
		transferByID:   transfer,
		transferParts: []model.VolumeTransferPart{{
			TransferID: transfer.ID, PartNumber: 1, Offset: 0, Size: 4,
			SHA256: strings.Repeat("a", 64), State: model.VolumeTransferPartStateReserved,
			LeaseToken: "active", LeaseExpiresAt: &leaseExpiresAt,
		}},
	}
	err := NewService(repository).PreflightVolumeTransferPart(context.Background(), transfer.ProjectID, transfer.ID, model.VolumeTransferPart{
		Offset: 0, Size: 4, SHA256: strings.Repeat("a", 64),
	})
	if ErrorCode(err) != CodeTransferPartInProgress {
		t.Fatalf("preflight code=%q err=%v", ErrorCode(err), err)
	}
}

func TestWriteVolumeTransferPartExpiresOwnLeaseWhenCommitFails(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{
		lockedTransfer: model.VolumeTransfer{
			ID: "vtx_commit_failure", ProjectID: "prj_demo", State: model.VolumeTransferStateUploading, ExpectedBytes: 4,
		},
		completePartErr: errors.New("database unavailable"),
	}
	_, _, err := NewService(repository).WriteVolumeTransferPart(context.Background(), "prj_demo", "vtx_commit_failure", model.VolumeTransferPart{
		Offset: 0, Size: 4, SHA256: strings.Repeat("a", 64),
	}, func(context.Context, int) (string, error) { return "etag-written", nil })
	if err == nil || len(repository.transferParts) != 1 || repository.transferParts[0].LeaseExpiresAt == nil || repository.transferParts[0].LeaseExpiresAt.After(timeNowUTC()) {
		t.Fatalf("commit error=%v part=%#v", err, repository.transferParts)
	}
}

func TestWriteVolumeTransferPartTakesOverStaleMatchingLease(t *testing.T) {
	t.Parallel()
	expiredAt := timeNowUTC().Add(-time.Minute)
	repository := &repositoryStub{
		lockedTransfer: model.VolumeTransfer{
			ID: "vtx_stale", ProjectID: "prj_demo", State: model.VolumeTransferStateUploading, ExpectedBytes: 4,
		},
		transferParts: []model.VolumeTransferPart{{
			TransferID: "vtx_stale", PartNumber: 1, Offset: 0, Size: 4,
			SHA256: strings.Repeat("a", 64), State: model.VolumeTransferPartStateReserved,
			LeaseToken: "stale", LeaseExpiresAt: &expiredAt,
		}},
	}
	service := NewService(repository)
	part, nextOffset, err := service.WriteVolumeTransferPart(context.Background(), "prj_demo", "vtx_stale", model.VolumeTransferPart{
		Offset: 0, Size: 4, SHA256: strings.Repeat("a", 64),
	}, func(_ context.Context, partNumber int) (string, error) {
		if partNumber != 1 {
			t.Fatalf("takeover part number=%d", partNumber)
		}
		return "etag-takeover", nil
	})
	if err != nil || nextOffset != 4 || part.State != model.VolumeTransferPartStateCompleted || part.ETag != "etag-takeover" {
		t.Fatalf("takeover part=%#v offset=%d err=%v", part, nextOffset, err)
	}
}

func TestRestoreReleasePendingDeploymentVolumeMountUsesNarrowTransition(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{}
	result, err := NewService(repository).RestoreReleasePendingDeploymentVolumeMount(
		context.Background(), "prj_demo", "dvmt_demo",
	)
	if err != nil {
		t.Fatalf("restore release-pending deployment volume mount: %v", err)
	}
	if result.ActivationState != model.DeploymentVolumeActivationReserved || repository.mountTransitionTo != model.DeploymentVolumeActivationReserved {
		t.Fatalf("restore result=%#v transitionTo=%q", result, repository.mountTransitionTo)
	}
	if len(repository.mountTransitionFrom) != 1 || repository.mountTransitionFrom[0] != model.DeploymentVolumeActivationReleasePending {
		t.Fatalf("restore transition from=%v", repository.mountTransitionFrom)
	}
}

func TestBeginDeploymentVolumeUnbindKeepsReservedMountUntilAuthoritativeDetach(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{}
	result, err := NewService(repository).BeginDeploymentVolumeUnbind(context.Background(), "project", "mount")
	if err != nil {
		t.Fatal(err)
	}
	if result.ActivationState != model.DeploymentVolumeActivationReleasePending || repository.mountTransitionTo != model.DeploymentVolumeActivationReleasePending {
		t.Fatalf("unbind transition = %#v, repository to=%q", result, repository.mountTransitionTo)
	}
	wantFrom := map[string]bool{
		model.DeploymentVolumeActivationReserved: true,
		model.DeploymentVolumeActivationActive:   true,
		model.DeploymentVolumeActivationError:    true,
	}
	if len(repository.mountTransitionFrom) != len(wantFrom) {
		t.Fatalf("unbind transition from=%v", repository.mountTransitionFrom)
	}
	for _, state := range repository.mountTransitionFrom {
		if !wantFrom[state] {
			t.Fatalf("unexpected unbind source state %q", state)
		}
	}
}

func validBlankVolumeInput() CreateProjectVolumeInput {
	return CreateProjectVolumeInput{
		ProjectID: "prj_demo", DisplayName: "postgres-data", ClusterID: "rclu_demo", Namespace: "project-demo",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		CapacityRequest: "10Gi", CapacityBytes: 10 * 1024 * 1024 * 1024, StorageClassName: "standard",
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		ActorID: "usr_demo", IdempotencyKey: "volume-create-demo-0001",
	}
}
