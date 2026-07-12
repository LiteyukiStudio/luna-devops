package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

type gatewayRouteInput struct {
	ApplicationID          string `json:"applicationId" binding:"required"`
	EnvironmentID          string `json:"environmentId"`
	DeploymentTargetID     string `json:"deploymentTargetId" binding:"required"`
	Host                   string `json:"host"`
	DomainSuffix           string `json:"domainSuffix"`
	Path                   string `json:"path"`
	ServicePort            int    `json:"servicePort"`
	TLSMode                string `json:"tlsMode"`
	DNSStatus              string `json:"dnsStatus"`
	Status                 string `json:"status"`
	Enabled                *bool  `json:"enabled"`
	IsDefault              bool   `json:"isDefault"`
	ParentGatewayName      string `json:"parentGatewayName"`
	ParentGatewayNamespace string `json:"parentGatewayNamespace"`
	SectionName            string `json:"sectionName"`
	PathMatchType          string `json:"pathMatchType"`
	RequestHeaders         string `json:"requestHeaders"`
	ResponseHeaders        string `json:"responseHeaders"`
	URLRewrite             string `json:"urlRewrite"`
	RequestRedirect        string `json:"requestRedirect"`
	BackendWeight          int    `json:"backendWeight"`
	HostnameAliases        string `json:"hostnameAliases"`
}

type gatewayRouteAdvancedConfig struct {
	ParentGatewayName      string
	ParentGatewayNamespace string
	SectionName            string
	PathMatchType          string
	RequestHeaders         string
	ResponseHeaders        string
	URLRewrite             string
	RequestRedirect        string
	BackendWeight          int
	HostnameAliases        string
}

func (h *Handlers) gatewayRouteFromInput(ctx *gin.Context, project model.Project, user model.User, creatorID string, input gatewayRouteInput, routeID string) (model.GatewayRoute, bool) {
	target, application, environment, cluster, ok := h.gatewayRouteTargetContext(ctx, project.ID, input)
	if !ok {
		return model.GatewayRoute{}, false
	}
	domainSuffix, ok := h.gatewayRouteDomainSuffix(ctx, input.DomainSuffix, cluster)
	if !ok {
		return model.GatewayRoute{}, false
	}
	host := h.normalizeGatewayHost(input.Host, cluster, domainSuffix)
	if host == "" {
		host = h.defaultGatewayHost(project, target.Stage, application.Slug, cluster, domainSuffix)
	}
	if host == "" {
		writeError(ctx, http.StatusBadRequest, "请输入域名或选择部署配置")
		return model.GatewayRoute{}, false
	}
	if h.gatewayHostExists(host, routeID) {
		writeError(ctx, http.StatusBadRequest, "域名已被占用")
		return model.GatewayRoute{}, false
	}
	servicePort := fallbackInt(input.ServicePort, deploymentTargetServicePort(target))
	if servicePort <= 0 || servicePort > 65535 {
		writeError(ctx, http.StatusBadRequest, "服务端口必须在 1 到 65535 之间")
		return model.GatewayRoute{}, false
	}
	if !deploymentTargetHasServicePort(target, servicePort) {
		writeError(ctx, http.StatusBadRequest, "访问入口端口必须来自部署配置的服务端口列表")
		return model.GatewayRoute{}, false
	}
	advanced, ok := h.gatewayRouteAdvancedConfig(ctx, project.ID, user, cluster, input)
	if !ok {
		return model.GatewayRoute{}, false
	}

	tlsMode := normalizeTLSMode(input.TLSMode)
	certStatus := "disabled"
	if tlsMode != "http-only" {
		certStatus = "pending"
	}
	return model.GatewayRoute{
		ID:                     routeID,
		ProjectID:              project.ID,
		ApplicationID:          application.ID,
		EnvironmentID:          environment.ID,
		DeploymentTargetID:     target.ID,
		Host:                   host,
		DomainSuffix:           domainSuffix,
		Path:                   fallback(strings.TrimSpace(input.Path), "/"),
		ServicePort:            servicePort,
		TLSMode:                tlsMode,
		CertificateStatus:      certStatus,
		CNAMEName:              host,
		CNAMETarget:            h.gatewayCNAMETarget(cluster, domainSuffix),
		DNSStatus:              fallback(strings.TrimSpace(input.DNSStatus), "pending"),
		Status:                 fallback(strings.TrimSpace(input.Status), "pending"),
		Enabled:                gatewayRouteInputEnabled(input.Enabled),
		IsDefault:              input.IsDefault,
		ParentGatewayName:      advanced.ParentGatewayName,
		ParentGatewayNamespace: advanced.ParentGatewayNamespace,
		SectionName:            advanced.SectionName,
		PathMatchType:          advanced.PathMatchType,
		RequestHeaders:         advanced.RequestHeaders,
		ResponseHeaders:        advanced.ResponseHeaders,
		URLRewrite:             advanced.URLRewrite,
		RequestRedirect:        advanced.RequestRedirect,
		BackendWeight:          advanced.BackendWeight,
		HostnameAliases:        advanced.HostnameAliases,
		CreatedBy:              creatorID,
	}, true
}

func (h *Handlers) ensureGatewayRouteBackendAvailable(ctx *gin.Context, route model.GatewayRoute) bool {
	if !route.Enabled {
		return true
	}
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "gateway_route.deployment_target_missing", "部署配置不存在，不能创建访问入口")
		return false
	}
	cluster, err := h.runtimeClusterForDeploymentTargetValue(target)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "gateway_route.runtime_cluster_missing", "部署配置运行集群不存在，不能创建访问入口")
		return false
	}
	if strings.TrimSpace(cluster.KubeconfigRef) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "gateway_route.runtime_cluster_kubeconfig_missing", "运行集群缺少 kubeconfig，无法检查访问入口后端服务")
		return false
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "gateway_route.runtime_cluster_kubeconfig_missing", "运行集群缺少 kubeconfig，无法检查访问入口后端服务")
		return false
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "gateway_route.runtime_cluster_kubeconfig_invalid", "运行集群 kubeconfig 无效，无法检查访问入口后端服务")
		return false
	}
	namespace := apiProjectNamespace(route.ProjectID)
	serviceName := apiApplicationResourceName(target)
	snapshot, err := client.GetServiceBackendSnapshot(ctx.Request.Context(), namespace, serviceName, int32(route.ServicePort))
	if err != nil {
		writeErrorCode(ctx, http.StatusBadGateway, "gateway_route.backend_check_failed", "访问入口后端服务检查失败，请确认运行集群连接和权限")
		return false
	}
	if !snapshot.ServiceExists {
		writeErrorCode(ctx, http.StatusConflict, "gateway_route.backend_service_missing", fmt.Sprintf("后端 Service %s/%s 不存在，请先重新发布部署配置以恢复 Service 后再创建访问入口", namespace, serviceName))
		return false
	}
	if !snapshot.PortExists {
		writeErrorCode(ctx, http.StatusConflict, "gateway_route.backend_service_port_missing", fmt.Sprintf("后端 Service %s/%s 未暴露端口 %d，请调整部署配置并重新发布后再创建访问入口", namespace, serviceName, route.ServicePort))
		return false
	}
	return true
}

func (h *Handlers) gatewayRouteAdvancedConfig(ctx *gin.Context, projectID string, user model.User, cluster model.RuntimeCluster, input gatewayRouteInput) (gatewayRouteAdvancedConfig, bool) {
	sectionName, err := normalizeGatewayRouteSectionName(input.SectionName, cluster)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return gatewayRouteAdvancedConfig{}, false
	}
	config := gatewayRouteAdvancedConfig{
		ParentGatewayName:      dnsLabelName(input.ParentGatewayName),
		ParentGatewayNamespace: dnsLabelName(input.ParentGatewayNamespace),
		SectionName:            sectionName,
		PathMatchType:          normalizeHTTPRoutePathMatchType(input.PathMatchType),
		RequestHeaders:         strings.TrimSpace(input.RequestHeaders),
		ResponseHeaders:        strings.TrimSpace(input.ResponseHeaders),
		URLRewrite:             strings.TrimSpace(input.URLRewrite),
		RequestRedirect:        strings.TrimSpace(input.RequestRedirect),
		BackendWeight:          normalizeBackendWeight(input.BackendWeight),
		HostnameAliases:        strings.TrimSpace(input.HostnameAliases),
	}
	if !gatewayAdvancedConfigPresent(config) {
		return config, true
	}
	projectAdmin := user.Role == "platform_admin" || h.currentProjectRoleAllows(ctx, projectID, user.ID, "owner", "admin")
	if gatewayAdvancedConfigRequiresProjectAdmin(config) && !projectAdmin {
		writeError(ctx, http.StatusForbidden, "只有项目 Owner/Admin 可以维护访问入口高级配置")
		return gatewayRouteAdvancedConfig{}, false
	}
	platformAdmin := user.Role == "platform_admin"
	if _, err := parseGatewayHeaderMap(config.RequestHeaders, platformAdmin); err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("请求头配置无效: %s", err.Error()))
		return gatewayRouteAdvancedConfig{}, false
	}
	if _, err := parseGatewayHeaderMap(config.ResponseHeaders, platformAdmin); err != nil {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("响应头配置无效: %s", err.Error()))
		return gatewayRouteAdvancedConfig{}, false
	}
	if err := validateGatewayRouteFilterJSON("URL rewrite", config.URLRewrite); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return gatewayRouteAdvancedConfig{}, false
	}
	if err := validateGatewayRouteFilterJSON("Request redirect", config.RequestRedirect); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return gatewayRouteAdvancedConfig{}, false
	}
	if config.URLRewrite != "" && config.RequestRedirect != "" {
		writeError(ctx, http.StatusBadRequest, "URL rewrite 和请求重定向不能同时配置")
		return gatewayRouteAdvancedConfig{}, false
	}
	return config, true
}

func normalizeGatewayRouteSectionName(value string, cluster model.RuntimeCluster) (string, error) {
	sectionName := dnsLabelName(value)
	if sectionName == "" {
		return "", nil
	}
	for _, allowed := range []string{
		fallback(dnsLabelName(cluster.GatewayHTTPListenerName), "web"),
		fallback(dnsLabelName(cluster.GatewayHTTPSListenerName), "websecure"),
	} {
		if sectionName == allowed {
			return sectionName, nil
		}
	}
	return "", fmt.Errorf("Listener Section 只能选择当前集群的 %s 或 %s", fallback(cluster.GatewayHTTPListenerName, "web"), fallback(cluster.GatewayHTTPSListenerName, "websecure"))
}

func gatewayAdvancedConfigRequiresProjectAdmin(config gatewayRouteAdvancedConfig) bool {
	return config.ParentGatewayName != "" ||
		config.ParentGatewayNamespace != "" ||
		config.PathMatchType != "PathPrefix" ||
		config.RequestHeaders != "" ||
		config.ResponseHeaders != "" ||
		config.URLRewrite != "" ||
		config.RequestRedirect != "" ||
		config.BackendWeight != 1 ||
		config.HostnameAliases != ""
}

func gatewayAdvancedConfigPresent(config gatewayRouteAdvancedConfig) bool {
	return config.ParentGatewayName != "" ||
		config.ParentGatewayNamespace != "" ||
		config.SectionName != "" ||
		config.PathMatchType != "PathPrefix" ||
		config.RequestHeaders != "" ||
		config.ResponseHeaders != "" ||
		config.URLRewrite != "" ||
		config.RequestRedirect != "" ||
		config.BackendWeight != 1 ||
		config.HostnameAliases != ""
}

func deploymentTargetServicePort(target model.DeploymentTarget) int {
	ports := model.DeploymentTargetServicePorts(target)
	if len(ports) > 0 {
		return ports[0].Port
	}
	return fallbackInt(target.ServicePort, 8080)
}

func deploymentTargetHasServicePort(target model.DeploymentTarget, port int) bool {
	for _, item := range model.DeploymentTargetServicePorts(target) {
		if item.Port == port {
			return true
		}
	}
	return false
}

func (h *Handlers) gatewayRouteTargetContext(ctx *gin.Context, projectID string, input gatewayRouteInput) (model.DeploymentTarget, model.Application, model.Environment, model.RuntimeCluster, bool) {
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and enabled = ?", strings.TrimSpace(input.DeploymentTargetID), projectID, true).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "部署配置不存在或不属于当前项目空间")
		return model.DeploymentTarget{}, model.Application{}, model.Environment{}, model.RuntimeCluster{}, false
	}
	if applicationID := strings.TrimSpace(input.ApplicationID); applicationID != "" && applicationID != target.ApplicationID {
		writeError(ctx, http.StatusBadRequest, "部署配置不属于当前应用")
		return model.DeploymentTarget{}, model.Application{}, model.Environment{}, model.RuntimeCluster{}, false
	}
	var application model.Application
	if err := h.db.First(&application, "id = ? and project_id = ?", target.ApplicationID, projectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "应用不存在或不属于当前项目空间")
		return model.DeploymentTarget{}, model.Application{}, model.Environment{}, model.RuntimeCluster{}, false
	}
	if !applicationCanMutate(application) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能维护访问入口")
		return model.DeploymentTarget{}, model.Application{}, model.Environment{}, model.RuntimeCluster{}, false
	}
	cluster, err := h.runtimeClusterForDeploymentTargetValue(target)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "部署配置运行集群不存在，不能创建访问入口")
		return model.DeploymentTarget{}, model.Application{}, model.Environment{}, model.RuntimeCluster{}, false
	}
	return target, application, deploymentTargetEnvironmentProfile(target), cluster, true
}

func (h *Handlers) runtimeClusterForGatewayRoute(route model.GatewayRoute) (model.RuntimeCluster, error) {
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		return model.RuntimeCluster{}, err
	}
	return h.runtimeClusterForDeploymentTargetValue(target)
}

func (h *Handlers) runtimeClusterForDeploymentTargetValue(target model.DeploymentTarget) (model.RuntimeCluster, error) {
	return runtimeClusterForDeploymentTargetDB(h.db, target)
}

func gatewayRouteInputEnabled(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func normalizeTLSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "http-challenge", "manual-cert":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "http-only"
	}
}
