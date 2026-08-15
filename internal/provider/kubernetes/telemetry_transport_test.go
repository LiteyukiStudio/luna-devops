package kubernetes

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type kubernetesRoundTripperFunc func(*http.Request) (*http.Response, error)

func (fn kubernetesRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestKubernetesTransportRedactsTelemetryAndRestoresNetworkTarget(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	var networkURL string
	var networkHost string
	var traceparent string
	base := kubernetesRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		networkURL = request.URL.String()
		networkHost = request.Host
		traceparent = request.Header.Get("traceparent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://cluster-sensitive.example.com/api/v1/namespaces/project-sensitive/persistentvolumeclaims/claim-sensitive?watch=false", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	request.Host = "cluster-sensitive.example.com"
	response, err := instrumentKubernetesHTTPTransport(base).RoundTrip(request)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	_ = response.Body.Close()
	if networkURL != request.URL.String() || networkHost != request.Host {
		t.Fatalf("network target = %q host=%q, want %q host=%q", networkURL, networkHost, request.URL.String(), request.Host)
	}
	if traceparent == "" {
		t.Fatal("instrumented request did not propagate traceparent")
	}
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			value := attr.Value.Emit()
			if strings.Contains(value, "sensitive") || strings.Contains(value, "persistentvolumeclaims") || strings.Contains(value, "watch") {
				t.Fatalf("Kubernetes target leaked in telemetry attribute %s=%q", attr.Key, value)
			}
		}
	}
}

func TestKubernetesTransportPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	base := kubernetesRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cluster.example.com/api/v1/persistentvolumeclaims", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	cancel()
	if _, err := instrumentKubernetesHTTPTransport(base).RoundTrip(request); err != context.Canceled {
		t.Fatalf("RoundTrip() error = %v, want context.Canceled", err)
	}
}
