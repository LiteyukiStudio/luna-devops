package worker

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type volumeWorkerServiceStub struct {
	volumeWorkerService
	getVolumeFn          func(context.Context, string, string) (model.ProjectVolume, error)
	getFn                func(context.Context, string, string) (model.ProjectVolume, error)
	getTransferFn        func(context.Context, string, string) (model.VolumeTransfer, error)
	getMaintenanceFn     func(context.Context, string) (model.VolumeTransfer, error)
	claimFn              func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error)
	renewFn              func(context.Context, string, string, string, int64, time.Time) (model.VolumeTransfer, error)
	confirmFn            func(context.Context, string, string, int64) (model.VolumeTransfer, error)
	readyFn              func(context.Context, string, string, int64) (model.VolumeTransfer, error)
	failStaleFn          func(context.Context, string, string, time.Time, string, string) (model.VolumeTransfer, error)
	failExecutionFn      func(context.Context, string, string, string, string) (model.VolumeTransfer, error)
	cleanupFn            func(context.Context, string, string) (model.VolumeTransfer, error)
	listExpiredFn        func(context.Context, time.Time, int) ([]model.VolumeTransfer, error)
	expireFn             func(context.Context, string, string, time.Time) (model.VolumeTransfer, error)
	listMountsFn         func(context.Context, string, string) ([]model.DeploymentVolumeMount, error)
	setLifecycleFn       func(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error)
	completeDeletionFn   func(context.Context, string, string) (model.ProjectVolume, error)
	listStaleVolumesFn   func(context.Context, volume.MaintenanceScanOptions) ([]model.ProjectVolume, error)
	listStaleTransfersFn func(context.Context, volume.MaintenanceScanOptions) ([]model.VolumeTransfer, error)
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
func (s *volumeWorkerServiceStub) FailVolumeTransferExecution(ctx context.Context, projectID, transferID, code, message string) (model.VolumeTransfer, error) {
	return s.failExecutionFn(ctx, projectID, transferID, code, message)
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
func (s *volumeWorkerServiceStub) ListStaleProjectVolumeOperations(ctx context.Context, options volume.MaintenanceScanOptions) ([]model.ProjectVolume, error) {
	return s.listStaleVolumesFn(ctx, options)
}
func (s *volumeWorkerServiceStub) ListStaleVolumeTransferOperations(ctx context.Context, options volume.MaintenanceScanOptions) ([]model.VolumeTransfer, error) {
	return s.listStaleTransfersFn(ctx, options)
}

type volumeTaskEnqueuerStub struct {
	provision []tasks.VolumeProvisionPayload
	imports   []tasks.VolumeTransferPayload
	exports   []tasks.VolumeTransferPayload
	deletes   []tasks.VolumeDeletePayload
}

func (s *volumeTaskEnqueuerStub) EnqueueVolumeProvision(_ context.Context, payload tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error) {
	s.provision = append(s.provision, payload)
	return nil, nil
}
func (s *volumeTaskEnqueuerStub) EnqueueVolumeImport(_ context.Context, payload tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	s.imports = append(s.imports, payload)
	return nil, nil
}
func (s *volumeTaskEnqueuerStub) EnqueueVolumeExport(_ context.Context, payload tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	s.exports = append(s.exports, payload)
	return nil, nil
}
func (s *volumeTaskEnqueuerStub) EnqueueVolumeDelete(_ context.Context, payload tasks.VolumeDeletePayload) (*asynq.TaskInfo, error) {
	s.deletes = append(s.deletes, payload)
	return nil, nil
}

type projectVolumeProviderStub struct {
	kubeprovider.ProjectVolumeProvider
	createFn         func(context.Context, kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error)
	adoptFn          func(context.Context, kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error)
	expandFn         func(context.Context, string, string, string, string, string) (kubeprovider.ProjectVolumeClaimObservation, error)
	deleteFn         func(context.Context, string, string, string, string) error
	createSnapshotFn func(context.Context, kubeprovider.ProjectVolumeSnapshotSpec) (kubeprovider.VolumeSnapshotObservation, error)
	observeFn        func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error)
}

func (s *projectVolumeProviderStub) CreateProjectVolumeClaim(ctx context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
	return s.createFn(ctx, spec)
}
func (s *projectVolumeProviderStub) AdoptExistingProjectVolumeClaim(ctx context.Context, spec kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
	return s.adoptFn(ctx, spec)
}
func (s *projectVolumeProviderStub) ExpandProjectVolumeClaim(ctx context.Context, namespace, claim, projectID, volumeID, capacity string) (kubeprovider.ProjectVolumeClaimObservation, error) {
	return s.expandFn(ctx, namespace, claim, projectID, volumeID, capacity)
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
	runner := newVolumeRunnerWithStubNamespace(t, service, provider)
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
	runner := newVolumeRunnerWithStubNamespace(t, service, provider)
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
	runner := newVolumeRunnerWithStubNamespace(t, service, provider)
	task, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err := runner.handleVolumeProvision(context.Background(), task); !errors.Is(err, asynq.SkipRetry) || err.Error() != "skip retry for the task: "+volume.CodeOwnershipConflict {
		t.Fatalf("error = %v", err)
	}
}

func TestRetainedVolumeExpandAdoptsClaimBeforeExpansion(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.SourceKind = model.ProjectVolumeSourceRetained
	projectVolume.CapacityRequest = "2Gi"
	adopted := false
	provider := &projectVolumeProviderStub{
		adoptFn: func(_ context.Context, spec kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if spec.ProjectID != projectVolume.ProjectID || spec.VolumeID != projectVolume.ID ||
				spec.Namespace != projectVolume.Namespace || spec.ClaimName != projectVolume.ClaimName {
				t.Fatalf("adoption spec = %#v", spec)
			}
			if spec.ExpectedCapacity != "" || spec.ExpectedStorageClassName != "" || spec.ExpectedAccessMode != "" || spec.ExpectedVolumeMode != "" {
				t.Fatalf("retained claim adoption unexpectedly used updated desired spec: %#v", spec)
			}
			adopted = true
			return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
		},
		expandFn: func(_ context.Context, namespace, claim, projectID, volumeID, capacity string) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if !adopted {
				t.Fatal("claim expansion ran before retained claim adoption")
			}
			if namespace != projectVolume.Namespace || claim != projectVolume.ClaimName || projectID != projectVolume.ProjectID || volumeID != projectVolume.ID || capacity != "2Gi" {
				t.Fatalf("expand input namespace=%q claim=%q project=%q volume=%q capacity=%q", namespace, claim, projectID, volumeID, capacity)
			}
			return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
		},
	}
	if err := (&Runner{}).applyProjectVolumeOperation(t.Context(), provider, projectVolume, tasks.VolumeOperationExpand); err != nil {
		t.Fatalf("expand retained project volume: %v", err)
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

func TestRetainedVolumeDeleteAdoptsClaimBeforeDeletion(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.SourceKind = model.ProjectVolumeSourceRetained
	projectVolume.LifecycleState = model.ProjectVolumeLifecycleDeleting
	projectVolume.PendingOperation = volume.OperationDelete
	adopted := false
	completed := false
	provider := &projectVolumeProviderStub{
		adoptFn: func(_ context.Context, spec kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if spec.ProjectID != projectVolume.ProjectID || spec.VolumeID != projectVolume.ID ||
				spec.Namespace != projectVolume.Namespace || spec.ClaimName != projectVolume.ClaimName {
				t.Fatalf("adoption spec = %#v", spec)
			}
			adopted = true
			return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
		},
		deleteFn: func(context.Context, string, string, string, string) error {
			if !adopted {
				t.Fatal("claim deletion ran before retained claim adoption")
			}
			return kubeprovider.ErrProjectVolumeClaimNotFound
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
	if err := runner.handleVolumeDelete(t.Context(), task); err != nil || !completed {
		t.Fatalf("delete retained project volume error=%v completed=%t", err, completed)
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
	runner := newVolumeRunnerWithStubNamespace(t, service, claimProvider)
	runner.volumeTransferJobImage = "worker:test"
	runner.volumeTransferMaxBytes = 1 << 30
	runner.volumeTransferProviderFactory = func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) {
		return streamProvider, nil
	}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeImport(requestCtx, task); err != nil || !ready {
		t.Fatalf("handle import error=%v ready=%t", err, ready)
	}
}

func TestPrepareSnapshotExportUsesTemporaryClaim(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.SourceKind = model.ProjectVolumeSourceRetained
	transfer := transferFixture(model.VolumeTransferDirectionExport)
	transfer.ConsistencyMode = model.VolumeTransferConsistencySnapshot
	var snapshotName string
	adopted := false
	provider := &projectVolumeProviderStub{
		adoptFn: func(_ context.Context, spec kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if spec.ProjectID != projectVolume.ProjectID || spec.VolumeID != projectVolume.ID ||
				spec.Namespace != projectVolume.Namespace || spec.ClaimName != projectVolume.ClaimName {
				t.Fatalf("adoption spec = %#v", spec)
			}
			adopted = true
			return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
		},
		createSnapshotFn: func(_ context.Context, spec kubeprovider.ProjectVolumeSnapshotSpec) (kubeprovider.VolumeSnapshotObservation, error) {
			if !adopted {
				t.Fatal("snapshot creation ran before retained claim adoption")
			}
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
	if err != nil || !adopted || cleanup == nil || claim != cleanup.claimName || snapshotName != cleanup.snapshotName {
		t.Fatalf("claim=%q cleanup=%#v snapshot=%q adopted=%t err=%v", claim, cleanup, snapshotName, adopted, err)
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
	runner := &Runner{volumeService: service, volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) { return provider, nil }}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{})
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
	runner := &Runner{volumeService: service, volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) {
		return provider, nil
	}}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{TransferID: transfer.ID})
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
		volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) {
			return streamProvider, nil
		},
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
			return claimProvider, nil
		},
	}
	task, _ := tasks.NewVolumeTransferCleanupTask(tasks.VolumeTransferCleanupPayload{TransferID: transfer.ID})
	if err := runner.handleVolumeTransferCleanup(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if !claimDeleted {
		t.Fatal("failed import claim was not deleted immediately")
	}
}

func TestVolumeReconcileUsesQueueBudgetWindowForCrashRecovery(t *testing.T) {
	startedAt := time.Now().UTC()
	var transferCutoffs []time.Time
	service := &volumeWorkerServiceStub{
		listStaleVolumesFn: func(_ context.Context, options volume.MaintenanceScanOptions) ([]model.ProjectVolume, error) {
			want := startedAt.Add(-volumeOperationRecoveryAfter)
			if options.Cutoff.Before(want.Add(-time.Second)) || options.Cutoff.After(want.Add(time.Second)) {
				t.Fatalf("project recovery cutoff = %v, want around %v", options.Cutoff, want)
			}
			return []model.ProjectVolume{{
				ID: "pvol_recover", ProjectID: "prj_1", LifecycleState: model.ProjectVolumeLifecycleProvisioning, PendingOperation: volume.OperationProvision,
			}}, nil
		},
		listStaleTransfersFn: func(_ context.Context, options volume.MaintenanceScanOptions) ([]model.VolumeTransfer, error) {
			transferCutoffs = append(transferCutoffs, options.Cutoff)
			if len(transferCutoffs) == 1 {
				return []model.VolumeTransfer{{
					ID: "vtx_recent_preparing", ProjectID: "prj_1", ProjectVolumeID: "pvol_1", Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStatePreparing,
				}}, nil
			}
			return []model.VolumeTransfer{
				{ID: "vtx_recover", ProjectID: "prj_1", ProjectVolumeID: "pvol_1", Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStatePreparing},
				{ID: "vtx_ready", ProjectID: "prj_1", ProjectVolumeID: "pvol_1", Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateReady},
				{ID: "vtx_streaming", ProjectID: "prj_1", ProjectVolumeID: "pvol_1", Direction: model.VolumeTransferDirectionExport, State: model.VolumeTransferStateStreaming},
			}, nil
		},
	}
	enqueuer := &volumeTaskEnqueuerStub{}
	runner := &Runner{volumeService: service, volumeTaskEnqueuer: enqueuer}
	if err := runner.handleVolumeReconcile(t.Context(), asynq.NewTask(tasks.TypeVolumeReconcile, []byte(`{"actorId":"legacy"}`))); err != nil {
		t.Fatalf("handleVolumeReconcile() error = %v", err)
	}
	if len(transferCutoffs) != 2 {
		t.Fatalf("transfer scans = %d, want heartbeat and recovery scans", len(transferCutoffs))
	}
	if delta := transferCutoffs[0].Sub(transferCutoffs[1]); delta < volumeOperationRecoveryAfter-volumeTransferHeartbeatStaleAfter-time.Second || delta > volumeOperationRecoveryAfter-volumeTransferHeartbeatStaleAfter+time.Second {
		t.Fatalf("transfer cutoff separation = %v", delta)
	}
	if len(enqueuer.provision) != 1 || len(enqueuer.imports) != 1 || enqueuer.imports[0].TransferID != "vtx_recover" || len(enqueuer.exports) != 0 {
		t.Fatalf("recovered provision=%#v imports=%#v exports=%#v", enqueuer.provision, enqueuer.imports, enqueuer.exports)
	}
}

func TestFailStaleStreamingImportCleansRuntimeAndPartialClaim(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionImport)
	transfer.State = model.VolumeTransferStateStreaming
	cutoff := time.Now().UTC().Add(-volumeTransferHeartbeatStaleAfter)
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
		volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) {
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
		volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) {
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
		volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) { return provider, nil }}
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

func activeVolumeTestProject() model.Project {
	return model.Project{
		ID:                  "prj_test",
		Identifier:          "test",
		KubernetesNamespace: "project-test",
		DeleteStatus:        "active",
	}
}

var registerStubSQLDriverOnce sync.Once

type stubSQLDriver struct{}
type stubSQLConn struct{}
type stubSQLRows struct{}

func (stubSQLDriver) Open(string) (driver.Conn, error) { return stubSQLConn{}, nil }
func (stubSQLConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}
func (stubSQLConn) Close() error              { return nil }
func (stubSQLConn) Begin() (driver.Tx, error) { return stubSQLConn{}, nil }
func (stubSQLConn) Commit() error             { return nil }
func (stubSQLConn) Rollback() error           { return nil }
func (stubSQLConn) QueryContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return stubSQLRows{}, nil
}
func (stubSQLConn) ExecContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return driver.RowsAffected(1), nil
}
func (stubSQLRows) Columns() []string         { return []string{"id"} }
func (stubSQLRows) Close() error              { return nil }
func (stubSQLRows) Next([]driver.Value) error { return io.EOF }

func openStubWorkerGormDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerStubSQLDriverOnce.Do(func() {
		sql.Register("worker_unit_stub_pg", stubSQLDriver{})
	})
	sqlDB, err := sql.Open("worker_unit_stub_pg", "")
	if err != nil {
		t.Fatalf("open stub sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open stub gorm db: %v", err)
	}
	return db
}

func openVolumeRunnerTestDB(t *testing.T, project model.Project, projectErr error) *gorm.DB {
	t.Helper()
	db := openStubWorkerGormDB(t)
	if err := db.Callback().Query().After("gorm:query").Register("test:volume_runner_seed", func(tx *gorm.DB) {
		switch dest := tx.Statement.Dest.(type) {
		case *model.Project:
			if projectErr != nil {
				tx.Error = projectErr
				return
			}
			tx.Error = nil
			*dest = project
		case *model.RuntimeCluster:
			tx.Error = nil
			*dest = model.RuntimeCluster{ID: "rcl_test", Scope: "global", GatewayNamespace: "luna-gateway"}
		}
	}); err != nil {
		t.Fatalf("register volume runner query callback: %v", err)
	}
	return db
}

func newVolumeRunnerWithStubNamespace(t *testing.T, service volumeWorkerService, claimProvider kubeprovider.ProjectVolumeProvider) *Runner {
	t.Helper()
	return &Runner{
		db:            openVolumeRunnerTestDB(t, activeVolumeTestProject(), nil),
		volumeService: service,
		kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
			return fakeNamespaceManager{}, nil
		},
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return claimProvider, nil },
	}
}

func TestHandleVolumeProvisionCreatesMissingNamespaceAndNetworkPoliciesBeforeClaim(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.PendingOperation = volume.OperationProvision
	namespaceGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	networkPolicyGVR := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	networkPolicyGVK := schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
		ns := action.GetNamespace()
		if _, err := clientset.Tracker().Get(namespaceGVR, "", ns); err != nil {
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, ns)
		}
		listObj, err := clientset.Tracker().List(networkPolicyGVR, networkPolicyGVK, ns)
		if err != nil {
			return true, nil, err
		}
		policies, ok := listObj.(*networkingv1.NetworkPolicyList)
		if !ok || len(policies.Items) == 0 {
			return true, nil, errors.New("workload network policies were not synced before PVC creation")
		}
		return false, nil, nil
	})
	kubeClient := kubeprovider.NewClientForInterface(clientset)

	markedReady := false
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		setLifecycleFn: func(_ context.Context, _, _ string, _ []string, to, code, _ string) (model.ProjectVolume, error) {
			if to != model.ProjectVolumeLifecycleReady || code != "" {
				t.Fatalf("unexpected lifecycle transition to=%q code=%q", to, code)
			}
			markedReady = true
			return projectVolume, nil
		},
	}
	runner := &Runner{
		db:                           openVolumeRunnerTestDB(t, activeVolumeTestProject(), nil),
		volumeService:                service,
		kubernetesManagerFactory:     func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) { return kubeClient, nil },
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return kubeClient, nil },
	}
	task, err := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.handleVolumeProvision(context.Background(), task); err != nil {
		t.Fatalf("handleVolumeProvision() error = %v", err)
	}
	if !markedReady {
		t.Fatal("expected volume lifecycle to transition to ready")
	}
	ns, err := clientset.CoreV1().Namespaces().Get(context.Background(), projectVolume.Namespace, metav1.GetOptions{})
	if err != nil || ns.Labels[kubeprovider.ProjectIDLabel] != projectVolume.ProjectID {
		t.Fatalf("namespace = %#v, err = %v", ns, err)
	}
	claim, err := clientset.CoreV1().PersistentVolumeClaims(projectVolume.Namespace).Get(context.Background(), projectVolume.ClaimName, metav1.GetOptions{})
	if err != nil || claim.Labels[kubeprovider.ProjectVolumeIDLabel] != projectVolume.ID {
		t.Fatalf("claim = %#v, err = %v", claim, err)
	}
}

func TestHandleVolumeImportCreatesMissingNamespaceBeforeClaimAndTransferPod(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.SourceKind = model.ProjectVolumeSourceArchiveImport
	transfer := transferFixture(model.VolumeTransferDirectionImport)
	claimed := transfer
	claimed.ExecutionGeneration = 1
	confirmed := claimed
	now := time.Now().UTC()
	confirmed.JobCreatedAt = &now

	namespaceGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	networkPolicyGVR := schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}
	networkPolicyGVK := schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"}
	clientset := fake.NewSimpleClientset()
	requireInitializedNamespace := func(ns string) error {
		if _, err := clientset.Tracker().Get(namespaceGVR, "", ns); err != nil {
			return apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, ns)
		}
		listObj, err := clientset.Tracker().List(networkPolicyGVR, networkPolicyGVK, ns)
		if err != nil {
			return err
		}
		policies, ok := listObj.(*networkingv1.NetworkPolicyList)
		if !ok || len(policies.Items) == 0 {
			return errors.New("workload network policies were not synced before volume import resources")
		}
		return nil
	}
	clientset.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if err := requireInitializedNamespace(action.GetNamespace()); err != nil {
			return true, nil, err
		}
		return false, nil, nil
	})
	clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if err := requireInitializedNamespace(action.GetNamespace()); err != nil {
			return true, nil, err
		}
		createAction := action.(k8stesting.CreateAction)
		pod := createAction.GetObject().(*corev1.Pod).DeepCopy()
		pod.Status.Phase = corev1.PodRunning
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
		if err := clientset.Tracker().Add(pod); err != nil {
			return true, nil, err
		}
		return true, pod, nil
	})
	kubeClient := kubeprovider.NewClientForInterface(clientset)

	ready := false
	service := &volumeWorkerServiceStub{
		getVolumeFn:   func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		claimFn: func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error) {
			return claimed, nil
		},
		renewFn: func(context.Context, string, string, string, int64, time.Time) (model.VolumeTransfer, error) {
			return claimed, nil
		},
		confirmFn: func(context.Context, string, string, int64) (model.VolumeTransfer, error) { return confirmed, nil },
		readyFn: func(context.Context, string, string, int64) (model.VolumeTransfer, error) {
			ready = true
			result := confirmed
			result.State = model.VolumeTransferStateReady
			return result, nil
		},
	}
	runner := &Runner{
		db:                            openVolumeRunnerTestDB(t, activeVolumeTestProject(), nil),
		volumeService:                 service,
		volumeTransferJobImage:        "worker:test",
		volumeTransferMaxBytes:        1 << 30,
		kubernetesManagerFactory:      func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) { return kubeClient, nil },
		projectVolumeProviderFactory:  func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return kubeClient, nil },
		volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) { return kubeClient, nil },
	}
	task, err := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.handleVolumeImport(context.Background(), task); err != nil || !ready {
		t.Fatalf("handleVolumeImport() error = %v, ready = %t", err, ready)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims(projectVolume.Namespace).Get(context.Background(), projectVolume.ClaimName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected import PVC to exist: %v", err)
	}
}

func TestHandleVolumeProvisionAndImportConvergeWithoutRetryWhenProjectUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		project    model.Project
		projectErr error
		wantCode   string
	}{
		{name: "missing_project", projectErr: gorm.ErrRecordNotFound, wantCode: volume.CodeNotFound},
		{name: "deleting_project", project: model.Project{ID: "prj_test", KubernetesNamespace: "project-test", DeleteStatus: "deleting"}, wantCode: volume.CodeStateConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
			projectVolume.PendingOperation = volume.OperationProvision
			provisionTransitioned := false
			provisionService := &volumeWorkerServiceStub{
				getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
				setLifecycleFn: func(_ context.Context, _, _ string, _ []string, to, code, _ string) (model.ProjectVolume, error) {
					if to != model.ProjectVolumeLifecycleError || code != tc.wantCode {
						t.Fatalf("provision transition to=%q code=%q, want error/%q", to, code, tc.wantCode)
					}
					provisionTransitioned = true
					return projectVolume, nil
				},
			}
			provisionRunner := &Runner{
				db:            openVolumeRunnerTestDB(t, tc.project, tc.projectErr),
				volumeService: provisionService,
				kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
					t.Fatal("unavailable project must not create namespace")
					return nil, nil
				},
			}
			provisionTask, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
			err := provisionRunner.handleVolumeProvision(context.Background(), provisionTask)
			if !errors.Is(err, asynq.SkipRetry) || !provisionTransitioned {
				t.Fatalf("provision err=%v transitioned=%t, want SkipRetry", err, provisionTransitioned)
			}

			transfer := transferFixture(model.VolumeTransferDirectionImport)
			claimed := transfer
			claimed.ExecutionGeneration = 1
			transferFailed := false
			clientset := fake.NewSimpleClientset()
			kubeClient := kubeprovider.NewClientForInterface(clientset)
			importService := &volumeWorkerServiceStub{
				getVolumeFn:   func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
				getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
				claimFn: func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error) {
					return claimed, nil
				},
				renewFn: func(context.Context, string, string, string, int64, time.Time) (model.VolumeTransfer, error) {
					return claimed, nil
				},
				failExecutionFn: func(_ context.Context, _, _, code, _ string) (model.VolumeTransfer, error) {
					if code != tc.wantCode {
						t.Fatalf("transfer failure code = %q, want %q", code, tc.wantCode)
					}
					transferFailed = true
					failed := claimed
					failed.State = model.VolumeTransferStateFailed
					return failed, nil
				},
				cleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
					return claimed, nil
				},
			}
			importRunner := &Runner{
				db:            openVolumeRunnerTestDB(t, tc.project, tc.projectErr),
				volumeService: importService,
				kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) {
					t.Fatal("unavailable project must not create namespace for import")
					return nil, nil
				},
				projectVolumeProviderFactory:  func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return kubeClient, nil },
				volumeTransferProviderFactory: func(context.Context, string) (kubeprovider.VolumeTransferProvider, error) { return kubeClient, nil },
			}
			importTask, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
			err = importRunner.handleVolumeImport(context.Background(), importTask)
			if !errors.Is(err, asynq.SkipRetry) || !transferFailed {
				t.Fatalf("import err=%v failed=%t, want SkipRetry", err, transferFailed)
			}
		})
	}
}

func TestHandleVolumeProvisionTreatsUnmanagedNamespaceConflictAsPermanentOwnershipError(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	projectVolume.PendingOperation = volume.OperationProvision
	clientset := fake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: projectVolume.Namespace},
	})
	kubeClient := kubeprovider.NewClientForInterface(clientset)

	transitioned := false
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		setLifecycleFn: func(_ context.Context, _, _ string, _ []string, to, code, _ string) (model.ProjectVolume, error) {
			if to != model.ProjectVolumeLifecycleError || code != volume.CodeOwnershipConflict {
				t.Fatalf("transition to=%q code=%q, want error/%q", to, code, volume.CodeOwnershipConflict)
			}
			transitioned = true
			return projectVolume, nil
		},
	}
	runner := &Runner{
		db:                           openVolumeRunnerTestDB(t, activeVolumeTestProject(), nil),
		volumeService:                service,
		kubernetesManagerFactory:     func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) { return kubeClient, nil },
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return kubeClient, nil },
	}
	task, _ := tasks.NewVolumeProvisionTask(tasks.VolumeProvisionPayload{ProjectID: projectVolume.ProjectID, VolumeID: projectVolume.ID})
	err := runner.handleVolumeProvision(context.Background(), task)
	if !errors.Is(err, asynq.SkipRetry) || !transitioned {
		t.Fatalf("err = %v, transitioned = %t; want SkipRetry with ownership_conflict", err, transitioned)
	}
}

func TestFailAndCleanupVolumeTransferSkipsKubernetesCleanupWhenCommitConflicts(t *testing.T) {
	projectVolume := transferProjectVolume(model.ProjectVolumeModeFilesystem)
	transfer := transferFixture(model.VolumeTransferDirectionImport)
	service := &volumeWorkerServiceStub{
		failExecutionFn: func(context.Context, string, string, string, string) (model.VolumeTransfer, error) {
			return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "cancelled concurrently"}
		},
	}
	streamProvider := &volumeTransferProviderStub{
		cleanupFn: func(context.Context, string, string) error {
			t.Fatal("Kubernetes transfer cleanup must not run when FailVolumeTransferExecution did not commit")
			return nil
		},
	}
	runner := &Runner{volumeService: service}
	err := runner.failAndCleanupVolumeTransfer(context.Background(), service, streamProvider, transfer, projectVolume, nil, volume.CodeStateConflict, errors.New("aborted"))
	if err == nil || errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("err = %v, want retryable state_conflict error without cleanup side effects", err)
	}
}
