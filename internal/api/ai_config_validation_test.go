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
		"ai.assistant.enabled", "ai.provider.base_url", "ai.provider.api_key", "ai.provider.default_model",
		"ai.web.proxy_enabled", "ai.web.proxy_pool",
		"ai.runtime.provider_timeout_seconds", "ai.runtime.run_timeout_seconds", "ai.runtime.agent_concurrent_runs",
		"ai.runtime.context_input_k_tokens", "ai.runtime.max_request_retries",
		"ai.observability.enabled", "ai.observability.prometheus_url", "ai.observability.prometheus_token",
		"ai.observability.loki_url", "ai.observability.loki_tenant_id", "ai.observability.loki_token",
		"ai.observability.tempo_url", "ai.observability.tempo_tenant_id", "ai.observability.tempo_token",
		"ai.access.mode",
		"ai.quota.user_concurrent_runs", "ai.quota.user_daily_tokens", "ai.quota.project_concurrent_runs",
		"ai.quota.run_max_tool_calls", "ai.quota.platform_daily_cost_soft", "ai.quota.platform_daily_cost_hard",
		"ai.retention.conversation_days", "ai.retention.run_event_days", "ai.retention.checkpoint_days",
		"ai.context.compression_trigger_ratio", "ai.context.compression_target_ratio",
		"ai.context.recent_turn_count", "ai.context.max_recent_turn_count",
		"ai.context.max_uncompressed_turn_count", "ai.context.max_compression_turns_per_compile",
		"ai.context.summary_input_k_tokens", "ai.context.summary_max_output_tokens",
		"ai.context.historical_tool_k_tokens",
		"ai.model.max_output_tokens",
		"ai.run.max_model_steps", "ai.run.max_input_k_bytes", "ai.run.navigate_action_ttl_seconds",
		"ai.tools.result_payload_k_bytes", "ai.tools.max_card_repair_attempts",
	}
	for _, key := range expected {
		if definition := configDefinitionByKey(key); definition == nil {
			t.Errorf("missing AI config definition %s", key)
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

func TestAIConfigAcceptsSafePublicProviderWithoutManualDomainAllowlist(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	if err := h.validateAIConfigValues(map[string]string{"ai.provider.base_url": "https://1.1.1.1/v1"}); err != nil {
		t.Fatalf("safe public Provider URL rejected: %v", err)
	}
}

func TestAIConfigRejectsUnsafeRuntimeBounds(t *testing.T) {
	for key, value := range map[string]string{
		"ai.runtime.provider_timeout_seconds":          "121",
		"ai.runtime.max_request_retries":               "11",
		"ai.runtime.run_timeout_seconds":               "10",
		"ai.runtime.agent_concurrent_runs":             "0",
		"ai.runtime.context_input_k_tokens":            "32",
		"ai.context.recent_turn_count":                 "0",
		"ai.context.max_recent_turn_count":             "1",
		"ai.context.max_uncompressed_turn_count":       "3",
		"ai.context.max_compression_turns_per_compile": "7",
		"ai.context.summary_input_k_tokens":            "3",
		"ai.context.summary_max_output_tokens":         "199",
		"ai.context.historical_tool_k_tokens":          "0",
		"ai.context.compression_trigger_ratio":         "0.99",
		"ai.context.compression_target_ratio":          "0.05",
		"ai.model.max_output_tokens":                   "16385",
		"ai.run.max_model_steps":                       "201",
		"ai.run.max_input_k_bytes":                     "7",
		"ai.run.navigate_action_ttl_seconds":           "601",
		"ai.tools.result_payload_k_bytes":              "3",
		"ai.tools.max_card_repair_attempts":            "11",
	} {
		h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
		if err := h.validateAIConfigValues(map[string]string{key: value}); err == nil {
			t.Errorf("unsafe runtime setting accepted: %s=%s", key, value)
		}
	}
}

func TestAIConfigRejectsInconsistentAdvancedContextSettings(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"trigger not greater than target": {
			"ai.context.compression_trigger_ratio": "0.5",
			"ai.context.compression_target_ratio":  "0.5",
		},
		"recent exceeds max recent": {
			"ai.context.recent_turn_count":     "8",
			"ai.context.max_recent_turn_count": "4",
		},
	} {
		h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
		if err := h.validateAIConfigValues(values); err == nil {
			t.Errorf("inconsistent advanced context settings accepted: %s", name)
		}
	}
}

func TestAIConfigAcceptsAdvancedContextSettingsWithinBounds(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	if err := h.validateAIConfigValues(map[string]string{
		"ai.context.compression_trigger_ratio": "0.9",
		"ai.context.compression_target_ratio":  "0.4",
		"ai.context.recent_turn_count":         "6",
		"ai.context.max_recent_turn_count":     "10",
	}); err != nil {
		t.Fatalf("valid advanced context settings rejected: %v", err)
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
