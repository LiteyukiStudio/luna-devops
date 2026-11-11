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
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const resourceCleanupRecoveryAfter = 2 * time.Hour

func (r *Runner) recoverStaleResourceCleanups(ctx context.Context) error {
	if r.taskClient == nil {
		return nil
	}
	applicationPayloads, err := r.staleApplicationDeletePayloads(ctx, time.Now().Add(-resourceCleanupRecoveryAfter))
	if err != nil {
		return err
	}
	for _, payload := range applicationPayloads {
		if _, err := r.taskClient.EnqueueApplicationDelete(ctx, payload); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			return err
		}
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

func (r *Runner) staleApplicationDeletePayloads(ctx context.Context, cutoff time.Time) ([]tasks.ApplicationDeletePayload, error) {
	var applications []model.Application
	if err := r.db.WithContext(ctx).
		Where("delete_status = ? and delete_started_at < ?", "deleting", cutoff).
		Order("delete_started_at asc").
		Limit(50).
		Find(&applications).Error; err != nil {
		return nil, err
	}
	payloads := make([]tasks.ApplicationDeletePayload, 0, len(applications))
	for _, application := range applications {
		payloads = append(payloads, tasks.ApplicationDeletePayload{
			ApplicationID: application.ID,
			ProjectID:     application.ProjectID,
			ActorID:       "system:cleanup-recovery",
		})
	}
	return payloads, nil
}

func (r *Runner) staleResourceCleanupPayloads(ctx context.Context, cutoff time.Time) ([]tasks.ResourceCleanupPayload, error) {
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
	// SkipRetry 表示确定性冲突（例如仍被服务关系引用），立即收敛到 delete_failed，不等待重试耗尽。
	if err != nil && (errors.Is(err, asynq.SkipRetry) || resourceCleanupAttemptExhausted(ctx)) {
		_ = r.markResourceCleanupFailed(ctx, payload, err)
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
	if err := r.cleanupProjectNamespaces(ctx, project, payload.ActorID); err != nil {
		return err
	}
	return r.finishProjectDelete(ctx, project)
}

func (r *Runner) cleanupProjectNamespaces(ctx context.Context, project model.Project, actorID string) error {
	targets, err := r.projectCleanupClusterTargets(ctx, project.ID)
	if err != nil {
		return err
	}
	return r.cleanupProjectNamespacesForDeploymentTargets(ctx, project, targets, actorID)
}

func (r *Runner) cleanupProjectNamespacesForDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget, actorID string) error {
	if len(targets) == 0 {
		return nil
	}
	var routes []model.GatewayRoute
	if err := r.db.WithContext(ctx).Where("project_id = ?", project.ID).Find(&routes).Error; err != nil {
		return err
	}
	namespace := projectNamespace(project)
	for _, target := range targets {
		manager, err := r.kubernetesManager(ctx, target)
		if err != nil {
			return err
		}
		if err := deleteManagedNamespace(ctx, manager, namespace, project.ID); err != nil {
			return err
		}
		// 命名空间删除已确认，项目 HTTPRoute 已不存在；网关证书命名空间里的证书需单独回收。
		if err := r.releaseGatewayCertificates(ctx, manager, target, project, routes, actorID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) projectCleanupClusterTargets(ctx context.Context, projectID string) ([]model.DeploymentTarget, error) {
	targets, err := r.activeRuntimeClusterTargets(ctx)
	if err != nil || len(targets) > 0 {
		return targets, err
	}
	var targetCount int64
	if err := r.db.WithContext(ctx).Unscoped().Model(&model.DeploymentTarget{}).
		Where("project_id = ?", projectID).Count(&targetCount).Error; err != nil {
		return nil, err
	}
	var volumeCount int64
	if err := r.db.WithContext(ctx).Unscoped().Model(&model.ProjectVolume{}).
		Where("project_id = ?", projectID).Count(&volumeCount).Error; err != nil {
		return nil, err
	}
	if targetCount+volumeCount > 0 {
		return nil, fmt.Errorf("project %s has runtime resources but no active runtime cluster is available for cleanup", projectID)
	}
	return nil, nil
}

func (r *Runner) activeRuntimeClusterTargets(ctx context.Context) ([]model.DeploymentTarget, error) {
	// A legacy empty cluster ID meant "the current default", so after the
	// default changes only checking stored target references can miss resources.
	var clusters []model.RuntimeCluster
	if err := runtimecluster.ActiveScope(r.db.WithContext(ctx)).
		Select("id").
		Order("created_at asc").
		Find(&clusters).Error; err != nil {
		return nil, err
	}
	targets := make([]model.DeploymentTarget, 0, len(clusters))
	for _, cluster := range clusters {
		targets = append(targets, model.DeploymentTarget{ClusterID: cluster.ID})
	}
	return targets, nil
}

func deleteManagedNamespace(ctx context.Context, manager kubeprovider.NamespaceManager, namespace, projectID string) error {
	options := kubeprovider.ResourceListOptions{Kind: "namespaces", ProjectID: projectID}
	items, err := manager.ListManagedResources(ctx, options)
	if err != nil {
		if isKubernetesNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect namespace %s ownership: %w", namespace, err)
	}
	owned := false
	for _, item := range items {
		if item.Name == namespace && strings.TrimSpace(item.ProjectID) == strings.TrimSpace(projectID) {
			owned = true
			break
		}
	}
	if !owned {
		return nil
	}
	if err := manager.DeleteManagedResource(ctx, "Namespace", "", namespace); err != nil && !isKubernetesNotFound(err) {
		return err
	}
	items, err = manager.ListManagedResources(ctx, options)
	if err != nil {
		return fmt.Errorf("confirm namespace %s deletion: %w", namespace, err)
	}
	for _, item := range items {
		if item.Name == namespace && strings.TrimSpace(item.ProjectID) == strings.TrimSpace(projectID) {
			return fmt.Errorf("namespace %s deletion is still in progress", namespace)
		}
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
	// 入站服务关系必须在清理访问入口和运行资源之前检查：否则依赖方已失去后端，finalize 才发现冲突为时已晚。
	if err := workerStage(ctx, "deployment_target_delete.check_references", func(stageCtx context.Context) error {
		return ensureDeploymentTargetHasNoIncomingBindings(r.db.WithContext(stageCtx), target)
	}); err != nil {
		return deploymentTargetDeleteFailure(deploymentTargetDeleteCodeCleanupFailed, err)
	}
	if err := workerStage(ctx, "deployment_target_delete.cleanup_runtime", func(stageCtx context.Context) error {
		if err := r.cleanupDeploymentTargetGatewayRoutes(stageCtx, target, payload.ActorID); err != nil {
			return err
		}
		return r.cleanupDeploymentTargetRuntimeResources(stageCtx, target)
	}); err != nil {
		return deploymentTargetDeleteFailure(deploymentTargetDeleteCodeCleanupFailed, err)
	}
	if err := workerStage(ctx, "deployment_target_delete.finalize", func(stageCtx context.Context) error {
		return r.finishDeploymentTargetDelete(stageCtx, target)
	}); err != nil {
		return deploymentTargetDeleteFailure(deploymentTargetDeleteCodeFinalizeFailed, err)
	}
	return nil
}

const (
	// deploymentTargetDeleteCodeServiceBindingInUse 与 API 删除守卫共用稳定码：仍有服务关系依赖该部署配置。
	deploymentTargetDeleteCodeServiceBindingInUse = "service_binding_in_use"
	deploymentTargetDeleteCodeCleanupFailed       = "deployment_target.delete_cleanup_failed"
	deploymentTargetDeleteCodeFinalizeFailed      = "deployment_target.delete_finalize_failed"
)

var errDeploymentTargetReferencedByServiceBinding = errors.New("deployment target is referenced by active service bindings")

// deploymentTargetDeleteError 携带部署配置删除失败阶段对应的稳定码，供终态写入 delete_code。
type deploymentTargetDeleteError struct {
	code string
	err  error
}

func (e *deploymentTargetDeleteError) Error() string { return e.err.Error() }
func (e *deploymentTargetDeleteError) Unwrap() error { return e.err }

// deploymentTargetDeleteFailure 把阶段错误映射为稳定码；入站服务关系冲突是确定性失败，标记 SkipRetry 立即收敛。
func deploymentTargetDeleteFailure(code string, err error) error {
	if errors.Is(err, errDeploymentTargetReferencedByServiceBinding) {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, &deploymentTargetDeleteError{code: deploymentTargetDeleteCodeServiceBindingInUse, err: err})
	}
	return &deploymentTargetDeleteError{code: code, err: err}
}

func deploymentTargetDeleteFailureCode(err error) string {
	var deleteErr *deploymentTargetDeleteError
	if errors.As(err, &deleteErr) {
		return deleteErr.code
	}
	return deploymentTargetDeleteCodeCleanupFailed
}

// ensureDeploymentTargetHasNoIncomingBindings 统计指向该部署配置的启用服务关系；停用关系随部署配置一起删除。
func ensureDeploymentTargetHasNoIncomingBindings(db *gorm.DB, target model.DeploymentTarget) error {
	var incomingActive int64
	if err := db.Model(&model.ServiceBinding{}).
		Where("target_deployment_target_id = ? and enabled = ?", target.ID, true).
		Count(&incomingActive).Error; err != nil {
		return err
	}
	if incomingActive > 0 {
		return fmt.Errorf("%w: %d", errDeploymentTargetReferencedByServiceBinding, incomingActive)
	}
	return nil
}

func (r *Runner) cleanupDeploymentTargetGatewayRoutes(ctx context.Context, target model.DeploymentTarget, actorID string) error {
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
		if err := r.cleanupDeletedGatewayRoute(ctx, route, actorID); err != nil {
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
	if err := r.cleanupDeletedGatewayRoute(ctx, route, payload.ActorID); err != nil {
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
	var cluster model.RuntimeCluster
	if err := r.db.WithContext(ctx).Unscoped().First(&cluster, "id = ?", clusterID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !resourceCleanupCanRun(cluster.DeleteStatus) {
		return nil
	}
	return r.finishRuntimeClusterDelete(ctx, cluster)
}

func (r *Runner) finishRuntimeClusterDelete(ctx context.Context, cluster model.RuntimeCluster) error {
	finishedAt := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		targetCount, volumeCount, err := runtimecluster.ActiveProjectReferenceCounts(tx, cluster.ID)
		if err != nil {
			return err
		}
		if targetCount > 0 || volumeCount > 0 {
			return fmt.Errorf("runtime cluster %s is referenced by %d active deployment targets and %d active project volumes", cluster.ID, targetCount, volumeCount)
		}
		if err := r.secrets.DeleteRefContextWithDB(ctx, tx, cluster.KubeconfigRef, "runtime_cluster:"+cluster.ID+":kubeconfig"); err != nil {
			return err
		}
		if err := tx.Where("resource_type = ? and resource_id = ?", "runtime_cluster", cluster.ID).
			Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		// 出站例外属于集群；授权随外键级联删除。
		if err := tx.Where("runtime_cluster_id = ?", cluster.ID).Delete(&model.NetworkEgressException{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.RuntimeCluster{}).Where("id = ?", cluster.ID).Updates(map[string]any{
			"delete_status": "deleted", "delete_message": "", "delete_finished_at": &finishedAt,
			"kubeconfig_ref": "",
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
	manager, err := r.kubernetesManager(ctx, target)
	if err != nil {
		return err
	}
	namespace := deploymentNamespace(project)
	kinds := []string{"services", "workloads", "configs"}
	for _, kind := range kinds {
		if err := deleteManagedResourcesAndConfirm(ctx, manager, kubeprovider.ResourceListOptions{
			Kind:               kind,
			Namespace:          namespace,
			ProjectID:          target.ProjectID,
			ApplicationID:      target.ApplicationID,
			DeploymentTargetID: target.ID,
		}); err != nil {
			return err
		}
	}
	return r.releaseDeploymentTargetVolumeMountsAfterCleanup(ctx, target, manager, namespace)
}

func deleteManagedResourcesAndConfirm(ctx context.Context, manager kubeprovider.NamespaceManager, options kubeprovider.ResourceListOptions) error {
	items, err := manager.ListManagedResources(ctx, options)
	if err != nil {
		if isKubernetesNotFound(err) {
			return nil
		}
		return fmt.Errorf("list %s resources in %s: %w", options.Kind, options.Namespace, err)
	}
	for _, item := range items {
		if err := manager.DeleteManagedResource(ctx, item.Kind, item.Namespace, item.Name); err != nil && !isKubernetesNotFound(err) {
			return fmt.Errorf("delete %s %s/%s: %w", item.Kind, item.Namespace, item.Name, err)
		}
	}
	remaining, err := manager.ListManagedResources(ctx, options)
	if err != nil {
		if isKubernetesNotFound(err) {
			return nil
		}
		return fmt.Errorf("confirm %s resource deletion in %s: %w", options.Kind, options.Namespace, err)
	}
	if len(remaining) > 0 {
		return fmt.Errorf("%d %s resources in %s are still deleting", len(remaining), options.Kind, options.Namespace)
	}
	return nil
}

// cleanupGatewayRuntimeResources 只删除访问入口的 HTTPRoute；停用入口保留证书，重新启用时无需重新签发。
func (r *Runner) cleanupGatewayRuntimeResources(ctx context.Context, route model.GatewayRoute) error {
	_, err := r.deleteGatewayHTTPRoute(ctx, route)
	return err
}

// cleanupDeletedGatewayRoute 删除访问入口的 HTTPRoute 后回收其网关证书。
func (r *Runner) cleanupDeletedGatewayRoute(ctx context.Context, route model.GatewayRoute, actorID string) error {
	runtime, err := r.deleteGatewayHTTPRoute(ctx, route)
	if err != nil || runtime.manager == nil {
		return err
	}
	return r.releaseGatewayCertificates(ctx, runtime.manager, runtime.target, runtime.project, []model.GatewayRoute{route}, actorID)
}

type gatewayRouteRuntime struct {
	project model.Project
	target  model.DeploymentTarget
	manager kubeprovider.NamespaceManager
}

// deleteGatewayHTTPRoute 删除访问入口的 HTTPRoute；部署配置已不存在时返回空 manager，表示无运行时可清理。
func (r *Runner) deleteGatewayHTTPRoute(ctx context.Context, route model.GatewayRoute) (gatewayRouteRuntime, error) {
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", route.ProjectID).Error; err != nil {
		return gatewayRouteRuntime{}, fmt.Errorf("project not found: %w", err)
	}
	var target model.DeploymentTarget
	if err := r.db.WithContext(ctx).First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return gatewayRouteRuntime{}, nil
		}
		return gatewayRouteRuntime{}, fmt.Errorf("deployment target not found: %w", err)
	}
	manager, err := r.kubernetesManager(ctx, target)
	if err != nil {
		return gatewayRouteRuntime{}, err
	}
	namespace := deploymentNamespace(project)
	if err := manager.DeleteHTTPRoute(ctx, namespace, gatewayRuntimeName(route)); err != nil && !isKubernetesNotFound(err) {
		return gatewayRouteRuntime{}, fmt.Errorf("delete HTTPRoute %s/%s: %w", namespace, gatewayRuntimeName(route), err)
	}
	return gatewayRouteRuntime{project: project, target: target, manager: manager}, nil
}

func (r *Runner) markGatewayRouteDeleteFailed(routeID string, err error) error {
	return markCleanupFailed(r.db, &model.GatewayRoute{}, routeID, err)
}

func resourceCleanupAttemptExhausted(ctx context.Context) bool {
	retry, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxRetryOK := asynq.GetMaxRetry(ctx)
	return !retryOK || !maxRetryOK || retry >= maxRetry
}

func (r *Runner) markResourceCleanupFailed(ctx context.Context, payload tasks.ResourceCleanupPayload, err error) error {
	// Persist the terminal attempt state even when the task context was cancelled.
	// WithoutCancel preserves trace values while the timeout bounds this detached write.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	db := r.db.WithContext(persistCtx)
	switch strings.TrimSpace(payload.ResourceType) {
	case "project":
		return markCleanupFailed(db, &model.Project{}, payload.ResourceID, err)
	case "deployment_target":
		return markDeploymentTargetDeleteFailed(db, payload.ResourceID, deploymentTargetDeleteFailureCode(err), err)
	case "gateway_route":
		return markCleanupFailed(db, &model.GatewayRoute{}, payload.ResourceID, err)
	case "runtime_config":
		return markCleanupFailed(db, &model.ProjectRuntimeConfigSet{}, payload.ResourceID, err)
	case "runtime_cluster":
		return markCleanupFailed(db, &model.RuntimeCluster{}, payload.ResourceID, err)
	default:
		return nil
	}
}

// markDeploymentTargetDeleteFailed 写入部署配置删除终态与稳定码，并让随同进入 deleting 的访问入口一起落 delete_failed，
// 避免过期恢复在部署配置删除失败后仍单独删除这些入口。
func markDeploymentTargetDeleteFailed(db *gorm.DB, targetID, code string, err error) error {
	finishedAt := time.Now()
	message := ""
	if err != nil {
		message = err.Error()
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var target model.DeploymentTarget
		if err := tx.Select("id", "project_id", "application_id").First(&target, "id = ?", targetID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Updates(map[string]any{
			"delete_status":      "delete_failed",
			"delete_code":        code,
			"delete_message":     trimReleaseLogContent(message),
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.GatewayRoute{}).
			Where("project_id = ? and application_id = ? and deployment_target_id = ? and delete_status = ?", target.ProjectID, target.ApplicationID, target.ID, "deleting").
			Updates(map[string]any{
				"delete_status":      "delete_failed",
				"delete_message":     trimReleaseLogContent(message),
				"delete_finished_at": &finishedAt,
			}).Error
	})
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

func (r *Runner) finishProjectDelete(ctx context.Context, project model.Project) error {
	finishedAt := time.Now()
	var sharePeers []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 已删除的项目空间不再持有出站例外授权。
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.NetworkEgressExceptionGrant{}).Error; err != nil {
			return err
		}
		// 已删除的项目空间不再提供或使用跨项目共享；对端策略在提交后立即重新下发。
		peers, err := runtimecluster.SharePeerProjectIDs(tx, "provider_project_id = ? or consumer_project_id = ?", project.ID, project.ID)
		if err != nil {
			return err
		}
		sharePeers = peers
		if err := tx.Where("provider_project_id = ? or consumer_project_id = ?", project.ID, project.ID).Delete(&model.NetworkServiceShare{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.DeploymentVolumeMount{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.ProjectVolume{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.DeploymentTarget{}).Where("project_id = ?", project.ID).Updates(map[string]any{
			"delete_status":      "deleted",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.DeploymentTarget{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Application{}).Where("project_id = ?", project.ID).Updates(map[string]any{
			"delete_status":      "deleted",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.Application{}).Error; err != nil {
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
	if err != nil {
		return err
	}
	r.reconcileSharePeers(ctx, sharePeers, project.ID)
	return nil
}

func (r *Runner) finishDeploymentTargetDelete(ctx context.Context, target model.DeploymentTarget) error {
	finishedAt := time.Now()
	var sharePeers []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureDeploymentTargetHasNoIncomingBindings(tx, target); err != nil {
			return err
		}
		if err := tx.Where("source_deployment_target_id = ? or (target_deployment_target_id = ? and enabled = ?)", target.ID, target.ID, false).
			Delete(&model.ServiceBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("source_deployment_target_id = ? or target_deployment_target_id = ?", target.ID, target.ID).
			Delete(&model.ProjectTopologyEdge{}).Error; err != nil {
			return err
		}
		peers, err := runtimecluster.SharePeerProjectIDs(tx, "deployment_target_id = ?", target.ID)
		if err != nil {
			return err
		}
		sharePeers = peers
		if err := tx.Where("deployment_target_id = ?", target.ID).Delete(&model.NetworkServiceShare{}).Error; err != nil {
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
			"delete_code":        "",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&target).Error
	})
	if err != nil {
		return err
	}
	r.reconcileSharePeers(ctx, sharePeers, "")
	return nil
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
