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
			return r.reconcileClusterNetworkPolicies(stageCtx, cluster, strings.TrimSpace(payload.ProjectID))
		}); err != nil {
			errs = append(errs, fmt.Errorf("runtime cluster %s: %w", cluster.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) reconcileClusterNetworkPolicies(ctx context.Context, cluster model.RuntimeCluster, onlyProjectID string) error {
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
		if onlyProjectID != "" && projectID != onlyProjectID {
			continue
		}
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

type networkPolicyProbeEnqueuer interface {
	EnqueueNetworkPolicyProbe(ctx context.Context, payload tasks.NetworkPolicyProbePayload) (*asynq.TaskInfo, error)
}

// networkPolicyProbeCooldown 防止连接测试等入口在短时间内反复在集群中创建探测 Pod。
const networkPolicyProbeCooldown = 10 * time.Minute

// handleNetworkPolicyProbe 主动探测集群是否执行 NetworkPolicy，结果作为工作流结果写回集群，只供展示，不拦截部署。
// 未指定集群的周期任务只按集群分别入队，避免一个慢集群拖垮其他集群的探测。
func (r *Runner) handleNetworkPolicyProbe(ctx context.Context, task *asynq.Task) error {
	var payload tasks.NetworkPolicyProbePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("%w: invalid network policy probe payload", asynq.SkipRetry)
	}
	clusterID := strings.TrimSpace(payload.RuntimeClusterID)
	if clusterID == "" {
		return r.enqueueNetworkPolicyProbes(ctx)
	}
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(r.db.Model(&model.RuntimeCluster{})).Where("id = ?", clusterID).Take(&cluster).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if cluster.NetworkPolicyProbedAt != nil && time.Since(*cluster.NetworkPolicyProbedAt) < networkPolicyProbeCooldown {
		return nil
	}
	return workerStage(ctx, "network_policy.probe_cluster", func(stageCtx context.Context) error {
		return r.probeClusterNetworkPolicy(stageCtx, cluster)
	})
}

func (r *Runner) enqueueNetworkPolicyProbes(ctx context.Context) error {
	if r.networkPolicyProbeEnqueuer == nil {
		return errors.New("network policy probe enqueuer is not configured")
	}
	var clusterIDs []string
	if err := runtimecluster.ActiveScope(r.db.Model(&model.RuntimeCluster{})).Order("id asc").Pluck("id", &clusterIDs).Error; err != nil {
		return err
	}
	var errs []error
	for _, clusterID := range clusterIDs {
		if _, err := r.networkPolicyProbeEnqueuer.EnqueueNetworkPolicyProbe(ctx, tasks.NetworkPolicyProbePayload{RuntimeClusterID: clusterID}); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			errs = append(errs, fmt.Errorf("runtime cluster %s: %w", clusterID, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) probeClusterNetworkPolicy(ctx context.Context, cluster model.RuntimeCluster) error {
	result := kubeprovider.NetworkPolicyProbeResult{Enforcement: kubeprovider.NetworkPolicyEnforcementUnknown, Code: "runtime_cluster.network_policy_probe_unavailable"}
	manager, probeErr := r.kubernetesManager(ctx, model.DeploymentTarget{ClusterID: cluster.ID})
	if probeErr == nil {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		var probed kubeprovider.NetworkPolicyProbeResult
		probed, probeErr = manager.ProbeNetworkPolicyEnforcement(probeCtx, r.buildExecutorImage, cluster.ID)
		cancel()
		if probeErr == nil {
			result = probed
		}
	}
	now := time.Now().UTC()
	saveErr := r.db.WithContext(ctx).Model(&model.RuntimeCluster{}).Where("id = ?", cluster.ID).Updates(map[string]any{
		"network_policy_enforcement": result.Enforcement,
		"network_policy_probe_code":  result.Code,
		"network_policy_probed_at":   &now,
	}).Error
	// 结论已落库为 unknown；探测错误继续返回，让 Stage Span 记录失败原因。
	return errors.Join(probeErr, saveErr)
}
