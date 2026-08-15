package transferjob

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ContextWithRemoteTrace accepts only W3C traceparent/tracestate. Baggage,
// cookies, authorization and request content are intentionally not available
// to a Transfer Job.
func ContextWithRemoteTrace(parent context.Context, traceparent, tracestate string) (context.Context, error) {
	traceparent = strings.TrimSpace(traceparent)
	tracestate = strings.TrimSpace(tracestate)
	if traceparent == "" {
		if tracestate != "" {
			return nil, invalidConfig("tracestate without traceparent")
		}
		return parent, nil
	}
	if len(traceparent) > 128 || len(tracestate) > 512 || strings.ContainsAny(traceparent+tracestate, "\r\n") {
		return nil, invalidConfig("trace context")
	}
	carrier := propagation.MapCarrier{"traceparent": traceparent}
	if tracestate != "" {
		carrier["tracestate"] = tracestate
	}
	ctx := propagation.TraceContext{}.Extract(parent, carrier)
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() || !spanContext.IsRemote() {
		return nil, invalidConfig("trace context")
	}
	return ctx, nil
}
