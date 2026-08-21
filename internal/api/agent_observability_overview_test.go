package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
	for _, key := range []string{"inputTokens", "outputTokens", "runDurationP95"} {
		if queries[key] == "" {
			t.Fatalf("missing summary query %s", key)
		}
	}
}

func TestAgentObservabilityUnavailableResponseIsStableAndNonCacheable(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	writeAgentObservabilityUnavailable(ctx, "ai.observability.trace_unavailable", "Trace detail is unavailable")

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", recorder.Code)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["status"] != "unavailable" || response["observationCode"] != "ai.observability.trace_unavailable" || response["code"] != "ai.observability.trace_unavailable" {
		t.Fatalf("response = %#v", response)
	}
	if response["requestId"] == "" || response["retryable"] != true {
		t.Fatalf("response metadata = %#v", response)
	}
}

func TestObservabilityRangeUsesAllowlistedWindows(t *testing.T) {
	tests := []struct {
		input    string
		wantText string
		want     time.Duration
	}{
		{input: "1h", wantText: "1h", want: time.Hour},
		{input: "6h", wantText: "6h", want: 6 * time.Hour},
		{input: "24h", wantText: "24h", want: 24 * time.Hour},
		{input: "7d", wantText: "7d", want: 7 * 24 * time.Hour},
		{input: "30d", wantText: "30d", want: 30 * 24 * time.Hour},
		{input: "1y", wantText: "1y", want: 365 * 24 * time.Hour},
		{input: "unbounded", wantText: "1h", want: time.Hour},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			gotText, got := observabilityRange(test.input)
			if gotText != test.wantText || got != test.want {
				t.Fatalf("observabilityRange(%q) = (%q, %s), want (%q, %s)", test.input, gotText, got, test.wantText, test.want)
			}
		})
	}
}
