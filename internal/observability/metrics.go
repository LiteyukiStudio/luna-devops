package observability

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

type MetricsConfig struct {
	Enabled bool
	Addr    string
	Path    string
	Service string
}

type DependencyCheck func(context.Context) error

type BusinessRunMetric struct {
	Status     string
	Type       string
	StartedAt  *time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
}

type DeploymentRuntimeMetric struct {
	DesiredReplicas   int32
	ReadyReplicas     int32
	AvailableReplicas int32
	UpdatedReplicas   int32
}

func (c MetricsConfig) Active() bool {
	return c.Enabled && strings.TrimSpace(c.Addr) != ""
}

func (c MetricsConfig) WithDefaultAddr(addr string) MetricsConfig {
	if c.Enabled && strings.TrimSpace(c.Addr) == "" {
		c.Addr = addr
	}
	return c
}

func (c MetricsConfig) normalizedPath() string {
	path := strings.TrimSpace(c.Path)
	if path == "" {
		return "/metrics"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func NewRegistry(service string) *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        "luna_devops_up",
			Help:        "Whether this Luna service process is running.",
			ConstLabels: prometheus.Labels{"service": service},
		}, func() float64 { return 1 }),
	)
	return registry
}

func RegisterDBStats(registry *prometheus.Registry, db *sql.DB, name string) {
	if registry == nil || db == nil {
		return
	}
	registry.MustRegister(collectors.NewDBStatsCollector(db, stableLabel(name, "database")))
}

func StartMetricsServer(config MetricsConfig, registry *prometheus.Registry) (*http.Server, error) {
	if !config.Active() {
		if config.Enabled {
			telemetry.Logger().Warn("metrics endpoint disabled",
				slog.String("event.name", "metrics.endpoint.disabled"),
				slog.String("service.component", config.Service),
				slog.String("reason.code", "metrics_address_empty"),
			)
		}
		return nil, nil
	}
	path := config.normalizedPath()
	mux := http.NewServeMux()
	mux.Handle(path, NewMetricsHandler(registry))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	server := &http.Server{
		Addr:              config.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", config.Addr)
	if err != nil {
		return nil, err
	}
	server.Addr = listener.Addr().String()
	go func() {
		telemetry.Logger().Info("metrics endpoint started",
			slog.String("event.name", "metrics.endpoint.started"),
			slog.String("service.component", config.Service),
			slog.String("server.address", server.Addr),
			slog.String("url.path", path),
		)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			telemetry.LogError(context.Background(), "Metrics endpoint failed",
				"metrics.endpoint.failed", "metrics.serve", "server.listen.failed", err,
				slog.String("service.component", config.Service))
		}
	}()
	return server, nil
}

func NewMetricsHandler(registry *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry})
}

func ShutdownMetricsServer(ctx context.Context, server *http.Server) {
	if server == nil {
		return
	}
	if err := server.Shutdown(ctx); err != nil {
		telemetry.LogError(ctx, "Metrics endpoint shutdown failed",
			"metrics.endpoint.shutdown_failed", "metrics.shutdown", "server.shutdown.failed", err)
	}
}

type HTTPMetrics struct {
	duration *prometheus.HistogramVec
	errors   *prometheus.CounterVec
	inflight *prometheus.GaugeVec
	requests *prometheus.CounterVec
}

func NewHTTPMetrics(registry *prometheus.Registry, service string) *HTTPMetrics {
	metrics := &HTTPMetrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "luna_devops_http_request_duration_seconds",
			Help:        "Duration of HTTP requests handled by Luna.",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     prometheus.DefBuckets,
		}, []string{"method", "route"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "luna_devops_api_errors_total",
			Help:        "Total HTTP error responses returned by Luna API.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"route", "status_class"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "luna_devops_http_request_inflight",
			Help:        "Current in-flight HTTP requests handled by Luna.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"route"}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "luna_devops_http_requests_total",
			Help:        "Total HTTP requests handled by Luna.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"method", "route", "status_code"}),
	}
	registry.MustRegister(metrics.duration, metrics.errors, metrics.inflight, metrics.requests)
	return metrics
}

func (m *HTTPMetrics) GinMiddleware() gin.HandlerFunc {
	if m == nil {
		return func(ctx *gin.Context) { ctx.Next() }
	}
	return func(ctx *gin.Context) {
		start := time.Now()
		route := routeLabel(ctx)
		m.inflight.WithLabelValues(route).Inc()
		defer func() {
			statusCode := ctx.Writer.Status()
			statusClass := strconv.Itoa(statusCode/100) + "xx"
			m.inflight.WithLabelValues(route).Dec()
			m.duration.WithLabelValues(ctx.Request.Method, route).Observe(time.Since(start).Seconds())
			m.requests.WithLabelValues(ctx.Request.Method, route, strconv.Itoa(statusCode)).Inc()
			if statusCode >= http.StatusBadRequest {
				m.errors.WithLabelValues(route, statusClass).Inc()
			}
		}()
		ctx.Next()
	}
}

type WorkerMetrics struct {
	queueFor               func(taskType string) string
	otelBuildDuration      otelmetric.Float64Histogram
	otelBuildRuns          otelmetric.Int64Counter
	otelCompleted          otelmetric.Int64Counter
	otelDeploymentObserved otelmetric.Int64Counter
	otelDeploymentRatio    otelmetric.Float64Histogram
	otelDeploymentReplicas otelmetric.Int64Histogram
	otelDuration           otelmetric.Float64Histogram
	otelGatewaySync        otelmetric.Int64Counter
	otelGatewayDuration    otelmetric.Float64Histogram
	otelInflight           otelmetric.Int64UpDownCounter
	otelReleaseDuration    otelmetric.Float64Histogram
	otelReleases           otelmetric.Int64Counter
	otelRetries            otelmetric.Int64Counter
	otelStarted            otelmetric.Int64Counter
}

func NewWorkerMetrics() *WorkerMetrics {
	meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/observability/worker")
	return &WorkerMetrics{
		otelBuildDuration:      mustFloat64Histogram(meter, "luna_devops_build_run_duration_seconds", "Duration of Luna build runs.", "s"),
		otelBuildRuns:          mustInt64Counter(meter, "luna_devops_build_runs_total", "Total Luna build runs completed by status and trigger type."),
		otelCompleted:          mustInt64Counter(meter, "luna_devops_worker_task_completed_total", "Total worker tasks completed by Luna."),
		otelDeploymentObserved: mustInt64Counter(meter, "luna_devops_deployment_observations_total", "Total Kubernetes deployment observations grouped by readiness state."),
		otelDeploymentRatio:    mustFloat64Histogram(meter, "luna_devops_deployment_ready_ratio", "Distribution of ready replica ratios observed for Luna deployments.", "1"),
		otelDeploymentReplicas: mustInt64Histogram(meter, "luna_devops_deployment_replica_count", "Distribution of desired, ready, available, and updated replica counts.", "{replica}"),
		otelDuration:           mustFloat64Histogram(meter, "luna_devops_worker_task_duration_seconds", "Duration of worker tasks processed by Luna.", "s"),
		otelGatewaySync:        mustInt64Counter(meter, "luna_devops_gateway_sync_total", "Total Luna gateway sync operations."),
		otelGatewayDuration:    mustFloat64Histogram(meter, "luna_devops_gateway_sync_duration_seconds", "Duration of Luna gateway sync operations.", "s"),
		otelInflight:           mustInt64UpDownCounter(meter, "luna_devops_worker_task_inflight", "Current in-flight worker tasks processed by Luna."),
		otelReleaseDuration:    mustFloat64Histogram(meter, "luna_devops_release_duration_seconds", "Duration of Luna release runs.", "s"),
		otelReleases:           mustInt64Counter(meter, "luna_devops_releases_total", "Total Luna releases completed by status and type."),
		otelRetries:            mustInt64Counter(meter, "luna_devops_worker_task_retries_total", "Total worker task retry attempts observed by Luna."),
		otelStarted:            mustInt64Counter(meter, "luna_devops_worker_task_started_total", "Total worker tasks started by Luna."),
	}
}

func (m *WorkerMetrics) WithQueueResolver(queueFor func(taskType string) string) *WorkerMetrics {
	if m == nil {
		return nil
	}
	m.queueFor = queueFor
	return m
}

func (m *WorkerMetrics) Middleware(next asynq.Handler) asynq.Handler {
	if m == nil {
		return next
	}
	return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
		start := time.Now()
		taskType := task.Type()
		queue := m.queueName(taskType)
		m.otelStarted.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("queue", queue), attribute.String("task_type", taskType)))
		if retryCount, ok := asynq.GetRetryCount(ctx); ok && retryCount > 0 {
			m.otelRetries.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("queue", queue), attribute.String("task_type", taskType)))
		}
		m.otelInflight.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("task_type", taskType)))
		err := next.ProcessTask(ctx, task)
		result := "succeeded"
		if err != nil {
			result = "failed"
		}
		attrs := otelmetric.WithAttributes(attribute.String("queue", queue), attribute.String("task_type", taskType), attribute.String("result", result))
		m.otelInflight.Add(ctx, -1, otelmetric.WithAttributes(attribute.String("task_type", taskType)))
		m.otelDuration.Record(ctx, time.Since(start).Seconds(), attrs)
		m.otelCompleted.Add(ctx, 1, attrs)
		return err
	})
}

func (m *WorkerMetrics) RecordBuildRun(ctx context.Context, run BusinessRunMetric) {
	if m == nil {
		return
	}
	status := stableLabel(run.Status, "unknown")
	triggerType := stableLabel(run.Type, "unknown")
	attrs := otelmetric.WithAttributes(attribute.String("status", status), attribute.String("trigger_type", triggerType))
	m.otelBuildRuns.Add(ctx, 1, attrs)
	if duration, ok := runDuration(run); ok {
		m.otelBuildDuration.Record(ctx, duration.Seconds(), attrs)
	}
}

func (m *WorkerMetrics) RecordRelease(ctx context.Context, run BusinessRunMetric) {
	if m == nil {
		return
	}
	status := stableLabel(run.Status, "unknown")
	releaseType := stableLabel(run.Type, "deploy")
	attrs := otelmetric.WithAttributes(attribute.String("status", status), attribute.String("type", releaseType))
	m.otelReleases.Add(ctx, 1, attrs)
	if duration, ok := runDuration(run); ok {
		m.otelReleaseDuration.Record(ctx, duration.Seconds(), attrs)
	}
}

func (m *WorkerMetrics) RecordGatewaySync(ctx context.Context, operation string, result string, duration time.Duration) {
	if m == nil {
		return
	}
	operation = stableLabel(operation, "apply")
	result = stableLabel(result, "unknown")
	attrs := otelmetric.WithAttributes(attribute.String("operation", operation), attribute.String("result", result))
	m.otelGatewaySync.Add(ctx, 1, attrs)
	if duration >= 0 {
		m.otelGatewayDuration.Record(ctx, duration.Seconds(), attrs)
	}
}

func (m *WorkerMetrics) SetDeploymentRuntime(ctx context.Context, metric DeploymentRuntimeMetric) {
	if m == nil {
		return
	}
	desired := float64(nonNegativeInt32(metric.DesiredReplicas))
	ready := float64(nonNegativeInt32(metric.ReadyReplicas))
	available := float64(nonNegativeInt32(metric.AvailableReplicas))
	updated := float64(nonNegativeInt32(metric.UpdatedReplicas))
	state := "ready"
	if desired > 0 && available < desired {
		state = "degraded"
	}
	readyRatio := 1.0
	if desired > 0 {
		readyRatio = ready / desired
	}
	m.otelDeploymentObserved.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("state", state)))
	m.otelDeploymentRatio.Record(ctx, readyRatio, otelmetric.WithAttributes(attribute.String("state", state)))
	m.otelDeploymentReplicas.Record(ctx, int64(desired), otelmetric.WithAttributes(attribute.String("kind", "desired")))
	m.otelDeploymentReplicas.Record(ctx, int64(ready), otelmetric.WithAttributes(attribute.String("kind", "ready")))
	m.otelDeploymentReplicas.Record(ctx, int64(available), otelmetric.WithAttributes(attribute.String("kind", "available")))
	m.otelDeploymentReplicas.Record(ctx, int64(updated), otelmetric.WithAttributes(attribute.String("kind", "updated")))
}

func (m *WorkerMetrics) queueName(taskType string) string {
	if m.queueFor == nil {
		return "unknown"
	}
	queue := strings.TrimSpace(m.queueFor(taskType))
	if queue == "" {
		return "unknown"
	}
	return queue
}

func mustInt64Counter(meter otelmetric.Meter, name string, description string) otelmetric.Int64Counter {
	instrument, err := meter.Int64Counter(name, otelmetric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return instrument
}

func mustInt64UpDownCounter(meter otelmetric.Meter, name string, description string) otelmetric.Int64UpDownCounter {
	instrument, err := meter.Int64UpDownCounter(name, otelmetric.WithDescription(description))
	if err != nil {
		panic(err)
	}
	return instrument
}

func mustInt64Histogram(meter otelmetric.Meter, name string, description string, unit string) otelmetric.Int64Histogram {
	instrument, err := meter.Int64Histogram(name, otelmetric.WithDescription(description), otelmetric.WithUnit(unit))
	if err != nil {
		panic(err)
	}
	return instrument
}

func mustFloat64Histogram(meter otelmetric.Meter, name string, description string, unit string) otelmetric.Float64Histogram {
	instrument, err := meter.Float64Histogram(name, otelmetric.WithDescription(description), otelmetric.WithUnit(unit))
	if err != nil {
		panic(err)
	}
	return instrument
}

type DependencyCollector struct {
	checks   map[string]DependencyCheck
	duration *prometheus.Desc
	up       *prometheus.Desc
}

func NewDependencyCollector(service string, checks map[string]DependencyCheck) prometheus.Collector {
	normalized := make(map[string]DependencyCheck, len(checks))
	for name, check := range checks {
		name = stableLabel(name, "")
		if name == "" || check == nil {
			continue
		}
		normalized[name] = check
	}
	return &DependencyCollector{
		checks: normalized,
		up: prometheus.NewDesc(
			"luna_devops_dependency_up",
			"Whether a Luna runtime dependency is reachable.",
			[]string{"dependency"},
			prometheus.Labels{"service": service},
		),
		duration: prometheus.NewDesc(
			"luna_devops_dependency_check_duration_seconds",
			"Duration of Luna dependency health checks.",
			[]string{"dependency"},
			prometheus.Labels{"service": service},
		),
	}
}

func (c *DependencyCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.duration
}

func (c *DependencyCollector) Collect(ch chan<- prometheus.Metric) {
	for name, check := range c.checks {
		start := time.Now()
		// Prometheus collection is an independent runtime lifecycle operation,
		// not a child of an application request.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := check(ctx)
		cancel()
		up := 1.0
		if err != nil {
			up = 0
		}
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, up, name)
		ch <- prometheus.MustNewConstMetric(c.duration, prometheus.GaugeValue, time.Since(start).Seconds(), name)
	}
}

func routeLabel(ctx *gin.Context) string {
	if route := strings.TrimSpace(ctx.FullPath()); route != "" {
		return route
	}
	if ctx.Request == nil || strings.TrimSpace(ctx.Request.URL.Path) == "" {
		return "unknown"
	}
	return "unmatched"
}

func runDuration(run BusinessRunMetric) (time.Duration, bool) {
	if run.FinishedAt == nil {
		return 0, false
	}
	start := run.CreatedAt
	if run.StartedAt != nil {
		start = *run.StartedAt
	}
	if start.IsZero() || run.FinishedAt.Before(start) {
		return 0, false
	}
	return run.FinishedAt.Sub(start), true
}

func stableLabel(value string, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_", ":", "_", "/", "_", ".", "_")
	return replacer.Replace(value)
}

func nonNegativeInt32(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value
}
