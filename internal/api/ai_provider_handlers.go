package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

var aiProviderConfigKeys = []string{
	"ai.provider.base_url",
	"ai.provider.default_model",
	"ai.runtime.provider_timeout_seconds",
	"ai.runtime.run_timeout_seconds",
	"ai.runtime.agent_concurrent_runs",
}

func (h *Handlers) GetAIProviderConfigInternal(ctx *gin.Context) {
	if !requireAIAgentService(ctx) {
		return
	}
	values := h.configs.get(aiProviderConfigKeys)
	var secretConfig model.AppConfig
	_ = h.db.First(&secretConfig, "key = ?", "ai.provider.api_key").Error
	apiKey := h.secrets.Resolve(secretConfig.Value)
	version := aiProviderConfigVersion(values, secretConfig.UpdatedAt.String())
	baseURL := strings.TrimSpace(values["ai.provider.base_url"])
	modelName := strings.TrimSpace(values["ai.provider.default_model"])
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("ETag", `"`+version+`"`)
	ctx.JSON(http.StatusOK, gin.H{
		"version": version,
		"provider": gin.H{
			"baseUrl":    baseURL,
			"model":      modelName,
			"apiKey":     apiKey,
			"configured": baseURL != "" && modelName != "" && strings.TrimSpace(apiKey) != "",
		},
		"runtime": gin.H{
			"providerTimeoutMs":   aiRuntimeMilliseconds(values, "ai.runtime.provider_timeout_seconds", 30),
			"runTimeoutMs":        aiRuntimeMilliseconds(values, "ai.runtime.run_timeout_seconds", 300),
			"agentConcurrentRuns": aiRuntimeInteger(values, "ai.runtime.agent_concurrent_runs", 2),
		},
	})
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
	actor, ok := h.aiActorFromSession(ctx)
	if !ok {
		return
	}
	response, err := h.aiAgent.Do(ctx.Request.Context(), actor, aiagent.Request{
		Method: http.MethodPost, Path: "/internal/v1/provider/test",
		ContentType: "application/json", Body: []byte(`{}`),
	})
	if err != nil {
		h.audit(user.ID, "ai.provider.test", "ai.provider", false, "Agent connection test failed")
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.provider_unavailable", "AI Provider test failed")
		return
	}
	defer response.Body.Close()
	h.audit(user.ID, "ai.provider.test", "ai.provider", response.StatusCode >= 200 && response.StatusCode < 300, "AI Provider connection tested through Agent")
	ctx.Header("Cache-Control", "no-store")
	ctx.Status(response.StatusCode)
	_, _ = io.Copy(ctx.Writer, response.Body)
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
