package api

import (
	"strings"
	"testing"
)

func TestAIConfigDefinitionsCoverSpecificationCatalog(t *testing.T) {
	expected := []string{
		"ai.assistant.enabled", "ai.provider.base_url", "ai.provider.api_key", "ai.provider.default_model",
		"ai.runtime.provider_timeout_seconds", "ai.runtime.run_timeout_seconds", "ai.runtime.agent_concurrent_runs",
		"ai.access.mode", "ai.access.user_ids", "ai.access.project_ids",
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
	cache := &configCache{values: map[string]string{"ai.provider.api_key": "secret-id:sec_private"}}
	value := cache.get([]string{"ai.provider.api_key"})["ai.provider.api_key"]
	if value != "true" || strings.Contains(value, "sec_private") {
		t.Fatalf("masked API key = %q", value)
	}
}

func TestAIConfigRejectsUnsafeProviderURLBeforeSaving(t *testing.T) {
	h := &Handlers{configs: &configCache{values: map[string]string{
		"ai.provider.base_url": "", "ai.provider.default_model": "",
		"ai.runtime.provider_timeout_seconds": "30", "ai.runtime.run_timeout_seconds": "300",
		"ai.runtime.agent_concurrent_runs": "2",
		"ai.access.mode":                   "admins", "ai.access.user_ids": "[]", "ai.access.project_ids": "[]",
		"ai.quota.user_concurrent_runs": "2", "ai.quota.user_daily_tokens": "200000",
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
		"ai.runtime.agent_concurrent_runs": "2",
		"ai.access.mode":                   "admins", "ai.access.user_ids": "[]", "ai.access.project_ids": "[]",
		"ai.quota.user_concurrent_runs": "2", "ai.quota.user_daily_tokens": "200000",
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
