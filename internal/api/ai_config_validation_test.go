package api

import (
	"strings"
	"testing"
)

func aiConfigDefaults() map[string]string {
	defaults := make(map[string]string, len(configDefinitions))
	for _, definition := range configDefinitions {
		defaults[definition.Key] = definition.Default
	}
	return defaults
}

func TestAIConfigDefinitionsCoverSpecificationCatalog(t *testing.T) {
	expected := []string{
		"ai.assistant.enabled", "ai.provider.base_url", "ai.provider.api_key",
		"ai.web.proxy_enabled", "ai.web.proxy_pool",
		"ai.runtime.provider_timeout_seconds", "ai.runtime.run_timeout_seconds", "ai.runtime.agent_concurrent_runs",
		"ai.runtime.max_request_retries",
		"ai.observability.enabled", "ai.observability.prometheus_url", "ai.observability.prometheus_token",
		"ai.observability.loki_url", "ai.observability.loki_tenant_id", "ai.observability.loki_token",
		"ai.observability.tempo_url", "ai.observability.tempo_tenant_id", "ai.observability.tempo_token",
		"ai.access.mode",
		"ai.quota.user_concurrent_runs", "ai.quota.user_daily_tokens", "ai.quota.project_concurrent_runs",
		"ai.quota.run_max_tool_calls", "ai.quota.platform_daily_cost_soft", "ai.quota.platform_daily_cost_hard",
		"ai.retention.conversation_days", "ai.retention.run_event_days", "ai.retention.checkpoint_days",
		"ai.context.max_uncompressed_turn_count", "ai.context.max_compression_turns_per_compile",
		"ai.context.summary_max_output_tokens",
		"ai.model.max_output_tokens",
		"ai.run.max_model_steps", "ai.run.max_input_k_bytes", "ai.run.navigate_action_ttl_seconds",
		"ai.tools.max_card_repair_attempts",
	}
	for _, key := range expected {
		if definition := configDefinitionByKey(key); definition == nil {
			t.Errorf("missing AI config definition %s", key)
		}
	}
	if got := aiConfigDefaults()["ai.quota.run_max_tool_calls"]; got != "256" {
		t.Fatalf("Run tool-call guard default = %q, want 256", got)
	}
}

func TestAINumericConfigDefinitionsHaveStrictWriteBounds(t *testing.T) {
	for _, definition := range configDefinitions {
		if !strings.HasPrefix(definition.Key, "ai.") || definition.Type != "number" {
			continue
		}
		_, integerBounded := aiIntegerConfigBounds[definition.Key]
		costBounded := definition.Key == "ai.quota.platform_daily_cost_soft" || definition.Key == "ai.quota.platform_daily_cost_hard"
		if !integerBounded && !costBounded {
			t.Errorf("AI numeric config %s has no strict write contract", definition.Key)
		}
	}
}

func TestAIProviderAPIKeyIsMaskedByConfigCache(t *testing.T) {
	cache := &configCache{values: map[string]string{
		"ai.provider.api_key": "secret-id:sec_private",
		"ai.web.proxy_pool":   "secret-id:sec_proxy_private",
	}}
	values := cache.get([]string{"ai.provider.api_key", "ai.web.proxy_pool"})
	for key, value := range values {
		if value != "true" || strings.Contains(value, "private") {
			t.Fatalf("masked %s = %q", key, value)
		}
	}
}

func TestAIConfigRejectsUnsafeProviderURLBeforeSaving(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	for _, raw := range []string{"http://api.example.com/v1", "https://user:pass@api.example.com/v1"} {
		if err := h.validateAIConfigValues(map[string]string{"ai.provider.base_url": raw}); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}

func TestAIConfigInputTypesAreStrictOnlyForSubmittedValues(t *testing.T) {
	for name, values := range map[string]map[string]any{
		"boolean object":         {"ai.assistant.enabled": map[string]any{"value": true}},
		"invalid boolean string": {"ai.web.proxy_enabled": "sometimes"},
		"null secret":            {"ai.provider.api_key": nil},
		"numeric tenant":         {"ai.observability.loki_tenant_id": 123},
		"numeric text setting":   {"ai.observability.loki_tenant_id": 100},
	} {
		if err := validateAIConfigInputTypes(values); err == nil {
			t.Errorf("invalid submitted AI config type accepted: %s", name)
		}
	}
	for name, values := range map[string]map[string]any{
		"native boolean":           {"ai.assistant.enabled": true},
		"canonical boolean string": {"ai.observability.enabled": "false"},
		"secret string":            {"ai.provider.api_key": "secret"},
		"text setting":             {"ai.observability.loki_tenant_id": "tenant-a"},
	} {
		if err := validateAIConfigInputTypes(values); err != nil {
			t.Errorf("valid submitted AI config type rejected for %s: %v", name, err)
		}
	}
}

func TestAIConfigAcceptsSafePublicProviderWithoutManualDomainAllowlist(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	if err := h.validateAIConfigValues(map[string]string{"ai.provider.base_url": "https://1.1.1.1/v1"}); err != nil {
		t.Fatalf("safe public Provider URL rejected: %v", err)
	}
}

func TestAIConfigRejectsUnsafeRuntimeBounds(t *testing.T) {
	for key, value := range map[string]string{
		"ai.runtime.provider_timeout_seconds":          "901",
		"ai.runtime.max_request_retries":               "11",
		"ai.runtime.run_timeout_seconds":               "10",
		"ai.runtime.agent_concurrent_runs":             "0",
		"ai.quota.run_max_tool_calls":                  "31",
		"ai.context.max_uncompressed_turn_count":       "3",
		"ai.context.max_compression_turns_per_compile": "7",
		"ai.context.summary_max_output_tokens":         "199",
		"ai.model.max_output_tokens":                   "131073",
		"ai.run.max_model_steps":                       "1025",
		"ai.run.max_input_k_bytes":                     "7",
		"ai.run.navigate_action_ttl_seconds":           "601",
		"ai.tools.max_card_repair_attempts":            "11",
	} {
		h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
		if err := h.validateAIConfigValues(map[string]string{key: value}); err == nil {
			t.Errorf("unsafe runtime setting accepted: %s=%s", key, value)
		}
	}
}

func TestAIConfigRejectsNonFiniteNumbers(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	for key, value := range map[string]string{
		"ai.quota.platform_daily_cost_soft": "NaN",
		"ai.quota.platform_daily_cost_hard": "+Inf",
	} {
		if err := h.validateAIConfigValues(map[string]string{key: value}); err == nil {
			t.Errorf("non-finite value accepted for %s", key)
		}
	}
}

func TestAIConfigAcceptsHighRunToolCallGuard(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	if err := h.validateAIConfigValues(map[string]string{"ai.quota.run_max_tool_calls": "2048"}); err != nil {
		t.Fatalf("high Run tool-call guard rejected: %v", err)
	}
	if err := h.validateAIConfigValues(map[string]string{"ai.quota.run_max_tool_calls": "2049"}); err == nil {
		t.Fatal("Run tool-call guard above hard limit was accepted")
	}
}

func TestAIConfigEditingUnrelatedFieldDoesNotRevalidateStoredBaseURL(t *testing.T) {
	// Regression: previously validateAIConfigValues merged the submission into the
	// full stored config and re-ran the egress/DNS check on ai.provider.base_url,
	// so editing an unrelated field failed whenever the
	// stored base_url no longer resolved to a public address.
	defaults := aiConfigDefaults()
	defaults["ai.provider.base_url"] = "https://api.internal-only.example/v1"
	h := &Handlers{configs: &configCache{values: defaults}}
	if err := h.validateAIConfigValues(map[string]string{"ai.runtime.max_request_retries": "4"}); err != nil {
		t.Fatalf("editing unrelated setting revalidated stored base_url: %v", err)
	}
}

func TestAIConfigRequiresProxyPoolWhenEnabled(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	if err := h.validateAIConfigValues(map[string]string{"ai.web.proxy_enabled": "true"}); err == nil {
		t.Fatal("enabled proxy pool without a configured secret was accepted")
	}
	if err := h.validateAIConfigValues(map[string]string{
		"ai.web.proxy_enabled": "true",
		"ai.web.proxy_pool":    "true",
	}); err != nil {
		t.Fatalf("configured proxy pool rejected: %v", err)
	}
}

func TestAIConfigRequiresAllObservabilitySourcesWhenEnabled(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	if err := h.validateAIConfigValues(map[string]string{"ai.observability.enabled": "true"}); err == nil {
		t.Fatal("Agent observability without query sources was accepted")
	}
	if err := h.validateAIConfigValues(map[string]string{
		"ai.observability.enabled":        "true",
		"ai.observability.prometheus_url": "http://prometheus:9090",
		"ai.observability.loki_url":       "http://loki:3100",
		"ai.observability.tempo_url":      "http://tempo:3200",
	}); err != nil {
		t.Fatalf("complete Agent observability configuration rejected: %v", err)
	}
}

func TestAIConfigDefaultsToAuthenticatedUsersAndAcceptsAdminRestriction(t *testing.T) {
	definition := configDefinitionByKey("ai.access.mode")
	if definition == nil || definition.Default != "all_authenticated" {
		t.Fatalf("AI access default = %#v", definition)
	}
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	if err := h.validateAIConfigValues(map[string]string{"ai.access.mode": "admins"}); err != nil {
		t.Fatalf("admin-only access mode rejected: %v", err)
	}
	if err := h.validateAIConfigValues(map[string]string{"ai.access.mode": "allowlist"}); err == nil {
		t.Fatal("unsupported access mode accepted")
	}
}
