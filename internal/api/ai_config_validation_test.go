package api

import (
	"strings"
	"testing"
)

func TestAIConfigDefinitionsCoverSpecificationCatalog(t *testing.T) {
	expected := []string{
		"ai.assistant.enabled", "ai.provider.base_url", "ai.provider.api_key", "ai.provider.default_model",
		"ai.web.proxy_enabled", "ai.web.proxy_pool",
		"ai.runtime.provider_timeout_seconds", "ai.runtime.run_timeout_seconds", "ai.runtime.agent_concurrent_runs",
		"ai.runtime.context_input_k_tokens",
		"ai.observability.enabled", "ai.observability.prometheus_url", "ai.observability.prometheus_token",
		"ai.observability.loki_url", "ai.observability.loki_tenant_id", "ai.observability.loki_token",
		"ai.observability.tempo_url", "ai.observability.tempo_tenant_id", "ai.observability.tempo_token",
		"ai.access.mode",
		"ai.quota.user_concurrent_runs", "ai.quota.user_daily_tokens", "ai.quota.project_concurrent_runs",
		"ai.quota.run_max_tool_calls", "ai.quota.platform_daily_cost_soft", "ai.quota.platform_daily_cost_hard",
		"ai.retention.conversation_days", "ai.retention.run_event_days", "ai.retention.checkpoint_days",
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
	h := &Handlers{configs: &configCache{values: map[string]string{
		"ai.provider.base_url": "", "ai.provider.default_model": "",
		"ai.runtime.provider_timeout_seconds": "30", "ai.runtime.run_timeout_seconds": "300",
		"ai.runtime.agent_concurrent_runs":  "2",
		"ai.runtime.context_input_k_tokens": "256",
		"ai.access.mode":                    "all_authenticated",
		"ai.quota.user_concurrent_runs":     "2", "ai.quota.user_daily_tokens": "200000",
		"ai.quota.project_concurrent_runs": "5", "ai.quota.run_max_tool_calls": "20",
		"ai.quota.platform_daily_cost_soft": "0", "ai.quota.platform_daily_cost_hard": "0",
		"ai.retention.conversation_days": "90", "ai.retention.run_event_days": "30",
		"ai.retention.checkpoint_days": "7", "security.egress.domainAllowList": "api.example.com",
	}}}
	for _, raw := range []string{"http://api.example.com/v1", "https://user:pass@api.example.com/v1"} {
		if err := h.validateAIConfigValues(map[string]string{"ai.provider.base_url": raw}); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}

func TestAIConfigAcceptsSafePublicProviderWithoutManualDomainAllowlist(t *testing.T) {
	h := &Handlers{configs: &configCache{values: map[string]string{
		"ai.provider.base_url": "", "ai.provider.default_model": "",
		"ai.runtime.provider_timeout_seconds": "30", "ai.runtime.run_timeout_seconds": "300",
		"ai.runtime.agent_concurrent_runs":  "2",
		"ai.runtime.context_input_k_tokens": "256",
		"ai.access.mode":                    "all_authenticated",
		"ai.quota.user_concurrent_runs":     "2", "ai.quota.user_daily_tokens": "200000",
		"ai.quota.project_concurrent_runs": "5", "ai.quota.run_max_tool_calls": "20",
		"ai.quota.platform_daily_cost_soft": "0", "ai.quota.platform_daily_cost_hard": "0",
		"ai.retention.conversation_days": "90", "ai.retention.run_event_days": "30",
		"ai.retention.checkpoint_days": "7", "security.egress.domainAllowList": "",
	}}}
	if err := h.validateAIConfigValues(map[string]string{"ai.provider.base_url": "https://1.1.1.1/v1"}); err != nil {
		t.Fatalf("safe public Provider URL rejected: %v", err)
	}
}

func TestAIConfigRejectsUnsafeRuntimeBounds(t *testing.T) {
	for key, value := range map[string]string{
		"ai.runtime.provider_timeout_seconds": "121",
		"ai.runtime.run_timeout_seconds":      "10",
		"ai.runtime.agent_concurrent_runs":    "0",
		"ai.runtime.context_input_k_tokens":   "32",
	} {
		defaults := make(map[string]string, len(configDefinitions))
		for _, definition := range configDefinitions {
			defaults[definition.Key] = definition.Default
		}
		h := &Handlers{configs: &configCache{values: defaults}}
		if err := h.validateAIConfigValues(map[string]string{key: value}); err == nil {
			t.Errorf("unsafe runtime setting accepted: %s=%s", key, value)
		}
	}
}

func TestAIConfigRequiresProxyPoolWhenEnabled(t *testing.T) {
	defaults := make(map[string]string, len(configDefinitions))
	for _, definition := range configDefinitions {
		defaults[definition.Key] = definition.Default
	}
	h := &Handlers{configs: &configCache{values: defaults}}
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
	defaults := make(map[string]string, len(configDefinitions))
	for _, definition := range configDefinitions {
		defaults[definition.Key] = definition.Default
	}
	h := &Handlers{configs: &configCache{values: defaults}}
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
	defaults := make(map[string]string, len(configDefinitions))
	for _, item := range configDefinitions {
		defaults[item.Key] = item.Default
	}
	h := &Handlers{configs: &configCache{values: defaults}}
	if err := h.validateAIConfigValues(map[string]string{"ai.access.mode": "admins"}); err != nil {
		t.Fatalf("admin-only access mode rejected: %v", err)
	}
	if err := h.validateAIConfigValues(map[string]string{"ai.access.mode": "allowlist"}); err == nil {
		t.Fatal("unsupported access mode accepted")
	}
}
