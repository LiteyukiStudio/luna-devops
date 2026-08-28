package telemetry

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestInstrumentHTTPTransportRedactsTelemetryURLWithoutChangingRequest(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})

	var actualURL string
	transport := InstrumentHTTPTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		actualURL = request.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    request,
		}, nil
	}))
	request, err := http.NewRequest(http.MethodGet, "https://user:password@example.com/hooks/path-secret-marker?token=query-secret-marker#fragment", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if actualURL != "https://user:password@example.com/hooks/path-secret-marker?token=query-secret-marker#fragment" {
		t.Fatalf("network request URL = %q", actualURL)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	for _, attr := range spans[0].Attributes() {
		value := attr.Value.Emit()
		if strings.Contains(value, "path-secret-marker") || strings.Contains(value, "query-secret-marker") || strings.Contains(value, "password") || strings.Contains(value, "user@") {
			t.Fatalf("telemetry attribute %s leaked URL credentials, path, or query: %q", attr.Key, value)
		}
	}
}
