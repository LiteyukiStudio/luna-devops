package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"gorm.io/gorm"
)

const defaultPageSize = 100

type commandOptions struct {
	AcknowledgeSensitiveMetadata bool
	PageSize                     int
	ProjectID                    string
}

type auditFinding struct {
	ResourceType string   `json:"resourceType"`
	ResourceID   string   `json:"resourceId"`
	ProjectID    string   `json:"projectId"`
	Keys         []string `json:"keys"`
}

type auditReport struct {
	DeploymentTargetsScanned int            `json:"deploymentTargetsScanned"`
	RuntimeConfigSetsScanned int            `json:"runtimeConfigSetsScanned"`
	Findings                 []auditFinding `json:"findings"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = os.Stderr.WriteString("runtime environment audit failed\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	options, err := parseOptions(args)
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
		return errors.New("runtime environment audit database unavailable")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.New("runtime environment audit database handle unavailable")
	}
	defer sqlDB.Close()

	report, err := inspect(ctx, db, options)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func parseOptions(args []string) (commandOptions, error) {
	options := commandOptions{PageSize: defaultPageSize}
	flags := flag.NewFlagSet("audit-runtime-environment", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&options.AcknowledgeSensitiveMetadata, "acknowledge-sensitive-metadata", false, "confirm that output contains resource IDs and suspected secret key names")
	flags.IntVar(&options.PageSize, "page-size", defaultPageSize, "database scan page size (1-500)")
	flags.StringVar(&options.ProjectID, "project-id", "", "limit the read-only scan to one project space")
	if err := flags.Parse(args); err != nil {
		return commandOptions{}, fmt.Errorf("parse audit-runtime-environment flags: %w", err)
	}
	if flags.NArg() != 0 || options.PageSize < 1 || options.PageSize > 500 || !options.AcknowledgeSensitiveMetadata {
		return commandOptions{}, errors.New("invalid audit-runtime-environment options")
	}
	options.ProjectID = strings.TrimSpace(options.ProjectID)
	return options, nil
}

func inspect(ctx context.Context, db *gorm.DB, options commandOptions) (auditReport, error) {
	report := auditReport{Findings: []auditFinding{}}
	for offset := 0; ; offset += options.PageSize {
		var targets []model.DeploymentTarget
		query := db.WithContext(ctx).Select("id", "project_id", "env_vars").Order("id asc").Limit(options.PageSize).Offset(offset)
		if options.ProjectID != "" {
			query = query.Where("project_id = ?", options.ProjectID)
		}
		if err := query.Find(&targets).Error; err != nil {
			return auditReport{}, errors.New("inspect deployment target environment metadata")
		}
		for _, target := range targets {
			finding, ok, err := inspectRow("deployment_target", target.ID, target.ProjectID, target.EnvVars)
			if err != nil {
				return auditReport{}, errors.New("inspect deployment target environment metadata")
			}
			if ok {
				report.Findings = append(report.Findings, finding)
			}
		}
		report.DeploymentTargetsScanned += len(targets)
		if len(targets) < options.PageSize {
			break
		}
	}
	for offset := 0; ; offset += options.PageSize {
		var sets []model.ProjectRuntimeConfigSet
		query := db.WithContext(ctx).Select("id", "project_id", "env_vars").Order("id asc").Limit(options.PageSize).Offset(offset)
		if options.ProjectID != "" {
			query = query.Where("project_id = ?", options.ProjectID)
		}
		if err := query.Find(&sets).Error; err != nil {
			return auditReport{}, errors.New("inspect runtime config set environment metadata")
		}
		for _, set := range sets {
			finding, ok, err := inspectRow("runtime_config_set", set.ID, set.ProjectID, set.EnvVars)
			if err != nil {
				return auditReport{}, errors.New("inspect runtime config set environment metadata")
			}
			if ok {
				report.Findings = append(report.Findings, finding)
			}
		}
		report.RuntimeConfigSetsScanned += len(sets)
		if len(sets) < options.PageSize {
			break
		}
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].ResourceType == report.Findings[j].ResourceType {
			return report.Findings[i].ResourceID < report.Findings[j].ResourceID
		}
		return report.Findings[i].ResourceType < report.Findings[j].ResourceType
	})
	return report, nil
}

func inspectRow(resourceType, resourceID, projectID, raw string) (auditFinding, bool, error) {
	keys, err := runtimeconfig.SuspectedSecretKeys(raw)
	if err != nil {
		return auditFinding{}, false, err
	}
	if len(keys) == 0 {
		return auditFinding{}, false, nil
	}
	return auditFinding{ResourceType: resourceType, ResourceID: resourceID, ProjectID: projectID, Keys: keys}, true, nil
}
