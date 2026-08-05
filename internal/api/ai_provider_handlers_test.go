package api

import "testing"

func TestAIProviderConfigVersionIncludesRuntimePolicy(t *testing.T) {
	values := map[string]string{
		"ai.provider.base_url":                "https://example.com/v1",
		"ai.provider.default_model":           "example-model",
		"ai.runtime.provider_timeout_seconds": "30",
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
