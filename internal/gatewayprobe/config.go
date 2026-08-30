package gatewayprobe

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
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
	apiBaseURL, err := httpURLFromEnv("API_BASE_URL", "", true)
	if err != nil {
		return Config{}, err
	}
	traefikMetricsURL, err := httpURLFromEnv(
		"TRAEFIK_METRICS_URL",
		"http://traefik."+gatewayNamespace+".svc.cluster.local:9100/metrics",
		false,
	)
	if err != nil {
		return Config{}, err
	}
	scrapeInterval, err := durationFromEnv("SCRAPE_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}
	routeRefreshInterval, err := durationFromEnv("ROUTE_REFRESH_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}
	httpTimeout, err := durationFromEnv("HTTP_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		APIBaseURL:           strings.TrimRight(apiBaseURL, "/"),
		ReportToken:          strings.TrimSpace(os.Getenv("REPORT_TOKEN")),
		RuntimeClusterID:     strings.TrimSpace(os.Getenv("RUNTIME_CLUSTER_ID")),
		Mode:                 firstNonEmpty(os.Getenv("MODE"), "traefik-metrics"),
		ControllerType:       firstNonEmpty(os.Getenv("CONTROLLER_TYPE"), "traefik"),
		GatewayNamespace:     gatewayNamespace,
		TraefikMetricsURL:    traefikMetricsURL,
		ProbeAddr:            firstNonEmpty(os.Getenv("PROBE_ADDR"), ":9090"),
		ScrapeInterval:       scrapeInterval,
		RouteRefreshInterval: routeRefreshInterval,
		HTTPTimeout:          httpTimeout,
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
	if cfg.ControllerType != "traefik" && cfg.ControllerType != "generic" {
		return Config{}, fmt.Errorf("CONTROLLER_TYPE must be traefik or generic")
	}
	if problems := k8svalidation.IsDNS1123Label(cfg.GatewayNamespace); len(problems) > 0 {
		return Config{}, fmt.Errorf("GATEWAY_NAMESPACE must be a valid Kubernetes namespace: %s", strings.Join(problems, "; "))
	}
	if err := validateProbeAddr(cfg.ProbeAddr); err != nil {
		return Config{}, err
	}
	if cfg.ScrapeInterval < 10*time.Second {
		return Config{}, fmt.Errorf("SCRAPE_INTERVAL must be at least 10s")
	}
	if cfg.RouteRefreshInterval < 10*time.Second {
		return Config{}, fmt.Errorf("ROUTE_REFRESH_INTERVAL must be at least 10s")
	}
	return cfg, nil
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed, nil
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second, nil
	}
	return 0, fmt.Errorf("%s must be a positive duration or number of seconds", key)
}

func httpURLFromEnv(key string, fallback string, required bool) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		value = fallback
	}
	if value == "" {
		if required {
			return "", fmt.Errorf("%s is required", key)
		}
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s must be an absolute http or https URL", key)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s must not contain credentials, query parameters, or fragments", key)
	}
	if port := parsed.Port(); port != "" {
		portNumber, parseErr := strconv.Atoi(port)
		if parseErr != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("%s port must be between 1 and 65535", key)
		}
	}
	return value, nil
}

func validateProbeAddr(value string) error {
	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("PROBE_ADDR must use IP:port or :port format")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("PROBE_ADDR port must be between 1 and 65535")
	}
	return nil
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
