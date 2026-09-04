package api

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

type platformHost struct {
	domainHost
}

func (host platformHost) Config() Config { return host.handlers.config }
func (host platformHost) ObserveRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster) {
	host.handlers.domains.runtime.ObserveRuntimeClusters(ctx, clusters)
}
func (host platformHost) DashboardRegistryAvailable(ctx context.Context, user model.User, registry model.ArtifactRegistry) bool {
	return host.handlers.pingRegistry(ctx, user, registry).Success
}
func (host platformHost) AllowRate(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return host.handlers.rateLimiter.allow(ctx, key, limit, window)
}
