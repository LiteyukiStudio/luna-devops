package gatewayprobe

import "testing"

func TestConfigFromEnvDerivesTraefikMetricsURLFromGatewayNamespace(t *testing.T) {
	t.Setenv("API_BASE_URL", "https://devops.example.com/")
	t.Setenv("REPORT_TOKEN", "token")
	t.Setenv("RUNTIME_CLUSTER_ID", "rcl_1")
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
