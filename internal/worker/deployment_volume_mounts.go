package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

type applicationVolumeAttachmentObserver interface {
	ObserveApplicationVolumeAttachments(context.Context, string, string, string) (map[string]kubeprovider.ApplicationVolumeAttachment, error)
}

func (r *Runner) deploymentTargetDataVolumes(ctx context.Context, target model.DeploymentTarget, namespace string) ([]kubeprovider.ApplicationDataVolume, error) {
	service, err := r.projectVolumeService()
	if err != nil {
		return nil, err
	}
	mounts, err := service.ListDeploymentTargetMounts(ctx, target.ProjectID, target.ID)
	if err != nil {
		return nil, err
	}
	resolved := make([]kubeprovider.ApplicationDataVolume, 0, len(mounts))
	for _, mount := range mounts {
		if mount.ActivationState == model.DeploymentVolumeActivationReleasePending {
			continue
		}
		item := kubeprovider.ApplicationDataVolume{
			Name:              mount.LogicalName,
			MountPath:         optionalMountPath(mount.MountPath),
			DevicePath:        optionalMountPath(mount.DevicePath),
			ReadOnly:          mount.ReadOnly,
			EmptyDirMedium:    mount.EmptyDirMedium,
			EmptyDirSizeLimit: mount.EmptyDirSizeLimit,
		}
		switch mount.SourceType {
		case model.DeploymentVolumeSourceEmptyDir:
			item.SourceType = "emptyDir"
		case model.DeploymentVolumeSourceProjectVolume:
			if mount.ProjectVolumeID == nil {
				return nil, fmt.Errorf("deployment volume %s has no project volume id", mount.ID)
			}
			projectVolume, volumeErr := service.GetProjectVolume(ctx, target.ProjectID, *mount.ProjectVolumeID)
			if volumeErr != nil {
				return nil, volumeErr
			}
			if !volume.CanAttachProjectVolume(projectVolume) {
				return nil, fmt.Errorf("project volume %s is not attachable", projectVolume.ID)
			}
			if projectVolume.ClusterID != target.ClusterID || projectVolume.Namespace != namespace {
				return nil, fmt.Errorf("project volume %s is not compatible with the deployment target", projectVolume.ID)
			}
			item.SourceType = "projectVolume"
			item.ProjectVolumeID = projectVolume.ID
			item.ClaimName = projectVolume.ClaimName
		default:
			return nil, fmt.Errorf("deployment volume %s has unsupported source type", mount.ID)
		}
		resolved = append(resolved, item)
	}
	return resolved, nil
}

func (r *Runner) reconcileDeploymentVolumeMounts(ctx context.Context, target model.DeploymentTarget, namespace string) error {
	service, err := r.projectVolumeService()
	if err != nil {
		return err
	}
	mounts, err := service.ListDeploymentTargetMounts(ctx, target.ProjectID, target.ID)
	if err != nil || len(mounts) == 0 {
		return err
	}
	manager, err := r.kubernetesManager(ctx, target)
	if err != nil {
		return err
	}
	observer, ok := manager.(applicationVolumeAttachmentObserver)
	if !ok {
		return fmt.Errorf("kubernetes provider does not support application volume observation")
	}
	attachments, err := observer.ObserveApplicationVolumeAttachments(ctx, namespace, applicationResourceName(target), target.WorkloadType)
	if err != nil {
		return err
	}
	for _, mount := range mounts {
		attachment, attached := attachments[mount.LogicalName]
		if mount.ActivationState == model.DeploymentVolumeActivationReleasePending {
			if attached {
				return fmt.Errorf("deployment volume %s is still attached after rollout", mount.ID)
			}
			if err := service.CompleteDeploymentVolumeUnbind(ctx, target.ProjectID, mount.ID); err != nil {
				return err
			}
			continue
		}
		if mount.ActivationState != model.DeploymentVolumeActivationReserved && mount.ActivationState != model.DeploymentVolumeActivationActive && mount.ActivationState != model.DeploymentVolumeActivationError {
			continue
		}
		matches, matchErr := deploymentVolumeAttachmentMatches(ctx, service, target.ProjectID, mount, attachment, attached)
		if matchErr != nil {
			return matchErr
		}
		if !matches {
			_, _ = service.FailDeploymentVolumeMount(ctx, target.ProjectID, mount.ID, volume.CodeBindingConflict, "authoritative workload volume attachment does not match the desired relation")
			return fmt.Errorf("deployment volume %s does not match the workload after rollout", mount.ID)
		}
		if mount.ActivationState != model.DeploymentVolumeActivationActive {
			if _, err := service.ActivateDeploymentVolumeMount(ctx, target.ProjectID, mount.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Runner) releaseDeploymentTargetVolumeMountsAfterCleanup(ctx context.Context, target model.DeploymentTarget, manager kubeprovider.NamespaceManager, namespace string) error {
	service, err := r.projectVolumeService()
	if err != nil {
		return err
	}
	mounts, err := service.ListDeploymentTargetMounts(ctx, target.ProjectID, target.ID)
	if err != nil || len(mounts) == 0 {
		return err
	}
	for index := range mounts {
		if mounts[index].ActivationState == model.DeploymentVolumeActivationReleasePending {
			continue
		}
		mounts[index], err = service.BeginDeploymentVolumeUnbind(ctx, target.ProjectID, mounts[index].ID)
		if err != nil {
			return err
		}
	}
	observer, ok := manager.(applicationVolumeAttachmentObserver)
	if !ok {
		return fmt.Errorf("kubernetes provider does not support application volume observation")
	}
	_, err = observer.ObserveApplicationVolumeAttachments(ctx, namespace, applicationResourceName(target), target.WorkloadType)
	if err == nil {
		return fmt.Errorf("application workload deletion is still pending before volume release")
	}
	if !isKubernetesNotFound(err) {
		return err
	}
	for _, mount := range mounts {
		if err := service.CompleteDeploymentVolumeUnbind(ctx, target.ProjectID, mount.ID); err != nil {
			return err
		}
	}
	return nil
}

func deploymentVolumeAttachmentMatches(ctx context.Context, service volumeWorkerService, projectID string, mount model.DeploymentVolumeMount, attachment kubeprovider.ApplicationVolumeAttachment, attached bool) (bool, error) {
	if !attached || optionalMountPath(mount.MountPath) != attachment.MountPath || optionalMountPath(mount.DevicePath) != attachment.DevicePath || mount.ReadOnly != attachment.ReadOnly {
		return false, nil
	}
	switch mount.SourceType {
	case model.DeploymentVolumeSourceEmptyDir:
		return attachment.EmptyDir && attachment.ClaimName == "", nil
	case model.DeploymentVolumeSourceProjectVolume:
		if mount.ProjectVolumeID == nil {
			return false, nil
		}
		projectVolume, err := service.GetProjectVolume(ctx, projectID, *mount.ProjectVolumeID)
		if err != nil {
			return false, err
		}
		return !attachment.EmptyDir && attachment.ClaimName == projectVolume.ClaimName, nil
	default:
		return false, nil
	}
}

func optionalMountPath(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
