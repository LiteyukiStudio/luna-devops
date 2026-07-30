package api

import (
	"testing"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestGatewayRouteStatusFromSummary(t *testing.T) {
	tests := map[string]string{
		"accepted": "ready",
		"pending":  "progressing",
		"failed":   "degraded",
		"":         "unknown",
	}
	for input, want := range tests {
		if got := gatewayRouteStatusFromSummary(input); got != want {
			t.Fatalf("gatewayRouteStatusFromSummary(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGatewayRouteConditionsCopiesProviderSnapshot(t *testing.T) {
	got := gatewayRouteConditions([]kubeprovider.RouteConditionSnapshot{{
		Type:               "Accepted",
		Status:             "True",
		Reason:             "Accepted",
		Message:            "ready",
		ObservedGeneration: 3,
	}})
	if len(got) != 1 || got[0].Type != "Accepted" || got[0].ObservedGeneration != 3 {
		t.Fatalf("gatewayRouteConditions() = %#v", got)
	}
}
