package api

import (
	"encoding/json"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
)

func TestAIProviderModelsIncludeAgentRequiredTokenLimits(t *testing.T) {
	encoded, err := json.Marshal(aiProviderModels([]model.AIModel{{
		ID:                            "aimod_test",
		Name:                          "test-model",
		MaxContextTokens:              524_288,
		MaxOutputTokens:               65_536,
		InputCreditsPerMillion:        decimal.RequireFromString("1.25"),
		OutputCreditsPerMillion:       decimal.RequireFromString("2.5"),
		CachedInputCreditsPerMillion:  decimal.RequireFromString("0.5"),
		CachedOutputCreditsPerMillion: decimal.RequireFromString("0.75"),
	}}))
	if err != nil {
		t.Fatalf("marshal provider models: %v", err)
	}

	var response []struct {
		ID                            string `json:"id"`
		Name                          string `json:"name"`
		MaxContextTokens              *int64 `json:"maxContextTokens"`
		MaxOutputTokens               *int64 `json:"maxOutputTokens"`
		InputCreditsPerMillion        string `json:"inputCreditsPerMillion"`
		OutputCreditsPerMillion       string `json:"outputCreditsPerMillion"`
		CachedInputCreditsPerMillion  string `json:"cachedInputCreditsPerMillion"`
		CachedOutputCreditsPerMillion string `json:"cachedOutputCreditsPerMillion"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("unmarshal provider models: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("provider model count = %d, want 1", len(response))
	}

	got := response[0]
	if got.MaxContextTokens == nil || *got.MaxContextTokens != 524_288 {
		t.Fatalf("maxContextTokens = %v, want 524288", got.MaxContextTokens)
	}
	if got.MaxOutputTokens == nil || *got.MaxOutputTokens != 65_536 {
		t.Fatalf("maxOutputTokens = %v, want 65536", got.MaxOutputTokens)
	}
	if got.ID != "aimod_test" || got.Name != "test-model" {
		t.Fatalf("model identity = %q/%q", got.ID, got.Name)
	}
	if got.InputCreditsPerMillion != "1.25" || got.OutputCreditsPerMillion != "2.5" ||
		got.CachedInputCreditsPerMillion != "0.5" || got.CachedOutputCreditsPerMillion != "0.75" {
		t.Fatalf("model prices = %#v", got)
	}
}

func TestAIProviderConfigVersionIncludesRuntimePolicy(t *testing.T) {
	values := map[string]string{
		"ai.provider.base_url":                "https://example.com/v1",
		"ai.provider.default_model":           "example-model",
		"ai.runtime.provider_timeout_seconds": "30",
		"ai.runtime.max_request_retries":      "5",
		"ai.runtime.run_timeout_seconds":      "300",
		"ai.runtime.agent_concurrent_runs":    "2",
		"ai.runtime.context_input_k_tokens":   "256",
	}
	initial := aiProviderConfigVersion(values, "secret-v1")
	values["ai.runtime.agent_concurrent_runs"] = "3"
	updated := aiProviderConfigVersion(values, "secret-v1")
	if initial == updated {
		t.Fatal("runtime policy change did not update Provider config version")
	}
}

func TestAIProviderConfigContextBudgetUsesKTokens(t *testing.T) {
	values := map[string]string{"context": "256"}
	if got := aiRuntimeKTokens(values, "context", 128); got != 262144 {
		t.Fatalf("context input token budget = %d, want 262144", got)
	}
}

func TestAIRuntimeValueConversion(t *testing.T) {
	values := map[string]string{
		"timeout":     "45",
		"concurrency": "4",
		"invalid":     "not-a-number",
	}
	if got := aiRuntimeMilliseconds(values, "timeout", 30); got != 45000 {
		t.Fatalf("timeout = %d, want 45000", got)
	}
	if got := aiRuntimeInteger(values, "concurrency", 2); got != 4 {
		t.Fatalf("concurrency = %d, want 4", got)
	}
	if got := aiRuntimeInteger(values, "invalid", 2); got != 2 {
		t.Fatalf("invalid fallback = %d, want 2", got)
	}
}
