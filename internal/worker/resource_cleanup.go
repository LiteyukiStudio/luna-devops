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

const resourceCleanupRecoveryAfter = 2 * time.Hour

func (r *Runner) recoverStaleResourceCleanups(ctx context.Context) error {
	if r.taskClient == nil {
		return nil
	}
	payloads, err := r.staleResourceCleanupPayloads(ctx, time.Now().Add(-resourceCleanupRecoveryAfter))
	if err != nil {
		return err
	}
	for _, payload := range payloads {
		if _, err := r.taskClient.EnqueueResourceCleanup(ctx, payload); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			return err
		}
	}
	return nil
}

func (r *Runner) staleResourceCleanupPayloads(ctx context.Context, cutoff time.Time) ([]tasks.ResourceCleanupPayload, error) {
	if r.db == nil {
		return nil, nil
	}
	payloads := make([]tasks.ResourceCleanupPayload, 0)
	var projects []model.Project
	if err := r.db.WithContext(ctx).Where("delete_status = ? and delete_started_at < ?", "deleting", cutoff).Limit(20).Find(&projects).Error; err != nil {
		return nil, err
	}
	for _, project := range projects {
		payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "project", ResourceID: project.ID, ProjectID: project.ID, ActorID: "system:cleanup-recovery"})
	}
	var targets []model.DeploymentTarget
	if err := r.db.WithContext(ctx).Where("delete_status = ? and delete_started_at < ?", "deleting", cutoff).Limit(50).Find(&targets).Error; err != nil {
		return nil, err
	}
	for _, target := range targets {
		payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "deployment_target", ResourceID: target.ID, ProjectID: target.ProjectID, ActorID: "system:cleanup-recovery"})
	}
	var routes []model.GatewayRoute
	if err := r.db.WithContext(ctx).Where("delete_status = ? and delete_started_at < ?", "deleting", cutoff).Limit(50).Find(&routes).Error; err != nil {
		return nil, err
	}
	for _, route := range routes {
		payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "gateway_route", ResourceID: route.ID, ProjectID: route.ProjectID, ActorID: "system:cleanup-recovery"})
	}
	var sets []model.ProjectRuntimeConfigSet
	if err := r.db.WithContext(ctx).Where("delete_status = ? and delete_started_at < ?", "deleting", cutoff).Limit(50).Find(&sets).Error; err != nil {
		return nil, err
	}
	for _, set := range sets {
		payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "runtime_config", ResourceID: set.ID, ProjectID: set.ProjectID, ActorID: "system:cleanup-recovery"})
	}
	var clusters []model.RuntimeCluster
	if err := r.db.WithContext(ctx).Where("delete_status = ? and delete_started_at < ?", "deleting", cutoff).Limit(20).Find(&clusters).Error; err != nil {
		return nil, err
	}
	for _, cluster := range clusters {
		payloads = append(payloads, tasks.ResourceCleanupPayload{ResourceType: "runtime_cluster", ResourceID: cluster.ID, ActorID: "system:cleanup-recovery"})
	}
	return payloads, nil
}

func (r *Runner) handleResourceCleanup(ctx context.Context, task *asynq.Task) error {
	var payload tasks.ResourceCleanupPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	err := r.handleResourceCleanupPayload(ctx, payload)
	if err != nil && resourceCleanupAttemptExhausted(ctx) {
		_ = r.markResourceCleanupFailed(payload, err)
		_ = r.auditResourceCleanupFailure(ctx, payload)
	}
	return err
}

func (r *Runner) auditResourceCleanupFailure(ctx context.Context, payload tasks.ResourceCleanupPayload) error {
	action := resourceCleanupAuditAction(payload.ResourceType)
	if action == "" {
		return nil
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return r.db.WithContext(auditCtx).Create(&model.AuditLog{
		ID:       id.New("aud"),
		UserID:   strings.TrimSpace(payload.ActorID),
		Action:   action,
		Resource: strings.TrimSpace(payload.ResourceID),
		Success:  false,
		Message:  "cleanup_failed",
	}).Error
}

func resourceCleanupAuditAction(resourceType string) string {
	switch strings.TrimSpace(resourceType) {
	case "project":
		return "project.delete"
	case "deployment_target":
		return "deployment.delete"
	case "gateway_route":
		return "gateway.delete"
	case "runtime_config":
		return "runtime_config.delete"
	case "runtime_cluster":
		return "runtime_cluster.delete"
	default:
		return ""
	}
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
	case "runtime_cluster":
		return r.cleanupRuntimeCluster(ctx, payload)
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
		return err
	}
	if err := r.cleanupProjectKubectlNamespaces(ctx, project); err != nil {
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
		return nil
	}
	seen := map[string]bool{}
	for _, target := range targets {
		environment := deploymentTargetEnvironment(target)
		key := projectCleanupEnvironmentKey(environment)
		if seen[key] {
			continue
		}
		seen[key] = true
		manager, err := r.kubernetesManager(ctx, environment)
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
	if err := r.cleanupDeploymentTargetGatewayRoutes(ctx, target); err != nil {
		return err
	}
	if err := r.cleanupDeploymentTargetRuntimeResources(ctx, target); err != nil {
		return err
	}
	return r.finishDeploymentTargetDelete(target)
}

func (r *Runner) cleanupDeploymentTargetGatewayRoutes(ctx context.Context, target model.DeploymentTarget) error {
	var routes []model.GatewayRoute
	if err := r.db.Where("project_id = ? and application_id = ? and deployment_target_id = ?", target.ProjectID, target.ApplicationID, target.ID).Find(&routes).Error; err != nil {
		return err
	}
	startedAt := time.Now()
	for _, route := range routes {
		status := strings.TrimSpace(route.DeleteStatus)
		if status != "deleting" && status != "delete_failed" {
			if err := r.db.Model(&model.GatewayRoute{}).Where("id = ?", route.ID).Updates(map[string]any{
				"delete_status":      "deleting",
				"delete_message":     "",
				"delete_started_at":  &startedAt,
				"delete_finished_at": nil,
			}).Error; err != nil {
				return err
			}
			route.DeleteStatus = "deleting"
		}
		if err := r.cleanupGatewayRuntimeResources(ctx, route); err != nil {
			if resourceCleanupAttemptExhausted(ctx) {
				_ = r.markGatewayRouteDeleteFailed(route.ID, err)
			}
			return err
		}
		if err := r.finishGatewayRouteDelete(route); err != nil {
			return err
		}
	}
	return nil
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

func (r *Runner) cleanupRuntimeCluster(ctx context.Context, payload tasks.ResourceCleanupPayload) error {
	clusterID := strings.TrimSpace(payload.ResourceID)
	if clusterID == "" {
		return nil
	}
	return r.withKubectlGatewayClusterLock(ctx, clusterID, func(lockCtx context.Context) error {
		var cluster model.RuntimeCluster
		if err := r.db.WithContext(lockCtx).Unscoped().First(&cluster, "id = ?", clusterID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if !resourceCleanupCanRun(cluster.DeleteStatus) {
			return nil
		}
		if err := waitForKubectlGatewayDrain(lockCtx, cluster.KubeGatewayDrainUntil); err != nil {
			return err
		}
		if cluster.KubeGatewayCleanupCompletedAt == nil {
			if strings.TrimSpace(cluster.KubeconfigRef) != "" {
				manager, err := r.kubectlGatewayManagerForCluster(lockCtx, cluster)
				if err != nil {
					return err
				}
				projects, err := r.listKubectlGatewayProjects(lockCtx, cluster)
				if err != nil {
					return err
				}
				spec := kubectlGatewayAccessSpec(cluster, projects)
				if err := manager.CleanupManagedResources(lockCtx, spec); err != nil {
					return err
				}
				if err := manager.CleanupGatewayAccess(lockCtx, spec); err != nil {
					return err
				}
			}
			completedAt := time.Now().UTC()
			if err := r.db.WithContext(lockCtx).Model(&model.RuntimeCluster{}).Where("id = ? and delete_status = ?", cluster.ID, "deleting").
				Update("kube_gateway_cleanup_completed_at", &completedAt).Error; err != nil {
				return err
			}
			cluster.KubeGatewayCleanupCompletedAt = &completedAt
		}
		return r.finishRuntimeClusterDelete(lockCtx, cluster)
	})
}

func waitForKubectlGatewayDrain(ctx context.Context, drainUntil *time.Time) error {
	if drainUntil == nil {
		return nil
	}
	delay := time.Until(drainUntil.UTC())
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *Runner) finishRuntimeClusterDelete(ctx context.Context, cluster model.RuntimeCluster) error {
	finishedAt := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.secrets.DeleteRefContextWithDB(ctx, tx, cluster.KubeconfigRef, "runtime_cluster:"+cluster.ID+":kubeconfig"); err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? and resource_id = ?", "runtime_cluster", cluster.ID).
			Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("runtime_cluster_id = ?", cluster.ID).Delete(&model.KubeAccessBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.RuntimeCluster{}).Where("id = ?", cluster.ID).Updates(map[string]any{
			"delete_status": "deleted", "delete_message": "", "delete_finished_at": &finishedAt,
			"kubeconfig_ref": "", "kube_gateway_enabled": false,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&cluster).Error
	})
}

func resourceCleanupCanRun(status string) bool {
	return strings.TrimSpace(status) == "deleting"
}

func (r *Runner) cleanupDeploymentTargetRuntimeResources(ctx context.Context, target model.DeploymentTarget) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", target.ProjectID).Error; err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	environment := deploymentTargetEnvironment(target)
	manager, err := r.kubernetesManager(ctx, environment)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project, environment)
	kinds := []string{"services", "workloads", "configs"}
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
			if err := manager.DeleteManagedResource(ctx, item.Kind, item.Namespace, item.Name); err != nil && !isKubernetesNotFound(err) {
				return fmt.Errorf("delete %s %s/%s: %w", item.Kind, item.Namespace, item.Name, err)
			}
		}
	}
	return r.releaseDeploymentTargetVolumeMountsAfterCleanup(ctx, target, manager, namespace)
}

func (r *Runner) cleanupGatewayRuntimeResources(ctx context.Context, route model.GatewayRoute) error {
	var project model.Project
	if err := r.db.First(&project, "id = ?", route.ProjectID).Error; err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	var target model.DeploymentTarget
	if err := r.db.First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("deployment target not found: %w", err)
	}
	environment := deploymentTargetEnvironment(target)
	manager, err := r.kubernetesManager(ctx, environment)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project, environment)
	if err := manager.DeleteHTTPRoute(ctx, namespace, gatewayRuntimeName(route)); err != nil && !isKubernetesNotFound(err) {
		return fmt.Errorf("delete HTTPRoute %s/%s: %w", namespace, gatewayRuntimeName(route), err)
	}
	return nil
}

func (r *Runner) markGatewayRouteDeleteFailed(routeID string, err error) error {
	return markCleanupFailed(r.db, &model.GatewayRoute{}, routeID, err)
}

func resourceCleanupAttemptExhausted(ctx context.Context) bool {
	retry, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxRetryOK := asynq.GetMaxRetry(ctx)
	return !retryOK || !maxRetryOK || retry >= maxRetry
}

func (r *Runner) markResourceCleanupFailed(payload tasks.ResourceCleanupPayload, err error) error {
	switch strings.TrimSpace(payload.ResourceType) {
	case "project":
		return markCleanupFailed(r.db, &model.Project{}, payload.ResourceID, err)
	case "deployment_target":
		return markCleanupFailed(r.db, &model.DeploymentTarget{}, payload.ResourceID, err)
	case "gateway_route":
		return markCleanupFailed(r.db, &model.GatewayRoute{}, payload.ResourceID, err)
	case "runtime_config":
		return markCleanupFailed(r.db, &model.ProjectRuntimeConfigSet{}, payload.ResourceID, err)
	case "runtime_cluster":
		return markCleanupFailed(r.db, &model.RuntimeCluster{}, payload.ResourceID, err)
	default:
		return nil
	}
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
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.KubeAccessBinding{}).Error; err != nil {
			return err
		}
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
		var incomingActive int64
		if err := tx.Model(&model.ServiceBinding{}).
			Where("target_deployment_target_id = ? and enabled = ?", target.ID, true).
			Count(&incomingActive).Error; err != nil {
			return err
		}
		if incomingActive > 0 {
			return fmt.Errorf("deployment target is referenced by %d active service bindings", incomingActive)
		}
		if err := tx.Where("source_deployment_target_id = ? or (target_deployment_target_id = ? and enabled = ?)", target.ID, target.ID, false).
			Delete(&model.ServiceBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("source_deployment_target_id = ? or target_deployment_target_id = ?", target.ID, target.ID).
			Delete(&model.ProjectTopologyEdge{}).Error; err != nil {
			return err
		}
		if err := tx.Where("target_id = ?", target.ID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.GatewayRoute{}).
			Where("project_id = ? and application_id = ? and deployment_target_id = ?", target.ProjectID, target.ApplicationID, target.ID).
			Updates(map[string]any{
				"delete_status":      "deleted",
				"delete_message":     "",
				"delete_finished_at": &finishedAt,
			}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? and application_id = ? and deployment_target_id = ?", target.ProjectID, target.ApplicationID, target.ID).Delete(&model.GatewayRoute{}).Error; err != nil {
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
		if err := tx.Select("id", "runtime_config_refs").Where("project_id = ?", set.ProjectID).Find(&targets).Error; err != nil {
			return err
		}
		for _, target := range targets {
			nextRefs := removeLiveRuntimeConfigRef(target.RuntimeConfigRefs, set.ID)
			if nextRefs != target.RuntimeConfigRefs {
				if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Updates(map[string]any{
					"runtime_config_refs": nextRefs,
				}).Error; err != nil {
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

func removeLiveRuntimeConfigRef(raw string, setID string) string {
	refs := model.DecodeDeploymentRuntimeConfigRefs(raw)
	if len(refs) == 0 {
		return raw
	}
	next := make([]model.DeploymentRuntimeConfigRef, 0, len(refs))
	for _, ref := range refs {
		if ref.SetID == setID && ref.Mode == model.RuntimeConfigRefModeLive {
			continue
		}
		next = append(next, ref)
	}
	return model.EncodeDeploymentRuntimeConfigRefs(next)
}
