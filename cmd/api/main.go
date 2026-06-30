package main

import (
	"context"
	"log"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/observability"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/webui"
)

func main() {
	cfg := config.Load()
	if err := secret.ValidateEncryptionConfig(); err != nil {
		log.Fatalf("%v; set SECRET_ENCRYPTION_KEY or run local development with APP_ENV=development", err)
	}

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	metricsConfig := observability.MetricsConfig{
		Enabled: cfg.MetricsEnabled,
		Addr:    cfg.MetricsAddr,
		Path:    cfg.MetricsPath,
		Service: "api",
	}
	var httpMetrics *observability.HTTPMetrics
	if metricsConfig.Active() {
		metricsRegistry := observability.NewRegistry("api")
		metricsServer, err := observability.StartMetricsServer(metricsConfig, metricsRegistry)
		if err != nil {
			log.Fatalf("start api metrics server: %v", err)
		}
		defer observability.ShutdownMetricsServer(shutdownContext(), metricsServer)
		httpMetrics = observability.NewHTTPMetrics(metricsRegistry, "api")
	} else if cfg.MetricsEnabled {
		log.Println("api metrics disabled: METRICS_ADDR is empty")
	}

	router := api.NewRouterWithStaticFSAndMetrics(db, webui.FS, httpMetrics)

	log.Printf("api listening on %s", cfg.APIAddr)
	if err := router.Run(cfg.APIAddr); err != nil {
		log.Fatalf("run api: %v", err)
	}
}

func shutdownContext() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	return ctx
}
