package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/fixeddecimal"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/shopspring/decimal"
)

func containsAIConfig[T any](values map[string]T) bool {
	for key := range values {
		if strings.HasPrefix(key, "ai.") {
			return true
		}
	}
	return false
}

// validateAIConfigValues validates only the keys present in values. Configuration
// values that depend on runtime network state (egress/DNS resolution) or that
// reference secrets are validated only when the caller submits them, so editing an
// unrelated field never fails because of an unrelated stored value.
func (h *Handlers) validateAIConfigValues(values map[string]string) error {
	// base_url is only validated when it is part of this submission; validating a
	// stored URL against the live egress policy on every unrelated change makes
	// editing unrelated fields fail spuriously when DNS/egress state changes.
	if raw, submitted := values["ai.provider.base_url"]; submitted {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			parsed, err := url.Parse(raw)
			if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" {
				return fmt.Errorf("ai.provider.base_url must be an HTTPS URL without user info")
			}
			allowlist := splitConfigList(h.configs.get([]string{"security.egress.domainAllowList"})["security.egress.domainAllowList"])
			policy := security.PublicEgressPolicy()
			policy.DomainAllowList = allowlist
			policy.DomainBlockList = splitConfigList(h.configs.get([]string{"security.egress.domainBlockList"})["security.egress.domainBlockList"])
			policy.IPAllowList = splitConfigList(h.configs.get([]string{"security.egress.ipAllowList"})["security.egress.ipAllowList"])
			policy.IPBlockList = splitConfigList(h.configs.get([]string{"security.egress.ipBlockList"})["security.egress.ipBlockList"])
			policy.AllowedPorts = []int{443}
			if _, err := policy.ValidateURL(raw); err != nil {
				return fmt.Errorf("ai.provider.base_url is blocked by egress policy")
			}
		}
	}
	if enabled, submitted := values["ai.web.proxy_enabled"]; submitted && configBool(enabled) {
		pool := strings.TrimSpace(values["ai.web.proxy_pool"])
		if pool == "" {
			pool = h.configs.get([]string{"ai.web.proxy_pool"})["ai.web.proxy_pool"]
		}
		if pool != "true" {
			return fmt.Errorf("ai.web.proxy_pool is required when the proxy pool is enabled")
		}
	}
	for _, key := range []string{
		"ai.observability.prometheus_url",
		"ai.observability.loki_url",
		"ai.observability.tempo_url",
	} {
		raw, submitted := values[key]
		if !submitted {
			// Only enforce presence when observability is being enabled in this submission.
			if enabled, ok := values["ai.observability.enabled"]; ok && configBool(enabled) {
				stored := strings.TrimSpace(h.configs.get([]string{key})[key])
				if stored == "" {
					return fmt.Errorf("%s is required when Agent observability is enabled", key)
				}
			}
			continue
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			if configBool(h.configs.get([]string{"ai.observability.enabled"})["ai.observability.enabled"]) || configBool(values["ai.observability.enabled"]) {
				return fmt.Errorf("%s is required when Agent observability is enabled", key)
			}
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("%s must be an HTTP(S) URL without user info, query, or fragment", key)
		}
	}
	for key, bounds := range map[string][2]int{
		"ai.runtime.provider_timeout_seconds":          {1, 900},
		"ai.runtime.max_request_retries":               {0, 10},
		"ai.runtime.run_timeout_seconds":               {30, 7200},
		"ai.runtime.agent_concurrent_runs":             {1, 100},
		"ai.runtime.context_input_k_tokens":            {64, 2048},
		"ai.quota.user_concurrent_runs":                {1, 100},
		"ai.quota.user_daily_tokens":                   {1000, 10000000},
		"ai.quota.project_concurrent_runs":             {1, 100},
		"ai.quota.run_max_tool_calls":                  {1, 100},
		"ai.retention.conversation_days":               {0, 365},
		"ai.retention.run_event_days":                  {0, 90},
		"ai.retention.checkpoint_days":                 {1, 30},
		"ai.context.recent_turn_count":                 {1, 32},
		"ai.context.max_recent_turn_count":             {2, 64},
		"ai.context.max_uncompressed_turn_count":       {4, 128},
		"ai.context.max_compression_turns_per_compile": {8, 1024},
		"ai.context.summary_input_k_tokens":            {4, 512},
		"ai.context.summary_max_output_tokens":         {200, 32768},
		"ai.context.historical_tool_k_tokens":          {1, 256},
		"ai.model.max_output_tokens":                   {256, 131072},
		"ai.run.max_model_steps":                       {1, 1024},
		"ai.run.max_total_tokens":                      {16384, 16000000},
		"ai.run.max_input_k_bytes":                     {8, 8192},
		"ai.run.navigate_action_ttl_seconds":           {10, 600},
		"ai.tools.result_payload_k_bytes":              {4, 4096},
		"ai.tools.max_card_repair_attempts":            {1, 10},
	} {
		raw, submitted := values[key]
		if !submitted {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || number < bounds[0] || number > bounds[1] {
			return fmt.Errorf("%s must be between %d and %d", key, bounds[0], bounds[1])
		}
	}
	for key, bounds := range map[string][2]float64{
		"ai.context.compression_trigger_ratio": {0.5, 0.95},
		"ai.context.compression_target_ratio":  {0.1, 0.8},
	} {
		raw, submitted := values[key]
		if !submitted {
			continue
		}
		ratio, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || ratio < bounds[0] || ratio > bounds[1] {
			return fmt.Errorf("%s must be between %.2f and %.2f", key, bounds[0], bounds[1])
		}
	}
	// Cross-field constraints are only evaluated when at least one of the fields
	// is part of this submission; the other side falls back to the stored value.
	if _, tSubmitted := values["ai.context.compression_trigger_ratio"]; tSubmitted || values["ai.context.compression_target_ratio"] != "" {
		triggerRaw := firstSubmittedOrStored(values, h, "ai.context.compression_trigger_ratio")
		targetRaw := firstSubmittedOrStored(values, h, "ai.context.compression_target_ratio")
		triggerRatio, triggerErr := strconv.ParseFloat(strings.TrimSpace(triggerRaw), 64)
		targetRatio, targetErr := strconv.ParseFloat(strings.TrimSpace(targetRaw), 64)
		if triggerErr == nil && targetErr == nil && triggerRatio <= targetRatio {
			return fmt.Errorf("ai.context.compression_trigger_ratio must be greater than ai.context.compression_target_ratio")
		}
	}
	if _, rSubmitted := values["ai.context.recent_turn_count"]; rSubmitted || values["ai.context.max_recent_turn_count"] != "" {
		recentRaw := firstSubmittedOrStored(values, h, "ai.context.recent_turn_count")
		maxRecentRaw := firstSubmittedOrStored(values, h, "ai.context.max_recent_turn_count")
		recentTurns, recentErr := strconv.Atoi(strings.TrimSpace(recentRaw))
		maxRecentTurns, maxRecentErr := strconv.Atoi(strings.TrimSpace(maxRecentRaw))
		if recentErr == nil && maxRecentErr == nil && recentTurns > maxRecentTurns {
			return fmt.Errorf("ai.context.recent_turn_count must not exceed ai.context.max_recent_turn_count")
		}
	}
	for _, key := range []string{"ai.quota.platform_daily_cost_soft", "ai.quota.platform_daily_cost_hard"} {
		raw, submitted := values[key]
		if !submitted {
			continue
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || number < 0 {
			return fmt.Errorf("%s must be a non-negative number", key)
		}
	}
	if raw, submitted := values["ai.run.max_credits"]; submitted {
		// fixeddecimal.Parse intentionally rejects surrounding whitespace; the API
		// contract requires the canonical decimal form.
		if _, err := fixeddecimal.Parse(raw, false, decimal.NewFromInt(100_000_000)); err != nil {
			return fmt.Errorf("ai.run.max_credits must be a positive decimal no greater than 100000000")
		}
	}
	if mode, submitted := values["ai.access.mode"]; submitted {
		mode = strings.TrimSpace(mode)
		if mode != "all_authenticated" && mode != "admins" {
			return fmt.Errorf("ai.access.mode must be all_authenticated or admins")
		}
	}
	return nil
}

// firstSubmittedOrStored returns the submitted value for key when present in this
// request, otherwise the value currently stored in the config cache.
func firstSubmittedOrStored(values map[string]string, h *Handlers, key string) string {
	if raw, ok := values[key]; ok && strings.TrimSpace(raw) != "" {
		return raw
	}
	return h.configs.get([]string{key})[key]
}
