package registryapi

import (
	"context"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"gorm.io/gorm"
)

type ArtifactRegistryOutput = artifactRegistryOutput
type RegistryTestResult = registryTestResult

func RegistryResponse(registry model.ArtifactRegistry) ArtifactRegistryOutput {
	return registryResponse(registry)
}

func NormalizeRegistryProvider(value string) string {
	return normalizeRegistryProvider(value)
}

func ContainerImagePageQuery(query *gorm.DB, pagination transportapi.PaginationParams) *gorm.DB {
	return containerImagePageQuery(query, pagination)
}

func (h *Handler) RegistryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return h.registryPushCredentialForProject(user, registry, projectID, ctx)
}

func (h *Handler) RegistryCredentialInput(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryprovider.Credential {
	return h.registryCredentialInput(ctx, user, registry)
}

func (h *Handler) PingRegistry(ctx context.Context, user model.User, registry model.ArtifactRegistry) RegistryTestResult {
	return h.pingRegistry(ctx, user, registry)
}
