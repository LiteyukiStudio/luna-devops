package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
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
	APIAddr                         string
	PublicBaseURL                   string
	DatabaseURL                     string
	DatabaseMaxOpenConns            int
	DatabaseMaxIdleConns            int
	DatabaseConnMaxLifetime         time.Duration
	DatabaseConnMaxIdleTime         time.Duration
	RedisAddr                       string
	TrustedProxyCIDRs               []string
	BootstrapToken                  string
	MetricsEnabled                  bool
	MetricsAddr                     string
	MetricsPath                     string
	BuildExecutorImage              string
	BuildNPMRegistry                string
	BuildEgressMode                 string
	BuildCacheEnabled               bool
	BuildCacheTag                   string
	BuildJobTimeoutSeconds          int64
	BuildJobTTLSeconds              int64
	BuildPrivateEgressCIDRs         []string
	BuildPrivateEgressPorts         []int
	BuildBlockedEgressCIDRs         []string
	DeployRolloutTimeoutSeconds     int64
	CertManagerClusterIssuer        string
	VolumeTransferStore             string
	VolumeTransferS3Endpoint        string
	VolumeTransferS3Region          string
	VolumeTransferS3Bucket          string
	VolumeTransferS3AccessKeyID     string
	VolumeTransferS3SecretKey       string
	VolumeTransferS3PathStyle       bool
	VolumeTransferObjectTTL         time.Duration
	VolumeTransferMaxBytes          int64
	VolumeTransferSpoolDir          string
	VolumeTransferSpoolMaxBytes     int64
	VolumeTransferSpoolMinFreeBytes int64
	VolumeTransferSpoolOrphanAge    time.Duration
	VolumeTransferCallbackURL       string
	VolumeTransferJobImage          string
}

func Load() Config {
	LoadEnvironment()

	return Config{
		APIAddr:                         env("API_ADDR", ":8080"),
		PublicBaseURL:                   strings.TrimRight(env("PUBLIC_BASE_URL", ""), "/"),
		DatabaseURL:                     env("DATABASE_URL", "postgres://devops:devops@localhost:5432/devops?sslmode=disable"),
		DatabaseMaxOpenConns:            envInt("DB_MAX_OPEN_CONNS", 20),
		DatabaseMaxIdleConns:            envInt("DB_MAX_IDLE_CONNS", 5),
		DatabaseConnMaxLifetime:         envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		DatabaseConnMaxIdleTime:         envDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		RedisAddr:                       strings.TrimSpace(env("REDIS_ADDR", "redis://localhost:6379/0")),
		TrustedProxyCIDRs:               trustedProxyCIDRs(env("TRUSTED_PROXY_CIDRS", "")),
		BootstrapToken:                  strings.TrimSpace(env("BOOTSTRAP_TOKEN", "")),
		MetricsEnabled:                  envBool("METRICS_ENABLED", false),
		MetricsAddr:                     env("METRICS_ADDR", ""),
		MetricsPath:                     normalizeMetricsPath(env("METRICS_PATH", "/metrics")),
		BuildExecutorImage:              env("BUILD_EXECUTOR_IMAGE", "moby/buildkit:v0.24.0-rootless"),
		BuildNPMRegistry:                env("BUILD_NPM_REGISTRY", ""),
		BuildEgressMode:                 buildEgressMode(env("BUILD_EGRESS_MODE", "restricted")),
		BuildCacheEnabled:               envBool("BUILD_CACHE_ENABLED", false),
		BuildCacheTag:                   env("BUILD_CACHE_TAG", "buildcache"),
		BuildJobTimeoutSeconds:          int64(envInt("BUILD_JOB_TIMEOUT_SECONDS", 1800)),
		BuildJobTTLSeconds:              int64(envInt("BUILD_JOB_TTL_SECONDS", 3600)),
		BuildPrivateEgressCIDRs:         envList("BUILD_PRIVATE_EGRESS_CIDRS"),
		BuildPrivateEgressPorts:         envPortList("BUILD_PRIVATE_EGRESS_PORTS", []int{443}),
		BuildBlockedEgressCIDRs:         append(defaultBuildBlockedEgressCIDRs(), envList("BUILD_BLOCKED_EGRESS_CIDRS")...),
		DeployRolloutTimeoutSeconds:     int64(envInt("DEPLOY_ROLLOUT_TIMEOUT_SECONDS", 600)),
		CertManagerClusterIssuer:        env("CERT_MANAGER_CLUSTER_ISSUER", "letsencrypt-http01"),
		VolumeTransferStore:             strings.ToLower(strings.TrimSpace(env("VOLUME_TRANSFER_STORE", ""))),
		VolumeTransferS3Endpoint:        strings.TrimRight(strings.TrimSpace(env("VOLUME_TRANSFER_S3_ENDPOINT", "")), "/"),
		VolumeTransferS3Region:          strings.TrimSpace(env("VOLUME_TRANSFER_S3_REGION", "us-east-1")),
		VolumeTransferS3Bucket:          strings.TrimSpace(env("VOLUME_TRANSFER_S3_BUCKET", "")),
		VolumeTransferS3AccessKeyID:     strings.TrimSpace(env("VOLUME_TRANSFER_S3_ACCESS_KEY_ID", "")),
		VolumeTransferS3SecretKey:       strings.TrimSpace(env("VOLUME_TRANSFER_S3_SECRET_ACCESS_KEY", "")),
		VolumeTransferS3PathStyle:       envBool("VOLUME_TRANSFER_S3_PATH_STYLE", true),
		VolumeTransferObjectTTL:         envDuration("VOLUME_TRANSFER_OBJECT_TTL", 24*time.Hour),
		VolumeTransferMaxBytes:          envByteQuantity("VOLUME_TRANSFER_MAX_BYTES", 100*1024*1024*1024),
		VolumeTransferSpoolDir:          strings.TrimSpace(env("VOLUME_TRANSFER_SPOOL_DIR", filepath.Join(os.TempDir(), "luna-devops-volume-transfer-spool"))),
		VolumeTransferSpoolMaxBytes:     envByteQuantity("VOLUME_TRANSFER_SPOOL_MAX_BYTES", 2*1024*1024*1024),
		VolumeTransferSpoolMinFreeBytes: envByteQuantity("VOLUME_TRANSFER_SPOOL_MIN_FREE_BYTES", 1024*1024*1024),
		VolumeTransferSpoolOrphanAge:    envDuration("VOLUME_TRANSFER_SPOOL_ORPHAN_AGE", 24*time.Hour),
		VolumeTransferCallbackURL:       strings.TrimRight(strings.TrimSpace(env("VOLUME_TRANSFER_CALLBACK_BASE_URL", "")), "/"),
		VolumeTransferJobImage:          strings.TrimSpace(env("VOLUME_TRANSFER_JOB_IMAGE", "")),
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
	return c.VolumeTransferStore == "s3"
}

func (c Config) ValidateVolumeTransfer() error {
	if c.VolumeTransferStore == "" {
		return nil
	}
	if c.VolumeTransferStore != "s3" {
		return fmt.Errorf("unsupported VOLUME_TRANSFER_STORE %q", c.VolumeTransferStore)
	}
	required := []struct {
		key   string
		value string
	}{
		{key: "VOLUME_TRANSFER_S3_ENDPOINT", value: c.VolumeTransferS3Endpoint},
		{key: "VOLUME_TRANSFER_S3_BUCKET", value: c.VolumeTransferS3Bucket},
		{key: "VOLUME_TRANSFER_S3_ACCESS_KEY_ID", value: c.VolumeTransferS3AccessKeyID},
		{key: "VOLUME_TRANSFER_S3_SECRET_ACCESS_KEY", value: c.VolumeTransferS3SecretKey},
		{key: "VOLUME_TRANSFER_CALLBACK_BASE_URL", value: c.VolumeTransferCallbackURL},
		{key: "VOLUME_TRANSFER_JOB_IMAGE", value: c.VolumeTransferJobImage},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required when VOLUME_TRANSFER_STORE=s3", field.key)
		}
	}
	if c.VolumeTransferObjectTTL <= 0 {
		return errors.New("VOLUME_TRANSFER_OBJECT_TTL must be positive")
	}
	if c.VolumeTransferMaxBytes < minimumVolumeTransferBytes || c.VolumeTransferMaxBytes > maximumVolumeTransferBytes {
		return errors.New("VOLUME_TRANSFER_MAX_BYTES must be between 1Gi and 5Ti")
	}
	if c.VolumeTransferSpoolDir != "" && !filepath.IsAbs(c.VolumeTransferSpoolDir) {
		return errors.New("VOLUME_TRANSFER_SPOOL_DIR must be an absolute path")
	}
	if c.VolumeTransferSpoolMaxBytes > 0 && c.VolumeTransferSpoolMaxBytes < requiredVolumeTransferChunkSize(c.VolumeTransferMaxBytes) {
		return errors.New("VOLUME_TRANSFER_SPOOL_MAX_BYTES must fit one server-selected upload chunk")
	}
	if c.VolumeTransferSpoolMinFreeBytes < 0 || c.VolumeTransferSpoolOrphanAge < 0 {
		return errors.New("volume transfer spool limits must not be negative")
	}
	if err := validateVolumeTransferEndpoint(c.VolumeTransferS3Endpoint, false); err != nil {
		return fmt.Errorf("invalid VOLUME_TRANSFER_S3_ENDPOINT: %w", err)
	}
	if err := validateVolumeTransferEndpoint(c.VolumeTransferCallbackURL, true); err != nil {
		return fmt.Errorf("invalid VOLUME_TRANSFER_CALLBACK_BASE_URL: %w", err)
	}
	return nil
}

func requiredVolumeTransferChunkSize(expectedBytes int64) int64 {
	const (
		minimum = int64(64 * 1024 * 1024)
		parts   = int64(10_000)
		mib     = int64(1024 * 1024)
	)
	required := (expectedBytes + parts - 1) / parts
	required = ((required + mib - 1) / mib) * mib
	if required < minimum {
		return minimum
	}
	return required
}

func validateVolumeTransferEndpoint(value string, requireHTTPS bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("a credential-free absolute URL is required")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return errors.New("HTTPS is required")
	}
	if !requireHTTPS && parsed.Scheme != "https" && !(RuntimeMode() == "development" && parsed.Scheme == "http") {
		return errors.New("HTTPS is required outside development")
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
