package api

import (
	"strings"
	"testing"
)

func TestAgentObservabilitySummaryQueriesUseSelectedRange(t *testing.T) {
	queries := agentObservabilitySummaryQueries("6h")
	for key, query := range queries {
		if !strings.Contains(query, "[6h]") {
			t.Fatalf("query %s does not use selected range: %s", key, query)
		}
		if strings.Contains(query, "[5m]") || strings.Contains(query, "rate(") {
			t.Fatalf("query %s still uses a rolling rate: %s", key, query)
		}
	}
	for _, key := range []string{"inputTokens", "outputTokens", "toolCalls", "runDurationP95"} {
		if queries[key] == "" {
			t.Fatalf("missing summary query %s", key)
		}
	}
}
