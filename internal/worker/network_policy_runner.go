package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// syncProjectNetworkPolicies 按数据库期望配置与集群实时节点地址覆盖项目 Namespace 的受管网络策略。
func (r *Runner) syncProjectNetworkPolicies(ctx context.Context, manager kubeprovider.NamespaceManager, cluster model.RuntimeCluster, projectID string, namespace string, nodeAddresses []string) error {
	desired, err := runtimecluster.DesiredWorkloadNetworkPolicies(ctx, r.db, cluster, projectID, namespace, nodeAddresses)
	if err != nil {
		return err
	}
	return manager.SyncWorkloadNetworkPolicies(ctx, namespace, desired)
}

// handleNetworkPolicyReconcile 周期或在集群网络配置变更后重新下发全部项目 Namespace 的策略，并改回手动修改。
// 单个集群失败不影响其他集群。
func (r *Runner) handleNetworkPolicyReconcile(ctx context.Context, task *asynq.Task) error {
	var payload tasks.NetworkPolicyReconcilePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: invalid network policy reconcile payload", asynq.SkipRetry)
	}
	query := runtimecluster.ActiveScope(r.db.Model(&model.RuntimeCluster{}))
	if clusterID := strings.TrimSpace(payload.RuntimeClusterID); clusterID != "" {
		query = query.Where("id = ?", clusterID)
	}
	var clusters []model.RuntimeCluster
	if err := query.Order("id asc").Find(&clusters).Error; err != nil {
		return err
	}
	var errs []error
	for _, cluster := range clusters {
		if err := workerStage(ctx, "network_policy.reconcile_cluster", func(stageCtx context.Context) error {
			return r.reconcileClusterNetworkPolicies(stageCtx, cluster)
		}); err != nil {
			errs = append(errs, fmt.Errorf("runtime cluster %s: %w", cluster.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) reconcileClusterNetworkPolicies(ctx context.Context, cluster model.RuntimeCluster) error {
	manager, err := r.kubernetesManager(ctx, model.DeploymentTarget{ClusterID: cluster.ID})
	if err != nil {
		return err
	}
	namespaces, err := manager.ListProjectNamespaces(ctx)
	if err != nil {
		return err
	}
	nodeAddresses, err := manager.ClusterNodeAddresses(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for namespace, projectID := range namespaces {
		var project model.Project
		if err := r.db.WithContext(ctx).Select("id", "kubernetes_namespace", "delete_status").
			Where("id = ? and kubernetes_namespace = ?", projectID, namespace).Take(&project).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// 数据库中已不存在的项目空间由资源清理流程处理，这里不为其渲染策略。
				continue
			}
			errs = append(errs, fmt.Errorf("namespace %s: %w", namespace, err))
			continue
		}
		if !model.DeleteStatusActive(project.DeleteStatus) {
			continue
		}
		if err := r.syncProjectNetworkPolicies(ctx, manager, cluster, project.ID, namespace, nodeAddresses); err != nil {
			errs = append(errs, fmt.Errorf("namespace %s: %w", namespace, err))
		}
	}
	return errors.Join(errs...)
}
