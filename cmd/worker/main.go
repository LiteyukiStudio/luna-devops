package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
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
	sharedconfig.LoadEnvironment()
	runtime, err := telemetry.Setup(ctx, telemetry.ServiceConfig{ServiceName: "luna-worker"})
	if err != nil {
		telemetry.LogError(ctx, "Worker startup failed", "worker.startup.failed", "worker.startup",
			"telemetry.initialization.failed",
			telemetry.WrapError("telemetry.initialization.failed", "verify the OTEL exporter configuration", "initialize telemetry", err))
		return 1
	}
	defer func() {
		shutdownCtx, cancel := shutdownContext()
		defer cancel()
		if err := runtime.Shutdown(shutdownCtx); err != nil {
			telemetry.LogError(shutdownCtx, "Worker telemetry shutdown failed", "telemetry.shutdown.failed",
				"telemetry.shutdown", "telemetry.shutdown.failed", err)
		}
	}()

	if err := run(ctx); err != nil {
		telemetry.LogError(ctx, "Worker startup failed", "worker.startup.failed", "worker.startup", "worker.startup.failed", err)
		return 1
	}
	return 0
}

func run(ctx context.Context) error {
	cfg, err := worker.LoadConfig()
	if err != nil {
		return telemetry.WrapError("config.invalid", "verify Worker and shared environment variables", "load Worker configuration", err)
	}
	if err := secret.ValidateEncryptionConfig(); err != nil {
		return telemetry.WrapError("config.invalid", "set SECRET_ENCRYPTION_KEY or use APP_ENV=development locally", "validate encryption configuration", err)
	}

	if err := redisconfig.CheckConnection(ctx, cfg.RedisOptions()); err != nil {
		return telemetry.WrapError("dependency.redis.unavailable", "start Redis or verify REDIS_ADDR", "connect Redis", err)
	}

	db, err := database.OpenContext(ctx, cfg.DatabaseURL, database.Options{
		MaxOpenConns:    cfg.DatabaseMaxOpenConns,
		MaxIdleConns:    cfg.DatabaseMaxIdleConns,
		ConnMaxLifetime: cfg.DatabaseConnMaxLifetime,
		ConnMaxIdleTime: cfg.DatabaseConnMaxIdleTime,
	})
	if err != nil {
		return telemetry.WrapError("dependency.postgres.unavailable", "start PostgreSQL or verify DATABASE_URL", "connect PostgreSQL", err)
	}
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		defer sqlDB.Close()
		registration, registerErr := telemetry.RegisterDBPoolMetrics(sqlDB, "postgres")
		if registerErr != nil {
			return telemetry.WrapError("telemetry.initialization.failed", "verify the database telemetry configuration", "register database pool metrics", registerErr)
		}
		defer registration.Unregister()
	}

	workerMetrics := observability.NewWorkerMetrics().WithQueueResolver(func(taskType string) string {
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
		return telemetry.WrapError("dependency.redis.unavailable", "start Redis or verify REDIS_ADDR", "register worker queue metrics", err)
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
		BuildEgressMode:             cfg.BuildEgressMode,
		BuildCacheEnabled:           cfg.BuildCacheEnabled,
		BuildCacheTag:               cfg.BuildCacheTag,
		BuildJobTimeoutSeconds:      cfg.BuildJobTimeoutSeconds,
		BuildJobTTLSeconds:          cfg.BuildJobTTLSeconds,
		BuildPrivateEgressCIDRs:     cfg.BuildPrivateEgressCIDRs,
		BuildPrivateEgressPorts:     cfg.BuildPrivateEgressPorts,
		BuildBlockedEgressCIDRs:     cfg.BuildBlockedEgressCIDRs,
		VolumeTransferJobImage:      cfg.VolumeTransferJobImage,
		VolumeTransferMaxBytes:      cfg.VolumeTransferMaxBytes,
	}
	telemetry.Logger().InfoContext(ctx, "worker service starting",
		slog.String("event.name", "worker.starting"),
	)
	if err := worker.RunWithRedis(cfg.RedisOptions(), db, options); err != nil {
		return telemetry.WrapError("worker.startup.failed", "verify Redis, PostgreSQL and Worker configuration", "run worker", err)
	}
	return nil
}

func shutdownContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
