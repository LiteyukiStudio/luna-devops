package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type applicationHost struct {
	domainHost
}

func (host applicationHost) NotificationEnqueuer() notification.DeliveryEnqueuer {
	return host.handlers.taskClient
}

func applicationDataVolumeInputsToRoot(inputs []applicationapi.DeploymentTargetDataVolumeInput) []deploymentTargetDataVolumeInput {
	result := make([]deploymentTargetDataVolumeInput, 0, len(inputs))
	for _, input := range inputs {
		converted := deploymentTargetDataVolumeInput{
			LogicalName: input.LogicalName, SourceType: input.SourceType, ProjectVolumeID: input.ProjectVolumeID,
			MountPath: input.MountPath, DevicePath: input.DevicePath, ReadOnly: input.ReadOnly,
		}
		if input.EmptyDir != nil {
			converted.EmptyDir = &deploymentTargetEmptyDirInput{Medium: input.EmptyDir.Medium, SizeLimit: input.EmptyDir.SizeLimit}
		}
		result = append(result, converted)
	}
	return result
}

func applicationDataVolumeInputsFromRoot(inputs []deploymentTargetDataVolumeInput) []applicationapi.DeploymentTargetDataVolumeInput {
	result := make([]applicationapi.DeploymentTargetDataVolumeInput, 0, len(inputs))
	for _, input := range inputs {
		converted := applicationapi.DeploymentTargetDataVolumeInput{
			LogicalName: input.LogicalName, SourceType: input.SourceType, ProjectVolumeID: input.ProjectVolumeID,
			MountPath: input.MountPath, DevicePath: input.DevicePath, ReadOnly: input.ReadOnly,
		}
		if input.EmptyDir != nil {
			converted.EmptyDir = &applicationapi.DeploymentTargetEmptyDirInput{Medium: input.EmptyDir.Medium, SizeLimit: input.EmptyDir.SizeLimit}
		}
		result = append(result, converted)
	}
	return result
}

func applicationVolumeChangesFromRoot(changes deploymentVolumeMountChanges) applicationapi.DeploymentVolumeMountChanges {
	attempted := make([]applicationapi.DeploymentVolumeAuditRecord, 0, len(changes.Attempted))
	for _, record := range changes.Attempted {
		attempted = append(attempted, applicationapi.DeploymentVolumeAuditRecord{
			Action: record.Action, Resource: record.Resource, Message: record.Message,
		})
	}
	return applicationapi.DeploymentVolumeMountChanges{
		Bound: changes.Bound, Unbound: changes.Unbound, HookBindings: changes.HookBindings, Attempted: attempted,
	}
}

func applicationVolumeChangesToRoot(changes applicationapi.DeploymentVolumeMountChanges) deploymentVolumeMountChanges {
	attempted := make([]deploymentVolumeAuditRecord, 0, len(changes.Attempted))
	for _, record := range changes.Attempted {
		attempted = append(attempted, deploymentVolumeAuditRecord{
			Action: record.Action, Resource: record.Resource, Message: record.Message,
		})
	}
	return deploymentVolumeMountChanges{
		Bound: changes.Bound, Unbound: changes.Unbound, HookBindings: changes.HookBindings, Attempted: attempted,
	}
}

func (host applicationHost) SyncDeploymentTargetVolumeMounts(ctx context.Context, tx *gorm.DB, target model.DeploymentTarget, inputs []applicationapi.DeploymentTargetDataVolumeInput) (applicationapi.DeploymentVolumeMountChanges, error) {
	changes, err := syncDeploymentTargetVolumeMounts(ctx, tx, target, applicationDataVolumeInputsToRoot(inputs))
	return applicationVolumeChangesFromRoot(changes), err
}

func (host applicationHost) NextReleaseRevisionFor(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error) {
	return nextReleaseRevisionFor(tx, projectID, applicationID, deploymentTargetID)
}

func (host applicationHost) AuditDeploymentVolumeMountFailure(ctx context.Context, userID string, changes applicationapi.DeploymentVolumeMountChanges, err error) {
	host.handlers.auditDeploymentVolumeMountFailure(ctx, userID, applicationVolumeChangesToRoot(changes), err)
}

func (host applicationHost) AuditDeploymentVolumeMountChanges(ctx context.Context, userID string, target model.DeploymentTarget, changes applicationapi.DeploymentVolumeMountChanges) {
	host.handlers.auditDeploymentVolumeMountChanges(ctx, userID, target, applicationVolumeChangesToRoot(changes))
}

func (host applicationHost) WriteVolumeError(ctx *gin.Context, err error) { writeVolumeError(ctx, err) }

func (host applicationHost) DeploymentTargetVolumeMountsByTarget(ctx context.Context, targets []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error) {
	return host.handlers.deploymentTargetVolumeMountsByTarget(ctx, targets)
}

func (host applicationHost) DeploymentTargetResponseFromModel(target model.DeploymentTarget, mounts []model.DeploymentVolumeMount) any {
	return deploymentTargetResponseFromModel(target, mounts)
}

func (host applicationHost) NormalizePublicStage(value string) (string, bool) {
	return normalizePublicStage(value)
}

func (host applicationHost) WriteDeploymentStageInvalid(ctx *gin.Context, path, detail string) {
	writeDeploymentStageInvalid(ctx, path, detail)
}

func (host applicationHost) NormalizeBuildResourceQuantity(ctx *gin.Context, value, fallbackValue, label string) (string, bool) {
	return normalizeBuildResourceQuantity(ctx, value, fallbackValue, label)
}

func (host applicationHost) NormalizeDataVolumes(ctx *gin.Context, inputs []applicationapi.DeploymentTargetDataVolumeInput) ([]applicationapi.DeploymentTargetDataVolumeInput, bool) {
	normalized, ok := normalizeDataVolumes(ctx, applicationDataVolumeInputsToRoot(inputs))
	return applicationDataVolumeInputsFromRoot(normalized), ok
}

func (host applicationHost) NormalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool) {
	return runtimeapi.NormalizeRuntimeConfigFilesInput(ctx, value)
}

func (host applicationHost) NormalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	return runtimeapi.NormalizeRuntimeConfigFilePathInput(ctx, value)
}

func (host applicationHost) IsBuildEnvKey(value string) bool { return buildapi.IsBuildEnvKey(value) }

func (host applicationHost) ObserveDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget) {
	host.handlers.observeDeploymentTargets(ctx, project, targets)
}

func (host applicationHost) DeploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string {
	return runtimeapi.DeploymentTargetNamespace(project, target)
}

func (host applicationHost) KubernetesClientForDeploymentTargetObservation(project model.Project, target model.DeploymentTarget, ctx context.Context) (*kubeprovider.Client, string, string) {
	return host.handlers.kubernetesClientForDeploymentTargetObservation(project, target, ctx)
}

func (host applicationHost) EnqueueApplicationDelete(ctx context.Context, app model.Application, actorID string, deleteData bool) bool {
	if host.handlers.taskClient == nil {
		return false
	}
	_, err := host.handlers.taskClient.EnqueueApplicationDelete(ctx, tasks.ApplicationDeletePayload{
		ApplicationID: app.ID, ProjectID: app.ProjectID, ActorID: actorID, DeleteData: deleteData,
	})
	return err == nil
}
