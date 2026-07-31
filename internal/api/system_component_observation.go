package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

func (h *Handlers) observeSystemComponentInstallations(ctx context.Context, items []model.SystemComponentInstallation) {
	const concurrency = 6
	guard := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	for index := range items {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case guard <- struct{}{}:
				defer func() { <-guard }()
			case <-ctx.Done():
				items[index] = unavailableSystemComponent(items[index], "system_component.observation_cancelled")
				return
			}
			items[index] = h.observeSystemComponentInstallation(ctx, items[index])
		}()
	}
	wait.Wait()
}

func (h *Handlers) observeSystemComponentInstallation(ctx context.Context, item model.SystemComponentInstallation) model.SystemComponentInstallation {
	observedAt := time.Now().UTC()
	item.ObservedAt = &observedAt
	if strings.TrimSpace(item.DeploymentTargetID) == "" || strings.TrimSpace(item.ProjectID) == "" {
		item.RuntimeStatus = observation.StatusNotConfigured
		item.ObservationCode = "system_component.deployment_target_not_configured"
		return item
	}

	var project model.Project
	if err := h.dbWithContext(ctx).First(&project, "id = ?", item.ProjectID).Error; err != nil {
		return unavailableSystemComponent(item, "system_component.project_unavailable")
	}
	var target model.DeploymentTarget
	if err := h.dbWithContext(ctx).First(&target, "id = ? and project_id = ? and deleted_at is null", item.DeploymentTargetID, item.ProjectID).Error; err != nil {
		item.RuntimeStatus = observation.StatusNotFound
		item.ObservationCode = "system_component.deployment_target_not_found"
		return item
	}
	target = h.observeDeploymentTarget(ctx, project, target)
	item.RuntimeStatus = target.Status
	item.ObservationCode = target.ObservationCode
	item.ObservedAt = target.LastCheckedAt
	return item
}

func unavailableSystemComponent(item model.SystemComponentInstallation, code string) model.SystemComponentInstallation {
	observedAt := time.Now().UTC()
	item.RuntimeStatus = observation.StatusUnavailable
	item.ObservationCode = code
	item.ObservedAt = &observedAt
	return item
}
