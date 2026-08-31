package kubeproxy

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTelemetryUsesSingleStableServerSpanWithoutSensitiveAttributes(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(context.Background()) })

	request, err := http.NewRequest(http.MethodGet, "https://gateway.example/kube/v1/bindings/sensitive-binding/api/v1/namespaces/project-a/pods/private-pod?labelSelector=secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer sensitive-token")
	telemetry := NewTelemetry(nil)
	request, boundary := telemetry.StartRequest(request, RequestInfo{Verb: "get", Transport: TransportNormal}, "workload")
	_, child := telemetry.StartInternal(request.Context(), "kubernetes.proxy", trace.SpanKindClient)
	child.End()
	boundary.End(http.StatusOK, "", nil)

	spans := recorder.Ended()
	serverCount := 0
	var serverSpanID trace.SpanID
	var proxyParentID trace.SpanID
	for _, span := range spans {
		if span.SpanKind() == trace.SpanKindServer {
			serverCount++
			if span.Name() != "kube.gateway.request" {
				t.Fatalf("unexpected server span name %q", span.Name())
			}
			serverSpanID = span.SpanContext().SpanID()
		} else if span.Name() == "kubernetes.proxy" {
			proxyParentID = span.Parent().SpanID()
		}
		text := span.Name()
		for _, attribute := range span.Attributes() {
			text += string(attribute.Key) + "=" + attribute.Value.Emit()
		}
		for _, sensitive := range []string{"sensitive-binding", "private-pod", "sensitive-token", "labelSelector"} {
			if strings.Contains(text, sensitive) {
				t.Fatalf("span leaked %q: %s", sensitive, text)
			}
		}
	}
	if serverCount != 1 {
		t.Fatalf("expected one server span, got %d", serverCount)
	}
	if !serverSpanID.IsValid() || proxyParentID != serverSpanID {
		t.Fatalf("proxy span is not a child of the single gateway span: server=%s parent=%s", serverSpanID, proxyParentID)
	}
}
