package config

import (
	"errors"
	"strings"
)

// LoadWorker is the only Worker startup adapter that reads deployment environment.
func LoadWorker() (WorkerConfig, error) {
	snapshot, environmentErr := loadEnvironmentSnapshot()
	shared, sharedErr := loadSharedFrom(snapshot, environmentErr)
	raw, decodeErr := decodeEnvironment[workerEnvironment](snapshot)
	cfg, validationErr := buildWorker(raw, shared)
	return cfg, errors.Join(sharedErr, decodeErr, validationErr)
}

func buildWorker(raw workerEnvironment, shared Shared) (WorkerConfig, error) {
	cfg := WorkerConfig{
		Shared:                      shared,
		DatabaseMaxOpenConns:        raw.Database.MaxOpenConns,
		DatabaseMaxIdleConns:        raw.Database.MaxIdleConns,
		DatabaseConnMaxLifetime:     raw.Database.ConnMaxLifetime,
		DatabaseConnMaxIdleTime:     raw.Database.ConnMaxIdleTime,
		BuildExecutorImage:          strings.TrimSpace(raw.BuildExecutorImage),
		BuildCacheEnabled:           raw.BuildCacheEnabled,
		BuildJobTimeoutSeconds:      int64(raw.BuildJobTimeoutSeconds),
		BuildJobTTLSeconds:          int64(raw.BuildJobTTLSeconds),
		DeployRolloutTimeoutSeconds: int64(raw.DeployRolloutTimeout),
		CertManagerClusterIssuer:    strings.TrimSpace(raw.CertManagerClusterIssuer),
	}

	var errs []error
	if shared.Mode == "production" && shared.PublicBaseURL == "" {
		errs = append(errs, errors.New("PUBLIC_BASE_URL is required by Worker in production"))
	}
	if cfg.DatabaseMaxOpenConns <= 0 {
		errs = append(errs, errors.New("WORKER_DB_MAX_OPEN_CONNS must be positive"))
	}
	if cfg.DatabaseMaxIdleConns < 0 || cfg.DatabaseMaxIdleConns > cfg.DatabaseMaxOpenConns {
		errs = append(errs, errors.New("WORKER_DB_MAX_IDLE_CONNS must be between 0 and WORKER_DB_MAX_OPEN_CONNS"))
	}
	if raw.BuildJobTimeoutSeconds <= 0 {
		errs = append(errs, errors.New("BUILD_JOB_TIMEOUT_SECONDS must be positive"))
	}
	if raw.BuildJobTTLSeconds < 0 {
		errs = append(errs, errors.New("BUILD_JOB_TTL_SECONDS must be non-negative"))
	}
	if raw.DeployRolloutTimeout <= 0 {
		errs = append(errs, errors.New("DEPLOY_ROLLOUT_TIMEOUT_SECONDS must be positive"))
	}

	return cfg, errors.Join(errs...)
}
