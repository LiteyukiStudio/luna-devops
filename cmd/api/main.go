package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/observability"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/webui"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() (runErr error) {
	config.LoadEnvironment()
	ctx := context.Background()
	telemetryRuntime, err := telemetry.Setup(ctx, telemetry.ServiceConfig{ServiceName: "luna-devops-api"})
	if err != nil {
		telemetry.LogError(ctx, "API startup failed", "api.startup.failed", "api.startup", "telemetry.initialization.failed",
			telemetry.WrapError("telemetry.initialization.failed", "verify the OTEL exporter configuration", "initialize telemetry", err))
		return err
	}
	defer func() {
		if runErr != nil {
			telemetry.LogError(ctx, "API startup failed", "api.startup.failed", "api.startup", "server.startup.failed", runErr)
		}
		if err := telemetryRuntime.Shutdown(context.Background()); err != nil {
			telemetry.LogError(ctx, "API telemetry shutdown failed", "telemetry.shutdown.failed", "telemetry.shutdown",
				"telemetry.shutdown.failed", err)
		}
	}()
	cfg := config.Load()
	if err := cfg.ValidateRedis(); err != nil {
		return telemetry.WrapError("config.invalid", "set REDIS_ADDR to a redis:// or rediss:// URI", "validate Redis configuration", err)
	}
	if err := cfg.ValidateVolumeTransfer(); err != nil {
		return telemetry.WrapError("config.invalid", "verify VOLUME_TRANSFER_MAX_BYTES and VOLUME_TRANSFER_JOB_IMAGE", "validate volume transfer configuration", err)
	}
	if err := secret.ValidateEncryptionConfig(); err != nil {
		return telemetry.WrapError("config.invalid", "set a stable SECRET_ENCRYPTION_KEY", "validate encryption configuration", err)
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

	if err := database.MigrateContext(ctx, db); err != nil {
		return telemetry.WrapError("database.migration.failed", "inspect migration state and PostgreSQL permissions", "migrate database", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return telemetry.WrapError("dependency.postgres.unavailable", "verify DATABASE_URL and PostgreSQL connectivity", "open database telemetry handle", err)
	}
	defer sqlDB.Close()
	dbMetricRegistration, err := telemetry.RegisterDBPoolMetrics(sqlDB, "postgres")
	if err != nil {
		return telemetry.WrapError("telemetry.initialization.failed", "verify the database telemetry configuration", "register database telemetry", err)
	}
	if dbMetricRegistration != nil {
		defer dbMetricRegistration.Unregister()
	}

	metricsConfig := observability.MetricsConfig{
		Enabled: cfg.MetricsEnabled,
		Addr:    cfg.MetricsAddr,
		Path:    cfg.MetricsPath,
		Service: "api",
	}.WithDefaultAddr(":9090")
	var httpMetrics *observability.HTTPMetrics
	if metricsConfig.Active() {
		metricsRegistry := observability.NewRegistry("api")
		observability.RegisterDBStats(metricsRegistry, sqlDB, "postgres")
		dependencyChecks := map[string]observability.DependencyCheck{
			"postgres": sqlDB.PingContext,
		}
		var redisClient *redis.Client
		if cfg.RedisAddr != "" {
			redisClient = redis.NewClient(cfg.RedisOptions().GoRedis())
			defer redisClient.Close()
			dependencyChecks["redis"] = func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			}
		}
		metricsRegistry.MustRegister(observability.NewDependencyCollector("api", dependencyChecks))
		metricsServer, err := observability.StartMetricsServer(metricsConfig, metricsRegistry)
		if err != nil {
			return telemetry.WrapError("server.listen.failed", "verify METRICS_ADDR is available", "listen on API metrics address", err)
		}
		defer func() {
			ctx, cancel := shutdownContext()
			defer cancel()
			observability.ShutdownMetricsServer(ctx, metricsServer)
		}()
		httpMetrics = observability.NewHTTPMetrics(metricsRegistry, "api")
	}

	router := api.NewRouterWithStaticFSAndMetrics(db, webui.FS, httpMetrics)

	slog.Info("API listening", "event.name", "service.started", "server.address", cfg.APIAddr, "telemetry.enabled", telemetryRuntime.Active())
	return runAPIServer(router, cfg.APIAddr)
}

type apiRouter interface {
	Run(...string) error
}

func runAPIServer(router apiRouter, address string) error {
	if err := router.Run(address); err != nil {
		return telemetry.WrapError("server.listen.failed", "verify API_ADDR is valid and the port is available", "listen on API address", err)
	}
	return nil
}

func shutdownContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
