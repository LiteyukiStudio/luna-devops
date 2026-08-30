package gatewayprobe

import (
	"strings"
	"testing"
	"time"
)

func TestConfigFromEnvDerivesTraefikMetricsURLFromGatewayNamespace(t *testing.T) {
	setValidProbeEnvironment(t)
	t.Setenv("GATEWAY_NAMESPACE", "edge-system")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv returned error: %v", err)
	}
	if cfg.APIBaseURL != "https://devops.example.com" {
		t.Fatalf("apiBaseURL = %q", cfg.APIBaseURL)
	}
	want := "http://traefik.edge-system.svc.cluster.local:9100/metrics"
	if cfg.TraefikMetricsURL != want {
		t.Fatalf("traefikMetricsURL = %q, want %q", cfg.TraefikMetricsURL, want)
	}
}

func TestConfigFromEnvKeepsDefaultsForEmptyOptionalValues(t *testing.T) {
	setValidProbeEnvironment(t)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv returned error: %v", err)
	}
	if cfg.Mode != "traefik-metrics" || cfg.ControllerType != "traefik" || cfg.GatewayNamespace != "kube-system" {
		t.Fatalf("default identity config = %#v", cfg)
	}
	if cfg.TraefikMetricsURL != "http://traefik.kube-system.svc.cluster.local:9100/metrics" || cfg.ProbeAddr != ":9090" {
		t.Fatalf("default endpoint config = %#v", cfg)
	}
	if cfg.ScrapeInterval != time.Minute || cfg.RouteRefreshInterval != time.Minute || cfg.HTTPTimeout != 15*time.Second {
		t.Fatalf("default duration config = %#v", cfg)
	}
}

func TestConfigFromEnvAcceptsStrictExplicitValues(t *testing.T) {
	setValidProbeEnvironment(t)
	t.Setenv("API_BASE_URL", "https://devops.example.com/platform/")
	t.Setenv("TRAEFIK_METRICS_URL", "https://metrics.example.com/metrics")
	t.Setenv("CONTROLLER_TYPE", "generic")
	t.Setenv("GATEWAY_NAMESPACE", "edge-system")
	t.Setenv("PROBE_ADDR", "127.0.0.1:9191")
	t.Setenv("SCRAPE_INTERVAL", "30s")
	t.Setenv("ROUTE_REFRESH_INTERVAL", "45")
	t.Setenv("HTTP_TIMEOUT", "5s")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv returned error: %v", err)
	}
	if cfg.APIBaseURL != "https://devops.example.com/platform" || cfg.TraefikMetricsURL != "https://metrics.example.com/metrics" {
		t.Fatalf("explicit URLs = %#v", cfg)
	}
	if cfg.ScrapeInterval != 30*time.Second || cfg.RouteRefreshInterval != 45*time.Second || cfg.HTTPTimeout != 5*time.Second {
		t.Fatalf("explicit durations = %#v", cfg)
	}
}

func TestConfigFromEnvRejectsInvalidExplicitValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "missing API URL", key: "API_BASE_URL", value: "", want: "API_BASE_URL is required"},
		{name: "relative API URL", key: "API_BASE_URL", value: "devops.example.com", want: "absolute http or https URL"},
		{name: "API URL query", key: "API_BASE_URL", value: "https://devops.example.com?debug=true", want: "query parameters"},
		{name: "API URL port", key: "API_BASE_URL", value: "https://devops.example.com:70000", want: "port must be between"},
		{name: "metrics URL scheme", key: "TRAEFIK_METRICS_URL", value: "ftp://metrics.example.com/metrics", want: "absolute http or https URL"},
		{name: "missing report token", key: "REPORT_TOKEN", value: "", want: "REPORT_TOKEN is required"},
		{name: "missing cluster ID", key: "RUNTIME_CLUSTER_ID", value: "", want: "RUNTIME_CLUSTER_ID is required"},
		{name: "mode", key: "MODE", value: "prometheus", want: "unsupported MODE"},
		{name: "controller type", key: "CONTROLLER_TYPE", value: "nginx", want: "CONTROLLER_TYPE"},
		{name: "namespace", key: "GATEWAY_NAMESPACE", value: "Edge_System", want: "GATEWAY_NAMESPACE"},
		{name: "listen address", key: "PROBE_ADDR", value: "9090", want: "PROBE_ADDR"},
		{name: "listen port", key: "PROBE_ADDR", value: ":70000", want: "between 1 and 65535"},
		{name: "scrape duration syntax", key: "SCRAPE_INTERVAL", value: "later", want: "positive duration"},
		{name: "scrape duration minimum", key: "SCRAPE_INTERVAL", value: "5s", want: "at least 10s"},
		{name: "route refresh duration", key: "ROUTE_REFRESH_INTERVAL", value: "0", want: "positive duration"},
		{name: "HTTP timeout", key: "HTTP_TIMEOUT", value: "-1s", want: "positive duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidProbeEnvironment(t)
			t.Setenv(tt.key, tt.value)
			_, err := ConfigFromEnv()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ConfigFromEnv error = %v, want %q", err, tt.want)
			}
		})
	}
}

func setValidProbeEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("API_BASE_URL", "https://devops.example.com/")
	t.Setenv("REPORT_TOKEN", "token")
	t.Setenv("RUNTIME_CLUSTER_ID", "rcl_1")
	for _, key := range []string{
		"MODE",
		"CONTROLLER_TYPE",
		"GATEWAY_NAMESPACE",
		"TRAEFIK_METRICS_URL",
		"PROBE_ADDR",
		"SCRAPE_INTERVAL",
		"ROUTE_REFRESH_INTERVAL",
		"HTTP_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
}
