package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
)

type gatewayHost struct {
	domainHost
}

func (host gatewayHost) EnsureGatewayRouteCanMutate(ctx *gin.Context, route model.GatewayRoute) bool {
	return host.handlers.ensureGatewayRouteCanMutate(ctx, route)
}

func (host gatewayHost) EnqueueGatewayApply(ctx context.Context, route model.GatewayRoute, actorID string) bool {
	if host.handlers.taskClient == nil {
		return false
	}
	_, err := host.handlers.taskClient.EnqueueGatewayApply(ctx, tasks.GatewayApplyPayload{
		GatewayRouteID:          route.ID,
		ProjectID:               route.ProjectID,
		ActorID:                 actorID,
		RouteUpdatedAtUnixMicro: route.UpdatedAt.UTC().UnixMicro(),
	})
	return err == nil
}

func (gatewayHost) NormalizeStage(value string) string { return normalizeStage(value) }

func (gatewayHost) NormalizeGatewayPublicScheme(value string) string {
	return runtimeapi.NormalizeGatewayPublicScheme(value)
}

func (gatewayHost) NormalizePort(value, fallbackValue int) int {
	return runtimeapi.NormalizePort(value, fallbackValue)
}

func (host gatewayHost) RuntimeClusterForDeploymentTarget(ctx context.Context, target model.DeploymentTarget) (model.RuntimeCluster, error) {
	return runtimeapi.RuntimeClusterForDeploymentTargetDB(host.handlers.dbWithContext(ctx), target)
}
