package main

import (
	"log"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/secret"
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

	options := worker.Options{
		DeployRolloutTimeoutSeconds: cfg.DeployRolloutTimeoutSeconds,
		CertManagerClusterIssuer:    cfg.CertManagerClusterIssuer,
	}
	if err := worker.Run(cfg.RedisAddr, db, options); err != nil {
		log.Fatalf("run worker: %v", err)
	}
}
