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

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	metricsConfig := observability.MetricsConfig{
		Enabled: cfg.MetricsEnabled,
		Addr:    cfg.MetricsAddr,
		Path:    cfg.MetricsPath,
		Service: "worker",
	}
	var workerMetrics *observability.WorkerMetrics
	if metricsConfig.Active() {
		metricsRegistry := observability.NewRegistry("worker")
		metricsServer, err := observability.StartMetricsServer(metricsConfig, metricsRegistry)
		if err != nil {
			log.Fatalf("start worker metrics server: %v", err)
		}
		defer observability.ShutdownMetricsServer(shutdownContext(), metricsServer)
		workerMetrics = observability.NewWorkerMetrics(metricsRegistry, "worker").WithQueueResolver(func(taskType string) string {
			return tasks.PolicyForType(taskType).Queue
		})
	} else if cfg.MetricsEnabled {
		log.Println("worker metrics disabled: METRICS_ADDR is empty")
	}

	options := worker.Options{
		DeployRolloutTimeoutSeconds: cfg.DeployRolloutTimeoutSeconds,
		CertManagerClusterIssuer:    cfg.CertManagerClusterIssuer,
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
	if err := worker.Run(cfg.RedisAddr, db, options); err != nil {
		log.Fatalf("run worker: %v", err)
	}
}

func shutdownContext() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}
