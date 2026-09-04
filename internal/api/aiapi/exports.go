package aiapi

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type ProviderModel = aiProviderModel

func ContainsAIConfig[T any](values map[string]T) bool { return containsAIConfig(values) }

func ValidateAIConfigInputTypes(values map[string]any, definitionForKey func(string) *ConfigDefinition) error {
	return validateAIConfigInputTypes(values, definitionForKey)
}

func (h *Handler) ValidateAIConfigValues(values map[string]string) error {
	return h.validateAIConfigValues(values)
}

func (h *Handler) AIAccessAllowed(role string) bool { return h.aiAccessAllowed(role) }

func ProviderSelectConfig(values map[string]string, key string, definitionForKey func(string) *ConfigDefinition) string {
	return aiProviderSelectConfig(values, key, definitionForKey)
}

func ProviderRuntimeConfig(values map[string]string, definitionForKey func(string) *ConfigDefinition) gin.H {
	return aiProviderRuntimeConfig(values, definitionForKey)
}

func ProviderConfigVersion(values map[string]string, secretVersion string) string {
	return aiProviderConfigVersion(values, secretVersion)
}

func ProviderModels(configured []model.AIModel) []ProviderModel { return aiProviderModels(configured) }

func IntegerConfigBounds() map[string][2]int {
	result := make(map[string][2]int, len(aiIntegerConfigBounds))
	for key, bounds := range aiIntegerConfigBounds {
		result[key] = bounds
	}
	return result
}

func ProxyRouteKeys() []string {
	keys := make([]string, 0, len(aiProxyRoutes))
	for key := range aiProxyRoutes {
		keys = append(keys, key)
	}
	return keys
}

func (h *Handler) AIToolExecutionIdentityMiddleware() gin.HandlerFunc {
	return h.aiToolExecutionIdentityMiddleware()
}

func (h *Handler) ReadAIBody(ctx *gin.Context) ([]byte, bool) { return h.readAIBody(ctx) }

func (h *Handler) CopyAIResponse(ctx *gin.Context, response *aiagent.Response, fallbackStatus int, errorCode string) {
	h.copyAIResponse(ctx, response, fallbackStatus, errorCode)
}

func (h *Handler) AIPlatformSession(ctx context.Context) (string, string, bool) {
	actor, ok := h.host.AIPlatformActor(ctx)
	return actor.UserID, actor.SessionID, ok && actor.UserID != "" && actor.SessionID != ""
}

func (h *Handler) CurrentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	return h.currentAIPlatformUser(ctx)
}
