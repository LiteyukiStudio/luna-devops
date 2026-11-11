package runtimecluster

import (
	"context"
	"fmt"
	"strconv"
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

// NetworkGatewayPorts 是网关监听端口（与 Worker 渲染 Gateway 时的默认值一致）。
func NetworkGatewayPorts(cluster model.RuntimeCluster) []int {
	ports := []int{cluster.GatewayHTTPListenerPort, cluster.GatewayHTTPSListenerPort}
	defaults := []int{8080, 8443}
	for index, port := range ports {
		if port <= 0 {
			ports[index] = defaults[index]
		}
	}
	return ports
}

const (
	BuildEgressRestricted = "restricted"
	BuildEgressPermissive = "permissive"
)

// BuildPrivateEgressPorts 解析构建私网镜像站放行端口（逗号或换行分隔），API 已保证格式合法。
func BuildPrivateEgressPorts(raw string) []int {
	ports := []int{}
	for _, item := range strings.FieldsFunc(raw, func(char rune) bool { return char == ',' || char == '\n' }) {
		if port, err := strconv.Atoi(strings.TrimSpace(item)); err == nil {
			ports = append(ports, port)
		}
	}
	return ports
}

// ParseEgressAllowlist 解析项目空间出站白名单：每行 `CIDR [端口,端口]`，端口省略表示全部端口。
// 最多 64 条、每条最多 16 个端口；不能是 /0。
func ParseEgressAllowlist(raw string) ([]kubeprovider.EgressAllowRule, error) {
	rules := []kubeprovider.EgressAllowRule{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) > 2 {
			return nil, fmt.Errorf("egress allowlist line %q must be `CIDR [ports]`", strings.TrimSpace(line))
		}
		prefix, err := kubeprovider.ParseNetworkCIDR(fields[0])
		if err != nil || prefix.Bits() == 0 {
			return nil, fmt.Errorf("egress allowlist CIDR %q is invalid", fields[0])
		}
		rule := kubeprovider.EgressAllowRule{CIDR: prefix.String()}
		if len(fields) == 2 {
			for _, item := range strings.Split(fields[1], ",") {
				port, err := strconv.Atoi(strings.TrimSpace(item))
				if err != nil || port < 1 || port > 65535 {
					return nil, fmt.Errorf("egress allowlist port %q is invalid", item)
				}
				rule.Ports = append(rule.Ports, port)
			}
			if len(rule.Ports) > 16 {
				return nil, fmt.Errorf("at most 16 ports per egress allowlist entry")
			}
		}
		rules = append(rules, rule)
	}
	if len(rules) > 64 {
		return nil, fmt.Errorf("at most 64 egress allowlist entries are allowed")
	}
	return rules, nil
}

// grantedEgressExceptions 读取本集群授权给项目空间的出站例外。
func grantedEgressExceptions(ctx context.Context, db *gorm.DB, clusterID string, projectID string) ([]kubeprovider.EgressException, error) {
	// 显式列名：GORM 会把 CIDR、IDs 这类缩写推导成 c_id_r、i_ds，与真实列名不一致。
	var rows []struct {
		ID                  string `gorm:"column:id"`
		CIDR                string `gorm:"column:cidr"`
		Ports               string `gorm:"column:ports"`
		Protocol            string `gorm:"column:protocol"`
		DeploymentTargetIDs string `gorm:"column:deployment_target_ids"`
		IncludeBuilds       bool   `gorm:"column:include_builds"`
	}
	if err := db.WithContext(ctx).Table("network_egress_exception_grants as g").
		Select("e.id, e.cidr, e.ports, e.protocol, g.deployment_target_ids, g.include_builds").
		Joins("join network_egress_exceptions e on e.id = g.exception_id").
		Where("e.runtime_cluster_id = ? and g.project_id = ?", clusterID, projectID).
		Order("e.id asc").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]kubeprovider.EgressException, 0, len(rows))
	for _, row := range rows {
		result = append(result, kubeprovider.EgressException{
			ID:            row.ID,
			CIDR:          row.CIDR,
			Ports:         BuildPrivateEgressPorts(row.Ports),
			Protocol:      row.Protocol,
			TargetIDs:     SplitIDList(row.DeploymentTargetIDs),
			IncludeBuilds: row.IncludeBuilds,
		})
	}
	return result, nil
}

// SplitIDList 解析逗号分隔的 ID 列表。
func SplitIDList(raw string) []string {
	result := []string{}
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
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
	var project model.Project
	if err := db.WithContext(ctx).Select("id", "network_ingress_mode", "network_egress_mode", "network_egress_allowlist").
		Where("id = ?", projectID).Take(&project).Error; err != nil {
		return nil, err
	}
	allowlist, err := ParseEgressAllowlist(project.NetworkEgressAllowlist)
	if err != nil {
		return nil, err
	}
	var targets []model.DeploymentTarget
	if err := db.WithContext(ctx).Select("id", "internal_only").Where("project_id = ?", projectID).Find(&targets).Error; err != nil {
		return nil, err
	}
	targetIDs := make([]string, 0, len(targets))
	internal := []string{}
	for _, target := range targets {
		targetIDs = append(targetIDs, target.ID)
		if target.InternalOnly {
			internal = append(internal, target.ID)
		}
	}
	var bindings []model.ServiceBinding
	if err := db.WithContext(ctx).Select("source_deployment_target_id", "target_deployment_target_id").
		Where("project_id = ? and enabled = ?", projectID, true).Find(&bindings).Error; err != nil {
		return nil, err
	}
	dependencies := map[string][]string{}
	for _, binding := range bindings {
		dependencies[binding.TargetDeploymentTargetID] = append(dependencies[binding.TargetDeploymentTargetID], binding.SourceDeploymentTargetID)
	}
	exceptions, err := grantedEgressExceptions(ctx, db, cluster.ID, projectID)
	if err != nil {
		return nil, err
	}
	var trusted []string
	if err := db.WithContext(ctx).Model(&model.SystemComponentInstallation{}).
		Where("runtime_cluster_id = ? and project_id = ? and namespace = ? and deployment_target_id <> ''", cluster.ID, projectID, namespace).
		Pluck("deployment_target_id", &trusted).Error; err != nil {
		return nil, err
	}
	return kubeprovider.RenderWorkloadNetworkPolicies(kubeprovider.WorkloadNetworkPolicyInput{
		Namespace:                  namespace,
		GatewayNamespace:           NetworkGatewayNamespace(cluster),
		GatewayPorts:               NetworkGatewayPorts(cluster),
		BlockedCIDRs:               NetworkBlockedCIDRs(cluster.NetworkBlockedCIDRs),
		NodeAddresses:              nodeAddresses,
		TrustedDeploymentTargetIDs: trusted,
		IngressMode:                project.NetworkIngressMode,
		DeploymentTargetIDs:        targetIDs,
		Dependencies:               dependencies,
		InternalTargetIDs:          internal,
		EgressMode:                 project.NetworkEgressMode,
		EgressAllowlist:            allowlist,
		EgressExceptions:           exceptions,
		BuildEgress: kubeprovider.BuildEgressInput{
			Permissive:   strings.TrimSpace(cluster.BuildEgressMode) == BuildEgressPermissive,
			PrivateCIDRs: NetworkBlockedCIDRs(cluster.BuildPrivateEgressCIDRs),
			PrivatePorts: BuildPrivateEgressPorts(cluster.BuildPrivateEgressPorts),
		},
	})
}

// EgressExceptionAuditMarker 返回审计中的出站例外标记，供成员变更与访问入口审计标出防跳板风险；
// 读取失败时标为 unknown，不把失败当作“没有例外”。
func EgressExceptionAuditMarker(db *gorm.DB, projectID string) string {
	var count int64
	if err := db.Table("network_egress_exception_grants").Where("project_id = ?", projectID).Count(&count).Error; err != nil {
		return "egressExceptions=unknown"
	}
	return fmt.Sprintf("egressExceptions=%d", count)
}
