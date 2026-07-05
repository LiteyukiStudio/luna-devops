package gatewayprobe

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIBaseURL           string
	ReportToken          string
	RuntimeClusterID     string
	Mode                 string
	ControllerType       string
	GatewayNamespace     string
	TraefikMetricsURL    string
	ProbeAddr            string
	ScrapeInterval       time.Duration
	RouteRefreshInterval time.Duration
	HTTPTimeout          time.Duration
}

func ConfigFromEnv() (Config, error) {
	gatewayNamespace := firstNonEmpty(os.Getenv("GATEWAY_NAMESPACE"), "kube-system")
	cfg := Config{
		APIBaseURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("API_BASE_URL")), "/"),
		ReportToken:          strings.TrimSpace(os.Getenv("REPORT_TOKEN")),
		RuntimeClusterID:     strings.TrimSpace(os.Getenv("RUNTIME_CLUSTER_ID")),
		Mode:                 firstNonEmpty(os.Getenv("MODE"), "traefik-metrics"),
		ControllerType:       firstNonEmpty(os.Getenv("CONTROLLER_TYPE"), "traefik"),
		GatewayNamespace:     gatewayNamespace,
		TraefikMetricsURL:    firstNonEmpty(os.Getenv("TRAEFIK_METRICS_URL"), "http://traefik."+gatewayNamespace+".svc.cluster.local:9100/metrics"),
		ProbeAddr:            firstNonEmpty(os.Getenv("PROBE_ADDR"), ":9090"),
		ScrapeInterval:       durationFromEnv("SCRAPE_INTERVAL", time.Minute),
		RouteRefreshInterval: durationFromEnv("ROUTE_REFRESH_INTERVAL", time.Minute),
		HTTPTimeout:          durationFromEnv("HTTP_TIMEOUT", 15*time.Second),
	}
	if cfg.APIBaseURL == "" {
		return Config{}, fmt.Errorf("API_BASE_URL is required")
	}
	if cfg.ReportToken == "" {
		return Config{}, fmt.Errorf("REPORT_TOKEN is required")
	}
	if cfg.RuntimeClusterID == "" {
		return Config{}, fmt.Errorf("RUNTIME_CLUSTER_ID is required")
	}
	if cfg.Mode != "traefik-metrics" {
		return Config{}, fmt.Errorf("unsupported MODE %q", cfg.Mode)
	}
	if cfg.ScrapeInterval < 10*time.Second {
		cfg.ScrapeInterval = 10 * time.Second
	}
	if cfg.RouteRefreshInterval < 10*time.Second {
		cfg.RouteRefreshInterval = 10 * time.Second
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 15 * time.Second
	}
	return cfg, nil
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
