package deploymentapi

import (
	"context"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"gorm.io/gorm"
)

type deploymentVolumeMountChanges struct {
	Bound        []model.DeploymentVolumeMount
	Unbound      []model.DeploymentVolumeMount
	HookBindings []model.DeploymentTargetHookBinding
	// Attempted records every binding mutation that belongs to the surrounding
	// transaction. It is retained when that transaction rolls back so the HTTP
	// boundary can emit a failure audit without persisting paths or user input.
	Attempted []deploymentVolumeAuditRecord
}

type deploymentVolumeAuditRecord struct {
	Action   string
	Resource string
	Message  string
}

func syncDeploymentTargetVolumeMounts(ctx context.Context, tx *gorm.DB, target model.DeploymentTarget, inputs []deploymentTargetDataVolumeInput) (deploymentVolumeMountChanges, error) {
	changes := deploymentVolumeMountChanges{}
	service := volume.NewGormService(tx)
	existing, err := service.ListDeploymentTargetMounts(ctx, target.ProjectID, target.ID)
	if err != nil {
		return changes, err
	}
	desired := make([]volume.ReserveMountInput, 0, len(inputs))
	for _, input := range inputs {
		switch input.SourceType {
		case "projectVolume":
			desired = append(desired, volume.ReserveMountInput{
				ProjectID: target.ProjectID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID,
				SourceType: model.DeploymentVolumeSourceProjectVolume, ProjectVolumeID: strings.TrimSpace(input.ProjectVolumeID),
				LogicalName: input.LogicalName, MountPath: input.MountPath, DevicePath: input.DevicePath, ReadOnly: input.ReadOnly,
			})
		case "emptyDir":
			emptyDir := deploymentTargetEmptyDirInput{}
			if input.EmptyDir != nil {
				emptyDir = *input.EmptyDir
			}
			desired = append(desired, volume.ReserveMountInput{
				ProjectID: target.ProjectID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID,
				SourceType: model.DeploymentVolumeSourceEmptyDir, LogicalName: input.LogicalName, MountPath: input.MountPath,
				EmptyDirMedium: emptyDir.Medium, EmptyDirSizeLimit: emptyDir.SizeLimit,
			})
		}
	}

	matchedExisting := make(map[string]bool, len(existing))
	for _, wanted := range desired {
		matched := false
		for _, current := range existing {
			if current.LogicalName != wanted.LogicalName {
				continue
			}
			if !deploymentVolumeMountMatches(current, wanted) {
				changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForMount("deployment_volume.bind", target, current))
				return changes, &volume.DomainError{Code: volume.CodeBindingConflict, Message: "an active deployment volume must be removed before reusing its logical name or path"}
			}
			if current.ActivationState == model.DeploymentVolumeActivationReleasePending {
				changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForMount("deployment_volume.bind", target, current))
				restored, restoreErr := service.RestoreReleasePendingDeploymentVolumeMount(ctx, target.ProjectID, current.ID)
				if restoreErr != nil {
					return changes, restoreErr
				}
				changes.Bound = append(changes.Bound, restored)
			}
			matchedExisting[current.ID] = true
			matched = true
			break
		}
		if !matched {
			changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForDesired("deployment_volume.bind", target, wanted))
			mount, reserveErr := service.ReserveDeploymentVolumeMount(ctx, wanted)
			if reserveErr != nil {
				return changes, reserveErr
			}
			changes.Bound = append(changes.Bound, mount)
		}
	}
	for _, current := range existing {
		if matchedExisting[current.ID] || current.ActivationState == model.DeploymentVolumeActivationReleasePending {
			continue
		}
		changes.Attempted = append(changes.Attempted, deploymentVolumeAuditRecordForMount("deployment_volume.unbind", target, current))
		mount, unbindErr := service.BeginDeploymentVolumeUnbind(ctx, target.ProjectID, current.ID)
		if unbindErr != nil {
			return changes, unbindErr
		}
		changes.Unbound = append(changes.Unbound, mount)
	}
	return changes, nil
}

func (h *Handlers) deploymentTargetVolumeMountsByTarget(ctx context.Context, targets []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error) {
	result := make(map[string][]model.DeploymentVolumeMount, len(targets))
	if len(targets) == 0 {
		return result, nil
	}
	targetIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		targetIDs = append(targetIDs, target.ID)
		result[target.ID] = []model.DeploymentVolumeMount{}
	}
	var mounts []model.DeploymentVolumeMount
	if err := h.dbWithContext(ctx).Where("deployment_target_id in ?", targetIDs).
		Order("created_at asc, id asc").Find(&mounts).Error; err != nil {
		return nil, err
	}
	for _, mount := range mounts {
		result[mount.DeploymentTargetID] = append(result[mount.DeploymentTargetID], mount)
	}
	return result, nil
}

func deploymentTargetDataVolumeResponses(mounts []model.DeploymentVolumeMount) []deploymentTargetDataVolumeResponse {
	responses := make([]deploymentTargetDataVolumeResponse, 0, len(mounts))
	for _, mount := range mounts {
		response := deploymentTargetDataVolumeResponse{
			BindingID:       mount.ID,
			LogicalName:     mount.LogicalName,
			ProjectVolumeID: optionalStringValue(mount.ProjectVolumeID),
			MountPath:       optionalStringValue(mount.MountPath),
			DevicePath:      optionalStringValue(mount.DevicePath),
			ReadOnly:        mount.ReadOnly,
			ActivationState: mount.ActivationState,
		}
		switch mount.SourceType {
		case model.DeploymentVolumeSourceProjectVolume:
			response.SourceType = "projectVolume"
		case model.DeploymentVolumeSourceEmptyDir:
			response.SourceType = "emptyDir"
			response.EmptyDir = &deploymentTargetEmptyDirInput{Medium: mount.EmptyDirMedium, SizeLimit: mount.EmptyDirSizeLimit}
		default:
			continue
		}
		responses = append(responses, response)
	}
	return responses
}

func (h *Handlers) auditDeploymentVolumeMountChanges(ctx context.Context, userID string, target model.DeploymentTarget, changes deploymentVolumeMountChanges) {
	for _, record := range deploymentVolumeAuditRecords(target, changes) {
		h.auditWithContext(userID, record.Action, record.Resource, true, record.Message, ctx)
	}
}

func (h *Handlers) auditDeploymentVolumeMountFailure(ctx context.Context, userID string, changes deploymentVolumeMountChanges, err error) {
	for _, record := range deploymentVolumeFailureAuditRecords(changes, err) {
		h.auditWithContext(userID, record.Action, record.Resource, false, record.Message, ctx)
	}
}

func deploymentVolumeFailureAuditRecords(changes deploymentVolumeMountChanges, err error) []deploymentVolumeAuditRecord {
	code := volumeAuditErrorCode(err)
	records := make([]deploymentVolumeAuditRecord, 0, len(changes.Attempted))
	for _, attempted := range changes.Attempted {
		records = append(records, deploymentVolumeAuditRecord{
			Action:   attempted.Action,
			Resource: attempted.Resource,
			Message:  code,
		})
	}
	return records
}

func deploymentVolumeAuditRecords(target model.DeploymentTarget, changes deploymentVolumeMountChanges) []deploymentVolumeAuditRecord {
	records := make([]deploymentVolumeAuditRecord, 0, len(changes.Bound)+len(changes.Unbound))
	for _, mount := range changes.Bound {
		records = append(records, deploymentVolumeAuditRecord{Action: "deployment_volume.bind", Resource: mount.ID, Message: deploymentVolumeAuditMessage(target.ID, mount)})
	}
	for _, mount := range changes.Unbound {
		records = append(records, deploymentVolumeAuditRecord{Action: "deployment_volume.unbind", Resource: mount.ID, Message: deploymentVolumeAuditMessage(target.ID, mount)})
	}
	return records
}

func deploymentVolumeAuditMessage(targetID string, mount model.DeploymentVolumeMount) string {
	if mount.ProjectVolumeID == nil {
		return strings.TrimSpace(targetID)
	}
	return strings.TrimSpace(targetID) + ":" + strings.TrimSpace(*mount.ProjectVolumeID)
}

func deploymentVolumeAuditRecordForMount(action string, target model.DeploymentTarget, mount model.DeploymentVolumeMount) deploymentVolumeAuditRecord {
	return deploymentVolumeAuditRecord{Action: action, Resource: mount.ID, Message: deploymentVolumeAuditMessage(target.ID, mount)}
}

func deploymentVolumeAuditRecordForDesired(action string, target model.DeploymentTarget, wanted volume.ReserveMountInput) deploymentVolumeAuditRecord {
	resource := strings.TrimSpace(wanted.ProjectVolumeID)
	if resource == "" {
		resource = strings.TrimSpace(target.ID)
	}
	message := strings.TrimSpace(target.ID)
	if strings.TrimSpace(wanted.ProjectVolumeID) != "" {
		message += ":" + strings.TrimSpace(wanted.ProjectVolumeID)
	}
	return deploymentVolumeAuditRecord{Action: action, Resource: resource, Message: message}
}

func deploymentVolumeMountMatches(current model.DeploymentVolumeMount, wanted volume.ReserveMountInput) bool {
	return current.SourceType == wanted.SourceType && optionalStringValue(current.ProjectVolumeID) == strings.TrimSpace(wanted.ProjectVolumeID) &&
		optionalStringValue(current.MountPath) == strings.TrimSpace(wanted.MountPath) && optionalStringValue(current.DevicePath) == strings.TrimSpace(wanted.DevicePath) &&
		current.ReadOnly == wanted.ReadOnly && current.EmptyDirMedium == strings.TrimSpace(wanted.EmptyDirMedium) &&
		current.EmptyDirSizeLimit == strings.TrimSpace(wanted.EmptyDirSizeLimit)
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
