package api

import (
	"context"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/registryapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type registryHost struct {
	domainHost
}

func (host registryHost) ApplyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User, ctx context.Context) *gorm.DB {
	return host.handlers.applyScopedResourceVisibilityForUser(query, resourceType, user, ctx)
}

func (host registryHost) ScopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return host.handlers.scopedResourceDefaultProjectIDMap(resourceType, resourceIDs, ctx)
}

func (host registryHost) SecretAvailable() bool {
	return host.handlers.secrets.Available()
}

func (host registryHost) AllowRegistrySearch(ctx *gin.Context, userID string) bool {
	limit := 60
	if host.handlers.mode == "development" {
		limit = developmentRateLimit
	}
	allowed, err := host.handlers.rateLimiter.allow(ctx.Request.Context(), "registry_search:"+userID, limit, time.Minute)
	if allowed {
		return true
	}
	if err != nil && host.handlers.mode == "development" {
		return true
	}
	transportapi.WriteError(ctx, http.StatusTooManyRequests, "镜像搜索请求过于频繁，请稍后再试")
	return false
}

func (h *Handlers) registryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return h.domains.registry.RegistryPushCredentialForProject(user, registry, projectID, ctx)
}

func (h *Handlers) registryCredentialInput(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryprovider.Credential {
	return h.domains.registry.RegistryCredentialInput(ctx, user, registry)
}

func (h *Handlers) pingRegistry(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryapi.RegistryTestResult {
	return h.domains.registry.PingRegistry(ctx, user, registry)
}

func registryResponse(registry model.ArtifactRegistry) registryapi.ArtifactRegistryOutput {
	return registryapi.RegistryResponse(registry)
}

func normalizeRegistryProvider(value string) string {
	return registryapi.NormalizeRegistryProvider(value)
}

func containerImagePageQuery(query *gorm.DB, pagination transportapi.PaginationParams) *gorm.DB {
	return registryapi.ContainerImagePageQuery(query, pagination)
}
