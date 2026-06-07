package main

import (
	"log"

	"github.com/LiteyukiStudio/devops/internal/api"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/secret"
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

	router := api.NewRouter(db)

	log.Printf("api listening on %s", cfg.APIAddr)
	if err := router.Run(cfg.APIAddr); err != nil {
		log.Fatalf("run api: %v", err)
	}
}
