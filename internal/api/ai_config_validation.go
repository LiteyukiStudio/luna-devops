package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/security"
)

func containsAIConfig[T any](values map[string]T) bool {
	for key := range values {
		if strings.HasPrefix(key, "ai.") {
			return true
		}
	}
	return false
}

func (h *Handlers) validateAIConfigValues(values map[string]string) error {
	current := h.configs.get(knownConfigKeys())
	for key, value := range values {
		current[key] = value
	}
	if raw := strings.TrimSpace(current["ai.provider.base_url"]); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" {
			return fmt.Errorf("ai.provider.base_url must be an HTTPS URL without user info")
		}
		allowlist := splitConfigList(current["security.egress.domainAllowList"])
		policy := security.PublicEgressPolicy()
		policy.DomainAllowList = allowlist
		policy.DomainBlockList = splitConfigList(current["security.egress.domainBlockList"])
		policy.IPAllowList = splitConfigList(current["security.egress.ipAllowList"])
		policy.IPBlockList = splitConfigList(current["security.egress.ipBlockList"])
		policy.AllowedPorts = []int{443}
		if _, err := policy.ValidateURL(raw); err != nil {
			return fmt.Errorf("ai.provider.base_url is blocked by egress policy")
		}
	}
	if configBool(current["ai.web.proxy_enabled"]) && current["ai.web.proxy_pool"] != "true" {
		return fmt.Errorf("ai.web.proxy_pool is required when the proxy pool is enabled")
	}
	for key, bounds := range map[string][2]int{
		"ai.runtime.provider_timeout_seconds": {1, 120},
		"ai.runtime.run_timeout_seconds":      {30, 900},
		"ai.runtime.agent_concurrent_runs":    {1, 10},
		"ai.quota.user_concurrent_runs":       {1, 10},
		"ai.quota.user_daily_tokens":          {1000, 10000000},
		"ai.quota.project_concurrent_runs":    {1, 50},
		"ai.quota.run_max_tool_calls":         {1, 100},
		"ai.retention.conversation_days":      {0, 365},
		"ai.retention.run_event_days":         {0, 90},
		"ai.retention.checkpoint_days":        {1, 30},
	} {
		number, err := strconv.Atoi(strings.TrimSpace(current[key]))
		if err != nil || number < bounds[0] || number > bounds[1] {
			return fmt.Errorf("%s must be between %d and %d", key, bounds[0], bounds[1])
		}
	}
	for _, key := range []string{"ai.quota.platform_daily_cost_soft", "ai.quota.platform_daily_cost_hard"} {
		number, err := strconv.ParseFloat(strings.TrimSpace(current[key]), 64)
		if err != nil || number < 0 {
			return fmt.Errorf("%s must be a non-negative number", key)
		}
	}
	if current["ai.access.mode"] == "all_authenticated" {
		hard, _ := strconv.ParseFloat(strings.TrimSpace(current["ai.quota.platform_daily_cost_hard"]), 64)
		if hard <= 0 {
			return fmt.Errorf("all_authenticated access requires a positive hard daily cost limit")
		}
	}
	for _, key := range []string{"ai.access.user_ids", "ai.access.project_ids"} {
		var value []any
		if json.Unmarshal([]byte(current[key]), &value) != nil {
			return fmt.Errorf("%s must be a JSON array", key)
		}
	}
	return nil
}
