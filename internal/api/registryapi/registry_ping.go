package registryapi

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
)

func (h *Handler) pingRegistry(parent context.Context, user model.User, registry model.ArtifactRegistry) registryTestResult {
	credentialInput := h.registryCredentialInput(parent, user, registry)
	result := registryprovider.Ping(parent, registry.Endpoint, h.egressPolicyForUser(user, parent), credentialInput)
	return registryTestResult{
		Success:    result.Success,
		StatusCode: result.StatusCode,
		Message:    result.Message,
		Endpoint:   result.Endpoint,
	}
}
