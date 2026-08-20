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

func TestAIProviderRuntimeConfigKeepsValidValues(t *testing.T) {
	values := aiConfigDefaults()
	values["ai.runtime.provider_timeout_seconds"] = "45"
	values["ai.runtime.context_input_k_tokens"] = "2048"
	values["ai.quota.run_max_tool_calls"] = "2048"
	values["ai.run.max_credits"] = "125.5"

	runtime := aiProviderRuntimeConfig(values)
	if got := runtime["providerTimeoutMs"]; got != 45_000 {
		t.Fatalf("providerTimeoutMs = %v, want 45000", got)
	}
	if got := runtime["contextInputTokenBudget"]; got != 2048*1024 {
		t.Fatalf("contextInputTokenBudget = %v, want %d", got, 2048*1024)
	}
	if got := runtime["runMaxToolCalls"]; got != 2048 {
		t.Fatalf("runMaxToolCalls = %v, want 2048", got)
	}
	if got := runtime["runTotalCreditBudget"]; got != "125.5" {
		t.Fatalf("runTotalCreditBudget = %v, want 125.5", got)
	}
}

func TestAIProviderRuntimeConfigNormalizesLegacyValuesWithoutMutatingStorage(t *testing.T) {
	values := aiConfigDefaults()
	values["ai.runtime.provider_timeout_seconds"] = "0"
	values["ai.runtime.max_request_retries"] = "not-a-number"
	values["ai.runtime.context_input_k_tokens"] = "2049"
	values["ai.quota.run_max_tool_calls"] = "20"
	values["ai.run.max_total_tokens"] = "1"
	values["ai.run.max_credits"] = "invalid"
	values["ai.run.max_input_k_bytes"] = "9000"
	values["ai.context.compression_trigger_ratio"] = "0.5"
	values["ai.context.compression_target_ratio"] = "0.6"
	values["ai.context.recent_turn_count"] = "32"
	values["ai.context.max_recent_turn_count"] = "2"

	runtime := aiProviderRuntimeConfig(values)
	wants := map[string]any{
		"providerTimeoutMs":              300_000,
		"maxRequestRetries":              5,
		"contextInputTokenBudget":        1024 * 1024,
		"runMaxToolCalls":                256,
		"runTotalTokenBudget":            2_000_000,
		"runTotalCreditBudget":           "10000",
		"maxInputBytes":                  1024 * 1024,
		"contextCompressionTriggerRatio": 0.9,
		"contextCompressionTargetRatio":  0.7,
		"contextRecentTurnCount":         16,
		"contextMaxRecentTurnCount":      32,
	}
	for key, want := range wants {
		if got := runtime[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if got := values["ai.quota.run_max_tool_calls"]; got != "20" {
		t.Fatalf("stored legacy value was mutated to %q", got)
	}
	if got := values["ai.run.max_credits"]; got != "invalid" {
		t.Fatalf("stored legacy credit value was mutated to %q", got)
	}
}

func TestAIProviderRuntimeConfigNormalizesNonFiniteLegacyRatios(t *testing.T) {
	values := aiConfigDefaults()
	values["ai.context.compression_trigger_ratio"] = "NaN"
	values["ai.context.compression_target_ratio"] = "+Inf"
	runtime := aiProviderRuntimeConfig(values)
	if got := runtime["contextCompressionTriggerRatio"]; got != 0.9 {
		t.Fatalf("contextCompressionTriggerRatio = %v, want 0.9", got)
	}
	if got := runtime["contextCompressionTargetRatio"]; got != 0.7 {
		t.Fatalf("contextCompressionTargetRatio = %v, want 0.7", got)
	}
}

func TestAIProviderRuntimeConfigDefaultsHaveValidationContracts(t *testing.T) {
	// Calling the complete mapper proves every runtime key has a definition,
	// a parseable default and a shared read/write range contract.
	runtime := aiProviderRuntimeConfig(aiConfigDefaults())
	if len(runtime) != 24 {
		t.Fatalf("runtime field count = %d, want 24", len(runtime))
	}
}
