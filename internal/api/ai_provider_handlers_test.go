package api

import (
	"encoding/json"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
)

func TestAIProviderModelsIncludeAgentRequiredTokenLimits(t *testing.T) {
	encoded, err := json.Marshal(aiProviderModels([]model.AIModel{{
		ID:                           "aimod_test",
		Name:                         "test-model",
		MaxContextTokens:             524_288,
		MaxOutputTokens:              65_536,
		InputCreditsPerMillion:       decimal.RequireFromString("1.25"),
		OutputCreditsPerMillion:      decimal.RequireFromString("2.5"),
		CachedInputCreditsPerMillion: decimal.RequireFromString("0.5"),
	}}))
	if err != nil {
		t.Fatalf("marshal provider models: %v", err)
	}

	var response []struct {
		ID                           string `json:"id"`
		Name                         string `json:"name"`
		MaxContextTokens             *int64 `json:"maxContextTokens"`
		MaxOutputTokens              *int64 `json:"maxOutputTokens"`
		InputCreditsPerMillion       string `json:"inputCreditsPerMillion"`
		OutputCreditsPerMillion      string `json:"outputCreditsPerMillion"`
		CachedInputCreditsPerMillion string `json:"cachedInputCreditsPerMillion"`
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
		got.CachedInputCreditsPerMillion != "0.5" {
		t.Fatalf("model prices = %#v", got)
	}
}

func TestAIProviderInternalOpenAPIRequiresCapabilityPolicies(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	internal := schemas["AIProviderInternalConfig"].(map[string]any)
	provider := internal["properties"].(map[string]any)["provider"].(map[string]any)
	required, _ := schemaStringList(provider["required"])
	for _, field := range []string{"providerCompatibility", "promptCacheKeyMode"} {
		if !containsSchemaField(required, field) {
			t.Fatalf("AIProviderInternalConfig.provider does not require %s: %#v", field, required)
		}
	}

	wants := map[string][]string{
		"AIProviderCompatibility": {"auto", "openai", "deepseek"},
		"AIPromptCacheKeyMode":    {"auto", "enabled", "disabled"},
	}
	for name, want := range wants {
		schema := schemas[name].(map[string]any)
		got, _ := schemaStringList(schema["enum"])
		if len(got) != len(want) {
			t.Fatalf("%s enum = %#v, want %#v", name, got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("%s enum = %#v, want %#v", name, got, want)
			}
		}
	}

	runtime := internal["properties"].(map[string]any)["runtime"].(map[string]any)
	if additionalProperties, ok := runtime["additionalProperties"].(bool); !ok || additionalProperties {
		t.Fatalf("AIProviderInternalConfig.runtime additionalProperties = %#v, want false", runtime["additionalProperties"])
	}
	runtimeFields := map[string][2]float64{
		"providerTimeoutMs":                    {1_000, 900_000},
		"maxRequestRetries":                    {0, 10},
		"runTimeoutMs":                         {30_000, 7_200_000},
		"agentConcurrentRuns":                  {1, 100},
		"userConcurrentRuns":                   {1, 100},
		"assistantMaxOutputTokens":             {256, 131_072},
		"maxModelSteps":                        {1, 1_024},
		"runMaxToolCalls":                      {32, 2_048},
		"maxInputBytes":                        {8_192, 8_388_608},
		"navigateActionTtlSeconds":             {10, 600},
		"maxCardRepairAttempts":                {1, 10},
		"contextMaxUncompressedTurnCount":      {4, 128},
		"contextMaxCompressionTurnsPerCompile": {8, 1_024},
		"contextSummaryMaxOutputTokens":        {200, 32_768},
	}
	runtimeRequired, _ := schemaStringList(runtime["required"])
	if len(runtimeRequired) != len(runtimeFields) {
		t.Fatalf("AIProviderInternalConfig.runtime required = %#v, want exactly %d fields", runtimeRequired, len(runtimeFields))
	}
	runtimeProperties := runtime["properties"].(map[string]any)
	if len(runtimeProperties) != len(runtimeFields) {
		t.Fatalf("AIProviderInternalConfig.runtime properties = %#v, want exactly %d fields", runtimeProperties, len(runtimeFields))
	}
	mappedRuntime := aiProviderRuntimeConfig(aiConfigDefaults())
	if len(mappedRuntime) != len(runtimeFields) {
		t.Fatalf("AI Provider runtime response = %#v, want exactly %d fields", mappedRuntime, len(runtimeFields))
	}
	for field, bounds := range runtimeFields {
		if !containsSchemaField(runtimeRequired, field) {
			t.Fatalf("AIProviderInternalConfig.runtime does not require %s: %#v", field, runtimeRequired)
		}
		property, ok := runtimeProperties[field].(map[string]any)
		if !ok {
			t.Fatalf("AIProviderInternalConfig.runtime property %s missing", field)
		}
		if property["minimum"] != bounds[0] || property["maximum"] != bounds[1] {
			t.Fatalf("AIProviderInternalConfig.runtime.%s bounds = [%v,%v], want [%v,%v]", field, property["minimum"], property["maximum"], bounds[0], bounds[1])
		}
		value, ok := mappedRuntime[field].(int)
		if !ok || float64(value) < bounds[0] || float64(value) > bounds[1] {
			t.Fatalf("AI Provider runtime response %s = %#v, want an integer in [%v,%v]", field, mappedRuntime[field], bounds[0], bounds[1])
		}
	}
}

func TestAIProviderConfigVersionIncludesRuntimePolicy(t *testing.T) {
	values := map[string]string{
		"ai.provider.base_url":                 "https://example.com/v1",
		"ai.provider.compatibility":            "auto",
		"ai.provider.prompt_cache_key_mode":    "auto",
		"ai.provider.channel_affinity_enabled": "true",
		"ai.runtime.provider_timeout_seconds":  "30",
		"ai.runtime.max_request_retries":       "5",
		"ai.runtime.run_timeout_seconds":       "300",
		"ai.runtime.agent_concurrent_runs":     "2",
	}
	initial := aiProviderConfigVersion(values, "secret-v1")
	values["ai.runtime.agent_concurrent_runs"] = "3"
	updated := aiProviderConfigVersion(values, "secret-v1")
	if initial == updated {
		t.Fatal("runtime policy change did not update Provider config version")
	}
	values["ai.provider.channel_affinity_enabled"] = "false"
	withoutAffinity := aiProviderConfigVersion(values, "secret-v1")
	if updated == withoutAffinity {
		t.Fatal("channel affinity policy change did not update Provider config version")
	}
	values["ai.provider.compatibility"] = "deepseek"
	withExplicitCompatibility := aiProviderConfigVersion(values, "secret-v1")
	if withoutAffinity == withExplicitCompatibility {
		t.Fatal("Provider compatibility change did not update Provider config version")
	}
	values["ai.provider.prompt_cache_key_mode"] = "enabled"
	withPromptCacheKey := aiProviderConfigVersion(values, "secret-v1")
	if withExplicitCompatibility == withPromptCacheKey {
		t.Fatal("prompt cache key policy change did not update Provider config version")
	}
}

func TestAIProviderSelectConfigPreservesValidValuesAndDefaultsInvalidStorage(t *testing.T) {
	values := aiConfigDefaults()
	values["ai.provider.compatibility"] = "deepseek"
	values["ai.provider.prompt_cache_key_mode"] = "enabled"
	if got := aiProviderSelectConfig(values, "ai.provider.compatibility"); got != "deepseek" {
		t.Fatalf("Provider compatibility = %q, want deepseek", got)
	}
	if got := aiProviderSelectConfig(values, "ai.provider.prompt_cache_key_mode"); got != "enabled" {
		t.Fatalf("prompt cache key mode = %q, want enabled", got)
	}
	values["ai.provider.compatibility"] = "legacy-invalid"
	if got := aiProviderSelectConfig(values, "ai.provider.compatibility"); got != "auto" {
		t.Fatalf("invalid Provider compatibility = %q, want auto", got)
	}
}

func TestAIProviderRuntimeConfigKeepsValidValues(t *testing.T) {
	values := aiConfigDefaults()
	values["ai.runtime.provider_timeout_seconds"] = "45"
	values["ai.quota.run_max_tool_calls"] = "2048"

	runtime := aiProviderRuntimeConfig(values)
	if got := runtime["providerTimeoutMs"]; got != 45_000 {
		t.Fatalf("providerTimeoutMs = %v, want 45000", got)
	}
	if got := runtime["runMaxToolCalls"]; got != 2048 {
		t.Fatalf("runMaxToolCalls = %v, want 2048", got)
	}
}

func TestAIProviderRuntimeConfigNormalizesLegacyValuesWithoutMutatingStorage(t *testing.T) {
	values := aiConfigDefaults()
	values["ai.runtime.provider_timeout_seconds"] = "0"
	values["ai.runtime.max_request_retries"] = "not-a-number"
	values["ai.quota.run_max_tool_calls"] = "20"
	values["ai.run.max_input_k_bytes"] = "9000"

	runtime := aiProviderRuntimeConfig(values)
	wants := map[string]any{
		"providerTimeoutMs": 300_000,
		"maxRequestRetries": 5,
		"runMaxToolCalls":   256,
		"maxInputBytes":     1024 * 1024,
	}
	for key, want := range wants {
		if got := runtime[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if got := values["ai.quota.run_max_tool_calls"]; got != "20" {
		t.Fatalf("stored legacy value was mutated to %q", got)
	}
}

func TestAIProviderRuntimeConfigOmitsAgentLocalContextPolicy(t *testing.T) {
	values := aiConfigDefaults()
	runtime := aiProviderRuntimeConfig(values)
	for _, key := range []string{
		"toolResultPayloadBudget",
		"contextCompressionTriggerRatio",
		"contextRecentTurnCount",
	} {
		if _, ok := runtime[key]; ok {
			t.Errorf("runtime must not publish Agent-local setting %q", key)
		}
	}
}

func TestAIProviderRuntimeConfigDefaultsHaveValidationContracts(t *testing.T) {
	// Calling the complete mapper proves every runtime key has a definition,
	// a parseable default and a shared read/write range contract.
	runtime := aiProviderRuntimeConfig(aiConfigDefaults())
	if len(runtime) != 14 {
		t.Fatalf("runtime field count = %d, want 14", len(runtime))
	}
}
