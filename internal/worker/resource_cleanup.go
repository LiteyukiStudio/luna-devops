package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func (r *Runner) retryPendingResourceCleanups(ctx context.Context) {
	for _, payload := range r.pendingResourceCleanupPayloads() {
		if err := r.handleResourceCleanupPayload(ctx, payload); err != nil {
			log.Printf("resource cleanup retry skipped type=%s id=%s: %v", payload.ResourceType, payload.ResourceID, err)
		}
	}
}

func (r *Runner) pendingResourceCleanupPayloads() []tasks.ResourceCleanupPayload {
	if r.db == nil {
		return nil
	}
	statuses := []string{"deleting", "delete_failed"}
	payloads := make([]tasks.ResourceCleanupPayload, 0)
	var projects []model.Project
	if err := r.db.Where("delete_status in ?", statuses).Limit(20).Find(&projects).Error; err == nil {
		for _, project := range projects {
			payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "project", ResourceID: project.ID, ProjectID: project.ID, ActorID: "system", DeleteData: true})
		}
	}
	var targets []model.DeploymentTarget
	if err := r.db.Where("delete_status in ?", statuses).Limit(50).Find(&targets).Error; err == nil {
		for _, target := range targets {
			payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "deployment_target", ResourceID: target.ID, ProjectID: target.ProjectID, ActorID: "system", DeleteData: !target.DataRetentionEnabled})
		}
	}
	var routes []model.GatewayRoute
	if err := r.db.Where("delete_status in ?", statuses).Limit(50).Find(&routes).Error; err == nil {
		for _, route := range routes {
			payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "gateway_route", ResourceID: route.ID, ProjectID: route.ProjectID, ActorID: "system"})
		}
	}
	var sets []model.ProjectRuntimeConfigSet
	if err := r.db.Where("delete_status in ?", statuses).Limit(50).Find(&sets).Error; err == nil {
		for _, set := range sets {
			payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "runtime_config", ResourceID: set.ID, ProjectID: set.ProjectID, ActorID: "system"})
		}
	}
	return payloads
}

func (r *Runner) handleResourceCleanup(ctx context.Context, task *asynq.Task) error {
	var payload tasks.ResourceCleanupPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	return r.handleResourceCleanupPayload(ctx, payload)
}

func (r *Runner) handleResourceCleanupPayload(ctx context.Context, payload tasks.ResourceCleanupPayload) error {
	switch strings.TrimSpace(payload.ResourceType) {
	case "project":
		return r.cleanupProject(ctx, payload)
	case "deployment_target":
		return r.cleanupDeploymentTarget(ctx, payload)
	case "gateway_route":
		return r.cleanupGatewayRoute(ctx, payload)
	case "runtime_config":
		return r.cleanupRuntimeConfigSet(payload)
	default:
		return fmt.Errorf("unsupported cleanup resource type: %s", payload.ResourceType)
	}
}

func (r *Runner) cleanupProject(ctx context.Context, payload tasks.ResourceCleanupPayload) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !resourceCleanupCanRun(project.DeleteStatus) {
		return nil
	}
	if err := r.cleanupProjectNamespaces(ctx, project); err != nil {
		_ = r.markProjectDeleteFailed(project.ID, err)
		return err
	}
	return r.finishProjectDelete(project)
}

func (r *Runner) cleanupProjectNamespaces(ctx context.Context, project model.Project) error {
	targets, err := r.projectCleanupDeploymentTargets(project.ID)
	if err != nil {
		return err
	}
	return r.cleanupProjectNamespacesForDeploymentTargets(ctx, project, targets)
}

func (r *Runner) cleanupProjectNamespacesForDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget) error {
	namespace := projectNamespace(project)
	if len(targets) == 0 {
		manager, err := r.kubernetesManager(model.Environment{})
		if err != nil {
			return err
		}
		return deleteManagedNamespace(ctx, manager, namespace)
	}
	seen := map[string]bool{}
	for _, target := range targets {
		environment := deploymentTargetEnvironment(target)
		key := projectCleanupEnvironmentKey(environment)
		if seen[key] {
			continue
		}
		seen[key] = true
		manager, err := r.kubernetesManager(environment)
		if err != nil {
			return err
		}
		if err := deleteManagedNamespace(ctx, manager, namespace); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) projectCleanupDeploymentTargets(projectID string) ([]model.DeploymentTarget, error) {
	var targets []model.DeploymentTarget
	if err := r.db.Where("project_id = ?", projectID).Order("created_at asc").Find(&targets).Error; err != nil {
		return nil, err
	}
	return targets, nil
}

func projectCleanupEnvironmentKey(environment model.Environment) string {
	clusterID := strings.TrimSpace(environment.ClusterID)
	if clusterID == "" {
		return "default"
	}
	return "cluster:" + clusterID
}

func deleteManagedNamespace(ctx context.Context, manager kubeprovider.NamespaceManager, namespace string) error {
	if err := manager.DeleteManagedResource(ctx, "Namespace", "", namespace); err != nil && !isKubernetesNotFound(err) {
		return err
	}
	return nil
}

func (r *Runner) cleanupDeploymentTarget(ctx context.Context, payload tasks.ResourceCleanupPayload) error {
	var target model.DeploymentTarget
	if err := r.db.First(&target, "id = ? and project_id = ?", payload.ResourceID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !resourceCleanupCanRun(target.DeleteStatus) {
		return nil
	}
	if err := r.cleanupDeploymentTargetRuntimeResources(ctx, target, payload.DeleteData); err != nil {
		_ = r.markDeploymentTargetDeleteFailed(target.ID, err)
		return err
	}
	return r.finishDeploymentTargetDelete(target)
}

func (r *Runner) cleanupGatewayRoute(ctx context.Context, payload tasks.ResourceCleanupPayload) error {
	var route model.GatewayRoute
	if err := r.db.First(&route, "id = ? and project_id = ?", payload.ResourceID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !resourceCleanupCanRun(route.DeleteStatus) {
		return nil
	}
	if err := r.cleanupGatewayRuntimeResources(ctx, route); err != nil {
		_ = r.markGatewayRouteDeleteFailed(route.ID, err)
		return err
	}
	return r.finishGatewayRouteDelete(route)
}

func (r *Runner) cleanupRuntimeConfigSet(payload tasks.ResourceCleanupPayload) error {
	var set model.ProjectRuntimeConfigSet
	if err := r.db.First(&set, "id = ? and project_id = ?", payload.ResourceID, payload.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !resourceCleanupCanRun(set.DeleteStatus) {
		return nil
	}
	return r.finishRuntimeConfigSetDelete(set)
}

func resourceCleanupCanRun(status string) bool {
	status = strings.TrimSpace(status)
	return status == "deleting" || status == "delete_failed"
}

func (r *Runner) cleanupDeploymentTargetRuntimeResources(ctx context.Context, target model.DeploymentTarget, deleteData bool) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", target.ProjectID).Error; err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	environment := deploymentTargetEnvironment(target)
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project, environment)
	kinds := []string{"services", "workloads", "configs"}
	if deleteData {
		kinds = append(kinds, "storage")
	}
	for _, kind := range kinds {
		items, err := manager.ListManagedResources(ctx, kubeprovider.ResourceListOptions{
			Kind:               kind,
			Namespace:          namespace,
			ProjectID:          target.ProjectID,
			ApplicationID:      target.ApplicationID,
			EnvironmentID:      target.EnvironmentID,
			DeploymentTargetID: target.ID,
		})
		if err != nil {
			if isKubernetesNotFound(err) {
				continue
			}
			return fmt.Errorf("list %s resources in %s: %w", kind, namespace, err)
		}
		for _, item := range items {
			if !deleteData && strings.EqualFold(item.Kind, "PersistentVolumeClaim") {
				continue
			}
			if err := manager.DeleteManagedResource(ctx, item.Kind, item.Namespace, item.Name); err != nil && !isKubernetesNotFound(err) {
				return fmt.Errorf("delete %s %s/%s: %w", item.Kind, item.Namespace, item.Name, err)
			}
		}
	}
	return nil
}

func (r *Runner) cleanupGatewayRuntimeResources(ctx context.Context, route model.GatewayRoute) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", route.ProjectID).Error; err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	var target model.DeploymentTarget
	if err := r.db.First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		return fmt.Errorf("deployment target not found: %w", err)
	}
	environment := deploymentTargetEnvironment(target)
	manager, err := r.kubernetesManager(environment)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project, environment)
	items, err := manager.ListManagedResources(ctx, kubeprovider.ResourceListOptions{
		Kind:               "services",
		Namespace:          namespace,
		ProjectID:          route.ProjectID,
		ApplicationID:      route.ApplicationID,
		EnvironmentID:      route.EnvironmentID,
		DeploymentTargetID: route.DeploymentTargetID,
		RouteID:            route.ID,
	})
	if err != nil {
		if isKubernetesNotFound(err) {
			return nil
		}
		return fmt.Errorf("list gateway resources in %s: %w", namespace, err)
	}
	for _, item := range items {
		if !strings.EqualFold(item.Kind, "Ingress") {
			continue
		}
		if err := manager.DeleteManagedResource(ctx, item.Kind, item.Namespace, item.Name); err != nil && !isKubernetesNotFound(err) {
			return fmt.Errorf("delete %s %s/%s: %w", item.Kind, item.Namespace, item.Name, err)
		}
	}
	return nil
}

func (r *Runner) markProjectDeleteFailed(projectID string, err error) error {
	return markCleanupFailed(r.db, &model.Project{}, projectID, err)
}

func (r *Runner) markDeploymentTargetDeleteFailed(targetID string, err error) error {
	return markCleanupFailed(r.db, &model.DeploymentTarget{}, targetID, err)
}

func (r *Runner) markGatewayRouteDeleteFailed(routeID string, err error) error {
	return markCleanupFailed(r.db, &model.GatewayRoute{}, routeID, err)
}

func markCleanupFailed(db *gorm.DB, model any, id string, err error) error {
	finishedAt := time.Now()
	message := ""
	if err != nil {
		message = err.Error()
	}
	return db.Model(model).Where("id = ?", id).Updates(map[string]any{
		"delete_status":      "delete_failed",
		"delete_message":     trimReleaseLogContent(message),
		"delete_finished_at": &finishedAt,
	}).Error
}

func (r *Runner) finishProjectDelete(project model.Project) error {
	finishedAt := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Project{}).Where("id = ?", project.ID).Updates(map[string]any{
			"delete_status":      "deleted",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&project).Error
	})
}

func (r *Runner) finishDeploymentTargetDelete(target model.DeploymentTarget) error {
	finishedAt := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("target_id = ?", target.ID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Updates(map[string]any{
			"delete_status":      "deleted",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&target).Error
	})
}

func (r *Runner) finishGatewayRouteDelete(route model.GatewayRoute) error {
	finishedAt := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.GatewayRoute{}).Where("id = ?", route.ID).Updates(map[string]any{
			"delete_status":      "deleted",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&route).Error
	})
}

func (r *Runner) finishRuntimeConfigSetDelete(set model.ProjectRuntimeConfigSet) error {
	finishedAt := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		var targets []model.DeploymentTarget
		if err := tx.Select("id", "runtime_config_set_ids").Where("project_id = ?", set.ProjectID).Find(&targets).Error; err != nil {
			return err
		}
		for _, target := range targets {
			nextIDs := removeRuntimeConfigSetID(target.RuntimeConfigSetIDs, set.ID)
			if nextIDs != target.RuntimeConfigSetIDs {
				if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Update("runtime_config_set_ids", nextIDs).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&model.ProjectRuntimeConfigSet{}).Where("id = ?", set.ID).Updates(map[string]any{
			"delete_status":      "deleted",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&set).Error
	})
}

func removeRuntimeConfigSetID(raw string, setID string) string {
	setID = strings.TrimSpace(setID)
	if setID == "" {
		return raw
	}
	next := make([]string, 0)
	for _, id := range runtimeConfigSetIDs(raw) {
		if id != setID {
			next = append(next, id)
		}
	}
	if len(next) == 0 {
		return ""
	}
	content, err := json.Marshal(next)
	if err != nil {
		return raw
	}
	return string(content)
}
