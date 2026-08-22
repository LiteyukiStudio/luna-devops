package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
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
	ServiceName    string
	ServiceVersion string
}

// Runtime owns the process-wide OpenTelemetry providers. A runtime is active
// only when OTEL_EXPORTER_OTLP_ENDPOINT is explicitly configured.
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
	slog.SetDefault(newProcessLogger(serviceName, nil))

	if strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) == "" {
		return &Runtime{}, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcessExecutableName(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(strings.TrimSpace(config.ServiceVersion)),
		),
	)
	if err != nil {
		return nil, err
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, err
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	logExporter, err := otlploghttp.New(ctx)
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
	slog.SetDefault(newProcessLogger(serviceName, otelslog.NewHandler(serviceName,
		otelslog.WithLoggerProvider(loggerProvider),
	)))

	return &Runtime{
		active:         true,
		loggerProvider: loggerProvider,
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
	}, nil
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
