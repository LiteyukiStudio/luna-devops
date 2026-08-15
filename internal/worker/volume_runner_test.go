package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/trace"
)

type volumeWorkerServiceStub struct {
	volumeWorkerService
	getFn                    func(context.Context, string, string) (model.ProjectVolume, error)
	getMaintenanceFn         func(context.Context, string) (model.ProjectVolume, error)
	getTransferMaintenanceFn func(context.Context, string) (model.VolumeTransfer, error)
	setLifecycleFn           func(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error)
	completeDeletionFn       func(context.Context, string, string) (model.ProjectVolume, error)
	listStaleVolumesFn       func(context.Context, volume.MaintenanceScanOptions) ([]model.ProjectVolume, error)
	listStaleTransfersFn     func(context.Context, volume.MaintenanceScanOptions) ([]model.VolumeTransfer, error)
	listExpiredFn            func(context.Context, time.Time, int) ([]model.VolumeTransfer, error)
	expireTransferFn         func(context.Context, string, string, time.Time) (model.VolumeTransfer, error)
	claimObjectCleanupFn     func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error)
	renewObjectCleanupFn     func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error)
	completeObjectFn         func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error)
	releaseObjectFn          func(context.Context, string, string, string) error
	getTransferFn            func(context.Context, string, string) (model.VolumeTransfer, error)
	listTransferPartsFn      func(context.Context, string, int, int) ([]model.VolumeTransferPart, int64, error)
	claimTransferFn          func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error)
	renewTransferLeaseFn     func(context.Context, string, string, string, int64, time.Time) (model.VolumeTransfer, error)
	prepareTransferFn        func(context.Context, string, string, string, string, int64, string, time.Time) (model.VolumeTransfer, error)
	confirmJobCreatedFn      func(context.Context, string, string, int64) (model.VolumeTransfer, error)
	markJobSucceededFn       func(context.Context, string, string) (model.VolumeTransfer, error)
	finalizeTransferFn       func(context.Context, string, string) (model.VolumeTransfer, error)
	failTransferFn           func(context.Context, string, string, string, string) (model.VolumeTransfer, error)
	markCleanupFn            func(context.Context, string, string) (model.VolumeTransfer, error)
	transitionTransferFn     func(context.Context, string, string, string, string, string) (model.VolumeTransfer, error)
	completeCancelledFn      func(context.Context, string, string, string) (model.ProjectVolume, error)
	listMountsFn             func(context.Context, string, string) ([]model.DeploymentVolumeMount, error)
	activateMountFn          func(context.Context, string, string) (model.DeploymentVolumeMount, error)
	failMountFn              func(context.Context, string, string, string, string) (model.DeploymentVolumeMount, error)
	beginUnbindFn            func(context.Context, string, string) (model.DeploymentVolumeMount, error)
	completeUnbindFn         func(context.Context, string, string) error
}

func (stub *volumeWorkerServiceStub) GetProjectVolume(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	return stub.getFn(ctx, projectID, volumeID)
}

func (stub *volumeWorkerServiceStub) GetProjectVolumeForMaintenance(ctx context.Context, volumeID string) (model.ProjectVolume, error) {
	return stub.getMaintenanceFn(ctx, volumeID)
}

func (stub *volumeWorkerServiceStub) GetVolumeTransferForMaintenance(ctx context.Context, transferID string) (model.VolumeTransfer, error) {
	if stub.getTransferMaintenanceFn == nil {
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferNotFound, Message: "not found"}
	}
	return stub.getTransferMaintenanceFn(ctx, transferID)
}

func (stub *volumeWorkerServiceStub) SetProjectVolumeLifecycle(ctx context.Context, projectID, volumeID string, from []string, to, code, message string) (model.ProjectVolume, error) {
	return stub.setLifecycleFn(ctx, projectID, volumeID, from, to, code, message)
}

func (stub *volumeWorkerServiceStub) CompleteProjectVolumeDeletion(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	return stub.completeDeletionFn(ctx, projectID, volumeID)
}

func (stub *volumeWorkerServiceStub) ListStaleProjectVolumeOperations(ctx context.Context, options volume.MaintenanceScanOptions) ([]model.ProjectVolume, error) {
	return stub.listStaleVolumesFn(ctx, options)
}

func (stub *volumeWorkerServiceStub) ListStaleVolumeTransferOperations(ctx context.Context, options volume.MaintenanceScanOptions) ([]model.VolumeTransfer, error) {
	return stub.listStaleTransfersFn(ctx, options)
}

func (stub *volumeWorkerServiceStub) ListExpiredVolumeTransferObjects(ctx context.Context, now time.Time, limit int) ([]model.VolumeTransfer, error) {
	return stub.listExpiredFn(ctx, now, limit)
}

func (stub *volumeWorkerServiceStub) ExpireVolumeTransfer(ctx context.Context, projectID, transferID string, now time.Time) (model.VolumeTransfer, error) {
	return stub.expireTransferFn(ctx, projectID, transferID, now)
}

func (stub *volumeWorkerServiceStub) ClaimVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string, expiresAt time.Time) (model.VolumeTransfer, error) {
	if stub.claimObjectCleanupFn == nil {
		return model.VolumeTransfer{}, nil
	}
	return stub.claimObjectCleanupFn(ctx, projectID, transferID, leaseToken, expiresAt)
}

func (stub *volumeWorkerServiceStub) RenewVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string, expiresAt time.Time) (model.VolumeTransfer, error) {
	if stub.renewObjectCleanupFn == nil {
		return model.VolumeTransfer{ID: transferID, ProjectID: projectID, ObjectOwned: true}, nil
	}
	return stub.renewObjectCleanupFn(ctx, projectID, transferID, leaseToken, expiresAt)
}

func (stub *volumeWorkerServiceStub) CompleteVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string, deletedAt time.Time) (model.VolumeTransfer, error) {
	if stub.completeObjectFn == nil {
		return model.VolumeTransfer{}, nil
	}
	return stub.completeObjectFn(ctx, projectID, transferID, leaseToken, deletedAt)
}

func (stub *volumeWorkerServiceStub) ReleaseVolumeTransferObjectCleanup(ctx context.Context, projectID, transferID, leaseToken string) error {
	if stub.releaseObjectFn == nil {
		return nil
	}
	return stub.releaseObjectFn(ctx, projectID, transferID, leaseToken)
}

func (stub *volumeWorkerServiceStub) GetVolumeTransfer(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	return stub.getTransferFn(ctx, projectID, transferID)
}

func (stub *volumeWorkerServiceStub) ListVolumeTransferParts(ctx context.Context, transferID string, page, pageSize int) ([]model.VolumeTransferPart, int64, error) {
	if stub.listTransferPartsFn == nil {
		return nil, 0, nil
	}
	return stub.listTransferPartsFn(ctx, transferID, page, pageSize)
}

func (stub *volumeWorkerServiceStub) ClaimVolumeTransferExecution(ctx context.Context, projectID, transferID, expectedState, leaseOwner string, leaseExpiresAt time.Time) (model.VolumeTransfer, error) {
	return stub.claimTransferFn(ctx, projectID, transferID, expectedState, leaseOwner, leaseExpiresAt)
}

func (stub *volumeWorkerServiceStub) RenewVolumeTransferExecutionLease(ctx context.Context, projectID, transferID, leaseOwner string, generation int64, leaseExpiresAt time.Time) (model.VolumeTransfer, error) {
	return stub.renewTransferLeaseFn(ctx, projectID, transferID, leaseOwner, generation, leaseExpiresAt)
}

func (stub *volumeWorkerServiceStub) PrepareVolumeTransferExecution(ctx context.Context, projectID, transferID, expectedState, leaseOwner string, generation int64, tokenHash string, expiresAt time.Time) (model.VolumeTransfer, error) {
	return stub.prepareTransferFn(ctx, projectID, transferID, expectedState, leaseOwner, generation, tokenHash, expiresAt)
}

func (stub *volumeWorkerServiceStub) ConfirmVolumeTransferJobCreated(ctx context.Context, projectID, transferID string, generation int64) (model.VolumeTransfer, error) {
	return stub.confirmJobCreatedFn(ctx, projectID, transferID, generation)
}

func (stub *volumeWorkerServiceStub) MarkVolumeTransferJobSucceeded(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	return stub.markJobSucceededFn(ctx, projectID, transferID)
}

func (stub *volumeWorkerServiceStub) FinalizeVolumeTransferExecution(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	return stub.finalizeTransferFn(ctx, projectID, transferID)
}

func (stub *volumeWorkerServiceStub) FailVolumeTransferExecution(ctx context.Context, projectID, transferID, code, message string) (model.VolumeTransfer, error) {
	return stub.failTransferFn(ctx, projectID, transferID, code, message)
}

func (stub *volumeWorkerServiceStub) MarkVolumeTransferExecutionCleanupCompleted(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	return stub.markCleanupFn(ctx, projectID, transferID)
}

func (stub *volumeWorkerServiceStub) TransitionVolumeTransfer(ctx context.Context, projectID, transferID, to, code, message string) (model.VolumeTransfer, error) {
	return stub.transitionTransferFn(ctx, projectID, transferID, to, code, message)
}

func (stub *volumeWorkerServiceStub) CompleteCancelledVolumeImport(ctx context.Context, projectID, volumeID, transferID string) (model.ProjectVolume, error) {
	return stub.completeCancelledFn(ctx, projectID, volumeID, transferID)
}

func (stub *volumeWorkerServiceStub) ListDeploymentTargetMounts(ctx context.Context, projectID, targetID string) ([]model.DeploymentVolumeMount, error) {
	return stub.listMountsFn(ctx, projectID, targetID)
}

func (stub *volumeWorkerServiceStub) ActivateDeploymentVolumeMount(ctx context.Context, projectID, mountID string) (model.DeploymentVolumeMount, error) {
	return stub.activateMountFn(ctx, projectID, mountID)
}

func (stub *volumeWorkerServiceStub) FailDeploymentVolumeMount(ctx context.Context, projectID, mountID, code, message string) (model.DeploymentVolumeMount, error) {
	return stub.failMountFn(ctx, projectID, mountID, code, message)
}

func (stub *volumeWorkerServiceStub) BeginDeploymentVolumeUnbind(ctx context.Context, projectID, mountID string) (model.DeploymentVolumeMount, error) {
	return stub.beginUnbindFn(ctx, projectID, mountID)
}

func (stub *volumeWorkerServiceStub) CompleteDeploymentVolumeUnbind(ctx context.Context, projectID, mountID string) error {
	return stub.completeUnbindFn(ctx, projectID, mountID)
}

type projectVolumeProviderStub struct {
	kubeprovider.ProjectVolumeProvider
	createFn  func(context.Context, kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error)
	deleteFn  func(context.Context, string, string, string, string) error
	observeFn func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error)
	inspectFn func(context.Context, kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ExistingProjectVolumeClaimInspection, error)
}

func (stub *projectVolumeProviderStub) CreateProjectVolumeClaim(ctx context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
	return stub.createFn(ctx, spec)
}

func (stub *projectVolumeProviderStub) DeleteProjectVolumeClaim(ctx context.Context, namespace, claimName, projectID, volumeID string) error {
	return stub.deleteFn(ctx, namespace, claimName, projectID, volumeID)
}

func (stub *projectVolumeProviderStub) ObserveProjectVolumeClaim(ctx context.Context, namespace, claimName string) (kubeprovider.ProjectVolumeClaimObservation, error) {
	return stub.observeFn(ctx, namespace, claimName)
}

func (stub *projectVolumeProviderStub) InspectExistingProjectVolumeClaim(ctx context.Context, spec kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ExistingProjectVolumeClaimInspection, error) {
	return stub.inspectFn(ctx, spec)
}

type volumeTaskEnqueuerStub struct {
	volumeTaskEnqueuer
	provisionFn func(context.Context, tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error)
	importFn    func(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error)
	exportFn    func(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error)
	deleteFn    func(context.Context, tasks.VolumeDeletePayload) (*asynq.TaskInfo, error)
}

func (stub *volumeTaskEnqueuerStub) EnqueueVolumeProvision(ctx context.Context, payload tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error) {
	return stub.provisionFn(ctx, payload)
}

func (stub *volumeTaskEnqueuerStub) EnqueueVolumeImport(ctx context.Context, payload tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	return stub.importFn(ctx, payload)
}

func (stub *volumeTaskEnqueuerStub) EnqueueVolumeExport(ctx context.Context, payload tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	return stub.exportFn(ctx, payload)
}

func (stub *volumeTaskEnqueuerStub) EnqueueVolumeDelete(ctx context.Context, payload tasks.VolumeDeletePayload) (*asynq.TaskInfo, error) {
	return stub.deleteFn(ctx, payload)
}

type volumeStoreStub struct {
	volumestore.Store
	abortFn  func(context.Context, string, string) error
	deleteFn func(context.Context, string) error
}

func (stub *volumeStoreStub) AbortMultipart(ctx context.Context, key, uploadID string) error {
	return stub.abortFn(ctx, key, uploadID)
}

func (stub *volumeStoreStub) Delete(ctx context.Context, key string) error {
	return stub.deleteFn(ctx, key)
}

func TestHandleVolumeProvisionPropagatesTraceAndMarksReady(t *testing.T) {
	projectVolume := managedProjectVolume(model.ProjectVolumeLifecycleProvisioning, volume.OperationProvision)
	traceID := trace.TraceID{1, 2, 3, 4}
	spanID := trace.SpanID{5, 6, 7, 8}
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true,
	}))
	var created kubeprovider.ProjectVolumeClaimSpec
	provider := &projectVolumeProviderStub{createFn: func(received context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
		if got := trace.SpanContextFromContext(received).TraceID(); got != traceID {
			t.Fatalf("provider trace ID = %s, want %s", got, traceID)
		}
		created = spec
		return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
	}}
	setCalled := false
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		setLifecycleFn: func(_ context.Context, _, _ string, from []string, to, code, message string) (model.ProjectVolume, error) {
			setCalled = true
			if len(from) != 1 || from[0] != model.ProjectVolumeLifecycleProvisioning || to != model.ProjectVolumeLifecycleReady || code != "" || message != "" {
				t.Fatalf("unexpected transition: from=%v to=%q code=%q message=%q", from, to, code, message)
			}
			return projectVolume, nil
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
		return provider, nil
	}}
	task, err := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.handleVolumeProvision(ctx, task); err != nil {
		t.Fatalf("handleVolumeProvision() error = %v", err)
	}
	if !setCalled || created.ClaimName != projectVolume.ClaimName || created.ProjectID != projectVolume.ProjectID {
		t.Fatalf("provisioning did not reach provider/lifecycle: set=%t spec=%+v", setCalled, created)
	}
}

func TestHandleVolumeProvisionCancellationStopsProvider(t *testing.T) {
	projectVolume := managedProjectVolume(model.ProjectVolumeLifecycleProvisioning, volume.OperationProvision)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &projectVolumeProviderStub{createFn: func(received context.Context, _ kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
		<-received.Done()
		return kubeprovider.ProjectVolumeClaimObservation{}, received.Err()
	}}
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		setLifecycleFn: func(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error) {
			t.Fatal("cancelled attempt must not enter a terminal state")
			return model.ProjectVolume{}, nil
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
		return provider, nil
	}}
	task, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err := runner.handleVolumeProvision(ctx, task); !errors.Is(err, context.Canceled) {
		t.Fatalf("handleVolumeProvision() error = %v, want context.Canceled", err)
	}
}

func TestHandleVolumeProvisionPermanentFailureUsesStableCode(t *testing.T) {
	projectVolume := managedProjectVolume(model.ProjectVolumeLifecycleProvisioning, volume.OperationProvision)
	provider := &projectVolumeProviderStub{createFn: func(context.Context, kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
		return kubeprovider.ProjectVolumeClaimObservation{}, kubeprovider.ErrProjectVolumeOwnershipConflict
	}}
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		setLifecycleFn: func(_ context.Context, _, _ string, _ []string, to, code, internalMessage string) (model.ProjectVolume, error) {
			if to != model.ProjectVolumeLifecycleError || code != volume.CodeOwnershipConflict || internalMessage == "" {
				t.Fatalf("unexpected failure transition: to=%q code=%q message=%q", to, code, internalMessage)
			}
			return projectVolume, nil
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
		return provider, nil
	}}
	task, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	err := runner.handleVolumeProvision(context.Background(), task)
	if !errors.Is(err, asynq.SkipRetry) || err.Error() != "skip retry for the task: "+volume.CodeOwnershipConflict {
		t.Fatalf("permanent error = %q, want stable skip-retry code", err)
	}
}

func TestHandleVolumeDeleteCompletesOnlyAfterClaimDisappears(t *testing.T) {
	projectVolume := managedProjectVolume(model.ProjectVolumeLifecycleDeleting, volume.OperationDelete)
	completed := false
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		completeDeletionFn: func(context.Context, string, string) (model.ProjectVolume, error) {
			completed = true
			return projectVolume, nil
		},
	}
	claimExists := true
	provider := &projectVolumeProviderStub{
		deleteFn: func(context.Context, string, string, string, string) error { return nil },
		observeFn: func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if claimExists {
				return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
			}
			return kubeprovider.ProjectVolumeClaimObservation{}, kubeprovider.ErrProjectVolumeClaimNotFound
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
		return provider, nil
	}}
	task, _ := tasks.NewVolumeDeleteTask(tasks.VolumeDeletePayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err := runner.handleVolumeDelete(context.Background(), task); err == nil || err.Error() != volume.CodeDeletionPending || completed {
		t.Fatalf("first delete error=%v completed=%t", err, completed)
	}
	claimExists = false
	if err := runner.handleVolumeDelete(context.Background(), task); err != nil || !completed {
		t.Fatalf("second delete error=%v completed=%t", err, completed)
	}
}

func TestHandleVolumeReconcileUsesBoundedMaintenanceScan(t *testing.T) {
	projectVolume := managedProjectVolume(model.ProjectVolumeLifecycleProvisioning, volume.OperationProvision)
	transfer := model.VolumeTransfer{ID: "vtx_1", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID, Direction: model.VolumeTransferDirectionExport, ActorID: "usr_1"}
	service := &volumeWorkerServiceStub{
		listStaleVolumesFn: func(_ context.Context, options volume.MaintenanceScanOptions) ([]model.ProjectVolume, error) {
			if options.Limit != volumeMaintenanceBatch || options.Cutoff.IsZero() {
				t.Fatalf("unexpected volume scan options: %+v", options)
			}
			return []model.ProjectVolume{projectVolume}, nil
		},
		listStaleTransfersFn: func(_ context.Context, options volume.MaintenanceScanOptions) ([]model.VolumeTransfer, error) {
			if options.Limit != volumeMaintenanceBatch {
				t.Fatalf("unexpected transfer scan options: %+v", options)
			}
			return []model.VolumeTransfer{transfer}, nil
		},
	}
	provisioned, exported := false, false
	enqueuer := &volumeTaskEnqueuerStub{
		provisionFn: func(_ context.Context, payload tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error) {
			provisioned = payload.VolumeID == projectVolume.ID
			return nil, nil
		},
		exportFn: func(_ context.Context, payload tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
			exported = payload.TransferID == transfer.ID
			return nil, nil
		},
	}
	runner := &Runner{volumeService: service, volumeTaskEnqueuer: enqueuer}
	task, _ := tasks.NewVolumeReconcileTask(tasks.VolumeReconcilePayload{ActorID: "system"})
	if err := runner.handleVolumeReconcile(context.Background(), task); err != nil || !provisioned || !exported {
		t.Fatalf("reconcile error=%v provisioned=%t exported=%t", err, provisioned, exported)
	}
}

func TestHandleVolumeTransferCleanupExpiresAndDeletesObject(t *testing.T) {
	projectVolume := managedProjectVolume(model.ProjectVolumeLifecycleProvisioning, volume.OperationImport)
	projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
	transfer := model.VolumeTransfer{
		ID: "vtx_1", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID, Direction: model.VolumeTransferDirectionImport,
		State:       model.VolumeTransferStateUploading,
		ObjectOwned: true,
		ObjectKey:   "transfers/vtx_1", MultipartUploadID: "upload_1", ExpiresAt: time.Now().Add(-time.Hour),
	}
	marked, aborted, deleted, volumeFailed := false, false, false, false
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		listExpiredFn: func(context.Context, time.Time, int) ([]model.VolumeTransfer, error) {
			return []model.VolumeTransfer{transfer}, nil
		},
		expireTransferFn: func(context.Context, string, string, time.Time) (model.VolumeTransfer, error) {
			transfer.State = model.VolumeTransferStateExpired
			return transfer, nil
		},
		setLifecycleFn: func(_ context.Context, projectID, volumeID string, from []string, to, code, message string) (model.ProjectVolume, error) {
			if projectID != transfer.ProjectID || volumeID != transfer.ProjectVolumeID || len(from) != 1 ||
				from[0] != model.ProjectVolumeLifecycleProvisioning || to != model.ProjectVolumeLifecycleError ||
				code != volume.CodeTransferExpired || message != "volume import upload expired" {
				t.Fatalf("unexpected expired import transition: project=%q volume=%q from=%v to=%q code=%q message=%q", projectID, volumeID, from, to, code, message)
			}
			volumeFailed = true
			return model.ProjectVolume{}, nil
		},
		claimObjectCleanupFn: func(_ context.Context, _, _ string, _ string, _ time.Time) (model.VolumeTransfer, error) {
			transfer.ObjectOwned = true
			return transfer, nil
		},
		completeObjectFn: func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error) {
			marked = true
			return transfer, nil
		},
		markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			now := time.Now().UTC()
			transfer.ExecutionCleanupCompletedAt = &now
			return transfer, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{cleanupFn: func(context.Context, string, string) error { return nil }}
	store := &volumeStoreStub{
		abortFn:  func(context.Context, string, string) error { aborted = true; return nil },
		deleteFn: func(context.Context, string) error { deleted = true; return nil },
	}
	runner := &Runner{
		volumeService: service, volumeTransferStore: store,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return jobProvider, nil
		},
	}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{ActorID: "system"})
	if err := runner.handleVolumeTransferCleanup(context.Background(), task); err != nil || !marked || !aborted || !deleted || !volumeFailed {
		t.Fatalf("cleanup error=%v marked=%t aborted=%t deleted=%t volumeFailed=%t", err, marked, aborted, deleted, volumeFailed)
	}
}

func TestPeriodicObjectCleanupWaitsForDurableExecutionCleanup(t *testing.T) {
	projectVolume := managedProjectVolume(model.ProjectVolumeLifecycleReady, "")
	transfer := model.VolumeTransfer{
		ID: "vtx_cleanup_order", ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
		Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
		ConsistencyMode: model.VolumeTransferConsistencyUnmounted, State: model.VolumeTransferStateUploading,
		ObjectKey: "transfers/cleanup-order", ObjectOwned: true, ExpiresAt: time.Now().Add(-time.Hour),
	}
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		listExpiredFn: func(context.Context, time.Time, int) ([]model.VolumeTransfer, error) {
			return []model.VolumeTransfer{transfer}, nil
		},
		expireTransferFn: func(context.Context, string, string, time.Time) (model.VolumeTransfer, error) {
			transfer.State = model.VolumeTransferStateExpired
			return transfer, nil
		},
		markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			t.Fatal("cleanup marker must not be written after provider failure")
			return model.VolumeTransfer{}, nil
		},
		claimObjectCleanupFn: func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error) {
			t.Fatal("object deletion marker must wait for execution cleanup")
			return model.VolumeTransfer{}, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{cleanupFn: func(context.Context, string, string) error {
		return errors.New("temporary Kubernetes cleanup failure")
	}}
	store := &volumeStoreStub{
		abortFn: func(context.Context, string, string) error {
			t.Fatal("multipart cleanup must wait for execution cleanup")
			return nil
		},
		deleteFn: func(context.Context, string) error {
			t.Fatal("object cleanup must wait for execution cleanup")
			return nil
		},
	}
	runner := &Runner{
		volumeService: service, volumeTransferStore: store,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return jobProvider, nil
		},
	}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{ActorID: "system"})
	if err := runner.handleVolumeTransferCleanup(context.Background(), task); err == nil || err.Error() != volume.CodeClusterUnavailable {
		t.Fatalf("periodic cleanup failure error=%v", err)
	}
}

func TestTargetedObjectCleanupUsesExactMaintenanceLookup(t *testing.T) {
	t.Parallel()
	called := false
	service := &volumeWorkerServiceStub{
		getTransferMaintenanceFn: func(_ context.Context, transferID string) (model.VolumeTransfer, error) {
			called = true
			if transferID != "vtx_beyond_periodic_batch" {
				t.Fatalf("transfer id = %q", transferID)
			}
			return model.VolumeTransfer{
				ID: transferID, ProjectID: "prj_1", ObjectOwned: true,
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			}, nil
		},
		listExpiredFn: func(context.Context, time.Time, int) ([]model.VolumeTransfer, error) {
			t.Fatal("targeted cleanup must not scan or filter the first periodic batch")
			return nil, nil
		},
	}
	runner := &Runner{volumeService: service}
	task, err := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{
		TransferID: "vtx_beyond_periodic_batch", ActorID: "system",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.handleVolumeTransferCleanup(context.Background(), task); err != nil {
		t.Fatalf("targeted cleanup: %v", err)
	}
	if !called {
		t.Fatal("targeted maintenance lookup was not called")
	}
}

func managedProjectVolume(state, operation string) model.ProjectVolume {
	return model.ProjectVolume{
		ID: "pvol_1", ProjectID: "prj_1", ClusterID: "rcl_1", Namespace: "project-1", ClaimName: "luna-pvol-1",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		LifecycleState: state, PendingOperation: operation, CapacityRequest: "1Gi", CapacityBytes: 1 << 30,
		StorageClassName: "standard", AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
	}
}
