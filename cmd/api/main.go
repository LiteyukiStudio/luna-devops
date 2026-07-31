package main

import (
	"context"
	"fmt"
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
	telemetryRuntime, err := telemetry.Setup(context.Background(), telemetry.ServiceConfig{ServiceName: "luna-devops-api"})
	if err != nil {
		slog.Error("initialize telemetry", "event.name", "telemetry.initialization.failed", "error.type", telemetry.ErrorType(err))
		return err
	}
	defer func() {
		if runErr != nil {
			slog.Error("API stopped", "event.name", "service.failed", "error.type", telemetry.ErrorType(runErr))
		}
		if err := telemetryRuntime.Shutdown(context.Background()); err != nil {
			slog.Error("shutdown telemetry", "event.name", "telemetry.shutdown.failed", "error.type", telemetry.ErrorType(err))
		}
	}()
	cfg := config.Load()
	if err := cfg.ValidateRedis(); err != nil {
		return fmt.Errorf("validate Redis configuration: %w", err)
	}
	if err := secret.ValidateEncryptionConfig(); err != nil {
		return fmt.Errorf("validate encryption configuration: %w", err)
	}
	if err := redisconfig.CheckConnection(context.Background(), cfg.RedisOptions()); err != nil {
		return fmt.Errorf("connect Redis: %w", err)
	}

	db, err := database.OpenContext(context.Background(), cfg.DatabaseURL, database.Options{
		MaxOpenConns:    cfg.DatabaseMaxOpenConns,
		MaxIdleConns:    cfg.DatabaseMaxIdleConns,
		ConnMaxLifetime: cfg.DatabaseConnMaxLifetime,
		ConnMaxIdleTime: cfg.DatabaseConnMaxIdleTime,
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if err := database.MigrateContext(context.Background(), db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("open database telemetry handle: %w", err)
	}
	dbMetricRegistration, err := telemetry.RegisterDBPoolMetrics(sqlDB, "postgres")
	if err != nil {
		return fmt.Errorf("register database telemetry: %w", err)
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
			if err := telemetry.InstrumentRedis(redisClient); err != nil {
				return fmt.Errorf("instrument Redis metrics client: %w", err)
			}
			defer redisClient.Close()
			dependencyChecks["redis"] = func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			}
		}
		metricsRegistry.MustRegister(observability.NewDependencyCollector("api", dependencyChecks))
		metricsServer, err := observability.StartMetricsServer(metricsConfig, metricsRegistry)
		if err != nil {
			return fmt.Errorf("start API metrics server: %w", err)
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
	if err := router.Run(cfg.APIAddr); err != nil {
		return fmt.Errorf("run API: %w", err)
	}
	return nil
}

func shutdownContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
