package observability

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestWorkerMetricsExportThroughOTelWithoutPrometheusRegistry(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	workerMetrics := NewWorkerMetrics(nil, "worker").WithQueueResolver(func(string) string { return "build" })
	handler := workerMetrics.Middleware(asynq.HandlerFunc(func(context.Context, *asynq.Task) error { return nil }))
	if err := handler.ProcessTask(context.Background(), asynq.NewTask("build:run", nil)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}

	var resourceMetrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &resourceMetrics); err != nil {
		t.Fatalf("collect OTel metrics: %v", err)
	}
	assertOTelMetricPresent(t, resourceMetrics, "luna_devops_worker_task_completed_total")
	assertOTelMetricPresent(t, resourceMetrics, "luna_devops_worker_task_duration_seconds")
}

func TestAsynqQueueTelemetryExportsCurrentState(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	registration, err := RegisterAsynqQueueTelemetry(fakeQueueInspector{
		"build": &asynq.QueueInfo{Queue: "build", Pending: 3, Retry: 1, Latency: 2 * time.Second},
	}, []string{"build"})
	if err != nil {
		t.Fatalf("register queue telemetry: %v", err)
	}
	t.Cleanup(func() { _ = registration.Unregister() })

	var resourceMetrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &resourceMetrics); err != nil {
		t.Fatalf("collect queue telemetry: %v", err)
	}
	assertOTelMetricPresent(t, resourceMetrics, "luna_devops_asynq_queue_depth")
	assertOTelMetricPresent(t, resourceMetrics, "luna_devops_asynq_queue_latency_seconds")
}

func assertOTelMetricPresent(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, scope := range resourceMetrics.ScopeMetrics {
		for _, current := range scope.Metrics {
			if current.Name == name {
				return
			}
		}
	}
	t.Fatalf("OTel metric %q was not collected", name)
}
