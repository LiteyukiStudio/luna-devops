package worker

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
)

type Config struct {
	sharedconfig.Shared
	DatabaseMaxOpenConns        int
	DatabaseMaxIdleConns        int
	DatabaseConnMaxLifetime     time.Duration
	DatabaseConnMaxIdleTime     time.Duration
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
}

func LoadConfig() (Config, error) {
	shared, sharedErr := sharedconfig.LoadShared()
	cfg := Config{
		Shared:                   shared,
		BuildExecutorImage:       sharedconfig.String("BUILD_EXECUTOR_IMAGE", "moby/buildkit:v0.24.0-rootless"),
		BuildCacheTag:            sharedconfig.String("BUILD_CACHE_TAG", "buildcache"),
		CertManagerClusterIssuer: sharedconfig.String("CERT_MANAGER_CLUSTER_ISSUER", "letsencrypt-http01"),
	}

	var errs []error
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	if shared.Mode == "production" && shared.PublicBaseURL == "" {
		errs = append(errs, errors.New("PUBLIC_BASE_URL is required by Worker in production"))
	}
	cfg.BuildPrivateEgressCIDRs, sharedErr = parseCIDRList(
		"BUILD_PRIVATE_EGRESS_CIDRS",
		sharedconfig.List("BUILD_PRIVATE_EGRESS_CIDRS"),
	)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.BuildBlockedEgressCIDRs, sharedErr = parseCIDRList(
		"BUILD_BLOCKED_EGRESS_CIDRS",
		append([]string{"169.254.169.254/32"}, sharedconfig.List("BUILD_BLOCKED_EGRESS_CIDRS")...),
	)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseMaxOpenConns, sharedErr = sharedconfig.Int("WORKER_DB_MAX_OPEN_CONNS", 20)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseMaxIdleConns, sharedErr = sharedconfig.Int("WORKER_DB_MAX_IDLE_CONNS", 5)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseConnMaxLifetime, sharedErr = sharedconfig.Duration("WORKER_DB_CONN_MAX_LIFETIME", 30*time.Minute)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	cfg.DatabaseConnMaxIdleTime, sharedErr = sharedconfig.Duration("WORKER_DB_CONN_MAX_IDLE_TIME", 5*time.Minute)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	if cfg.DatabaseMaxOpenConns <= 0 {
		errs = append(errs, errors.New("WORKER_DB_MAX_OPEN_CONNS must be positive"))
	}
	if cfg.DatabaseMaxIdleConns < 0 || cfg.DatabaseMaxIdleConns > cfg.DatabaseMaxOpenConns {
		errs = append(errs, errors.New("WORKER_DB_MAX_IDLE_CONNS must be between 0 and WORKER_DB_MAX_OPEN_CONNS"))
	}

	switch strings.ToLower(strings.TrimSpace(sharedconfig.String("BUILD_EGRESS_MODE", "restricted"))) {
	case "restricted":
		cfg.BuildEgressMode = "restricted"
	case "permissive":
		cfg.BuildEgressMode = "permissive"
	default:
		errs = append(errs, errors.New("BUILD_EGRESS_MODE must be restricted or permissive"))
	}
	cfg.BuildCacheEnabled, sharedErr = sharedconfig.Bool("BUILD_CACHE_ENABLED", false)
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	buildTimeout, parseErr := positiveInt("BUILD_JOB_TIMEOUT_SECONDS", 1800)
	if parseErr != nil {
		errs = append(errs, parseErr)
	}
	cfg.BuildJobTimeoutSeconds = int64(buildTimeout)
	buildTTL, parseErr := nonNegativeInt("BUILD_JOB_TTL_SECONDS", 3600)
	if parseErr != nil {
		errs = append(errs, parseErr)
	}
	cfg.BuildJobTTLSeconds = int64(buildTTL)
	cfg.BuildPrivateEgressPorts, sharedErr = sharedconfig.PortList("BUILD_PRIVATE_EGRESS_PORTS", []int{443})
	if sharedErr != nil {
		errs = append(errs, sharedErr)
	}
	deployTimeout, parseErr := positiveInt("DEPLOY_ROLLOUT_TIMEOUT_SECONDS", 600)
	if parseErr != nil {
		errs = append(errs, parseErr)
	}
	cfg.DeployRolloutTimeoutSeconds = int64(deployTimeout)
	return cfg, errors.Join(errs...)
}

func positiveInt(key string, fallback int) (int, error) {
	value, err := sharedconfig.Int(key, fallback)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return value, nil
}

func nonNegativeInt(key string, fallback int) (int, error) {
	value, err := sharedconfig.Int(key, fallback)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be non-negative", key)
	}
	return value, nil
}

func parseCIDRList(key string, values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[netip.Prefix]struct{}, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("%s must contain valid CIDRs", key)
		}
		prefix = prefix.Masked()
		if _, exists := seen[prefix]; exists {
			continue
		}
		seen[prefix] = struct{}{}
		result = append(result, prefix.String())
	}
	return result, nil
}
