package runtimeapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/util/validation"
)

type aiListClustersInput struct {
	Cluster         *string `json:"cluster,omitempty" desc:"按运行集群 ID（clu_xxx）或精确名称筛选单个集群"`
	Project         *string `json:"project,omitempty" desc:"按项目 ID 或 identifier 筛选该项目可用集群"`
	IncludePressure *bool   `json:"includePressure,omitempty" default:"true" desc:"是否附带集群实时负载等级观测（默认 true）"`
	Search          *string `json:"search,omitempty" desc:"按集群名称或 Endpoint 模糊搜索"`
	SortBy          *string `json:"sortBy,omitempty" enum:"createdAt|name|scope" default:"createdAt" desc:"排序字段"`
	SortOrder       *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page            *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize        *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type aiClusterResourcePolicyInput struct {
	CPURequestPercent    *int `json:"cpuRequestPercent,omitempty" min:"0" desc:"CPU Request 占 Limit 百分比（0 表示仅设 Request 不设 Limit）"`
	MemoryRequestPercent *int `json:"memoryRequestPercent,omitempty" min:"0" desc:"内存 Request 占 Limit 百分比"`
	CPULimitPercent      *int `json:"cpuLimitPercent,omitempty" min:"0" desc:"CPU Limit 超卖百分比"`
	MemoryLimitPercent   *int `json:"memoryLimitPercent,omitempty" min:"0" desc:"内存 Limit 超卖百分比"`
}

type aiClusterGatewayConfigInput struct {
	AddDomainSuffixes               []string          `json:"addDomainSuffixes,omitempty" desc:"增量添加网关泛域名后缀"`
	RemoveDomainSuffixes            []string          `json:"removeDomainSuffixes,omitempty" desc:"增量移除网关泛域名后缀"`
	PublicScheme                    *string           `json:"publicScheme,omitempty" enum:"http|https" desc:"公网入口协议"`
	PublicPort                      *int              `json:"publicPort,omitempty" min:"1" max:"65535" desc:"公网入口端口"`
	ControllerType                  *string           `json:"controllerType,omitempty" enum:"traefik|generic" desc:"网关控制器类型"`
	ClassName                       *string           `json:"className,omitempty" desc:"GatewayClass 名称"`
	GatewayName                     *string           `json:"gatewayName,omitempty" desc:"Gateway 资源名称"`
	GatewayNamespace                *string           `json:"gatewayNamespace,omitempty" desc:"Gateway 所在命名空间"`
	HTTPListenerName                *string           `json:"httpListenerName,omitempty" desc:"HTTP Listener 名称"`
	HTTPListenerPort                *int              `json:"httpListenerPort,omitempty" min:"1" max:"65535" desc:"HTTP Listener 端口"`
	HTTPSListenerName               *string           `json:"httpsListenerName,omitempty" desc:"HTTPS Listener 名称"`
	HTTPSListenerPort               *int              `json:"httpsListenerPort,omitempty" min:"1" max:"65535" desc:"HTTPS Listener 端口"`
	TLSSecretName                   *string           `json:"tlsSecretName,omitempty" desc:"默认 TLS Secret 名称"`
	TLSSecretNamespace              *string           `json:"tlsSecretNamespace,omitempty" desc:"默认 TLS Secret 命名空间"`
	ClearTLSSecret                  *bool             `json:"clearTlsSecret,omitempty" desc:"清空默认 TLS Secret 配置"`
	CertIssuerKind                  *string           `json:"certIssuerKind,omitempty" enum:"ClusterIssuer|Issuer" desc:"cert-manager 签发器类型"`
	CertIssuerName                  *string           `json:"certIssuerName,omitempty" desc:"cert-manager 签发器名称"`
	ClearCertIssuerName             *bool             `json:"clearCertIssuerName,omitempty" desc:"清空签发器名称"`
	CertificateNamespace            *string           `json:"certificateNamespace,omitempty" desc:"Certificate 资源所在命名空间"`
	ClearCertificateNamespace       *bool             `json:"clearCertificateNamespace,omitempty" desc:"清空 Certificate 命名空间"`
	WildcardCertEnabled             *bool             `json:"wildcardCertEnabled,omitempty" desc:"是否启用通配符证书"`
	WildcardCertDomain              *string           `json:"wildcardCertDomain,omitempty" desc:"通配符证书域名"`
	WildcardCertSecretName          *string           `json:"wildcardCertSecretName,omitempty" desc:"通配符证书 Secret 名称"`
	ClearWildcardCert               *bool             `json:"clearWildcardCert,omitempty" desc:"禁用并清空通配符证书配置"`
	ExternalTLSMode                 *string           `json:"externalTlsMode,omitempty" enum:"none|gateway|upstream" desc:"外部 TLS 终结模式"`
	ForwardedHeadersMode            *string           `json:"forwardedHeadersMode,omitempty" enum:"preserve|overwrite|none" desc:"转发头处理模式"`
	AddTrustedProxyCIDRs            []string          `json:"addTrustedProxyCidrs,omitempty" desc:"增量添加可信代理 CIDR"`
	RemoveTrustedProxyCIDRs         []string          `json:"removeTrustedProxyCidrs,omitempty" desc:"增量移除可信代理 CIDR"`
	ClearTrustedProxyCIDRs          *bool             `json:"clearTrustedProxyCidrs,omitempty" desc:"清空全部可信代理 CIDR"`
	UpsertDefaultRequestHeaders     map[string]string `json:"upsertDefaultRequestHeaders,omitempty" desc:"增量设置网关默认请求头"`
	RemoveDefaultRequestHeaderKeys  []string          `json:"removeDefaultRequestHeaderKeys,omitempty" desc:"移除指定网关默认请求头"`
	UpsertDefaultResponseHeaders    map[string]string `json:"upsertDefaultResponseHeaders,omitempty" desc:"增量设置网关默认响应头"`
	RemoveDefaultResponseHeaderKeys []string          `json:"removeDefaultResponseHeaderKeys,omitempty" desc:"移除指定网关默认响应头"`
}

type aiClusterNetworkSecurityInput struct {
	NetworkGatewayNamespace       *string  `json:"networkGatewayNamespace,omitempty" desc:"入站网关真实 Pod 所在命名空间（为空继承 gatewayNamespace）"`
	ClearNetworkGatewayNamespace  *bool    `json:"clearNetworkGatewayNamespace,omitempty" desc:"清空 networkGatewayNamespace 恢复继承 gatewayNamespace"`
	AddNetworkBlockedCIDRs        []string `json:"addNetworkBlockedCidrs,omitempty" desc:"增量添加集群级额外阻断出站 CIDR"`
	RemoveNetworkBlockedCIDRs     []string `json:"removeNetworkBlockedCidrs,omitempty" desc:"增量移除集群级阻断出站 CIDR"`
	BuildEgressMode               *string  `json:"buildEgressMode,omitempty" enum:"restricted|permissive" desc:"构建网络出站模式"`
	AddBuildPrivateEgressCIDRs    []string `json:"addBuildPrivateEgressCidrs,omitempty" desc:"增量添加构建私网镜像站放行 CIDR"`
	RemoveBuildPrivateEgressCIDRs []string `json:"removeBuildPrivateEgressCidrs,omitempty" desc:"增量移除构建私网镜像站放行 CIDR"`
	AddBuildPrivateEgressPorts    []string `json:"addBuildPrivateEgressPorts,omitempty" desc:"增量添加构建私网放行端口（如 443）"`
	RemoveBuildPrivateEgressPorts []string `json:"removeBuildPrivateEgressPorts,omitempty" desc:"增量移除构建私网放行端口"`
}

type aiSaveClusterInput struct {
	Cluster             *string                        `json:"cluster,omitempty" desc:"集群 ID（clu_xxx）或唯一名称；省略为新建，传入为纯增量更新"`
	Name                *string                        `json:"name,omitempty" desc:"集群名称（新建必填）"`
	Endpoint            *string                        `json:"endpoint,omitempty" desc:"集群展示或 API Endpoint"`
	Scope               *string                        `json:"scope,omitempty" enum:"user|project|global" desc:"集群可见作用域（更新省略保留原作用域）"`
	AddProjects         []string                       `json:"addProjects,omitempty" desc:"scope=project 时增量授权的项目 ID 或 identifier 列表"`
	RemoveProjects      []string                       `json:"removeProjects,omitempty" desc:"scope=project 时增量移除授权的项目 ID 或 identifier 列表"`
	Kubeconfig          *string                        `json:"kubeconfig,omitempty" sensitive:"true" desc:"内联安全 HTTPS Kubeconfig（更新省略保留原凭据）"`
	IsDefault           *bool                          `json:"isDefault,omitempty" desc:"是否设为平台默认集群（仅 global 集群可用）"`
	MaxConcurrentBuilds *int                           `json:"maxConcurrentBuilds,omitempty" min:"1" desc:"最大并发构建任务数"`
	ResourcePolicy      *aiClusterResourcePolicyInput  `json:"resourcePolicy,omitempty" desc:"算力超卖与请求/上限比例策略"`
	GatewayConfig       *aiClusterGatewayConfigInput   `json:"gatewayConfig,omitempty" desc:"网关域名后缀、TLS 与默认头增量配置"`
	NetworkSecurity     *aiClusterNetworkSecurityInput `json:"networkSecurity,omitempty" desc:"网络隔离与构建私网出站增量配置"`
}

type aiTestClusterInput struct {
	Cluster string `json:"cluster" desc:"运行集群 ID（clu_xxx）或唯一名称"`
}

type aiDeleteClusterInput struct {
	Cluster string `json:"cluster" desc:"运行集群 ID（clu_xxx）或唯一名称"`
}

func (h *Handler) registerClusterAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[aiListClustersInput, map[string]any]{
		Name:           "runtime_list_clusters",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster",
		Summary:        "分页查看当前可见的运行集群配置摘要、网关域名后缀及实时负载等级",
		Purpose:        aitool.Text("查询当前可见或指定项目可用的运行集群列表、结构化网关/网络策略配置及实时负载压力等级。", "List visible runtime clusters with structured gateway/network configuration and live pressure status."),
		AliasesZH:      []string{"查看运行集群", "集群列表", "集群负载"},
		AliasesEN:      []string{"list clusters", "runtime clusters", "cluster pressure"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionClusterRead)},
		Execute:        h.execListClustersAI,
	})

	aitool.Register(reg, aitool.Spec[aiSaveClusterInput, map[string]any]{
		Name:           "runtime_save_cluster",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster",
		Summary:        "创建或纯增量修改运行集群配置",
		Purpose:        aitool.Text("新建运行集群或对存量集群执行局部增量修改（保留所有未传超卖比、网关配置、出站策略与 Kubeconfig）。", "Create or incrementally patch a runtime cluster without resetting omitted fields."),
		AliasesZH:      []string{"创建集群", "修改运行集群", "更新集群配置"},
		AliasesEN:      []string{"create cluster", "update cluster", "save runtime cluster"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionClusterManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "runtime_list_clusters",
				Family:         "resource",
				Expect:         "present",
				IDFrom:         "result.id",
				ReadIDArgument: "cluster",
				Collection:     true,
			},
		},
		Preview: h.previewSaveClusterAI,
		Execute: h.execSaveClusterAI,
	})

	aitool.Register(reg, aitool.Spec[aiTestClusterInput, map[string]any]{
		Name:           "runtime_test_cluster",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster",
		Summary:        "主动探测运行集群 API Server 连接与权限并记录审计日志",
		Purpose:        aitool.Text("主动测试运行集群 Kubernetes API Server 连通性与凭据可用性，并记录业务审计。", "Actively probe Kubernetes API Server connectivity for a runtime cluster and record an audit log."),
		AliasesZH:      []string{"测试集群连接", "探测集群连通性"},
		AliasesEN:      []string{"test cluster", "ping runtime cluster"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{string(authz.ActionClusterUse)},
		Preview:        h.previewTestClusterAI,
		Execute:        h.execTestClusterAI,
	})

	aitool.Register(reg, aitool.Spec[aiDeleteClusterInput, map[string]any]{
		Name:           "runtime_delete_cluster",
		Domain:         aitool.DomainRuntime,
		Category:       "cluster",
		Summary:        "异步移除运行集群配置",
		Purpose:        aitool.Text("在确认无活跃部署目标和数据卷引用后，异步删除运行集群及其关联凭据。", "Asynchronously delete an unreferenced runtime cluster."),
		AliasesZH:      []string{"删除集群", "移除运行集群"},
		AliasesEN:      []string{"delete cluster", "remove runtime cluster"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionClusterManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "runtime_list_clusters",
				Family:          "resource",
				Expect:          "absent",
				IDFrom:          "result.id",
				ReadIDArgument:  "cluster",
				Collection:      true,
				StatusField:     "deleteStatus",
				PendingStatuses: []string{"active", "deleting"},
				SuccessStatuses: []string{"deleted"},
				FailureStatuses: []string{"delete_failed"},
			},
		},
		Preview: h.previewDeleteClusterAI,
		Execute: h.execDeleteClusterAI,
	})
}

func splitDelimitedTokens(raw string) []string {
	items := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';' || r == '\r'
	})
	out := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func decodeHeaderMapForDTO(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return map[string]string{}
	}
	parsed, err := parseGatewayHeaderMap(trimmed, true)
	if err == nil && parsed != nil {
		return parsed
	}
	var fallbackMap map[string]string
	if json.Unmarshal([]byte(trimmed), &fallbackMap) == nil && fallbackMap != nil {
		return fallbackMap
	}
	return map[string]string{}
}

func structuredRuntimeClusterDTO(cluster model.RuntimeCluster, pressure *runtimeClusterPressureResponse, isAdmin bool) map[string]any {
	projectIDs := cluster.ProjectIDs
	if projectIDs == nil {
		projectIDs = []string{}
	}
	suffixes := cluster.GatewayDomainSuffixes
	if suffixes == nil {
		suffixes = runtimecluster.DecodeGatewayDomainSuffixes(cluster.GatewayDomainSuffixesRaw)
	}
	if suffixes == nil {
		suffixes = []string{}
	}
	dto := map[string]any{
		"id":                            cluster.ID,
		"name":                          cluster.Name,
		"endpoint":                      cluster.Endpoint,
		"scope":                         cluster.Scope,
		"ownerRef":                      cluster.OwnerRef,
		"projectIds":                    projectIDs,
		"kubeconfigSet":                 cluster.KubeconfigSet,
		"isDefault":                     cluster.IsDefault,
		"maxConcurrentBuilds":           cluster.MaxConcurrentBuilds,
		"cpuRequestPercent":             cluster.CPURequestPercent,
		"memoryRequestPercent":          cluster.MemoryRequestPercent,
		"cpuLimitPercent":               cluster.CPULimitPercent,
		"memoryLimitPercent":            cluster.MemoryLimitPercent,
		"gatewayDomainSuffixes":         suffixes,
		"gatewayPublicScheme":           cluster.GatewayPublicScheme,
		"gatewayPublicPort":             cluster.GatewayPublicPort,
		"gatewayControllerType":         cluster.GatewayControllerType,
		"gatewayClassName":              cluster.GatewayClassName,
		"gatewayName":                   cluster.GatewayName,
		"gatewayNamespace":              cluster.GatewayNamespace,
		"gatewayHttpListenerName":       cluster.GatewayHTTPListenerName,
		"gatewayHttpListenerPort":       cluster.GatewayHTTPListenerPort,
		"gatewayHttpsListenerName":      cluster.GatewayHTTPSListenerName,
		"gatewayHttpsListenerPort":      cluster.GatewayHTTPSListenerPort,
		"gatewayTlsSecretName":          cluster.GatewayTLSSecretName,
		"gatewayTlsSecretNamespace":     cluster.GatewayTLSSecretNamespace,
		"gatewayCertIssuerKind":         cluster.GatewayCertIssuerKind,
		"gatewayCertIssuerName":         cluster.GatewayCertIssuerName,
		"gatewayCertificateNamespace":   cluster.GatewayCertificateNamespace,
		"gatewayWildcardCertEnabled":    cluster.GatewayWildcardCertEnabled,
		"gatewayWildcardCertDomain":     cluster.GatewayWildcardCertDomain,
		"gatewayWildcardCertSecretName": cluster.GatewayWildcardCertSecretName,
		"gatewayExternalTlsMode":        cluster.GatewayExternalTLSMode,
		"gatewayForwardedHeadersMode":   cluster.GatewayForwardedHeadersMode,
		"gatewayTrustedProxyCidrs":      splitDelimitedTokens(cluster.GatewayTrustedProxyCIDRs),
		"gatewayDefaultRequestHeaders":  decodeHeaderMapForDTO(cluster.GatewayDefaultRequestHeaders),
		"gatewayDefaultResponseHeaders": decodeHeaderMapForDTO(cluster.GatewayDefaultResponseHeaders),
		"networkBlockedCidrs":           splitDelimitedTokens(cluster.NetworkBlockedCIDRs),
		"networkGatewayNamespace":       cluster.NetworkGatewayNamespace,
		"buildEgressMode":               cluster.BuildEgressMode,
		"buildPrivateEgressCidrs":       splitDelimitedTokens(cluster.BuildPrivateEgressCIDRs),
		"buildPrivateEgressPorts":       splitDelimitedTokens(cluster.BuildPrivateEgressPorts),
		"networkPolicyEnforcement":      cluster.NetworkPolicyEnforcement,
		"deleteStatus":                  cluster.DeleteStatus,
		"status":                        cluster.Status,
		"createdBy":                     cluster.CreatedBy,
		"createdAt":                     cluster.CreatedAt,
		"updatedAt":                     cluster.UpdatedAt,
	}
	if cluster.NetworkPolicyProbeCode != "" {
		dto["networkPolicyProbeCode"] = cluster.NetworkPolicyProbeCode
	}
	if cluster.NetworkPolicyProbedAt != nil {
		dto["networkPolicyProbedAt"] = cluster.NetworkPolicyProbedAt
	}
	if cluster.DeleteStartedAt != nil {
		dto["deleteStartedAt"] = cluster.DeleteStartedAt
	}
	if cluster.DeleteFinishedAt != nil {
		dto["deleteFinishedAt"] = cluster.DeleteFinishedAt
	}
	if cluster.DeleteObservationCode != "" {
		dto["deleteObservationCode"] = cluster.DeleteObservationCode
	}
	if cluster.ObservationCode != "" {
		dto["observationCode"] = cluster.ObservationCode
	}
	if cluster.LastCheckedAt != nil {
		dto["lastCheckedAt"] = cluster.LastCheckedAt
	}
	if pressure != nil {
		pMap := map[string]any{
			"status":        pressure.Status,
			"pressureLevel": pressure.PressureLevel,
			"observedAt":    pressure.ObservedAt,
		}
		if pressure.ObservationCode != "" {
			pMap["observationCode"] = pressure.ObservationCode
		}
		if isAdmin {
			if pressure.PressureScore != nil {
				pMap["pressureScore"] = *pressure.PressureScore
			}
			if pressure.Details != nil {
				pMap["details"] = pressure.Details
			}
		}
		dto["pressure"] = pMap
		dto["pressureLevel"] = pressure.PressureLevel
	}
	return dto
}

func (h *Handler) execListClustersAI(tc *aitool.ToolContext, in aiListClustersInput) (map[string]any, error) {
	tc.SetNoStore()
	user := tc.User
	effectiveProjectRef := ""
	if in.Project != nil && strings.TrimSpace(*in.Project) != "" {
		effectiveProjectRef = strings.TrimSpace(*in.Project)
	} else if strings.TrimSpace(tc.ConversationProjectID) != "" {
		effectiveProjectRef = strings.TrimSpace(tc.ConversationProjectID)
	}

	resolvedProjectID := ""
	if effectiveProjectRef != "" {
		_, proj, err := h.authorizeAIProject(tc, effectiveProjectRef, authz.ActionClusterRead)
		if err != nil {
			return nil, err
		}
		resolvedProjectID = proj.ID
	}

	sortBy := "createdAt"
	if in.SortBy != nil && strings.TrimSpace(*in.SortBy) != "" {
		sortBy = strings.TrimSpace(*in.SortBy)
	}
	sortColMap := map[string]string{"createdAt": "created_at", "name": "name", "scope": "scope"}
	sortCol, ok := sortColMap[sortBy]
	if !ok {
		return nil, aitool.BadRequest("argument_invalid", "sortBy must be createdAt, name, or scope")
	}
	sortOrder := "desc"
	if in.SortOrder != nil && strings.TrimSpace(*in.SortOrder) != "" {
		sortOrder = strings.ToLower(strings.TrimSpace(*in.SortOrder))
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("argument_invalid", "sortOrder must be asc or desc")
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	visibility := projectservice.ListVisibilityRelated
	if resolvedProjectID == "" && strings.TrimSpace(tc.ConversationProjectID) == "" && authz.IsPlatformAdmin(user.Role) {
		visibility = projectservice.ListVisibilityAll
	}

	db := h.dbWithContext(tc.Context())
	sub, rec := newSubGin(tc)
	if in.Search != nil && strings.TrimSpace(*in.Search) != "" {
		q := sub.Request.URL.Query()
		q.Set("search", strings.TrimSpace(*in.Search))
		sub.Request.URL.RawQuery = q.Encode()
	}
	query, visible := h.applyScopedResourceListVisibility(sub, db.Model(&model.RuntimeCluster{}), scopedResourceRuntimeCluster, user, resolvedProjectID, visibility)
	if !visible {
		return nil, abortedToolError(rec, "runtime_cluster.forbidden", "cluster list authorization failed")
	}
	query = applyRuntimeClusterSearch(sub, query, user)
	if in.Cluster != nil && strings.TrimSpace(*in.Cluster) != "" {
		clusterRef := strings.TrimSpace(*in.Cluster)
		query = query.Where("(runtime_clusters.id = ? OR LOWER(runtime_clusters.name) = LOWER(?))", clusterRef, clusterRef)
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var clusters []model.RuntimeCluster
	if err := query.Order(sortCol + " " + sortOrder + ", runtime_clusters.id asc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&clusters).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	for idx := range clusters {
		formatted, fmtErr := h.runtimeClusterResponseForUser(user, clusters[idx], tc.Context())
		if fmtErr != nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "project.authorization_unavailable", fmtErr.Error())
		}
		clusters[idx] = formatted
	}
	h.observeRuntimeClusters(tc.Context(), clusters)

	includePressure := true
	if in.IncludePressure != nil {
		includePressure = *in.IncludePressure
	}
	isAdmin := authz.IsPlatformAdmin(user.Role)
	pressureByID := make(map[string]runtimeClusterPressureResponse, len(clusters))
	if includePressure && len(clusters) > 0 {
		var active []model.RuntimeCluster
		for _, c := range clusters {
			if runtimecluster.IsActive(c) {
				active = append(active, c)
			}
		}
		if len(active) > 0 {
			for _, pr := range h.observeRuntimeClusterPressure(tc.Context(), active, isAdmin) {
				pressureByID[pr.ClusterID] = pr
			}
		}
	}

	items := make([]map[string]any, 0, len(clusters))
	for _, c := range clusters {
		var pPtr *runtimeClusterPressureResponse
		if includePressure {
			if pr, exists := pressureByID[c.ID]; exists {
				prCopy := pr
				pPtr = &prCopy
			} else if !runtimecluster.IsActive(c) {
				unavail := unavailableRuntimeClusterPressure(c.ID, "runtime_cluster."+runtimecluster.NormalizeDeleteStatus(c.DeleteStatus))
				pPtr = &unavail
			}
		}
		items = append(items, structuredRuntimeClusterDTO(c, pPtr, isAdmin))
	}

	out := aitool.PageEnvelope(items, page, pageSize, sortBy, sortOrder, total)
	if resolvedProjectID != "" {
		out["projectId"] = resolvedProjectID
	}
	return out, nil
}

func resolveProjectRefList(db *gorm.DB, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, raw := range refs {
		proj, err := aitool.ResolveProject(db, raw)
		if err != nil {
			return nil, err
		}
		if !seen[proj.ID] {
			seen[proj.ID] = true
			out = append(out, proj.ID)
		}
	}
	return out, nil
}

func mergeStringSet(existing, add, remove []string) []string {
	removeSet := make(map[string]bool, len(remove))
	for _, item := range remove {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			removeSet[trimmed] = true
		}
	}
	seen := make(map[string]bool, len(existing)+len(add))
	out := make([]string, 0, len(existing)+len(add))
	for _, item := range existing {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || removeSet[trimmed] || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	for _, item := range add {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || removeSet[trimmed] || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func mergeDomainSuffixSet(existing, add, remove []string) []string {
	normRemove := make(map[string]bool, len(remove))
	for _, r := range runtimecluster.NormalizeGatewayDomainSuffixes(remove) {
		normRemove[r] = true
	}
	combined := make([]string, 0, len(existing)+len(add))
	for _, e := range runtimecluster.NormalizeGatewayDomainSuffixes(existing) {
		if !normRemove[e] {
			combined = append(combined, e)
		}
	}
	for _, a := range runtimecluster.NormalizeGatewayDomainSuffixes(add) {
		if !normRemove[a] {
			combined = append(combined, a)
		}
	}
	return runtimecluster.NormalizeGatewayDomainSuffixes(combined)
}

func mergeCIDRSet(existingRaw string, add, remove []string, clearAll bool, parseFn func(string) (netip.Prefix, error)) (string, error) {
	if clearAll {
		existingRaw = ""
	}
	removeSet := make(map[netip.Prefix]bool, len(remove))
	for _, r := range remove {
		p, err := parseFn(strings.TrimSpace(r))
		if err != nil {
			return "", err
		}
		removeSet[p.Masked()] = true
	}
	var ordered []netip.Prefix
	seen := make(map[netip.Prefix]bool)
	for _, tok := range splitDelimitedTokens(existingRaw) {
		p, err := parseFn(tok)
		if err != nil {
			continue
		}
		p = p.Masked()
		if !removeSet[p] && !seen[p] {
			seen[p] = true
			ordered = append(ordered, p)
		}
	}
	for _, a := range add {
		p, err := parseFn(strings.TrimSpace(a))
		if err != nil {
			return "", err
		}
		p = p.Masked()
		if !removeSet[p] && !seen[p] {
			seen[p] = true
			ordered = append(ordered, p)
		}
	}
	strs := make([]string, len(ordered))
	for i, p := range ordered {
		strs[i] = p.String()
	}
	return strings.Join(strs, "\n"), nil
}

func mergeGatewayHeaderJSON(existingRaw string, upsert map[string]string, removeKeys []string, allowPrivileged bool, label string) (string, error) {
	if len(upsert) == 0 && len(removeKeys) == 0 {
		return existingRaw, nil
	}
	headers := decodeHeaderMapForDTO(existingRaw)
	for _, k := range removeKeys {
		trimmed := strings.TrimSpace(k)
		for existingKey := range headers {
			if strings.EqualFold(existingKey, trimmed) {
				delete(headers, existingKey)
			}
		}
	}
	for k, v := range upsert {
		key := strings.TrimSpace(k)
		if key == "" {
			return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("%s header key must not be empty", label))
		}
		for existingKey := range headers {
			if existingKey != key && strings.EqualFold(existingKey, key) {
				delete(headers, existingKey)
			}
		}
		headers[key] = v
	}
	encoded := encodeGatewayHeaderMap(headers)
	validated, err := parseGatewayHeaderMap(encoded, allowPrivileged)
	if err != nil {
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid %s: %s", label, err.Error()))
	}
	return encodeGatewayHeaderMap(validated), nil
}

func mergeBuildEgressPorts(existingRaw string, add, remove []string) (string, error) {
	if len(add) == 0 && len(remove) == 0 {
		if strings.TrimSpace(existingRaw) == "" {
			return "443", nil
		}
		return existingRaw, nil
	}
	removeSet := make(map[int]bool, len(remove))
	for _, r := range remove {
		p, err := strconv.Atoi(strings.TrimSpace(r))
		if err != nil || p < 1 || p > 65535 {
			return "", aitool.BadRequest("runtime_cluster.build_private_egress_ports_invalid", fmt.Sprintf("invalid port %q", r))
		}
		removeSet[p] = true
	}
	var ordered []int
	seen := make(map[int]bool)
	for _, tok := range splitDelimitedTokens(existingRaw) {
		p, err := strconv.Atoi(tok)
		if err == nil && p >= 1 && p <= 65535 && !removeSet[p] && !seen[p] {
			seen[p] = true
			ordered = append(ordered, p)
		}
	}
	for _, a := range add {
		p, err := strconv.Atoi(strings.TrimSpace(a))
		if err != nil || p < 1 || p > 65535 {
			return "", aitool.BadRequest("runtime_cluster.build_private_egress_ports_invalid", fmt.Sprintf("invalid port %q", a))
		}
		if !removeSet[p] && !seen[p] {
			seen[p] = true
			ordered = append(ordered, p)
		}
	}
	if len(ordered) > 16 {
		return "", aitool.BadRequest("runtime_cluster.build_private_egress_ports_invalid", "at most 16 build private egress ports are allowed")
	}
	if len(ordered) == 0 {
		return "443", nil
	}
	strs := make([]string, len(ordered))
	for i, p := range ordered {
		strs[i] = strconv.Itoa(p)
	}
	return strings.Join(strs, ","), nil
}

type preparedClusterSave struct {
	isUpdate            bool
	existing            model.RuntimeCluster
	next                model.RuntimeCluster
	flattenedKubeconfig string
	kubeconfigProvided  bool
}

func (h *Handler) prepareClusterSaveForAI(tc *aitool.ToolContext, in aiSaveClusterInput) (preparedClusterSave, error) {
	user := tc.User
	db := h.dbWithContext(tc.Context())
	isAdmin := authz.IsPlatformAdmin(user.Role)

	addProjectIDs, err := resolveProjectRefList(db, in.AddProjects)
	if err != nil {
		return preparedClusterSave{}, err
	}
	removeProjectIDs, err := resolveProjectRefList(db, in.RemoveProjects)
	if err != nil {
		return preparedClusterSave{}, err
	}
	for _, p := range addProjectIDs {
		if containsString(removeProjectIDs, p) {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("project %s cannot appear in both addProjects and removeProjects", p))
		}
	}

	isUpdate := in.Cluster != nil && strings.TrimSpace(*in.Cluster) != ""
	var existing model.RuntimeCluster
	var existingProjectIDs []string
	var next model.RuntimeCluster

	if isUpdate {
		existing, err = h.resolveClusterForAI(tc, *in.Cluster, "", true)
		if err != nil {
			return preparedClusterSave{}, err
		}
		if !runtimecluster.IsActive(existing) {
			return preparedClusterSave{}, aitool.Conflict("runtime_cluster.not_active", "runtime cluster is not active")
		}
		if err := h.requireClusterManageForAI(tc, existing, "无权维护该运行集群"); err != nil {
			return preparedClusterSave{}, err
		}
		existingProjectIDs = h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, existing.ID, tc.Context())
		if err := enforceScopedClusterSessionInvariant(tc, existing.Scope, existing.OwnerRef, existingProjectIDs); err != nil {
			return preparedClusterSave{}, err
		}
		if in.Kubeconfig != nil && strings.TrimSpace(*in.Kubeconfig) != "" && !h.canReplaceRuntimeClusterKubeconfig(user, existing) {
			return preparedClusterSave{}, aitool.Forbidden("runtime_cluster.kubeconfig_forbidden", "only the cluster creator or a platform administrator can replace kubeconfig")
		}
		next = existing
		next.ProjectIDs = append([]string(nil), existingProjectIDs...)
		next.GatewayDomainSuffixes = runtimecluster.DecodeGatewayDomainSuffixes(existing.GatewayDomainSuffixesRaw)
	} else {
		if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "name is required when creating a runtime cluster")
		}
		if len(removeProjectIDs) > 0 {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "removeProjects cannot be used when creating a new runtime cluster")
		}
		defaults := resourcepolicy.Default()
		next = model.RuntimeCluster{
			ID:                          id.New("clu"),
			Scope:                       "user",
			OwnerRef:                    user.ID,
			MaxConcurrentBuilds:         defaultClusterBuildConcurrency,
			CPURequestPercent:           defaults.CPURequestPercent,
			MemoryRequestPercent:        defaults.MemoryRequestPercent,
			CPULimitPercent:             defaults.CPULimitPercent,
			MemoryLimitPercent:          defaults.MemoryLimitPercent,
			GatewayPublicScheme:         "http",
			GatewayPublicPort:           80,
			GatewayControllerType:       "traefik",
			GatewayClassName:            "traefik",
			GatewayName:                 "luna-gateway",
			GatewayNamespace:            "kube-system",
			GatewayHTTPListenerName:     "web",
			GatewayHTTPListenerPort:     8080,
			GatewayHTTPSListenerName:    "websecure",
			GatewayHTTPSListenerPort:    8443,
			GatewayCertIssuerKind:       "ClusterIssuer",
			GatewayExternalTLSMode:      "none",
			GatewayForwardedHeadersMode: "preserve",
			BuildEgressMode:             runtimecluster.BuildEgressRestricted,
			BuildPrivateEgressPorts:     "443",
			DeleteStatus:                "active",
			CreatedBy:                   user.ID,
		}
		if len(addProjectIDs) > 0 {
			next.Scope = "project"
		}
	}

	if in.Name != nil {
		trimmedName := strings.TrimSpace(*in.Name)
		if trimmedName == "" {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "name must not be empty")
		}
		next.Name = trimmedName
	}
	if in.Endpoint != nil {
		next.Endpoint = strings.TrimSpace(*in.Endpoint)
	}

	targetScope := next.Scope
	if in.Scope != nil && strings.TrimSpace(*in.Scope) != "" {
		targetScope = strings.ToLower(strings.TrimSpace(*in.Scope))
	}
	if targetScope != "project" && (len(addProjectIDs) > 0 || len(removeProjectIDs) > 0) {
		return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "addProjects/removeProjects are only valid when scope is 'project'")
	}

	var candidateProjectIDs []string
	if targetScope == "project" {
		candidateProjectIDs = mergeStringSet(existingProjectIDs, addProjectIDs, removeProjectIDs)
		if len(candidateProjectIDs) == 0 {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "scope=project requires at least one authorized project")
		}
	}
	candidateOwnerRef := user.ID
	if isUpdate && targetScope == "user" && existing.Scope == "user" && strings.TrimSpace(existing.OwnerRef) != "" {
		candidateOwnerRef = existing.OwnerRef
	}

	sub, rec := newSubGin(tc)
	normScope, normOwnerRef, normProjectIDs, ok := h.normalizeScopedOwnerWithProjects(sub, user, targetScope, candidateOwnerRef, candidateProjectIDs, "只有平台管理员可以维护全局运行集群")
	if !ok {
		return preparedClusterSave{}, abortedToolError(rec, "runtime_cluster.scope_forbidden", "cluster scope validation failed")
	}
	if err := enforceScopedClusterSessionInvariant(tc, normScope, normOwnerRef, normProjectIDs); err != nil {
		return preparedClusterSave{}, err
	}
	next.Scope = normScope
	next.OwnerRef = normOwnerRef
	next.ProjectIDs = normProjectIDs

	if in.IsDefault != nil {
		if *in.IsDefault && next.Scope != "global" {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "isDefault=true is only allowed for global runtime clusters")
		}
		next.IsDefault = *in.IsDefault
	} else if next.Scope != "global" {
		next.IsDefault = false
	}

	if in.MaxConcurrentBuilds != nil {
		if *in.MaxConcurrentBuilds < 1 {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "maxConcurrentBuilds must be >= 1")
		}
		next.MaxConcurrentBuilds = normalizeBuildConcurrency(*in.MaxConcurrentBuilds, defaultClusterBuildConcurrency)
	}

	flattenedKubeconfig := ""
	kubeconfigProvided := false
	if in.Kubeconfig != nil {
		if strings.TrimSpace(*in.Kubeconfig) == "" {
			return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "kubeconfig must not be an empty string; omit kubeconfig to keep existing credentials")
		}
		flat, kErr := flattenKubeconfig(*in.Kubeconfig)
		if kErr != nil {
			return preparedClusterSave{}, aitool.BadRequest("runtime_cluster.invalid_kubeconfig", kErr.Error())
		}
		flattenedKubeconfig = flat
		kubeconfigProvided = true
	}

	if in.ResourcePolicy != nil {
		pol := resourcepolicy.Policy{
			CPURequestPercent:    next.CPURequestPercent,
			MemoryRequestPercent: next.MemoryRequestPercent,
			CPULimitPercent:      next.CPULimitPercent,
			MemoryLimitPercent:   next.MemoryLimitPercent,
		}
		if in.ResourcePolicy.CPURequestPercent != nil {
			pol.CPURequestPercent = *in.ResourcePolicy.CPURequestPercent
		}
		if in.ResourcePolicy.MemoryRequestPercent != nil {
			pol.MemoryRequestPercent = *in.ResourcePolicy.MemoryRequestPercent
		}
		if in.ResourcePolicy.CPULimitPercent != nil {
			pol.CPULimitPercent = *in.ResourcePolicy.CPULimitPercent
		}
		if in.ResourcePolicy.MemoryLimitPercent != nil {
			pol.MemoryLimitPercent = *in.ResourcePolicy.MemoryLimitPercent
		}
		if polErr := pol.Validate(); polErr != nil {
			return preparedClusterSave{}, aitool.BadRequest("runtime.resource_policy_invalid", polErr.Error())
		}
		next.CPURequestPercent = pol.CPURequestPercent
		next.MemoryRequestPercent = pol.MemoryRequestPercent
		next.CPULimitPercent = pol.CPULimitPercent
		next.MemoryLimitPercent = pol.MemoryLimitPercent
	}

	if gw := in.GatewayConfig; gw != nil {
		if len(gw.AddDomainSuffixes) > 0 || len(gw.RemoveDomainSuffixes) > 0 {
			next.GatewayDomainSuffixes = mergeDomainSuffixSet(next.GatewayDomainSuffixes, gw.AddDomainSuffixes, gw.RemoveDomainSuffixes)
			next.GatewayDomainSuffixesRaw = runtimecluster.EncodeGatewayDomainSuffixes(next.GatewayDomainSuffixes)
		}
		if gw.PublicScheme != nil {
			scheme := normalizeGatewayPublicScheme(*gw.PublicScheme)
			schemeChanged := scheme != next.GatewayPublicScheme
			next.GatewayPublicScheme = scheme
			if gw.PublicPort == nil && (!isUpdate || (schemeChanged && (next.GatewayPublicPort == 80 || next.GatewayPublicPort == 443))) {
				next.GatewayPublicPort = normalizeGatewayPublicPort(0, scheme)
			}
		}
		if gw.PublicPort != nil {
			next.GatewayPublicPort = normalizeGatewayPublicPort(*gw.PublicPort, next.GatewayPublicScheme)
		}
		if gw.ControllerType != nil {
			next.GatewayControllerType = normalizeGatewayControllerType(*gw.ControllerType)
		}
		if gw.ClassName != nil && strings.TrimSpace(*gw.ClassName) != "" {
			next.GatewayClassName = strings.TrimSpace(*gw.ClassName)
		}
		if gw.GatewayName != nil && dnsLabelName(*gw.GatewayName) != "" {
			next.GatewayName = dnsLabelName(*gw.GatewayName)
		}
		if gw.GatewayNamespace != nil && dnsLabelName(*gw.GatewayNamespace) != "" {
			next.GatewayNamespace = dnsLabelName(*gw.GatewayNamespace)
		}
		if gw.HTTPListenerName != nil && dnsLabelName(*gw.HTTPListenerName) != "" {
			next.GatewayHTTPListenerName = dnsLabelName(*gw.HTTPListenerName)
		}
		if gw.HTTPListenerPort != nil {
			next.GatewayHTTPListenerPort = normalizePort(*gw.HTTPListenerPort, 8080)
		}
		if gw.HTTPSListenerName != nil && dnsLabelName(*gw.HTTPSListenerName) != "" {
			next.GatewayHTTPSListenerName = dnsLabelName(*gw.HTTPSListenerName)
		}
		if gw.HTTPSListenerPort != nil {
			next.GatewayHTTPSListenerPort = normalizePort(*gw.HTTPSListenerPort, 8443)
		}
		if gw.ClearTLSSecret != nil && *gw.ClearTLSSecret {
			next.GatewayTLSSecretName = ""
			next.GatewayTLSSecretNamespace = ""
		} else {
			if gw.TLSSecretName != nil {
				next.GatewayTLSSecretName = dnsLabelName(*gw.TLSSecretName)
			}
			if gw.TLSSecretNamespace != nil {
				next.GatewayTLSSecretNamespace = dnsLabelName(*gw.TLSSecretNamespace)
			}
		}
		if gw.CertIssuerKind != nil {
			next.GatewayCertIssuerKind = normalizeGatewayCertIssuerKind(*gw.CertIssuerKind)
		}
		if gw.ClearCertIssuerName != nil && *gw.ClearCertIssuerName {
			next.GatewayCertIssuerName = ""
		} else if gw.CertIssuerName != nil {
			next.GatewayCertIssuerName = dnsLabelName(*gw.CertIssuerName)
		}
		if gw.ClearCertificateNamespace != nil && *gw.ClearCertificateNamespace {
			next.GatewayCertificateNamespace = ""
		} else if gw.CertificateNamespace != nil {
			next.GatewayCertificateNamespace = dnsLabelName(*gw.CertificateNamespace)
		}
		if gw.ClearWildcardCert != nil && *gw.ClearWildcardCert {
			next.GatewayWildcardCertEnabled = false
			next.GatewayWildcardCertDomain = ""
			next.GatewayWildcardCertSecretName = ""
		} else {
			if gw.WildcardCertEnabled != nil {
				next.GatewayWildcardCertEnabled = *gw.WildcardCertEnabled
			}
			if gw.WildcardCertDomain != nil {
				next.GatewayWildcardCertDomain = runtimecluster.NormalizeGatewayDomainSuffix(*gw.WildcardCertDomain)
			}
			if gw.WildcardCertSecretName != nil {
				next.GatewayWildcardCertSecretName = dnsLabelName(*gw.WildcardCertSecretName)
			}
		}
		if gw.ExternalTLSMode != nil {
			next.GatewayExternalTLSMode = normalizeGatewayExternalTLSMode(*gw.ExternalTLSMode)
		}
		if gw.ForwardedHeadersMode != nil {
			next.GatewayForwardedHeadersMode = normalizeGatewayForwardedHeadersMode(*gw.ForwardedHeadersMode)
		}
		if (gw.ClearTrustedProxyCIDRs != nil && *gw.ClearTrustedProxyCIDRs) || len(gw.AddTrustedProxyCIDRs) > 0 || len(gw.RemoveTrustedProxyCIDRs) > 0 {
			mergedTrusted, cidrErr := mergeCIDRSet(
				next.GatewayTrustedProxyCIDRs,
				gw.AddTrustedProxyCIDRs,
				gw.RemoveTrustedProxyCIDRs,
				gw.ClearTrustedProxyCIDRs != nil && *gw.ClearTrustedProxyCIDRs,
				netip.ParsePrefix,
			)
			if cidrErr != nil {
				return preparedClusterSave{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid trusted proxy CIDR: %s", cidrErr.Error()))
			}
			next.GatewayTrustedProxyCIDRs = mergedTrusted
		}
		reqHdrJSON, reqErr := mergeGatewayHeaderJSON(
			next.GatewayDefaultRequestHeaders,
			gw.UpsertDefaultRequestHeaders,
			gw.RemoveDefaultRequestHeaderKeys,
			isAdmin,
			"defaultRequestHeaders",
		)
		if reqErr != nil {
			return preparedClusterSave{}, reqErr
		}
		next.GatewayDefaultRequestHeaders = reqHdrJSON

		respHdrJSON, respErr := mergeGatewayHeaderJSON(
			next.GatewayDefaultResponseHeaders,
			gw.UpsertDefaultResponseHeaders,
			gw.RemoveDefaultResponseHeaderKeys,
			isAdmin,
			"defaultResponseHeaders",
		)
		if respErr != nil {
			return preparedClusterSave{}, respErr
		}
		next.GatewayDefaultResponseHeaders = respHdrJSON
	}

	if ns := in.NetworkSecurity; ns != nil {
		if ns.ClearNetworkGatewayNamespace != nil && *ns.ClearNetworkGatewayNamespace {
			next.NetworkGatewayNamespace = ""
		} else if ns.NetworkGatewayNamespace != nil {
			trimmed := strings.TrimSpace(*ns.NetworkGatewayNamespace)
			if trimmed == "" {
				return preparedClusterSave{}, aitool.BadRequest("argument_invalid", "use clearNetworkGatewayNamespace=true to clear networkGatewayNamespace")
			}
			if len(validation.IsDNS1123Label(trimmed)) > 0 {
				return preparedClusterSave{}, aitool.BadRequest("runtime_cluster.network_gateway_namespace_invalid", "networkGatewayNamespace must be a DNS-1123 label")
			}
			next.NetworkGatewayNamespace = trimmed
		}
		if len(ns.AddNetworkBlockedCIDRs) > 0 || len(ns.RemoveNetworkBlockedCIDRs) > 0 {
			mergedBlocked, cErr := mergeCIDRSet(next.NetworkBlockedCIDRs, ns.AddNetworkBlockedCIDRs, ns.RemoveNetworkBlockedCIDRs, false, kubeprovider.ParseNetworkCIDR)
			if cErr != nil {
				return preparedClusterSave{}, aitool.BadRequest("runtime_cluster.network_blocked_cidrs_invalid", cErr.Error())
			}
			normalizedBlocked, nErr := normalizeNetworkBlockedCIDRs(mergedBlocked)
			if nErr != nil {
				return preparedClusterSave{}, aitool.BadRequest("runtime_cluster.network_blocked_cidrs_invalid", nErr.Error())
			}
			next.NetworkBlockedCIDRs = normalizedBlocked
		}
		if ns.BuildEgressMode != nil {
			mode := strings.TrimSpace(*ns.BuildEgressMode)
			if mode != runtimecluster.BuildEgressRestricted && mode != runtimecluster.BuildEgressPermissive {
				return preparedClusterSave{}, aitool.BadRequest("runtime_cluster.build_egress_mode_invalid", "buildEgressMode must be restricted or permissive")
			}
			next.BuildEgressMode = mode
		}
		if len(ns.AddBuildPrivateEgressCIDRs) > 0 || len(ns.RemoveBuildPrivateEgressCIDRs) > 0 {
			mergedPrivate, cErr := mergeCIDRSet(next.BuildPrivateEgressCIDRs, ns.AddBuildPrivateEgressCIDRs, ns.RemoveBuildPrivateEgressCIDRs, false, kubeprovider.ParseNetworkCIDR)
			if cErr != nil {
				return preparedClusterSave{}, aitool.BadRequest("runtime_cluster.build_private_egress_cidrs_invalid", cErr.Error())
			}
			normalizedPrivate, nErr := normalizeBuildPrivateEgressCIDRs(mergedPrivate)
			if nErr != nil {
				return preparedClusterSave{}, aitool.BadRequest("runtime_cluster.build_private_egress_cidrs_invalid", nErr.Error())
			}
			next.BuildPrivateEgressCIDRs = normalizedPrivate
		}
		if len(ns.AddBuildPrivateEgressPorts) > 0 || len(ns.RemoveBuildPrivateEgressPorts) > 0 {
			mergedPorts, pErr := mergeBuildEgressPorts(next.BuildPrivateEgressPorts, ns.AddBuildPrivateEgressPorts, ns.RemoveBuildPrivateEgressPorts)
			if pErr != nil {
				return preparedClusterSave{}, pErr
			}
			next.BuildPrivateEgressPorts = mergedPorts
		}
	}

	next.GatewayDomainSuffixesRaw = runtimecluster.EncodeGatewayDomainSuffixes(next.GatewayDomainSuffixes)

	// Dry-run check in update mode so R12 preview rejects 409 in-use scope narrowing before approval.
	if isUpdate {
		if err := h.previewCheckClusterScopeNarrowing(db, existing, next); err != nil {
			return preparedClusterSave{}, err
		}
	}

	return preparedClusterSave{
		isUpdate:            isUpdate,
		existing:            existing,
		next:                next,
		flattenedKubeconfig: flattenedKubeconfig,
		kubeconfigProvided:  kubeconfigProvided,
	}, nil
}

func (h *Handler) previewCheckClusterScopeNarrowing(db *gorm.DB, previous, next model.RuntimeCluster) error {
	if previous.Scope == "global" && next.Scope != "global" {
		var otherDefaultCount int64
		if err := runtimecluster.ActiveScope(db).Model(&model.RuntimeCluster{}).
			Where("scope = ? AND id <> ?", "global", next.ID).
			Count(&otherDefaultCount).Error; err != nil {
			return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if otherDefaultCount == 0 {
			var implicitTargetCount int64
			if err := db.Model(&model.DeploymentTarget{}).
				Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
				Where("cluster_id = ''").
				Count(&implicitTargetCount).Error; err != nil {
				return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if implicitTargetCount > 0 {
				return aitool.Conflict("runtime_cluster.in_use", "runtime cluster is still referenced by deployment targets inheriting the default global cluster")
			}
		}
	}
	if next.Scope == "global" {
		return nil
	}
	referenced := map[string]bool{}
	var targetProjs []string
	_ = db.Model(&model.DeploymentTarget{}).
		Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
		Where("deployment_targets.cluster_id = ?", next.ID).
		Distinct("deployment_targets.project_id").
		Pluck("deployment_targets.project_id", &targetProjs).Error
	for _, p := range targetProjs {
		referenced[p] = true
	}
	var volProjs []string
	_ = db.Model(&model.ProjectVolume{}).
		Joins("join projects on projects.id = project_volumes.project_id and projects.deleted_at is null").
		Where("project_volumes.cluster_id = ?", next.ID).
		Distinct("project_volumes.project_id").
		Pluck("project_volumes.project_id", &volProjs).Error
	for _, p := range volProjs {
		referenced[p] = true
	}
	for projID := range referenced {
		switch next.Scope {
		case "project":
			if !containsString(next.ProjectIDs, projID) {
				return aitool.Conflict("runtime_cluster.in_use", fmt.Sprintf("project %s still has active deployment targets or volumes on this cluster", projID))
			}
		case "user":
			var memberCount int64
			_ = db.Model(&model.ProjectMember{}).Where("project_id = ? AND user_id = ?", projID, next.OwnerRef).Count(&memberCount).Error
			if memberCount == 0 {
				return aitool.Conflict("runtime_cluster.in_use", fmt.Sprintf("project %s still has active resources on this cluster", projID))
			}
		}
	}
	return nil
}

func (h *Handler) previewSaveClusterAI(tc *aitool.ToolContext, in aiSaveClusterInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareClusterSaveForAI(tc, in)
	if err != nil {
		return nil, err
	}
	if prep.kubeconfigProvided && !h.secrets.Available() {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "runtime_cluster.secret_store_unavailable", "secret store is unavailable")
	}
	var sensitiveKeys []string
	if prep.kubeconfigProvided {
		sensitiveKeys = []string{"kubeconfig"}
	}
	if !prep.isUpdate {
		changes := []aitool.PreviewChange{
			{Field: "name", After: prep.next.Name, Action: "create"},
			{Field: "scope", After: prep.next.Scope, Action: "create"},
		}
		if len(prep.next.ProjectIDs) > 0 {
			changes = append(changes, aitool.PreviewChange{Field: "projectIds", After: prep.next.ProjectIDs, Action: "create"})
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("创建运行集群 %s（scope=%s）", prep.next.Name, prep.next.Scope),
				TargetResource: "runtime_cluster",
				ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
				Changes:        changes,
				SensitiveKeys:  sensitiveKeys,
			},
		}, nil
	}

	var changes []aitool.PreviewChange
	if prep.next.Name != prep.existing.Name {
		changes = append(changes, aitool.PreviewChange{Field: "name", Before: prep.existing.Name, After: prep.next.Name, Action: "update"})
	}
	if prep.next.Scope != prep.existing.Scope {
		changes = append(changes, aitool.PreviewChange{Field: "scope", Before: prep.existing.Scope, After: prep.next.Scope, Action: "update"})
	}
	if prep.next.Endpoint != prep.existing.Endpoint {
		changes = append(changes, aitool.PreviewChange{Field: "endpoint", Before: prep.existing.Endpoint, After: prep.next.Endpoint, Action: "update"})
	}
	if prep.next.BuildEgressMode != prep.existing.BuildEgressMode {
		changes = append(changes, aitool.PreviewChange{Field: "buildEgressMode", Before: prep.existing.BuildEgressMode, After: prep.next.BuildEgressMode, Action: "update"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("增量修改运行集群 %s（%s）", prep.existing.Name, prep.existing.ID),
			TargetResource: "runtime_cluster",
			TargetID:       prep.existing.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes:        changes,
			SensitiveKeys:  sensitiveKeys,
		},
	}, nil
}

func (h *Handler) execSaveClusterAI(tc *aitool.ToolContext, in aiSaveClusterInput) (map[string]any, error) {
	user := tc.User
	prep, err := h.prepareClusterSaveForAI(tc, in)
	if err != nil {
		return nil, err
	}
	if prep.kubeconfigProvided && !h.secrets.Available() {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "runtime_cluster.secret_store_unavailable", "secret store is unavailable")
	}

	var saved model.RuntimeCluster
	if !prep.isUpdate {
		auditMeta := runtimeClusterSafeAuditMetadata(model.RuntimeCluster{}, prep.next, prep.kubeconfigProvided)
		persisted, createErr := h.createRuntimeClusterWithDefault(prep.next, prep.flattenedKubeconfig, user.ID, tc.Context())
		if createErr != nil {
			auditWithSafeMetadata(h, user.ID, "runtime_cluster.create", prep.next.ID, false, runtimeClusterPersistenceAuditMessage(createErr), auditMeta, tc.Context())
			return nil, aitool.BadRequest("runtime_cluster.create_failed", createErr.Error())
		}
		auditWithSafeMetadata(h, user.ID, "runtime_cluster.create", persisted.ID, true, "", auditMeta, tc.Context())
		h.host.EnqueueNetworkPolicyProbe(tc.Context(), persisted.ID)
		saved = persisted
	} else {
		previous, updated, grantsPruned, networkChanged, updateErr := h.updateRuntimeClusterWithDefault(
			prep.existing.ID,
			prep.next,
			prep.flattenedKubeconfig,
			prep.kubeconfigProvided,
			user.ID,
			tc.Context(),
		)
		if updateErr != nil {
			if previous.ID == "" {
				previous = prep.existing
			}
			auditMeta := runtimeClusterSafeAuditMetadata(previous, prep.next, prep.kubeconfigProvided)
			auditWithSafeMetadata(h, user.ID, "runtime_cluster.update", prep.existing.ID, false, runtimeClusterPersistenceAuditMessage(updateErr), auditMeta, tc.Context())
			if errors.Is(updateErr, errRuntimeClusterNotActive) {
				return nil, aitool.Conflict("runtime_cluster.not_active", "runtime cluster is not active")
			}
			if errors.Is(updateErr, errRuntimeClusterInUse) {
				return nil, aitool.Conflict("runtime_cluster.in_use", "runtime cluster is referenced by active deployment targets or project volumes")
			}
			return nil, aitool.BadRequest("runtime_cluster.update_failed", updateErr.Error())
		}
		auditMeta := runtimeClusterSafeAuditMetadata(previous, updated, prep.kubeconfigProvided)
		auditWithSafeMetadata(h, user.ID, "runtime_cluster.update", updated.ID, true, "", auditMeta, tc.Context())
		if networkChanged || grantsPruned {
			h.host.EnqueueNetworkPolicyReconcile(tc.Context(), updated.ID)
		}
		if prep.kubeconfigProvided {
			h.host.EnqueueNetworkPolicyProbe(tc.Context(), updated.ID)
		}
		saved = updated
	}

	saved, err = h.runtimeClusterResponseForUser(user, saved, tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "project.authorization_unavailable", err.Error())
	}
	saved = h.observeRuntimeCluster(tc.Context(), saved)
	dto := structuredRuntimeClusterDTO(saved, nil, authz.IsPlatformAdmin(user.Role))
	if convProj := strings.TrimSpace(tc.ConversationProjectID); convProj != "" {
		dto["projectId"] = convProj
	}
	return dto, nil
}

func (h *Handler) validateClusterUsableInSessionForAI(tc *aitool.ToolContext, cluster model.RuntimeCluster) error {
	if !h.canUseScopedResourceByID(tc.User, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, tc.Context()) {
		return aitool.Forbidden("runtime_cluster.forbidden", "you do not have permission to test this runtime cluster")
	}
	convProjID := strings.TrimSpace(tc.ConversationProjectID)
	if convProjID == "" {
		return nil
	}
	sub, rec := newSubGin(tc)
	if _, _, ok := h.authorizeProjectByID(sub, convProjID, authz.ActionClusterUse); !ok {
		return abortedToolError(rec, "project.forbidden", "project authorization failed")
	}
	boundProjectIDs := h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, cluster.ID, tc.Context())
	switch normalizeOwnerScope(cluster.Scope) {
	case "global":
		return nil
	case "project":
		if containsString(boundProjectIDs, convProjID) {
			return nil
		}
	case "user":
		if strings.TrimSpace(cluster.OwnerRef) == strings.TrimSpace(tc.User.ID) {
			return nil
		}
	}
	return projectScopeMismatchError("runtime cluster is not usable within the bound conversation project")
}

func (h *Handler) previewTestClusterAI(tc *aitool.ToolContext, in aiTestClusterInput) (*aitool.PreviewResult, error) {
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, strings.TrimSpace(tc.ConversationProjectID), false)
	if err != nil {
		return nil, err
	}
	if err := h.validateClusterUsableInSessionForAI(tc, cluster); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cluster.KubeconfigRef) == "" {
		return nil, aitool.BadRequest("runtime_cluster.kubeconfig_not_configured", "runtime cluster does not have a kubeconfig configured")
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("主动探测运行集群 %s（%s）API Server 连通性", cluster.Name, cluster.ID),
			TargetResource: "runtime_cluster",
			TargetID:       cluster.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
		},
	}, nil
}

func (h *Handler) execTestClusterAI(tc *aitool.ToolContext, in aiTestClusterInput) (map[string]any, error) {
	tc.SetNoStore()
	user := tc.User
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, strings.TrimSpace(tc.ConversationProjectID), false)
	if err != nil {
		return nil, err
	}
	if err := h.validateClusterUsableInSessionForAI(tc, cluster); err != nil {
		return nil, err
	}
	cluster, err = h.runtimeClusterResponseForUser(user, cluster, tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "project.authorization_unavailable", err.Error())
	}
	cluster = h.observeRuntimeCluster(tc.Context(), cluster)
	switch cluster.Status {
	case observation.StatusNotConfigured:
		h.auditWithContext(user.ID, "runtime_cluster.test", cluster.ID, false, cluster.ObservationCode, tc.Context())
		return nil, aitool.BadRequest(cluster.ObservationCode, "runtime cluster does not have a usable kubeconfig")
	case observation.StatusUnavailable:
		h.auditWithContext(user.ID, "runtime_cluster.test", cluster.ID, false, cluster.ObservationCode, tc.Context())
		return nil, aitool.NewError(http.StatusBadGateway, cluster.ObservationCode, "failed to connect to runtime cluster API server")
	default:
		if canManage, mErr := h.canInspectScopedResourceConfigByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, tc.Context()); mErr == nil && canManage {
			h.host.EnqueueNetworkPolicyProbe(tc.Context(), cluster.ID)
		}
		h.auditWithContext(user.ID, "runtime_cluster.test", cluster.ID, true, "ready", tc.Context())
		dto := structuredRuntimeClusterDTO(cluster, nil, authz.IsPlatformAdmin(user.Role))
		if convProj := strings.TrimSpace(tc.ConversationProjectID); convProj != "" {
			dto["projectId"] = convProj
		}
		return dto, nil
	}
}

func (h *Handler) previewDeleteClusterAI(tc *aitool.ToolContext, in aiDeleteClusterInput) (*aitool.PreviewResult, error) {
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, "", true)
	if err != nil {
		return nil, err
	}
	if err := h.requireClusterManageForAI(tc, cluster, "无权维护该运行集群"); err != nil {
		return nil, err
	}
	boundProjectIDs := h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, cluster.ID, tc.Context())
	if err := enforceScopedClusterSessionInvariant(tc, cluster.Scope, cluster.OwnerRef, boundProjectIDs); err != nil {
		return nil, err
	}
	if !h.deleteStatusCanStart(cluster.DeleteStatus) {
		return nil, aitool.Conflict("runtime_cluster.delete_in_progress", "runtime cluster deletion is already in progress")
	}
	db := h.dbWithContext(tc.Context())
	targetCount, volumeCount, err := runtimeClusterReferenceCounts(db, cluster)
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if targetCount > 0 || volumeCount > 0 {
		e := aitool.Conflict("runtime_cluster.in_use", "runtime cluster is still referenced by active deployment targets or project volumes")
		e.Details = map[string]any{
			"deploymentTargetCount": targetCount,
			"projectVolumeCount":    volumeCount,
		}
		return nil, e
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("异步移除运行集群 %s（%s）", cluster.Name, cluster.ID),
			TargetResource: "runtime_cluster",
			TargetID:       cluster.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "deleteStatus", Before: cluster.DeleteStatus, After: "deleting", Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) execDeleteClusterAI(tc *aitool.ToolContext, in aiDeleteClusterInput) (map[string]any, error) {
	user := tc.User
	cluster, err := h.resolveClusterForAI(tc, in.Cluster, "", true)
	if err != nil {
		return nil, err
	}
	if err := h.requireClusterManageForAI(tc, cluster, "无权维护该运行集群"); err != nil {
		return nil, err
	}
	boundProjectIDs := h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, cluster.ID, tc.Context())
	if err := enforceScopedClusterSessionInvariant(tc, cluster.Scope, cluster.OwnerRef, boundProjectIDs); err != nil {
		return nil, err
	}
	if !h.deleteStatusCanStart(cluster.DeleteStatus) {
		return nil, aitool.Conflict("runtime_cluster.delete_in_progress", "runtime cluster deletion is already in progress")
	}
	db := h.dbWithContext(tc.Context())
	if err := db.Transaction(func(tx *gorm.DB) error {
		var locked model.RuntimeCluster
		if lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", cluster.ID).Error; lockErr != nil {
			return lockErr
		}
		targetCount, volumeCount, refErr := runtimeClusterReferenceCounts(tx, locked)
		if refErr != nil {
			return refErr
		}
		if targetCount > 0 || volumeCount > 0 {
			return errRuntimeClusterInUse
		}
		return h.markResourceDeleting(tx, &model.RuntimeCluster{}, locked.ID)
	}); err != nil {
		if errors.Is(err, errRuntimeClusterInUse) {
			return nil, aitool.Conflict("runtime_cluster.in_use", "runtime cluster is referenced by active deployment targets or project volumes")
		}
		if h.host.ResourceDeleteAlreadyStarted(err) {
			return nil, aitool.Conflict("runtime_cluster.delete_in_progress", "runtime cluster deletion is already in progress")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "runtime_cluster.delete_failed", err.Error())
	}
	if !h.enqueueResourceCleanup(tc.Context(), "runtime_cluster", cluster.ID, "", user.ID) {
		_ = h.markResourceDeleteFailed(db, &model.RuntimeCluster{}, cluster.ID, "资源清理任务投递失败，请稍后重试")
		h.auditWithContext(user.ID, "runtime_cluster.delete", cluster.ID, false, "cleanup_enqueue_failed", tc.Context())
		return nil, aitool.NewError(http.StatusServiceUnavailable, "runtime_cluster.cleanup_enqueue_failed", "failed to enqueue runtime cluster cleanup")
	}
	h.auditWithContext(user.ID, "runtime_cluster.delete", cluster.ID, true, "cleanup_queued", tc.Context())
	resp := map[string]any{
		"id":           cluster.ID,
		"deleteStatus": "deleting",
	}
	if convProj := strings.TrimSpace(tc.ConversationProjectID); convProj != "" {
		resp["projectId"] = convProj
	}
	return resp, nil
}
