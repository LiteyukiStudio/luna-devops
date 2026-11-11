package runtimecluster

import (
	"context"
	"strings"

	"gorm.io/gorm"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

// NetworkGatewayNamespace 是网关控制器 Pod 所在的 Namespace，未单独配置时与 Gateway 资源同 Namespace。
func NetworkGatewayNamespace(cluster model.RuntimeCluster) string {
	for _, value := range []string{cluster.NetworkGatewayNamespace, cluster.GatewayNamespace} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "kube-system"
}

// NetworkBlockedCIDRs 解析管理员追加的出站拒绝网段（每行一个）。
func NetworkBlockedCIDRs(raw string) []string {
	result := []string{}
	for _, line := range strings.Split(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

// DesiredWorkloadNetworkPolicies 由数据库期望配置与集群实时节点地址渲染项目 Namespace 的受管策略。
// Worker 下发与 API 状态比对共用此函数，保证两边看到的是同一份期望。nodeAddresses 由调用方按集群读取一次。
func DesiredWorkloadNetworkPolicies(ctx context.Context, db *gorm.DB, cluster model.RuntimeCluster, projectID string, namespace string, nodeAddresses []string) ([]networkingv1.NetworkPolicy, error) {
	var trusted []string
	if err := db.WithContext(ctx).Model(&model.SystemComponentInstallation{}).
		Where("runtime_cluster_id = ? and project_id = ? and namespace = ? and deployment_target_id <> ''", cluster.ID, projectID, namespace).
		Pluck("deployment_target_id", &trusted).Error; err != nil {
		return nil, err
	}
	return kubeprovider.RenderWorkloadNetworkPolicies(kubeprovider.WorkloadNetworkPolicyInput{
		Namespace:                  namespace,
		GatewayNamespace:           NetworkGatewayNamespace(cluster),
		BlockedCIDRs:               NetworkBlockedCIDRs(cluster.NetworkBlockedCIDRs),
		NodeAddresses:              nodeAddresses,
		TrustedDeploymentTargetIDs: trusted,
	})
}
