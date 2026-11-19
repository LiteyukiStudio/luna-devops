package runtimeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"gorm.io/gorm"
)

type aiInspectClusterGovernanceInput struct {
	Cluster          string  `json:"cluster" desc:"运行集群 ID（clu_xxx）或唯一名称"`
	Section          string  `json:"section" enum:"network_policies|egress_exceptions|pending_network_shares|k8s_resources|k8s_resource_detail" desc:"明确单一治理子视图"`
	Exception        *string `json:"exception,omitempty" desc:"section=egress_exceptions 时按例外 ID（nex_xxx）或名称精确过滤"`
	ShareID          *string `json:"shareId,omitempty" desc:"section=pending_network_shares 时按跨项目共享记录 ID 精确过滤"`
	Project          *string `json:"project,omitempty" desc:"按项目 ID 或 identifier 筛选"`
	Application      *string `json:"application,omitempty" desc:"section=k8s_resources 时按应用 ID、identifier 或名称筛选"`
	Target           *string `json:"target,omitempty" desc:"section=k8s_resources 时按部署目标 ID、stage 或名称筛选"`
	ResourceCategory *string `json:"resourceCategory,omitempty" enum:"namespaces|workloads|services|configs|storage" desc:"section=k8s_resources 时必填资源类别"`
	ResourceKind     *string `json:"resourceKind,omitempty" enum:"Namespace|Deployment|StatefulSet|Pod|HorizontalPodAutoscaler|Service|HTTPRoute|Gateway|ConfigMap|Secret|PersistentVolumeClaim" desc:"section=k8s_resource_detail 时必填 K8s 资源类型"`
	Namespace        *string `json:"namespace,omitempty" desc:"K8s 命名空间（k8s_resources 可选过滤；k8s_resource_detail 非 Namespace 资源必填）"`
	ResourceName     *string `json:"resourceName,omitempty" desc:"section=k8s_resource_detail 时必填资源名称"`
	Page             *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码（k8s_resource_detail 事件固定首页）"`
	PageSize         *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type aiEgressExceptionGrantInput struct {
	Project       string   `json:"project" desc:"授权项目 ID 或 identifier"`
	Targets       []string `json:"targets,omitempty" desc:"限定生效的部署目标列表（支持 dplt_xxx 或 <appIdentifier>:<stage>；更新既有项目授权时省略将严格保持原范围不变）"`
	AllTargets    *bool    `json:"allTargets,omitempty" desc:"显式放宽为整项目全部部署目标生效（与 targets 互斥）"`
	IncludeBuilds *bool    `json:"includeBuilds,omitempty" desc:"是否同时允许该项目构建容器使用此出站例外"`
}

type aiSaveClusterEgressExceptionInput struct {
	Cluster        string                        `json:"cluster" desc:"运行集群 ID（clu_xxx）或唯一名称"`
	Exception      *string                       `json:"exception,omitempty" desc:"出站例外 ID（nex_xxx）或同集群唯一名称；省略为新建，传入为增量更新"`
	Name           *string                       `json:"name,omitempty" desc:"例外名称（新建必填，1..64 字符）"`
	CIDR           *string                       `json:"cidr,omitempty" desc:"目标 IPv4/IPv6 CIDR（新建必填，禁止 /0 与云元数据地址）"`
	Ports          *string                       `json:"ports,omitempty" desc:"逗号分隔端口列表，如 443,5432（省略保持原值）"`
	ClearPorts     *bool                         `json:"clearPorts,omitempty" desc:"显式清空端口限制以放行该协议全部端口"`
	Protocol       *string                       `json:"protocol,omitempty" enum:"tcp|udp|any" default:"tcp" desc:"协议类型（新建默认 tcp）"`
	Reason         *string                       `json:"reason,omitempty" desc:"审计原因说明（新建必填，1..500 字符）"`
	UpsertGrants   []aiEgressExceptionGrantInput `json:"upsertGrants,omitempty" desc:"按项目增量新增或更新例外授权"`
	RemoveProjects []string                      `json:"removeProjects,omitempty" desc:"撤销指定项目的该条出站例外授权"`
}

type aiDeleteClusterEgressExceptionInput struct {
	Cluster   string `json:"cluster" desc:"运行集群 ID（clu_xxx）或唯一名称"`
	Exception string `json:"exception" desc:"出站例外 ID（nex_xxx）或同集群唯一名称"`
}

type aiApproveClusterNetworkShareInput struct {
	Cluster string `json:"cluster" desc:"运行集群 ID（clu_xxx）或唯一名称"`
	ShareID string `json:"shareId" desc:"待审批跨项目网络共享记录 ID"`
}

type aiDeleteClusterK8sResourceInput struct {
	Cluster      string  `json:"cluster" desc:"运行集群 ID（clu_xxx）或唯一名称"`
	ResourceKind string  `json:"resourceKind" enum:"Namespace|Deployment|StatefulSet|Pod|HorizontalPodAutoscaler|Service|HTTPRoute|Gateway|ConfigMap|Secret|PersistentVolumeClaim" desc:"受管 K8s 资源类型（11 类白名单）"`
	Namespace    *string `json:"namespace,omitempty" desc:"K8s 命名空间（非 Namespace 资源必填）"`
	ResourceName string  `json:"resourceName" desc:"K8s 资源名称"`
}

type aiListSystemComponentsInput struct {
	Cluster     *string `json:"cluster,omitempty" desc:"按运行集群 ID（clu_xxx）或名称过滤"`
	ComponentID *string `json:"componentId,omitempty" desc:"按系统组件 ID 过滤（如 luna-gateway-traffic-probe）"`
}

type aiInstallSystemComponentInput struct {
	Template          string  `json:"template" desc:"系统组件模板 ID 或 slug（如 luna-gateway-traffic-probe）"`
	Cluster           string  `json:"cluster" desc:"目标运行集群 ID（clu_xxx）或唯一名称"`
	APIBaseURL        string  `json:"apiBaseUrl" format:"uri" desc:"探针回报数据的平台 API 根地址"`
	Mode              *string `json:"mode,omitempty" enum:"traefik-metrics" default:"traefik-metrics" desc:"探针采集模式（默认 traefik-metrics）"`
	TraefikMetricsURL *string `json:"traefikMetricsUrl,omitempty" format:"uri" desc:"集群内 Traefik Metrics 地址"`
	Image             *string `json:"image,omitempty" desc:"自定义探针镜像覆盖"`
	ProvisionAccess   *bool   `json:"provisionAccess,omitempty" default:"true" desc:"是否自动配置专用 ServiceAccount 与 RBAC（默认 true）"`
}

func (h *Handler) registerClusterGovernanceAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[aiInspectClusterGovernanceInput, map[string]any]{
		Name:           "runtime_inspect_cluster_governance",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster_governance",
		Summary:        "查看运行集群的 NetworkPolicy 对账状态、出站例外清单、待审批跨项目共享或受管 K8s 资源/脱敏 YAML/事件",
		Purpose:        aitool.Text("按单一子视图（network_policies / egress_exceptions / pending_network_shares / k8s_resources / k8s_resource_detail）查看运行集群治理状态或项目受管 K8s 资源详情。", "Inspect cluster governance state or managed Kubernetes resources for a single selected section."),
		AliasesZH:      []string{"集群网络策略状态", "出站例外列表", "集群受管资源", "查看 K8s YAML"},
		AliasesEN:      []string{"inspect cluster governance", "cluster network policies", "cluster k8s resources"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionClusterRead)},
		Execute:        h.execInspectClusterGovernanceAI,
	})

	aitool.Register(reg, aitool.Spec[aiSaveClusterEgressExceptionInput, map[string]any]{
		Name:           "runtime_save_cluster_egress_exception",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster_governance",
		Summary:        "创建或增量更新集群受限网段出站例外及项目授权",
		Purpose:        aitool.Text("在可维护的运行集群上创建或局部增量修改受限网段出站例外与项目级授权范围，并触发 NetworkPolicy 对账。", "Create or incrementally update a cluster egress exception and project grants."),
		AliasesZH:      []string{"配置集群出站例外", "放行私网出站网段", "保存出站例外"},
		AliasesEN:      []string{"save cluster egress exception", "allow private egress cidr"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionClusterManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_inspect_cluster_governance",
				Family:          "resource",
				Expect:          "present",
				IDFrom:          "result.id",
				ReadIDArgument:  "exception",
				Collection:      true,
				StaticArguments: map[string]any{"section": "egress_exceptions"},
			},
		},
		Preview: h.previewSaveClusterEgressExceptionAI,
		Execute: h.execSaveClusterEgressExceptionAI,
	})

	aitool.Register(reg, aitool.Spec[aiDeleteClusterEgressExceptionInput, map[string]any]{
		Name:           "runtime_delete_cluster_egress_exception",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster_governance",
		Summary:        "删除集群出站例外及其全部项目授权并触发 NetworkPolicy 对账",
		Purpose:        aitool.Text("删除运行集群上的出站例外规则及关联项目授权，并立即触发受影响项目空间 NetworkPolicy 对账。", "Delete a cluster egress exception and trigger NetworkPolicy reconciliation."),
		AliasesZH:      []string{"删除集群出站例外", "撤销出站放行规则"},
		AliasesEN:      []string{"delete cluster egress exception", "remove egress exception"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionClusterManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_inspect_cluster_governance",
				Family:          "resource",
				Expect:          "absent",
				IDFrom:          "result.id",
				ReadIDArgument:  "exception",
				Collection:      true,
				StaticArguments: map[string]any{"section": "egress_exceptions"},
			},
		},
		Preview: h.previewDeleteClusterEgressExceptionAI,
		Execute: h.execDeleteClusterEgressExceptionAI,
	})

	aitool.Register(reg, aitool.Spec[aiApproveClusterNetworkShareInput, map[string]any]{
		Name:           "runtime_approve_cluster_network_share",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster_governance",
		Summary:        "作为运行集群维护者批准持有出站例外的项目对外提供的跨项目内网服务共享",
		Purpose:        aitool.Text("由集群维护者审查并批准持有私网出站例外的提供方项目向消费方项目开放的内网服务共享。", "Approve a pending cross-project network service share on a managed runtime cluster."),
		AliasesZH:      []string{"批准跨项目网络共享", "集群审批服务共享"},
		AliasesEN:      []string{"approve cluster network share", "approve service share"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionClusterManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_inspect_cluster_governance",
				Family:          "resource",
				Expect:          "absent",
				IDFrom:          "result.id",
				ReadIDArgument:  "shareId",
				Collection:      true,
				StaticArguments: map[string]any{"section": "pending_network_shares"},
			},
		},
		Preview: h.previewApproveClusterNetworkShareAI,
		Execute: h.execApproveClusterNetworkShareAI,
	})

	aitool.Register(reg, aitool.Spec[aiDeleteClusterK8sResourceInput, map[string]any]{
		Name:           "runtime_delete_cluster_k8s_resource",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster_governance",
		Summary:        "删除运行集群中受控 11 类白名单内的单个受管 K8s 资源",
		Purpose:        aitool.Text("清理运行集群中归属平台管理（含平台受管标签）且在 11 类白名单内的单个孤儿或异常 Kubernetes 资源。", "Delete a single Luna-managed Kubernetes resource within the 11-kind allowlist."),
		AliasesZH:      []string{"删除集群 K8s 资源", "清理异常 Pod", "删除受管资源"},
		AliasesEN:      []string{"delete cluster k8s resource", "remove managed kubernetes resource"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionClusterManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_inspect_cluster_governance",
				Family:          "resource",
				Expect:          "absent",
				IDFrom:          "argument.resourceName",
				ReadIDArgument:  "resourceName",
				StaticArguments: map[string]any{"section": "k8s_resource_detail"},
			},
		},
		Preview: h.previewDeleteClusterK8sResourceAI,
		Execute: h.execDeleteClusterK8sResourceAI,
	})
}

func (h *Handler) registerSystemComponentAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[aiListSystemComponentsInput, map[string]any]{
		Name:              "admin_list_system_components",
		Domain:            aitool.DomainAdmin,
		Category:          "system_component",
		Summary:           "查看各运行集群上的平台系统组件安装状态、Pod 实时健康度及最近 10 分钟流量计费上报鲜活度",
		Purpose:           aitool.Text("聚合查看全平台各集群上的系统组件（如网关流量探针）安装状态、实时运行健康度及流量计量上报鲜活度。", "Inspect platform system component installations and gateway traffic probe reporting freshness across clusters."),
		AliasesZH:         []string{"查看系统组件", "网关流量探针状态", "系统探针列表"},
		AliasesEN:         []string{"list system components", "gateway traffic probe status"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL0,
		Effect:            aitool.EffectRead,
		RequiredScopes:    []string{string(authz.ActionClusterRead)},
		Execute:           h.execListSystemComponentsAI,
	})

	aitool.Register(reg, aitool.Spec[aiInstallSystemComponentInput, map[string]any]{
		Name:              "admin_install_system_component",
		Domain:            aitool.DomainAdmin,
		Category:          "system_component",
		Summary:           "向指定运行集群安装或更新平台系统组件（异步触发系统项目 Release）",
		Purpose:           aitool.Text("在指定运行集群上安装或升级平台系统组件（如 luna-gateway-traffic-probe），自动维护系统项目应用并异步触发 Release 部署。", "Install or update a platform system component on a target runtime cluster."),
		AliasesZH:         []string{"安装系统组件", "部署网关流量探针", "升级系统探针"},
		AliasesEN:         []string{"install system component", "deploy gateway traffic probe"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL2,
		Effect:            aitool.EffectAsync,
		RequiredScopes:    []string{string(authz.ActionClusterManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID: "delivery_get_release",
				Family:      "release",
			},
		},
		Preview: h.previewInstallSystemComponentAI,
		Execute: h.execInstallSystemComponentAI,
	})
}

func (h *Handler) observeClusterNetworkPoliciesFiltered(ctx context.Context, cluster model.RuntimeCluster, filterProjectID string) runtimeClusterNetworkPolicyResponse {
	if strings.TrimSpace(cluster.KubeconfigRef) == "" {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.kubeconfig_not_configured")
	}
	kubeconfig := strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if kubeconfig == "" {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.kubeconfig_unavailable")
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.invalid_kubeconfig")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if strings.TrimSpace(filterProjectID) == "" {
		return observeWorkloadNetworkPolicies(probeCtx, h.dbWithContext(probeCtx), cluster, client)
	}
	return observeWorkloadNetworkPoliciesForProject(probeCtx, h.dbWithContext(probeCtx), cluster, client, strings.TrimSpace(filterProjectID))
}

func observeWorkloadNetworkPoliciesForProject(
	ctx context.Context,
	db *gorm.DB,
	cluster model.RuntimeCluster,
	observer workloadNetworkPolicyObserver,
	filterProjectID string,
) runtimeClusterNetworkPolicyResponse {
	namespaces, err := observer.ListProjectNamespaces(ctx)
	if err != nil {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, networkPolicyObservationCode(err))
	}
	nodes, err := observer.ClusterNodeAddresses(ctx)
	if err != nil {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, networkPolicyObservationCode(err))
	}
	response := runtimeClusterNetworkPolicyResponse{ClusterID: cluster.ID, Status: observation.StatusReady, ObservedAt: time.Now().UTC()}
	for namespace, projectID := range namespaces {
		if strings.TrimSpace(projectID) != filterProjectID {
			continue
		}
		var project model.Project
		if err := db.WithContext(ctx).Select("id", "delete_status").
			Where("id = ? and kubernetes_namespace = ?", projectID, namespace).Take(&project).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.network_policy_unavailable")
		}
		if !model.DeleteStatusActive(project.DeleteStatus) {
			continue
		}
		desired, err := runtimecluster.DesiredWorkloadNetworkPolicies(ctx, db, cluster, project.ID, namespace, nodes)
		if err != nil {
			return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.network_policy_unavailable")
		}
		state, err := observer.WorkloadNetworkPolicyState(ctx, namespace, desired)
		if err != nil {
			return unavailableRuntimeClusterNetworkPolicy(cluster.ID, networkPolicyObservationCode(err))
		}
		response.NamespaceCount++
		if state == kubeprovider.WorkloadNetworkPolicySynced {
			response.SyncedCount++
		} else {
			response.DriftedCount++
		}
	}
	if response.DriftedCount > 0 {
		response.Status = observation.StatusDegraded
		response.ObservationCode = "runtime_cluster.network_policy_drifted"
	}
	return response
}

func (h *Handler) execInspectClusterGovernanceAI(tc *aitool.ToolContext, in aiInspectClusterGovernanceInput) (map[string]any, error) {
	tc.SetNoStore()
	user := tc.User
	db := h.dbWithContext(tc.Context())

	effectiveProjectRef := ""
	if in.Project != nil && strings.TrimSpace(*in.Project) != "" {
		effectiveProjectRef = strings.TrimSpace(*in.Project)
	} else if strings.TrimSpace(tc.ConversationProjectID) != "" {
		effectiveProjectRef = strings.TrimSpace(tc.ConversationProjectID)
	}
	effectiveProjectID := ""
	if effectiveProjectRef != "" {
		_, proj, err := h.authorizeAIProject(tc, effectiveProjectRef, authz.ActionClusterRead)
		if err != nil {
			return nil, err
		}
		effectiveProjectID = proj.ID
	}

	cluster, err := h.resolveClusterForAI(tc, in.Cluster, effectiveProjectID, false)
	if err != nil {
		return nil, err
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	section := strings.TrimSpace(in.Section)

	attachMeta := func(out map[string]any) map[string]any {
		out["clusterId"] = cluster.ID
		out["section"] = section
		if effectiveProjectID != "" {
			out["projectId"] = effectiveProjectID
		}
		return out
	}

	switch section {
	case "network_policies":
		if err := h.requireClusterManageForAI(tc, cluster, "无权查看该运行集群的网络隔离状态"); err != nil {
			return nil, err
		}
		np := h.observeClusterNetworkPoliciesFiltered(tc.Context(), cluster, effectiveProjectID)
		np.Enforcement = cluster.NetworkPolicyEnforcement
		np.EnforcementCode = cluster.NetworkPolicyProbeCode
		np.ProbedAt = cluster.NetworkPolicyProbedAt
		out := map[string]any{
			"status":         np.Status,
			"observedAt":     np.ObservedAt,
			"namespaceCount": np.NamespaceCount,
			"syncedCount":    np.SyncedCount,
			"driftedCount":   np.DriftedCount,
			"enforcement":    np.Enforcement,
		}
		if np.ObservationCode != "" {
			out["observationCode"] = np.ObservationCode
		}
		if np.EnforcementCode != "" {
			out["enforcementCode"] = np.EnforcementCode
		}
		if np.ProbedAt != nil {
			out["probedAt"] = np.ProbedAt
		}
		return attachMeta(out), nil

	case "egress_exceptions":
		if err := h.requireClusterManageForAI(tc, cluster, "无权维护该运行集群的出站例外"); err != nil {
			return nil, err
		}
		query := db.Model(&model.NetworkEgressException{}).Where("runtime_cluster_id = ?", cluster.ID)
		if effectiveProjectID != "" {
			query = query.Where(
				"EXISTS (SELECT 1 FROM network_egress_exception_grants AS g WHERE g.exception_id = network_egress_exceptions.id AND g.project_id = ?)",
				effectiveProjectID,
			)
		}
		if in.Exception != nil && strings.TrimSpace(*in.Exception) != "" {
			ref := strings.TrimSpace(*in.Exception)
			query = query.Where("(id = ? OR LOWER(name) = LOWER(?))", ref, ref)
		}
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var exceptions []model.NetworkEgressException
		if err := query.Preload("Grants").Order("created_at desc, id asc").
			Offset((page - 1) * pageSize).Limit(pageSize).Find(&exceptions).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		sub, _ := newSubGin(tc)
		items, fmtErr := h.egressExceptionResponses(sub, cluster, exceptions)
		if fmtErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", fmtErr.Error())
		}
		if strings.TrimSpace(tc.ConversationProjectID) != "" {
			convID := strings.TrimSpace(tc.ConversationProjectID)
			for i := range items {
				var scopedGrants []egressExceptionGrantResponse
				for _, g := range items[i].Grants {
					if g.ProjectID == convID {
						scopedGrants = append(scopedGrants, g)
					}
				}
				items[i].Grants = scopedGrants
			}
		}
		return attachMeta(aitool.PageEnvelope(items, page, pageSize, "createdAt", "desc", total)), nil

	case "pending_network_shares":
		if err := h.requireClusterManageForAI(tc, cluster, "无权维护该运行集群的网络共享审批"); err != nil {
			return nil, err
		}
		query, qErr := runtimecluster.ServiceSharesOnCluster(tc.Context(), db, cluster)
		if qErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", qErr.Error())
		}
		query = runtimecluster.SharesNeedingApproval(query, cluster.ID)
		if effectiveProjectID != "" {
			query = query.Where("(s.provider_project_id = ? OR s.consumer_project_id = ?)", effectiveProjectID, effectiveProjectID)
		}
		if in.ShareID != nil && strings.TrimSpace(*in.ShareID) != "" {
			query = query.Where("s.id = ?", strings.TrimSpace(*in.ShareID))
		}
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		var shares []model.NetworkServiceShare
		if err := query.Select("s.*").Order("s.created_at desc, s.id asc").
			Offset((page - 1) * pageSize).Limit(pageSize).Find(&shares).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		items, respErr := projectapi.NetworkServiceShareResponses(db, shares)
		if respErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", respErr.Error())
		}
		return attachMeta(aitool.PageEnvelope(items, page, pageSize, "createdAt", "desc", total)), nil

	case "k8s_resources":
		if !h.canUseScopedResourceByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, tc.Context()) {
			return nil, aitool.Forbidden("runtime_cluster.forbidden", "无权查看该集群资源")
		}
		if in.ResourceCategory == nil || !validRuntimeResourceCategory(strings.TrimSpace(*in.ResourceCategory)) {
			return nil, aitool.BadRequest("cluster.resource_category_invalid", "resourceCategory is required when section=k8s_resources (namespaces|workloads|services|configs|storage)")
		}
		category := strings.TrimSpace(*in.ResourceCategory)
		var appID, targetID string
		if in.Application != nil && strings.TrimSpace(*in.Application) != "" {
			if effectiveProjectID == "" {
				return nil, aitool.BadRequest("project.required", "project is required when filtering k8s_resources by application")
			}
			app, appErr := aitool.ResolveApplication(db, effectiveProjectID, *in.Application)
			if appErr != nil {
				return nil, appErr
			}
			appID = app.ID
		}
		if in.Target != nil && strings.TrimSpace(*in.Target) != "" {
			if appID == "" {
				return nil, aitool.BadRequest("application.required", "application is required when filtering k8s_resources by target")
			}
			tgt, tgtErr := aitool.ResolveDeploymentTarget(db, effectiveProjectID, appID, *in.Target)
			if tgtErr != nil {
				return nil, tgtErr
			}
			targetID = tgt.ID
		}
		client, clientErr := h.kubeClientForClusterAI(tc.Context(), cluster)
		if clientErr != nil {
			return nil, clientErr
		}
		namespace := ""
		if in.Namespace != nil {
			namespace = strings.TrimSpace(*in.Namespace)
		}
		reqCtx, cancel := context.WithTimeout(tc.Context(), 10*time.Second)
		defer cancel()
		snapshots, listErr := client.ListManagedResources(reqCtx, kubeprovider.ResourceListOptions{
			Kind:               category,
			Namespace:          namespace,
			ProjectID:          effectiveProjectID,
			ApplicationID:      appID,
			DeploymentTargetID: targetID,
		})
		if listErr != nil {
			return nil, aitool.NewError(http.StatusBadGateway, "cluster.upstream_unavailable", "failed to list managed cluster resources")
		}
		visibility := projectservice.ListVisibilityRelated
		if effectiveProjectID == "" && strings.TrimSpace(tc.ConversationProjectID) == "" && authz.IsPlatformAdmin(user.Role) {
			visibility = projectservice.ListVisibilityAll
		}
		sub, _ := newSubGin(tc)
		snapshots = h.filterClusterResourceSnapshots(sub, user, snapshots, visibility, effectiveProjectID)
		responses, respErr := h.clusterResourceResponses(snapshots, tc.Context())
		if respErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", respErr.Error())
		}
		if isWorkloadResourceKind(category) {
			responses = groupWorkloadPodResponses(responses)
		}
		pagination := paginationParams{Page: page, PageSize: pageSize, SortBy: "updatedAt", SortOrder: "desc"}
		sortClusterResourceResponses(responses, pagination)
		paged := paginateSlice(responses, pagination)
		out := aitool.PageEnvelope(paged, page, pageSize, "updatedAt", "desc", int64(len(responses)))
		out["resourceCategory"] = category
		return attachMeta(out), nil

	case "k8s_resource_detail":
		if !h.canUseScopedResourceByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, tc.Context()) {
			return nil, aitool.Forbidden("runtime_cluster.forbidden", "无权查看该集群资源")
		}
		if in.ResourceKind == nil || !validRuntimeResourceKind(strings.TrimSpace(*in.ResourceKind)) {
			return nil, aitool.BadRequest("cluster.resource_kind_invalid", "resourceKind is required when section=k8s_resource_detail")
		}
		kind := strings.TrimSpace(*in.ResourceKind)
		name := ""
		if in.ResourceName != nil {
			name = strings.TrimSpace(*in.ResourceName)
		}
		if name == "" {
			return nil, aitool.BadRequest("cluster.resource_name_required", "resourceName is required when section=k8s_resource_detail")
		}
		namespace := ""
		if in.Namespace != nil {
			namespace = strings.TrimSpace(*in.Namespace)
		}
		if kind != "Namespace" && namespace == "" {
			return nil, aitool.BadRequest("cluster.resource_namespace_required", "namespace is required for namespaced Kubernetes resources")
		}
		client, clientErr := h.kubeClientForClusterAI(tc.Context(), cluster)
		if clientErr != nil {
			return nil, clientErr
		}
		reqCtx, cancel := context.WithTimeout(tc.Context(), 10*time.Second)
		defer cancel()
		yamlContent, snapshot, yErr := client.GetManagedResourceYAML(reqCtx, kind, namespace, name)
		if yErr != nil {
			return nil, classifyClusterK8sResourceError(yErr)
		}
		enriched, enrichErr := h.clusterResourceResponses([]kubeprovider.ResourceSnapshot{snapshot}, tc.Context())
		if enrichErr != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", enrichErr.Error())
		}
		resourceResp := enriched[0]
		snapshot.ProjectID = resourceResp.ProjectID
		snapshot.ApplicationID = resourceResp.ApplicationID
		snapshot.DeploymentTargetID = resourceResp.DeploymentTargetID

		if convProjID := strings.TrimSpace(tc.ConversationProjectID); convProjID != "" && strings.TrimSpace(resourceResp.ProjectID) != convProjID {
			return nil, projectScopeMismatchError("managed Kubernetes resource does not belong to the bound conversation project")
		}
		if effectiveProjectID != "" && strings.TrimSpace(resourceResp.ProjectID) != effectiveProjectID {
			return nil, aitool.NotFound("cluster.resource_not_found", "managed Kubernetes resource was not found in the requested project")
		}
		sub, rec := newSubGin(tc)
		if !h.canInspectClusterResourceSnapshot(sub, user, snapshot) {
			return nil, abortedToolError(rec, "cluster.resource_forbidden", "not authorized to inspect this cluster resource")
		}

		truncatedYAML, truncated, truncatedBytes := truncateUTF8Bytes(yamlContent, 65536)
		events := []kubeprovider.ResourceEventSnapshot{}
		if evPage, _, evErr := client.ListManagedResourceEventsPage(reqCtx, kind, namespace, name, int64(pageSize)); evErr == nil && evPage.Items != nil {
			events = evPage.Items
			sort.Slice(events, func(i, j int) bool {
				return events[i].LastSeen.After(events[j].LastSeen)
			})
		}

		out := map[string]any{
			"id":           resourceResp.Name,
			"resourceName": resourceResp.Name,
			"resourceKind": resourceResp.Kind,
			"namespace":    resourceResp.Namespace,
			"resource":     resourceResp,
			"yaml":         truncatedYAML,
			"truncated":    truncated,
			"events":       events,
		}
		if truncated {
			out["truncatedBytes"] = truncatedBytes
		}
		if strings.TrimSpace(resourceResp.ProjectID) != "" && effectiveProjectID == "" {
			out["projectId"] = strings.TrimSpace(resourceResp.ProjectID)
		}
		return attachMeta(out), nil

	default:
		return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("unsupported governance section %q", section))
	}
}

func (h *Handler) kubeClientForClusterAI(ctx context.Context, cluster model.RuntimeCluster) (*kubeprovider.Client, error) {
	if strings.TrimSpace(cluster.KubeconfigRef) == "" {
		return nil, aitool.BadRequest("runtime_cluster.kubeconfig_not_configured", "runtime cluster lacks a configured kubeconfig")
	}
	kubeconfig := strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if kubeconfig == "" {
		return nil, aitool.BadRequest("runtime_cluster.kubeconfig_unavailable", "runtime cluster kubeconfig is unavailable")
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, aitool.BadRequest("runtime_cluster.invalid_kubeconfig", "runtime cluster kubeconfig is invalid")
	}
	return client, nil
}

func resolveEgressExceptionForAI(db *gorm.DB, clusterID, ref string) (model.NetworkEgressException, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.NetworkEgressException{}, aitool.BadRequest("argument_invalid", "exception is required")
	}
	var items []model.NetworkEgressException
	if err := db.Preload("Grants").Where("runtime_cluster_id = ?", clusterID).Order("created_at asc").Find(&items).Error; err != nil {
		return model.NetworkEgressException{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		items,
		"egress_exception",
		"runtime_cluster.egress_exception_not_found",
		ref,
		func(item model.NetworkEgressException) string { return item.ID },
		func(item model.NetworkEgressException) string { return item.Name },
	)
}

func resolveEgressGrantTargetID(db *gorm.DB, projectID, rawRef string) (string, error) {
	trimmed := strings.TrimSpace(rawRef)
	if trimmed == "" {
		return "", aitool.BadRequest("runtime_cluster.egress_exception_grant_invalid", "target reference must not be empty")
	}
	if appPart, stagePart, hasColon := strings.Cut(trimmed, ":"); hasColon {
		app, appErr := aitool.ResolveApplication(db, projectID, appPart)
		if appErr != nil {
			return "", appErr
		}
		tgt, tgtErr := aitool.ResolveDeploymentTarget(db, projectID, app.ID, stagePart)
		if tgtErr != nil {
			return "", tgtErr
		}
		return tgt.ID, nil
	}
	var byID model.DeploymentTarget
	if err := db.Where("project_id = ? AND id = ? AND deleted_at IS NULL", projectID, trimmed).First(&byID).Error; err == nil {
		return byID.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return "", aitool.BadRequest("runtime_cluster.egress_exception_grant_invalid",
		fmt.Sprintf("deployment target %q was not found in project %s (use dplt_xxx or <appIdentifier>:<stage>)", trimmed, projectID))
}

type preparedEgressExceptionSave struct {
	isUpdate bool
	cluster  model.RuntimeCluster
	existing model.NetworkEgressException
	next     model.NetworkEgressException
}

func (h *Handler) prepareSaveEgressExceptionAI(tc *aitool.ToolContext, in aiSaveClusterEgressExceptionInput) (preparedEgressExceptionSave, error) {
	db := h.dbWithContext(tc.Context())
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, "", false)
	if err != nil {
		return preparedEgressExceptionSave{}, err
	}
	if err := h.requireClusterManageForAI(tc, cluster, "无权维护该运行集群的出站例外"); err != nil {
		return preparedEgressExceptionSave{}, err
	}

	convProjID := strings.TrimSpace(tc.ConversationProjectID)
	isUpdate := in.Exception != nil && strings.TrimSpace(*in.Exception) != ""

	var existing model.NetworkEgressException
	var next model.NetworkEgressException
	now := time.Now().UTC()

	if isUpdate {
		existing, err = resolveEgressExceptionForAI(db, cluster.ID, *in.Exception)
		if err != nil {
			return preparedEgressExceptionSave{}, err
		}
		if convProjID != "" {
			if len(existing.Grants) != 1 || existing.Grants[0].ProjectID != convProjID {
				return preparedEgressExceptionSave{}, projectScopeMismatchError(
					"egress exception has empty or multi-project/cross-project grants; switch to a global conversation to modify it")
			}
		}
		next = existing
		next.UpdatedAt = now
	} else {
		if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_name_invalid", "name is required when creating an egress exception")
		}
		if in.CIDR == nil || strings.TrimSpace(*in.CIDR) == "" {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_cidr_invalid", "cidr is required when creating an egress exception")
		}
		if in.Reason == nil || strings.TrimSpace(*in.Reason) == "" {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_reason_invalid", "reason is required when creating an egress exception")
		}
		if len(in.RemoveProjects) > 0 {
			return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid", "removeProjects cannot be used when creating an egress exception")
		}
		if convProjID != "" && len(in.UpsertGrants) == 0 {
			return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid", "upsertGrants must specify the bound conversation project when creating an egress exception in a project session")
		}
		next = model.NetworkEgressException{
			ID:               id.New("nex"),
			RuntimeClusterID: cluster.ID,
			Protocol:         kubeprovider.EgressExceptionProtocolTCP,
			CreatedBy:        tc.User.ID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if count := utf8.RuneCountInString(name); count == 0 || count > 64 {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_name_invalid", "name must be 1-64 characters")
		}
		next.Name = name
	}
	if in.Reason != nil {
		reason := strings.TrimSpace(*in.Reason)
		if count := utf8.RuneCountInString(reason); count == 0 || count > 500 {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_reason_invalid", "reason must be 1-500 characters")
		}
		next.Reason = reason
	}
	if in.CIDR != nil {
		prefix, cErr := kubeprovider.ParseNetworkCIDR(*in.CIDR)
		if cErr != nil || prefix.Bits() == 0 {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_cidr_invalid", "cidr must be a valid IPv4 or IPv6 CIDR other than /0")
		}
		for _, metadata := range kubeprovider.CloudMetadataCIDRs() {
			if prefix == netip.MustParsePrefix(metadata) {
				return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_cidr_invalid", "cidr must not be a cloud metadata address")
			}
		}
		next.CIDR = prefix.String()
	}
	if in.ClearPorts != nil && *in.ClearPorts {
		if in.Ports != nil && strings.TrimSpace(*in.Ports) != "" {
			return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid", "ports and clearPorts=true are mutually exclusive")
		}
		next.Ports = ""
	} else if in.Ports != nil {
		if strings.TrimSpace(*in.Ports) == "" {
			return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid", "use clearPorts=true to clear port restrictions rather than passing an empty string")
		}
		normPorts, pErr := normalizeBuildPrivateEgressPorts(*in.Ports)
		if pErr != nil {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_ports_invalid", pErr.Error())
		}
		next.Ports = normPorts
	}
	if in.Protocol != nil && strings.TrimSpace(*in.Protocol) != "" {
		proto := strings.TrimSpace(*in.Protocol)
		switch proto {
		case kubeprovider.EgressExceptionProtocolTCP, kubeprovider.EgressExceptionProtocolUDP, kubeprovider.EgressExceptionProtocolAny:
			next.Protocol = proto
		default:
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_protocol_invalid", "protocol must be tcp, udp or any")
		}
	}

	// Merge grants incrementally while preserving R5/R7 target boundaries on existing grants.
	existingGrantByProject := make(map[string]model.NetworkEgressExceptionGrant, len(existing.Grants))
	orderedProjects := make([]string, 0, len(existing.Grants)+len(in.UpsertGrants))
	for _, g := range existing.Grants {
		existingGrantByProject[g.ProjectID] = g
		orderedProjects = append(orderedProjects, g.ProjectID)
	}

	removedProjects, err := resolveProjectRefList(db, in.RemoveProjects)
	if err != nil {
		return preparedEgressExceptionSave{}, err
	}
	removeSet := make(map[string]bool, len(removedProjects))
	for _, p := range removedProjects {
		removeSet[p] = true
		delete(existingGrantByProject, p)
	}

	seenUpsert := make(map[string]bool, len(in.UpsertGrants))
	for _, ug := range in.UpsertGrants {
		proj, pErr := aitool.ResolveProject(db, ug.Project)
		if pErr != nil {
			return preparedEgressExceptionSave{}, pErr
		}
		if removeSet[proj.ID] {
			return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("project %s cannot appear in both upsertGrants and removeProjects", proj.ID))
		}
		if seenUpsert[proj.ID] {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_grant_invalid", fmt.Sprintf("duplicate project %s in upsertGrants", proj.ID))
		}
		seenUpsert[proj.ID] = true

		usable, uErr := runtimecluster.ProjectCanDeployToCluster(db, cluster, proj.ID)
		if uErr != nil || !usable {
			return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_grant_invalid",
				fmt.Sprintf("project %s is unavailable for runtime cluster %s", proj.ID, cluster.ID))
		}

		allTargets := ug.AllTargets != nil && *ug.AllTargets
		if allTargets && len(ug.Targets) > 0 {
			return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid", "targets and allTargets=true are mutually exclusive")
		}

		prevGrant, hadPrev := existingGrantByProject[proj.ID]
		var targetIDs []string
		switch {
		case allTargets:
			targetIDs = nil
		case ug.Targets != nil:
			if len(ug.Targets) == 0 && hadPrev && strings.TrimSpace(prevGrant.DeploymentTargetIDs) != "" {
				return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid", "pass allTargets=true to widen an existing grant to all targets in the project")
			}
			resolvedTargets := make([]string, 0, len(ug.Targets))
			seenTarget := make(map[string]bool, len(ug.Targets))
			for _, rawT := range ug.Targets {
				tID, tErr := resolveEgressGrantTargetID(db, proj.ID, rawT)
				if tErr != nil {
					return preparedEgressExceptionSave{}, tErr
				}
				if !seenTarget[tID] {
					seenTarget[tID] = true
					resolvedTargets = append(resolvedTargets, tID)
				}
			}
			if len(resolvedTargets) > maxEgressExceptionTarget {
				return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_grant_invalid",
					fmt.Sprintf("at most %d deployment targets per grant", maxEgressExceptionTarget))
			}
			targetIDs = resolvedTargets
		case hadPrev:
			// R5/R7: preserve existing target scope when targets is omitted on an existing grant!
			targetIDs = runtimecluster.SplitIDList(prevGrant.DeploymentTargetIDs)
		default:
			targetIDs = nil
		}

		includeBuilds := false
		if ug.IncludeBuilds != nil {
			includeBuilds = *ug.IncludeBuilds
		} else if hadPrev {
			includeBuilds = prevGrant.IncludeBuilds
		}

		grantID := id.New("nexg")
		createdAt := now
		if hadPrev && prevGrant.ID != "" {
			grantID = prevGrant.ID
			createdAt = prevGrant.CreatedAt
		} else {
			orderedProjects = append(orderedProjects, proj.ID)
		}
		existingGrantByProject[proj.ID] = model.NetworkEgressExceptionGrant{
			ID:                  grantID,
			ExceptionID:         next.ID,
			ProjectID:           proj.ID,
			DeploymentTargetIDs: strings.Join(targetIDs, ","),
			IncludeBuilds:       includeBuilds,
			CreatedAt:           createdAt,
		}
	}

	nextGrants := make([]model.NetworkEgressExceptionGrant, 0, len(existingGrantByProject))
	addedOrder := make(map[string]bool, len(existingGrantByProject))
	for _, pID := range orderedProjects {
		if g, exists := existingGrantByProject[pID]; exists && !addedOrder[pID] {
			addedOrder[pID] = true
			nextGrants = append(nextGrants, g)
		}
	}
	if len(nextGrants) > maxEgressExceptionGrants {
		return preparedEgressExceptionSave{}, aitool.BadRequest("runtime_cluster.egress_exception_grant_invalid",
			fmt.Sprintf("at most %d grants are allowed", maxEgressExceptionGrants))
	}

	if convProjID != "" {
		if len(nextGrants) == 0 {
			return preparedEgressExceptionSave{}, aitool.BadRequest("argument_invalid",
				"cannot empty the sole project grant in a single-project conversation; call runtime_delete_cluster_egress_exception to remove the exception")
		}
		if len(nextGrants) != 1 || nextGrants[0].ProjectID != convProjID {
			return preparedEgressExceptionSave{}, projectScopeMismatchError("egress exception grants must strictly match the bound conversation project")
		}
	}

	next.Grants = nextGrants
	return preparedEgressExceptionSave{
		isUpdate: isUpdate,
		cluster:  cluster,
		existing: existing,
		next:     next,
	}, nil
}

func (h *Handler) previewSaveClusterEgressExceptionAI(tc *aitool.ToolContext, in aiSaveClusterEgressExceptionInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareSaveEgressExceptionAI(tc, in)
	if err != nil {
		return nil, err
	}
	grantProjects := make([]string, len(prep.next.Grants))
	for i, g := range prep.next.Grants {
		grantProjects[i] = g.ProjectID
	}
	if !prep.isUpdate {
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("在运行集群 %s 上新建出站例外 %s（%s）", prep.cluster.Name, prep.next.Name, prep.next.CIDR),
				TargetResource: "network_egress_exception",
				ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
				Changes: []aitool.PreviewChange{
					{Field: "name", After: prep.next.Name, Action: "create"},
					{Field: "cidr", After: prep.next.CIDR, Action: "create"},
					{Field: "protocol", After: prep.next.Protocol, Action: "create"},
					{Field: "ports", After: prep.next.Ports, Action: "create"},
					{Field: "grantedProjects", After: grantProjects, Action: "grant"},
				},
			},
		}, nil
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("增量更新运行集群 %s 的出站例外 %s（%s）", prep.cluster.Name, prep.existing.Name, prep.existing.ID),
			TargetResource: "network_egress_exception",
			TargetID:       prep.existing.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "cidr", Before: prep.existing.CIDR, After: prep.next.CIDR, Action: "update"},
				{Field: "ports", Before: prep.existing.Ports, After: prep.next.Ports, Action: "update"},
				{Field: "protocol", Before: prep.existing.Protocol, After: prep.next.Protocol, Action: "update"},
				{Field: "grantedProjects", After: grantProjects, Action: "patch"},
			},
		},
	}, nil
}

func (h *Handler) execSaveClusterEgressExceptionAI(tc *aitool.ToolContext, in aiSaveClusterEgressExceptionInput) (map[string]any, error) {
	prep, err := h.prepareSaveEgressExceptionAI(tc, in)
	if err != nil {
		return nil, err
	}
	db := h.dbWithContext(tc.Context())
	sub, _ := newSubGin(tc)

	if !prep.isUpdate {
		if err := db.Create(&prep.next).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "runtime_cluster.egress_exception_save_failed", err.Error())
		}
		h.finishEgressExceptionMutation(sub, tc.User, prep.cluster, "create", prep.next)
	} else {
		if err := db.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&model.NetworkEgressException{}).
				Where("id = ? AND runtime_cluster_id = ?", prep.next.ID, prep.cluster.ID).
				Updates(map[string]any{
					"name":       prep.next.Name,
					"cidr":       prep.next.CIDR,
					"ports":      prep.next.Ports,
					"protocol":   prep.next.Protocol,
					"reason":     prep.next.Reason,
					"updated_at": prep.next.UpdatedAt,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			if delErr := tx.Where("exception_id = ?", prep.next.ID).Delete(&model.NetworkEgressExceptionGrant{}).Error; delErr != nil {
				return delErr
			}
			if len(prep.next.Grants) > 0 {
				return tx.Create(&prep.next.Grants).Error
			}
			return nil
		}); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("runtime_cluster.egress_exception_not_found", "egress exception not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "runtime_cluster.egress_exception_save_failed", err.Error())
		}
		h.finishEgressExceptionMutation(sub, tc.User, prep.cluster, "update", prep.next)
	}

	items, fmtErr := h.egressExceptionResponses(sub, prep.cluster, []model.NetworkEgressException{prep.next})
	if fmtErr != nil || len(items) == 0 {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "egress exception readback failed")
	}
	raw, _ := json.Marshal(items[0])
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if convProj := strings.TrimSpace(tc.ConversationProjectID); convProj != "" {
		out["projectId"] = convProj
	}
	return out, nil
}

func (h *Handler) previewDeleteClusterEgressExceptionAI(tc *aitool.ToolContext, in aiDeleteClusterEgressExceptionInput) (*aitool.PreviewResult, error) {
	db := h.dbWithContext(tc.Context())
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, "", false)
	if err != nil {
		return nil, err
	}
	if err := h.requireClusterManageForAI(tc, cluster, "无权维护该运行集群的出站例外"); err != nil {
		return nil, err
	}
	existing, err := resolveEgressExceptionForAI(db, cluster.ID, in.Exception)
	if err != nil {
		return nil, err
	}
	if convProjID := strings.TrimSpace(tc.ConversationProjectID); convProjID != "" {
		if len(existing.Grants) != 1 || existing.Grants[0].ProjectID != convProjID {
			return nil, projectScopeMismatchError("egress exception does not belong exclusively to the bound conversation project")
		}
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("删除运行集群 %s 的出站例外 %s（%s）并触发 NetworkPolicy 对账", cluster.Name, existing.Name, existing.CIDR),
			TargetResource: "network_egress_exception",
			TargetID:       existing.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "exception", Before: existing.Name, After: nil, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) execDeleteClusterEgressExceptionAI(tc *aitool.ToolContext, in aiDeleteClusterEgressExceptionInput) (map[string]any, error) {
	db := h.dbWithContext(tc.Context())
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, "", false)
	if err != nil {
		return nil, err
	}
	if err := h.requireClusterManageForAI(tc, cluster, "无权维护该运行集群的出站例外"); err != nil {
		return nil, err
	}
	existing, err := resolveEgressExceptionForAI(db, cluster.ID, in.Exception)
	if err != nil {
		return nil, err
	}
	if convProjID := strings.TrimSpace(tc.ConversationProjectID); convProjID != "" {
		if len(existing.Grants) != 1 || existing.Grants[0].ProjectID != convProjID {
			return nil, projectScopeMismatchError("egress exception does not belong exclusively to the bound conversation project")
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if delGrantsErr := tx.Where("exception_id = ?", existing.ID).Delete(&model.NetworkEgressExceptionGrant{}).Error; delGrantsErr != nil {
			return delGrantsErr
		}
		res := tx.Delete(&model.NetworkEgressException{}, "id = ? AND runtime_cluster_id = ?", existing.ID, cluster.ID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NotFound("runtime_cluster.egress_exception_not_found", "egress exception not found")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "runtime_cluster.egress_exception_delete_failed", err.Error())
	}
	sub, _ := newSubGin(tc)
	h.finishEgressExceptionMutation(sub, tc.User, cluster, "delete", existing)
	out := map[string]any{
		"id":        existing.ID,
		"clusterId": cluster.ID,
		"deleted":   true,
	}
	if convProj := strings.TrimSpace(tc.ConversationProjectID); convProj != "" {
		out["projectId"] = convProj
	}
	return out, nil
}

func (h *Handler) resolvePendingShareForClusterAI(tc *aitool.ToolContext, clusterRef, shareID string) (model.RuntimeCluster, model.NetworkServiceShare, error) {
	db := h.dbWithContext(tc.Context())
	cluster, err := h.resolveClusterForAI(tc, clusterRef, "", false)
	if err != nil {
		return model.RuntimeCluster{}, model.NetworkServiceShare{}, err
	}
	if err := h.requireClusterManageForAI(tc, cluster, "无权审批该运行集群的跨项目共享"); err != nil {
		return model.RuntimeCluster{}, model.NetworkServiceShare{}, err
	}
	convProjID := strings.TrimSpace(tc.ConversationProjectID)
	if convProjID != "" {
		usable, uErr := runtimecluster.ProjectCanDeployToCluster(db, cluster, convProjID)
		if uErr != nil || !usable {
			return model.RuntimeCluster{}, model.NetworkServiceShare{}, projectScopeMismatchError("runtime cluster is not usable for the bound conversation project")
		}
	}
	shareID = strings.TrimSpace(shareID)
	if shareID == "" {
		return model.RuntimeCluster{}, model.NetworkServiceShare{}, aitool.BadRequest("argument_invalid", "shareId is required")
	}
	query, qErr := runtimecluster.ServiceSharesOnCluster(tc.Context(), db, cluster)
	if qErr != nil {
		return model.RuntimeCluster{}, model.NetworkServiceShare{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", qErr.Error())
	}
	var share model.NetworkServiceShare
	if err := runtimecluster.SharesNeedingApproval(query, cluster.ID).Select("s.*").Where("s.id = ?", shareID).Take(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.RuntimeCluster{}, model.NetworkServiceShare{}, aitool.NotFound("network_share.not_found", "pending network share was not found on this cluster")
		}
		return model.RuntimeCluster{}, model.NetworkServiceShare{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if convProjID != "" && share.ProviderProjectID != convProjID {
		return model.RuntimeCluster{}, model.NetworkServiceShare{}, projectScopeMismatchError("only shares provided by the bound conversation project can be approved in this session")
	}
	return cluster, share, nil
}

func (h *Handler) previewApproveClusterNetworkShareAI(tc *aitool.ToolContext, in aiApproveClusterNetworkShareInput) (*aitool.PreviewResult, error) {
	db := h.dbWithContext(tc.Context())
	cluster, share, err := h.resolvePendingShareForClusterAI(tc, in.Cluster, in.ShareID)
	if err != nil {
		return nil, err
	}
	items, respErr := projectapi.NetworkServiceShareResponses(db, []model.NetworkServiceShare{share})
	providerLabel := share.ProviderProjectID
	consumerLabel := share.ConsumerProjectID
	if respErr == nil && len(items) == 1 {
		if items[0].ProviderProjectName != "" {
			providerLabel = fmt.Sprintf("%s (%s)", items[0].ProviderProjectName, share.ProviderProjectID)
		}
		if items[0].ConsumerProjectName != "" {
			consumerLabel = fmt.Sprintf("%s (%s)", items[0].ConsumerProjectName, share.ConsumerProjectID)
		}
	}
	flow := fmt.Sprintf("%s -> %s", providerLabel, consumerLabel)
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("在集群 %s 上批准跨项目内网服务共享：%s", cluster.Name, flow),
			TargetResource: "network_service_share",
			TargetID:       share.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "providerToConsumer", After: flow, Action: "approve"},
				{Field: "approvedRuntimeClusterId", Before: share.ApprovedRuntimeClusterID, After: cluster.ID, Action: "approve"},
			},
			Impact: []string{
				fmt.Sprintf("跨项目流量方向：%s（提供方持有集群出站例外，批准后消费方可通过该共享访问提供方内网服务）", flow),
			},
		},
	}, nil
}

func (h *Handler) execApproveClusterNetworkShareAI(tc *aitool.ToolContext, in aiApproveClusterNetworkShareInput) (map[string]any, error) {
	db := h.dbWithContext(tc.Context())
	cluster, share, err := h.resolvePendingShareForClusterAI(tc, in.Cluster, in.ShareID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	res := db.Model(&model.NetworkServiceShare{}).Where("id = ?", share.ID).Updates(map[string]any{
		"approved_runtime_cluster_id": cluster.ID,
		"approved_at":                 now,
		"approved_by":                 tc.User.ID,
		"updated_at":                  now,
	})
	if res.Error != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "network_share.save_failed", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return nil, aitool.NotFound("network_share.not_found", "network share not found")
	}
	share.ApprovedRuntimeClusterID = cluster.ID
	share.ApprovedAt = &now
	share.ApprovedBy = tc.User.ID
	share.UpdatedAt = now
	h.auditWithContext(tc.User.ID, "runtime_cluster.network_share.approve", share.ID, true,
		fmt.Sprintf("cluster=%s target=%s provider=%s consumer=%s", cluster.ID, share.DeploymentTargetID, share.ProviderProjectID, share.ConsumerProjectID), tc.Context())
	h.host.EnqueueNetworkPolicyReconcile(tc.Context(), cluster.ID)

	items, respErr := projectapi.NetworkServiceShareResponses(db, []model.NetworkServiceShare{share})
	if respErr != nil || len(items) == 0 {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", "network share readback failed")
	}
	raw, _ := json.Marshal(items[0])
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if convProj := strings.TrimSpace(tc.ConversationProjectID); convProj != "" {
		out["projectId"] = convProj
	}
	return out, nil
}

func (h *Handler) resolveManagedK8sResourceForDeleteAI(
	tc *aitool.ToolContext,
	in aiDeleteClusterK8sResourceInput,
) (model.RuntimeCluster, *kubeprovider.Client, clusterResourceResponse, error) {
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, "", false)
	if err != nil {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, err
	}
	if err := h.requireClusterManageForAI(tc, cluster, "无权维护该集群资源"); err != nil {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, err
	}
	kind := strings.TrimSpace(in.ResourceKind)
	if !validRuntimeResourceKind(kind) {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, aitool.BadRequest("cluster.resource_kind_invalid", "resourceKind must be one of the 11 supported Kubernetes resource kinds")
	}
	name := strings.TrimSpace(in.ResourceName)
	if name == "" {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, aitool.BadRequest("cluster.resource_name_required", "resourceName is required")
	}
	namespace := ""
	if in.Namespace != nil {
		namespace = strings.TrimSpace(*in.Namespace)
	}
	if kind != "Namespace" && namespace == "" {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, aitool.BadRequest("cluster.resource_namespace_required", "namespace is required for namespaced Kubernetes resources")
	}
	client, err := h.kubeClientForClusterAI(tc.Context(), cluster)
	if err != nil {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, err
	}
	reqCtx, cancel := context.WithTimeout(tc.Context(), 10*time.Second)
	defer cancel()
	snapshot, getErr := client.GetManagedResource(reqCtx, kind, namespace, name)
	if getErr != nil {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, classifyClusterK8sResourceError(getErr)
	}
	enriched, enrichErr := h.clusterResourceResponses([]kubeprovider.ResourceSnapshot{snapshot}, tc.Context())
	if enrichErr != nil {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", enrichErr.Error())
	}
	resp := enriched[0]
	snapshot.ProjectID = resp.ProjectID
	snapshot.ApplicationID = resp.ApplicationID
	snapshot.DeploymentTargetID = resp.DeploymentTargetID

	if convProjID := strings.TrimSpace(tc.ConversationProjectID); convProjID != "" && strings.TrimSpace(resp.ProjectID) != convProjID {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, projectScopeMismatchError("managed Kubernetes resource does not belong to the bound conversation project")
	}
	sub, rec := newSubGin(tc)
	if !h.canManageClusterResourceSnapshot(sub, tc.User, snapshot) {
		return model.RuntimeCluster{}, nil, clusterResourceResponse{}, abortedToolError(rec, "cluster.resource_forbidden", "insufficient project permission to delete this cluster resource")
	}
	return cluster, client, resp, nil
}

func (h *Handler) previewDeleteClusterK8sResourceAI(tc *aitool.ToolContext, in aiDeleteClusterK8sResourceInput) (*aitool.PreviewResult, error) {
	cluster, _, resp, err := h.resolveManagedK8sResourceForDeleteAI(tc, in)
	if err != nil {
		return nil, err
	}
	projID := strings.TrimSpace(tc.ConversationProjectID)
	if projID == "" {
		projID = strings.TrimSpace(resp.ProjectID)
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("删除集群 %s 上的受管 K8s 资源 %s/%s（namespace=%s）", cluster.Name, resp.Kind, resp.Name, resp.Namespace),
			TargetResource: "k8s_resource",
			TargetID:       resp.Name,
			ProjectID:      projID,
			Changes: []aitool.PreviewChange{
				{Field: "resourceKind", Before: resp.Kind, Action: "delete"},
				{Field: "namespace", Before: resp.Namespace, Action: "delete"},
				{Field: "resourceName", Before: resp.Name, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) execDeleteClusterK8sResourceAI(tc *aitool.ToolContext, in aiDeleteClusterK8sResourceInput) (map[string]any, error) {
	cluster, client, resp, err := h.resolveManagedK8sResourceForDeleteAI(tc, in)
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(tc.Context(), 10*time.Second)
	defer cancel()
	if delErr := client.DeleteManagedResource(reqCtx, resp.Kind, resp.Namespace, resp.Name); delErr != nil {
		return nil, classifyClusterK8sResourceError(delErr)
	}
	h.auditWithContext(tc.User.ID, "runtime_cluster_resource.delete", cluster.ID, true,
		strings.Join([]string{resp.Kind, resp.Namespace, resp.Name}, "/"), tc.Context())

	out := map[string]any{
		"id":           resp.Name,
		"clusterId":    cluster.ID,
		"resourceKind": resp.Kind,
		"namespace":    resp.Namespace,
		"resourceName": resp.Name,
		"deleted":      true,
	}
	if projID := firstNonEmpty(tc.ConversationProjectID, resp.ProjectID); projID != "" {
		out["projectId"] = projID
	}
	return out, nil
}

const aiGatewayTrafficReportFreshness = 10 * time.Minute

func systemComponentObservationRank(status string) int {
	switch status {
	case observation.StatusReady:
		return 5
	case observation.StatusDegraded:
		return 4
	case observation.StatusProgressing:
		return 3
	case observation.StatusScaledToZero:
		return 2
	case observation.StatusUnavailable:
		return 1
	default:
		return 0
	}
}

func (h *Handler) computeGatewayTrafficProbeStatus(ctx context.Context, db *gorm.DB) (map[string]any, error) {
	var probeInstallations []model.SystemComponentInstallation
	if err := db.Where("component_id = ?", systemComponentGatewayTrafficProbe).Order("created_at desc").Find(&probeInstallations).Error; err != nil {
		return nil, err
	}
	if len(probeInstallations) == 0 {
		observedAt := time.Now().UTC()
		return map[string]any{
			"available":             false,
			"installed":             false,
			"status":                observation.StatusNotConfigured,
			"componentId":           systemComponentGatewayTrafficProbe,
			"installableTemplateId": "luna-gateway-traffic-probe",
			"observationCode":       "billing.gateway_traffic_probe_not_configured",
			"observedAt":            observedAt,
		}, nil
	}

	h.observeSystemComponentInstallations(ctx, probeInstallations)
	bestStatus := observation.StatusUnknown
	bestCode := "billing.gateway_traffic_probe_unknown"
	var bestObservedAt *time.Time
	bestRank := -1
	for i := range probeInstallations {
		rank := systemComponentObservationRank(probeInstallations[i].RuntimeStatus)
		if rank > bestRank {
			bestRank = rank
			bestStatus = probeInstallations[i].RuntimeStatus
			bestCode = probeInstallations[i].ObservationCode
			bestObservedAt = probeInstallations[i].ObservedAt
		}
	}

	var latestUsage model.BillingUsageRecord
	usageRes := db.Where("resource_type = ? AND meter = ? AND status = ?", billing.ResourceTypeGateway, "gateway.egress_gib", "settled").
		Order("period_end desc").
		First(&latestUsage)
	if usageRes.Error != nil && !errors.Is(usageRes.Error, gorm.ErrRecordNotFound) {
		return nil, usageRes.Error
	}
	hasReport := usageRes.Error == nil
	var lastReportedAt, lastWindowStart, lastWindowEnd *time.Time
	reportFresh := false
	if hasReport {
		repAt := latestUsage.CreatedAt.UTC()
		if latestUsage.SettledAt != nil {
			repAt = latestUsage.SettledAt.UTC()
		}
		ws := latestUsage.PeriodStart.UTC()
		we := latestUsage.PeriodEnd.UTC()
		lastReportedAt = &repAt
		lastWindowStart = &ws
		lastWindowEnd = &we
		reportFresh = we.After(time.Now().UTC().Add(-aiGatewayTrafficReportFreshness))
	}
	if bestStatus == observation.StatusReady {
		switch {
		case !hasReport:
			bestCode = "billing.gateway_traffic_waiting_report"
		case !reportFresh:
			bestCode = "billing.gateway_traffic_report_stale"
		}
	}

	out := map[string]any{
		"available":             bestStatus == observation.StatusReady && reportFresh,
		"installed":             true,
		"status":                bestStatus,
		"componentId":           systemComponentGatewayTrafficProbe,
		"installableTemplateId": "luna-gateway-traffic-probe",
		"observationCode":       bestCode,
	}
	if bestObservedAt != nil {
		out["observedAt"] = bestObservedAt
	}
	if lastReportedAt != nil {
		out["lastReportedAt"] = lastReportedAt
	}
	if lastWindowStart != nil {
		out["lastWindowStart"] = lastWindowStart
	}
	if lastWindowEnd != nil {
		out["lastWindowEnd"] = lastWindowEnd
	}
	return out, nil
}

func (h *Handler) execListSystemComponentsAI(tc *aitool.ToolContext, in aiListSystemComponentsInput) (map[string]any, error) {
	tc.SetNoStore()
	if _, err := h.requireAIPlatformAdmin(tc); err != nil {
		return nil, err
	}
	db := h.dbWithContext(tc.Context())
	query := db.Model(&model.SystemComponentInstallation{}).Order("component_id asc, runtime_cluster_id asc")

	if in.ComponentID != nil && strings.TrimSpace(*in.ComponentID) != "" {
		query = query.Where("component_id = ?", strings.TrimSpace(*in.ComponentID))
	}
	if in.Cluster != nil && strings.TrimSpace(*in.Cluster) != "" {
		cluster, err := h.resolveClusterForAI(tc, *in.Cluster, "", true)
		if err != nil {
			return nil, err
		}
		query = query.Where("runtime_cluster_id = ?", cluster.ID)
	}

	var items []model.SystemComponentInstallation
	if err := query.Find(&items).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if items == nil {
		items = []model.SystemComponentInstallation{}
	}
	h.observeSystemComponentInstallations(tc.Context(), items)

	trafficStatus, err := h.computeGatewayTrafficProbeStatus(tc.Context(), db)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	return map[string]any{
		"items":                      items,
		"gatewayTrafficProbeEnabled": hasReadySystemComponent(items, systemComponentGatewayTrafficProbe),
		"gatewayTrafficStatus":       trafficStatus,
		"available":                  trafficStatus["available"],
		"status":                     trafficStatus["status"],
		"observationCode":            trafficStatus["observationCode"],
	}, nil
}

type validatedSystemComponentInstall struct {
	user              model.User
	template          appstore.Template
	componentID       string
	cluster           model.RuntimeCluster
	mode              string
	apiBaseURL        string
	traefikMetricsURL string
	imageOverride     string
	provisionAccess   bool
	configJSON        string
}

func (h *Handler) validateInstallSystemComponentAI(tc *aitool.ToolContext, in aiInstallSystemComponentInput) (validatedSystemComponentInstall, error) {
	user, err := h.requireAIPlatformAdmin(tc)
	if err != nil {
		return validatedSystemComponentInstall{}, err
	}
	template, found, findErr := appstore.Find(strings.TrimSpace(in.Template))
	if findErr != nil {
		return validatedSystemComponentInstall{}, aitool.NewError(http.StatusInternalServerError, "system_component.template_lookup_failed", findErr.Error())
	}
	if !found || strings.TrimSpace(template.Kind) != "system_component" || strings.TrimSpace(template.SystemComponent) == "" {
		return validatedSystemComponentInstall{}, aitool.NotFound("system_component.template_not_found", "system component template not found")
	}
	cluster, cErr := h.resolveClusterForAI(tc, in.Cluster, "", false)
	if cErr != nil {
		return validatedSystemComponentInstall{}, cErr
	}
	if !h.host.TaskQueueAvailable() {
		return validatedSystemComponentInstall{}, aitool.NewError(http.StatusServiceUnavailable, "system_component.task_queue_unavailable", "task queue is not configured")
	}
	mode := "traefik-metrics"
	if in.Mode != nil && strings.TrimSpace(*in.Mode) != "" {
		mode = strings.TrimSpace(*in.Mode)
	}
	if mode != "traefik-metrics" {
		return validatedSystemComponentInstall{}, aitool.BadRequest("argument_invalid", "mode must be traefik-metrics")
	}
	apiBaseURL := strings.TrimRight(strings.TrimSpace(in.APIBaseURL), "/")
	if apiBaseURL == "" {
		return validatedSystemComponentInstall{}, aitool.BadRequest("system_component.api_base_url_required", "apiBaseUrl is required")
	}
	if uErr := validateOptionalHTTPURL(apiBaseURL, "API base URL"); uErr != nil {
		return validatedSystemComponentInstall{}, aitool.BadRequest("system_component.api_base_url_invalid", uErr.Error())
	}
	traefikMetricsURL := ""
	if in.TraefikMetricsURL != nil {
		traefikMetricsURL = strings.TrimSpace(*in.TraefikMetricsURL)
	}
	if uErr := validateOptionalHTTPURL(traefikMetricsURL, "Traefik metrics URL"); uErr != nil {
		return validatedSystemComponentInstall{}, aitool.BadRequest("system_component.traefik_metrics_url_invalid", uErr.Error())
	}
	imageOverride := ""
	if in.Image != nil {
		imageOverride = strings.TrimSpace(*in.Image)
	}
	provisionAccess := true
	if in.ProvisionAccess != nil {
		provisionAccess = *in.ProvisionAccess
	}
	cfgBytes, _ := json.Marshal(map[string]string{
		"apiBaseUrl":        apiBaseURL,
		"traefikMetricsUrl": traefikMetricsURL,
		"image":             imageOverride,
	})
	return validatedSystemComponentInstall{
		user:              user,
		template:          template,
		componentID:       strings.TrimSpace(template.SystemComponent),
		cluster:           cluster,
		mode:              mode,
		apiBaseURL:        apiBaseURL,
		traefikMetricsURL: traefikMetricsURL,
		imageOverride:     imageOverride,
		provisionAccess:   provisionAccess,
		configJSON:        string(cfgBytes),
	}, nil
}

func (h *Handler) previewInstallSystemComponentAI(tc *aitool.ToolContext, in aiInstallSystemComponentInput) (*aitool.PreviewResult, error) {
	v, err := h.validateInstallSystemComponentAI(tc, in)
	if err != nil {
		return nil, err
	}
	var existing model.SystemComponentInstallation
	action := "install"
	if err := h.dbWithContext(tc.Context()).
		First(&existing, "component_id = ? AND runtime_cluster_id = ?", v.componentID, v.cluster.ID).Error; err == nil {
		action = "update"
	}
	resolvedImg := firstNonEmpty(v.imageOverride, v.template.Image, "liteyukistudio/devops-gateway-traffic-probe:nightly")
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("在运行集群 %s 上安装/升级平台系统组件 %s", v.cluster.Name, v.componentID),
			TargetResource: "system_component",
			TargetID:       v.componentID,
			Changes: []aitool.PreviewChange{
				{Field: "cluster", After: fmt.Sprintf("%s (%s)", v.cluster.Name, v.cluster.ID), Action: action},
				{Field: "image", After: resolvedImg, Action: action},
				{Field: "apiBaseUrl", After: v.apiBaseURL, Action: action},
				{Field: "provisionAccess", After: v.provisionAccess, Action: action},
			},
		},
	}, nil
}

func (h *Handler) execInstallSystemComponentAI(tc *aitool.ToolContext, in aiInstallSystemComponentInput) (map[string]any, error) {
	v, err := h.validateInstallSystemComponentAI(tc, in)
	if err != nil {
		return nil, err
	}
	if !h.secrets.Available() {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "system_component.secret_store_unavailable", "secret store is unavailable")
	}
	platformProject, pErr := h.ensurePlatformSystemProject(v.user, tc.Context())
	if pErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "system_component.project_init_failed", pErr.Error())
	}

	reportToken := "lyd_probe_" + randomHex(32)
	sub, rec := newSubGin(tc)
	plan, ok := h.systemComponentApplicationPlan(
		sub,
		v.user,
		platformProject,
		v.cluster,
		v.template,
		v.componentID,
		v.mode,
		v.configJSON,
		v.apiBaseURL,
		v.traefikMetricsURL,
		reportToken,
		v.imageOverride,
		v.provisionAccess,
	)
	if !ok {
		return nil, abortedToolError(rec, "system_component.plan_failed", "failed to prepare system component deployment plan")
	}
	if err := h.persistSystemComponentApplicationPlan(plan, tc.Context()); err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "system_component.persist_failed", err.Error())
	}
	// Reload installation ID in case OnConflict updated an existing row.
	var persistedInst model.SystemComponentInstallation
	if err := h.dbWithContext(tc.Context()).
		First(&persistedInst, "component_id = ? AND runtime_cluster_id = ?", v.componentID, v.cluster.ID).Error; err == nil {
		plan.Installation = persistedInst
	}

	if !h.enqueueDeployRun(tc.Context(), *plan.Release) {
		message := "平台组件部署任务投递失败"
		db := h.dbWithContext(tc.Context())
		_ = db.Model(&model.Release{}).
			Where("id = ? AND status IN ?", plan.Release.ID, model.ReleaseActiveStatuses).
			Updates(map[string]any{"status": "failed", "failure_code": model.ReleaseFailureDispatchFailed, "message": message}).Error
		_ = db.Model(&model.SystemComponentInstallation{}).
			Where("id = ?", plan.Installation.ID).
			Updates(map[string]any{"status": "failed", "message": message, "last_error": message}).Error
		return nil, aitool.NewError(http.StatusServiceUnavailable, "release.dispatch_failed", message)
	}

	h.auditWithContext(v.user.ID, "system_component.install", plan.Installation.ID, true, v.componentID, tc.Context())
	return map[string]any{
		"id":                 plan.Installation.ID,
		"installationId":     plan.Installation.ID,
		"projectId":          platformProject.ID,
		"applicationId":      plan.Application.ID,
		"deploymentTargetId": plan.DeploymentTarget.ID,
		"releaseId":          plan.Release.ID,
		"status":             plan.Release.Status,
		"installation":       plan.Installation,
		"application":        plan.Application,
		"deploymentTarget":   h.host.DeploymentTargetResponse(plan.DeploymentTarget),
		"release":            plan.Release,
	}, nil
}
