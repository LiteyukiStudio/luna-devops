package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/trace"
)

type volumeTransferJobProviderStub struct {
	kubeprovider.VolumeTransferJobProvider
	createFn  func(context.Context, kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error)
	observeFn func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error)
	cancelFn  func(context.Context, string, string) error
	cleanupFn func(context.Context, string, string) error
}

func (stub *volumeTransferJobProviderStub) CreateVolumeTransferJob(ctx context.Context, spec kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error) {
	return stub.createFn(ctx, spec)
}

func (stub *volumeTransferJobProviderStub) ObserveVolumeTransferJob(ctx context.Context, namespace, transferID string) (kubeprovider.VolumeTransferJobObservation, error) {
	return stub.observeFn(ctx, namespace, transferID)
}

func (stub *volumeTransferJobProviderStub) CancelVolumeTransferJob(ctx context.Context, namespace, transferID string) error {
	return stub.cancelFn(ctx, namespace, transferID)
}

func (stub *volumeTransferJobProviderStub) CleanupVolumeTransferJob(ctx context.Context, namespace, transferID string) error {
	return stub.cleanupFn(ctx, namespace, transferID)
}

func TestHandleVolumeImportPropagatesTraceAndWaitsForAuthoritativeSuccess(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	queued := volumeTransferFixture(model.VolumeTransferDirectionImport, model.VolumeTransferStateQueued)
	running := queued
	running.State = model.VolumeTransferStateRunning
	running.ExecutionGeneration = 1
	completionReportedAt := time.Now().UTC()
	reported := running
	reported.TransferredBytes = queued.ExpectedBytes
	reported.CompletionReportedAt = &completionReportedAt

	traceID := trace.TraceID{1, 3, 5, 7}
	spanID := trace.SpanID{2, 4, 6, 8}
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true,
	}))
	transferReads := 0
	preparedHash := ""
	ready := false
	claimCreated := false
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			transferReads++
			if transferReads == 1 {
				return queued, nil
			}
			return reported, nil
		},
		claimTransferFn: func(_ context.Context, _, _, expectedState, leaseOwner string, leaseExpiresAt time.Time) (model.VolumeTransfer, error) {
			if expectedState != model.VolumeTransferStateQueued || leaseOwner == "" || !leaseExpiresAt.After(time.Now()) {
				t.Fatalf("invalid execution claim: state=%q owner=%q expires=%s", expectedState, leaseOwner, leaseExpiresAt)
			}
			claimed := queued
			claimed.ExecutionGeneration = 1
			claimed.CreationLeaseOwner = leaseOwner
			claimed.CreationLeaseExpiresAt = &leaseExpiresAt
			return claimed, nil
		},
		prepareTransferFn: func(_ context.Context, _, _, expectedState, leaseOwner string, generation int64, tokenHash string, expiresAt time.Time) (model.VolumeTransfer, error) {
			if expectedState != model.VolumeTransferStateQueued || leaseOwner == "" || generation != 1 || len(tokenHash) != 64 || !expiresAt.After(time.Now()) {
				t.Fatalf("invalid execution preparation: state=%q owner=%q generation=%d hash=%q expires=%s", expectedState, leaseOwner, generation, tokenHash, expiresAt)
			}
			preparedHash = tokenHash
			prepared := running
			prepared.CreationLeaseOwner = leaseOwner
			return prepared, nil
		},
		confirmJobCreatedFn: func(received context.Context, _, _ string, generation int64) (model.VolumeTransfer, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID || generation != 1 {
				t.Fatalf("Job creation confirmation lost authority: trace=%s generation=%d", trace.SpanContextFromContext(received).TraceID(), generation)
			}
			confirmed := running
			now := time.Now().UTC()
			confirmed.JobCreatedAt = &now
			return confirmed, nil
		},
		markJobSucceededFn: func(received context.Context, _, _ string) (model.VolumeTransfer, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID {
				t.Fatal("job success marker lost the parent trace")
			}
			marked := reported
			markedAt := time.Now().UTC()
			marked.JobSucceededAt = &markedAt
			return marked, nil
		},
		finalizeTransferFn: func(received context.Context, _, _ string) (model.VolumeTransfer, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID {
				t.Fatal("transfer finalization lost the parent trace")
			}
			ready = true
			finalized := reported
			finalized.State = model.VolumeTransferStateSucceeded
			return finalized, nil
		},
		markCleanupFn: func(received context.Context, _, _ string) (model.VolumeTransfer, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID {
				t.Fatal("execution cleanup marker lost the parent trace")
			}
			finalized := reported
			finalized.State = model.VolumeTransferStateSucceeded
			markedAt := time.Now().UTC()
			finalized.ExecutionCleanupCompletedAt = &markedAt
			return finalized, nil
		},
	}
	claimProvider := &projectVolumeProviderStub{
		observeFn: func(context.Context, string, string) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if claimCreated {
				return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
			}
			return kubeprovider.ProjectVolumeClaimObservation{}, kubeprovider.ErrProjectVolumeClaimNotFound
		},
		createFn: func(received context.Context, spec kubeprovider.ProjectVolumeClaimSpec) (kubeprovider.ProjectVolumeClaimObservation, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID || spec.ClaimName != projectVolume.ClaimName {
				t.Fatalf("claim create lost trace or authority: trace=%s spec=%+v", trace.SpanContextFromContext(received).TraceID(), spec)
			}
			claimCreated = true
			return kubeprovider.ProjectVolumeClaimObservation{Exists: true}, nil
		},
		inspectFn: func(context.Context, kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ExistingProjectVolumeClaimInspection, error) {
			return kubeprovider.ExistingProjectVolumeClaimInspection{
				Observation: kubeprovider.ProjectVolumeClaimObservation{Exists: true},
				ManagedBy:   kubeprovider.ManagedByValue, ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
			}, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{
		createFn: func(received context.Context, spec kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error) {
			if trace.SpanContextFromContext(received).TraceID() != traceID || spec.CallbackBaseURL == "" || spec.Image == "" {
				t.Fatalf("job create lost trace/config: trace=%s spec=%+v", trace.SpanContextFromContext(received).TraceID(), spec)
			}
			digest := sha256.Sum256(spec.CallbackToken)
			if hex.EncodeToString(digest[:]) != preparedHash {
				t.Fatal("raw Job token does not match the hash persisted by the domain")
			}
			return kubeprovider.VolumeTransferJobReference{}, nil
		},
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
			return kubeprovider.VolumeTransferJobObservation{State: "succeeded", Reason: "completed"}, nil
		},
		cleanupFn: func(context.Context, string, string) error { return nil },
	}
	runner := &Runner{
		volumeService: service, volumeTransferCallbackURL: "https://api.example.test", volumeTransferJobImage: "worker:test",
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return claimProvider, nil },
		volumeTransferJobFactory:     func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) { return jobProvider, nil },
	}
	task, err := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: queued.ProjectID, VolumeID: queued.ProjectVolumeID, TransferID: queued.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.handleVolumeImport(ctx, task); err != nil {
		t.Fatalf("handleVolumeImport() error = %v", err)
	}
	if !ready {
		t.Fatal("project volume was not promoted after Job, callback, and PVC observations succeeded")
	}
}

func TestConcurrentVolumeTransferAttemptsFenceTokenPreparationAndJobCreation(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	queued := volumeTransferFixture(model.VolumeTransferDirectionExport, model.VolumeTransferStateQueued)
	queued.ConsistencyMode = model.VolumeTransferConsistencyLive
	done := queued
	done.State = model.VolumeTransferStateFailed
	now := time.Now().UTC()
	done.ExecutionCleanupCompletedAt = &now

	var mu sync.Mutex
	initialReads := 0
	initialBarrier := make(chan struct{})
	leaseHeld := false
	leaseOwner := ""
	claimCalls := 0
	prepareCalls := 0
	confirmCalls := 0
	createCalls := 0
	preparedHash := ""
	var secretToken []byte
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			mu.Lock()
			if initialReads < 2 {
				initialReads++
				if initialReads == 2 {
					close(initialBarrier)
				}
				mu.Unlock()
				<-initialBarrier
				return queued, nil
			}
			mu.Unlock()
			return done, nil
		},
		claimTransferFn: func(_ context.Context, _, _, expectedState, owner string, expiresAt time.Time) (model.VolumeTransfer, error) {
			mu.Lock()
			defer mu.Unlock()
			claimCalls++
			if expectedState != model.VolumeTransferStateQueued || owner == "" || !expiresAt.After(time.Now()) {
				t.Fatalf("invalid concurrent claim state=%q owner=%q expires=%s", expectedState, owner, expiresAt)
			}
			if leaseHeld {
				return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "execution lease held"}
			}
			leaseHeld = true
			leaseOwner = owner
			claimed := queued
			claimed.ExecutionGeneration = 1
			claimed.CreationLeaseOwner = owner
			claimed.CreationLeaseExpiresAt = &expiresAt
			return claimed, nil
		},
		prepareTransferFn: func(_ context.Context, _, _, expectedState, owner string, generation int64, tokenHash string, _ time.Time) (model.VolumeTransfer, error) {
			mu.Lock()
			defer mu.Unlock()
			prepareCalls++
			if expectedState != model.VolumeTransferStateQueued || owner != leaseOwner || generation != 1 {
				t.Fatalf("unfenced prepare state=%q owner=%q generation=%d", expectedState, owner, generation)
			}
			preparedHash = tokenHash
			prepared := queued
			prepared.State = model.VolumeTransferStateRunning
			prepared.ExecutionGeneration = generation
			prepared.CreationLeaseOwner = owner
			return prepared, nil
		},
		confirmJobCreatedFn: func(context.Context, string, string, int64) (model.VolumeTransfer, error) {
			mu.Lock()
			defer mu.Unlock()
			confirmCalls++
			confirmed := queued
			confirmed.State = model.VolumeTransferStateRunning
			confirmed.ExecutionGeneration = 1
			confirmed.JobCreatedAt = &now
			return confirmed, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{
		createFn: func(_ context.Context, spec kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error) {
			mu.Lock()
			defer mu.Unlock()
			createCalls++
			secretToken = append([]byte(nil), spec.CallbackToken...)
			return kubeprovider.VolumeTransferJobReference{}, nil
		},
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
			t.Fatal("completed test attempt must not wait on a second Job observation")
			return kubeprovider.VolumeTransferJobObservation{}, nil
		},
		cleanupFn: func(context.Context, string, string) error {
			t.Fatal("pre-cleaned terminal fixture must not run provider cleanup")
			return nil
		},
	}
	runner := &Runner{
		volumeService: service, volumeTransferCallbackURL: "https://api.example.test", volumeTransferJobImage: "worker:test",
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
			return &projectVolumeProviderStub{}, nil
		},
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return jobProvider, nil
		},
	}
	task, _ := tasks.NewVolumeExportTask(tasks.VolumeTransferPayload{ProjectID: queued.ProjectID, VolumeID: queued.ProjectVolumeID, TransferID: queued.ID})
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- runner.handleVolumeExport(context.Background(), task) }()
	}
	firstErr, secondErr := <-results, <-results
	if (firstErr == nil) == (secondErr == nil) {
		t.Fatalf("expected one winner and one retried attempt, errors=(%v, %v)", firstErr, secondErr)
	}
	retryErr := firstErr
	if retryErr == nil {
		retryErr = secondErr
	}
	if retryErr.Error() != volume.CodeClusterUnavailable {
		t.Fatalf("concurrent retry error=%v", retryErr)
	}
	mu.Lock()
	defer mu.Unlock()
	if claimCalls != 2 || prepareCalls != 1 || createCalls != 1 || confirmCalls != 1 {
		t.Fatalf("concurrent execution calls claim=%d prepare=%d create=%d confirm=%d", claimCalls, prepareCalls, createCalls, confirmCalls)
	}
	digest := sha256.Sum256(secretToken)
	if preparedHash == "" || hex.EncodeToString(digest[:]) != preparedHash {
		t.Fatalf("persisted callback hash=%q does not match created Secret token", preparedHash)
	}
}

func TestVolumeTransferExecutionLeaseHeartbeatSurvivesQueuedToRunningTransition(t *testing.T) {
	transfer := volumeTransferFixture(model.VolumeTransferDirectionExport, model.VolumeTransferStateQueued)
	transfer.ExecutionGeneration = 1
	transfer.CreationLeaseOwner = "worker-attempt-heartbeat"
	expiresAt := time.Now().UTC().Add(time.Minute)
	transfer.CreationLeaseExpiresAt = &expiresAt
	renewed := make(chan struct{}, 1)
	service := &volumeWorkerServiceStub{
		renewTransferLeaseFn: func(ctx context.Context, projectID, transferID, owner string, generation int64, nextExpiry time.Time) (model.VolumeTransfer, error) {
			if ctx.Err() != nil || projectID != transfer.ProjectID || transferID != transfer.ID || owner != transfer.CreationLeaseOwner ||
				generation != transfer.ExecutionGeneration || !nextExpiry.After(time.Now()) {
				t.Fatalf("invalid heartbeat project=%q transfer=%q owner=%q generation=%d expiry=%s err=%v",
					projectID, transferID, owner, generation, nextExpiry, ctx.Err())
			}
			select {
			case renewed <- struct{}{}:
			default:
			}
			result := transfer
			result.State = model.VolumeTransferStateRunning
			result.CreationLeaseExpiresAt = &nextExpiry
			return result, nil
		},
	}
	heartbeatCtx, stop := maintainVolumeTransferExecutionLease(context.Background(), service, transfer,
		transfer.CreationLeaseOwner, 5*time.Millisecond)
	select {
	case <-renewed:
	case <-time.After(time.Second):
		t.Fatal("execution lease heartbeat did not renew")
	}
	if err := heartbeatCtx.Err(); err != nil {
		t.Fatalf("queued-to-running renewal cancelled creation context: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop execution lease heartbeat: %v", err)
	}
}

func TestRunningTransferWithActiveCreationLeaseCannotCleanupOrRotate(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionExport, model.VolumeTransferStateRunning)
	transfer.JobCreatedAt = nil
	transfer.ExecutionGeneration = 1
	transfer.CreationLeaseOwner = "active-worker-attempt"
	expiresAt := time.Now().UTC().Add(time.Minute)
	transfer.CreationLeaseExpiresAt = &expiresAt
	service := &volumeWorkerServiceStub{
		getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		claimTransferFn: func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error) {
			return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "execution lease held"}
		},
		prepareTransferFn: func(context.Context, string, string, string, string, int64, string, time.Time) (model.VolumeTransfer, error) {
			t.Fatal("active lease contender must not rotate the callback token")
			return model.VolumeTransfer{}, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
			return kubeprovider.VolumeTransferJobObservation{State: "not_found"}, nil
		},
		cleanupFn: func(context.Context, string, string) error {
			t.Fatal("active lease contender must not remove resources being created")
			return nil
		},
		createFn: func(context.Context, kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error) {
			t.Fatal("active lease contender must not create a second Job")
			return kubeprovider.VolumeTransferJobReference{}, nil
		},
	}
	runner := &Runner{volumeService: service, volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
		return jobProvider, nil
	}}
	task, _ := tasks.NewVolumeExportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeExport(context.Background(), task); err == nil || err.Error() != volume.CodeClusterUnavailable {
		t.Fatalf("active creation lease contender error=%v", err)
	}
}

func TestExistingJobAfterCreateCrashIsConfirmedWithoutReplacement(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionExport, model.VolumeTransferStateRunning)
	transfer.JobCreatedAt = nil
	transfer.ExecutionGeneration = 3
	transfer.CreationLeaseOwner = "crashed-worker-attempt"
	expiresAt := time.Now().UTC().Add(time.Minute)
	transfer.CreationLeaseExpiresAt = &expiresAt
	done := transfer
	done.State = model.VolumeTransferStateFailed
	now := time.Now().UTC()
	done.ExecutionCleanupCompletedAt = &now
	reads := 0
	confirmed := false
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			reads++
			if reads == 1 {
				return transfer, nil
			}
			return done, nil
		},
		confirmJobCreatedFn: func(context.Context, string, string, int64) (model.VolumeTransfer, error) {
			confirmed = true
			result := transfer
			result.JobCreatedAt = &now
			result.CreationLeaseOwner = ""
			result.CreationLeaseExpiresAt = nil
			return result, nil
		},
		claimTransferFn: func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error) {
			t.Fatal("existing Job must be adopted without claiming a replacement generation")
			return model.VolumeTransfer{}, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
			return kubeprovider.VolumeTransferJobObservation{State: "running"}, nil
		},
		createFn: func(context.Context, kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error) {
			t.Fatal("existing Job must not be replaced")
			return kubeprovider.VolumeTransferJobReference{}, nil
		},
	}
	runner := &Runner{volumeService: service, volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
		return jobProvider, nil
	}}
	task, _ := tasks.NewVolumeExportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeExport(context.Background(), task); err != nil || !confirmed {
		t.Fatalf("existing Job recovery error=%v confirmed=%t", err, confirmed)
	}
}

func TestSucceededJobWithoutCompletionReportFailsWithStableCode(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionImport, model.VolumeTransferStateRunning)
	cleaned := false
	failedTransfer := false
	failedVolume := false
	service := &volumeWorkerServiceStub{
		getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		markJobSucceededFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			marked := transfer
			now := time.Now().UTC()
			marked.JobSucceededAt = &now
			return marked, nil
		},
		failTransferFn: func(_ context.Context, _, _ string, code, _ string) (model.VolumeTransfer, error) {
			if code != volume.CodeTransferCompletionMissing {
				t.Fatalf("unexpected transfer failure code=%q", code)
			}
			failedTransfer = true
			failedVolume = true
			failed := transfer
			failed.State = model.VolumeTransferStateFailed
			return failed, nil
		},
		markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			failed := transfer
			failed.State = model.VolumeTransferStateFailed
			now := time.Now().UTC()
			failed.ExecutionCleanupCompletedAt = &now
			return failed, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
			return kubeprovider.VolumeTransferJobObservation{State: "succeeded"}, nil
		},
		cleanupFn: func(context.Context, string, string) error { cleaned = true; return nil },
	}
	runner := &Runner{
		volumeService: service,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return jobProvider, nil
		},
	}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	err := runner.handleVolumeImport(context.Background(), task)
	if !errors.Is(err, asynq.SkipRetry) || !strings.Contains(err.Error(), volume.CodeTransferCompletionMissing) || !failedTransfer || !failedVolume || !cleaned {
		t.Fatalf("missing completion err=%v transferFailed=%t volumeFailed=%t cleaned=%t", err, failedTransfer, failedVolume, cleaned)
	}
}

func TestPersistedJobSuccessRetriesPVCAuthorityWithoutRerunningDataJob(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	now := time.Now().UTC()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionImport, model.VolumeTransferStateRunning)
	transfer.CompletionReportedAt = &now
	transfer.JobSucceededAt = &now
	inspectTransient := true
	finalized := 0
	cleaned := 0
	service := &volumeWorkerServiceStub{
		getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		finalizeTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			finalized++
			result := transfer
			result.State = model.VolumeTransferStateSucceeded
			return result, nil
		},
		markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			result := transfer
			result.State = model.VolumeTransferStateSucceeded
			now := time.Now().UTC()
			result.ExecutionCleanupCompletedAt = &now
			return result, nil
		},
	}
	claimProvider := &projectVolumeProviderStub{inspectFn: func(context.Context, kubeprovider.ExistingProjectVolumeClaimSpec) (kubeprovider.ExistingProjectVolumeClaimInspection, error) {
		if inspectTransient {
			return kubeprovider.ExistingProjectVolumeClaimInspection{}, errors.New("temporary Kubernetes API failure")
		}
		return kubeprovider.ExistingProjectVolumeClaimInspection{
			Observation: kubeprovider.ProjectVolumeClaimObservation{Exists: true}, ManagedBy: kubeprovider.ManagedByValue,
			ProjectID: projectVolume.ProjectID, ProjectVolumeID: projectVolume.ID,
		}, nil
	}}
	jobProvider := &volumeTransferJobProviderStub{
		createFn: func(context.Context, kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error) {
			t.Fatal("persisted success must not create another data Job")
			return kubeprovider.VolumeTransferJobReference{}, nil
		},
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
			t.Fatal("persisted success must survive a completed Job being removed by TTL")
			return kubeprovider.VolumeTransferJobObservation{}, nil
		},
		cleanupFn: func(context.Context, string, string) error { cleaned++; return nil },
	}
	runner := &Runner{
		volumeService: service,
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) {
			return claimProvider, nil
		},
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return jobProvider, nil
		},
	}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeImport(context.Background(), task); err == nil || err.Error() != volume.CodeClusterUnavailable {
		t.Fatalf("transient PVC inspection error=%v", err)
	}
	if finalized != 0 || cleaned != 0 {
		t.Fatalf("transient PVC failure finalized=%d cleaned=%d", finalized, cleaned)
	}

	inspectTransient = false
	if err := runner.handleVolumeImport(context.Background(), task); err != nil {
		t.Fatalf("retry after PVC recovery: %v", err)
	}
	if finalized != 1 || cleaned != 1 {
		t.Fatalf("recovered retry finalized=%d cleaned=%d", finalized, cleaned)
	}
}

func TestFinalizeDatabaseFailureKeepsSucceededJobForRetry(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	now := time.Now().UTC()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionExport, model.VolumeTransferStateRunning)
	transfer.CompletionReportedAt = &now
	transfer.JobSucceededAt = &now
	finalizeTransient := true
	cleaned := 0
	service := &volumeWorkerServiceStub{
		getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		finalizeTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			if finalizeTransient {
				return model.VolumeTransfer{}, errors.New("temporary PostgreSQL failure")
			}
			result := transfer
			result.State = model.VolumeTransferStateSucceeded
			return result, nil
		},
		markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			result := transfer
			result.State = model.VolumeTransferStateSucceeded
			now := time.Now().UTC()
			result.ExecutionCleanupCompletedAt = &now
			return result, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{
		observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
			t.Fatal("persisted Job success must skip Kubernetes observation")
			return kubeprovider.VolumeTransferJobObservation{}, nil
		},
		cleanupFn: func(context.Context, string, string) error { cleaned++; return nil },
	}
	runner := &Runner{volumeService: service, volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
		return jobProvider, nil
	}}
	task, _ := tasks.NewVolumeExportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeExport(context.Background(), task); err == nil || err.Error() != volume.CodeClusterUnavailable || cleaned != 0 {
		t.Fatalf("transient DB error=%v cleaned=%d", err, cleaned)
	}
	finalizeTransient = false
	if err := runner.handleVolumeExport(context.Background(), task); err != nil || cleaned != 1 {
		t.Fatalf("DB recovery error=%v cleaned=%d", err, cleaned)
	}
}

func TestLiveOrUnmountedExportWithoutJobAuthorityNeverRerunsDataJob(t *testing.T) {
	for _, consistency := range []string{model.VolumeTransferConsistencyLive, model.VolumeTransferConsistencyUnmounted} {
		for _, evidence := range []string{"reported_completion", "confirmed_job", "committed_part"} {
			t.Run(consistency+"/"+evidence, func(t *testing.T) {
				projectVolume := volumeTransferProjectVolume()
				now := time.Now().UTC()
				transfer := volumeTransferFixture(model.VolumeTransferDirectionExport, model.VolumeTransferStateRunning)
				transfer.ConsistencyMode = consistency
				transfer.CompletionReportedAt = nil
				transfer.JobCreatedAt = nil
				if evidence == "reported_completion" {
					transfer.CompletionReportedAt = &now
				}
				if evidence == "confirmed_job" {
					transfer.JobCreatedAt = &now
				}
				failed := false
				cleaned := false
				service := &volumeWorkerServiceStub{
					getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
					getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
					listTransferPartsFn: func(context.Context, string, int, int) ([]model.VolumeTransferPart, int64, error) {
						if evidence == "committed_part" {
							return []model.VolumeTransferPart{{TransferID: transfer.ID, PartNumber: 1, State: model.VolumeTransferPartStateCompleted}}, 1, nil
						}
						return nil, 0, nil
					},
					claimTransferFn: func(context.Context, string, string, string, string, time.Time) (model.VolumeTransfer, error) {
						t.Fatal("a reported transfer without Job authority must not claim a replacement execution")
						return model.VolumeTransfer{}, nil
					},
					prepareTransferFn: func(context.Context, string, string, string, string, int64, string, time.Time) (model.VolumeTransfer, error) {
						t.Fatal("a reported transfer without Job authority must not rotate credentials or rerun data movement")
						return model.VolumeTransfer{}, nil
					},
					finalizeTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
						t.Fatal("a completion report alone must not finalize the transfer")
						return model.VolumeTransfer{}, nil
					},
					failTransferFn: func(_ context.Context, _, _ string, code, _ string) (model.VolumeTransfer, error) {
						if code != volume.CodeTransferJobFailed {
							t.Fatalf("unexpected authority failure code=%q", code)
						}
						failed = true
						result := transfer
						result.State = model.VolumeTransferStateFailed
						return result, nil
					},
					markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
						result := transfer
						result.State = model.VolumeTransferStateFailed
						now := time.Now().UTC()
						result.ExecutionCleanupCompletedAt = &now
						return result, nil
					},
				}
				jobProvider := &volumeTransferJobProviderStub{
					createFn: func(context.Context, kubeprovider.VolumeTransferJobSpec) (kubeprovider.VolumeTransferJobReference, error) {
						t.Fatal("a reported transfer without Job authority must not create another Job")
						return kubeprovider.VolumeTransferJobReference{}, nil
					},
					observeFn: func(context.Context, string, string) (kubeprovider.VolumeTransferJobObservation, error) {
						return kubeprovider.VolumeTransferJobObservation{State: "not_found", Reason: "job_not_found"}, nil
					},
					cleanupFn: func(context.Context, string, string) error { cleaned = true; return nil },
				}
				runner := &Runner{volumeService: service, volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
					return jobProvider, nil
				}}
				task, _ := tasks.NewVolumeExportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
				err := runner.handleVolumeExport(context.Background(), task)
				if !errors.Is(err, asynq.SkipRetry) || !strings.Contains(err.Error(), volume.CodeTransferJobFailed) || !failed || !cleaned {
					t.Fatalf("missing Job authority err=%v failed=%t cleaned=%t", err, failed, cleaned)
				}
			})
		}
	}
}

func TestHandleCancelledVolumeImportCleansJobObjectClaimAndAsset(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionImport, model.VolumeTransferStateCancelled)
	cancelledJob := false
	cleanedJob := false
	deletedClaim := false
	deletedObject := false
	softDeletedAsset := false
	service := &volumeWorkerServiceStub{
		getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		claimObjectCleanupFn: func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error) {
			transfer.ObjectOwned = true
			return transfer, nil
		},
		completeObjectFn: func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error) {
			return transfer, nil
		},
		completeCancelledFn: func(context.Context, string, string, string) (model.ProjectVolume, error) {
			softDeletedAsset = true
			return projectVolume, nil
		},
		markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			now := time.Now().UTC()
			transfer.ExecutionCleanupCompletedAt = &now
			return transfer, nil
		},
	}
	claimProvider := &projectVolumeProviderStub{deleteFn: func(context.Context, string, string, string, string) error {
		deletedClaim = true
		return nil
	}}
	jobProvider := &volumeTransferJobProviderStub{
		cancelFn:  func(context.Context, string, string) error { cancelledJob = true; return nil },
		cleanupFn: func(context.Context, string, string) error { cleanedJob = true; return nil },
	}
	store := &volumeStoreStub{
		abortFn:  func(context.Context, string, string) error { return nil },
		deleteFn: func(context.Context, string) error { deletedObject = true; return nil },
	}
	runner := &Runner{
		volumeService: service, volumeTransferStore: store,
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return claimProvider, nil },
		volumeTransferJobFactory:     func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) { return jobProvider, nil },
	}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeImport(context.Background(), task); err != nil {
		t.Fatalf("handleVolumeImport(cancelled) error = %v", err)
	}
	if !cancelledJob || !cleanedJob || !deletedClaim || !deletedObject || !softDeletedAsset {
		t.Fatalf("incomplete cancellation cleanup: cancel=%t job=%t claim=%t object=%t asset=%t", cancelledJob, cleanedJob, deletedClaim, deletedObject, softDeletedAsset)
	}
}

func TestTerminalTransferWaitsForInFlightCreationLeaseBeforeCleanup(t *testing.T) {
	transfer := volumeTransferFixture(model.VolumeTransferDirectionImport, model.VolumeTransferStateCancelled)
	transfer.ExecutionGeneration = 1
	transfer.CreationLeaseOwner = "worker-creating-job"
	expiresAt := time.Now().UTC().Add(time.Minute)
	transfer.CreationLeaseExpiresAt = &expiresAt
	service := &volumeWorkerServiceStub{
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) {
			t.Fatal("terminal cleanup must wait before loading or releasing the ProjectVolume")
			return model.ProjectVolume{}, nil
		},
	}
	runner := &Runner{
		volumeService: service,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			t.Fatal("terminal cleanup must not race the active creation lease")
			return nil, nil
		},
	}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeImport(context.Background(), task); err == nil || err.Error() != volume.CodeClusterUnavailable {
		t.Fatalf("in-flight creation lease terminal error=%v", err)
	}
}

func TestCancelledTransferDoesNotReleaseAssetsWhenJobCleanupFails(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionImport, model.VolumeTransferStateCancelled)
	service := &volumeWorkerServiceStub{
		getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		claimObjectCleanupFn: func(context.Context, string, string, string, time.Time) (model.VolumeTransfer, error) {
			t.Fatal("object must not be marked deleted before Job cleanup succeeds")
			return model.VolumeTransfer{}, nil
		},
		completeCancelledFn: func(context.Context, string, string, string) (model.ProjectVolume, error) {
			t.Fatal("ProjectVolume must not be released before Job cleanup succeeds")
			return model.ProjectVolume{}, nil
		},
	}
	claimProvider := &projectVolumeProviderStub{deleteFn: func(context.Context, string, string, string, string) error {
		t.Fatal("claim must not be deleted before Job cleanup succeeds")
		return nil
	}}
	jobProvider := &volumeTransferJobProviderStub{
		cancelFn:  func(context.Context, string, string) error { return nil },
		cleanupFn: func(context.Context, string, string) error { return errors.New("temporary Kubernetes cleanup failure") },
	}
	store := &volumeStoreStub{deleteFn: func(context.Context, string) error {
		t.Fatal("object must not be deleted before Job cleanup succeeds")
		return nil
	}}
	runner := &Runner{
		volumeService: service, volumeTransferStore: store,
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return claimProvider, nil },
		volumeTransferJobFactory:     func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) { return jobProvider, nil },
	}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeImport(context.Background(), task); err == nil || err.Error() != volume.CodeClusterUnavailable {
		t.Fatalf("cleanup failure error = %v", err)
	}
}

func TestTerminalTransferCleanupRetriesAfterProviderRecoveryAndPersistsMarker(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionExport, model.VolumeTransferStateFailed)
	cleanupTransient := true
	cleanupCalls := 0
	markerCalls := 0
	service := &volumeWorkerServiceStub{
		getFn: func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			return transfer, nil
		},
		markCleanupFn: func(context.Context, string, string) (model.VolumeTransfer, error) {
			markerCalls++
			now := time.Now().UTC()
			transfer.ExecutionCleanupCompletedAt = &now
			return transfer, nil
		},
	}
	jobProvider := &volumeTransferJobProviderStub{cleanupFn: func(context.Context, string, string) error {
		cleanupCalls++
		if cleanupTransient {
			return errors.New("temporary Kubernetes cleanup failure")
		}
		return nil
	}}
	providerFactoryCalls := 0
	runner := &Runner{
		volumeService: service,
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			providerFactoryCalls++
			return jobProvider, nil
		},
	}
	task, _ := tasks.NewVolumeExportTask(tasks.VolumeTransferPayload{
		ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID,
	})
	if err := runner.handleVolumeExport(context.Background(), task); err == nil || err.Error() != volume.CodeClusterUnavailable || markerCalls != 0 {
		t.Fatalf("first cleanup error=%v markers=%d", err, markerCalls)
	}
	cleanupTransient = false
	if err := runner.handleVolumeExport(context.Background(), task); err != nil || markerCalls != 1 || transfer.ExecutionCleanupCompletedAt == nil {
		t.Fatalf("recovered cleanup error=%v markers=%d transfer=%#v", err, markerCalls, transfer)
	}
	if err := runner.handleVolumeExport(context.Background(), task); err != nil || cleanupCalls != 2 || providerFactoryCalls != 2 {
		t.Fatalf("idempotent terminal replay error=%v cleanupCalls=%d factoryCalls=%d", err, cleanupCalls, providerFactoryCalls)
	}
}

func TestHandleVolumeTransferCancellationStopsDownstream(t *testing.T) {
	projectVolume := volumeTransferProjectVolume()
	transfer := volumeTransferFixture(model.VolumeTransferDirectionImport, model.VolumeTransferStateQueued)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := &volumeWorkerServiceStub{
		getFn:         func(context.Context, string, string) (model.ProjectVolume, error) { return projectVolume, nil },
		getTransferFn: func(context.Context, string, string) (model.VolumeTransfer, error) { return transfer, nil },
		claimTransferFn: func(received context.Context, _, _, _, _ string, _ time.Time) (model.VolumeTransfer, error) {
			<-received.Done()
			return model.VolumeTransfer{}, received.Err()
		},
	}
	claimProvider := &projectVolumeProviderStub{observeFn: func(received context.Context, _, _ string) (kubeprovider.ProjectVolumeClaimObservation, error) {
		<-received.Done()
		return kubeprovider.ProjectVolumeClaimObservation{}, received.Err()
	}}
	runner := &Runner{
		volumeService:                service,
		projectVolumeProviderFactory: func(context.Context, string) (kubeprovider.ProjectVolumeProvider, error) { return claimProvider, nil },
		volumeTransferJobFactory: func(context.Context, string) (kubeprovider.VolumeTransferJobProvider, error) {
			return &volumeTransferJobProviderStub{}, nil
		},
	}
	task, _ := tasks.NewVolumeImportTask(tasks.VolumeTransferPayload{ProjectID: transfer.ProjectID, VolumeID: transfer.ProjectVolumeID, TransferID: transfer.ID})
	if err := runner.handleVolumeImport(ctx, task); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled transfer error = %v, want context.Canceled", err)
	}
}

func volumeTransferProjectVolume() model.ProjectVolume {
	return model.ProjectVolume{
		ID: "pvol_test", ProjectID: "prj_test", ClusterID: "rclu_test", Namespace: "project-test",
		ClaimName: "luna-pvol-test", OwnershipMode: model.ProjectVolumeOwnershipManaged,
		SourceKind: model.ProjectVolumeSourceArchiveImport, LifecycleState: model.ProjectVolumeLifecycleProvisioning,
		CapacityRequest: "1Gi", CapacityBytes: 1024 * 1024 * 1024, StorageClassName: "standard",
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
	}
}

func volumeTransferFixture(direction, state string) model.VolumeTransfer {
	transfer := model.VolumeTransfer{
		ID: "vtx_test", ProjectID: "prj_test", ProjectVolumeID: "pvol_test", Direction: direction,
		Format: model.VolumeTransferFormatTarGZ, ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		State: state, ObjectKey: "transfers/internal-id", ObjectOwned: true, MultipartUploadID: "upload-id",
		ExpectedBytes: 1024, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if state == model.VolumeTransferStateRunning {
		now := time.Now().UTC()
		transfer.JobCreatedAt = &now
	}
	return transfer
}
