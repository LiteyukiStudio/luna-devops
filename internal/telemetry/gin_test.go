package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestQueryTraceContextMiddlewareMovesPrivateParametersToHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(QueryTraceContextMiddleware())
	router.GET("/events", func(ctx *gin.Context) {
		if got := ctx.GetHeader("traceparent"); got != "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01" {
			t.Errorf("traceparent = %q", got)
		}
		if got := ctx.Query("after"); got != "42" {
			t.Errorf("after = %q", got)
		}
		if ctx.Request.URL.Query().Has("_otel_traceparent") {
			t.Error("private trace parameter reached handler")
		}
		ctx.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/events?after=42&_otel_traceparent=00-0123456789abcdef0123456789abcdef-0123456789abcdef-01", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestHTTPBoundaryDoesNotDuplicateAlreadyLoggedTerminalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	router := gin.New()
	router.Use(GinAccessLogMiddleware())
	router.GET("/failure", func(ctx *gin.Context) {
		LogError(ctx.Request.Context(), "Provider request failed", "provider.request.failed",
			"provider.request", "provider.request.failed", errors.New("dial tcp provider.internal:443: connection refused"))
		MarkHTTPErrorLogged(ctx)
		SetHTTPError(ctx, "provider.request.failed", "dial tcp provider.internal:443: connection refused")
		ctx.Status(http.StatusBadGateway)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil).WithContext(context.Background()))

	if got := strings.Count(logs.String(), `"level":"ERROR"`); got != 1 {
		t.Fatalf("terminal ERROR count = %d, logs = %s", got, logs.String())
	}
	if !strings.Contains(logs.String(), `"error.message":"dial tcp provider.internal:443: connection refused"`) {
		t.Fatalf("terminal diagnostic missing: %s", logs.String())
	}
}

func TestIsHealthCheckPathOnlyMatchesMachineProbes(t *testing.T) {
	for _, path := range []string{"/healthz", "/internal/health/live", "/internal/health/ready"} {
		if !IsHealthCheckPath(path) {
			t.Errorf("expected %q to be a health check path", path)
		}
	}
	for _, path := range []string{"/api/v1/meta", "/internal/v1/provider/test", "/api/v1/registries/reg_1/test"} {
		if IsHealthCheckPath(path) {
			t.Errorf("did not expect %q to be a health check path", path)
		}
	}
}

func TestIsKubeGatewayPathOnlyMatchesDedicatedPrefix(t *testing.T) {
	for _, path := range []string{"/kube", "/kube/", "/kube/v1/bindings/kbd_one/version"} {
		if !IsKubeGatewayPath(path) {
			t.Errorf("expected %q to be a Kubernetes gateway path", path)
		}
	}
	for _, path := range []string{"/kubectl", "/kubernetes", "/api/v1/kube-credentials", "/KUBE/v1"} {
		if IsKubeGatewayPath(path) {
			t.Errorf("did not expect %q to be a Kubernetes gateway path", path)
		}
	}
}

func TestGinKubeGatewayPathSkipsGenericServerSpanAndAccessLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := gin.New()
	router.Use(GinTracingMiddleware("test-api"), GinAccessLogMiddleware())
	router.GET("/kube/test", func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
	router.GET("/api/test", func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
	for _, path := range []string{"/kube/test", "/api/test"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("GET %s status = %d", path, response.Code)
		}
	}

	serverSpans := 0
	for _, span := range recorder.Ended() {
		if span.SpanKind() != trace.SpanKindServer {
			continue
		}
		serverSpans++
		text := span.Name()
		for _, value := range span.Attributes() {
			text += string(value.Key) + "=" + value.Value.Emit()
		}
		if strings.Contains(text, "/kube/") {
			t.Fatalf("generic server span captured Kubernetes path: %s", text)
		}
	}
	if serverSpans != 1 {
		t.Fatalf("generic server span count = %d, want one non-Kubernetes request", serverSpans)
	}
	if got := strings.Count(logs.String(), `"event.name":"http.request.completed"`); got != 1 || strings.Contains(logs.String(), "/kube/") {
		t.Fatalf("generic access logs crossed Kubernetes boundary: count=%d logs=%s", got, logs.String())
	}
}
