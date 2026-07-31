package telemetry

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestSetupIsDisabledWithoutExplicitEndpoint(t *testing.T) {
	previous, hadPrevious := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	t.Cleanup(func() {
		if hadPrevious {
			_ = os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", previous)
		} else {
			_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		}
	})
	_ = os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	runtime, err := Setup(context.Background(), ServiceConfig{ServiceName: "test-service"})
	if err != nil {
		t.Fatalf("setup disabled telemetry: %v", err)
	}
	if runtime.Active() {
		t.Fatal("telemetry unexpectedly active without an endpoint")
	}
}

func TestOperationDoesNotRecordRawErrorText(t *testing.T) {
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
	end(errors.New("token=must-not-leak"))
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	span := spans[0]
	if strings.Contains(span.Status().Description, "must-not-leak") {
		t.Fatal("span status leaked raw error text")
	}
	for _, event := range span.Events() {
		for _, attr := range event.Attributes {
			if strings.Contains(attr.Value.Emit(), "must-not-leak") {
				t.Fatalf("span event leaked raw error text in %s", attr.Key)
			}
		}
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
