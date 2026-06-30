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
	"github.com/hibiken/asynq"
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
	if !strings.Contains(body, `liteyuki_http_requests_total{method="GET",route="/healthz",service="api",status_code="200"} 1`) {
		t.Fatalf("metrics body did not contain request counter:\n%s", body)
	}
}

func TestWorkerMetricsMiddlewareExportsTask(t *testing.T) {
	registry := NewRegistry("worker")
	metrics := NewWorkerMetrics(registry, "worker").WithQueueResolver(func(string) string { return "build" })
	handler := metrics.Middleware(asynq.HandlerFunc(func(context.Context, *asynq.Task) error {
		return nil
	}))

	if err := handler.ProcessTask(context.Background(), asynq.NewTask("build:run", nil)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}

	metricsRecorder := httptest.NewRecorder()
	NewMetricsHandler(registry).ServeHTTP(metricsRecorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRecorder.Body.String()
	if !strings.Contains(body, `liteyuki_worker_task_completed_total{queue="build",result="succeeded",service="worker",task_type="build:run"} 1`) {
		t.Fatalf("metrics body did not contain worker counter:\n%s", body)
	}
}

func TestWorkerMetricsExportsBusinessMetrics(t *testing.T) {
	registry := NewRegistry("worker")
	metrics := NewWorkerMetrics(registry, "worker")
	startedAt := time.Now().Add(-2 * time.Minute)
	finishedAt := time.Now()

	metrics.RecordBuildRun(BusinessRunMetric{
		Status:     "succeeded",
		Type:       "manual",
		StartedAt:  &startedAt,
		FinishedAt: &finishedAt,
		CreatedAt:  startedAt.Add(-time.Minute),
	})
	metrics.RecordRelease(BusinessRunMetric{
		Status:     "failed",
		Type:       "deploy",
		StartedAt:  &startedAt,
		FinishedAt: &finishedAt,
		CreatedAt:  startedAt,
	})
	metrics.RecordGatewaySync("apply", "succeeded", 150*time.Millisecond)
	metrics.SetGatewayRoutes([]GatewayRouteMetric{{
		Status:            "active",
		TLSMode:           "http-only",
		DNSStatus:         "verified",
		CertificateStatus: "disabled",
	}})
	metrics.SetDeploymentRuntime(DeploymentRuntimeMetric{
		DeploymentTargetID: "dplt-1",
		EnvironmentID:      "env-1",
		DesiredReplicas:    3,
		ReadyReplicas:      2,
		AvailableReplicas:  2,
		UpdatedReplicas:    2,
	})

	metricsRecorder := httptest.NewRecorder()
	NewMetricsHandler(registry).ServeHTTP(metricsRecorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRecorder.Body.String()
	for _, expected := range []string{
		`liteyuki_build_runs_total{service="worker",status="succeeded",trigger_type="manual"} 1`,
		`liteyuki_releases_total{service="worker",status="failed",type="deploy"} 1`,
		`liteyuki_gateway_sync_total{operation="apply",result="succeeded",service="worker"} 1`,
		`liteyuki_gateway_routes_total{certificate_status="disabled",dns_status="verified",service="worker",status="active",tls_mode="http_only"} 1`,
		`liteyuki_deployment_unavailable_replicas{deployment_target_id="dplt_1",environment_id="env_1",service="worker"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics body did not contain %q:\n%s", expected, body)
		}
	}
}

func TestAsynqQueueCollectorExportsQueueInfo(t *testing.T) {
	registry := NewRegistry("worker")
	registry.MustRegister(NewAsynqQueueCollector("worker", fakeQueueInspector{
		"build": &asynq.QueueInfo{
			Queue:          "build",
			Pending:        2,
			Retry:          1,
			Latency:        3 * time.Second,
			ProcessedTotal: 10,
			FailedTotal:    4,
		},
	}, []string{"build"}))

	metricsRecorder := httptest.NewRecorder()
	NewMetricsHandler(registry).ServeHTTP(metricsRecorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metricsRecorder.Body.String()
	for _, expected := range []string{
		`liteyuki_asynq_queue_depth{queue="build",service="worker",state="pending"} 2`,
		`liteyuki_asynq_queue_depth{queue="build",service="worker",state="retry"} 1`,
		`liteyuki_asynq_queue_latency_seconds{queue="build",service="worker"} 3`,
		`liteyuki_asynq_queue_processed_total{queue="build",service="worker"} 10`,
		`liteyuki_asynq_queue_failed_total{queue="build",service="worker"} 4`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics body did not contain %q:\n%s", expected, body)
		}
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
	if !strings.Contains(string(body), "liteyuki_up") {
		t.Fatalf("metrics body did not contain liteyuki_up:\n%s", body)
	}
}

type fakeQueueInspector map[string]*asynq.QueueInfo

func (f fakeQueueInspector) GetQueueInfo(queue string) (*asynq.QueueInfo, error) {
	return f[queue], nil
}
