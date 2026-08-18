package api

import (
	"context"
	"sort"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

const applicationRuntimeNotDeployed = "not-deployed"

type applicationDeploymentSummary struct {
	TargetCount     int                                  `json:"targetCount"`
	DesiredReplicas int32                                `json:"desiredReplicas"`
	ReadyReplicas   int32                                `json:"readyReplicas"`
	Status          string                               `json:"status"`
	Targets         []applicationDeploymentTargetSummary `json:"targets"`
}

type applicationDeploymentTargetSummary struct {
	ID              string `json:"id"`
	Stage           string `json:"stage"`
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
	summary := applicationDeploymentSummary{
		TargetCount: len(targets),
		Status:      applicationRuntimeNotDeployed,
		Targets:     make([]applicationDeploymentTargetSummary, 0, len(targets)),
	}
	if len(targets) == 0 {
		return summary
	}

	allObservedTargetsHealthy := true
	hasRuntimeObservation := false
	for _, target := range targets {
		summary.Targets = append(summary.Targets, applicationDeploymentTargetSummary{
			ID:              target.ID,
			Stage:           target.Stage,
			DesiredReplicas: target.DesiredReplicas,
			ReadyReplicas:   target.ReadyReplicas,
			Status:          target.Status,
		})
		summary.DesiredReplicas += target.DesiredReplicas
		summary.ReadyReplicas += target.ReadyReplicas
		if target.Status == observation.StatusReady || target.Status == observation.StatusScaledToZero || target.Status == observation.StatusProgressing || target.Status == observation.StatusDegraded {
			hasRuntimeObservation = true
		}
		if target.Status != observation.StatusReady && target.Status != observation.StatusScaledToZero {
			allObservedTargetsHealthy = false
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
	sort.SliceStable(summary.Targets, func(i, j int) bool {
		left, right := summary.Targets[i], summary.Targets[j]
		if applicationDeploymentStatusPriority(left.Status) != applicationDeploymentStatusPriority(right.Status) {
			return applicationDeploymentStatusPriority(left.Status) < applicationDeploymentStatusPriority(right.Status)
		}
		return applicationDeploymentStagePriority(left.Stage) < applicationDeploymentStagePriority(right.Stage)
	})

	if summary.Status == observation.StatusUnavailable || summary.Status == observation.StatusDegraded {
		return summary
	}
	if allObservedTargetsHealthy && hasRuntimeObservation && summary.DesiredReplicas == 0 {
		summary.Status = observation.StatusScaledToZero
		return summary
	}
	if allObservedTargetsHealthy && hasRuntimeObservation && summary.ReadyReplicas >= summary.DesiredReplicas {
		summary.Status = observation.StatusReady
		return summary
	}
	if hasRuntimeObservation {
		summary.Status = observation.StatusProgressing
	}
	return summary
}

func applicationDeploymentStatusPriority(status string) int {
	switch status {
	case observation.StatusUnavailable:
		return 0
	case observation.StatusDegraded:
		return 1
	case observation.StatusProgressing:
		return 2
	case observation.StatusNotFound:
		return 3
	case observation.StatusNotConfigured:
		return 4
	case observation.StatusReady:
		return 5
	case observation.StatusScaledToZero:
		return 6
	case "disabled":
		return 7
	default:
		return 8
	}
}

func applicationDeploymentStagePriority(stage string) int {
	switch stage {
	case "prod":
		return 0
	case "staging":
		return 1
	case "test":
		return 2
	case "dev":
		return 3
	default:
		return 4
	}
}
