package aimodel

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestParseInputValidationCodes(t *testing.T) {
	tests := []struct {
		name  string
		input WriteInput
		want  error
	}{
		{name: "name", input: WriteInput{}, want: ErrNameRequired},
		{name: "context limit", input: WriteInput{Name: "model", MaxContextTokens: 1, MaxOutputTokens: 256}, want: ErrMaxContextTokensInvalid},
		{name: "output limit", input: WriteInput{Name: "model", MaxContextTokens: 4096, MaxOutputTokens: 4096}, want: ErrMaxOutputTokensInvalid},
		{name: "input price", input: WriteInput{Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536, InputCreditsPerMillion: "-1"}, want: ErrInputPriceInvalid},
		{name: "output price", input: WriteInput{Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536, InputCreditsPerMillion: "0", OutputCreditsPerMillion: "invalid"}, want: ErrOutputPriceInvalid},
		{name: "cached input price", input: WriteInput{Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536, InputCreditsPerMillion: "0", OutputCreditsPerMillion: "0", CachedInputCreditsPerMillion: "-1"}, want: ErrCachedInputPriceInvalid},
		{name: "exponent price", input: WriteInput{Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536, InputCreditsPerMillion: "1e3", OutputCreditsPerMillion: "0", CachedInputCreditsPerMillion: "0"}, want: ErrInputPriceInvalid},
		{name: "price precision", input: WriteInput{Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536, InputCreditsPerMillion: "0.000000001", OutputCreditsPerMillion: "0", CachedInputCreditsPerMillion: "0"}, want: ErrInputPriceInvalid},
		{name: "price numeric overflow", input: WriteInput{Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536, InputCreditsPerMillion: "10000000000000000", OutputCreditsPerMillion: "0", CachedInputCreditsPerMillion: "0"}, want: ErrInputPriceInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseInput(test.input, true)
			if !errors.Is(err, test.want) {
				t.Fatalf("parseInput() error = %v, want %v", err, test.want)
			}
			if ErrorCode(err) == "" {
				t.Fatal("validation error has no stable code")
			}
		})
	}
}

func TestParseInputAcceptsNumeric24Scale8PriceBoundaries(t *testing.T) {
	for _, price := range []string{"0", "0.00000001", "9999999999999999.99999999"} {
		input := WriteInput{
			Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536,
			InputCreditsPerMillion: price, OutputCreditsPerMillion: "0",
			CachedInputCreditsPerMillion: "0",
		}
		if _, err := parseInput(input, true); err != nil {
			t.Fatalf("valid price %q rejected: %v", price, err)
		}
	}
}

func TestUpdateInputAllowsOmittedPrices(t *testing.T) {
	parsed, err := parseInput(WriteInput{Name: "model", MaxContextTokens: 524288, MaxOutputTokens: 65536}, false)
	if err != nil {
		t.Fatalf("parseInput() error = %v", err)
	}
	if parsed.Name != "model" {
		t.Fatalf("name = %q, want model", parsed.Name)
	}
}

func TestCreateFailurePreservesTraceAndDoesNotRecordInput(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(traceCaptureHandler{Handler: slog.NewJSONHandler(&logs, nil)}))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	parentContext, parentSpan := provider.Tracer("aimodel-test").Start(t.Context(), "test.request")
	parentSpanID := parentSpan.SpanContext().SpanID()
	secretMarker := "model-name-must-not-enter-telemetry"
	_, err := NewService(nil).Create(parentContext, writeInputFor(secretMarker, nil))
	if !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("Create() error = %v, want database unavailable", err)
	}
	parentSpan.End()

	var operationSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "ai_model.create" {
			operationSpan = span
			break
		}
	}
	if operationSpan == nil {
		t.Fatal("ai_model.create span was not recorded")
	}
	if operationSpan.Parent().SpanID() != parentSpanID {
		t.Fatalf("ai_model.create parent = %s, want %s", operationSpan.Parent().SpanID(), parentSpanID)
	}
	if operationSpan.Status().Code != codes.Error {
		t.Fatalf("span status = %v, want error", operationSpan.Status().Code)
	}
	for _, attr := range operationSpan.Attributes() {
		if strings.Contains(attr.Value.Emit(), secretMarker) {
			t.Fatalf("span attribute %s leaked model input", attr.Key)
		}
	}
	for _, event := range operationSpan.Events() {
		for _, attr := range event.Attributes {
			if strings.Contains(attr.Value.Emit(), secretMarker) {
				t.Fatalf("span event attribute %s leaked model input", attr.Key)
			}
		}
	}
	logOutput := logs.String()
	for _, expected := range []string{
		`"event.name":"ai.model.create_failed"`,
		`"operation":"create"`,
		`"outcome":"failed"`,
		`"error.code":"ai.model_write_failed"`,
		`"trace_id":"` + operationSpan.SpanContext().TraceID().String() + `"`,
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("failure log does not contain %q: %s", expected, logOutput)
		}
	}
	if strings.Contains(logOutput, secretMarker) {
		t.Fatalf("failure log leaked model input: %s", logOutput)
	}
}

type traceCaptureHandler struct {
	slog.Handler
}

func (h traceCaptureHandler) Handle(ctx context.Context, record slog.Record) error {
	if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
		record.AddAttrs(slog.String("trace_id", spanContext.TraceID().String()))
	}
	return h.Handler.Handle(ctx, record)
}

func (h traceCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceCaptureHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h traceCaptureHandler) WithGroup(name string) slog.Handler {
	return traceCaptureHandler{Handler: h.Handler.WithGroup(name)}
}
