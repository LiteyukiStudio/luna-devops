package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/observability"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/worker"
	"github.com/hibiken/asynq"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx := context.Background()
	config.LoadEnvironment()
	runtime, err := telemetry.Setup(ctx, telemetry.ServiceConfig{ServiceName: "luna-worker"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize telemetry: %v\n", err)
		return 1
	}
	defer func() {
		shutdownCtx, cancel := shutdownContext()
		defer cancel()
		if err := runtime.Shutdown(shutdownCtx); err != nil {
			telemetry.Logger().ErrorContext(shutdownCtx, "telemetry shutdown failed",
				slog.String("event.name", "telemetry.shutdown.failed"),
				slog.String("error.type", telemetry.ErrorType(err)),
			)
		}
	}()

	if err := run(ctx); err != nil {
		telemetry.RecordError(ctx, "worker.run.failed", err)
		return 1
	}
	return 0
}

func run(ctx context.Context) error {
	cfg := config.Load()
	if err := cfg.ValidateRedis(); err != nil {
		return err
	}
	if err := secret.ValidateEncryptionConfig(); err != nil {
		return fmt.Errorf("%w; set SECRET_ENCRYPTION_KEY or run local development with APP_ENV=development", err)
	}

	if err := redisconfig.CheckConnection(ctx, cfg.RedisOptions()); err != nil {
		return fmt.Errorf("connect Redis: %w", err)
	}

	db, err := database.OpenContext(ctx, cfg.DatabaseURL, database.Options{
		MaxOpenConns:    cfg.DatabaseMaxOpenConns,
		MaxIdleConns:    cfg.DatabaseMaxIdleConns,
		ConnMaxLifetime: cfg.DatabaseConnMaxLifetime,
		ConnMaxIdleTime: cfg.DatabaseConnMaxIdleTime,
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		defer sqlDB.Close()
		registration, registerErr := telemetry.RegisterDBPoolMetrics(sqlDB, "postgres")
		if registerErr != nil {
			return fmt.Errorf("register database pool metrics: %w", registerErr)
		}
		defer registration.Unregister()
	}

	workerMetrics := observability.NewWorkerMetrics(nil, "worker").WithQueueResolver(func(taskType string) string {
		return tasks.PolicyForType(taskType).Queue
	})
	queueInspector := asynq.NewInspector(cfg.RedisOptions().Asynq())
	defer queueInspector.Close()
	queueRegistration, err := observability.RegisterAsynqQueueTelemetry(queueInspector, []string{
		tasks.QueueBuild,
		tasks.QueueDeploy,
		tasks.QueueLight,
	})
	if err != nil {
		return fmt.Errorf("register worker queue metrics: %w", err)
	}
	if queueRegistration != nil {
		defer queueRegistration.Unregister()
	}

	options := worker.Options{
		DeployRolloutTimeoutSeconds: cfg.DeployRolloutTimeoutSeconds,
		CertManagerClusterIssuer:    cfg.CertManagerClusterIssuer,
		PublicBaseURL:               cfg.PublicBaseURL,
		WorkerMetrics:               workerMetrics,
		BuildExecutorImage:          cfg.BuildExecutorImage,
		BuildNPMRegistry:            cfg.BuildNPMRegistry,
		BuildEgressMode:             cfg.BuildEgressMode,
		BuildCacheEnabled:           cfg.BuildCacheEnabled,
		BuildCacheTag:               cfg.BuildCacheTag,
		BuildJobTimeoutSeconds:      cfg.BuildJobTimeoutSeconds,
		BuildJobTTLSeconds:          cfg.BuildJobTTLSeconds,
		BuildPrivateEgressCIDRs:     cfg.BuildPrivateEgressCIDRs,
		BuildPrivateEgressPorts:     cfg.BuildPrivateEgressPorts,
		BuildBlockedEgressCIDRs:     cfg.BuildBlockedEgressCIDRs,
	}
	telemetry.Logger().InfoContext(ctx, "worker service starting",
		slog.String("event.name", "worker.starting"),
	)
	if err := worker.RunWithRedis(cfg.RedisOptions(), db, options); err != nil {
		return fmt.Errorf("run worker: %w", err)
	}
	return nil
}

func shutdownContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
