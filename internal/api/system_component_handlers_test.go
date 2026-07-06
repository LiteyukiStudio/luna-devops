package api

import (
	"encoding/json"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestSystemComponentProbeEnvUsesCustomTraefikMetricsURL(t *testing.T) {
	envJSON := systemComponentProbeEnv(
		model.RuntimeCluster{ID: "clu_1", GatewayNamespace: "edge-system", GatewayControllerType: "traefik"},
		"gateway-traffic-probe",
		"traefik-metrics",
		"https://devops.example.com/",
		"http://traefik-metrics.edge-system.svc.cluster.local:8082/metrics",
	)
	var env map[string]string
	if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
		t.Fatalf("unmarshal env json: %v", err)
	}
	if env["API_BASE_URL"] != "https://devops.example.com" {
		t.Fatalf("API_BASE_URL = %q", env["API_BASE_URL"])
	}
	want := "http://traefik-metrics.edge-system.svc.cluster.local:8082/metrics"
	if env["TRAEFIK_METRICS_URL"] != want {
		t.Fatalf("TRAEFIK_METRICS_URL = %q, want %q", env["TRAEFIK_METRICS_URL"], want)
	}
}

func TestSystemComponentProbeEnvDerivesDefaultTraefikMetricsURL(t *testing.T) {
	envJSON := systemComponentProbeEnv(
		model.RuntimeCluster{ID: "clu_1", GatewayNamespace: "edge-system"},
		"gateway-traffic-probe",
		"traefik-metrics",
		"https://devops.example.com",
		"",
	)
	var env map[string]string
	if err := json.Unmarshal([]byte(envJSON), &env); err != nil {
		t.Fatalf("unmarshal env json: %v", err)
	}
	want := "http://traefik.edge-system.svc.cluster.local:9100/metrics"
	if env["TRAEFIK_METRICS_URL"] != want {
		t.Fatalf("TRAEFIK_METRICS_URL = %q, want %q", env["TRAEFIK_METRICS_URL"], want)
	}
}
