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
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const (
	// applicationDeleteCodeServiceBindingInUse 与 API 删除守卫共用稳定码：仍有其他应用通过服务关系依赖本应用。
	applicationDeleteCodeServiceBindingInUse = "service_binding_in_use"
	applicationDeleteCodeCleanupFailed       = "application.delete_cleanup_failed"
	applicationDeleteCodeFinalizeFailed      = "application.delete_finalize_failed"
)

var errApplicationReferencedByServiceBinding = errors.New("application is referenced by active service bindings")

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
		"delete_code":    "",
	}).Error
	// 入站服务关系必须在删除运行资源之前检查：否则依赖方已失去后端，finalize 才发现冲突为时已晚。
	if err := workerStage(ctx, "application_delete.check_references", func(stageCtx context.Context) error {
		return ensureApplicationHasNoIncomingBindings(r.db.WithContext(stageCtx), app)
	}); err != nil {
		return r.failApplicationDelete(ctx, payload.ApplicationID, applicationDeleteCodeCleanupFailed, err)
	}
	if err := workerStage(ctx, "application_delete.cleanup_runtime", func(stageCtx context.Context) error {
		return r.cleanupApplicationRuntimeResources(stageCtx, payload)
	}); err != nil {
		return r.failApplicationDelete(ctx, payload.ApplicationID, applicationDeleteCodeCleanupFailed, err)
	}
	if err := workerStage(ctx, "application_delete.finalize", func(stageCtx context.Context) error {
		return r.finishApplicationDelete(stageCtx, app)
	}); err != nil {
		return r.failApplicationDelete(ctx, payload.ApplicationID, applicationDeleteCodeFinalizeFailed, err)
	}
	return nil
}

// failApplicationDelete 让删除任务收敛到 delete_failed：入站服务关系冲突立即终止且不重试；
// 其他错误在最后一次尝试时落终态，避免应用永久停留在 deleting 并被过期恢复无限重投。
func (r *Runner) failApplicationDelete(ctx context.Context, applicationID, code string, err error) error {
	terminal := errors.Is(err, errApplicationReferencedByServiceBinding)
	if terminal {
		code = applicationDeleteCodeServiceBindingInUse
	}
	if terminal || resourceCleanupAttemptExhausted(ctx) {
		// Persist the terminal attempt state even when the task context was cancelled.
		// WithoutCancel preserves trace values while the timeout bounds this detached write.
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.markApplicationDeleteFailed(persistCtx, applicationID, code, err)
	}
	if terminal {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}
	return err
}

// ensureApplicationHasNoIncomingBindings 只统计来自其他应用的启用服务关系；同一应用内部部署配置之间的关系随应用一起删除。
func ensureApplicationHasNoIncomingBindings(db *gorm.DB, app model.Application) error {
	var incomingActive int64
	if err := db.Model(&model.ServiceBinding{}).
		Where("target_application_id = ? and source_application_id <> ? and enabled = ?", app.ID, app.ID, true).
		Count(&incomingActive).Error; err != nil {
		return err
	}
	if incomingActive > 0 {
		return fmt.Errorf("%w: %d", errApplicationReferencedByServiceBinding, incomingActive)
	}
	return nil
}

func (r *Runner) cleanupApplicationRuntimeResources(ctx context.Context, payload tasks.ApplicationDeletePayload) error {
	var project model.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", payload.ProjectID).Error; err != nil {
		return fmt.Errorf("project not found: %w", err)
	}
	var targets []model.DeploymentTarget
	if err := r.db.WithContext(ctx).Unscoped().Where("project_id = ? and application_id = ?", payload.ProjectID, payload.ApplicationID).Find(&targets).Error; err != nil {
		return err
	}
	clusterTargets, err := r.activeRuntimeClusterTargets(ctx)
	if err != nil {
		return err
	}
	if len(clusterTargets) == 0 && len(targets) > 0 {
		return fmt.Errorf("application %s has runtime resources but no active runtime cluster is available for cleanup", payload.ApplicationID)
	}
	var routes []model.GatewayRoute
	if len(clusterTargets) > 0 {
		if err := r.db.WithContext(ctx).Where("project_id = ? and application_id = ?", payload.ProjectID, payload.ApplicationID).Find(&routes).Error; err != nil {
			return err
		}
	}
	kinds := []string{"services", "workloads", "configs"}
	namespace := deploymentNamespace(project)
	for _, clusterTarget := range clusterTargets {
		manager, err := r.kubernetesManager(ctx, clusterTarget)
		if err != nil {
			return err
		}
		for _, kind := range kinds {
			if err := deleteManagedResourcesAndConfirm(ctx, manager, kubeprovider.ResourceListOptions{
				Kind:          kind,
				Namespace:     namespace,
				ProjectID:     payload.ProjectID,
				ApplicationID: payload.ApplicationID,
			}); err != nil {
				return err
			}
		}
		// "services" 已删除并确认本应用的 HTTPRoute，随后回收这些入口留在网关证书命名空间的证书。
		if err := r.releaseGatewayCertificates(ctx, manager, clusterTarget, project, routes, payload.ActorID); err != nil {
			return err
		}
	}
	for _, target := range targets {
		manager, err := r.kubernetesManager(ctx, target)
		if err != nil {
			return err
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

func (r *Runner) markApplicationDeleteFailed(ctx context.Context, applicationID, code string, err error) error {
	finishedAt := time.Now()
	message := ""
	if err != nil {
		message = err.Error()
	}
	return r.db.WithContext(ctx).Model(&model.Application{}).Where("id = ?", applicationID).Updates(map[string]any{
		"delete_status":      "delete_failed",
		"delete_code":        code,
		"delete_message":     trimReleaseLogContent(message),
		"delete_finished_at": &finishedAt,
	}).Error
}

func (r *Runner) finishApplicationDelete(ctx context.Context, app model.Application) error {
	finishedAt := time.Now()
	var sharePeers []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureApplicationHasNoIncomingBindings(tx, app); err != nil {
			return err
		}
		if err := tx.Where("source_application_id = ? or target_application_id = ?", app.ID, app.ID).
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
		// 应用删除时其部署配置提供的跨项目共享一并删除，提交后立即重新下发对端策略。
		appTargets := tx.Model(&model.DeploymentTarget{}).Select("id").Where("project_id = ? and application_id = ?", app.ProjectID, app.ID)
		peers, err := runtimecluster.SharePeerProjectIDs(tx, "deployment_target_id in (?)", appTargets)
		if err != nil {
			return err
		}
		sharePeers = peers
		if err := tx.Where("deployment_target_id in (?)", appTargets).Delete(&model.NetworkServiceShare{}).Error; err != nil {
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
			"delete_status":      "deleted",
			"delete_code":        "",
			"delete_message":     "",
			"delete_finished_at": &finishedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Delete(&app).Error
	})
	if err != nil {
		return err
	}
	r.reconcileSharePeers(ctx, sharePeers, "")
	return nil
}
