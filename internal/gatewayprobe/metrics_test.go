package gatewayprobe

import (
	"strings"
	"testing"
)

func TestParseTraefikMetricsMatchesHTTPRouteCandidates(t *testing.T) {
	routes := []RouteRef{{
		ID:         "gwr_123",
		Namespace:  "ns-demo",
		Name:       "liteyuki-gateway-gwr-123",
		Candidates: routeCandidates("gwr_123", "ns-demo", "liteyuki-gateway-gwr-123", []string{"demo.example.com"}),
	}}
	raw := `
# HELP traefik_router_responses_bytes_total Response bytes.
# TYPE traefik_router_responses_bytes_total counter
traefik_router_responses_bytes_total{router="ns-demo-liteyuki-gateway-gwr-123-web@kubernetesgateway",code="200"} 2048
# HELP traefik_router_requests_total Requests.
# TYPE traefik_router_requests_total counter
traefik_router_requests_total{router="ns-demo-liteyuki-gateway-gwr-123-web@kubernetesgateway",code="200"} 8
traefik_router_requests_total{router="unmanaged@kubernetesgateway",code="200"} 9
`
	counters, err := ParseTraefikMetrics(strings.NewReader(raw), routes)
	if err != nil {
		t.Fatalf("ParseTraefikMetrics returned error: %v", err)
	}
	got := counters["gwr_123"]
	if got.ResponseBytes != 2048 || got.RequestCount != 8 {
		t.Fatalf("counters = %#v", got)
	}
}

func TestPositiveCounterDeltaHandlesReset(t *testing.T) {
	if got := positiveCounterDelta(120, 100); got != 20 {
		t.Fatalf("delta = %d", got)
	}
	if got := positiveCounterDelta(5, 100); got != 5 {
		t.Fatalf("reset delta = %d", got)
	}
}
