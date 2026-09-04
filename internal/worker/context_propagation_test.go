package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newDryRunWorkerTestRunner(t *testing.T, options Options) *Runner {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=worker_test dbname=worker_test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run worker database: %v", err)
	}
	return newRunner(db, options)
}

func TestDeploymentDatabaseOperationsPreserveTraceAndCancellationContext(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=context_test dbname=context_test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	var mu sync.Mutex
	var observed []context.Context
	capture := func(tx *gorm.DB) {
		mu.Lock()
		observed = append(observed, tx.Statement.Context)
		mu.Unlock()
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:capture_query_context", capture); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:capture_update_context", capture); err != nil {
		t.Fatalf("register update callback: %v", err)
	}

	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	parent := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
		Remote:  true,
	}))
	ctx, cancel := context.WithCancel(parent)
	cancel()

	runner := &Runner{db: db}
	_, _ = runner.releaseDeploymentTarget(ctx, model.Release{
		ID:                 "rel_context",
		ProjectID:          "prj_context",
		ApplicationID:      "app_context",
		DeploymentTargetID: "dt_context",
	})
	_ = runner.finishDeployRelease(ctx, model.Release{ID: "rel_context"}, "failed", "cancelled")
	_ = runner.markApplicationDeleteFailed(ctx, "app_context", context.Canceled)

	mu.Lock()
	contexts := append([]context.Context(nil), observed...)
	mu.Unlock()
	if len(contexts) < 3 {
		t.Fatalf("observed database contexts = %d, want at least 3", len(contexts))
	}
	for index, observedCtx := range contexts {
		if observedCtx.Err() != context.Canceled {
			t.Fatalf("database context %d cancellation = %v, want context.Canceled", index, observedCtx.Err())
		}
		if got := trace.SpanContextFromContext(observedCtx).TraceID(); got != traceID {
			t.Fatalf("database context %d trace ID = %s, want %s", index, got, traceID)
		}
	}
}

func TestAIBillingDatabaseOperationsPreserveStageTraceContext(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=context_test dbname=context_test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	var observed context.Context
	if err := db.Callback().Row().Before("gorm:row").Register("test:capture_ai_billing_context", func(tx *gorm.DB) {
		observed = tx.Statement.Context
	}); err != nil {
		t.Fatalf("register row callback: %v", err)
	}
	traceID := trace.TraceID{9, 8, 7, 6, 5, 4, 3, 2, 1, 9, 8, 7, 6, 5, 4, 3}
	parent := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: trace.SpanID{9, 8, 7, 6, 5, 4, 3, 2}, Remote: true,
	}))
	if err := (&Runner{db: db}).handleBillingAI(parent, nil); err == nil {
		t.Fatal("handle AI billing returned nil error in unsupported dry-run scan")
	}
	if observed == nil {
		t.Fatal("AI billing did not execute a database operation")
	}
	if got := trace.SpanContextFromContext(observed).TraceID(); got != traceID {
		t.Fatalf("AI billing database trace ID = %s, want %s", got, traceID)
	}
}

func TestRuntimeObservationWritePreservesTraceContext(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 port=1 user=context_test dbname=context_test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	var observed context.Context
	if err := db.Callback().Create().Before("gorm:create").Register("test:capture_runtime_observation_context", func(tx *gorm.DB) {
		observed = tx.Statement.Context
	}); err != nil {
		t.Fatalf("register create callback: %v", err)
	}
	traceID := trace.TraceID{2, 4, 6, 8, 10, 12, 14, 16, 1, 3, 5, 7, 9, 11, 13, 15}
	parent := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: trace.SpanID{2, 4, 6, 8, 1, 3, 5, 7}, Remote: true,
	}))
	now := time.Now().UTC()
	err = (&Runner{db: db}).recordRuntimeObservation(
		parent,
		model.DeploymentTarget{ID: "dplt_context", ProjectID: "prj_context", CPURequest: "1", MemoryRequest: "1Gi"},
		model.RuntimeCluster{ID: "clu_context", CPURequestPercent: 10, MemoryRequestPercent: 25, CPULimitPercent: 100, MemoryLimitPercent: 100},
		kubeprovider.DeploymentSnapshot{DesiredReplicas: 1, CreatedAt: now.Add(-time.Hour), ObservedAt: now, Phase: "running"},
		kubeprovider.RuntimeMetricsSnapshot{Available: true, PodCount: 1, ContainerCount: 1, CPUUsageMilli: 25, MemoryUsageBytes: 64 * 1024 * 1024},
	)
	if err != nil {
		t.Fatalf("record runtime observation: %v", err)
	}
	if observed == nil {
		t.Fatal("runtime observation did not execute a database create")
	}
	if got := trace.SpanContextFromContext(observed).TraceID(); got != traceID {
		t.Fatalf("runtime observation database trace ID = %s, want %s", got, traceID)
	}
}
