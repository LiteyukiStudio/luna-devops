package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/aiapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type aiPlatformActorContextKey struct{}

type aiPlatformActor struct {
	UserID    string
	SessionID string
	ProjectID string
}

type aiHost struct {
	domainHost
}

func (host aiHost) SetCurrentUser(ctx *gin.Context, user model.User) {
	ctx.Set("luna.devops.current_user", user)
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

func aiConfigDefinition(key string) *aiapi.ConfigDefinition {
	definition := configDefinitionByKey(key)
	if definition == nil {
		return nil
	}
	return &aiapi.ConfigDefinition{Type: definition.Type, Default: definition.Default, Options: definition.Options}
}
