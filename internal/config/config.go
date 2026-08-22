package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/joho/godotenv"
	"k8s.io/apimachinery/pkg/api/resource"
)

var envLoadOnce sync.Once

const (
	minimumVolumeTransferBytes = int64(1 * 1024 * 1024 * 1024)
	maximumVolumeTransferBytes = int64(5 * 1024 * 1024 * 1024 * 1024)
)

// LoadEnvironment loads the configured dotenv file before process-wide
// infrastructure such as telemetry is initialized. Load remains idempotent.
func LoadEnvironment() {
	loadEnvFile()
}

type Config struct {
	APIAddr                     string
	PublicBaseURL               string
	DatabaseURL                 string
	DatabaseMaxOpenConns        int
	DatabaseMaxIdleConns        int
	DatabaseConnMaxLifetime     time.Duration
	DatabaseConnMaxIdleTime     time.Duration
	RedisAddr                   string
	TrustedProxyCIDRs           []string
	BootstrapToken              string
	MetricsEnabled              bool
	MetricsAddr                 string
	MetricsPath                 string
	BuildExecutorImage          string
	BuildEgressMode             string
	BuildCacheEnabled           bool
	BuildCacheTag               string
	BuildJobTimeoutSeconds      int64
	BuildJobTTLSeconds          int64
	BuildPrivateEgressCIDRs     []string
	BuildPrivateEgressPorts     []int
	BuildBlockedEgressCIDRs     []string
	DeployRolloutTimeoutSeconds int64
	CertManagerClusterIssuer    string
	VolumeTransferMaxBytes      int64
	VolumeTransferJobImage      string
}

func Load() Config {
	LoadEnvironment()

	return Config{
		APIAddr:                     env("API_ADDR", ":8080"),
		PublicBaseURL:               strings.TrimRight(env("PUBLIC_BASE_URL", ""), "/"),
		DatabaseURL:                 env("DATABASE_URL", "postgres://devops:devops@localhost:5432/devops?sslmode=disable"),
		DatabaseMaxOpenConns:        envInt("DB_MAX_OPEN_CONNS", 20),
		DatabaseMaxIdleConns:        envInt("DB_MAX_IDLE_CONNS", 5),
		DatabaseConnMaxLifetime:     envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		DatabaseConnMaxIdleTime:     envDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		RedisAddr:                   strings.TrimSpace(env("REDIS_ADDR", "redis://localhost:6379/0")),
		TrustedProxyCIDRs:           trustedProxyCIDRs(env("TRUSTED_PROXY_CIDRS", "")),
		BootstrapToken:              strings.TrimSpace(env("BOOTSTRAP_TOKEN", "")),
		MetricsEnabled:              envBool("METRICS_ENABLED", false),
		MetricsAddr:                 env("METRICS_ADDR", ""),
		MetricsPath:                 normalizeMetricsPath(env("METRICS_PATH", "/metrics")),
		BuildExecutorImage:          env("BUILD_EXECUTOR_IMAGE", "moby/buildkit:v0.24.0-rootless"),
		BuildEgressMode:             buildEgressMode(env("BUILD_EGRESS_MODE", "restricted")),
		BuildCacheEnabled:           envBool("BUILD_CACHE_ENABLED", false),
		BuildCacheTag:               env("BUILD_CACHE_TAG", "buildcache"),
		BuildJobTimeoutSeconds:      int64(envInt("BUILD_JOB_TIMEOUT_SECONDS", 1800)),
		BuildJobTTLSeconds:          int64(envInt("BUILD_JOB_TTL_SECONDS", 3600)),
		BuildPrivateEgressCIDRs:     envList("BUILD_PRIVATE_EGRESS_CIDRS"),
		BuildPrivateEgressPorts:     envPortList("BUILD_PRIVATE_EGRESS_PORTS", []int{443}),
		BuildBlockedEgressCIDRs:     append(defaultBuildBlockedEgressCIDRs(), envList("BUILD_BLOCKED_EGRESS_CIDRS")...),
		DeployRolloutTimeoutSeconds: int64(envInt("DEPLOY_ROLLOUT_TIMEOUT_SECONDS", 600)),
		CertManagerClusterIssuer:    env("CERT_MANAGER_CLUSTER_ISSUER", "letsencrypt-http01"),
		VolumeTransferMaxBytes:      envByteQuantity("VOLUME_TRANSFER_MAX_BYTES", 100*1024*1024*1024),
		VolumeTransferJobImage:      strings.TrimSpace(env("VOLUME_TRANSFER_JOB_IMAGE", "")),
	}
}

func (c Config) RedisOptions() redisconfig.Options {
	return redisconfig.MustParse(c.RedisAddr)
}

func (c Config) ValidateRedis() error {
	if _, err := redisconfig.Parse(c.RedisAddr); err != nil {
		return fmt.Errorf("invalid REDIS_ADDR: %w", err)
	}
	return nil
}

func (c Config) VolumeTransferEnabled() bool {
	return c.VolumeTransferJobImage != ""
}

func (c Config) ValidateVolumeTransfer() error {
	if c.VolumeTransferMaxBytes < minimumVolumeTransferBytes || c.VolumeTransferMaxBytes > maximumVolumeTransferBytes {
		return errors.New("VOLUME_TRANSFER_MAX_BYTES must be between 1Gi and 5Ti")
	}
	return nil
}

func trustedProxyCIDRs(raw string) []string {
	values, err := parseTrustedProxyCIDRs(raw)
	if err != nil {
		slog.Warn("trusted proxy configuration rejected",
			"event.name", "config.trusted_proxy.invalid",
			"error.type", fmt.Sprintf("%T", err),
		)
		return nil
	}
	return values
}

func parseTrustedProxyCIDRs(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[netip.Prefix]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, err
		}
		prefix = prefix.Masked()
		if _, exists := seen[prefix]; exists {
			continue
		}
		seen[prefix] = struct{}{}
		values = append(values, prefix.String())
	}
	return values, nil
}

func RuntimeMode() string {
	switch strings.ToLower(os.Getenv("APP_ENV")) {
	case "production", "prod":
		return "production"
	case "development", "dev", "local":
		return "development"
	}
	return "production"
}

func loadEnvFile() {
	envLoadOnce.Do(loadEnvFileOnce)
}

func loadEnvFileOnce() {
	envFile := strings.TrimSpace(os.Getenv("ENV_FILE"))
	if envFile == "" {
		envFile = ".env"
	}
	loadEnvFiles(envFile)
}

func resetEnvLoaderForTest() {
	envLoadOnce = sync.Once{}
}

func loadEnvFiles(paths ...string) {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if err := godotenv.Load(path); err != nil {
			if RuntimeMode() == "development" {
				slog.Debug("environment file not loaded; using process environment",
					"event.name", "config.env_file.not_loaded",
					"file.path", path,
					"error.type", fmt.Sprintf("%T", err),
				)
			}
			continue
		}
		if RuntimeMode() == "development" {
			slog.Debug("environment file loaded", "event.name", "config.env_file.loaded", "file.path", path)
		}
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func envByteQuantity(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil || quantity.Sign() <= 0 {
		return fallback
	}
	return quantity.Value()
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envList(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func envPortList(key string, fallback []int) []int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return append([]int(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 1 || value > 65535 || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	if len(values) == 0 {
		return append([]int(nil), fallback...)
	}
	return values
}

func buildEgressMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "permissive":
		return "permissive"
	default:
		return "restricted"
	}
}

func defaultBuildBlockedEgressCIDRs() []string {
	return []string{"169.254.169.254/32"}
}
