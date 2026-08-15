package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/transferjob"
)

func main() {
	os.Exit(run())
}

func run() int {
	startupCtx := context.Background()
	runtime, err := telemetry.Setup(startupCtx, telemetry.ServiceConfig{ServiceName: "luna-volume-transfer"})
	if err != nil {
		_, _ = os.Stderr.WriteString("initialize volume transfer telemetry failed\n")
		return 1
	}
	defer func() {
		// Process shutdown is independent of a cancelled Job operation and is
		// therefore intentionally bounded from a fresh lifecycle context.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runtime.Shutdown(shutdownCtx)
	}()
	logger := telemetry.Logger()

	config, err := transferjob.ConfigFromEnv()
	if err != nil {
		logger.Error("volume transfer configuration rejected",
			slog.String("event.name", "volume_transfer.config.invalid"),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		return 2
	}
	jobCtx, err := transferjob.ContextWithRemoteTrace(startupCtx, config.Traceparent, config.Tracestate)
	if err != nil {
		logger.Error("volume transfer trace context rejected",
			slog.String("event.name", "volume_transfer.trace.invalid"),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		return 2
	}
	jobCtx, stop := signal.NotifyContext(jobCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runner, err := transferjob.NewRunner(config, nil)
	if err != nil {
		logger.Error("volume transfer client initialization failed",
			slog.String("event.name", "volume_transfer.client.failed"),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		return 2
	}
	defer runner.Close()
	logger.InfoContext(jobCtx, "volume transfer started",
		slog.String("event.name", "volume_transfer.started"),
		slog.String("direction", config.Direction),
		slog.String("format", config.Format),
	)
	result, err := runner.Run(jobCtx)
	if err != nil {
		logger.ErrorContext(jobCtx, "volume transfer failed",
			slog.String("event.name", "volume_transfer.failed"),
			slog.String("direction", config.Direction),
			slog.String("format", config.Format),
			slog.String("error.code", transferjob.ErrorCode(err)),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		return 1
	}
	logger.InfoContext(jobCtx, "volume transfer completed",
		slog.String("event.name", "volume_transfer.completed"),
		slog.String("direction", config.Direction),
		slog.String("format", config.Format),
		slog.Int64("transferred_bytes", result.TransferredBytes),
		slog.Int64("processed_files", result.ProcessedFiles),
	)
	return 0
}
