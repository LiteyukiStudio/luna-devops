package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

var aiProviderConfigKeys = []string{
	"ai.provider.base_url",
	"ai.provider.default_model",
	"ai.runtime.provider_timeout_seconds",
	"ai.runtime.max_request_retries",
	"ai.runtime.run_timeout_seconds",
	"ai.runtime.agent_concurrent_runs",
	"ai.runtime.context_input_k_tokens",
	"ai.quota.user_concurrent_runs",
	"ai.model.max_output_tokens",
	"ai.run.max_model_steps",
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
			"baseUrl": baseURL,
			// model is retained as a compatibility hint for older Agents; new
			// requests must select from models by id.
			"model":      firstAIModelName(configuredModels),
			"apiKey":     apiKey,
			"configured": baseURL != "" && len(models) > 0 && strings.TrimSpace(apiKey) != "",
			"models":     models,
		},
		"runtime": gin.H{
			"providerTimeoutMs":                    aiRuntimeMilliseconds(values, "ai.runtime.provider_timeout_seconds", 300),
			"maxRequestRetries":                    aiRuntimeInteger(values, "ai.runtime.max_request_retries", 5),
			"runTimeoutMs":                         aiRuntimeMilliseconds(values, "ai.runtime.run_timeout_seconds", 3600),
			"agentConcurrentRuns":                  aiRuntimeInteger(values, "ai.runtime.agent_concurrent_runs", 10),
			"userConcurrentRuns":                   aiRuntimeInteger(values, "ai.quota.user_concurrent_runs", 10),
			"contextInputTokenBudget":              aiRuntimeKTokens(values, "ai.runtime.context_input_k_tokens", 1024),
			"assistantMaxOutputTokens":             aiRuntimeInteger(values, "ai.model.max_output_tokens", 65536),
			"maxModelSteps":                        aiRuntimeInteger(values, "ai.run.max_model_steps", 256),
			"runTotalTokenBudget":                  aiRuntimeInteger(values, "ai.run.max_total_tokens", 2_000_000),
			"runTotalCreditBudget":                 aiRuntimeString(values, "ai.run.max_credits", "10000"),
			"maxInputBytes":                        aiRuntimeKTokens(values, "ai.run.max_input_k_bytes", 1024),
			"navigateActionTtlSeconds":             aiRuntimeInteger(values, "ai.run.navigate_action_ttl_seconds", 120),
			"toolResultPayloadBudget":              aiRuntimeKTokens(values, "ai.tools.result_payload_k_bytes", 512),
			"maxCardRepairAttempts":                aiRuntimeInteger(values, "ai.tools.max_card_repair_attempts", 5),
			"contextCompressionTriggerRatio":       aiRuntimeRatio(values, "ai.context.compression_trigger_ratio", 0.9),
			"contextCompressionTargetRatio":        aiRuntimeRatio(values, "ai.context.compression_target_ratio", 0.7),
			"contextRecentTurnCount":               aiRuntimeInteger(values, "ai.context.recent_turn_count", 16),
			"contextMaxRecentTurnCount":            aiRuntimeInteger(values, "ai.context.max_recent_turn_count", 32),
			"contextMaxUncompressedTurnCount":      aiRuntimeInteger(values, "ai.context.max_uncompressed_turn_count", 64),
			"contextMaxCompressionTurnsPerCompile": aiRuntimeInteger(values, "ai.context.max_compression_turns_per_compile", 512),
			"contextSummaryInputTokenBudget":       aiRuntimeKTokens(values, "ai.context.summary_input_k_tokens", 256),
			"contextSummaryMaxOutputTokens":        aiRuntimeInteger(values, "ai.context.summary_max_output_tokens", 16384),
			"contextHistoricalToolTokenBudget":     aiRuntimeKTokens(values, "ai.context.historical_tool_k_tokens", 64),
		},
		"toolCatalog": toolCatalog,
	})
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

func firstAIModelName(models []model.AIModel) string {
	if len(models) == 0 {
		return ""
	}
	return models[0].Name
}

func aiProviderConfigVersionWithModels(base string, models []model.AIModel) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(base))
	for _, item := range models {
		_, _ = hash.Write([]byte("\x00" + item.ID + "\x00" + item.Name + "\x00" + strconv.FormatInt(item.MaxContextTokens, 10) + "\x00" + strconv.FormatInt(item.MaxOutputTokens, 10) + "\x00" + item.InputCreditsPerMillion.String() + "\x00" + item.OutputCreditsPerMillion.String() + "\x00" + item.CachedInputCreditsPerMillion.String() + "\x00" + item.CachedOutputCreditsPerMillion.String() + "\x00" + item.UpdatedAt.UTC().String()))
	}
	return "aipcfg_" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func aiRuntimeRatio(values map[string]string, key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(values[key]), 64)
	if err != nil {
		return fallback
	}
	return value
}

func aiRuntimeKTokens(values map[string]string, key string, fallback int) int {
	return aiRuntimeInteger(values, key, fallback) * 1024
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

func aiRuntimeMilliseconds(values map[string]string, key string, fallbackSeconds int) int {
	return aiRuntimeInteger(values, key, fallbackSeconds) * 1000
}

func aiRuntimeInteger(values map[string]string, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(values[key]))
	if err != nil {
		return fallback
	}
	return value
}

func aiRuntimeString(values map[string]string, key string, fallback string) string {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return fallback
	}
	return value
}
