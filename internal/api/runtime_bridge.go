package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type runtimeHost struct {
	domainHost
}

func (host runtimeHost) EnsureRuntimeConfigSetCanMutate(ctx *gin.Context, set model.ProjectRuntimeConfigSet) bool {
	return host.handlers.ensureRuntimeConfigSetCanMutate(ctx, set)
}
func (host runtimeHost) ApplyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool) {
	return host.handlers.applyScopedResourceVisibility(ctx, query, resourceType, user, projectID)
}
func (host runtimeHost) AuditWithSafeMetadata(userID, action, resource string, success bool, message string, metadata any, ctx context.Context) {
	switch value := metadata.(type) {
	case runtimeClusterAuditMetadata:
		auditWithSafeMetadata(host.handlers, userID, action, resource, success, message, value, ctx)
	}
}
func (host runtimeHost) ObserveDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	return host.handlers.observeDeploymentTarget(ctx, project, target)
}
func (host runtimeHost) ContinuousAuthorizationActive(ctx context.Context, binding runtimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return host.handlers.continuousAuthorizationActive(ctx, binding, authorizationAllowed)
}
func (host runtimeHost) RuntimeTerminalRedis() redis.UniversalClient {
	if host.handlers.rateLimiter == nil {
		return nil
	}
	return host.handlers.rateLimiter.redis
}
func (host runtimeHost) TaskQueueAvailable() bool { return host.handlers.taskClient != nil }
func (runtimeHost) TemplateApplicationIcon(template appstore.Template) string {
	return applicationapi.TemplateApplicationIcon(template)
}
func (runtimeHost) ShortID(value string) string { return applicationapi.ShortID(value) }
func (runtimeHost) NextReleaseRevision(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error) {
	return nextReleaseRevisionFor(tx, projectID, applicationID, deploymentTargetID)
}
func (runtimeHost) DeploymentTargetResponse(target model.DeploymentTarget) any {
	return deploymentTargetResponseFromModel(target)
}
