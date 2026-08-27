package volume

import (
	"context"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

// ListDeploymentTargetMounts returns the complete bounded-by-target desired
// mount set. A deployment target is a fixed configuration object; each mount
// path is unique, so this list does not expose a high-growth project query.
func (service *Service) ListDeploymentTargetMounts(ctx context.Context, projectID, targetID string) (result []model.DeploymentVolumeMount, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume", "mount.list_target")
	defer func() { end(err) }()
	projectID = strings.TrimSpace(projectID)
	targetID = strings.TrimSpace(targetID)
	if projectID == "" || targetID == "" {
		return nil, newDomainError(CodeInvalidInput, "project id and deployment target id are required")
	}
	return service.repository.ListDeploymentTargetMounts(ctx, projectID, targetID)
}

// RestoreReleasePendingDeploymentVolumeMount restores an unchanged desired
// mount while a previous rollout is still waiting to detach it. The narrow
// transition avoids creating a second relation that could bypass RWO
// exclusivity. If the worker already completed deletion, callers receive a
// stable state conflict and may retry the deployment update.
func (service *Service) RestoreReleasePendingDeploymentVolumeMount(ctx context.Context, projectID, mountID string) (model.DeploymentVolumeMount, error) {
	return service.transitionMount(ctx, "bind.restore", projectID, mountID,
		[]string{model.DeploymentVolumeActivationReleasePending},
		model.DeploymentVolumeActivationReserved, "", "")
}
