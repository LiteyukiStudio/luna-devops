package telemetry

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	httpMetricsOnce     sync.Once
	httpRequestCounter  metric.Int64Counter
	httpRequestDuration metric.Float64Histogram
	httpRequestInflight metric.Int64UpDownCounter
)

const (
	httpErrorCodeContextKey   = "luna.telemetry.error_code"
	httpErrorDetailContextKey = "luna.telemetry.error_detail"
	httpErrorLoggedContextKey = "luna.telemetry.error_logged"
)

func SetHTTPError(ctx *gin.Context, code, detail string) {
	if ctx == nil {
		return
	}
	ctx.Set(httpErrorCodeContextKey, strings.TrimSpace(code))
	ctx.Set(httpErrorDetailContextKey, strings.TrimSpace(detail))
}

func MarkHTTPErrorLogged(ctx *gin.Context) {
	if ctx != nil {
		ctx.Set(httpErrorLoggedContextKey, true)
	}
}

func GinTracingMiddleware(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName, otelgin.WithFilter(func(request *http.Request) bool {
		return !IsHealthCheckPath(request.URL.Path)
	}))
}

// IsHealthCheckPath identifies machine probes that should remain metrics-only.
func IsHealthCheckPath(path string) bool {
	return path == "/healthz" || path == "/internal/health/live" || path == "/internal/health/ready"
}

// QueryTraceContextMiddleware bridges W3C context for browser EventSource and
// WebSocket APIs, which cannot set arbitrary request headers. The private
// query parameters are removed before routing and never reach access logs.
func QueryTraceContextMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		query := ctx.Request.URL.Query()
		for parameter, header := range map[string]string{
			"_otel_traceparent": "traceparent",
			"_otel_tracestate":  "tracestate",
		} {
			value := query.Get(parameter)
			if value != "" && len(value) <= 512 {
				ctx.Request.Header.Set(header, value)
			}
			query.Del(parameter)
		}
		ctx.Request.URL.RawQuery = query.Encode()
		ctx.Next()
	}
}

// GinAccessLogMiddleware emits one completion record at the HTTP boundary.
// Query strings and request/response bodies are deliberately excluded.
func GinAccessLogMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if IsHealthCheckPath(ctx.Request.URL.Path) {
			ctx.Next()
			return
		}
		httpMetricsOnce.Do(func() {
			meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/telemetry/http")
			httpRequestCounter, _ = meter.Int64Counter("luna_devops_http_server_request_total",
				metric.WithDescription("Total HTTP requests completed by Luna services."))
			httpRequestDuration, _ = meter.Float64Histogram("luna_devops_http_server_request_duration_seconds",
				metric.WithDescription("Duration of HTTP requests handled by Luna services."), metric.WithUnit("s"))
			httpRequestInflight, _ = meter.Int64UpDownCounter("luna_devops_http_server_request_inflight",
				metric.WithDescription("Current HTTP requests handled by Luna services."))
		})
		startedAt := time.Now()
		route := ctx.FullPath()
		if route == "" {
			route = "unmatched"
		}
		inflightAttrs := metric.WithAttributes(
			attribute.String("http.request.method", ctx.Request.Method),
			attribute.String("http.route", route),
		)
		if httpRequestInflight != nil {
			httpRequestInflight.Add(ctx.Request.Context(), 1, inflightAttrs)
			defer httpRequestInflight.Add(ctx.Request.Context(), -1, inflightAttrs)
		}
		ctx.Next()
		route = ctx.FullPath()
		if route == "" {
			route = "unmatched"
		}
		requestCtx := ctx.Request.Context()
		statusClass := strconv.Itoa(ctx.Writer.Status()/100) + "xx"
		requestMetricAttrs := metric.WithAttributes(
			attribute.String("http.request.method", ctx.Request.Method),
			attribute.String("http.route", route),
			attribute.String("http.response.status_class", statusClass),
		)
		if httpRequestCounter != nil {
			httpRequestCounter.Add(requestCtx, 1, requestMetricAttrs)
		}
		if httpRequestDuration != nil {
			httpRequestDuration.Record(requestCtx, time.Since(startedAt).Seconds(), requestMetricAttrs)
		}
		span := trace.SpanFromContext(requestCtx)
		span.SetAttributes(
			attribute.String("http.route", route),
			attribute.String("luna.request.id", RequestIDFromContext(requestCtx)),
			attribute.Int("http.response.status_code", ctx.Writer.Status()),
		)
		attrs := []any{
			slog.String("event.name", "http.request.completed"),
			slog.String("operation", "http.request"),
			slog.String("http.request.method", ctx.Request.Method),
			slog.String("http.route", route),
			slog.Int("http.response.status_code", ctx.Writer.Status()),
			slog.Int64("http.server.duration_ms", time.Since(startedAt).Milliseconds()),
			slog.String("network.protocol.version", ctx.Request.Proto),
		}
		if ctx.Writer.Status() < http.StatusBadRequest {
			attrs = append(attrs, slog.String("outcome", "succeeded"))
		}
		if ctx.Writer.Status() >= http.StatusInternalServerError {
			attrs = append(attrs, httpErrorAttrs(ctx, "failed")...)
			if logged, _ := ctx.Get(httpErrorLoggedContextKey); logged == true {
				Logger().InfoContext(requestCtx, "HTTP request completed with previously logged failure", attrs...)
				return
			}
			Logger().ErrorContext(requestCtx, "HTTP request failed", attrs...)
			return
		}
		if ctx.Writer.Status() >= http.StatusBadRequest {
			attrs = append(attrs, slog.String("http.response.status_class", statusClass))
			attrs = append(attrs, httpErrorAttrs(ctx, "rejected")...)
			if logged, _ := ctx.Get(httpErrorLoggedContextKey); logged == true {
				Logger().InfoContext(requestCtx, "HTTP request completed with previously logged rejection", attrs...)
				return
			}
			Logger().WarnContext(requestCtx, "HTTP request rejected", attrs...)
			return
		}
		Logger().InfoContext(requestCtx, "HTTP request completed", attrs...)
	}
}

func httpErrorAttrs(ctx *gin.Context, outcome string) []any {
	attrs := []any{slog.String("outcome", outcome)}
	if code, ok := ctx.Get(httpErrorCodeContextKey); ok {
		if value, valid := code.(string); valid && value != "" {
			attrs = append(attrs, slog.String("error.code", value))
		}
	}
	if detail, ok := ctx.Get(httpErrorDetailContextKey); ok {
		if value, valid := detail.(string); valid && value != "" {
			attrs = append(attrs,
				slog.String("error.type", "api.response_error"),
				slog.String("error.message", value),
			)
		}
	}
	return attrs
}
