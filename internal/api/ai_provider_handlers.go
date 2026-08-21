package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/fixeddecimal"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

var aiProviderConfigKeys = []string{
	"ai.provider.base_url",
	"ai.runtime.provider_timeout_seconds",
	"ai.runtime.max_request_retries",
	"ai.runtime.run_timeout_seconds",
	"ai.runtime.agent_concurrent_runs",
	"ai.runtime.context_input_k_tokens",
	"ai.quota.user_concurrent_runs",
	"ai.model.max_output_tokens",
	"ai.run.max_model_steps",
	"ai.quota.run_max_tool_calls",
	"ai.run.max_total_tokens",
	"ai.run.max_credits",
	"ai.run.max_input_k_bytes",
	"ai.run.navigate_action_ttl_seconds",
	"ai.tools.result_payload_k_bytes",
	"ai.tools.max_card_repair_attempts",
	"ai.context.compression_trigger_ratio",
	"ai.context.compression_target_ratio",
	"ai.context.recent_turn_count",
	"ai.context.max_recent_turn_count",
	"ai.context.max_uncompressed_turn_count",
	"ai.context.max_compression_turns_per_compile",
	"ai.context.summary_input_k_tokens",
	"ai.context.summary_max_output_tokens",
	"ai.context.historical_tool_k_tokens",
}

func (h *Handlers) GetAIProviderConfigInternal(ctx *gin.Context) {
	if !requireAIAgentService(ctx) {
		return
	}
	toolCatalog, err := aitool.PlatformCatalog()
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool catalog is unavailable")
		return
	}
	values := h.configs.get(aiProviderConfigKeys)
	var secretConfig model.AppConfig
	_ = h.dbFor(ctx).First(&secretConfig, "key = ?", "ai.provider.api_key").Error
	apiKey := h.secrets.ResolveContext(ctx.Request.Context(), secretConfig.Value)
	version := aiProviderConfigVersion(values, secretConfig.UpdatedAt.String())
	version = aiProviderConfigVersionWithCatalog(version, toolCatalog)
	baseURL := strings.TrimSpace(values["ai.provider.base_url"])
	var configuredModels []model.AIModel
	if err := h.dbFor(ctx).Where("enabled = ?", true).Order("name asc").Find(&configuredModels).Error; err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.models_unavailable", "AI models are unavailable")
		return
	}
	version = aiProviderConfigVersionWithModels(version, configuredModels)
	models := aiProviderModels(configuredModels)
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("ETag", `"`+version+`"`)
	ctx.JSON(http.StatusOK, gin.H{
		"version": version,
		"provider": gin.H{
			"baseUrl":    baseURL,
			"apiKey":     apiKey,
			"configured": baseURL != "" && len(models) > 0 && strings.TrimSpace(apiKey) != "",
			"models":     models,
		},
		"runtime":     aiProviderRuntimeConfig(values),
		"toolCatalog": toolCatalog,
	})
}

func aiProviderConfigVersionWithCatalog(base string, operations []aitool.OpenAPIOperation) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(base))
	encoded, _ := json.Marshal(operations)
	_, _ = hash.Write(encoded)
	return "aipcfg_" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func aiProviderRuntimeConfig(values map[string]string) gin.H {
	triggerRatio := aiBoundedRatioConfig(values, "ai.context.compression_trigger_ratio")
	targetRatio := aiBoundedRatioConfig(values, "ai.context.compression_target_ratio")
	if triggerRatio <= targetRatio {
		triggerRatio = aiDefaultRatioConfig("ai.context.compression_trigger_ratio")
		targetRatio = aiDefaultRatioConfig("ai.context.compression_target_ratio")
	}
	recentTurns := aiBoundedIntegerConfig(values, "ai.context.recent_turn_count")
	maxRecentTurns := aiBoundedIntegerConfig(values, "ai.context.max_recent_turn_count")
	if recentTurns > maxRecentTurns {
		recentTurns = aiDefaultIntegerConfig("ai.context.recent_turn_count")
		maxRecentTurns = aiDefaultIntegerConfig("ai.context.max_recent_turn_count")
	}

	return gin.H{
		"providerTimeoutMs":                    aiBoundedIntegerConfig(values, "ai.runtime.provider_timeout_seconds") * 1000,
		"maxRequestRetries":                    aiBoundedIntegerConfig(values, "ai.runtime.max_request_retries"),
		"runTimeoutMs":                         aiBoundedIntegerConfig(values, "ai.runtime.run_timeout_seconds") * 1000,
		"agentConcurrentRuns":                  aiBoundedIntegerConfig(values, "ai.runtime.agent_concurrent_runs"),
		"userConcurrentRuns":                   aiBoundedIntegerConfig(values, "ai.quota.user_concurrent_runs"),
		"contextInputTokenBudget":              aiBoundedIntegerConfig(values, "ai.runtime.context_input_k_tokens") * 1024,
		"assistantMaxOutputTokens":             aiBoundedIntegerConfig(values, "ai.model.max_output_tokens"),
		"maxModelSteps":                        aiBoundedIntegerConfig(values, "ai.run.max_model_steps"),
		"runMaxToolCalls":                      aiBoundedIntegerConfig(values, "ai.quota.run_max_tool_calls"),
		"runTotalTokenBudget":                  aiBoundedIntegerConfig(values, "ai.run.max_total_tokens"),
		"runTotalCreditBudget":                 aiBoundedCreditConfig(values, "ai.run.max_credits"),
		"maxInputBytes":                        aiBoundedIntegerConfig(values, "ai.run.max_input_k_bytes") * 1024,
		"navigateActionTtlSeconds":             aiBoundedIntegerConfig(values, "ai.run.navigate_action_ttl_seconds"),
		"toolResultPayloadBudget":              aiBoundedIntegerConfig(values, "ai.tools.result_payload_k_bytes") * 1024,
		"maxCardRepairAttempts":                aiBoundedIntegerConfig(values, "ai.tools.max_card_repair_attempts"),
		"contextCompressionTriggerRatio":       triggerRatio,
		"contextCompressionTargetRatio":        targetRatio,
		"contextRecentTurnCount":               recentTurns,
		"contextMaxRecentTurnCount":            maxRecentTurns,
		"contextMaxUncompressedTurnCount":      aiBoundedIntegerConfig(values, "ai.context.max_uncompressed_turn_count"),
		"contextMaxCompressionTurnsPerCompile": aiBoundedIntegerConfig(values, "ai.context.max_compression_turns_per_compile"),
		"contextSummaryInputTokenBudget":       aiBoundedIntegerConfig(values, "ai.context.summary_input_k_tokens") * 1024,
		"contextSummaryMaxOutputTokens":        aiBoundedIntegerConfig(values, "ai.context.summary_max_output_tokens"),
		"contextHistoricalToolTokenBudget":     aiBoundedIntegerConfig(values, "ai.context.historical_tool_k_tokens") * 1024,
	}
}

func aiBoundedIntegerConfig(values map[string]string, key string) int {
	bounds, ok := aiIntegerConfigBounds[key]
	if !ok {
		panic("missing AI integer configuration bounds for " + key)
	}
	return aiRuntimeIntegerInRange(values, key, aiDefaultIntegerConfig(key), bounds[0], bounds[1])
}

func aiDefaultIntegerConfig(key string) int {
	definition := configDefinitionByKey(key)
	if definition == nil {
		panic("missing AI configuration definition for " + key)
	}
	value, err := strconv.Atoi(strings.TrimSpace(definition.Default))
	if err != nil {
		panic("invalid AI integer configuration default for " + key)
	}
	return value
}

func aiBoundedRatioConfig(values map[string]string, key string) float64 {
	bounds, ok := aiRatioConfigBounds[key]
	if !ok {
		panic("missing AI ratio configuration bounds for " + key)
	}
	fallback := aiDefaultRatioConfig(key)
	value, err := strconv.ParseFloat(strings.TrimSpace(values[key]), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < bounds[0] || value > bounds[1] {
		return fallback
	}
	return value
}

func aiDefaultRatioConfig(key string) float64 {
	definition := configDefinitionByKey(key)
	if definition == nil {
		panic("missing AI configuration definition for " + key)
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(definition.Default), 64)
	if err != nil {
		panic("invalid AI ratio configuration default for " + key)
	}
	return value
}

func aiBoundedCreditConfig(values map[string]string, key string) string {
	definition := configDefinitionByKey(key)
	if definition == nil {
		panic("missing AI configuration definition for " + key)
	}
	value := strings.TrimSpace(values[key])
	if _, err := fixeddecimal.Parse(value, false, decimal.NewFromInt(100_000_000)); err != nil {
		return definition.Default
	}
	return value
}

type aiProviderModel struct {
	ID                            string `json:"id"`
	Name                          string `json:"name"`
	MaxContextTokens              int64  `json:"maxContextTokens"`
	MaxOutputTokens               int64  `json:"maxOutputTokens"`
	InputCreditsPerMillion        string `json:"inputCreditsPerMillion"`
	OutputCreditsPerMillion       string `json:"outputCreditsPerMillion"`
	CachedInputCreditsPerMillion  string `json:"cachedInputCreditsPerMillion"`
	CachedOutputCreditsPerMillion string `json:"cachedOutputCreditsPerMillion"`
}

func aiProviderModels(configured []model.AIModel) []aiProviderModel {
	models := make([]aiProviderModel, 0, len(configured))
	for _, item := range configured {
		models = append(models, aiProviderModel{
			ID:                            item.ID,
			Name:                          item.Name,
			MaxContextTokens:              item.MaxContextTokens,
			MaxOutputTokens:               item.MaxOutputTokens,
			InputCreditsPerMillion:        item.InputCreditsPerMillion.String(),
			OutputCreditsPerMillion:       item.OutputCreditsPerMillion.String(),
			CachedInputCreditsPerMillion:  item.CachedInputCreditsPerMillion.String(),
			CachedOutputCreditsPerMillion: item.CachedOutputCreditsPerMillion.String(),
		})
	}
	return models
}

func aiProviderConfigVersionWithModels(base string, models []model.AIModel) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(base))
	for _, item := range models {
		_, _ = hash.Write([]byte("\x00" + item.ID + "\x00" + item.Name + "\x00" + strconv.FormatInt(item.MaxContextTokens, 10) + "\x00" + strconv.FormatInt(item.MaxOutputTokens, 10) + "\x00" + item.InputCreditsPerMillion.String() + "\x00" + item.OutputCreditsPerMillion.String() + "\x00" + item.CachedInputCreditsPerMillion.String() + "\x00" + item.CachedOutputCreditsPerMillion.String() + "\x00" + item.UpdatedAt.UTC().String()))
	}
	return "aipcfg_" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func (h *Handlers) TestAIProviderConnection(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	if h.aiUnavailableReason() != "" || h.aiAgent == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.agent_unavailable", "AI Agent is unavailable")
		return
	}
	actor, _, ok := h.aiActorFromSession(ctx)
	if !ok {
		return
	}
	response, err := h.aiAgent.Do(ctx.Request.Context(), actor, aiagent.Request{
		Method: http.MethodPost, Path: "/internal/v1/provider/test",
		ContentType: "application/json", Body: []byte(`{}`),
	})
	if err != nil {
		h.auditWithContext(user.ID, "ai.provider.test", "ai.provider", false, "Agent connection test failed", ctx.Request.Context())
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.provider_unavailable", "AI Provider test failed")
		return
	}
	defer response.Body.Close()
	h.auditWithContext(user.ID, "ai.provider.test", "ai.provider", response.StatusCode >= 200 && response.StatusCode < 300, "AI Provider connection tested through Agent", ctx.Request.Context())
	ctx.Header("Cache-Control", "no-store")
	h.copyAIResponse(ctx, response, http.StatusOK, "ai.provider_unavailable")
}

func aiProviderConfigVersion(values map[string]string, secretVersion string) string {
	hash := sha256.New()
	for _, key := range aiProviderConfigKeys {
		_, _ = hash.Write([]byte(key + "\x00" + values[key] + "\x00"))
	}
	_, _ = hash.Write([]byte(secretVersion))
	return "aipcfg_" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func aiRuntimeInteger(values map[string]string, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(values[key]))
	if err != nil {
		return fallback
	}
	return value
}

func aiRuntimeIntegerInRange(values map[string]string, key string, fallback, minimum, maximum int) int {
	value := aiRuntimeInteger(values, key, fallback)
	if value < minimum || value > maximum {
		return fallback
	}
	return value
}
