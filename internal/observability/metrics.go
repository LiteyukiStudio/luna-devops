package observability

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

type GatewayRouteMetric struct {
	Status            string
	TLSMode           string
	DNSStatus         string
	CertificateStatus string
}

type DeploymentRuntimeMetric struct {
	DeploymentTargetID string
	EnvironmentID      string
	DesiredReplicas    int32
	ReadyReplicas      int32
	AvailableReplicas  int32
	UpdatedReplicas    int32
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
			Name:        "liteyuki_up",
			Help:        "Whether this Liteyuki service process is running.",
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
			log.Printf("metrics disabled for %s: METRICS_ADDR is empty", config.Service)
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
		log.Printf("%s metrics listening on %s%s", config.Service, config.Addr, path)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("serve %s metrics: %v", config.Service, err)
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
		log.Printf("shutdown metrics server: %v", err)
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
			Name:        "liteyuki_http_request_duration_seconds",
			Help:        "Duration of HTTP requests handled by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     prometheus.DefBuckets,
		}, []string{"method", "route"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_api_errors_total",
			Help:        "Total HTTP error responses returned by Liteyuki API.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"route", "status_class"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_http_request_inflight",
			Help:        "Current in-flight HTTP requests handled by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"route"}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_http_requests_total",
			Help:        "Total HTTP requests handled by Liteyuki.",
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
	buildDuration                 *prometheus.HistogramVec
	buildRuns                     *prometheus.CounterVec
	completed                     *prometheus.CounterVec
	deploymentAvailableReplicas   *prometheus.GaugeVec
	deploymentDesiredReplicas     *prometheus.GaugeVec
	deploymentReadyReplicas       *prometheus.GaugeVec
	deploymentUnavailableReplicas *prometheus.GaugeVec
	deploymentUpdatedReplicas     *prometheus.GaugeVec
	duration                      *prometheus.HistogramVec
	gatewayRoutes                 *prometheus.GaugeVec
	gatewaySync                   *prometheus.CounterVec
	gatewaySyncDuration           *prometheus.HistogramVec
	inflight                      *prometheus.GaugeVec
	queueFor                      func(taskType string) string
	releaseDuration               *prometheus.HistogramVec
	releases                      *prometheus.CounterVec
	retries                       *prometheus.CounterVec
	started                       *prometheus.CounterVec
}

func NewWorkerMetrics(registry *prometheus.Registry, service string) *WorkerMetrics {
	metrics := &WorkerMetrics{
		buildDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "liteyuki_build_run_duration_seconds",
			Help:        "Duration of Liteyuki build runs.",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     []float64{30, 60, 120, 300, 600, 900, 1800, 3600, 5400},
		}, []string{"status", "trigger_type"}),
		buildRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_build_runs_total",
			Help:        "Total Liteyuki build runs completed by status and trigger type.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"status", "trigger_type"}),
		completed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_worker_task_completed_total",
			Help:        "Total worker tasks completed by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"queue", "task_type", "result"}),
		deploymentAvailableReplicas: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_deployment_available_replicas",
			Help:        "Available replicas reported by Kubernetes for a Liteyuki deployment target.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"deployment_target_id", "environment_id"}),
		deploymentDesiredReplicas: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_deployment_desired_replicas",
			Help:        "Desired replicas reported by Kubernetes for a Liteyuki deployment target.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"deployment_target_id", "environment_id"}),
		deploymentReadyReplicas: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_deployment_ready_replicas",
			Help:        "Ready replicas reported by Kubernetes for a Liteyuki deployment target.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"deployment_target_id", "environment_id"}),
		deploymentUnavailableReplicas: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_deployment_unavailable_replicas",
			Help:        "Unavailable replicas reported by Kubernetes for a Liteyuki deployment target.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"deployment_target_id", "environment_id"}),
		deploymentUpdatedReplicas: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_deployment_updated_replicas",
			Help:        "Updated replicas reported by Kubernetes for a Liteyuki deployment target.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"deployment_target_id", "environment_id"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "liteyuki_worker_task_duration_seconds",
			Help:        "Duration of worker tasks processed by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     prometheus.DefBuckets,
		}, []string{"task_type", "result"}),
		gatewayRoutes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_gateway_routes_total",
			Help:        "Current Liteyuki gateway route count by status and TLS state.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"status", "tls_mode", "dns_status", "certificate_status"}),
		gatewaySync: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_gateway_sync_total",
			Help:        "Total Liteyuki gateway sync operations.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"operation", "result"}),
		gatewaySyncDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "liteyuki_gateway_sync_duration_seconds",
			Help:        "Duration of Liteyuki gateway sync operations.",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"operation", "result"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_worker_task_inflight",
			Help:        "Current in-flight worker tasks processed by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"task_type"}),
		releaseDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "liteyuki_release_duration_seconds",
			Help:        "Duration of Liteyuki release runs.",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     []float64{5, 10, 30, 60, 120, 300, 600, 900, 1800},
		}, []string{"status", "type"}),
		releases: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_releases_total",
			Help:        "Total Liteyuki releases completed by status and type.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"status", "type"}),
		retries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_worker_task_retries_total",
			Help:        "Total worker task retry attempts observed by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"queue", "task_type"}),
		started: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_worker_task_started_total",
			Help:        "Total worker tasks started by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"queue", "task_type"}),
	}
	registry.MustRegister(
		metrics.buildDuration,
		metrics.buildRuns,
		metrics.completed,
		metrics.deploymentAvailableReplicas,
		metrics.deploymentDesiredReplicas,
		metrics.deploymentReadyReplicas,
		metrics.deploymentUnavailableReplicas,
		metrics.deploymentUpdatedReplicas,
		metrics.duration,
		metrics.gatewayRoutes,
		metrics.gatewaySync,
		metrics.gatewaySyncDuration,
		metrics.inflight,
		metrics.releaseDuration,
		metrics.releases,
		metrics.retries,
		metrics.started,
	)
	return metrics
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
		m.started.WithLabelValues(queue, taskType).Inc()
		if retryCount, ok := asynq.GetRetryCount(ctx); ok && retryCount > 0 {
			m.retries.WithLabelValues(queue, taskType).Inc()
		}
		m.inflight.WithLabelValues(taskType).Inc()
		err := next.ProcessTask(ctx, task)
		result := "succeeded"
		if err != nil {
			result = "failed"
		}
		m.inflight.WithLabelValues(taskType).Dec()
		m.duration.WithLabelValues(taskType, result).Observe(time.Since(start).Seconds())
		m.completed.WithLabelValues(queue, taskType, result).Inc()
		return err
	})
}

func (m *WorkerMetrics) RecordBuildRun(run BusinessRunMetric) {
	if m == nil {
		return
	}
	status := stableLabel(run.Status, "unknown")
	triggerType := stableLabel(run.Type, "unknown")
	m.buildRuns.WithLabelValues(status, triggerType).Inc()
	if duration, ok := runDuration(run); ok {
		m.buildDuration.WithLabelValues(status, triggerType).Observe(duration.Seconds())
	}
}

func (m *WorkerMetrics) RecordRelease(run BusinessRunMetric) {
	if m == nil {
		return
	}
	status := stableLabel(run.Status, "unknown")
	releaseType := stableLabel(run.Type, "deploy")
	m.releases.WithLabelValues(status, releaseType).Inc()
	if duration, ok := runDuration(run); ok {
		m.releaseDuration.WithLabelValues(status, releaseType).Observe(duration.Seconds())
	}
}

func (m *WorkerMetrics) RecordGatewaySync(operation string, result string, duration time.Duration) {
	if m == nil {
		return
	}
	operation = stableLabel(operation, "apply")
	result = stableLabel(result, "unknown")
	m.gatewaySync.WithLabelValues(operation, result).Inc()
	if duration >= 0 {
		m.gatewaySyncDuration.WithLabelValues(operation, result).Observe(duration.Seconds())
	}
}

func (m *WorkerMetrics) SetGatewayRoutes(routes []GatewayRouteMetric) {
	if m == nil {
		return
	}
	m.gatewayRoutes.Reset()
	for _, route := range routes {
		m.gatewayRoutes.WithLabelValues(
			stableLabel(route.Status, "unknown"),
			stableLabel(route.TLSMode, "unknown"),
			stableLabel(route.DNSStatus, "unknown"),
			stableLabel(route.CertificateStatus, "unknown"),
		).Inc()
	}
}

func (m *WorkerMetrics) SetDeploymentRuntime(metric DeploymentRuntimeMetric) {
	if m == nil {
		return
	}
	targetID := stableLabel(metric.DeploymentTargetID, "unknown")
	environmentID := stableLabel(metric.EnvironmentID, "unknown")
	desired := float64(nonNegativeInt32(metric.DesiredReplicas))
	ready := float64(nonNegativeInt32(metric.ReadyReplicas))
	available := float64(nonNegativeInt32(metric.AvailableReplicas))
	updated := float64(nonNegativeInt32(metric.UpdatedReplicas))
	unavailable := desired - available
	if unavailable < 0 {
		unavailable = 0
	}
	m.deploymentDesiredReplicas.WithLabelValues(targetID, environmentID).Set(desired)
	m.deploymentReadyReplicas.WithLabelValues(targetID, environmentID).Set(ready)
	m.deploymentAvailableReplicas.WithLabelValues(targetID, environmentID).Set(available)
	m.deploymentUpdatedReplicas.WithLabelValues(targetID, environmentID).Set(updated)
	m.deploymentUnavailableReplicas.WithLabelValues(targetID, environmentID).Set(unavailable)
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
			"liteyuki_dependency_up",
			"Whether a Liteyuki runtime dependency is reachable.",
			[]string{"dependency"},
			prometheus.Labels{"service": service},
		),
		duration: prometheus.NewDesc(
			"liteyuki_dependency_check_duration_seconds",
			"Duration of Liteyuki dependency health checks.",
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

type AsynqQueueCollector struct {
	inspector interface {
		GetQueueInfo(string) (*asynq.QueueInfo, error)
	}
	queues    []string
	depth     *prometheus.Desc
	failed    *prometheus.Desc
	latency   *prometheus.Desc
	processed *prometheus.Desc
}

func NewAsynqQueueCollector(service string, inspector interface {
	GetQueueInfo(string) (*asynq.QueueInfo, error)
}, queues []string) prometheus.Collector {
	return &AsynqQueueCollector{
		inspector: inspector,
		queues:    append([]string(nil), queues...),
		depth: prometheus.NewDesc(
			"liteyuki_asynq_queue_depth",
			"Current Asynq queue task count by state.",
			[]string{"queue", "state"},
			prometheus.Labels{"service": service},
		),
		failed: prometheus.NewDesc(
			"liteyuki_asynq_queue_failed_total",
			"Total Asynq tasks failed by queue.",
			[]string{"queue"},
			prometheus.Labels{"service": service},
		),
		latency: prometheus.NewDesc(
			"liteyuki_asynq_queue_latency_seconds",
			"Latency of the oldest pending task in an Asynq queue.",
			[]string{"queue"},
			prometheus.Labels{"service": service},
		),
		processed: prometheus.NewDesc(
			"liteyuki_asynq_queue_processed_total",
			"Total Asynq tasks processed by queue.",
			[]string{"queue"},
			prometheus.Labels{"service": service},
		),
	}
}

func (c *AsynqQueueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.depth
	ch <- c.failed
	ch <- c.latency
	ch <- c.processed
}

func (c *AsynqQueueCollector) Collect(ch chan<- prometheus.Metric) {
	if c.inspector == nil {
		return
	}
	for _, queue := range c.queues {
		info, err := c.inspector.GetQueueInfo(queue)
		if err != nil {
			continue
		}
		queueName := stableLabel(info.Queue, queue)
		states := map[string]int{
			"active":      info.Active,
			"aggregating": info.Aggregating,
			"archived":    info.Archived,
			"completed":   info.Completed,
			"pending":     info.Pending,
			"retry":       info.Retry,
			"scheduled":   info.Scheduled,
		}
		for state, value := range states {
			ch <- prometheus.MustNewConstMetric(c.depth, prometheus.GaugeValue, float64(value), queueName, state)
		}
		ch <- prometheus.MustNewConstMetric(c.failed, prometheus.CounterValue, float64(info.FailedTotal), queueName)
		ch <- prometheus.MustNewConstMetric(c.latency, prometheus.GaugeValue, info.Latency.Seconds(), queueName)
		ch <- prometheus.MustNewConstMetric(c.processed, prometheus.CounterValue, float64(info.ProcessedTotal), queueName)
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
