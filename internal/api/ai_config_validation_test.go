package api

import (
	"strings"
	"testing"
)

func TestAIConfigDefinitionsCoverSpecificationCatalog(t *testing.T) {
	expected := []string{
		"ai.assistant.enabled", "ai.provider.type", "ai.provider.base_url", "ai.provider.api_key",
		"ai.provider.default_model", "ai.provider.fallback_model", "ai.provider.model_pricing",
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

func TestAIDomainAllowlistRequiresExactOrSubdomainMatch(t *testing.T) {
	if !aiDomainAllowed("api.example.com", []string{"*.example.com"}) {
		t.Fatal("expected subdomain allowlist match")
	}
	for _, host := range []string{"example.com", "evil-example.com", "example.com.evil.test"} {
		if aiDomainAllowed(host, []string{"*.example.com"}) {
			t.Fatalf("unexpected allowlist match for %s", host)
		}
	}
}

func TestAIConfigRejectsUnsafeProviderURLBeforeSaving(t *testing.T) {
	h := &Handlers{configs: &configCache{values: map[string]string{
		"ai.provider.type": "", "ai.provider.base_url": "", "ai.provider.default_model": "",
		"ai.provider.fallback_model": "", "ai.provider.model_pricing": "[]",
		"ai.access.mode": "admins", "ai.access.user_ids": "[]", "ai.access.project_ids": "[]",
		"ai.quota.user_concurrent_runs": "2", "ai.quota.user_daily_tokens": "200000",
		"ai.quota.project_concurrent_runs": "5", "ai.quota.run_max_tool_calls": "20",
		"ai.quota.platform_daily_cost_soft": "0", "ai.quota.platform_daily_cost_hard": "0",
		"ai.retention.conversation_days": "90", "ai.retention.run_event_days": "30",
		"ai.retention.checkpoint_days": "7", "security.egress.domainAllowList": "api.example.com",
	}}}
	for _, raw := range []string{"http://api.example.com/v1", "https://user:pass@api.example.com/v1", "https://other.example.com/v1"} {
		if err := h.validateAIConfigValues(map[string]string{"ai.provider.base_url": raw}); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}
