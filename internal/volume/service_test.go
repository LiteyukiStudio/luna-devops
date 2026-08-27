package volume

import (
	"context"
	"errors"
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
	lockedTarget        model.DeploymentTarget
	lockedVolume        model.ProjectVolume
	createdMount        model.DeploymentVolumeMount
	lockedTransfer      model.VolumeTransfer
	createdTransfer     model.VolumeTransfer
	transferByID        model.VolumeTransfer
	transferGetErr      error
	transferTo          string
	transferErrorCode   string
	maintenanceLimits   []int
	blockingMounts      int64
	activeTransfers     int64
	softDeleted         bool
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

func (repository *repositoryStub) LockDeploymentTarget(ctx context.Context, _, _ string) (model.DeploymentTarget, error) {
	repository.captureContext(ctx)
	return repository.lockedTarget, nil
}

func (repository *repositoryStub) ListDeploymentTargetMounts(ctx context.Context, _, _ string) ([]model.DeploymentVolumeMount, error) {
	repository.captureContext(ctx)
	return nil, nil
}

func (repository *repositoryStub) CreateDeploymentVolumeMount(ctx context.Context, mount *model.DeploymentVolumeMount) error {
	repository.captureContext(ctx)
	repository.createdMount = *mount
	return nil
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

func (repository *repositoryStub) CompleteVolumeTransferStream(ctx context.Context, projectID, transferID string, completion TransferCompletion) (model.VolumeTransfer, error) {
	repository.captureContext(ctx)
	result := repository.lockedTransfer
	result.ID = transferID
	result.ProjectID = projectID
	result.State = model.VolumeTransferStateSucceeded
	result.TransferredBytes = completion.TransferredBytes
	result.SHA256 = completion.SHA256
	repository.lockedTransfer = result
	return result, nil
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

func (repository *repositoryStub) captureContext(ctx context.Context) {
	repository.contextValue = ctx.Value(contextKey("request"))
	repository.contextTraceID = trace.SpanContextFromContext(ctx).TraceID()
}

type dispatcherStub struct {
	operation      VolumeOperation
	operations     []VolumeOperation
	contextValue   any
	contextTraceID trace.TraceID
	err            error
}

func TestVolumeServiceConstructorsRejectMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name string
		new  func()
	}{
		{name: "repository", new: func() { NewService(nil) }},
		{name: "database", new: func() { NewGormService(nil) }},
		{name: "complete dependencies", new: func() { NewServiceWithDependencies(nil, nil, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("constructor accepted a missing required dependency")
				}
			}()
			tt.new()
		})
	}
}

func (dispatcher *dispatcherStub) DispatchVolumeOperation(ctx context.Context, operation VolumeOperation) error {
	dispatcher.operation = operation
	dispatcher.operations = append(dispatcher.operations, operation)
	dispatcher.contextValue = ctx.Value(contextKey("request"))
	dispatcher.contextTraceID = trace.SpanContextFromContext(ctx).TraceID()
	return dispatcher.err
}

func TestTerminalVolumeTransferDispatchesTargetedCleanupWithoutChangingResult(t *testing.T) {
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	t.Run("completion", func(t *testing.T) {
		repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
			ID: "vtx_complete", ProjectID: "prj_complete", ProjectVolumeID: "pvol_complete",
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
			State: model.VolumeTransferStateStreaming, ActorID: "usr_complete",
		}}
		dispatcher := &dispatcherStub{err: errors.New("queue unavailable")}
		result, err := NewService(repository, dispatcher).CompleteVolumeTransferStream(context.Background(), "prj_complete", "vtx_complete", TransferCompletion{
			ExpectedState: model.VolumeTransferStateStreaming, TransferredBytes: 42, SHA256: checksum,
		})
		if err != nil || result.State != model.VolumeTransferStateSucceeded {
			t.Fatalf("completion result=%#v err=%v", result, err)
		}
		if dispatcher.operation.Kind != OperationCleanup || dispatcher.operation.TransferID != result.ID {
			t.Fatalf("cleanup operation = %#v", dispatcher.operation)
		}
	})

	t.Run("failure", func(t *testing.T) {
		repository := &repositoryStub{lockedTransfer: model.VolumeTransfer{
			ID: "vtx_fail", ProjectID: "prj_fail", ProjectVolumeID: "pvol_fail",
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
			State: model.VolumeTransferStateStreaming, ActorID: "usr_fail",
		}}
		dispatcher := &dispatcherStub{}
		result, err := NewService(repository, dispatcher).FailVolumeTransferExecution(context.Background(), "prj_fail", "vtx_fail", CodeTransferJobFailed, "helper failed")
		if err != nil || result.State != model.VolumeTransferStateFailed {
			t.Fatalf("failure result=%#v err=%v", result, err)
		}
		if dispatcher.operation.Kind != OperationCleanup || dispatcher.operation.TransferID != result.ID {
			t.Fatalf("cleanup operation = %#v", dispatcher.operation)
		}
	})
}

func TestCreateImportTransferDoesNotRequireOrPersistClientChecksum(t *testing.T) {
	repository := &repositoryStub{lockedVolume: model.ProjectVolume{
		ID: "pvol_import", ProjectID: "prj_import", SourceKind: model.ProjectVolumeSourceArchiveImport,
		LifecycleState: model.ProjectVolumeLifecycleProvisioning, VolumeMode: model.ProjectVolumeModeFilesystem,
	}}
	dispatcher := &dispatcherStub{}
	input := CreateVolumeTransferInput{
		ProjectID: "prj_import", ProjectVolumeID: "pvol_import", Direction: model.VolumeTransferDirectionImport,
		Format: model.VolumeTransferFormatTarGZ, ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		SourceFilename: "backup.tar.gz", ExpectedBytes: 42, ActorID: "usr_import", ExpiresAt: time.Now().Add(time.Hour),
		IdempotencyKey: "import-request-0001",
	}
	result, err := NewService(repository, dispatcher).CreateVolumeTransfer(t.Context(), input)
	if err != nil {
		t.Fatalf("CreateVolumeTransfer() error = %v", err)
	}
	if result.SHA256 != "" || repository.createdTransfer.SHA256 != "" {
		t.Fatalf("client checksum was persisted: result=%q stored=%q", result.SHA256, repository.createdTransfer.SHA256)
	}
	if dispatcher.operation.Kind != model.VolumeTransferDirectionImport || dispatcher.operation.TransferID != result.ID {
		t.Fatalf("dispatch operation = %#v", dispatcher.operation)
	}
}

func TestCompletedChecksumDoesNotChangeImportIdempotencyIdentity(t *testing.T) {
	input := CreateVolumeTransferInput{
		ProjectID: "prj_import", ProjectVolumeID: "pvol_import", Direction: model.VolumeTransferDirectionImport,
		Format: model.VolumeTransferFormatTarGZ, ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		SourceFilename: "backup.tar.gz", ExpectedBytes: 42, ActorID: "usr_import", ExpiresAt: time.Now().Add(time.Hour),
	}
	existing := model.VolumeTransfer{
		ProjectID: input.ProjectID, ProjectVolumeID: input.ProjectVolumeID, Direction: input.Direction,
		Format: input.Format, ConsistencyMode: input.ConsistencyMode, SourceFilename: input.SourceFilename,
		ExpectedBytes: input.ExpectedBytes, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ActorID: input.ActorID, ExpiresAt: input.ExpiresAt,
	}
	if !sameVolumeTransferRequest(existing, input) {
		t.Fatal("authoritative completion checksum must not make an idempotent create replay conflict")
	}
}

func TestCompleteImportStoresAuthoritativeChecksumWithoutDeclaredDigest(t *testing.T) {
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	repository := &repositoryStub{
		lockedTransfer: model.VolumeTransfer{
			ID: "vtx_import", ProjectID: "prj_import", ProjectVolumeID: "pvol_import", Direction: model.VolumeTransferDirectionImport,
			Format: model.VolumeTransferFormatTarGZ, State: model.VolumeTransferStateStreaming, ExpectedBytes: 42,
		},
		lockedVolume: model.ProjectVolume{
			ID: "pvol_import", ProjectID: "prj_import", LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationImport,
		},
	}
	result, err := NewService(repository, &dispatcherStub{}).CompleteVolumeTransferStream(t.Context(), "prj_import", "vtx_import", TransferCompletion{
		ExpectedState: model.VolumeTransferStateStreaming, TransferredBytes: 42, SHA256: checksum,
	})
	if err != nil || result.SHA256 != checksum || repository.transitionTo != model.ProjectVolumeLifecycleReady {
		t.Fatalf("completion result=%#v transition=%q err=%v", result, repository.transitionTo, err)
	}
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

func TestReserveDeploymentVolumeMountAllowsWaitForFirstConsumerProvisioning(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{
		lockedTarget: model.DeploymentTarget{
			ID: "dtgt_demo", ProjectID: "prj_demo", ApplicationID: "app_demo", ClusterID: "rclu_demo", Namespace: "luna-demo",
		},
		lockedVolume: model.ProjectVolume{
			ID: "pvol_demo", ProjectID: "prj_demo", ClusterID: "rclu_demo", Namespace: "luna-demo", ClaimName: "data",
			LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationProvision,
			AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		},
	}
	result, err := NewService(repository).ReserveDeploymentVolumeMount(context.Background(), ReserveMountInput{
		ProjectID: "prj_demo", ApplicationID: "app_demo", DeploymentTargetID: "dtgt_demo",
		SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: "pvol_demo", LogicalName: "data", MountPath: "/data",
	})
	if err != nil {
		t.Fatalf("reserve first-consumer volume: %v", err)
	}
	if result.ProjectVolumeID == nil || *result.ProjectVolumeID != "pvol_demo" || repository.createdMount.ID != result.ID {
		t.Fatalf("reserved mount=%#v created=%#v", result, repository.createdMount)
	}
}

func TestReserveDeploymentVolumeMountRejectsIncompleteImport(t *testing.T) {
	t.Parallel()
	repository := &repositoryStub{
		lockedTarget: model.DeploymentTarget{
			ID: "dtgt_demo", ProjectID: "prj_demo", ApplicationID: "app_demo", ClusterID: "rclu_demo", Namespace: "luna-demo",
		},
		lockedVolume: model.ProjectVolume{
			ID: "pvol_demo", ProjectID: "prj_demo", ClusterID: "rclu_demo", Namespace: "luna-demo", ClaimName: "data",
			LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: OperationImport,
			AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		},
	}
	_, err := NewService(repository).ReserveDeploymentVolumeMount(context.Background(), ReserveMountInput{
		ProjectID: "prj_demo", ApplicationID: "app_demo", DeploymentTargetID: "dtgt_demo",
		SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: "pvol_demo", LogicalName: "data", MountPath: "/data",
	})
	if ErrorCode(err) != CodeStateConflict {
		t.Fatalf("incomplete import error=%v code=%q", err, ErrorCode(err))
	}
}
