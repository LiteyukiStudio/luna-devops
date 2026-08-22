package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/trace"
)

type volumeWorkerServiceStub struct {
	volumeWorkerService
	getVolumeFn        func(context.Context, string, string) (model.ProjectVolume, error)
	getFn              func(context.Context, string, string) (model.ProjectVolume, error)
	getTransferFn      func(context.Context, string, string) (model.VolumeTransfer, error)
	getMaintenanceFn   func(context.Context, string) (model.VolumeTransfer, error)
	claimFn            func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error)
	renewFn            func(context.Context, string, string, string, int64, time.Time) (model.VolumeTransfer, error)
	confirmFn          func(context.Context, string, string, int64) (model.VolumeTransfer, error)
	readyFn            func(context.Context, string, string, int64) (model.VolumeTransfer, error)
	failStaleFn        func(context.Context, string, string, time.Time, string, string) (model.VolumeTransfer, error)
	cleanupFn          func(context.Context, string, string) (model.VolumeTransfer, error)
	listExpiredFn      func(context.Context, time.Time, int) ([]model.VolumeTransfer, error)
	expireFn           func(context.Context, string, string, time.Time) (model.VolumeTransfer, error)
	listMountsFn       func(context.Context, string, string) ([]model.DeploymentVolumeMount, error)
	setLifecycleFn     func(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error)
	completeDeletionFn func(context.Context, string, string) (model.ProjectVolume, error)
}

func (s *volumeWorkerServiceStub) GetProjectVolume(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	if s.getVolumeFn == nil {
		return s.getFn(ctx, projectID, volumeID)
	}
	return s.getVolumeFn(ctx, projectID, volumeID)
}
func (s *volumeWorkerServiceStub) ListDeploymentTargetMounts(ctx context.Context, projectID, targetID string) ([]model.DeploymentVolumeMount, error) {
	return s.listMountsFn(ctx, projectID, targetID)
}
func (s *volumeWorkerServiceStub) SetProjectVolumeLifecycle(ctx context.Context, projectID, volumeID string, from []string, to, code, message string) (model.ProjectVolume, error) {
	return s.setLifecycleFn(ctx, projectID, volumeID, from, to, code, message)
}
func (s *volumeWorkerServiceStub) CompleteProjectVolumeDeletion(ctx context.Context, projectID, volumeID string) (model.ProjectVolume, error) {
	return s.completeDeletionFn(ctx, projectID, volumeID)
}
func (s *volumeWorkerServiceStub) GetVolumeTransfer(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	return s.getTransferFn(ctx, projectID, transferID)
}
func (s *volumeWorkerServiceStub) GetVolumeTransferForMaintenance(ctx context.Context, transferID string) (model.VolumeTransfer, error) {
	return s.getMaintenanceFn(ctx, transferID)
}
func (s *volumeWorkerServiceStub) ClaimVolumeTransferExecution(ctx context.Context, projectID, transferID, state, owner string, expires time.Time) (model.VolumeTransfer, error) {
	return s.claimFn(ctx, projectID, transferID, state, owner, expires)
}
func (s *volumeWorkerServiceStub) RenewVolumeTransferExecutionLease(ctx context.Context, projectID, transferID, owner string, generation int64, expires time.Time) (model.VolumeTransfer, error) {
	return s.renewFn(ctx, projectID, transferID, owner, generation, expires)
}
func (s *volumeWorkerServiceStub) ConfirmVolumeTransferJobCreated(ctx context.Context, projectID, transferID string, generation int64) (model.VolumeTransfer, error) {
	return s.confirmFn(ctx, projectID, transferID, generation)
}
func (s *volumeWorkerServiceStub) MarkVolumeTransferReady(ctx context.Context, projectID, transferID string, generation int64) (model.VolumeTransfer, error) {
	return s.readyFn(ctx, projectID, transferID, generation)
}
func (s *volumeWorkerServiceStub) FailStaleVolumeTransfer(ctx context.Context, projectID, transferID string, cutoff time.Time, code, message string) (model.VolumeTransfer, error) {
	return s.failStaleFn(ctx, projectID, transferID, cutoff, code, message)
}
func (s *volumeWorkerServiceStub) MarkVolumeTransferExecutionCleanupCompleted(ctx context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	return s.cleanupFn(ctx, projectID, transferID)
}
func (s *volumeWorkerServiceStub) ListExpiredVolumeTransfers(ctx context.Context, now time.Time, limit int) ([]model.VolumeTransfer, error) {
	return s.listExpiredFn(ctx, now, limit)
}
func (s *volumeWorkerServiceStub) ExpireVolumeTransfer(ctx context.Context, projectID, transferID string, now time.Time) (model.VolumeTransfer, error) {
	return s.expireFn(ctx, projectID, transferID, now)
}

type projectVolumeProviderStub struct {
	kubeprovider.ProjectVolumeProvider
	createFn         func(context.Context, kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error)
	deleteFn         func(context.Context, string, string, string, string) error
	createSnapshotFn func(context.Context, kubeprovider.ProjectVolumeSnapshotSpec) (kubeprovider.VolumeSnapshotObservation, error)
	observeFn        func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error)
}

func (s *projectVolumeProviderStub) CreateProjectVolumeClaim(ctx context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
	return s.createFn(ctx, spec)
}
func (s *projectVolumeProviderStub) DeleteProjectVolumeClaim(ctx context.Context, namespace, claim, projectID, volumeID string) error {
	return s.deleteFn(ctx, namespace, claim, projectID, volumeID)
}
func (s *projectVolumeProviderStub) ObserveProjectVolumeClaim(ctx context.Context, namespace, claim string) (kubeprovider.ProjectVolumeClaimObservation, error) {
	return s.observeFn(ctx, namespace, claim)
}
func (s *projectVolumeProviderStub) CreateVolumeSnapshot(ctx context.Context, spec kubeprovider.ProjectVolumeSnapshotSpec) (kubeprovider.VolumeSnapshotObservation, error) {
	return s.createSnapshotFn(ctx, spec)
}

type volumeTransferProviderStub struct {
	kubeprovider.VolumeTransferProvider
	prepareFn func(context.Context, kubeprovider.VolumeTransferSpec) (kubeprovider.VolumeTransferReference, error)
	observeFn func(context.Context, string, string) (kubeprovider.VolumeTransferObservation, error)
	cleanupFn func(context.Context, string, string) error
}

func (s *volumeTransferProviderStub) PrepareVolumeTransfer(ctx context.Context, spec kubeprovider.VolumeTransferSpec) (kubeprovider.VolumeTransferReference, error) {
	return s.prepareFn(ctx, spec)
}
func (s *volumeTransferProviderStub) ObserveVolumeTransfer(ctx context.Context, namespace, transferID string) (kubeprovider.VolumeTransferObservation, error) {
	return s.observeFn(ctx, namespace, transferID)
}
func (s *volumeTransferProviderStub) CleanupVolumeTransfer(ctx context.Context, namespace, transferID string) error {
	return s.cleanupFn(ctx, namespace, transferID)
}

func TestHandleVolumeProvisionPropagatesTraceAndMarksReady(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.PendingOperation = volume.OperationProvision
	traceID := trace.TraceID{1, 2, 3, 4}
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: trace.SpanID{5, 6, 7, 8}, TraceFlags: trace.FlagsSampled, Remote: true,
	}))
	var created kubeprovider.ProjectVolumeClaimSpec
	provider := &projectVolumeProviderStub{createFn: func(received context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
		if trace.SpanContextFromContext(received).TraceID() != traceID {
			t.Fatal("claim creation lost parent trace")
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
				t.Fatalf("transition from=%v to=%q code=%q message=%q", from, to, code, message)
			}
			return projectVolume, nil
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return provider, nil }}
	task, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err := runner.handleVolumeProvision(ctx, task); err != nil || !setCalled || created.ClaimName != projectVolume.ClaimName {
		t.Fatalf("provision error=%v set=%t spec=%#v", err, setCalled, created)
	}
}

func TestHandleVolumeProvisionCancellationStopsProvider(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.PendingOperation = volume.OperationProvision
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &projectVolumeProviderStub{createFn: func(received context.Context, _ kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
		return kubeprovider.ProjectVolumeClaimObservation{}, received.Err()
	}}
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		setLifecycleFn: func(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error) {
			t.Fatal("cancelled attempt entered terminal state")
			return model.ProjectVolume{}, nil
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return provider, nil }}
	task, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err := runner.handleVolumeProvision(ctx, task); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleVolumeProvisionPermanentFailureUsesStableCode(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.PendingOperation = volume.OperationProvision
	provider := &projectVolumeProviderStub{createFn: func(context.Context, kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
		return kubeprovider.ProjectVolumeClaimObservation{}, kubeprovider.ErrProjectVolumeOwnershipConflict
	}}
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		setLifecycleFn: func(_ context.Context, _, _ string, _ []string, to, code, message string) (model.ProjectVolume, error) {
			if to != model.ProjectVolumeLifecycleError || code != volume.CodeOwnershipConflict || message == "" {
				t.Fatalf("transition to=%q code=%q message=%q", to, code, message)
			}
			return projectVolume, nil
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return provider, nil }}
	task, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err := runner.handleVolumeProvision(context.Background(), task); !errors.Is(err, asynq.SkipRetry) || err.Error() != "skip retry for the task: "+volume.CodeOwnershipConflict {
		t.Fatalf("error = %v", err)
	}
}

func TestHandleVolumeDeleteCompletesOnlyAfterClaimDisappears(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.LifecycleState = model.ProjectVolumeLifecycleDeleting
	projectVolume.PendingOperation = volume.OperationDelete
	claimExists := true
	completed := false
	provider := &projectVolumeProviderStub{
		deleteFn: func(context.Context, string, string, string, string) error { return nil },
		observeFn: func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if claimExists {
				return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
			}
			return kubeprovider.ProjectVolumeClaimObservation{}, kubeprovider.ErrProjectVolumeClaimNotFound
		},
	}
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		completeDeletionFn: func(context.Context, string, string) (model.ProjectVolume, error) {
			completed = true
			return projectVolume, nil
		},
	}
	runner := &Runner{volumeService: service, projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return provider, nil }}
	task, _ := tasks.NewVolumeDeleteTask(tasks.VolumeDeletePayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err := runner.handleVolumeDelete(context.Background(), task); err == nil || completed {
		t.Fatalf("first delete error=%v completed=%t", err, completed)
	}
	claimExists = false
	if err := runner.handleVolumeDelete(context.Background(), task); err != nil || !completed {
		t.Fatalf("second delete error=%v completed=%t", err, completed)
	}
}

func TestHandleVolumeImportPreparesPodAndStopsAtReady(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionImport)
	claimed := transfer
	claimed.ExecutionGeneration = 1
	confirmed := claimed
	now := time.Now().UTC()
	confirmed.JobCreatedAt = &now
	ready := false
	traceID := trace.TraceID{1, 2, 3, 4}
	spanID := trace.SpanID{5, 6, 7, 8}
	requestCtx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true,
	}))
	service := &volumeWorkerServiceStub{
		getVolumeFn:   func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		claimFn: func(_ context.Context, _, _, state, owner string, expires time.Time) (model.VolumeTransfer, error) {
			if state != model.VolumeTransferStatePreparing || owner == "" || !expires.After(time.Now()) {
				t.Fatalf("invalid preparation lease state=%q owner=%q", state, owner)
			}
			return claimed, nil
		},
		renewFn: func(context.Context, string, string, string, int64, time.Time) (model.VolumeTransfer, error) {
			return claimed, nil
		},
		confirmFn: func(context.Context, string, string, int64) (model.VolumeTransfer, error) { return confirmed, nil },
		readyFn: func(received context.Context, _, _ string, generation int64) (model.VolumeTransfer, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID {
				t.Fatal("ready transition lost the parent trace")
			}
			if generation != 1 {
				t.Fatalf("generation = %d", generation)
			}
			ready = true
			result := confirmed
			result.State = model.VolumeTransferStateReady
			return result, nil
		},
	}
	claimProvider := &projectVolumeProviderStub{createFn: func(_ context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
		if spec.ClaimName != projectVolume.ClaimName {
			t.Fatalf("claim = %q", spec.ClaimName)
		}
		return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
	}, observeFn: func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error) {
		return kubeprovider.ProjectVolumeClaimObservation{}, kubeprovider.ErrProjectVolumeClaimNotFound
	}}
	streamProvider := &volumeTransferProviderStub{
		prepareFn: func(received context.Context, spec kubeprovider.VolumeTransferSpec) (kubeprovider.VolumeTransferReference, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID {
				t.Fatal("Pod preparation lost the parent trace")
			}
			if spec.ExpectedBytes != transfer.ExpectedBytes || spec.ExpectedSHA256 != transfer.SHA256 ||
				spec.Direction != model.VolumeTransferDirectionImport || spec.MaxArchiveBytes != 1<<30 {
				t.Fatalf("spec = %#v", spec)
			}
			return kubeprovider.VolumeTransferReference{PodName: "luna-vtx-test"}, nil
		},
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferObservation, error) {
			return kubeprovider.VolumeTransferObservation{State: "ready", Reason: "ready"}, nil
		},
	}
	runner := &Runner{volumeService: service, volumeTransferJobImage: "worker:test",
		volumeTransferMaxBytes:       1 << 30,
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return claimProvider, nil },
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return streamProvider, nil
		}}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeImport(requestCtx, task); err != nil || !ready {
		t.Fatalf("handle import error=%v ready=%t", err, ready)
	}
}

func TestPrepareSnapshotExportUsesTemporaryClaim(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionExport)
	transfer.ConsistencyMode = model.VolumeTransferConsistencySnapshot
	var snapshotName string
	provider := &projectVolumeProviderStub{
		createSnapshotFn: func(_ context.Context, spec kubeprovider.ProjectVolumeSnapshotSpec) (kubeprovider.VolumeSnapshotObservation, error) {
			snapshotName = spec.Name
			return kubeprovider.VolumeSnapshotObservation{ReadyToUse: true}, nil
		},
		createFn: func(_ context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if spec.SourceSnapshotName != snapshotName || spec.SourceSnapshotKind != "VolumeSnapshot" {
				t.Fatalf("snapshot claim spec = %#v", spec)
			}
			return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
		},
	}
	runner := &Runner{projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return provider, nil }}
	claim, cleanup, err := runner.prepareVolumeTransferClaim(context.Background(), projectVolume, transfer)
	if err != nil || cleanup == nil || claim != cleanup.claimName || snapshotName != cleanup.snapshotName {
		t.Fatalf("claim=%q cleanup=%#v snapshot=%q err=%v", claim, cleanup, snapshotName, err)
	}
}

func TestPrepareLiveBlockExportIsRejected(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeBlock)
	transfer := transferFixture(model.VolumeTransferDirectionExport)
	transfer.ConsistencyMode = model.VolumeTransferConsistencyLive
	runner := &Runner{projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
		return &projectVolumeProviderStub{}, nil
	}}
	if _, _, err := runner.prepareVolumeTransferClaim(context.Background(), projectVolume, transfer); volume.ErrorCode(err) != volume.CodeTransferStateConflict {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareImportReplacesPartialClaimBeforeRetry(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionImport)
	deleted := false
	provider := &projectVolumeProviderStub{
		observeFn: func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if !deleted {
				return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
			}
			return kubeprovider.ProjectVolumeClaimObservation{}, kubeprovider.ErrProjectVolumeClaimNotFound
		},
		deleteFn: func(context.Context, string, string, string, string) error { deleted = true; return nil },
		createFn: func(context.Context, kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if !deleted {
				t.Fatal("partial claim was reused")
			}
			return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
		},
	}
	runner := &Runner{projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return provider, nil }}
	claim, _, err := runner.prepareVolumeTransferClaim(context.Background(), projectVolume, transfer)
	if err != nil || !deleted || claim != projectVolume.ClaimName {
		t.Fatalf("claim=%q deleted=%t err=%v", claim, deleted, err)
	}
}

func TestVolumeTransferCleanupExpiresReadySessionAndRemovesPod(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionExport)
	transfer.State = model.VolumeTransferStateReady
	transfer.ExpiresAt = time.Now().Add(-time.Minute)
	cleaned := false
	service := &volumeWorkerServiceStub{
		listExpiredFn: func(context.Context, time.Time, int) ([]model.VolumeTransfer, error) {
			return []model.VolumeTransfer{transfer}, nil
		},
		expireFn: func(context.Context, string, string, time.Time) (model.VolumeTransfer, error) {
			transfer.State = model.VolumeTransferStateExpired
			return transfer, nil
		},
		getVolumeFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		cleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			cleaned = true
			now := time.Now().UTC()
			transfer.ExecutionCleanupCompletedAt = &now
			return transfer, nil
		},
	}
	provider := &volumeTransferProviderStub{cleanupFn: func(context.Context, string, string) error { return nil }}
	runner := &Runner{volumeService: service, volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) { return provider, nil }}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{ActorID: "system"})
	if err := runner.handleVolumeTransferCleanup(context.Background(), task); err != nil || !cleaned {
		t.Fatalf("cleanup error=%v cleaned=%t", err, cleaned)
	}
}

func TestTargetedCleanupImmediatelyRemovesSucceededExportRuntime(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionExport)
	transfer.State = model.VolumeTransferStateSucceeded
	transfer.ExpiresAt = time.Now().UTC().Add(time.Hour)
	runtimeCleaned := false
	marked := false
	service := &volumeWorkerServiceStub{
		getMaintenanceFn: func(context.Context, string) (model.VolumeTransfer, error) { return transfer, nil },
		getVolumeFn:      func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		cleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			marked = true
			return transfer, nil
		},
	}
	provider := &volumeTransferProviderStub{cleanupFn: func(context.Context, string, string) error {
		runtimeCleaned = true
		return nil
	}}
	runner := &Runner{volumeService: service, volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
		return provider, nil
	}}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{TransferID: transfer.ID, ActorID: "system"})
	if err := runner.handleVolumeTransferCleanup(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if !runtimeCleaned || !marked {
		t.Fatalf("runtime cleaned=%t cleanup marked=%t", runtimeCleaned, marked)
	}
}

func TestTargetedCleanupImmediatelyRemovesFailedImportClaim(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionImport)
	transfer.State = model.VolumeTransferStateFailed
	transfer.ExpiresAt = time.Now().UTC().Add(time.Hour)
	claimDeleted := false
	service := &volumeWorkerServiceStub{
		getMaintenanceFn: func(context.Context, string) (model.VolumeTransfer, error) { return transfer, nil },
		getVolumeFn:      func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		cleanupFn:        func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
	}
	streamProvider := &volumeTransferProviderStub{cleanupFn: func(context.Context, string, string) error { return nil }}
	claimProvider := &projectVolumeProviderStub{deleteFn: func(_ context.Context, namespace, claim, projectID, volumeID string) error {
		if namespace != projectVolume.Namespace || claim != projectVolume.ClaimName || projectID != projectVolume.ProjectID || volumeID != projectVolume.ID {
			t.Fatalf("deleted claim namespace=%q claim=%q project=%q volume=%q", namespace, claim, projectID, volumeID)
		}
		claimDeleted = true
		return nil
	}}
	runner := &Runner{
		volumeService: service,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return streamProvider, nil
		},
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
			return claimProvider, nil
		},
	}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{TransferID: transfer.ID, ActorID: "system"})
	if err := runner.handleVolumeTransferCleanup(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if !claimDeleted {
		t.Fatal("failed import claim was not deleted immediately")
	}
}

func TestFailStaleStreamingImportCleansRuntimeAndPartialClaim(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionImport)
	transfer.State = model.VolumeTransferStateStreaming
	cutoff := time.Now().UTC().Add(-volumeReconcileStaleAfter)
	streamCleaned := false
	claimDeleted := false
	marked := false
	service := &volumeWorkerServiceStub{
		failStaleFn: func(_ context.Context, projectID, transferID string, receivedCutoff time.Time, code, message string) (model.VolumeTransfer, error) {
			if projectID != transfer.ProjectID || transferID != transfer.ID || !receivedCutoff.Equal(cutoff) ||
				code != volume.CodeTransferJobFailed || message == "" {
				t.Fatalf("stale failure project=%q transfer=%q cutoff=%v code=%q message=%q", projectID, transferID, receivedCutoff, code, message)
			}
			transfer.State = model.VolumeTransferStateFailed
			return transfer, nil
		},
		getVolumeFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		cleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			marked = true
			return transfer, nil
		},
	}
	streamProvider := &volumeTransferProviderStub{cleanupFn: func(context.Context, string, string) error {
		streamCleaned = true
		return nil
	}}
	claimProvider := &projectVolumeProviderStub{deleteFn: func(_ context.Context, namespace, claim, projectID, volumeID string) error {
		if namespace != projectVolume.Namespace || claim != projectVolume.ClaimName || projectID != projectVolume.ProjectID || volumeID != projectVolume.ID {
			t.Fatalf("deleted claim namespace=%q claim=%q project=%q volume=%q", namespace, claim, projectID, volumeID)
		}
		claimDeleted = true
		return nil
	}}
	runner := &Runner{
		volumeService: service,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return streamProvider, nil
		},
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
			return claimProvider, nil
		},
	}
	if err := runner.failStaleStreamingVolumeTransfer(context.Background(), service, transfer, cutoff); err != nil {
		t.Fatal(err)
	}
	if !streamCleaned || !claimDeleted || !marked {
		t.Fatalf("stream cleaned=%t claim deleted=%t cleanup marked=%t", streamCleaned, claimDeleted, marked)
	}
}

func TestFailStaleStreamingHeartbeatCASConflictDoesNotCleanup(t *testing.T) {
	transfer := transferFixture(model.VolumeTransferDirectionExport)
	transfer.State = model.VolumeTransferStateStreaming
	service := &volumeWorkerServiceStub{failStaleFn: func(context.Context, string, string, time.Time, string, string) (model.VolumeTransfer, error) {
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "stream heartbeat won"}
	}}
	runner := &Runner{
		volumeService: service,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			t.Fatal("provider cleanup ran after stale CAS conflict")
			return nil, nil
		},
	}
	if err := runner.failStaleStreamingVolumeTransfer(context.Background(), service, transfer, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestPreparationLeaseConflictDoesNotCreateSecondPod(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionExport)
	service := &volumeWorkerServiceStub{
		getVolumeFn:   func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		claimFn: func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error) {
			return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "preparation lease held"}
		},
	}
	created := false
	provider := &volumeTransferProviderStub{prepareFn: func(context.Context, kubeprovider.VolumeTransferSpec) (kubeprovider.VolumeTransferReference, error) {
		created = true
		return kubeprovider.VolumeTransferReference{}, nil
	}}
	runner := &Runner{volumeService: service, volumeTransferJobImage: "worker:test",
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
			return &projectVolumeProviderStub{}, nil
		},
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) { return provider, nil }}
	task, _ := tasks.NewVolumeExportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeExport(context.Background(), task); err == nil || created {
		t.Fatalf("error=%v second Pod created=%t", err, created)
	}
}

func transferProjectVolume(mode string) model.ProjectVolume {
	return model.ProjectVolume{ID: "pvol_test", ProjectID: "prj_test", ClusterID: "rcl_test", Namespace: "project-test",
		ClaimName: "claim-test", OwnershipMode: model.ProjectVolumeOwnershipManaged, LifecycleState: model.ProjectVolumeLifecycleProvisioning,
		CapacityRequest: "1Gi", CapacityBytes: 1 << 30, StorageClassName: "standard",
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: mode}
}

func transferFixture(direction string) model.VolumeTransfer {
	return model.VolumeTransfer{ID: "vtx_test", ProjectID: "prj_test", ProjectVolumeID: "pvol_test", Direction: direction,
		Format: model.VolumeTransferFormatTarGZ, ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		State: model.VolumeTransferStatePreparing, ExpectedBytes: 1024,
		SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now().UTC()}
}
