package api

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
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
		"ai.assistant.enabled", "ai.provider.base_url", "ai.provider.api_key", "ai.provider.compatibility",
		"ai.provider.prompt_cache_key_mode", "ai.provider.channel_affinity_enabled",
		"ai.web.proxy_enabled", "ai.web.proxy_pool",
		"ai.runtime.provider_timeout_seconds", "ai.runtime.run_timeout_seconds", "ai.runtime.agent_concurrent_runs",
		"ai.runtime.max_request_retries",
		"ai.observability.enabled", "ai.observability.prometheus_url", "ai.observability.prometheus_token",
		"ai.observability.loki_url", "ai.observability.loki_tenant_id", "ai.observability.loki_token",
		"ai.observability.tempo_url", "ai.observability.tempo_tenant_id", "ai.observability.tempo_token",
		"ai.access.mode",
		"ai.quota.user_concurrent_runs",
	}
	for _, key := range expected {
		if definition := configDefinitionByKey(key); definition == nil {
			t.Errorf("missing AI config definition %s", key)
		}
	}
	if got := aiConfigDefaults()["ai.provider.channel_affinity_enabled"]; got != "true" {
		t.Fatalf("channel affinity default = %q, want true", got)
	}
	for _, key := range []string{"ai.provider.compatibility", "ai.provider.prompt_cache_key_mode"} {
		if got := aiConfigDefaults()[key]; got != "auto" {
			t.Fatalf("%s default = %q, want auto", key, got)
		}
	}
}

func TestAINumericConfigDefinitionsHaveStrictWriteBounds(t *testing.T) {
	for _, definition := range configDefinitions {
		if !strings.HasPrefix(definition.Key, "ai.") || definition.Type != "number" {
			continue
		}
		if _, bounded := aiapi.IntegerConfigBounds()[definition.Key]; !bounded {
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
	h.domains = newDomainHandlers(h)
	for _, raw := range []string{"http://api.example.com/v1", "https://user:pass@api.example.com/v1"} {
		if err := h.domains.ai.ValidateAIConfigValues(map[string]string{"ai.provider.base_url": raw}); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}

func TestAIProviderCapabilityEnumsRejectUnknownValues(t *testing.T) {
	for key, values := range map[string][]string{
		"ai.provider.compatibility":         {"auto", "openai", "deepseek"},
		"ai.provider.prompt_cache_key_mode": {"auto", "enabled", "disabled"},
	} {
		definition := configDefinitionByKey(key)
		if definition == nil || definition.Type != "select" {
			t.Fatalf("%s is not a select definition", key)
		}
		for _, value := range values {
			if _, err := validateConfigValues(map[string]any{key: value}); err != nil {
				t.Errorf("%s rejected valid value %q: %v", key, value, err)
			}
		}
		if _, err := validateConfigValues(map[string]any{key: "unknown"}); err == nil {
			t.Errorf("%s accepted an unknown value", key)
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
		if err := aiapi.ValidateAIConfigInputTypes(values, aiConfigDefinition); err == nil {
			t.Errorf("invalid submitted AI config type accepted: %s", name)
		}
	}
	for name, values := range map[string]map[string]any{
		"native boolean":           {"ai.assistant.enabled": true},
		"canonical boolean string": {"ai.observability.enabled": "false"},
		"secret string":            {"ai.provider.api_key": "secret"},
		"text setting":             {"ai.observability.loki_tenant_id": "tenant-a"},
	} {
		if err := aiapi.ValidateAIConfigInputTypes(values, aiConfigDefinition); err != nil {
			t.Errorf("valid submitted AI config type rejected for %s: %v", name, err)
		}
	}
}

func TestAIConfigAcceptsSafePublicProviderWithoutManualDomainAllowlist(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	h.domains = newDomainHandlers(h)
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{"ai.provider.base_url": "https://1.1.1.1/v1"}); err != nil {
		t.Fatalf("safe public Provider URL rejected: %v", err)
	}
}

func TestAIConfigRejectsUnsafeRuntimeBounds(t *testing.T) {
	for key, value := range map[string]string{
		"ai.runtime.provider_timeout_seconds": "901",
		"ai.runtime.max_request_retries":      "11",
		"ai.runtime.run_timeout_seconds":      "10",
		"ai.runtime.agent_concurrent_runs":    "0",
		"ai.quota.user_concurrent_runs":       "0",
	} {
		h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
		h.domains = newDomainHandlers(h)
		if err := h.domains.ai.ValidateAIConfigValues(map[string]string{key: value}); err == nil {
			t.Errorf("unsafe runtime setting accepted: %s=%s", key, value)
		}
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
	h.domains = newDomainHandlers(h)
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{"ai.runtime.max_request_retries": "4"}); err != nil {
		t.Fatalf("editing unrelated setting revalidated stored base_url: %v", err)
	}
}

func TestAIConfigRequiresProxyPoolWhenEnabled(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	h.domains = newDomainHandlers(h)
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{"ai.web.proxy_enabled": "true"}); err == nil {
		t.Fatal("enabled proxy pool without a configured secret was accepted")
	}
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{
		"ai.web.proxy_enabled": "true",
		"ai.web.proxy_pool":    "true",
	}); err != nil {
		t.Fatalf("configured proxy pool rejected: %v", err)
	}
}

func TestAIConfigRequiresAllObservabilitySourcesWhenEnabled(t *testing.T) {
	h := &Handlers{configs: &configCache{values: aiConfigDefaults()}}
	h.domains = newDomainHandlers(h)
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{"ai.observability.enabled": "true"}); err == nil {
		t.Fatal("Agent observability without query sources was accepted")
	}
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{
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
	h.domains = newDomainHandlers(h)
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{"ai.access.mode": "admins"}); err != nil {
		t.Fatalf("admin-only access mode rejected: %v", err)
	}
	if err := h.domains.ai.ValidateAIConfigValues(map[string]string{"ai.access.mode": "allowlist"}); err == nil {
		t.Fatal("unsupported access mode accepted")
	}
}
