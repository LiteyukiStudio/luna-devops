package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestSetupIsDisabledWithoutExplicitEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://must-not-be-read.invalid:4318")

	runtime, err := Setup(context.Background(), ServiceConfig{ServiceName: "test-service"})
	if err != nil {
		t.Fatalf("setup disabled telemetry: %v", err)
	}
	if runtime.Active() {
		t.Fatal("telemetry unexpectedly active without an endpoint")
	}
}

func TestTelemetrySignalEndpointPreservesCollectorPrefix(t *testing.T) {
	if got := telemetrySignalEndpoint("https://collector.example.com/otel", "v1/traces"); got != "https://collector.example.com/otel/v1/traces" {
		t.Fatalf("telemetrySignalEndpoint() = %q", got)
	}
}

func TestSetupRejectsInvalidExplicitEndpoint(t *testing.T) {
	if _, err := Setup(t.Context(), ServiceConfig{ServiceName: "test-service", Endpoint: "grpc://collector:4317"}); err == nil {
		t.Fatal("Setup accepted a non-HTTP endpoint")
	}
}

func TestSetupIgnoresExporterEnvironmentOutsideSnapshot(t *testing.T) {
	for _, signal := range []string{"TRACES", "METRICS", "LOGS"} {
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT", "http://127.0.0.1:1/must-not-be-read")
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_HEADERS", "environment=must-not-be-read")
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_TIMEOUT", "1")
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_COMPRESSION", "gzip")
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_CERTIFICATE", "/missing/collector-ca.pem")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE", "delta")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_DEFAULT_HISTOGRAM_AGGREGATION", "base2_exponential_bucket_histogram")

	var mu sync.Mutex
	seen := make(map[string]int)
	var violations []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		seen[request.URL.Path]++
		if request.Header.Get("snapshot") != "expected" {
			violations = append(violations, "snapshot header missing")
		}
		if request.Header.Get("environment") != "" {
			violations = append(violations, "environment header was read")
		}
		if request.Header.Get("Content-Encoding") != "" {
			violations = append(violations, "environment compression was read")
		}
		mu.Unlock()
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	previousTracer := otel.GetTracerProvider()
	previousMeter := otel.GetMeterProvider()
	previousLogProvider := logglobal.GetLoggerProvider()
	previousLogger := slog.Default()
	defer func() {
		otel.SetTracerProvider(previousTracer)
		otel.SetMeterProvider(previousMeter)
		logglobal.SetLoggerProvider(previousLogProvider)
		slog.SetDefault(previousLogger)
	}()

	runtime, err := Setup(t.Context(), ServiceConfig{
		ServiceName: "snapshot-test", Endpoint: server.URL + "/collector", Headers: map[string]string{"snapshot": "expected"},
	})
	if err != nil {
		t.Fatalf("Setup() read exporter environment outside the snapshot: %v", err)
	}
	_, span := runtime.tracerProvider.Tracer("snapshot-test").Start(t.Context(), "snapshot")
	span.End()
	counter, err := runtime.meterProvider.Meter("snapshot-test").Int64Counter("snapshot_counter")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(t.Context(), 1)
	var record otellog.Record
	record.SetBody(otellog.StringValue("snapshot"))
	runtime.loggerProvider.Logger("snapshot-test").Emit(t.Context(), record)
	if err := errors.Join(
		runtime.tracerProvider.ForceFlush(t.Context()),
		runtime.meterProvider.ForceFlush(t.Context()),
		runtime.loggerProvider.ForceFlush(t.Context()),
		runtime.Shutdown(t.Context()),
	); err != nil {
		t.Fatalf("export explicit telemetry snapshot: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, signalPath := range []string{"/collector/v1/traces", "/collector/v1/metrics", "/collector/v1/logs"} {
		if seen[signalPath] == 0 {
			t.Errorf("explicit endpoint did not receive %s", signalPath)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("exporter read implicit environment: %v", violations)
	}
}

func TestOperationRecordsRedactedDiagnosticErrorText(t *testing.T) {
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})

	ctx, end := StartOperation(context.Background(), "test", "safe_error")
	_ = ctx
	end(errors.New("dial tcp postgres.internal:5432: connection refused; token=must-not-leak"))
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	span := spans[0]
	foundMessage := false
	for _, event := range span.Events() {
		for _, attr := range event.Attributes {
			value := attr.Value.Emit()
			if strings.Contains(value, "must-not-leak") {
				t.Fatalf("span event leaked raw error text in %s", attr.Key)
			}
			if attr.Key == "error.message" && strings.Contains(value, "postgres.internal:5432") && strings.Contains(value, "[REDACTED]") {
				foundMessage = true
			}
		}
	}
	if !foundMessage {
		t.Fatal("span event omitted redacted diagnostic error chain")
	}
}

func TestMapPropagationRoundTrip(t *testing.T) {
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}))
	otel.SetTextMapPropagator(defaultPropagator())
	carrier := InjectMap(ctx)
	extracted := ExtractMap(context.Background(), carrier)
	if got := trace.SpanContextFromContext(extracted); got.TraceID() != trace.SpanContextFromContext(ctx).TraceID() {
		t.Fatalf("trace id mismatch: got %s", got.TraceID())
	}
}
