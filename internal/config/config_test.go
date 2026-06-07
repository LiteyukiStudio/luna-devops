package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "API_ADDR")
	unsetEnv(t, "DATABASE_URL")
	unsetEnv(t, "REDIS_ADDR")

	envFile := filepath.Join(t.TempDir(), ".env.local")
	content := []byte("API_ADDR=:19090\nDATABASE_URL=postgres://user:pass@db:5432/app?sslmode=disable\nREDIS_ADDR=redis:6379\n")
	if err := os.WriteFile(envFile, content, 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("ENV_FILE", envFile)

	cfg := Load()
	if cfg.APIAddr != ":19090" {
		t.Fatalf("APIAddr = %q", cfg.APIAddr)
	}
	if cfg.DatabaseURL != "postgres://user:pass@db:5432/app?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.RedisAddr != "redis:6379" {
		t.Fatalf("RedisAddr = %q", cfg.RedisAddr)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	oldValue, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}

	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, oldValue)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func resetEnvLoader(t *testing.T) {
	t.Helper()
	resetEnvLoaderForTest()
	t.Cleanup(resetEnvLoaderForTest)
}

func TestEnvOverridesEnvFile(t *testing.T) {
	resetEnvLoader(t)
	envFile := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(envFile, []byte("API_ADDR=:19090\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("ENV_FILE", envFile)
	t.Setenv("API_ADDR", ":28080")

	cfg := Load()
	if cfg.APIAddr != ":28080" {
		t.Fatalf("APIAddr = %q", cfg.APIAddr)
	}
}

func TestLoadBuildPrivateEgressCIDRs(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("BUILD_PRIVATE_EGRESS_CIDRS", "10.20.0.0/16, fd00::/8 ,,")

	cfg := Load()
	if len(cfg.BuildPrivateEgressCIDRs) != 2 {
		t.Fatalf("BuildPrivateEgressCIDRs = %#v", cfg.BuildPrivateEgressCIDRs)
	}
	if cfg.BuildPrivateEgressCIDRs[0] != "10.20.0.0/16" || cfg.BuildPrivateEgressCIDRs[1] != "fd00::/8" {
		t.Fatalf("BuildPrivateEgressCIDRs = %#v", cfg.BuildPrivateEgressCIDRs)
	}
}

func TestLoadBuildBlockedEgressCIDRsIncludesMetadataDefault(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("BUILD_BLOCKED_EGRESS_CIDRS", "10.96.0.0/12")

	cfg := Load()
	if len(cfg.BuildBlockedEgressCIDRs) != 2 {
		t.Fatalf("BuildBlockedEgressCIDRs = %#v", cfg.BuildBlockedEgressCIDRs)
	}
	if cfg.BuildBlockedEgressCIDRs[0] != "169.254.169.254/32" || cfg.BuildBlockedEgressCIDRs[1] != "10.96.0.0/12" {
		t.Fatalf("BuildBlockedEgressCIDRs = %#v", cfg.BuildBlockedEgressCIDRs)
	}
}

func TestLoadDeployRolloutTimeoutSeconds(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("DEPLOY_ROLLOUT_TIMEOUT_SECONDS", "120")

	cfg := Load()
	if cfg.DeployRolloutTimeoutSeconds != 120 {
		t.Fatalf("DeployRolloutTimeoutSeconds = %d", cfg.DeployRolloutTimeoutSeconds)
	}
}

func TestLoadCertManagerClusterIssuer(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("CERT_MANAGER_CLUSTER_ISSUER", "letsencrypt-staging")

	cfg := Load()
	if cfg.CertManagerClusterIssuer != "letsencrypt-staging" {
		t.Fatalf("CertManagerClusterIssuer = %q", cfg.CertManagerClusterIssuer)
	}
}

func TestRuntimeModeDefaultsToProduction(t *testing.T) {
	unsetEnv(t, "APP_ENV")

	if got := RuntimeMode(); got != "production" {
		t.Fatalf("RuntimeMode() = %q, want production", got)
	}
}

func TestLoadEnvFileLogsPathInDevelopment(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "API_ADDR")
	unsetEnv(t, "DATABASE_URL")
	unsetEnv(t, "REDIS_ADDR")

	envFile := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(envFile, []byte("API_ADDR=:19090\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("APP_ENV", "development")
	t.Setenv("ENV_FILE", envFile)

	var output bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() {
		log.SetOutput(oldOutput)
	})

	_ = Load()

	got := output.String()
	if !strings.Contains(got, "loaded env file") || !strings.Contains(got, envFile) {
		t.Fatalf("log output %q does not include loaded env file path %q", got, envFile)
	}
}

func TestLoadDefaultsToEnvDevelopmentInDevelopment(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "API_ADDR")
	unsetEnv(t, "DATABASE_URL")
	unsetEnv(t, "REDIS_ADDR")
	unsetEnv(t, "ENV_FILE")

	workDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldDir)
	})

	if err := os.WriteFile(filepath.Join(workDir, ".env.development"), []byte("API_ADDR=:19091\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("APP_ENV", "development")

	cfg := Load()
	if cfg.APIAddr != ":19091" {
		t.Fatalf("APIAddr = %q", cfg.APIAddr)
	}
}

func TestLoadReadsEnvBeforeModeSpecificEnv(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "APP_ENV")
	unsetEnv(t, "API_ADDR")
	unsetEnv(t, "ENV_FILE")

	workDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldDir)
	})

	if err := os.WriteFile(filepath.Join(workDir, ".env"), []byte("APP_ENV=development\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".env.development"), []byte("API_ADDR=:19092\n"), 0o600); err != nil {
		t.Fatalf("write .env.development: %v", err)
	}

	cfg := Load()
	if cfg.APIAddr != ":19092" {
		t.Fatalf("APIAddr = %q", cfg.APIAddr)
	}
}

func TestLoadMissingDefaultEnvDevelopmentLogsFallback(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "API_ADDR")
	unsetEnv(t, "ENV_FILE")

	workDir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldDir)
	})

	t.Setenv("APP_ENV", "development")

	var output bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() {
		log.SetOutput(oldOutput)
	})

	cfg := Load()
	if cfg.APIAddr != ":8080" {
		t.Fatalf("APIAddr = %q", cfg.APIAddr)
	}

	got := output.String()
	if !strings.Contains(got, ".env.development") || !strings.Contains(got, "using process environment") {
		t.Fatalf("log output %q does not include .env.development fallback message", got)
	}
}
