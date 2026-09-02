package registryapi

import (
	"context"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

const registryObservationTimeout = 8 * time.Second

func (h *Handler) observeArtifactRegistries(parent context.Context, user model.User, registries []model.ArtifactRegistry) {
	var wg sync.WaitGroup
	limit := make(chan struct{}, 6)
	for index := range registries {
		wg.Add(1)
		go func(registry *model.ArtifactRegistry) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			h.observeArtifactRegistry(parent, user, registry)
		}(&registries[index])
	}
	wg.Wait()
}

func (h *Handler) observeArtifactRegistry(parent context.Context, user model.User, registry *model.ArtifactRegistry) {
	observedAt := time.Now().UTC()
	registry.LastCheckedAt = &observedAt
	if registry.Endpoint == "" {
		registry.Status = observation.StatusNotConfigured
		registry.ObservationCode = "registry.endpoint_missing"
		return
	}

	ctx, cancel := context.WithTimeout(parent, registryObservationTimeout)
	defer cancel()
	result := h.pingRegistry(ctx, user, *registry)
	if result.Success {
		registry.Status = observation.StatusReady
		registry.ObservationCode = ""
		return
	}
	if ctx.Err() != nil || result.StatusCode == 0 {
		registry.Status = observation.StatusUnavailable
		registry.ObservationCode = "registry.upstream_unavailable"
		return
	}
	registry.Status = observation.StatusDegraded
	registry.ObservationCode = "registry.health_check_failed"
}
