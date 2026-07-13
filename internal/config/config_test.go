package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestLoadRedisAuthentication(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "REDIS_ADDR")
	unsetEnv(t, "REDIS_USERNAME")
	unsetEnv(t, "REDIS_PASSWORD")
	unsetEnv(t, "REDIS_DB")
	t.Setenv("REDIS_ADDR", "redis.example.com:6379")
	t.Setenv("REDIS_USERNAME", "luna")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "4")

	options := Load().RedisOptions()
	if options.Addr != "redis.example.com:6379" || options.Username != "luna" || options.Password != "secret" || options.DB != 4 {
		t.Fatalf("RedisOptions() = %#v", options)
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

func TestLoadBuildEgressModeDefaultsToPermissive(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "BUILD_EGRESS_MODE")

	cfg := Load()
	if cfg.BuildEgressMode != "permissive" {
		t.Fatalf("BuildEgressMode = %q", cfg.BuildEgressMode)
	}
}

func TestLoadBuildEgressModeSupportsRestricted(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("BUILD_EGRESS_MODE", "restricted")

	cfg := Load()
	if cfg.BuildEgressMode != "restricted" {
		t.Fatalf("BuildEgressMode = %q", cfg.BuildEgressMode)
	}
}

func TestLoadMetricsConfigDefaultsDisabled(t *testing.T) {
	resetEnvLoaderForTest()
	unsetEnv(t, "METRICS_ENABLED")
	unsetEnv(t, "METRICS_ADDR")
	unsetEnv(t, "METRICS_PATH")

	cfg := Load()
	if cfg.MetricsEnabled {
		t.Fatalf("MetricsEnabled = true, want false")
	}
	if cfg.MetricsAddr != "" {
		t.Fatalf("MetricsAddr = %q, want empty", cfg.MetricsAddr)
	}
	if cfg.MetricsPath != "/metrics" {
		t.Fatalf("MetricsPath = %q, want /metrics", cfg.MetricsPath)
	}
}

func TestLoadMetricsConfigNormalizesPath(t *testing.T) {
	resetEnvLoaderForTest()
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_ADDR", ":19090")
	t.Setenv("METRICS_PATH", "metrics")

	cfg := Load()
	if !cfg.MetricsEnabled {
		t.Fatalf("MetricsEnabled = false, want true")
	}
	if cfg.MetricsAddr != ":19090" {
		t.Fatalf("MetricsAddr = %q", cfg.MetricsAddr)
	}
	if cfg.MetricsPath != "/metrics" {
		t.Fatalf("MetricsPath = %q, want /metrics", cfg.MetricsPath)
	}
}

func TestLoadDatabasePoolDefaults(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "DB_MAX_OPEN_CONNS")
	unsetEnv(t, "DB_MAX_IDLE_CONNS")
	unsetEnv(t, "DB_CONN_MAX_LIFETIME")
	unsetEnv(t, "DB_CONN_MAX_IDLE_TIME")
	unsetEnv(t, "DB_CONNECT_RETRY_ATTEMPTS")
	unsetEnv(t, "DB_CONNECT_RETRY_INTERVAL")

	cfg := Load()
	if cfg.DatabaseMaxOpenConns != 20 {
		t.Fatalf("DatabaseMaxOpenConns = %d", cfg.DatabaseMaxOpenConns)
	}
	if cfg.DatabaseMaxIdleConns != 5 {
		t.Fatalf("DatabaseMaxIdleConns = %d", cfg.DatabaseMaxIdleConns)
	}
	if cfg.DatabaseConnMaxLifetime != 30*time.Minute {
		t.Fatalf("DatabaseConnMaxLifetime = %s", cfg.DatabaseConnMaxLifetime)
	}
	if cfg.DatabaseConnMaxIdleTime != 5*time.Minute {
		t.Fatalf("DatabaseConnMaxIdleTime = %s", cfg.DatabaseConnMaxIdleTime)
	}
	if cfg.DatabaseConnectRetryAttempts != 12 {
		t.Fatalf("DatabaseConnectRetryAttempts = %d", cfg.DatabaseConnectRetryAttempts)
	}
	if cfg.DatabaseConnectRetryInterval != 5*time.Second {
		t.Fatalf("DatabaseConnectRetryInterval = %s", cfg.DatabaseConnectRetryInterval)
	}
}

func TestLoadDatabasePoolOverrides(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("DB_MAX_OPEN_CONNS", "8")
	t.Setenv("DB_MAX_IDLE_CONNS", "3")
	t.Setenv("DB_CONN_MAX_LIFETIME", "12m")
	t.Setenv("DB_CONN_MAX_IDLE_TIME", "90")
	t.Setenv("DB_CONNECT_RETRY_ATTEMPTS", "4")
	t.Setenv("DB_CONNECT_RETRY_INTERVAL", "2s")

	cfg := Load()
	if cfg.DatabaseMaxOpenConns != 8 {
		t.Fatalf("DatabaseMaxOpenConns = %d", cfg.DatabaseMaxOpenConns)
	}
	if cfg.DatabaseMaxIdleConns != 3 {
		t.Fatalf("DatabaseMaxIdleConns = %d", cfg.DatabaseMaxIdleConns)
	}
	if cfg.DatabaseConnMaxLifetime != 12*time.Minute {
		t.Fatalf("DatabaseConnMaxLifetime = %s", cfg.DatabaseConnMaxLifetime)
	}
	if cfg.DatabaseConnMaxIdleTime != 90*time.Second {
		t.Fatalf("DatabaseConnMaxIdleTime = %s", cfg.DatabaseConnMaxIdleTime)
	}
	if cfg.DatabaseConnectRetryAttempts != 4 {
		t.Fatalf("DatabaseConnectRetryAttempts = %d", cfg.DatabaseConnectRetryAttempts)
	}
	if cfg.DatabaseConnectRetryInterval != 2*time.Second {
		t.Fatalf("DatabaseConnectRetryInterval = %s", cfg.DatabaseConnectRetryInterval)
	}
}

func TestLoadBuildPrivateEgressPortsDefaultsTo443(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "BUILD_PRIVATE_EGRESS_PORTS")

	cfg := Load()
	if len(cfg.BuildPrivateEgressPorts) != 1 || cfg.BuildPrivateEgressPorts[0] != 443 {
		t.Fatalf("BuildPrivateEgressPorts = %#v", cfg.BuildPrivateEgressPorts)
	}
}

func TestLoadBuildPrivateEgressPorts(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("BUILD_PRIVATE_EGRESS_PORTS", "443, 5000, 8081, bad, 0, 5000, 65536")

	cfg := Load()
	expected := []int{443, 5000, 8081}
	if len(cfg.BuildPrivateEgressPorts) != len(expected) {
		t.Fatalf("BuildPrivateEgressPorts = %#v", cfg.BuildPrivateEgressPorts)
	}
	for index, value := range expected {
		if cfg.BuildPrivateEgressPorts[index] != value {
			t.Fatalf("BuildPrivateEgressPorts = %#v", cfg.BuildPrivateEgressPorts)
		}
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

func TestLoadBootstrapToken(t *testing.T) {
	resetEnvLoader(t)
	t.Setenv("BOOTSTRAP_TOKEN", "  bootstrap-secret  ")

	cfg := Load()
	if cfg.BootstrapToken != "bootstrap-secret" {
		t.Fatalf("BootstrapToken = %q", cfg.BootstrapToken)
	}
}

func TestParseTrustedProxyCIDRsNormalizesAndDeduplicates(t *testing.T) {
	got, err := parseTrustedProxyCIDRs(" 10.0.1.7/8,fd00::1234/8,10.0.0.0/8,, ")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.0/8", "fd00::/8"}
	if len(got) != len(want) {
		t.Fatalf("trusted proxy CIDRs = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("trusted proxy CIDRs = %#v", got)
		}
	}
}

func TestTrustedProxyCIDRsFailClosedOnInvalidEntry(t *testing.T) {
	if got := trustedProxyCIDRs("10.0.0.0/8,not-a-cidr"); len(got) != 0 {
		t.Fatalf("trusted proxy CIDRs = %#v, want none", got)
	}
}

func TestLoadTrustedProxyCIDRsDefaultsToNone(t *testing.T) {
	resetEnvLoader(t)
	unsetEnv(t, "TRUSTED_PROXY_CIDRS")

	if got := Load().TrustedProxyCIDRs; len(got) != 0 {
		t.Fatalf("TrustedProxyCIDRs = %#v, want none", got)
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
