package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTaskTelemetryMiddlewareContinuesProducerTrace(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	}()

	producerCtx, producerSpan := provider.Tracer("test").Start(context.Background(), "producer")
	headers := telemetry.InjectMap(producerCtx)
	wantTraceID := producerSpan.SpanContext().TraceID()
	producerSpan.End()

	var gotTraceID trace.TraceID
	handler := taskTelemetryMiddleware(asynq.HandlerFunc(func(ctx context.Context, _ *asynq.Task) error {
		gotTraceID = trace.SpanContextFromContext(ctx).TraceID()
		return nil
	}))
	if err := handler.ProcessTask(context.Background(), asynq.NewTaskWithHeaders(tasks.TypeSyncStatus, []byte("{}"), headers)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if gotTraceID != wantTraceID {
		t.Fatalf("consumer trace ID = %s, want %s", gotTraceID, wantTraceID)
	}
}

func TestAIBillingStageFailurePreservesParentAndRedactedDiagnosticText(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})
	parentCtx, parent := provider.Tracer("worker-test").Start(t.Context(), "billing-task")
	parentID := parent.SpanContext().SpanID()
	const secretMarker = "aibgt-high-cardinality-secret-marker"
	_, err := workerStageValue(parentCtx, "billing.settle_ai_usage", func(context.Context) (int, error) {
		return 0, errors.New("reservation " + secretMarker + " failed; token=must-not-leak")
	})
	parent.End()
	if err == nil {
		t.Fatal("failed AI billing stage returned nil error")
	}
	var stage sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "worker.billing.settle_ai_usage" {
			stage = span
			break
		}
	}
	if stage == nil {
		t.Fatal("AI billing stage span was not recorded")
	}
	if stage.Parent().SpanID() != parentID || stage.Status().Code != codes.Error {
		t.Fatalf("AI billing stage parent/status = %s/%s, want %s/Error", stage.Parent().SpanID(), stage.Status().Code, parentID)
	}
	for _, attr := range stage.Attributes() {
		if strings.Contains(attr.Value.Emit(), secretMarker) {
			t.Fatalf("span attribute %s exposed reservation text", attr.Key)
		}
	}
	foundDiagnostic := false
	for _, event := range stage.Events() {
		for _, attr := range event.Attributes {
			value := attr.Value.Emit()
			if strings.Contains(value, "must-not-leak") {
				t.Fatalf("span event %s exposed credential", attr.Key)
			}
			if attr.Key == "error.message" && strings.Contains(value, secretMarker) && strings.Contains(value, "[REDACTED]") {
				foundDiagnostic = true
			}
		}
	}
	if !foundDiagnostic {
		t.Fatal("span event omitted redacted diagnostic error message")
	}
}

func TestNewRunnerBuildJobOptions(t *testing.T) {
	for _, test := range []struct {
		name    string
		options Options
		timeout int64
		issuer  string
	}{
		{name: "defaults", timeout: 600, issuer: "letsencrypt-http01"},
		{name: "custom", options: Options{DeployRolloutTimeoutSeconds: 120, CertManagerClusterIssuer: "letsencrypt-staging"}, timeout: 120, issuer: "letsencrypt-staging"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := NewRunner(nil, test.options)
			if runner.deployRolloutTimeoutSeconds != test.timeout || runner.certManagerClusterIssuer != test.issuer {
				t.Fatalf("runner options = %d/%q, want %d/%q", runner.deployRolloutTimeoutSeconds, runner.certManagerClusterIssuer, test.timeout, test.issuer)
			}
		})
	}
}

func TestPeriodicTaskSpecsIncludeGitRefresh(t *testing.T) {
	specs, err := periodicTaskSpecs()
	if err != nil {
		t.Fatalf("periodicTaskSpecs returned error: %v", err)
	}
	foundGitRefresh := false
	foundAIBilling := false
	foundRuntimeBilling := false
	foundRetentionRun := false
	foundVolumeReconcile := false
	foundVolumeTransferCleanup := false
	for _, spec := range specs {
		if spec.Task.Type() == tasks.TypeGitAccountRefresh {
			foundGitRefresh = spec.Cron == "@every 5m" && spec.Queue == tasks.QueueLight
		}
		if spec.Task.Type() == tasks.TypeBillingRuntime {
			foundRuntimeBilling = spec.Cron == "@every 1m" && spec.Queue == tasks.QueueLight
		}
		if spec.Task.Type() == tasks.TypeBillingAI {
			foundAIBilling = spec.Cron == "@every 1m" && spec.Queue == tasks.QueueLight
		}
		if spec.Task.Type() == tasks.TypeRetentionRun {
			foundRetentionRun = spec.Cron == "@every 24h" && spec.Queue == tasks.QueueLight
		}
		if spec.Task.Type() == tasks.TypeVolumeReconcile {
			policy := tasks.PolicyForType(spec.Task.Type())
			foundVolumeReconcile = spec.Cron == "@every 5m" && spec.Queue == policy.Queue && spec.Timeout == policy.Timeout &&
				spec.MaxRetry == policy.MaxRetry && spec.Retention == policy.Retention && spec.Unique == policy.Unique
		}
		if spec.Task.Type() == tasks.TypeVolumeTransferCleanup {
			policy := tasks.PolicyForType(spec.Task.Type())
			foundVolumeTransferCleanup = spec.Cron == "@every 15m" && spec.Queue == policy.Queue && spec.Timeout == policy.Timeout &&
				spec.MaxRetry == policy.MaxRetry && spec.Retention == policy.Retention && spec.Unique == policy.Unique
		}
	}
	if !foundGitRefresh || !foundAIBilling || !foundRuntimeBilling || !foundRetentionRun || !foundVolumeReconcile || !foundVolumeTransferCleanup {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestPeriodicTaskOptionsPreserveZeroMaxRetry(t *testing.T) {
	options := periodicTaskOptions(periodicTaskSpec{Queue: tasks.QueueLight, Timeout: time.Minute})
	for _, option := range options {
		if option.Type() != asynq.MaxRetryOpt {
			continue
		}
		if got := option.Value(); got != 0 {
			t.Fatalf("max retry = %v, want 0", got)
		}
		return
	}
	t.Fatal("periodic task options did not include MaxRetry")
}

func TestRetentionHandlerIsRegistered(t *testing.T) {
	runner := NewRunner(nil, Options{})
	called := false
	runner.runAutomaticRetention = func(_ context.Context, _ time.Time) error {
		called = true
		return nil
	}

	mux := asynq.NewServeMux()
	registerTaskHandlers(mux, runner)
	if err := mux.ProcessTask(context.Background(), asynq.NewTask(tasks.TypeRetentionRun, []byte("{}"))); err != nil {
		t.Fatalf("retention handler returned error: %v", err)
	}
	if !called {
		t.Fatal("retention runner was not called")
	}
}

func TestRetentionHandlerReturnsRunnerError(t *testing.T) {
	wantErr := errors.New("retention failed")
	runner := NewRunner(nil, Options{})
	runner.runAutomaticRetention = func(_ context.Context, _ time.Time) error {
		return wantErr
	}

	handler := runner.withTaskContext((*Runner).handleRetentionRun)
	err := handler(context.Background(), asynq.NewTask(tasks.TypeRetentionRun, []byte("{}")))
	if !errors.Is(err, wantErr) {
		t.Fatalf("retention handler error = %v", err)
	}
}

func TestSyncStatusDoesNotRunAutomaticRetention(t *testing.T) {
	runner := NewRunner(nil, Options{})
	runner.runAutomaticRetention = func(_ context.Context, _ time.Time) error {
		return errors.New("retention failed")
	}

	err := runner.handleSyncStatus(context.Background(), asynq.NewTask(tasks.TypeSyncStatus, []byte("{}")))
	if err != nil {
		t.Fatalf("sync status error = %v", err)
	}
}

func TestCompletedHourlyWindowsReturnsOnlyCompleteHours(t *testing.T) {
	now := time.Date(2026, 6, 19, 15, 27, 30, 0, time.FixedZone("UTC+8", 8*3600))
	windows := completedHourlyWindows(now, 2)
	if len(windows) != 2 {
		t.Fatalf("windows = %#v", windows)
	}
	if !windows[0].Start.Equal(time.Date(2026, 6, 19, 5, 0, 0, 0, time.UTC)) || !windows[1].End.Equal(time.Date(2026, 6, 19, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("windows = %#v", windows)
	}
}

func TestRuntimeBillingEffectivePeriodProratesWindowStart(t *testing.T) {
	windowStart := time.Date(2026, 6, 19, 6, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	workloadCreatedAt := windowStart.Add(25 * time.Minute)
	start, end, ok := runtimeBillingEffectivePeriod(windowStart, windowEnd, workloadCreatedAt)
	if !ok || !start.Equal(workloadCreatedAt) || !end.Equal(windowEnd) {
		t.Fatalf("period = %s %s %v", start, end, ok)
	}
}

func TestRuntimeObservationWindowOnlyAcceptsCurrentMinute(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 10, 0, 0, time.UTC)
	start, end, ok := runtimeObservationWindow(now.Add(30*time.Second), now.Add(45*time.Second))
	if !ok || !start.Equal(now) || !end.Equal(now.Add(time.Minute)) {
		t.Fatalf("current observation window = %s %s %v", start, end, ok)
	}
	if _, _, ok := runtimeObservationWindow(now.Add(-time.Minute), now); ok {
		t.Fatal("closed historical minute must not be overwritten by a new observation")
	}
}

func TestApplyRuntimeClusterResourcePolicy(t *testing.T) {
	spec := kubeprovider.ApplicationResourcesSpec{DeploymentTargetID: "dplt_1", CPURequest: "1", MemoryRequest: "1Gi"}
	cluster := model.RuntimeCluster{ID: "clu_1", CPURequestPercent: 10, MemoryRequestPercent: 25, CPULimitPercent: 100, MemoryLimitPercent: 100}
	if err := applyRuntimeClusterResourcePolicy(&spec, cluster); err != nil {
		t.Fatalf("applyRuntimeClusterResourcePolicy() error = %v", err)
	}
	if spec.CPURequest != "100m" || spec.MemoryRequest != "256Mi" || spec.CPULimit != "1" || spec.MemoryLimit != "1Gi" {
		t.Fatalf("resources = %#v", spec)
	}
	cluster.CPURequestPercent, cluster.MemoryRequestPercent, cluster.CPULimitPercent, cluster.MemoryLimitPercent = 0, 0, 0, 0
	if err := applyRuntimeClusterResourcePolicy(&spec, cluster); err != nil {
		t.Fatalf("zero policy error = %v", err)
	}
	if spec.CPURequest != "" || spec.MemoryRequest != "" || spec.CPULimit != "" || spec.MemoryLimit != "" {
		t.Fatalf("zero policy resources = %#v", spec)
	}
}

func TestApplicationResourcesSpecIgnoresLegacyTargetLimitsAndAppliesCustomPolicy(t *testing.T) {
	spec, err := applicationResourcesSpec(
		model.Release{ID: "rel_policy", ImageRef: "example.invalid/app:test"},
		model.Project{ID: "prj_policy"},
		model.Application{ID: "app_policy"},
		model.Environment{ID: "env_policy", Replicas: 1, CPURequest: "2", MemoryRequest: "2Gi"},
		model.DeploymentTarget{
			ID: "dplt_policy", KubernetesName: "dplt-policy", WorkloadType: "Deployment", CPULimit: "9", MemoryLimit: "9Gi",
			ServicePorts: model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}}, 8080),
		},
		nil, nil, "policy-test", 60,
	)
	if err != nil {
		t.Fatalf("applicationResourcesSpec() error = %v", err)
	}
	if spec.CPULimit != "" || spec.MemoryLimit != "" {
		t.Fatalf("legacy target limits leaked into spec: %#v", spec)
	}
	cluster := model.RuntimeCluster{
		ID: "clu_policy", CPURequestPercent: 25, MemoryRequestPercent: 50,
		CPULimitPercent: 75, MemoryLimitPercent: 80,
	}
	if err := applyRuntimeClusterResourcePolicy(&spec, cluster); err != nil {
		t.Fatalf("applyRuntimeClusterResourcePolicy() error = %v", err)
	}
	if spec.CPURequest != "500m" || spec.MemoryRequest != "1Gi" || spec.CPULimit != "1500m" || spec.MemoryLimit != "1717986919" {
		t.Fatalf("custom policy resources = %#v", spec)
	}
}

func TestRecordRuntimeObservationIsMinuteIdempotentAndPersistsMetrics(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_observation_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeObservation{})
		},
	})
	runner := &Runner{db: db}
	observedAt := time.Now().UTC()
	target := model.DeploymentTarget{
		ID: "dplt_observation", ProjectID: "prj_observation", CPURequest: "1", MemoryRequest: "1Gi",
	}
	cluster := model.RuntimeCluster{
		ID: "clu_observation", CPURequestPercent: 10, MemoryRequestPercent: 25,
		CPULimitPercent: 100, MemoryLimitPercent: 100,
	}
	snapshot := kubeprovider.DeploymentSnapshot{
		DesiredReplicas: 2, UpdatedReplicas: 2, ReadyReplicas: 2, AvailableReplicas: 2,
		CreatedAt: observedAt.Add(-time.Hour), ObservedAt: observedAt, Phase: "running",
	}
	metrics := kubeprovider.RuntimeMetricsSnapshot{
		Available: true, PodCount: 2, ContainerCount: 4, CPUUsageMilli: 375,
		MemoryUsageBytes: 640 * 1024 * 1024, UpdatedAt: observedAt,
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := runner.recordRuntimeObservation(t.Context(), target, cluster, snapshot, metrics); err != nil {
			t.Fatalf("recordRuntimeObservation() attempt %d error = %v", attempt+1, err)
		}
	}
	var observations []model.RuntimeObservation
	if err := db.Find(&observations).Error; err != nil {
		t.Fatalf("load runtime observations: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("runtime observation count = %d, want 1", len(observations))
	}
	got := observations[0]
	if !got.MetricsAvailable || got.CPUUsageMilli != 375 || got.MemoryUsageBytes != 640*1024*1024 ||
		got.PodCount != 2 || got.ContainerCount != 4 || got.EffectiveCPURequest != "100m" || got.EffectiveMemoryRequest != "256Mi" {
		t.Fatalf("persisted runtime observation = %#v", got)
	}
}

func TestAggregateRuntimeObservationsUsesPerSampleMaximumAndSkipsGaps(t *testing.T) {
	start := time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)
	window := hourlyWindow{Start: start, End: start.Add(time.Hour)}
	policy := model.RuntimeObservation{CPURequestPercent: 10, MemoryRequestPercent: 25, CPULimitPercent: 100, MemoryLimitPercent: 100}
	first := policy
	first.PeriodStart, first.PeriodEnd, first.WorkloadCreatedAt = start, start.Add(time.Minute), start.Add(30*time.Second)
	first.DesiredReplicas, first.EffectiveCPURequest, first.EffectiveMemoryRequest = 2, "100m", "256Mi"
	first.MetricsAvailable, first.CPUUsageMilli, first.MemoryUsageBytes = true, 500, 128*1024*1024
	second := policy
	second.PeriodStart, second.PeriodEnd, second.WorkloadCreatedAt = start.Add(2*time.Minute), start.Add(3*time.Minute), start
	second.DesiredReplicas, second.EffectiveCPURequest, second.EffectiveMemoryRequest = 2, "100m", "256Mi"
	second.MetricsAvailable = false

	got, ok := aggregateRuntimeObservations(window, []model.RuntimeObservation{second, first})
	if !ok || got.sampleCount != 2 || got.metricsSampleCount != 1 || got.observedSeconds != 90 {
		t.Fatalf("aggregation metadata = %#v, %v", got, ok)
	}
	// CPU: 500m for 30s plus the 200m request floor for 60s.
	if got.cpuBilled.String() != "0.00749999999999999" || got.cpuRequest.String() != "0.005" || got.cpuActual.String() != "0.00416666666666665" {
		t.Fatalf("cpu aggregation = billed %s request %s actual %s", got.cpuBilled, got.cpuRequest, got.cpuActual)
	}
	// The missing minute between observations is intentionally not interpolated.
	if !got.memoryBilled.Equal(got.memoryRequest) {
		t.Fatalf("memory billed %s != request floor %s", got.memoryBilled, got.memoryRequest)
	}
}

func TestAggregateRuntimeObservationsAvoidsOverlapAndDoesNotEstimateDisabledRequest(t *testing.T) {
	start := time.Date(2026, 8, 22, 5, 0, 0, 0, time.UTC)
	observations := []model.RuntimeObservation{
		{PeriodStart: start, PeriodEnd: start.Add(time.Minute), WorkloadCreatedAt: start, DesiredReplicas: 1},
		{PeriodStart: start.Add(30 * time.Second), PeriodEnd: start.Add(90 * time.Second), WorkloadCreatedAt: start, DesiredReplicas: 1},
	}
	got, ok := aggregateRuntimeObservations(hourlyWindow{Start: start, End: start.Add(time.Hour)}, observations)
	if !ok || got.observedSeconds != 90 || !got.cpuBilled.IsZero() || !got.memoryBilled.IsZero() {
		t.Fatalf("aggregation = %#v, %v", got, ok)
	}
}

func TestAggregateRuntimeObservationsTakesMemoryActualMaximumAndSkipsStoppedMinute(t *testing.T) {
	start := time.Date(2026, 8, 22, 6, 0, 0, 0, time.UTC)
	observations := []model.RuntimeObservation{
		{
			PeriodStart: start, PeriodEnd: start.Add(time.Minute), WorkloadCreatedAt: start,
			DesiredReplicas: 1, EffectiveMemoryRequest: "256Mi", MetricsAvailable: true,
			MemoryUsageBytes: 512 * 1024 * 1024,
		},
		{
			PeriodStart: start.Add(time.Minute), PeriodEnd: start.Add(2 * time.Minute), WorkloadCreatedAt: start,
			DesiredReplicas: 0, EffectiveCPURequest: "1", EffectiveMemoryRequest: "1Gi", MetricsAvailable: true,
			CPUUsageMilli: 1000, MemoryUsageBytes: 1024 * 1024 * 1024,
		},
	}
	got, ok := aggregateRuntimeObservations(hourlyWindow{Start: start, End: start.Add(time.Hour)}, observations)
	if !ok || got.sampleCount != 2 || got.metricsSampleCount != 2 || got.observedSeconds != 60 {
		t.Fatalf("aggregation metadata = %#v, %v", got, ok)
	}
	wantMemoryGiBHours := decimal.RequireFromString("0.00833333333333335")
	if !got.memoryBilled.Equal(wantMemoryGiBHours) || !got.cpuBilled.IsZero() {
		t.Fatalf("billed CPU/memory = %s/%s, want 0/%s", got.cpuBilled, got.memoryBilled, wantMemoryGiBHours)
	}
}

func TestStorageBillingEffectivePeriodProratesWindowStart(t *testing.T) {
	windowStart := time.Date(2026, 6, 19, 6, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	targetCreatedAt := windowStart.Add(10 * time.Minute)
	start, end, ok := storageBillingEffectivePeriod(windowStart, windowEnd, targetCreatedAt)
	if !ok || !start.Equal(targetCreatedAt) || !end.Equal(windowEnd) {
		t.Fatalf("period = %s %s %v", start, end, ok)
	}
}

func TestStorageBillingEffectivePeriodDoesNotPrecedeClaimCreation(t *testing.T) {
	windowStart := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	volumeCreatedAt := windowStart.Add(5 * time.Minute)
	claimCreatedAt := windowStart.Add(20 * time.Minute)
	storageCreatedAt := volumeCreatedAt
	if claimCreatedAt.After(storageCreatedAt) {
		storageCreatedAt = claimCreatedAt
	}
	start, end, ok := storageBillingEffectivePeriod(windowStart, windowEnd, storageCreatedAt)
	if !ok || !start.Equal(claimCreatedAt) || !end.Equal(windowEnd) {
		t.Fatalf("period = %s %s %v", start, end, ok)
	}
}

func TestExpiredBuildJobUpdatesClearLease(t *testing.T) {
	finishedAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	updates := expiredBuildJobUpdates(finishedAt)
	if updates["status"] != "lost" || updates["message"] != "lease_expired" || updates["lease_token"] != "" || updates["lease_until"] != nil {
		t.Fatalf("updates = %#v", updates)
	}
	gotFinishedAt, ok := updates["finished_at"].(*time.Time)
	if !ok || !gotFinishedAt.Equal(finishedAt) {
		t.Fatalf("finished_at = %#v", updates["finished_at"])
	}
}

func TestKubernetesNotFoundDetection(t *testing.T) {
	err := apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "blog-dev")
	if !isKubernetesNotFound(err) {
		t.Fatalf("expected kubernetes not found error to be detected")
	}
	if isKubernetesNotFound(errors.New("dial tcp refused")) {
		t.Fatalf("expected network error not to be treated as not found")
	}
}

func TestProjectNamespaceSelection(t *testing.T) {
	for _, test := range []struct {
		name    string
		project model.Project
		resolve func(model.Project) string
		want    string
	}{
		{name: "persisted name", project: model.Project{ID: "prj_payments", Identifier: "payments", KubernetesNamespace: "luna-payments"}, resolve: projectNamespace, want: "luna-payments"},
		{name: "missing persisted name fails closed", project: model.Project{ID: "prj_abcdef1234567890", Identifier: "Demo_App"}, resolve: projectNamespace, want: ""},
		{name: "deployment ignores environment namespace", project: model.Project{ID: "prj_abcdef1234567890", Identifier: "demo", KubernetesNamespace: "luna-demo"}, resolve: func(project model.Project) string {
			return deploymentNamespace(project, model.Environment{Namespace: " Prod_App "})
		}, want: "luna-demo"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := test.resolve(test.project)
			if test.want != "" && got != test.want {
				t.Fatalf("namespace = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEnvironmentClusterLookupUsesEnvironmentClusterID(t *testing.T) {
	query, args := environmentClusterLookup(" rcl_env ")
	if query != "id = ? and type in ?" {
		t.Fatalf("query = %q", query)
	}
	if args[0] != "rcl_env" {
		t.Fatalf("cluster id arg = %#v", args[0])
	}
}

func TestRuntimeClusterKubeconfigErrorExplainsLocalFileRefs(t *testing.T) {
	err := runtimeClusterKubeconfigError(errors.New("invalid configuration: unable to read client-cert /Users/sfkm/.minikube/client.crt"))
	if !strings.Contains(err.Error(), "已内联证书的 kubeconfig") {
		t.Fatalf("error = %q", err)
	}
}

func TestApplicationResourceName(t *testing.T) {
	for _, test := range []struct {
		name   string
		target model.DeploymentTarget
		want   string
	}{
		{name: "persisted name", target: model.DeploymentTarget{ID: "dplt_payments_api_prod", KubernetesName: "luna-api-prod"}, want: "luna-api-prod"},
		{name: "missing persisted name fails closed", target: model.DeploymentTarget{ID: "dplt_abcdef1234567890"}, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := applicationResourceName(test.target); got != test.want {
				t.Fatalf("resource name = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildJobSpecUsesRestrictedServiceAccountAndBuildScope(t *testing.T) {
	spec := buildJobSpec(
		"build-job-1",
		"build-job-1-secret",
		model.Environment{ID: "env_dev"},
		model.BuildRun{BuildCPURequest: "750m", BuildMemoryRequest: "768Mi"},
		builder.Task{ProjectID: "prj_demo", ApplicationID: "app_api", DeploymentTargetID: "dplt_api", BuildRunID: "brn_1", JobID: "bjb_1"},
		"moby/buildkit:v0.24.0-rootless",
		false,
		"buildcache",
		1800,
		3600,
	)

	if spec.Spec.ActiveDeadlineSeconds == nil || *spec.Spec.ActiveDeadlineSeconds != 1800 {
		t.Fatalf("active deadline seconds = %#v", spec.Spec.ActiveDeadlineSeconds)
	}
	pod := spec.Spec.Template
	if pod.Labels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("pod labels = %#v", pod.Labels)
	}
	if pod.Spec.ServiceAccountName != buildJobServiceAccountName {
		t.Fatalf("service account = %q", pod.Spec.ServiceAccountName)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("automount service account token = %#v", pod.Spec.AutomountServiceAccountToken)
	}
	container := pod.Spec.Containers[0]
	if container.Resources.Requests.Cpu().String() != "750m" || container.Resources.Limits.Memory().String() != "768Mi" {
		t.Fatalf("resources = %#v", container.Resources)
	}
	if container.SecurityContext == nil {
		t.Fatal("container security context is nil")
	}
	if container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
		t.Fatalf("container should not be privileged: %#v", container.SecurityContext)
	}
	if container.SecurityContext.AllowPrivilegeEscalation == nil || !*container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("rootless BuildKit requires privilege escalation for newuidmap/newgidmap: %#v", container.SecurityContext)
	}
	if container.SecurityContext.RunAsUser == nil || *container.SecurityContext.RunAsUser != 1000 {
		t.Fatalf("runAsUser = %#v", container.SecurityContext.RunAsUser)
	}
	if container.SecurityContext.RunAsGroup == nil || *container.SecurityContext.RunAsGroup != 1000 {
		t.Fatalf("runAsGroup = %#v", container.SecurityContext.RunAsGroup)
	}
	if container.SecurityContext.SeccompProfile == nil || container.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeUnconfined {
		t.Fatalf("seccomp profile = %#v", container.SecurityContext.SeccompProfile)
	}
	if container.SecurityContext.AppArmorProfile == nil || container.SecurityContext.AppArmorProfile.Type != corev1.AppArmorProfileTypeUnconfined {
		t.Fatalf("appArmor profile = %#v", container.SecurityContext.AppArmorProfile)
	}
	var foundBuildkitFlags bool
	for _, env := range container.Env {
		if env.Name == "BUILDKITD_FLAGS" && strings.Contains(env.Value, "--oci-worker-no-process-sandbox") {
			foundBuildkitFlags = true
		}
	}
	if !foundBuildkitFlags {
		t.Fatalf("BUILDKITD_FLAGS not configured: %#v", container.Env)
	}
}

func TestBuildJobSecretSeparatesExplicitBuildArgs(t *testing.T) {
	secret := buildJobSecret(
		"build-secret",
		builder.Task{
			Build: builder.BuildPayload{
				Env: map[string]string{
					"EMBED_WEB": "false",
					"NODE_ENV":  "production",
				},
				BuildArgs: map[string]string{
					"EMBED_WEB": "true",
					"VERSION":   "${{ github.sha }}",
				},
			},
		},
		false,
		"buildcache",
	)

	if secret.StringData["env-EMBED_WEB"] != "true" {
		t.Fatalf("expected explicit build arg to override env value, got %q", secret.StringData["env-EMBED_WEB"])
	}
	if strings.Contains(secret.StringData["env-BUILD_ENV_KEYS"], "EMBED_WEB") {
		t.Fatalf("BUILD_ENV_KEYS should not include overridden arg: %q", secret.StringData["env-BUILD_ENV_KEYS"])
	}
	if !strings.Contains(secret.StringData["env-BUILD_ENV_KEYS"], "NODE_ENV") {
		t.Fatalf("BUILD_ENV_KEYS should include env variable: %q", secret.StringData["env-BUILD_ENV_KEYS"])
	}
	if !strings.Contains(secret.StringData["env-BUILD_ARG_KEYS"], "EMBED_WEB") || !strings.Contains(secret.StringData["env-BUILD_ARG_KEYS"], "VERSION") {
		t.Fatalf("BUILD_ARG_KEYS missing explicit args: %q", secret.StringData["env-BUILD_ARG_KEYS"])
	}
}

func TestBuildJobUsesRenderedTemplateDockerfile(t *testing.T) {
	task := builder.Task{
		Build: builder.BuildPayload{
			DefinitionMode:     "template",
			DockerfilePath:     "Dockerfile",
			TemplateDockerfile: "FROM scratch\n",
		},
	}
	secret := buildJobSecret("build-secret", task, false, "buildcache")
	if secret.StringData["env-BUILD_DEFINITION_MODE"] != "template" {
		t.Fatalf("BUILD_DEFINITION_MODE = %q", secret.StringData["env-BUILD_DEFINITION_MODE"])
	}
	if secret.StringData["template.Dockerfile"] != "FROM scratch\n" {
		t.Fatalf("template.Dockerfile = %q", secret.StringData["template.Dockerfile"])
	}

	spec := buildJobSpec("build-job", "build-secret", model.Environment{}, model.BuildRun{}, task, "executor", false, "buildcache", 1800, 3600)
	found := false
	for _, volume := range spec.Spec.Template.Spec.Volumes {
		if volume.Name != "executor-files" || volume.Secret == nil {
			continue
		}
		for _, item := range volume.Secret.Items {
			if item.Key == "template.Dockerfile" && item.Path == "template.Dockerfile" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("template.Dockerfile was not projected into the build job")
	}
	executorScript := builder.ExecutorScript()
	if !strings.Contains(executorScript, `dockerfile_dir="/executor"`) || !strings.Contains(executorScript, `dockerfile_name="template.Dockerfile"`) {
		t.Fatal("executor does not select the rendered template Dockerfile")
	}
}

func TestBuildJobSpecCopiesOnlyProjectedExecutorFiles(t *testing.T) {
	spec := buildJobSpec(
		"build-job-1",
		"build-job-1-secret",
		model.Environment{ID: "env_dev"},
		model.BuildRun{},
		builder.Task{
			ProjectID:          "prj_demo",
			ApplicationID:      "app_api",
			DeploymentTargetID: "dplt_api",
			BuildRunID:         "brn_1",
			JobID:              "bjb_1",
			Build: builder.BuildPayload{Hooks: []builder.HookPayload{{
				ID:     "prebuild",
				Script: "echo prebuild",
			}}},
		},
		"moby/buildkit:v0.24.0-rootless",
		false,
		"buildcache",
		1800,
		3600,
	)

	container := spec.Spec.Template.Spec.Containers[0]
	command := strings.Join(container.Command, " ")
	if strings.Contains(command, "cp -R /executor/.") {
		t.Fatalf("executor command should not copy projected volume internals: %s", command)
	}
	if !strings.Contains(command, "cp /executor/run.sh /workspace/run.sh") {
		t.Fatalf("executor command should copy run.sh explicitly: %s", command)
	}

	var scriptModes []int32
	for _, volume := range spec.Spec.Template.Spec.Volumes {
		if volume.Name != "executor-files" || volume.Secret == nil {
			continue
		}
		for _, item := range volume.Secret.Items {
			if strings.HasSuffix(item.Path, ".sh") {
				if item.Mode == nil {
					t.Fatalf("script item %s mode is nil", item.Path)
				}
				scriptModes = append(scriptModes, *item.Mode)
			}
		}
	}
	if len(scriptModes) != 2 {
		t.Fatalf("script modes = %#v", scriptModes)
	}
	for _, mode := range scriptModes {
		if mode != 0o555 {
			t.Fatalf("script mode = %#o, want 0555", mode)
		}
	}
}

func TestEnsureBuildJobServiceAccountDisablesTokenAutomount(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta:                   metav1.ObjectMeta{Name: buildJobServiceAccountName, Namespace: "ns-demo"},
		AutomountServiceAccountToken: boolPtr(true),
	})

	if err := ensureBuildJobServiceAccount(context.Background(), client, "ns-demo"); err != nil {
		t.Fatalf("ensureBuildJobServiceAccount returned error: %v", err)
	}

	serviceAccount, err := client.CoreV1().ServiceAccounts("ns-demo").Get(context.Background(), buildJobServiceAccountName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service account: %v", err)
	}
	if serviceAccount.Labels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("labels = %#v", serviceAccount.Labels)
	}
	if serviceAccount.AutomountServiceAccountToken == nil || *serviceAccount.AutomountServiceAccountToken {
		t.Fatalf("automount service account token = %#v", serviceAccount.AutomountServiceAccountToken)
	}
}

func TestWaitForBuildPodWaitsUntilExecutorLogsAreAvailable(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "build-job-pod",
			Namespace: "ns-demo",
			Labels:    map[string]string{"job-name": "build-job"},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "executor",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
			}},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		pod, err := client.CoreV1().Pods("ns-demo").Get(context.Background(), "build-job-pod", metav1.GetOptions{})
		if err != nil {
			return
		}
		pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}}
		_, _ = client.CoreV1().Pods("ns-demo").UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
	}()

	podName, err := waitForBuildPod(ctx, client, "ns-demo", "build-job")
	if err != nil {
		t.Fatalf("waitForBuildPod returned error: %v", err)
	}
	if podName != "build-job-pod" {
		t.Fatalf("podName = %q", podName)
	}
}

func TestWaitForBuildPodReturnsFatalStartupError(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "build-job-pod",
			Namespace: "ns-demo",
			Labels:    map[string]string{"job-name": "build-job"},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "executor",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull failed"}},
			}},
		},
	})

	_, err := waitForBuildPod(context.Background(), client, "ns-demo", "build-job")
	if err == nil || !strings.Contains(err.Error(), "ImagePullBackOff") {
		t.Fatalf("expected ImagePullBackOff error, got %v", err)
	}
}

func TestBuildKubernetesJobFailureMessageIncludesPodTerminationAndEvent(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "build-job-pod",
				Namespace: "ns-demo",
				Labels:    map[string]string{"job-name": "build-job"},
			},
			Status: corev1.PodStatus{
				Phase:  corev1.PodFailed,
				Reason: "OOMKilled",
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "executor",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						Reason:   "OOMKilled",
						ExitCode: 137,
					}},
				}},
			},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "build-job-pod.1", Namespace: "ns-demo"},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Namespace: "ns-demo",
				Name:      "build-job-pod",
			},
			Type:          corev1.EventTypeWarning,
			Reason:        "BackOff",
			Message:       "Back-off restarting failed container executor",
			LastTimestamp: now,
		},
	)
	runner := NewRunner(nil, Options{})

	message := runner.buildKubernetesJobFailureMessage(context.Background(), client, "ns-demo", "build-job", "kubernetes build job failed")

	for _, expected := range []string{"kubernetes build job failed", "OOMKilled", "exitCode=137", "Back-off restarting failed container executor"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message %q missing %q", message, expected)
		}
	}
}

func TestHTTPRouteSpecTargetsApplicationService(t *testing.T) {
	spec, err := httpRouteSpec(
		model.GatewayRoute{ID: "gwr_ABC_123", Host: "api.example.com", Path: "api", ServicePort: 8080, TLSMode: "http-challenge"},
		model.Project{ID: "prj_demo"},
		model.Application{Identifier: "api"},
		model.Environment{Slug: "dev"},
		model.RuntimeCluster{GatewayName: "luna-gateway", GatewayNamespace: "kube-system", GatewayClassName: "traefik"},
		"project-demo",
		"dplt-backend",
	)
	if err != nil {
		t.Fatalf("httpRouteSpec returned error: %v", err)
	}
	if spec.Name != "luna-gateway-gwr-abc-123" || spec.ServiceName != "dplt-backend" || spec.Path != "api" {
		t.Fatalf("spec = %#v", spec)
	}
	if spec.ParentGatewayName != "luna-gateway" || spec.ParentGatewayNamespace != "kube-system" {
		t.Fatalf("parent gateway = %s/%s", spec.ParentGatewayNamespace, spec.ParentGatewayName)
	}
	if spec.SectionName != "web" {
		t.Fatalf("section name = %q", spec.SectionName)
	}
}

func TestHTTPRouteSpecUsesHTTPSSectionNameWhenGatewayTerminatesTLS(t *testing.T) {
	spec, err := httpRouteSpec(
		model.GatewayRoute{ID: "gwr_1", Host: "api.example.com", ServicePort: 3000},
		model.Project{ID: "prj_demo"},
		model.Application{Identifier: "api"},
		model.Environment{Slug: "dev"},
		model.RuntimeCluster{GatewayPublicScheme: "https", GatewayExternalTLSMode: "gateway", GatewayHTTPSListenerName: "secure-internal"},
		"project-demo",
		"",
	)
	if err != nil {
		t.Fatalf("httpRouteSpec returned error: %v", err)
	}
	if spec.SectionName != "secure-internal" {
		t.Fatalf("section name = %q", spec.SectionName)
	}
}

func TestHTTPRouteSpecUsesHTTPSectionNameWhenTLSTerminatesUpstream(t *testing.T) {
	spec, err := httpRouteSpec(
		model.GatewayRoute{ID: "gwr_1", Host: "api.example.com", ServicePort: 3000},
		model.Project{ID: "prj_demo"},
		model.Application{Identifier: "api"},
		model.Environment{Slug: "dev"},
		model.RuntimeCluster{GatewayPublicScheme: "https", GatewayExternalTLSMode: "upstream", GatewayHTTPListenerName: "internal-web", GatewayHTTPSListenerName: "secure-internal"},
		"project-demo",
		"",
	)
	if err != nil {
		t.Fatalf("httpRouteSpec returned error: %v", err)
	}
	if spec.SectionName != "internal-web" {
		t.Fatalf("section name = %q", spec.SectionName)
	}
}

func TestGatewaySpecIncludesManualTLSSecret(t *testing.T) {
	spec := gatewaySpec(model.RuntimeCluster{
		GatewayExternalTLSMode:    "gateway",
		GatewayTLSSecretName:      "wildcard-apps-tls",
		GatewayTLSSecretNamespace: "certs",
	}, "prj_demo")

	if spec.TLSSecretName != "wildcard-apps-tls" || spec.TLSSecretNamespace != "certs" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestHTTPRouteSpecDefaultsBackendWeight(t *testing.T) {
	spec, err := httpRouteSpec(
		model.GatewayRoute{ID: "gwr_1", Host: "api.example.com", ServicePort: 3000, TLSMode: "http-only"},
		model.Project{ID: "prj_demo"},
		model.Application{Identifier: "api"},
		model.Environment{Slug: "dev"},
		model.RuntimeCluster{},
		"project-demo",
		"",
	)
	if err != nil {
		t.Fatalf("httpRouteSpec returned error: %v", err)
	}
	if spec.BackendWeight != 1 || spec.ServicePort != 3000 {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestHTTPRouteSpecMergesGatewayAdvancedConfig(t *testing.T) {
	spec, err := httpRouteSpec(
		model.GatewayRoute{
			ID:                     "gwr_1",
			Host:                   "api.example.com",
			ServicePort:            3000,
			TLSMode:                "http-only",
			RequestHeaders:         `{"X-App":"route"}`,
			ResponseHeaders:        `{"X-Frame-Options":"DENY"}`,
			URLRewrite:             `{"replacePrefixMatch":"/"}`,
			BackendWeight:          25,
			ParentGatewayName:      "route-gateway",
			ParentGatewayNamespace: "edge-system",
		},
		model.Project{ID: "prj_demo"},
		model.Application{Identifier: "api"},
		model.Environment{Slug: "dev"},
		model.RuntimeCluster{
			GatewayExternalTLSMode:        "upstream",
			GatewayForwardedHeadersMode:   "overwrite",
			GatewayDefaultRequestHeaders:  `{"X-Cluster":"default"}`,
			GatewayDefaultResponseHeaders: `{"X-Platform":"luna"}`,
		},
		"project-demo",
		"",
	)
	if err != nil {
		t.Fatalf("httpRouteSpec returned error: %v", err)
	}
	if spec.RequestHeaders["X-Cluster"] != "default" || spec.RequestHeaders["X-App"] != "route" || spec.RequestHeaders["X-Forwarded-Proto"] != "https" || spec.RequestHeaders["X-Forwarded-Port"] != "443" {
		t.Fatalf("request headers = %#v", spec.RequestHeaders)
	}
	if spec.ResponseHeaders["X-Platform"] != "luna" || spec.ResponseHeaders["X-Frame-Options"] != "DENY" {
		t.Fatalf("response headers = %#v", spec.ResponseHeaders)
	}
	if spec.URLRewrite == "" || spec.BackendWeight != 25 || spec.ParentGatewayName != "route-gateway" || spec.ParentGatewayNamespace != "edge-system" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestGatewayCertificateSpecUsesRouteTLSSecret(t *testing.T) {
	spec := gatewayCertificateSpec(
		model.GatewayRoute{ID: "gwr_1", Host: "api.example.com", TLSMode: "http-challenge"},
		model.Project{ID: "prj_demo"},
		"project-demo",
		"ClusterIssuer",
		"letsencrypt-staging",
	)
	if spec.Name != "luna-gateway-gwr-1" || spec.SecretName != "tls-api-example-com" || spec.ClusterIssuer != "letsencrypt-staging" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestGatewayCertificateSpecUsesRuntimeClusterIssuerConfig(t *testing.T) {
	cluster := model.RuntimeCluster{
		GatewayNamespace:            "kube-system",
		GatewayCertificateNamespace: "certs",
		GatewayCertIssuerKind:       "Issuer",
		GatewayCertIssuerName:       "tenant-issuer",
	}
	spec := gatewayCertificateSpec(
		model.GatewayRoute{ID: "gwr_1", Host: "api.example.com", TLSMode: "http-challenge"},
		model.Project{ID: "prj_demo"},
		gatewayCertificateNamespace(cluster, "project-demo"),
		gatewayCertificateIssuerKind(cluster),
		gatewayCertificateIssuerName(cluster, "letsencrypt-staging"),
	)
	if spec.Namespace != "certs" || spec.IssuerKind != "Issuer" || spec.ClusterIssuer != "tenant-issuer" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestGatewayWildcardCertificateSpecUsesClusterDomain(t *testing.T) {
	spec, ok := gatewayWildcardCertificateSpec(
		model.RuntimeCluster{
			GatewayRootDomain:             "apps.example.com",
			GatewayWildcardCertEnabled:    true,
			GatewayWildcardCertSecretName: "apps-wildcard-tls",
			GatewayCertificateNamespace:   "certs",
			GatewayCertIssuerKind:         "ClusterIssuer",
		},
		model.Project{ID: "prj_demo"},
		"project-demo",
		"letsencrypt-dns01",
	)
	if !ok {
		t.Fatal("wildcard certificate spec was not generated")
	}
	if spec.Namespace != "certs" || spec.SecretName != "apps-wildcard-tls" || spec.Host != "apps.example.com" || len(spec.DNSNames) != 1 || spec.DNSNames[0] != "*.apps.example.com" || spec.ClusterIssuer != "letsencrypt-dns01" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestGatewayDNSStatus(t *testing.T) {
	for _, test := range []struct {
		name     string
		resolver fakeCNameResolver
		want     string
	}{
		{name: "verified CNAME", resolver: fakeCNameResolver{cname: "gateway.example.com."}, want: "verified"},
		{name: "lookup failure", resolver: fakeCNameResolver{err: fmt.Errorf("not found")}, want: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := NewRunner(nil, Options{})
			runner.dnsResolver = test.resolver
			if got := runner.gatewayDNSStatus(context.Background(), model.GatewayRoute{Host: "app.example.com", CNAMETarget: "gateway.example.com"}); got != test.want {
				t.Fatalf("status = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseKeyValueMap(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  map[string]string
	}{
		{name: "JSON object", input: `{"APP_ENV":"prod","REPLICAS":"2"}`, want: map[string]string{"APP_ENV": "prod", "REPLICAS": "2"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseKeyValueMap(test.input)
			if err != nil {
				t.Fatalf("parseKeyValueMap returned error: %v", err)
			}
			for key, want := range test.want {
				if got[key] != want {
					t.Fatalf("values = %#v", got)
				}
			}
		})
	}
	for _, input := range []string{"APP_ENV=prod", `{"REPLICAS":2}`} {
		if _, err := parseKeyValueMap(input); err == nil {
			t.Fatalf("legacy runtime configuration %q was accepted", input)
		}
	}
}

type fakeCNameResolver struct {
	cname string
	err   error
}

func (r fakeCNameResolver) LookupCNAME(context.Context, string) (string, error) {
	return r.cname, r.err
}

type fakeNamespaceManager struct{}

func (fakeNamespaceManager) GetWorkloadSnapshot(context.Context, string, string, string) (kubeprovider.DeploymentSnapshot, error) {
	return kubeprovider.DeploymentSnapshot{}, nil
}

func (fakeNamespaceManager) EnsureNamespace(context.Context, string, map[string]string) error {
	return nil
}

func (fakeNamespaceManager) Ping(context.Context) error {
	return nil
}

func (fakeNamespaceManager) EnsureBuildPolicy(context.Context, networkpolicy.BuildPolicy) error {
	return nil
}

func (fakeNamespaceManager) ApplyApplicationResources(context.Context, kubeprovider.ApplicationResourcesSpec) error {
	return nil
}

func (fakeNamespaceManager) PreflightApplicationResources(context.Context, kubeprovider.ApplicationResourcesSpec) error {
	return nil
}

func (fakeNamespaceManager) ApplyApplicationRuntimeConfig(context.Context, kubeprovider.ApplicationResourcesSpec) error {
	return nil
}

func (fakeNamespaceManager) ApplyGatewayTrafficProbe(context.Context, kubeprovider.GatewayTrafficProbeSpec) error {
	return nil
}

func (fakeNamespaceManager) EnsureGatewayTrafficProbeAccess(context.Context, kubeprovider.GatewayTrafficProbeSpec) error {
	return nil
}

func (fakeNamespaceManager) RunHookJob(context.Context, kubeprovider.HookJobSpec) (kubeprovider.HookJobResult, error) {
	return kubeprovider.HookJobResult{}, nil
}

func (fakeNamespaceManager) GetDeploymentSnapshot(context.Context, string, string) (kubeprovider.DeploymentSnapshot, error) {
	return kubeprovider.DeploymentSnapshot{}, nil
}

func (fakeNamespaceManager) DetectGatewayAPISupport(context.Context) error {
	return nil
}

func (fakeNamespaceManager) EnsureGateway(context.Context, kubeprovider.GatewaySpec) error {
	return nil
}

func (fakeNamespaceManager) ApplyHTTPRoute(context.Context, kubeprovider.HTTPRouteSpec) error {
	return nil
}

func (fakeNamespaceManager) DeleteHTTPRoute(context.Context, string, string) error {
	return nil
}

func (fakeNamespaceManager) GetHTTPRouteStatus(context.Context, string, string) (kubeprovider.HTTPRouteStatusSnapshot, error) {
	return kubeprovider.HTTPRouteStatusSnapshot{}, nil
}

func (fakeNamespaceManager) GetServiceBackendSnapshot(context.Context, string, string, int32) (kubeprovider.ServiceBackendSnapshot, error) {
	return kubeprovider.ServiceBackendSnapshot{ServiceExists: true, PortExists: true, ReadyEndpoints: 1}, nil
}

func (fakeNamespaceManager) ApplyCertificate(context.Context, kubeprovider.CertificateSpec) error {
	return nil
}

func (fakeNamespaceManager) GetCertificateSnapshot(context.Context, string, string) (kubeprovider.CertificateSnapshot, error) {
	return kubeprovider.CertificateSnapshot{}, nil
}

func (fakeNamespaceManager) ListManagedResources(context.Context, kubeprovider.ResourceListOptions) ([]kubeprovider.ResourceSnapshot, error) {
	return nil, nil
}

func (fakeNamespaceManager) DeleteManagedResource(context.Context, string, string, string) error {
	return nil
}

type recordingNamespaceManager struct {
	fakeNamespaceManager
	deletions []string
	policies  []networkpolicy.BuildPolicy
	err       error
}

func (m *recordingNamespaceManager) DeleteManagedResource(_ context.Context, kind string, namespace string, name string) error {
	m.deletions = append(m.deletions, kind+"/"+namespace+"/"+name)
	return m.err
}

func (m *recordingNamespaceManager) EnsureBuildPolicy(_ context.Context, policy networkpolicy.BuildPolicy) error {
	m.policies = append(m.policies, policy)
	return m.err
}

func TestEnsureProjectNamespaceDefaultsToRestrictedBuildEgressPolicy(t *testing.T) {
	manager := &recordingNamespaceManager{}
	runner := NewRunner(nil, Options{})
	runner.kubernetesManagerFactory = func(model.Environment) (kubeprovider.NamespaceManager, error) {
		return manager, nil
	}

	if err := runner.ensureProjectNamespace(context.Background(), "ns-demo", model.Project{ID: "prj_demo"}, model.Environment{}); err != nil {
		t.Fatalf("ensureProjectNamespace returned error: %v", err)
	}
	if len(manager.policies) != 1 {
		t.Fatalf("policies = %#v", manager.policies)
	}
	policy := manager.policies[0]
	if policy.Name != "luna-build-egress" || policy.Namespace != "ns-demo" || policy.PodLabels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("policy = %#v", policy)
	}
	if len(policy.Egress) != 3 {
		t.Fatalf("expected dns and public HTTP(S) egress rules, got %#v", policy.Egress)
	}
	if len(policy.Egress[0].Ports) != 2 || policy.Egress[0].Ports[0].Number != 53 || policy.Egress[0].Ports[1].Number != 53 {
		t.Fatalf("expected DNS egress rule, got %#v", policy.Egress[0])
	}
	for _, rule := range policy.Egress[1:] {
		if len(rule.To) != 1 || len(rule.To[0].Except) == 0 {
			t.Fatalf("expected restricted public egress rule, got %#v", rule)
		}
	}
}

func TestEnsureProjectNamespaceSupportsExplicitPermissiveBuildEgressPolicy(t *testing.T) {
	manager := &recordingNamespaceManager{}
	runner := NewRunner(nil, Options{BuildEgressMode: "permissive"})
	runner.kubernetesManagerFactory = func(model.Environment) (kubeprovider.NamespaceManager, error) {
		return manager, nil
	}

	if err := runner.ensureProjectNamespace(context.Background(), "ns-demo", model.Project{ID: "prj_demo"}, model.Environment{}); err != nil {
		t.Fatalf("ensureProjectNamespace returned error: %v", err)
	}
	if len(manager.policies) != 1 {
		t.Fatalf("policies = %#v", manager.policies)
	}
	policy := manager.policies[0]
	if len(policy.Egress) != 1 || len(policy.Egress[0].To) != 0 || len(policy.Egress[0].Ports) != 0 {
		t.Fatalf("expected permissive egress rule, got %#v", policy.Egress)
	}
}

func TestEnsureProjectNamespaceAppliesRestrictedBuildEgressPolicy(t *testing.T) {
	manager := &recordingNamespaceManager{}
	runner := NewRunner(nil, Options{
		BuildEgressMode:         "restricted",
		BuildPrivateEgressCIDRs: []string{"10.20.0.0/16"},
		BuildPrivateEgressPorts: []int{443, 5000},
		BuildBlockedEgressCIDRs: []string{"169.254.169.254/32", "10.96.0.0/12"},
	})
	runner.kubernetesManagerFactory = func(model.Environment) (kubeprovider.NamespaceManager, error) {
		return manager, nil
	}

	if err := runner.ensureProjectNamespace(context.Background(), "ns-demo", model.Project{ID: "prj_demo"}, model.Environment{}); err != nil {
		t.Fatalf("ensureProjectNamespace returned error: %v", err)
	}
	if len(manager.policies) != 1 {
		t.Fatalf("policies = %#v", manager.policies)
	}
	policy := manager.policies[0]
	if policy.Name != "luna-build-egress" || policy.Namespace != "ns-demo" || policy.PodLabels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("policy = %#v", policy)
	}
	if len(policy.Egress) < 4 {
		t.Fatalf("expected dns, public, and private egress rules, got %#v", policy.Egress)
	}
	privateRule := policy.Egress[len(policy.Egress)-1]
	if privateRule.To[0].CIDR != "10.20.0.0/16" || len(privateRule.Ports) != 2 || privateRule.Ports[0].Number != 443 || privateRule.Ports[1].Number != 5000 {
		t.Fatalf("private egress rule = %#v", privateRule)
	}
}

func TestResourceCleanupCanRunOnlyAllowsDeleting(t *testing.T) {
	if !resourceCleanupCanRun("deleting") {
		t.Fatal("expected deleting resource to be cleanup runnable")
	}
	for _, status := range []string{"", "active", "deleted", "delete_failed"} {
		if resourceCleanupCanRun(status) {
			t.Fatalf("expected status %q to be skipped", status)
		}
	}
}

func TestCleanupProjectNamespacesCoversDistinctClusters(t *testing.T) {
	runner := NewRunner(nil, Options{})
	managers := map[string]*recordingNamespaceManager{}
	runner.kubernetesManagerFactory = func(environment model.Environment) (kubeprovider.NamespaceManager, error) {
		key := projectCleanupEnvironmentKey(environment)
		manager := managers[key]
		if manager == nil {
			manager = &recordingNamespaceManager{}
			managers[key] = manager
		}
		return manager, nil
	}

	project := model.Project{ID: "prj_abcdef1234567890", Identifier: "demo", KubernetesNamespace: "luna-demo"}
	targets := []model.DeploymentTarget{
		{ID: "dplt_dev", ClusterID: "rcl_one"},
		{ID: "dplt_prod", ClusterID: "rcl_two"},
		{ID: "dplt_stage", ClusterID: "rcl_one"},
		{ID: "dplt_default"},
	}

	if err := runner.cleanupProjectNamespacesForDeploymentTargets(context.Background(), project, targets); err != nil {
		t.Fatalf("cleanupProjectNamespacesForDeploymentTargets returned error: %v", err)
	}
	for _, key := range []string{"cluster:rcl_one", "cluster:rcl_two", "default"} {
		manager := managers[key]
		if manager == nil {
			t.Fatalf("manager %q was not used", key)
		}
		if len(manager.deletions) != 1 || manager.deletions[0] != "Namespace//luna-demo" {
			t.Fatalf("manager %q deletions = %#v", key, manager.deletions)
		}
	}
}

func TestCleanupProjectNamespacesWithoutDeploymentTargetsDoesNotRequireCluster(t *testing.T) {
	runner := NewRunner(nil, Options{})
	managerCalls := 0
	runner.kubernetesManagerFactory = func(model.Environment) (kubeprovider.NamespaceManager, error) {
		managerCalls++
		return nil, errors.New("unexpected manager call")
	}

	project := model.Project{ID: "prj_empty", Identifier: "empty"}
	if err := runner.cleanupProjectNamespacesForDeploymentTargets(context.Background(), project, nil); err != nil {
		t.Fatalf("cleanupProjectNamespacesForDeploymentTargets returned error: %v", err)
	}
	if managerCalls != 0 {
		t.Fatalf("manager calls = %d, want 0", managerCalls)
	}
}

func TestDeleteManagedNamespaceIgnoresKubernetesNotFound(t *testing.T) {
	manager := &recordingNamespaceManager{
		err: apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "ns-demo"),
	}

	if err := deleteManagedNamespace(context.Background(), manager, "ns-demo"); err != nil {
		t.Fatalf("deleteManagedNamespace returned error: %v", err)
	}
	if len(manager.deletions) != 1 {
		t.Fatalf("deletions = %#v", manager.deletions)
	}
}

func TestRedisTemplateResolvedPasswordReachesKubernetesSecretWithoutManifestLeak(t *testing.T) {
	const password = "redis-contract-secret-marker"

	template, found, err := appstore.Find("redis")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("redis template not found")
	}
	rendered, err := appstore.Render(template, map[string]string{"password": password})
	if err != nil {
		t.Fatal(err)
	}
	plainEnv, err := json.Marshal(rendered.Env)
	if err != nil {
		t.Fatal(err)
	}
	// Deploy runner resolves secret-id references before applicationResourcesSpec
	// merges this JSON into the provider-only SecretData boundary.
	resolvedSecretRefs, err := json.Marshal(rendered.SecretEnv)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := applicationResourcesSpec(
		model.Release{ID: "rel_redis", ImageRef: template.Image},
		model.Project{ID: "prj_redis", Identifier: "redis-project"},
		model.Application{ID: "app_redis", Identifier: "redis"},
		model.Environment{
			ID: "env_prod", Slug: "prod", Replicas: 1,
			CPURequest: template.DefaultCPU, MemoryRequest: template.DefaultMemory,
		},
		model.DeploymentTarget{
			ID: "dplt_redis", KubernetesName: "redis-prod",
			ContainerCommand: template.ContainerCommand, ContainerArgs: template.ContainerArgs,
			EnvVars: string(plainEnv), SecretRefs: string(resolvedSecretRefs),
			ServicePorts: model.EncodeDeploymentServicePorts(
				[]model.DeploymentServicePort{{Name: "redis", Port: template.ServicePort}}, template.ServicePort,
			),
		},
		nil,
		nil,
		"project-redis",
		120,
	)
	if err != nil {
		t.Fatalf("applicationResourcesSpec returned error: %v", err)
	}
	if got := spec.SecretData["REDIS_PASSWORD"]; got != password {
		t.Fatalf("resolved Redis password = %q, want secret marker", got)
	}
	if _, leaked := spec.ConfigData["REDIS_PASSWORD"]; leaked {
		t.Fatalf("Redis password key leaked into ConfigData: %#v", spec.ConfigData)
	}
	if strings.Contains(spec.ContainerArgs, password) {
		t.Fatalf("Redis password leaked into static container args: %q", spec.ContainerArgs)
	}

	clientset := fake.NewSimpleClientset()
	manager := kubeprovider.NewClientForInterface(clientset)
	if err := manager.ApplyApplicationResources(t.Context(), spec); err != nil {
		t.Fatalf("ApplyApplicationResources returned error: %v", err)
	}

	secret, err := clientset.CoreV1().Secrets(spec.Namespace).Get(t.Context(), spec.Name+"-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Redis secret: %v", err)
	}
	if got := string(secret.Data["REDIS_PASSWORD"]); got != password {
		t.Fatalf("Kubernetes Redis password = %q, want secret marker", got)
	}
	configMap, err := clientset.CoreV1().ConfigMaps(spec.Namespace).Get(t.Context(), spec.Name+"-config", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Redis config map: %v", err)
	}
	if _, leaked := configMap.Data["REDIS_PASSWORD"]; leaked {
		t.Fatalf("Redis password key leaked into ConfigMap: %#v", configMap.Data)
	}

	deployment, err := clientset.AppsV1().Deployments(spec.Namespace).Get(t.Context(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Redis deployment: %v", err)
	}
	container := deployment.Spec.Template.Spec.Containers[0]
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" || container.Command[1] != "-ec" {
		t.Fatalf("Redis container command = %#v", container.Command)
	}
	if len(container.Args) != 1 || container.Args[0] != template.ContainerArgs || !strings.Contains(container.Args[0], `"$REDIS_PASSWORD"`) {
		t.Fatalf("Redis container args = %#v", container.Args)
	}
	for _, environmentVariable := range container.Env {
		if environmentVariable.Name == "REDIS_PASSWORD" || environmentVariable.Value == password {
			t.Fatalf("Redis password leaked into ordinary container env: %#v", container.Env)
		}
	}
	secretEnvFrom := false
	for _, source := range container.EnvFrom {
		if source.SecretRef != nil && source.SecretRef.Name == spec.Name+"-secret" {
			secretEnvFrom = true
		}
	}
	if !secretEnvFrom {
		t.Fatalf("Redis container does not reference its Kubernetes Secret: %#v", container.EnvFrom)
	}
	manifest, err := json.Marshal(deployment)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), password) {
		t.Fatalf("Redis password leaked into Deployment manifest: %s", manifest)
	}
}

func TestApplicationResourcesSpecAppliesDefaults(t *testing.T) {
	spec, err := applicationResourcesSpec(
		model.Release{ImageRef: "registry.example.com/acme/api:v1"},
		model.Project{ID: "prj_demo", Identifier: "demo"},
		model.Application{ID: "app_api", Identifier: "api"},
		model.Environment{ID: "env_dev", Slug: "dev", EnvVars: `{"APP_ENV":"dev"}`, ConfigRefs: `{"LOG_LEVEL":"debug"}`, SecretRefs: `{"TOKEN":"secret"}`},
		model.DeploymentTarget{
			ID:             "dplt_backend",
			KubernetesName: "dplt-backend",
			ServicePorts:   model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}}, 8080),
		},
		nil,
		nil,
		"ns-demo",
		120,
	)
	if err != nil {
		t.Fatalf("applicationResourcesSpec returned error: %v", err)
	}
	if spec.Name != "dplt-backend" || spec.Namespace != "ns-demo" || spec.DeploymentTargetID != "dplt_backend" || spec.ServicePort != 8080 || spec.Replicas != 1 || spec.RolloutTimeoutSeconds != 120 {
		t.Fatalf("spec defaults = %#v", spec)
	}
	if spec.ConfigData["APP_ENV"] != "dev" || spec.ConfigData["LOG_LEVEL"] != "debug" || spec.SecretData["TOKEN"] != "secret" {
		t.Fatalf("spec data = config:%#v secret:%#v", spec.ConfigData, spec.SecretData)
	}
}

func TestApplicationResourcesSpecUsesSecretAsSingleAuthoritativeMode(t *testing.T) {
	spec, err := applicationResourcesSpec(
		model.Release{ImageRef: "registry.example.com/acme/api:v1"},
		model.Project{ID: "prj_demo", Identifier: "demo"},
		model.Application{ID: "app_api", Identifier: "api"},
		model.Environment{ID: "env_dev", Slug: "dev"},
		model.DeploymentTarget{
			ID: "dplt_backend", KubernetesName: "dplt-backend", EnvVars: `{"TOKEN":"must-not-render"}`, SecretRefs: `{"TOKEN":"secret-value"}`,
			ServicePorts: model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}}, 8080),
		},
		nil,
		nil,
		"ns-demo",
		120,
	)
	if err != nil {
		t.Fatalf("applicationResourcesSpec returned error: %v", err)
	}
	if _, leaked := spec.ConfigData["TOKEN"]; leaked {
		t.Fatalf("secret key leaked into ConfigData: %#v", spec.ConfigData)
	}
	if spec.SecretData["TOKEN"] != "secret-value" {
		t.Fatalf("SecretData = %#v, want authoritative TOKEN", spec.SecretData)
	}
}

func TestApplicationResourcesSpecMergesRuntimeConfigFiles(t *testing.T) {
	spec, err := applicationResourcesSpec(
		model.Release{ImageRef: "registry.example.com/acme/api:v1"},
		model.Project{ID: "prj_demo"},
		model.Application{ID: "app_api"},
		model.Environment{ID: "env_dev"},
		model.DeploymentTarget{
			ID: "dplt_backend", KubernetesName: "dplt-backend", ConfigFiles: `[{"path":"/app/config.yaml","content":"port: 3000"}]`,
			ServicePorts: model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: 8080}}, 8080),
		},
		[]model.ProjectRuntimeConfigSet{{ConfigFiles: `[{"path":"/app/config.yaml","content":"port: 8080"},{"path":"/app/base.yaml","content":"enabled: true"}]`}},
		nil,
		"ns-demo",
		120,
	)
	if err != nil {
		t.Fatalf("applicationResourcesSpec returned error: %v", err)
	}
	if len(spec.ConfigFiles) != 2 {
		t.Fatalf("config files = %#v", spec.ConfigFiles)
	}
	filesByPath := map[string]string{}
	for _, file := range spec.ConfigFiles {
		filesByPath[file.Path] = file.Content
	}
	if filesByPath["/app/config.yaml"] != "port: 3000" || filesByPath["/app/base.yaml"] != "enabled: true" {
		t.Fatalf("config files = %#v", spec.ConfigFiles)
	}
}

func TestReleaseFinishUpdatesIncludesTerminalFields(t *testing.T) {
	finishedAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	updates := releaseFinishUpdates("succeeded", "rollout completed", finishedAt)
	if updates["status"] != "succeeded" || updates["message"] != "rollout completed" {
		t.Fatalf("updates = %#v", updates)
	}
	gotFinishedAt, ok := updates["finished_at"].(*time.Time)
	if !ok || !gotFinishedAt.Equal(finishedAt) {
		t.Fatalf("finished_at = %#v", updates["finished_at"])
	}
}

func TestGitAccountDueForWorkerRefresh(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	soon := now.Add(4 * time.Minute)
	later := now.Add(10 * time.Minute)
	if !gitAccountDueForWorkerRefresh(model.GitAccount{Status: "unavailable", RefreshTokenRef: "secret", ExpiresAt: &soon}, now) {
		t.Fatal("expected account expiring soon to be due")
	}
	if gitAccountDueForWorkerRefresh(model.GitAccount{RefreshTokenRef: "secret", ExpiresAt: &later}, now) {
		t.Fatal("expected account outside refresh window to be skipped")
	}
	if !gitAccountDueForWorkerRefresh(model.GitAccount{Status: "degraded", RefreshTokenRef: "secret", ExpiresAt: &soon}, now) {
		t.Fatal("expected response-only observation status not to affect refresh scheduling")
	}
}

func TestGitAccountDueForWorkerRefreshSkipsAfterSuccessfulRefresh(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	refreshedExpiry := now.Add(1 * time.Hour)
	account := model.GitAccount{RefreshTokenRef: "secret", ExpiresAt: &refreshedExpiry}
	if gitAccountDueForWorkerRefresh(account, now) {
		t.Fatal("expected refreshed account to be skipped on replay")
	}
}
