package kubeproxy

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	tracer           trace.Tracer
	meter            metric.Meter
	logger           *slog.Logger
	once             sync.Once
	requests         metric.Int64Counter
	failures         metric.Int64Counter
	denials          metric.Int64Counter
	upstreamFailures metric.Int64Counter
	rateLimits       metric.Int64Counter
	auditFailures    metric.Int64Counter
	duration         metric.Float64Histogram
	inflight         metric.Int64UpDownCounter
}

func NewTelemetry(logger *slog.Logger) *Telemetry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Telemetry{
		tracer: otel.Tracer("github.com/LiteyukiStudio/devops/internal/kubeproxy"),
		meter:  otel.Meter("github.com/LiteyukiStudio/devops/internal/kubeproxy"), logger: logger,
	}
}

type RequestTelemetry struct {
	owner         *Telemetry
	ctx           context.Context
	span          trace.Span
	startedAt     time.Time
	attrs         []attribute.KeyValue
	inflightAttrs []attribute.KeyValue
	once          sync.Once
}

func (request *RequestTelemetry) Classify(info RequestInfo, category string) {
	if request == nil {
		return
	}
	classified := []attribute.KeyValue{
		attribute.String("http.request.method", stableMethod(info.Method)),
		attribute.String("luna.kube.transport", stableTransport(info.Transport)),
		attribute.String("luna.kube.verb_class", stableVerb(info.Verb)),
		attribute.String("luna.kube.resource_category", stableCategory(category)),
	}
	request.owner.inflight.Add(request.ctx, -1, metric.WithAttributes(request.inflightAttrs...))
	request.owner.inflight.Add(request.ctx, 1, metric.WithAttributes(classified...))
	request.attrs = classified
	request.inflightAttrs = append([]attribute.KeyValue(nil), classified...)
	request.span.SetAttributes(request.attrs...)
}

func (telemetry *Telemetry) StartRequest(request *http.Request, info RequestInfo, category string) (*http.Request, *RequestTelemetry) {
	if telemetry == nil {
		telemetry = NewTelemetry(nil)
	}
	telemetry.initialize()
	ctx := baggage.ContextWithoutBaggage(request.Context())
	ctx = propagation.TraceContext{}.Extract(ctx, propagation.HeaderCarrier(request.Header))
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", stableMethod(request.Method)),
		attribute.String("luna.kube.transport", stableTransport(info.Transport)),
		attribute.String("luna.kube.verb_class", stableVerb(info.Verb)),
		attribute.String("luna.kube.resource_category", stableCategory(category)),
	}
	ctx, span := telemetry.tracer.Start(ctx, "kube.gateway.request", trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attrs...))
	telemetry.inflight.Add(ctx, 1, metric.WithAttributes(attrs...))
	return request.WithContext(ctx), &RequestTelemetry{owner: telemetry, ctx: ctx, span: span, startedAt: time.Now(), attrs: attrs, inflightAttrs: append([]attribute.KeyValue(nil), attrs...)}
}

func (telemetry *Telemetry) StartInternal(ctx context.Context, operation string, kind trace.SpanKind) (context.Context, trace.Span) {
	if telemetry == nil {
		telemetry = NewTelemetry(nil)
	}
	return telemetry.tracer.Start(ctx, operation, trace.WithSpanKind(kind))
}

func (telemetry *Telemetry) InjectUpstream(ctx context.Context, header http.Header) {
	if header == nil {
		return
	}
	header.Del("traceparent")
	header.Del("tracestate")
	header.Del("baggage")
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(header))
}

func (telemetry *Telemetry) RecordAuditFailure(ctx context.Context) {
	if telemetry == nil {
		return
	}
	telemetry.initialize()
	telemetry.auditFailures.Add(ctx, 1)
	telemetry.logger.LogAttrs(ctx, slog.LevelError, "Kubernetes gateway audit finalization failed",
		slog.String("event.name", "kube.gateway.audit.finish_failed"),
		slog.String("operation", "kube.gateway.audit.finish"),
		slog.String("error.code", CodeAuditUnavailable),
	)
}

func (request *RequestTelemetry) End(statusCode int, errorCode string, err error) {
	if request == nil {
		return
	}
	request.once.Do(func() {
		outcome := "succeeded"
		if err != nil || statusCode >= 500 {
			outcome = "failed"
			request.span.SetStatus(codes.Error, stableErrorCode(errorCode))
		} else if statusCode >= 400 {
			outcome = "rejected"
			request.span.SetStatus(codes.Error, stableErrorCode(errorCode))
		} else {
			request.span.SetStatus(codes.Ok, "")
		}
		attrs := append(append([]attribute.KeyValue(nil), request.attrs...), attribute.String("outcome", outcome), attribute.String("error.code", stableErrorCode(errorCode)))
		request.span.SetAttributes(attribute.Int("http.response.status_code", statusCode))
		request.owner.requests.Add(request.ctx, 1, metric.WithAttributes(attrs...))
		if outcome == "failed" {
			request.owner.failures.Add(request.ctx, 1, metric.WithAttributes(attrs...))
		}
		if outcome == "rejected" {
			request.owner.denials.Add(request.ctx, 1, metric.WithAttributes(attrs...))
		}
		if stableErrorCode(errorCode) == CodeRateLimited {
			request.owner.rateLimits.Add(request.ctx, 1, metric.WithAttributes(attrs...))
		}
		switch stableErrorCode(errorCode) {
		case CodeUnavailable, CodeUpstreamTimeout, CodeMetricsSelectorUnavailable:
			request.owner.upstreamFailures.Add(request.ctx, 1, metric.WithAttributes(attrs...))
		}
		request.owner.duration.Record(request.ctx, time.Since(request.startedAt).Seconds(), metric.WithAttributes(attrs...))
		request.owner.inflight.Add(request.ctx, -1, metric.WithAttributes(request.inflightAttrs...))
		request.owner.logger.LogAttrs(request.ctx, slog.LevelInfo, "Kubernetes gateway request completed",
			slog.String("event.name", "kube.gateway.request.completed"), slog.String("operation", "kube.gateway.request"),
			slog.String("outcome", outcome), slog.Int("http.response.status_code", statusCode), slog.String("error.code", stableErrorCode(errorCode)),
		)
		request.span.End()
	})
}

func (telemetry *Telemetry) initialize() {
	telemetry.once.Do(func() {
		telemetry.requests, _ = telemetry.meter.Int64Counter("luna_devops_kube_gateway_request_total")
		telemetry.failures, _ = telemetry.meter.Int64Counter("luna_devops_kube_gateway_failure_total")
		telemetry.denials, _ = telemetry.meter.Int64Counter("luna_devops_kube_gateway_denial_total")
		telemetry.upstreamFailures, _ = telemetry.meter.Int64Counter("luna_devops_kube_gateway_upstream_failure_total")
		telemetry.rateLimits, _ = telemetry.meter.Int64Counter("luna_devops_kube_gateway_rate_limit_total")
		telemetry.auditFailures, _ = telemetry.meter.Int64Counter("luna_devops_kube_gateway_audit_failure_total")
		telemetry.duration, _ = telemetry.meter.Float64Histogram("luna_devops_kube_gateway_request_duration_seconds", metric.WithUnit("s"))
		telemetry.inflight, _ = telemetry.meter.Int64UpDownCounter("luna_devops_kube_gateway_request_inflight")
	})
}

func stableMethod(value string) string {
	switch value {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return value
	default:
		return "OTHER"
	}
}

func stableTransport(value TransportClass) string {
	switch value {
	case TransportNormal, TransportWatch, TransportLogs, TransportUpgrade:
		return string(value)
	default:
		return "unknown"
	}
}

func stableVerb(value string) string {
	switch value {
	case "get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "connect":
		return value
	default:
		return "unknown"
	}
}

func stableCategory(value string) string {
	switch value {
	case "workload", "network", "config", "storage", "gateway", "observation", "namespace", "platform_config", "review", "extra", "discovery":
		return value
	default:
		return "unknown"
	}
}

func stableErrorCode(value string) string {
	if value == "" {
		return "none"
	}
	if len(value) > 96 {
		return "kube_gateway.unknown"
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			continue
		}
		return "kube_gateway.unknown"
	}
	return value
}
