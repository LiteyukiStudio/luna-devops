package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"ai.provider.compatibility",
	"ai.provider.prompt_cache_key_mode",
	"ai.provider.channel_affinity_enabled",
	"ai.runtime.provider_timeout_seconds",
	"ai.runtime.max_request_retries",
	"ai.runtime.run_timeout_seconds",
	"ai.runtime.agent_concurrent_runs",
	"ai.quota.user_concurrent_runs",
	"ai.model.max_output_tokens",
	"ai.run.max_model_steps",
	"ai.quota.run_max_tool_calls",
	"ai.run.max_input_k_bytes",
	"ai.tools.max_card_repair_attempts",
	"ai.context.max_uncompressed_turn_count",
	"ai.context.max_compression_turns_per_compile",
	"ai.context.summary_max_output_tokens",
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
			"baseUrl":                baseURL,
			"apiKey":                 apiKey,
			"providerCompatibility":  aiProviderSelectConfig(values, "ai.provider.compatibility"),
			"promptCacheKeyMode":     aiProviderSelectConfig(values, "ai.provider.prompt_cache_key_mode"),
			"channelAffinityEnabled": configBool(values["ai.provider.channel_affinity_enabled"]),
			"configured":             baseURL != "" && len(models) > 0 && strings.TrimSpace(apiKey) != "",
			"models":                 models,
		},
		"runtime":     aiProviderRuntimeConfig(values),
		"toolCatalog": toolCatalog,
	})
}

func aiProviderSelectConfig(values map[string]string, key string) string {
	definition := configDefinitionByKey(key)
	if definition == nil || definition.Type != "select" {
		panic("missing AI Provider select configuration definition for " + key)
	}
	value := strings.TrimSpace(values[key])
	if configOptionAllowed(value, definition.Options) {
		return value
	}
	return definition.Default
}

func aiProviderConfigVersionWithCatalog(base string, operations []aitool.OpenAPIOperation) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(base))
	encoded, _ := json.Marshal(operations)
	_, _ = hash.Write(encoded)
	return "aipcfg_" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func aiProviderRuntimeConfig(values map[string]string) gin.H {
	return gin.H{
		"providerTimeoutMs":                    aiBoundedIntegerConfig(values, "ai.runtime.provider_timeout_seconds") * 1000,
		"maxRequestRetries":                    aiBoundedIntegerConfig(values, "ai.runtime.max_request_retries"),
		"runTimeoutMs":                         aiBoundedIntegerConfig(values, "ai.runtime.run_timeout_seconds") * 1000,
		"agentConcurrentRuns":                  aiBoundedIntegerConfig(values, "ai.runtime.agent_concurrent_runs"),
		"userConcurrentRuns":                   aiBoundedIntegerConfig(values, "ai.quota.user_concurrent_runs"),
		"assistantMaxOutputTokens":             aiBoundedIntegerConfig(values, "ai.model.max_output_tokens"),
		"maxModelSteps":                        aiBoundedIntegerConfig(values, "ai.run.max_model_steps"),
		"runMaxToolCalls":                      aiBoundedIntegerConfig(values, "ai.quota.run_max_tool_calls"),
		"maxInputBytes":                        aiBoundedIntegerConfig(values, "ai.run.max_input_k_bytes") * 1024,
		"maxCardRepairAttempts":                aiBoundedIntegerConfig(values, "ai.tools.max_card_repair_attempts"),
		"contextMaxUncompressedTurnCount":      aiBoundedIntegerConfig(values, "ai.context.max_uncompressed_turn_count"),
		"contextMaxCompressionTurnsPerCompile": aiBoundedIntegerConfig(values, "ai.context.max_compression_turns_per_compile"),
		"contextSummaryMaxOutputTokens":        aiBoundedIntegerConfig(values, "ai.context.summary_max_output_tokens"),
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

type aiProviderModel struct {
	ID                           string `json:"id"`
	Name                         string `json:"name"`
	MaxContextTokens             int64  `json:"maxContextTokens"`
	MaxOutputTokens              int64  `json:"maxOutputTokens"`
	InputCreditsPerMillion       string `json:"inputCreditsPerMillion"`
	OutputCreditsPerMillion      string `json:"outputCreditsPerMillion"`
	CachedInputCreditsPerMillion string `json:"cachedInputCreditsPerMillion"`
}

func aiProviderModels(configured []model.AIModel) []aiProviderModel {
	models := make([]aiProviderModel, 0, len(configured))
	for _, item := range configured {
		models = append(models, aiProviderModel{
			ID:                           item.ID,
			Name:                         item.Name,
			MaxContextTokens:             item.MaxContextTokens,
			MaxOutputTokens:              item.MaxOutputTokens,
			InputCreditsPerMillion:       item.InputCreditsPerMillion.String(),
			OutputCreditsPerMillion:      item.OutputCreditsPerMillion.String(),
			CachedInputCreditsPerMillion: item.CachedInputCreditsPerMillion.String(),
		})
	}
	return models
}

func aiProviderConfigVersionWithModels(base string, models []model.AIModel) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(base))
	for _, item := range models {
		_, _ = hash.Write([]byte("\x00" + item.ID + "\x00" + item.Name + "\x00" + strconv.FormatInt(item.MaxContextTokens, 10) + "\x00" + strconv.FormatInt(item.MaxOutputTokens, 10) + "\x00" + item.InputCreditsPerMillion.String() + "\x00" + item.OutputCreditsPerMillion.String() + "\x00" + item.CachedInputCreditsPerMillion.String() + "\x00" + item.UpdatedAt.UTC().String()))
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
