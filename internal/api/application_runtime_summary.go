package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

const applicationRuntimeNotDeployed = "not-deployed"

type applicationDeploymentSummary struct {
	TargetCount     int    `json:"targetCount"`
	DesiredReplicas int32  `json:"desiredReplicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	Status          string `json:"status"`
}

type applicationListItemResponse struct {
	model.Application
	DeploymentSummary applicationDeploymentSummary `json:"deploymentSummary"`
}

func (h *Handlers) applicationListItemsWithRuntime(ctx context.Context, project model.Project, applications []model.Application) ([]applicationListItemResponse, error) {
	items := make([]applicationListItemResponse, 0, len(applications))
	if len(applications) == 0 {
		return items, nil
	}

	applicationIDs := make([]string, 0, len(applications))
	for _, application := range applications {
		applicationIDs = append(applicationIDs, application.ID)
	}
	var targets []model.DeploymentTarget
	if err := h.dbWithContext(ctx).
		Where("project_id = ? and application_id in ? and delete_status <> ?", project.ID, applicationIDs, "deleted").
		Find(&targets).Error; err != nil {
		return nil, err
	}
	h.observeDeploymentTargets(ctx, project, targets)

	targetsByApplication := make(map[string][]model.DeploymentTarget, len(applications))
	for _, target := range targets {
		targetsByApplication[target.ApplicationID] = append(targetsByApplication[target.ApplicationID], target)
	}
	for _, application := range applications {
		items = append(items, applicationListItemResponse{
			Application:       application,
			DeploymentSummary: summarizeApplicationDeploymentTargets(targetsByApplication[application.ID]),
		})
	}
	return items, nil
}

func summarizeApplicationDeploymentTargets(targets []model.DeploymentTarget) applicationDeploymentSummary {
	summary := applicationDeploymentSummary{TargetCount: len(targets), Status: applicationRuntimeNotDeployed}
	if len(targets) == 0 {
		return summary
	}

	allReady := true
	hasRuntimeObservation := false
	for _, target := range targets {
		summary.DesiredReplicas += target.DesiredReplicas
		summary.ReadyReplicas += target.ReadyReplicas
		if target.Status == observation.StatusReady || target.Status == observation.StatusProgressing || target.Status == observation.StatusDegraded {
			hasRuntimeObservation = true
		}
		if target.Status != observation.StatusReady {
			allReady = false
		}
		switch target.Status {
		case observation.StatusUnavailable:
			summary.Status = observation.StatusUnavailable
		case observation.StatusDegraded:
			if summary.Status != observation.StatusUnavailable {
				summary.Status = observation.StatusDegraded
			}
		}
	}

	if summary.Status == observation.StatusUnavailable || summary.Status == observation.StatusDegraded {
		return summary
	}
	if allReady && hasRuntimeObservation && summary.ReadyReplicas >= summary.DesiredReplicas {
		summary.Status = observation.StatusReady
		return summary
	}
	if hasRuntimeObservation {
		summary.Status = observation.StatusProgressing
	}
	return summary
}
