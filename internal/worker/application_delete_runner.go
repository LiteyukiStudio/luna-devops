package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	if err := r.db.WithContext(ctx).First(&app, "id = ? and project_id = ?", payload.ApplicationID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !applicationDeleteTaskCanRun(app) {
		return nil
	}
	_ = r.db.WithContext(ctx).Model(&model.Application{}).Where("id = ?", app.ID).Updates(map[string]any{
		"delete_status":  "deleting",
		"delete_message": "",
	}).Error
	if err := workerStage(ctx, "application_delete.cleanup_runtime", func(stageCtx context.Context) error {
		return r.cleanupApplicationRuntimeResources(stageCtx, payload)
	}); err != nil {
		// Persist the terminal attempt state even when the task context was cancelled.
		// WithoutCancel preserves trace values while the timeout bounds this detached write.
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.markApplicationDeleteFailed(persistCtx, payload.ApplicationID, err)
		return err
	}
	return workerStage(ctx, "application_delete.finalize", func(stageCtx context.Context) error {
		return r.finishApplicationDelete(stageCtx, app)
	})
}

func (r *Runner) cleanupApplicationRuntimeResources(ctx context.Context, payload tasks.ApplicationDeletePayload) error {
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	var targets []model.DeploymentTarget
	if err := r.db.WithContext(ctx).Where("project_id = ? and application_id = ?", payload.ProjectID, payload.ApplicationID).Find(&targets).Error; err != nil {
		return err
	}
	if err := r.cleanupApplicationKubectlResources(ctx, project, payload.ApplicationID); err != nil {
		return err
	}
	kinds := []string{"services", "workloads", "configs"}
	for _, target := range targets {
		environment := deploymentTargetEnvironment(target)
		manager, err := r.kubernetesManager(ctx, environment)
		if err != nil {
			return err
		}
		namespace := deploymentNamespace(project, environment)
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
				if err := manager.DeleteManagedResource(ctx, item.Kind, item.Namespace, item.Name); err != nil && !isKubernetesNotFound(err) {
					return fmt.Errorf("delete %s %s/%s: %w", item.Kind, item.Namespace, item.Name, err)
				}
			}
		}
		if err := r.releaseDeploymentTargetVolumeMountsAfterCleanup(ctx, target, manager, namespace); err != nil {
			return err
		}
	}
	return nil
}

func applicationDeleteTaskCanRun(app model.Application) bool {
	status := strings.TrimSpace(app.DeleteStatus)
	return status == "deleting" || status == "delete_failed"
}

func applicationRuntimeCanMutate(app model.Application) bool {
	status := strings.TrimSpace(app.DeleteStatus)
	return status == "" || status == "active"
}

func (r *Runner) markApplicationDeleteFailed(ctx context.Context, applicationID string, err error) error {
	finishedAt := time.Now()
	message := ""
	if err != nil {
		message = err.Error()
	}
	return r.db.WithContext(ctx).Model(&model.Application{}).Where("id = ?", applicationID).Updates(map[string]any{
		"delete_status":      "delete_failed",
		"delete_message":     trimReleaseLogContent(message),
		"delete_finished_at": &finishedAt,
	}).Error
}

func (r *Runner) finishApplicationDelete(ctx context.Context, app model.Application) error {
	finishedAt := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if err := tx.Where("project_id = ? and application_id = ?", app.ProjectID, app.ID).Delete(&model.KubeAccessBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Application{}).Where("id = ?", app.ID).Updates(map[string]any{
			"delete_status":      "deleted",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&app).Error
	})
}
