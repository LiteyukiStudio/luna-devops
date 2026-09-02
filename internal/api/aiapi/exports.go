package aiapi

import (
	"context"
	"net/url"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type ProgressSnapshot = aiProgressSnapshot
type ProviderModel = aiProviderModel
type ObservabilitySummary = agentObservabilitySummary

func ContainsAIConfig[T any](values map[string]T) bool { return containsAIConfig(values) }

func ValidateAIConfigInputTypes(values map[string]any, definitionForKey func(string) *ConfigDefinition) error {
	return validateAIConfigInputTypes(values, definitionForKey)
}

func (h *Handler) ValidateAIConfigValues(values map[string]string) error {
	return h.validateAIConfigValues(values)
}

func (h *Handler) AIAccessAllowed(role string) bool { return h.aiAccessAllowed(role) }
func (h *Handler) AIAssistantEnabled() bool         { return h.aiAssistantEnabled() }

func ProviderSelectConfig(values map[string]string, key string, definitionForKey func(string) *ConfigDefinition) string {
	return aiProviderSelectConfig(values, key, definitionForKey)
}

func ProviderRuntimeConfig(values map[string]string, definitionForKey func(string) *ConfigDefinition) gin.H {
	return aiProviderRuntimeConfig(values, definitionForKey)
}

func ProviderConfigVersion(values map[string]string, secretVersion string) string {
	return aiProviderConfigVersion(values, secretVersion)
}

func ProviderConfigVersionWithCatalog(base string, operations []aitool.OpenAPIOperation) string {
	return aiProviderConfigVersionWithCatalog(base, operations)
}

func ProviderConfigVersionWithModels(base string, models []model.AIModel) string {
	return aiProviderConfigVersionWithModels(base, models)
}

func ProviderModels(configured []model.AIModel) []ProviderModel { return aiProviderModels(configured) }

func IntegerConfigBounds() map[string][2]int {
	result := make(map[string][2]int, len(aiIntegerConfigBounds))
	for key, bounds := range aiIntegerConfigBounds {
		result[key] = bounds
	}
	return result
}

func BuildRunProgress(run model.BuildRun) ProgressSnapshot { return buildRunProgress(run) }
func ReleaseProgress(release model.Release) ProgressSnapshot {
	return releaseProgress(release)
}
func HookRunProgress(run model.HookRun) ProgressSnapshot { return hookRunProgress(run) }
func AppTemplateInstallationProgress(installation model.AppTemplateInstallation) ProgressSnapshot {
	return appTemplateInstallationProgress(installation)
}
func AppTemplateReleaseProgress(installation model.AppTemplateInstallation, release model.Release) ProgressSnapshot {
	return appTemplateReleaseProgress(installation, release)
}
func ProgressRevision(value time.Time) string { return aiProgressRevision(value) }
func ProgressTerminal(state string) bool      { return aiProgressTerminal(state) }

func AgentObservabilitySummaryQueries(rangeText string) map[string]string {
	return agentObservabilitySummaryQueries(rangeText)
}
func ObservabilityRange(value string) (string, time.Duration) { return observabilityRange(value) }
func WriteAgentObservabilityUnavailable(ctx *gin.Context, code, detail string) {
	writeAgentObservabilityUnavailable(ctx, code, detail)
}

func RequestMatchesConversationProject(ctx *gin.Context, operation aitool.OpenAPIOperation, projectID string) bool {
	return aiRequestMatchesConversationProject(ctx, operation, projectID)
}
func OpenAPIPathToGin(path string) string { return openAPIPathToGin(path) }

func ProxyRouteKeys() []string {
	keys := make([]string, 0, len(aiProxyRoutes))
	for key := range aiProxyRoutes {
		keys = append(keys, key)
	}
	return keys
}

func PrepareTurn(ctx *gin.Context, actor aiagent.ActorContext, body []byte) ([]byte, aiagent.ActorContext, bool) {
	return prepareAITurn(ctx, actor, body)
}
func EnrichPageContext(raw any, actor aiagent.ActorContext, now time.Time) map[string]any {
	return enrichAIPageContext(raw, actor, now)
}
func ProjectIDFromRequest(ctx *gin.Context, body []byte) string {
	return projectIDFromAIRequest(ctx, body)
}
func NormalizedLocale(locale string) string                   { return normalizedAILocale(locale) }
func CloneQuery(query url.Values) url.Values                  { return cloneAIQuery(query) }
func ExpandInternalPath(path string, ctx *gin.Context) string { return expandAIInternalPath(path, ctx) }

func (h *Handler) AIToolExecutionIdentityMiddleware() gin.HandlerFunc {
	return h.aiToolExecutionIdentityMiddleware()
}

func (h *Handler) ReadAIBody(ctx *gin.Context) ([]byte, bool) { return h.readAIBody(ctx) }

func (h *Handler) CopyAIResponse(ctx *gin.Context, response *aiagent.Response, fallbackStatus int, errorCode string) {
	h.copyAIResponse(ctx, response, fallbackStatus, errorCode)
}

func (h *Handler) AIConversationProjectID(ctx *gin.Context) string {
	return h.aiConversationProjectID(ctx)
}

func (h *Handler) AIPlatformSession(ctx context.Context) (string, string, bool) {
	actor, ok := h.host.AIPlatformActor(ctx)
	return actor.UserID, actor.SessionID, ok && actor.UserID != "" && actor.SessionID != ""
}

func (h *Handler) CurrentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	return h.currentAIPlatformUser(ctx)
}
