package observability

import (
	"context"
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

func (c MetricsConfig) Active() bool {
	return c.Enabled && strings.TrimSpace(c.Addr) != ""
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
	registry.MustRegister(metrics.duration, metrics.inflight, metrics.requests)
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
			m.inflight.WithLabelValues(route).Dec()
			m.duration.WithLabelValues(ctx.Request.Method, route).Observe(time.Since(start).Seconds())
			m.requests.WithLabelValues(ctx.Request.Method, route, strconv.Itoa(ctx.Writer.Status())).Inc()
		}()
		ctx.Next()
	}
}

type WorkerMetrics struct {
	completed *prometheus.CounterVec
	duration  *prometheus.HistogramVec
	inflight  *prometheus.GaugeVec
	queueFor  func(taskType string) string
	started   *prometheus.CounterVec
}

func NewWorkerMetrics(registry *prometheus.Registry, service string) *WorkerMetrics {
	metrics := &WorkerMetrics{
		completed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_worker_task_completed_total",
			Help:        "Total worker tasks completed by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"queue", "task_type", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "liteyuki_worker_task_duration_seconds",
			Help:        "Duration of worker tasks processed by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
			Buckets:     prometheus.DefBuckets,
		}, []string{"task_type", "result"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "liteyuki_worker_task_inflight",
			Help:        "Current in-flight worker tasks processed by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"task_type"}),
		started: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "liteyuki_worker_task_started_total",
			Help:        "Total worker tasks started by Liteyuki.",
			ConstLabels: prometheus.Labels{"service": service},
		}, []string{"queue", "task_type"}),
	}
	registry.MustRegister(metrics.completed, metrics.duration, metrics.inflight, metrics.started)
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

func routeLabel(ctx *gin.Context) string {
	if route := strings.TrimSpace(ctx.FullPath()); route != "" {
		return route
	}
	if ctx.Request == nil || strings.TrimSpace(ctx.Request.URL.Path) == "" {
		return "unknown"
	}
	return "unmatched"
}
