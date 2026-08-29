package worker

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigOwnsWorkerEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("WORKER_DB_MAX_OPEN_CONNS", "12")
	t.Setenv("WORKER_DB_MAX_IDLE_CONNS", "4")
	t.Setenv("WORKER_DB_CONN_MAX_LIFETIME", "20m")
	t.Setenv("WORKER_DB_CONN_MAX_IDLE_TIME", "2m")
	t.Setenv("BUILD_EXECUTOR_IMAGE", "buildkit:test")
	t.Setenv("BUILD_EGRESS_MODE", "permissive")
	t.Setenv("BUILD_CACHE_ENABLED", "true")
	t.Setenv("BUILD_PRIVATE_EGRESS_CIDRS", "10.20.0.0/16,fd00::/8")
	t.Setenv("BUILD_PRIVATE_EGRESS_PORTS", "443,5000,443")
	t.Setenv("BUILD_BLOCKED_EGRESS_CIDRS", "10.96.0.0/12")
	t.Setenv("DEPLOY_ROLLOUT_TIMEOUT_SECONDS", "120")
	t.Setenv("CERT_MANAGER_CLUSTER_ISSUER", "letsencrypt-staging")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseMaxOpenConns != 12 || cfg.DatabaseMaxIdleConns != 4 || cfg.DatabaseConnMaxLifetime != 20*time.Minute || cfg.DatabaseConnMaxIdleTime != 2*time.Minute {
		t.Fatalf("Worker database config = %#v", cfg)
	}
	if cfg.BuildExecutorImage != "buildkit:test" || cfg.BuildEgressMode != "permissive" || !cfg.BuildCacheEnabled {
		t.Fatalf("Worker build config = %#v", cfg)
	}
	if len(cfg.BuildPrivateEgressPorts) != 2 || cfg.BuildPrivateEgressPorts[0] != 443 || cfg.BuildPrivateEgressPorts[1] != 5000 {
		t.Fatalf("BuildPrivateEgressPorts = %#v", cfg.BuildPrivateEgressPorts)
	}
	if len(cfg.BuildBlockedEgressCIDRs) != 2 || cfg.BuildBlockedEgressCIDRs[0] != "169.254.169.254/32" {
		t.Fatalf("BuildBlockedEgressCIDRs = %#v", cfg.BuildBlockedEgressCIDRs)
	}
	if cfg.DeployRolloutTimeoutSeconds != 120 || cfg.CertManagerClusterIssuer != "letsencrypt-staging" {
		t.Fatalf("Worker deploy config = %#v", cfg)
	}
}

func TestLoadConfigUsesIndependentWorkerDatabasePool(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("API_DB_MAX_OPEN_CONNS", "2")
	t.Setenv("WORKER_DB_MAX_OPEN_CONNS", "9")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseMaxOpenConns != 9 {
		t.Fatalf("DatabaseMaxOpenConns = %d", cfg.DatabaseMaxOpenConns)
	}
}

func TestLoadConfigAllowsImmediateBuildJobCleanup(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("BUILD_JOB_TTL_SECONDS", "0")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BuildJobTTLSeconds != 0 {
		t.Fatalf("BuildJobTTLSeconds = %d", cfg.BuildJobTTLSeconds)
	}
}

func TestLoadConfigRejectsInvalidWorkerEnvironment(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  string
	}{
		{key: "WORKER_DB_MAX_OPEN_CONNS", value: "0", want: "must be positive"},
		{key: "WORKER_DB_MAX_IDLE_CONNS", value: "21", want: "must be between"},
		{key: "BUILD_EGRESS_MODE", value: "unexpected", want: "restricted or permissive"},
		{key: "BUILD_PRIVATE_EGRESS_PORTS", value: "443,bad", want: "ports between"},
		{key: "BUILD_PRIVATE_EGRESS_CIDRS", value: "not-a-cidr", want: "valid CIDRs"},
		{key: "BUILD_BLOCKED_EGRESS_CIDRS", value: "10.0.0.0/99", want: "valid CIDRs"},
		{key: "BUILD_JOB_TIMEOUT_SECONDS", value: "0", want: "must be positive"},
		{key: "BUILD_JOB_TTL_SECONDS", value: "-1", want: "must be non-negative"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Setenv("APP_ENV", "development")
			t.Setenv(tt.key, tt.value)
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadConfigRequiresPublicBaseURLInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "")
	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "PUBLIC_BASE_URL is required") {
		t.Fatalf("error = %v", err)
	}
}
