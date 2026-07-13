package main

import (
	"context"
	"log"
	"time"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/observability"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/worker"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	if err := secret.ValidateEncryptionConfig(); err != nil {
		log.Fatalf("%v; set SECRET_ENCRYPTION_KEY or run local development with APP_ENV=development", err)
	}

	if cfg.RedisAddr == "" {
		log.Println("worker idle: REDIS_ADDR is empty")
		select {}
	}

	db, err := database.Open(cfg.DatabaseURL, database.Options{
		MaxOpenConns:         cfg.DatabaseMaxOpenConns,
		MaxIdleConns:         cfg.DatabaseMaxIdleConns,
		ConnMaxLifetime:      cfg.DatabaseConnMaxLifetime,
		ConnMaxIdleTime:      cfg.DatabaseConnMaxIdleTime,
		ConnectRetryAttempts: cfg.DatabaseConnectRetryAttempts,
		ConnectRetryInterval: cfg.DatabaseConnectRetryInterval,
	})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	metricsConfig := observability.MetricsConfig{
		Enabled: cfg.MetricsEnabled,
		Addr:    cfg.MetricsAddr,
		Path:    cfg.MetricsPath,
		Service: "worker",
	}.WithDefaultAddr(":9091")
	var workerMetrics *observability.WorkerMetrics
	if metricsConfig.Active() {
		metricsRegistry := observability.NewRegistry("worker")
		sqlDB, err := db.DB()
		if err != nil {
			log.Fatalf("open database metrics handle: %v", err)
		}
		observability.RegisterDBStats(metricsRegistry, sqlDB, "postgres")
		redisOptions := cfg.RedisOptions()
		redisClient := redis.NewClient(redisOptions.GoRedis())
		defer redisClient.Close()
		metricsRegistry.MustRegister(observability.NewDependencyCollector("worker", map[string]observability.DependencyCheck{
			"postgres": sqlDB.PingContext,
			"redis": func(ctx context.Context) error {
				return redisClient.Ping(ctx).Err()
			},
		}))
		queueInspector := asynq.NewInspector(redisOptions.Asynq())
		defer queueInspector.Close()
		metricsRegistry.MustRegister(observability.NewAsynqQueueCollector("worker", queueInspector, []string{
			tasks.QueueBuild,
			tasks.QueueDeploy,
			tasks.QueueLight,
		}))
		metricsServer, err := observability.StartMetricsServer(metricsConfig, metricsRegistry)
		if err != nil {
			log.Fatalf("start worker metrics server: %v", err)
		}
		defer func() {
			ctx, cancel := shutdownContext()
			defer cancel()
			observability.ShutdownMetricsServer(ctx, metricsServer)
		}()
		workerMetrics = observability.NewWorkerMetrics(metricsRegistry, "worker").WithQueueResolver(func(taskType string) string {
			return tasks.PolicyForType(taskType).Queue
		})
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
	if err := worker.RunWithRedis(cfg.RedisOptions(), db, options); err != nil {
		log.Fatalf("run worker: %v", err)
	}
}

func shutdownContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}
