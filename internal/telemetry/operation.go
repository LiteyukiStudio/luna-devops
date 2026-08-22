package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type OperationEnd func(error)

var (
	operationInstrumentsOnce sync.Once
	operationCounter         metric.Int64Counter
	operationDuration        metric.Float64Histogram
)

func StartOperation(ctx context.Context, domain, operation string, attrs ...attribute.KeyValue) (context.Context, OperationEnd) {
	return StartOperationWithKind(ctx, domain, operation, trace.SpanKindInternal, attrs...)
}

func StartOperationWithKind(ctx context.Context, domain, operation string, kind trace.SpanKind, attrs ...attribute.KeyValue) (context.Context, OperationEnd) {
	operationInstrumentsOnce.Do(func() {
		meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/telemetry")
		operationCounter, _ = meter.Int64Counter("luna_devops_operation_total",
			metric.WithDescription("Total completed Luna business operations."))
		operationDuration, _ = meter.Float64Histogram("luna_devops_operation_duration_seconds",
			metric.WithDescription("Duration of Luna business operations."), metric.WithUnit("s"))
	})
	spanAttrs := append([]attribute.KeyValue{
		attribute.String("luna.domain", domain),
		attribute.String("luna.operation", operation),
	}, attrs...)
	ctx, span := otel.Tracer("github.com/LiteyukiStudio/devops").Start(ctx,
		domain+"."+operation,
		trace.WithSpanKind(kind),
		trace.WithAttributes(spanAttrs...),
	)
	startedAt := time.Now()
	return ctx, func(err error) {
		outcome := ErrorOutcome(err)
		if err != nil {
			span.AddEvent("operation.error", trace.WithAttributes(
				attribute.String("error.type", ErrorType(err)),
				attribute.String("error.message", RedactText(err.Error())),
			))
			span.SetStatus(codes.Error, "failed")
		} else {
			span.SetStatus(codes.Ok, "")
		}
		metricAttrs := metric.WithAttributes(
			attribute.String("domain", domain),
			attribute.String("operation", operation),
			attribute.String("outcome", outcome),
		)
		if operationCounter != nil {
			operationCounter.Add(ctx, 1, metricAttrs)
		}
		if operationDuration != nil {
			operationDuration.Record(ctx, time.Since(startedAt).Seconds(), metricAttrs)
		}
		span.End()
	}
}

func RecordError(ctx context.Context, eventName string, err error, attrs ...slog.Attr) {
	if err == nil {
		return
	}
	LogError(ctx, "operation failed", eventName, eventName, "operation.failed", err, attrs...)
}

func ErrorType(err error) string {
	if err == nil {
		return ""
	}
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			break
		}
		err = unwrapped
	}
	return fmt.Sprintf("%T", err)
}

func ErrorOutcome(err error) string {
	if err == nil {
		return "succeeded"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "failed"
}
