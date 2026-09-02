package api

import (
	"context"
	"net/url"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const aiAssistantEnabledConfigKey = aiapi.AssistantEnabledConfigKey
const aiAccessModeConfigKey = aiapi.AccessModeConfigKey
const aiTextInputLimitBytes = aiapi.TextInputLimitBytes
const aiRequestBodyLimitBytes = aiapi.RequestBodyLimitBytes
const aiRunIDHeader = aiapi.RunIDHeader
const aiToolCallIDHeader = aiapi.ToolCallIDHeader

type aiPlatformActorContextKey struct{}

type aiPlatformActor struct {
	UserID    string
	SessionID string
	ProjectID string
}

type agentObservabilitySummary = aiapi.ObservabilitySummary

type aiHost struct {
	handlers *Handlers
}

func (host aiHost) DBFor(ctx *gin.Context) *gorm.DB { return host.handlers.dbFor(ctx) }
func (host aiHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}
func (host aiHost) CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return host.handlers.currentSessionFromCookie(ctx)
}
func (host aiHost) SetCurrentUser(ctx *gin.Context, user model.User) {
	ctx.Set("luna.devops.current_user", user)
}
func (host aiHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}
func (host aiHost) ConfigAvailable() bool { return host.handlers.configs != nil }
func (host aiHost) ConfigValues(keys []string) map[string]string {
	if host.handlers.configs == nil {
		return map[string]string{}
	}
	return host.handlers.configs.get(keys)
}
func (host aiHost) AllConfigValues() map[string]string {
	return host.ConfigValues(knownConfigKeys())
}
func (host aiHost) ConfigDefinition(key string) *aiapi.ConfigDefinition {
	return aiConfigDefinition(key)
}
func (host aiHost) ResolveSecret(ctx context.Context, ref string) string {
	return host.handlers.secrets.ResolveContext(ctx, ref)
}
func (host aiHost) AIAgent() aiagent.Client { return host.handlers.aiAgent }
func (host aiHost) AIDeploymentEnabled() bool {
	return host.handlers.aiDeploymentEnabled
}
func (host aiHost) AIActorOverride(ctx *gin.Context) (aiagent.ActorContext, string, bool, bool) {
	if host.handlers.aiActorResolver == nil {
		return aiagent.ActorContext{}, "", false, false
	}
	actor, role, ok := host.handlers.aiActorResolver(ctx)
	return actor, role, ok, true
}
func (host aiHost) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return host.handlers.findProjectForCurrentUserByID(ctx, projectID)
}
func (host aiHost) AIAgentCallbackServiceToken() string {
	return host.handlers.config.AIAgent.CallbackServiceToken
}
func (host aiHost) AIPlatformActor(ctx context.Context) (aiapi.PlatformActor, bool) {
	actor, ok := ctx.Value(aiPlatformActorContextKey{}).(aiPlatformActor)
	return aiapi.PlatformActor{UserID: actor.UserID, SessionID: actor.SessionID, ProjectID: actor.ProjectID}, ok
}
func (host aiHost) WithAIPlatformActor(ctx context.Context, actor aiapi.PlatformActor) context.Context {
	return context.WithValue(ctx, aiPlatformActorContextKey{}, aiPlatformActor{
		UserID: actor.UserID, SessionID: actor.SessionID, ProjectID: actor.ProjectID,
	})
}
func (host aiHost) AIToolService() *aitool.Service { return host.handlers.aiTools }
func (host aiHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}
func (host aiHost) MonitorProjectAuthorization(ctx *gin.Context, streamCtx context.Context, user model.User, projectID string, action authz.Action, revoke func()) (<-chan struct{}, bool) {
	binding, ok := host.handlers.requireContinuousAuthorizationBinding(ctx, user)
	if !ok {
		return nil, false
	}
	return host.handlers.monitorContinuousAuthorization(
		streamCtx,
		binding,
		func(checkCtx context.Context, currentUser model.User) bool {
			return host.handlers.projectContinuousAuthorizationAllowed(checkCtx, currentUser, projectID, action)
		},
		revoke,
	)
}
func (host aiHost) WriteContinuousAuthorizationRevoked(ctx *gin.Context) {
	writeContinuousAuthorizationRevoked(ctx)
}

func (h *Handlers) aiAPI() *aiapi.Handler { return aiapi.New(aiHost{handlers: h}) }

func (h *Handlers) GetAICapabilities(ctx *gin.Context) { h.aiAPI().GetAICapabilities(ctx) }
func (h *Handlers) ProxyAIRequest(ctx *gin.Context)    { h.aiAPI().ProxyAIRequest(ctx) }
func (h *Handlers) ListAIModels(ctx *gin.Context)      { h.aiAPI().ListAIModels(ctx) }
func (h *Handlers) ListAIModelConfigs(ctx *gin.Context) {
	h.aiAPI().ListAIModelConfigs(ctx)
}
func (h *Handlers) CreateAIModel(ctx *gin.Context) { h.aiAPI().CreateAIModel(ctx) }
func (h *Handlers) UpdateAIModel(ctx *gin.Context) { h.aiAPI().UpdateAIModel(ctx) }
func (h *Handlers) DeleteAIModel(ctx *gin.Context) { h.aiAPI().DeleteAIModel(ctx) }
func (h *Handlers) GetAIProviderConfigInternal(ctx *gin.Context) {
	h.aiAPI().GetAIProviderConfigInternal(ctx)
}
func (h *Handlers) TestAIProviderConnection(ctx *gin.Context) {
	h.aiAPI().TestAIProviderConnection(ctx)
}
func (h *Handlers) GetAIProgress(ctx *gin.Context)    { h.aiAPI().GetAIProgress(ctx) }
func (h *Handlers) StreamAIProgress(ctx *gin.Context) { h.aiAPI().StreamAIProgress(ctx) }
func (h *Handlers) ExecuteAIWebSearch(ctx *gin.Context) {
	h.aiAPI().ExecuteAIWebSearch(ctx)
}
func (h *Handlers) ExecuteAIFetchWebPage(ctx *gin.Context) {
	h.aiAPI().ExecuteAIFetchWebPage(ctx)
}
func (h *Handlers) TestAgentObservabilitySource(ctx *gin.Context) {
	h.aiAPI().TestAgentObservabilitySource(ctx)
}
func (h *Handlers) GetAgentObservabilityOverview(ctx *gin.Context) {
	h.aiAPI().GetAgentObservabilityOverview(ctx)
}
func (h *Handlers) GetAgentObservabilityTrace(ctx *gin.Context) {
	h.aiAPI().GetAgentObservabilityTrace(ctx)
}
func (h *Handlers) ListAgentObservabilityConversations(ctx *gin.Context) {
	h.aiAPI().ListAgentObservabilityConversations(ctx)
}
func (h *Handlers) ListAgentObservabilityTurns(ctx *gin.Context) {
	h.aiAPI().ListAgentObservabilityTurns(ctx)
}
func (h *Handlers) ListAgentObservabilityTools(ctx *gin.Context) {
	h.aiAPI().ListAgentObservabilityTools(ctx)
}
func (h *Handlers) ListAgentObservabilityToolCalls(ctx *gin.Context) {
	h.aiAPI().ListAgentObservabilityToolCalls(ctx)
}
func (h *Handlers) GetAgentObservabilityConversation(ctx *gin.Context) {
	h.aiAPI().GetAgentObservabilityConversation(ctx)
}

func (h *Handlers) aiToolExecutionIdentityMiddleware() gin.HandlerFunc {
	return h.aiAPI().AIToolExecutionIdentityMiddleware()
}
func (h *Handlers) aiAccessAllowed(role string) bool { return h.aiAPI().AIAccessAllowed(role) }
func (h *Handlers) aiAssistantEnabled() bool         { return h.aiAPI().AIAssistantEnabled() }
func (h *Handlers) readAIBody(ctx *gin.Context) ([]byte, bool) {
	return h.aiAPI().ReadAIBody(ctx)
}
func (h *Handlers) copyAIResponse(ctx *gin.Context, response *aiagent.Response, fallbackStatus int, errorCode string) {
	h.aiAPI().CopyAIResponse(ctx, response, fallbackStatus, errorCode)
}
func (h *Handlers) currentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	return h.aiAPI().CurrentAIPlatformUser(ctx)
}
func (h *Handlers) currentAIPlatformSession(ctx *gin.Context) (string, string, bool) {
	return h.aiAPI().AIPlatformSession(ctx.Request.Context())
}
func (h *Handlers) validateAIConfigValues(values map[string]string) error {
	return h.aiAPI().ValidateAIConfigValues(values)
}
func aiConversationProjectID(ctx *gin.Context) string {
	return aiapi.New(aiHost{}).AIConversationProjectID(ctx)
}

func aiConfigDefinition(key string) *aiapi.ConfigDefinition {
	definition := configDefinitionByKey(key)
	if definition == nil {
		return nil
	}
	return &aiapi.ConfigDefinition{Type: definition.Type, Default: definition.Default, Options: definition.Options}
}

func containsAIConfig[T any](values map[string]T) bool { return aiapi.ContainsAIConfig(values) }
func validateAIConfigInputTypes(values map[string]any) error {
	return aiapi.ValidateAIConfigInputTypes(values, aiConfigDefinition)
}

var aiIntegerConfigBounds = aiapi.IntegerConfigBounds()

func aiProviderSelectConfig(values map[string]string, key string) string {
	return aiapi.ProviderSelectConfig(values, key, aiConfigDefinition)
}
func aiProviderRuntimeConfig(values map[string]string) gin.H {
	return aiapi.ProviderRuntimeConfig(values, aiConfigDefinition)
}
func aiProviderConfigVersion(values map[string]string, secretVersion string) string {
	return aiapi.ProviderConfigVersion(values, secretVersion)
}
func aiProviderConfigVersionWithCatalog(base string, operations []aitool.OpenAPIOperation) string {
	return aiapi.ProviderConfigVersionWithCatalog(base, operations)
}
func aiProviderConfigVersionWithModels(base string, models []model.AIModel) string {
	return aiapi.ProviderConfigVersionWithModels(base, models)
}
func aiProviderModels(configured []model.AIModel) []aiapi.ProviderModel {
	return aiapi.ProviderModels(configured)
}

func buildRunProgress(run model.BuildRun) aiapi.ProgressSnapshot { return aiapi.BuildRunProgress(run) }
func releaseProgress(release model.Release) aiapi.ProgressSnapshot {
	return aiapi.ReleaseProgress(release)
}
func hookRunProgress(run model.HookRun) aiapi.ProgressSnapshot { return aiapi.HookRunProgress(run) }
func appTemplateInstallationProgress(installation model.AppTemplateInstallation) aiapi.ProgressSnapshot {
	return aiapi.AppTemplateInstallationProgress(installation)
}
func appTemplateReleaseProgress(installation model.AppTemplateInstallation, release model.Release) aiapi.ProgressSnapshot {
	return aiapi.AppTemplateReleaseProgress(installation, release)
}
func aiProgressRevision(value time.Time) string { return aiapi.ProgressRevision(value) }
func aiProgressTerminal(state string) bool      { return aiapi.ProgressTerminal(state) }

func agentObservabilitySummaryQueries(rangeText string) map[string]string {
	return aiapi.AgentObservabilitySummaryQueries(rangeText)
}
func observabilityRange(value string) (string, time.Duration) {
	return aiapi.ObservabilityRange(value)
}
func writeAgentObservabilityUnavailable(ctx *gin.Context, code, detail string) {
	aiapi.WriteAgentObservabilityUnavailable(ctx, code, detail)
}

func aiRequestMatchesConversationProject(ctx *gin.Context, operation aitool.OpenAPIOperation, projectID string) bool {
	return aiapi.RequestMatchesConversationProject(ctx, operation, projectID)
}
func openAPIPathToGin(path string) string { return aiapi.OpenAPIPathToGin(path) }

var aiProxyRoutes = func() map[string]struct{} {
	routes := map[string]struct{}{}
	for _, key := range aiapi.ProxyRouteKeys() {
		routes[key] = struct{}{}
	}
	return routes
}()

func prepareAITurn(ctx *gin.Context, actor aiagent.ActorContext, body []byte) ([]byte, aiagent.ActorContext, bool) {
	return aiapi.PrepareTurn(ctx, actor, body)
}
func enrichAIPageContext(raw any, actor aiagent.ActorContext, now time.Time) map[string]any {
	return aiapi.EnrichPageContext(raw, actor, now)
}
func projectIDFromAIRequest(ctx *gin.Context, body []byte) string {
	return aiapi.ProjectIDFromRequest(ctx, body)
}
func normalizedAILocale(locale string) string  { return aiapi.NormalizedLocale(locale) }
func cloneAIQuery(query url.Values) url.Values { return aiapi.CloneQuery(query) }
func expandAIInternalPath(path string, ctx *gin.Context) string {
	return aiapi.ExpandInternalPath(path, ctx)
}
