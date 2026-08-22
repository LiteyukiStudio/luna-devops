package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestLogFormatResolution(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		terminal  bool
		container bool
		want      logFormat
	}{
		{name: "explicit console", value: "console", want: logFormatConsole},
		{name: "explicit json", value: "json", terminal: true, want: logFormatJSON},
		{name: "auto tty", value: "auto", terminal: true, want: logFormatConsole},
		{name: "auto redirected", value: "auto", want: logFormatJSON},
		{name: "auto container", value: "auto", terminal: true, container: true, want: logFormatJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveLogFormat(test.value, test.terminal, test.container); got != test.want {
				t.Fatalf("resolveLogFormat() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConsoleColorAndNoColor(t *testing.T) {
	for _, test := range []struct {
		name    string
		color   string
		noColor bool
		hasANSI bool
	}{
		{name: "always", color: "always", hasANSI: true},
		{name: "never", color: "never"},
		{name: "no color wins", color: "always", noColor: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := newProcessLoggerWithOptions("test", nil, processLoggerOptions{
				Writer: &output, IsTerminal: true, Format: "console", Color: test.color, NoColor: test.noColor,
			})
			logger.Error("failed", "event.name", "test.failed")
			if got := strings.Contains(output.String(), "\x1b["); got != test.hasANSI {
				t.Fatalf("ANSI presence = %t, output = %q", got, output.String())
			}
		})
	}
}

func TestJSONNeverContainsANSIAndHonorsLevel(t *testing.T) {
	var output bytes.Buffer
	logger := newProcessLoggerWithOptions("test", nil, processLoggerOptions{
		Writer: &output, IsTerminal: true, Format: "json", Color: "always", Level: "warn",
	})
	logger.Info("hidden")
	logger.Warn("visible", "event.name", "test.visible")
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("JSON output contains ANSI: %q", output.String())
	}
	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "visible") {
		t.Fatalf("LOG_LEVEL was not applied: %q", output.String())
	}
}

func TestErrorAttrsKeepChainAndRedactOnlyCredentials(t *testing.T) {
	err := WrapError(
		"dependency.redis.unavailable",
		"start Redis or verify REDIS_ADDR",
		"connect Redis",
		errors.New("ping Redis: dial tcp localhost:6379: connect: connection refused; token=secret-value; file=/srv/luna/config.go"),
	)
	serialized := attrsString(ErrorAttrs(err, "fallback"))
	for _, expected := range []string{
		"dependency.redis.unavailable",
		"connect Redis: ping Redis: dial tcp localhost:6379: connect: connection refused",
		"/srv/luna/config.go",
		"start Redis or verify REDIS_ADDR",
	} {
		if !strings.Contains(serialized, expected) {
			t.Fatalf("diagnostic %q missing from %q", expected, serialized)
		}
	}
	if strings.Contains(serialized, "secret-value") || !strings.Contains(serialized, "[REDACTED]") {
		t.Fatalf("credential was not redacted: %q", serialized)
	}
}

func TestConsoleFanoutKeepsStructuredOTelRecord(t *testing.T) {
	var terminal bytes.Buffer
	var otel bytes.Buffer
	logger := newProcessLoggerWithOptions("test-service", slog.NewJSONHandler(&otel, nil), processLoggerOptions{
		Writer: &terminal, IsTerminal: true, Format: "console", Color: "always", Level: "debug",
	})
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx := ContextWithRequestID(context.Background(), "req_123")
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	}))
	err := WrapError("dependency.redis.unavailable", "start Redis", "connect Redis", errors.New("dial tcp localhost:6379: connection refused"))
	LogError(ctx, "API startup failed", "api.startup.failed", "api.startup", "server.startup.failed", err)

	if !strings.Contains(terminal.String(), "\x1b[31mERROR") || !strings.Contains(terminal.String(), "cause=") {
		t.Fatalf("console output is not readable: %q", terminal.String())
	}
	if strings.Contains(otel.String(), "\x1b[") {
		t.Fatalf("OTel record contains ANSI: %q", otel.String())
	}
	for _, field := range []string{"\"error.code\":\"dependency.redis.unavailable\"", "\"error.message\":\"connect Redis: dial tcp localhost:6379: connection refused\"", "\"request_id\":\"req_123\"", "\"trace_id\":\"01000000000000000000000000000000\""} {
		if !strings.Contains(otel.String(), field) {
			t.Fatalf("OTel record missing %s: %q", field, otel.String())
		}
	}
}

func attrsString(attrs []slog.Attr) string {
	var result strings.Builder
	for _, attr := range attrs {
		result.WriteString(attr.Key)
		result.WriteByte('=')
		result.WriteString(attr.Value.String())
		result.WriteByte('\n')
	}
	return result.String()
}
