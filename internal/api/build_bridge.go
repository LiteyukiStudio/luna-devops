package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/api/deploymentapi"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
)

type buildHost struct {
	domainHost
}

func (host buildHost) EnsureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool {
	return host.handlers.ensureBillingAllowsNewBuild(ctx, projectID)
}
func (host buildHost) BuildQueueAvailable() bool { return host.handlers.taskClient != nil }
func (host buildHost) EnqueueBuildRun(ctx context.Context, payload tasks.BuildRunPayload) error {
	_, err := host.handlers.taskClient.EnqueueBuildRun(ctx, payload)
	return err
}
func (host buildHost) NormalizeDeploymentSourceType(value string) string {
	return deploymentapi.NormalizeDeploymentSourceType(value)
}
func (host buildHost) NormalizeBuildResourceQuantityValue(value, fallbackValue, label string) (string, error) {
	return deploymentapi.NormalizeBuildResourceQuantityValue(value, fallbackValue, label)
}
func (host buildHost) NormalizeBuildTimeoutSecondsValue(value int) int {
	return deploymentapi.NormalizeBuildTimeoutSecondsValue(value)
}
