package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/term"
)

type requestIDKey struct{}

type logFormat string

const (
	logFormatAuto    logFormat = "auto"
	logFormatConsole logFormat = "console"
	logFormatJSON    logFormat = "json"
)

type logColor string

const (
	logColorAuto   logColor = "auto"
	logColorAlways logColor = "always"
	logColorNever  logColor = "never"
)

type processLoggerOptions struct {
	Writer      io.Writer
	IsTerminal  bool
	IsContainer bool
	Format      string
	Color       string
	Level       string
	NoColor     bool
}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if strings.TrimSpace(requestID) == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func Logger() *slog.Logger { return slog.Default() }

func newProcessLogger(serviceName string, otelHandler slog.Handler) *slog.Logger {
	return newProcessLoggerWithOptions(serviceName, otelHandler, processLoggerOptions{
		Writer:      os.Stderr,
		IsTerminal:  term.IsTerminal(int(os.Stderr.Fd())),
		IsContainer: runningInContainer(),
		Format:      os.Getenv("LOG_FORMAT"),
		Color:       os.Getenv("LOG_COLOR"),
		Level:       os.Getenv("LOG_LEVEL"),
		NoColor:     envPresent("NO_COLOR"),
	})
}

func newProcessLoggerWithOptions(serviceName string, otelHandler slog.Handler, options processLoggerOptions) *slog.Logger {
	if options.Writer == nil {
		options.Writer = io.Discard
	}
	level := parseLogLevel(options.Level)
	format := resolveLogFormat(options.Format, options.IsTerminal, options.IsContainer)
	var terminalHandler slog.Handler
	if format == logFormatConsole {
		terminalHandler = newConsoleHandler(options.Writer, level, resolveLogColor(options.Color, options.IsTerminal, options.NoColor))
	} else {
		terminalHandler = slog.NewJSONHandler(options.Writer, &slog.HandlerOptions{Level: level})
	}
	handlers := []slog.Handler{terminalHandler}
	if otelHandler != nil {
		handlers = append(handlers, otelHandler)
	}
	// Correlation and credential redaction happen before fan-out. The terminal
	// renderer and OTel therefore receive the same structured, ANSI-free Record.
	handler := &contextHandler{next: &fanoutHandler{handlers: handlers}}
	return slog.New(handler).With("service.name", serviceName)
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func resolveLogFormat(value string, isTerminal, isContainer bool) logFormat {
	switch logFormat(strings.ToLower(strings.TrimSpace(value))) {
	case logFormatConsole:
		return logFormatConsole
	case logFormatJSON:
		return logFormatJSON
	default:
		if isTerminal && !isContainer {
			return logFormatConsole
		}
		return logFormatJSON
	}
}

func resolveLogColor(value string, isTerminal, noColor bool) bool {
	if noColor {
		return false
	}
	switch logColor(strings.ToLower(strings.TrimSpace(value))) {
	case logColorAlways:
		return true
	case logColorNever:
		return false
	default:
		return isTerminal
	}
}

func envPresent(name string) bool {
	_, ok := os.LookupEnv(name)
	return ok
}

func runningInContainer() bool {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" || os.Getenv("container") != "" {
		return true
	}
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

type contextHandler struct{ next slog.Handler }

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		record.AddAttrs(slog.String("request_id", requestID))
	}
	return h.next.Handle(ctx, sanitizeRecord(record))
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{next: h.next.WithAttrs(sanitizeAttrs(attrs))}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{next: h.next.WithGroup(name)}
}

type fanoutHandler struct{ handlers []slog.Handler }

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var result error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record.Clone()); err != nil {
				result = err
			}
		}
	}
	return result
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return &fanoutHandler{handlers: handlers}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return &fanoutHandler{handlers: handlers}
}

type consoleHandler struct {
	writer io.Writer
	level  slog.Leveler
	color  bool
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func newConsoleHandler(writer io.Writer, level slog.Leveler, color bool) slog.Handler {
	return &consoleHandler{writer: writer, level: level, color: color, mu: &sync.Mutex{}}
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *consoleHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := append([]slog.Attr{}, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	fields := flattenAttrs(h.groups, attrs)
	sort.SliceStable(fields, func(i, j int) bool {
		return consoleFieldOrder(fields[i].Key) < consoleFieldOrder(fields[j].Key)
	})
	level := strings.ToUpper(record.Level.String())
	if h.color {
		level = consoleLevelColor(record.Level) + level + "\x1b[0m"
	}
	var output strings.Builder
	output.WriteString(level)
	output.WriteByte(' ')
	output.WriteString(consoleMessage(record.Message))
	for _, field := range fields {
		output.WriteByte(' ')
		output.WriteString(consoleFieldName(field.Key))
		output.WriteByte('=')
		output.WriteString(consoleValue(field.Value))
	}
	output.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.writer, output.String())
	return err
}

func consoleMessage(message string) string {
	return strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(message)
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func flattenAttrs(groups []string, attrs []slog.Attr) []slog.Attr {
	result := make([]slog.Attr, 0, len(attrs))
	var visit func([]string, slog.Attr)
	visit = func(prefix []string, attr slog.Attr) {
		attr.Value = attr.Value.Resolve()
		if attr.Equal(slog.Attr{}) {
			return
		}
		name := strings.Join(append(prefix, attr.Key), ".")
		if attr.Value.Kind() == slog.KindGroup {
			for _, child := range attr.Value.Group() {
				visit(append(prefix, attr.Key), child)
			}
			return
		}
		result = append(result, slog.Any(name, attr.Value.Any()))
	}
	for _, attr := range attrs {
		visit(groups, attr)
	}
	return result
}

func consoleFieldOrder(key string) int {
	switch key {
	case "error.code":
		return 0
	case "error.type":
		return 1
	case "error.message":
		return 2
	case "error.hint":
		return 3
	case "trace_id":
		return 4
	case "span_id":
		return 5
	case "request_id":
		return 6
	case "event.name":
		return 98
	case "service.name":
		return 99
	default:
		return 20
	}
}

func consoleFieldName(key string) string {
	switch key {
	case "error.code":
		return "code"
	case "error.message":
		return "cause"
	case "error.hint":
		return "hint"
	default:
		return key
	}
}

func consoleValue(value slog.Value) string {
	resolved := value.Resolve()
	switch resolved.Kind() {
	case slog.KindString:
		return fmt.Sprintf("%q", resolved.String())
	case slog.KindBool:
		return fmt.Sprintf("%t", resolved.Bool())
	case slog.KindInt64:
		return fmt.Sprintf("%d", resolved.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", resolved.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", resolved.Float64())
	case slog.KindDuration:
		return resolved.Duration().String()
	case slog.KindTime:
		return resolved.Time().Format("2006-01-02T15:04:05.000Z07:00")
	default:
		encoded, err := json.Marshal(resolved.Any())
		if err != nil {
			return fmt.Sprintf("%q", fmt.Sprint(resolved.Any()))
		}
		return string(encoded)
	}
}

func consoleLevelColor(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "\x1b[31m"
	case level >= slog.LevelWarn:
		return "\x1b[33m"
	case level <= slog.LevelDebug:
		return "\x1b[36m"
	default:
		return "\x1b[32m"
	}
}

var (
	bearerCredentialPattern = regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]+`)
	credentialPattern       = regexp.MustCompile(`(?i)\b(authorization|cookie|set-cookie|password|passwd|secret|token|client_secret|api[-_]?key|access_token|refresh_token|private_key)(\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;]+)`)
	urlCredentialPattern    = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^\s/@:]+:[^\s/@]+@`)
	privateKeyPattern       = regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----.*?-----END (?:[A-Z ]+ )?PRIVATE KEY-----`)
)

// RedactText removes credential values while retaining addresses, paths,
// resource identifiers and the rest of the diagnostic error chain.
func RedactText(value string) string {
	value = bearerCredentialPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = credentialPattern.ReplaceAllString(value, `${1}${2}[REDACTED]`)
	value = urlCredentialPattern.ReplaceAllString(value, `${1}[REDACTED]@`)
	return privateKeyPattern.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
}

func sanitizeRecord(record slog.Record) slog.Record {
	clone := slog.NewRecord(record.Time, record.Level, RedactText(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clone.AddAttrs(sanitizeAttr(attr))
		return true
	})
	return clone
}

func sanitizeAttrs(attrs []slog.Attr) []slog.Attr {
	result := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		result = append(result, sanitizeAttr(attr))
	}
	return result
}

func sanitizeAttr(attr slog.Attr) slog.Attr {
	value := attr.Value.Resolve()
	if isCredentialKey(attr.Key) {
		return slog.String(attr.Key, "[REDACTED]")
	}
	switch value.Kind() {
	case slog.KindString:
		return slog.String(attr.Key, RedactText(value.String()))
	case slog.KindGroup:
		return slog.Group(attr.Key, attrsToAny(sanitizeAttrs(value.Group()))...)
	default:
		return slog.Any(attr.Key, value.Any())
	}
}

func attrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for index := range attrs {
		result[index] = attrs[index]
	}
	return result
}

func isCredentialKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(strings.TrimSpace(key)))
	switch normalized {
	case "authorization", "cookie", "set_cookie", "password", "passwd", "secret", "client_secret", "api_key", "apikey", "access_token", "refresh_token", "private_key", "kubeconfig":
		return true
	default:
		return false
	}
}
