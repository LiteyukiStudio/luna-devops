package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMetricsConfigActiveRequiresEnabledAndAddr(t *testing.T) {
	tests := []struct {
		name   string
		config MetricsConfig
		want   bool
	}{
		{name: "disabled", config: MetricsConfig{Enabled: false, Addr: ":19090"}, want: false},
		{name: "missing addr", config: MetricsConfig{Enabled: true}, want: false},
		{name: "active", config: MetricsConfig{Enabled: true, Addr: ":19090"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.Active(); got != tt.want {
				t.Fatalf("Active() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestMetricsConfigWithDefaultAddr(t *testing.T) {
	config := MetricsConfig{Enabled: true, Path: "/metrics", Service: "api"}.WithDefaultAddr(":9090")
	if !config.Active() {
		t.Fatalf("config should be active after applying default addr")
	}
	if config.Addr != ":9090" {
		t.Fatalf("Addr = %q, want :9090", config.Addr)
	}
}

func TestStartMetricsServerDisabledDoesNotListen(t *testing.T) {
	server, err := StartMetricsServer(MetricsConfig{Enabled: false, Addr: "127.0.0.1:0", Path: "/metrics", Service: "test"}, NewRegistry("test"))
	if err != nil {
		t.Fatalf("StartMetricsServer returned error: %v", err)
	}
	if server != nil {
		t.Fatalf("StartMetricsServer returned server when disabled")
	}
}

func TestHTTPMetricsMiddlewareExportsRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := NewRegistry("api")
	router := gin.New()
	router.Use(NewHTTPMetrics(registry, "api").GinMiddleware())
	router.GET("/healthz", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "ok")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}

	metricsRecorder := httptest.NewRecorder()
	NewMetricsHandler(registry).ServeHTTP(metricsRecorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRecorder.Body.String()
	if !strings.Contains(body, `luna_devops_http_requests_total{method="GET",route="/healthz",service="api",status_code="200"} 1`) {
		t.Fatalf("metrics body did not contain request counter:\n%s", body)
	}
}

func TestStartMetricsServerServesMetrics(t *testing.T) {
	registry := NewRegistry("api")
	server, err := StartMetricsServer(MetricsConfig{Enabled: true, Addr: "127.0.0.1:0", Path: "/metrics", Service: "api"}, registry)
	if err != nil {
		t.Fatalf("StartMetricsServer returned error: %v", err)
	}
	defer ShutdownMetricsServer(context.Background(), server)

	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + server.Addr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics returned error: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "luna_devops_up") {
		t.Fatalf("metrics body did not contain luna_devops_up:\n%s", body)
	}
}
