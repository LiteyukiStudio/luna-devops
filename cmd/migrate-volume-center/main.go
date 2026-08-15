package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volumemigration"
)

type commandOptions struct {
	Apply              bool
	PageSize           int
	ProjectID          string
	ObservationTimeout time.Duration
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runtime, err := telemetry.Setup(ctx, telemetry.ServiceConfig{ServiceName: "luna-volume-center-migration"})
	if err != nil {
		_, _ = os.Stderr.WriteString("volume center migration telemetry initialization failed\n")
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runtime.Shutdown(shutdownCtx)
	}()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		telemetry.Logger().ErrorContext(ctx, "volume center migration failed",
			slog.String("event.name", "volume_center_migration.failed"),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	options, err := parseCommandOptions(args)
	if err != nil {
		return err
	}
	config.LoadEnvironment()
	cfg := config.Load()
	db, err := database.OpenContext(ctx, cfg.DatabaseURL, database.Options{
		MaxOpenConns: cfg.DatabaseMaxOpenConns, MaxIdleConns: cfg.DatabaseMaxIdleConns,
		ConnMaxLifetime: cfg.DatabaseConnMaxLifetime, ConnMaxIdleTime: cfg.DatabaseConnMaxIdleTime,
	})
	if err != nil {
		return errors.New("volume center migration database unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.New("volume center migration database handle unavailable")
	}
	defer sqlDB.Close()
	secretStore := secret.NewStore(db, nil)
	inspector := volumemigration.NewKubernetesInspector(db, secretStore, options.ObservationTimeout)
	service := volumemigration.NewService(volumemigration.NewGormRepository(db), inspector)
	report, err := service.Run(ctx, volumemigration.Options{
		Apply: options.Apply, PageSize: options.PageSize, ProjectID: options.ProjectID,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func parseCommandOptions(args []string) (commandOptions, error) {
	options := commandOptions{PageSize: volumemigration.DefaultPageSize, ObservationTimeout: 8 * time.Second}
	flags := flag.NewFlagSet("migrate-volume-center", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.Apply, "apply", false, "persist verified backfill records; defaults to dry-run")
	flags.IntVar(&options.PageSize, "page-size", volumemigration.DefaultPageSize, "source page size (1-100)")
	flags.StringVar(&options.ProjectID, "project-id", "", "limit the scan to one project id")
	flags.DurationVar(&options.ObservationTimeout, "observation-timeout", 8*time.Second, "per Kubernetes observation timeout")
	if err := flags.Parse(args); err != nil {
		return commandOptions{}, fmt.Errorf("parse migrate-volume-center flags: %w", err)
	}
	if flags.NArg() != 0 || options.PageSize < 1 || options.PageSize > volumemigration.MaxPageSize || options.ObservationTimeout <= 0 {
		return commandOptions{}, volumemigration.ErrInvalidOptions
	}
	options.ProjectID = strings.TrimSpace(options.ProjectID)
	return options, nil
}
