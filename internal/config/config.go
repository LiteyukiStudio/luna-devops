package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

var envLoadOnce sync.Once

type Config struct {
	APIAddr                     string
	DatabaseURL                 string
	RedisAddr                   string
	BuildExecutorImage          string
	BuildNPMRegistry            string
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
}

func Load() Config {
	loadEnvFile()

	return Config{
		APIAddr:                     env("API_ADDR", ":8080"),
		DatabaseURL:                 env("DATABASE_URL", "postgres://devops:devops@localhost:5432/devops?sslmode=disable"),
		RedisAddr:                   env("REDIS_ADDR", "localhost:6379"),
		BuildExecutorImage:          env("BUILD_EXECUTOR_IMAGE", "moby/buildkit:v0.24.0-rootless"),
		BuildNPMRegistry:            env("BUILD_NPM_REGISTRY", ""),
		BuildEgressMode:             buildEgressMode(env("BUILD_EGRESS_MODE", "permissive")),
		BuildCacheEnabled:           envBool("BUILD_CACHE_ENABLED", false),
		BuildCacheTag:               env("BUILD_CACHE_TAG", "buildcache"),
		BuildJobTimeoutSeconds:      int64(envInt("BUILD_JOB_TIMEOUT_SECONDS", 1800)),
		BuildJobTTLSeconds:          int64(envInt("BUILD_JOB_TTL_SECONDS", 3600)),
		BuildPrivateEgressCIDRs:     envList("BUILD_PRIVATE_EGRESS_CIDRS"),
		BuildPrivateEgressPorts:     envPortList("BUILD_PRIVATE_EGRESS_PORTS", []int{443}),
		BuildBlockedEgressCIDRs:     append(defaultBuildBlockedEgressCIDRs(), envList("BUILD_BLOCKED_EGRESS_CIDRS")...),
		DeployRolloutTimeoutSeconds: int64(envInt("DEPLOY_ROLLOUT_TIMEOUT_SECONDS", 600)),
		CertManagerClusterIssuer:    env("CERT_MANAGER_CLUSTER_ISSUER", "letsencrypt-http01"),
	}
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
