package config

import (
	"log"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/joho/godotenv"
)

var envLoadOnce sync.Once

type Config struct {
	APIAddr                      string
	PublicBaseURL                string
	DatabaseURL                  string
	DatabaseMaxOpenConns         int
	DatabaseMaxIdleConns         int
	DatabaseConnMaxLifetime      time.Duration
	DatabaseConnMaxIdleTime      time.Duration
	DatabaseConnectRetryAttempts int
	DatabaseConnectRetryInterval time.Duration
	RedisAddr                    string
	RedisUsername                string
	RedisPassword                string
	RedisDB                      int
	TrustedProxyCIDRs            []string
	BootstrapToken               string
	MetricsEnabled               bool
	MetricsAddr                  string
	MetricsPath                  string
	BuildExecutorImage           string
	BuildNPMRegistry             string
	BuildEgressMode              string
	BuildCacheEnabled            bool
	BuildCacheTag                string
	BuildJobTimeoutSeconds       int64
	BuildJobTTLSeconds           int64
	BuildPrivateEgressCIDRs      []string
	BuildPrivateEgressPorts      []int
	BuildBlockedEgressCIDRs      []string
	DeployRolloutTimeoutSeconds  int64
	CertManagerClusterIssuer     string
}

func Load() Config {
	loadEnvFile()

	return Config{
		APIAddr:                      env("API_ADDR", ":8080"),
		PublicBaseURL:                strings.TrimRight(env("PUBLIC_BASE_URL", ""), "/"),
		DatabaseURL:                  env("DATABASE_URL", "postgres://devops:devops@localhost:5432/devops?sslmode=disable"),
		DatabaseMaxOpenConns:         envInt("DB_MAX_OPEN_CONNS", 20),
		DatabaseMaxIdleConns:         envInt("DB_MAX_IDLE_CONNS", 5),
		DatabaseConnMaxLifetime:      envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		DatabaseConnMaxIdleTime:      envDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),
		DatabaseConnectRetryAttempts: envInt("DB_CONNECT_RETRY_ATTEMPTS", 12),
		DatabaseConnectRetryInterval: envDuration("DB_CONNECT_RETRY_INTERVAL", 5*time.Second),
		RedisAddr:                    env("REDIS_ADDR", "localhost:6379"),
		RedisUsername:                strings.TrimSpace(env("REDIS_USERNAME", "")),
		RedisPassword:                env("REDIS_PASSWORD", ""),
		RedisDB:                      envInt("REDIS_DB", 0),
		TrustedProxyCIDRs:            trustedProxyCIDRs(env("TRUSTED_PROXY_CIDRS", "")),
		BootstrapToken:               strings.TrimSpace(env("BOOTSTRAP_TOKEN", "")),
		MetricsEnabled:               envBool("METRICS_ENABLED", false),
		MetricsAddr:                  env("METRICS_ADDR", ""),
		MetricsPath:                  normalizeMetricsPath(env("METRICS_PATH", "/metrics")),
		BuildExecutorImage:           env("BUILD_EXECUTOR_IMAGE", "moby/buildkit:v0.24.0-rootless"),
		BuildNPMRegistry:             env("BUILD_NPM_REGISTRY", ""),
		BuildEgressMode:              buildEgressMode(env("BUILD_EGRESS_MODE", "permissive")),
		BuildCacheEnabled:            envBool("BUILD_CACHE_ENABLED", false),
		BuildCacheTag:                env("BUILD_CACHE_TAG", "buildcache"),
		BuildJobTimeoutSeconds:       int64(envInt("BUILD_JOB_TIMEOUT_SECONDS", 1800)),
		BuildJobTTLSeconds:           int64(envInt("BUILD_JOB_TTL_SECONDS", 3600)),
		BuildPrivateEgressCIDRs:      envList("BUILD_PRIVATE_EGRESS_CIDRS"),
		BuildPrivateEgressPorts:      envPortList("BUILD_PRIVATE_EGRESS_PORTS", []int{443}),
		BuildBlockedEgressCIDRs:      append(defaultBuildBlockedEgressCIDRs(), envList("BUILD_BLOCKED_EGRESS_CIDRS")...),
		DeployRolloutTimeoutSeconds:  int64(envInt("DEPLOY_ROLLOUT_TIMEOUT_SECONDS", 600)),
		CertManagerClusterIssuer:     env("CERT_MANAGER_CLUSTER_ISSUER", "letsencrypt-http01"),
	}
}

func (c Config) RedisOptions() redisconfig.Options {
	return redisconfig.Options{
		Addr:     c.RedisAddr,
		Username: c.RedisUsername,
		Password: c.RedisPassword,
		DB:       c.RedisDB,
	}.Normalized()
}

func trustedProxyCIDRs(raw string) []string {
	values, err := parseTrustedProxyCIDRs(raw)
	if err != nil {
		log.Printf("invalid TRUSTED_PROXY_CIDRS: %v; forwarded client addresses will not be trusted", err)
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
	loadEnvFiles(".env")

	mode := RuntimeMode()
	switch mode {
	case "development":
		loadEnvFiles(".env.development")
	case "production":
		loadEnvFiles(".env.production")
	}

	if envFile := strings.TrimSpace(os.Getenv("ENV_FILE")); envFile != "" {
		loadEnvFiles(envFile)
	}
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
				log.Printf("development mode: env file %s not loaded: %v; using process environment", path, err)
			}
			continue
		}
		if RuntimeMode() == "development" {
			log.Printf("development mode: loaded env file %s", path)
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
	case "restricted":
		return "restricted"
	default:
		return "permissive"
	}
}

func defaultBuildBlockedEgressCIDRs() []string {
	return []string{"169.254.169.254/32"}
}
