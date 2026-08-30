package telemetry

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const defaultShutdownTimeout = 5 * time.Second

type ServiceConfig struct {
	ServiceName        string
	ServiceVersion     string
	Endpoint           string
	Headers            map[string]string
	ResourceAttributes map[string]string
	LogFormat          string
	LogColor           string
	LogLevel           string
	NoColor            bool
}

// Runtime owns the process-wide OpenTelemetry providers. A runtime is active
// only when the startup configuration contains an OTLP endpoint.
type Runtime struct {
	active         bool
	loggerProvider *sdklog.LoggerProvider
	meterProvider  *sdkmetric.MeterProvider
	tracerProvider *sdktrace.TracerProvider
}

func (r *Runtime) Active() bool { return r != nil && r.active }

func Setup(ctx context.Context, config ServiceConfig) (*Runtime, error) {
	serviceName := strings.TrimSpace(config.ServiceName)
	if serviceName == "" {
		return nil, errors.New("telemetry service name is required")
	}

	otel.SetTextMapPropagator(defaultPropagator())
	// Configure stderr rendering before exporter setup so exporter/bootstrap
	// failures use the same diagnostic contract.
	loggerOptions := processLoggerOptions{
		Writer: osStderr(), IsTerminal: stderrIsTerminal(), IsContainer: runningInContainer(),
		Format: config.LogFormat, Color: config.LogColor, Level: config.LogLevel, NoColor: config.NoColor,
	}
	slog.SetDefault(newProcessLoggerWithOptions(serviceName, nil, loggerOptions))

	endpoint := strings.TrimRight(strings.TrimSpace(config.Endpoint), "/")
	if endpoint == "" {
		return &Runtime{}, nil
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Host == "" || (parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") || parsedEndpoint.User != nil || parsedEndpoint.RawQuery != "" || parsedEndpoint.Fragment != "" {
		return nil, errors.New("telemetry endpoint must be an absolute http or https URL without credentials, query parameters, or fragments")
	}

	resourceAttributes := make([]attribute.KeyValue, 0, len(config.ResourceAttributes))
	attributeNames := make([]string, 0, len(config.ResourceAttributes))
	for name := range config.ResourceAttributes {
		attributeNames = append(attributeNames, name)
	}
	sort.Strings(attributeNames)
	for _, name := range attributeNames {
		resourceAttributes = append(resourceAttributes, attribute.String(name, config.ResourceAttributes[name]))
	}
	res, err := resource.New(ctx,
		resource.WithProcessExecutableName(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			resourceAttributes...,
		),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(strings.TrimSpace(config.ServiceVersion)),
		),
	)
	if err != nil {
		return nil, err
	}

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(telemetrySignalEndpoint(endpoint, "v1/traces")),
		otlptracehttp.WithHeaders(cloneStringMap(config.Headers)),
		otlptracehttp.WithCompression(otlptracehttp.NoCompression),
		otlptracehttp.WithHTTPClient(newTelemetryHTTPClient()),
	)
	if err != nil {
		return nil, err
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(telemetrySignalEndpoint(endpoint, "v1/metrics")),
		otlpmetrichttp.WithHeaders(cloneStringMap(config.Headers)),
		otlpmetrichttp.WithCompression(otlpmetrichttp.NoCompression),
		otlpmetrichttp.WithHTTPClient(newTelemetryHTTPClient()),
		otlpmetrichttp.WithTemporalitySelector(sdkmetric.DefaultTemporalitySelector),
		otlpmetrichttp.WithAggregationSelector(sdkmetric.DefaultAggregationSelector),
	)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, err
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	logExporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(telemetrySignalEndpoint(endpoint, "v1/logs")),
		otlploghttp.WithHeaders(cloneStringMap(config.Headers)),
		otlploghttp.WithCompression(otlploghttp.NoCompression),
		otlploghttp.WithHTTPClient(newTelemetryHTTPClient()),
	)
	if err != nil {
		_ = meterProvider.Shutdown(ctx)
		_ = tracerProvider.Shutdown(ctx)
		return nil, err
	}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	logglobal.SetLoggerProvider(loggerProvider)
	slog.SetDefault(newProcessLoggerWithOptions(serviceName, otelslog.NewHandler(serviceName,
		otelslog.WithLoggerProvider(loggerProvider),
	), loggerOptions))

	return &Runtime{
		active:         true,
		loggerProvider: loggerProvider,
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
	}, nil
}

func newTelemetryHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

func telemetrySignalEndpoint(baseURL, signalPath string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	parsed.Path = path.Join(parsed.Path, signalPath)
	return parsed.String()
}

func cloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func defaultPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func (r *Runtime) Shutdown(parent context.Context) error {
	if !r.Active() {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, defaultShutdownTimeout)
	defer cancel()
	return errors.Join(
		r.loggerProvider.Shutdown(ctx),
		r.meterProvider.Shutdown(ctx),
		r.tracerProvider.Shutdown(ctx),
	)
}
