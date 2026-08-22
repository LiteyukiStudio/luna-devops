package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type diagnosticError struct {
	code string
	hint string
	err  error
}

func (err *diagnosticError) Error() string     { return err.err.Error() }
func (err *diagnosticError) Unwrap() error     { return err.err }
func (err *diagnosticError) ErrorCode() string { return err.code }
func (err *diagnosticError) ErrorHint() string { return err.hint }

// WrapError attaches stable boundary metadata without replacing the original
// error chain. It is intentionally small and is not a domain error framework.
func WrapError(code, hint, operation string, err error) error {
	if err == nil {
		return nil
	}
	operation = strings.TrimSpace(operation)
	if operation != "" {
		err = fmt.Errorf("%s: %w", operation, err)
	}
	return &diagnosticError{code: strings.TrimSpace(code), hint: strings.TrimSpace(hint), err: err}
}

func ErrorCode(err error, fallback string) string {
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) && strings.TrimSpace(coded.ErrorCode()) != "" {
		return coded.ErrorCode()
	}
	return fallback
}

func ErrorHint(err error) string {
	var hinted interface{ ErrorHint() string }
	if errors.As(err, &hinted) {
		return strings.TrimSpace(hinted.ErrorHint())
	}
	return ""
}

func ErrorAttrs(err error, fallbackCode string) []slog.Attr {
	if err == nil {
		return nil
	}
	attrs := []slog.Attr{
		slog.String("error.code", ErrorCode(err, fallbackCode)),
		slog.String("error.type", ErrorType(err)),
		slog.String("error.message", RedactText(err.Error())),
	}
	if hint := ErrorHint(err); hint != "" {
		attrs = append(attrs, slog.String("error.hint", hint))
	}
	return attrs
}

func LogError(ctx context.Context, message, eventName, operation, fallbackCode string, err error, attrs ...slog.Attr) {
	logFailure(ctx, slog.LevelError, message, eventName, operation, fallbackCode, err, attrs...)
}

func LogWarn(ctx context.Context, message, eventName, operation, fallbackCode string, err error, attrs ...slog.Attr) {
	logFailure(ctx, slog.LevelWarn, message, eventName, operation, fallbackCode, err, attrs...)
}

func logFailure(ctx context.Context, level slog.Level, message, eventName, operation, fallbackCode string, err error, attrs ...slog.Attr) {
	if err == nil {
		return
	}
	fields := []slog.Attr{
		slog.String("event.name", eventName),
		slog.String("operation", operation),
		slog.String("outcome", ErrorOutcome(err)),
	}
	fields = append(fields, ErrorAttrs(err, fallbackCode)...)
	fields = append(fields, attrs...)
	RecordSpanError(ctx, err, ErrorCode(err, fallbackCode))
	Logger().LogAttrs(ctx, level, message, fields...)
}

// RecordSpanError records the redacted diagnostic chain without emitting a
// second log at a lower-level boundary.
func RecordSpanError(ctx context.Context, err error, fallbackCode string) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	code := ErrorCode(err, fallbackCode)
	span.AddEvent("operation.error", trace.WithAttributes(
		attribute.String("error.code", code),
		attribute.String("error.type", ErrorType(err)),
		attribute.String("error.message", RedactText(err.Error())),
	))
	span.SetStatus(codes.Error, code)
}
