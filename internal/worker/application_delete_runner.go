package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func (r *Runner) handleApplicationDelete(ctx context.Context, task *asynq.Task) error {
	var payload tasks.ApplicationDeletePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	var app model.Application
	if err := r.db.First(&app, "id = ? and project_id = ?", payload.ApplicationID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !applicationDeleteTaskCanRun(app) {
		return nil
	}
	_ = r.db.Model(&model.Application{}).Where("id = ?", app.ID).Updates(map[string]any{
		"delete_status":  "deleting",
		"delete_message": "",
	}).Error
	if err := workerStage(ctx, "application_delete.cleanup_runtime", func(stageCtx context.Context) error {
		return r.cleanupApplicationRuntimeResources(stageCtx, payload)
	}); err != nil {
		_ = r.markApplicationDeleteFailed(payload.ApplicationID, err)
		return err
	}
	return workerStage(ctx, "application_delete.finalize", func(context.Context) error {
		return r.finishApplicationDelete(app, payload)
	})
}

func (r *Runner) cleanupApplicationRuntimeResources(ctx context.Context, payload tasks.ApplicationDeletePayload) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	var app model.Application
	if err := r.db.WithContext(ctx).First(&app, "id = ? and project_id = ?", payload.ApplicationID, payload.ProjectID).Error; err != nil {
		return fmt.Errorf("application not found: %w", err)
	}
	var targets []model.DeploymentTarget
	if err := r.db.Where("project_id = ? and application_id = ?", payload.ProjectID, payload.ApplicationID).Find(&targets).Error; err != nil {
		return err
	}
	kinds := []string{"services", "workloads", "configs"}
	if payload.DeleteData {
		kinds = append(kinds, "storage")
	}
	for _, target := range targets {
		environment := deploymentTargetEnvironment(target)
		manager, err := r.kubernetesManager(ctx, environment)
		if err != nil {
			return err
		}
		namespace := deploymentNamespace(project, environment)
		if !payload.DeleteData {
			if err := r.retainApplicationVolumes(ctx, manager, project, app, target, namespace); err != nil {
				return err
			}
		}
		for _, kind := range kinds {
			items, err := manager.ListManagedResources(ctx, kubeprovider.ResourceListOptions{
				Kind:          kind,
				Namespace:     namespace,
				ProjectID:     payload.ProjectID,
				ApplicationID: payload.ApplicationID,
			})
			if err != nil {
				if isKubernetesNotFound(err) {
					continue
				}
				return fmt.Errorf("list %s resources in %s: %w", kind, namespace, err)
			}
			for _, item := range items {
				if !payload.DeleteData && strings.EqualFold(item.Kind, "PersistentVolumeClaim") {
					continue
				}
				if err := manager.DeleteManagedResource(ctx, item.Kind, item.Namespace, item.Name); err != nil && !isKubernetesNotFound(err) {
					return fmt.Errorf("delete %s %s/%s: %w", item.Kind, item.Namespace, item.Name, err)
				}
			}
		}
	}
	return nil
}

func (r *Runner) retainApplicationVolumes(ctx context.Context, manager kubeprovider.NamespaceManager, project model.Project, app model.Application, target model.DeploymentTarget, namespace string) error {
	claims, err := manager.ListManagedPersistentVolumeClaims(ctx, namespace, target.ID)
	if err != nil {
		if isKubernetesNotFound(err) {
			return nil
		}
		return fmt.Errorf("list persistent data before retention: %w", err)
	}
	for _, claim := range claims {
		var retained model.RetainedVolume
		err := r.db.WithContext(ctx).Where("cluster_id = ? and namespace = ? and claim_name = ?", target.ClusterID, namespace, claim.Name).First(&retained).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			retained = model.RetainedVolume{
				ID: id.New("rvol"), ProjectID: project.ID,
				SourceApplicationID: app.ID, SourceApplicationName: app.Name, SourceDeploymentTargetID: target.ID,
				ClusterID: target.ClusterID, Namespace: namespace, ClaimName: claim.Name,
				VolumeName: retainedVolumeLogicalName(target, claim.Name), MountPath: retainedVolumeMountPath(target, claim.Name),
				Capacity: claim.Capacity, StorageClassName: claim.StorageClassName, AccessMode: claim.AccessMode, VolumeMode: claim.VolumeMode,
				Status: model.RetainedVolumeStatusRetaining, RetainedAt: time.Now(),
			}
			if err := r.db.WithContext(ctx).Create(&retained).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if retained.Status == model.RetainedVolumeStatusRetained {
			continue
		}
		if err := manager.RetainManagedPersistentVolumeClaim(ctx, namespace, claim.Name, target.ID, retained.ID); err != nil {
			_ = r.db.WithContext(ctx).Model(&retained).Updates(map[string]any{"status": model.RetainedVolumeStatusFailed, "last_error": trimReleaseLogContent(err.Error())}).Error
			return fmt.Errorf("retain persistent data %s/%s: %w", namespace, claim.Name, err)
		}
		if err := r.db.WithContext(ctx).Model(&retained).Updates(map[string]any{
			"source_application_id":       app.ID,
			"source_application_name":     app.Name,
			"source_deployment_target_id": target.ID,
			"status":                      model.RetainedVolumeStatusRetained,
			"claimed_by_application_id":   "",
			"claimed_by_target_id":        "",
			"claimed_at":                  nil,
			"retained_at":                 time.Now(),
			"last_error":                  "",
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func retainedVolumeLogicalName(target model.DeploymentTarget, claimName string) string {
	baseName := applicationResourceName(target)
	for _, volume := range deploymentTargetDataVolumes(target) {
		name := firstNonEmpty(strings.TrimSpace(volume.Name), "data")
		if claimName == baseName+"-data" && name == "data" || strings.HasSuffix(claimName, "-"+name+"-data") {
			return name
		}
	}
	return "data"
}

func retainedVolumeMountPath(target model.DeploymentTarget, claimName string) string {
	logicalName := retainedVolumeLogicalName(target, claimName)
	for _, volume := range deploymentTargetDataVolumes(target) {
		if firstNonEmpty(strings.TrimSpace(volume.Name), "data") == logicalName {
			return firstNonEmpty(strings.TrimSpace(volume.MountPath), "/data")
		}
	}
	return "/data"
}

func applicationDeleteTaskCanRun(app model.Application) bool {
	status := strings.TrimSpace(app.DeleteStatus)
	return status == "deleting" || status == "delete_failed"
}

func applicationRuntimeCanMutate(app model.Application) bool {
	status := strings.TrimSpace(app.DeleteStatus)
	return status == "" || status == "active"
}

func (r *Runner) markApplicationDeleteFailed(applicationID string, err error) error {
	finishedAt := time.Now()
	message := ""
	if err != nil {
		message = err.Error()
	}
	return r.db.Model(&model.Application{}).Where("id = ?", applicationID).Updates(map[string]any{
		"delete_status":      "delete_failed",
		"delete_message":     trimReleaseLogContent(message),
		"delete_finished_at": &finishedAt,
	}).Error
}

func (r *Runner) finishApplicationDelete(app model.Application, payload tasks.ApplicationDeletePayload) error {
	finishedAt := time.Now()
	dataRetentionMode := "retained"
	if payload.DeleteData {
		dataRetentionMode = "deleted"
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var incomingActive int64
		if err := tx.Model(&model.ServiceBinding{}).
			Where("target_application_id = ? and enabled = ?", app.ID, true).
			Count(&incomingActive).Error; err != nil {
			return err
		}
		if incomingActive > 0 {
			return fmt.Errorf("application is referenced by %d active service bindings", incomingActive)
		}
		if err := tx.Where("source_application_id = ? or (target_application_id = ? and enabled = ?)", app.ID, app.ID, false).
			Delete(&model.ServiceBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("source_application_id = ? or target_application_id = ?", app.ID, app.ID).
			Delete(&model.ProjectTopologyEdge{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? and application_id = ?", app.ProjectID, app.ID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? and application_id = ?", app.ProjectID, app.ID).Delete(&model.DeploymentTarget{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? and application_id = ?", app.ProjectID, app.ID).Delete(&model.GatewayRoute{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? and application_id = ?", app.ProjectID, app.ID).Delete(&model.RepositoryBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Application{}).Where("id = ?", app.ID).Updates(map[string]any{
			"delete_status":       "deleted",
			"delete_message":      "",
			"delete_finished_at":  &finishedAt,
			"data_retention_mode": dataRetentionMode,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&app).Error
	})
}
