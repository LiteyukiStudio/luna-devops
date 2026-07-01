package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListGatewayRoutes(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	query := h.db.Model(&model.GatewayRoute{}).Where("project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "host", "path", "status")
	var routes []model.GatewayRoute
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		if err := query.Order(orderByClause(pagination, map[string]string{
			"host":      "host",
			"status":    "status",
			"enabled":   "enabled",
			"createdAt": "created_at",
		}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&routes).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, paginatedResponse(h.gatewayRoutesWithAccessURL(routes), total, pagination))
		return
	}
	if err := query.Find(&routes).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.gatewayRoutesWithAccessURL(routes))
}

func (h *Handlers) CreateGatewayRoute(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var input gatewayRouteInput
	if !bindJSON(ctx, &input) {
		return
	}
	route, ok := h.gatewayRouteFromInput(ctx, project, user, user.ID, input, "")
	if !ok {
		return
	}
	route.ID = id.New("gwr")
	if !h.ensureGatewayRouteBackendAvailable(ctx, route) {
		return
	}
	if err := h.db.Create(&route).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.enqueueGatewayApply(ctx.Request.Context(), route) {
		route.Status = "failed"
		if err := h.db.Save(&route).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusServiceUnavailable, "网关任务投递失败，请稍后重试")
		return
	}
	ctx.JSON(http.StatusCreated, h.gatewayRouteWithAccessURL(route))
}

func (h *Handlers) UpdateGatewayRoute(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	route, ok := h.findGatewayRoute(ctx)
	if !ok {
		return
	}
	if !h.ensureGatewayRouteCanMutate(ctx, route) {
		return
	}
	var input gatewayRouteInput
	if !bindJSON(ctx, &input) {
		return
	}
	next, ok := h.gatewayRouteFromInput(ctx, project, user, route.CreatedBy, input, route.ID)
	if !ok {
		return
	}
	if !h.ensureGatewayRouteBackendAvailable(ctx, next) {
		return
	}
	route.ApplicationID = next.ApplicationID
	route.EnvironmentID = next.EnvironmentID
	route.DeploymentTargetID = next.DeploymentTargetID
	route.Host = next.Host
	route.Path = next.Path
	route.ServicePort = next.ServicePort
	route.TLSMode = next.TLSMode
	route.CertificateStatus = next.CertificateStatus
	route.CNAMEName = next.CNAMEName
	route.CNAMETarget = next.CNAMETarget
	route.DNSStatus = next.DNSStatus
	route.Status = next.Status
	route.Enabled = next.Enabled
	route.IsDefault = next.IsDefault
	route.ParentGatewayName = next.ParentGatewayName
	route.ParentGatewayNamespace = next.ParentGatewayNamespace
	route.SectionName = next.SectionName
	route.PathMatchType = next.PathMatchType
	route.RequestHeaders = next.RequestHeaders
	route.ResponseHeaders = next.ResponseHeaders
	route.URLRewrite = next.URLRewrite
	route.RequestRedirect = next.RequestRedirect
	route.BackendWeight = next.BackendWeight
	route.HostnameAliases = next.HostnameAliases
	if err := h.db.Save(&route).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.enqueueGatewayApply(ctx.Request.Context(), route) {
		route.Status = "failed"
		if err := h.db.Save(&route).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusServiceUnavailable, "网关任务投递失败，请稍后重试")
		return
	}
	ctx.JSON(http.StatusOK, h.gatewayRouteWithAccessURL(route))
}

func (h *Handlers) DeleteGatewayRoute(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	route, ok := h.findGatewayRoute(ctx)
	if !ok {
		return
	}
	if !deleteStatusCanStart(route.DeleteStatus) {
		writeError(ctx, http.StatusConflict, "访问入口正在删除中，请等待资源清理完成")
		return
	}
	if err := markResourceDeleting(h.db, &model.GatewayRoute{}, route.ID); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueResourceCleanup(ctx.Request.Context(), tasks.ResourceCleanupPayload{
		ResourceType: "gateway_route",
		ResourceID:   route.ID,
		ProjectID:    route.ProjectID,
		ActorID:      user.ID,
	}) {
		_ = markResourceDeleteFailed(h.db, &model.GatewayRoute{}, route.ID, "资源清理任务投递失败，请稍后重试")
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) CheckGatewayDomain(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	cluster := h.gatewayClusterForDomainCheck(ctx)
	host := h.normalizeGatewayHost(strings.TrimSpace(ctx.Query("host")), cluster)
	if host == "" {
		writeError(ctx, http.StatusBadRequest, "请输入域名")
		return
	}
	routeID := strings.TrimSpace(ctx.Query("routeId"))
	var routes []model.GatewayRoute
	if err := h.db.Select("id").
		Where("host = ?", host).
		Find(&routes).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	status := "available"
	available := true
	for _, route := range routes {
		if routeID != "" && route.ID == routeID {
			status = "current"
			continue
		}
		status = "conflict"
		available = false
		break
	}
	ctx.JSON(http.StatusOK, gin.H{"available": available, "host": host, "status": status})
}

func (h *Handlers) findGatewayRoute(ctx *gin.Context) (model.GatewayRoute, bool) {
	var route model.GatewayRoute
	if err := h.db.First(&route, "id = ? and project_id = ?", ctx.Param("routeId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "gateway route not found")
		return route, false
	}
	return route, true
}

func (h *Handlers) enqueueGatewayApply(ctx context.Context, route model.GatewayRoute) bool {
	if h.taskClient == nil {
		return false
	}
	_, err := h.taskClient.EnqueueGatewayApply(ctx, tasks.GatewayApplyPayload{
		GatewayRouteID: route.ID,
		ProjectID:      route.ProjectID,
		ActorID:        route.CreatedBy,
	})
	return err == nil
}

func (h *Handlers) gatewayRouteFromInput(ctx *gin.Context, project model.Project, user model.User, creatorID string, input gatewayRouteInput, routeID string) (model.GatewayRoute, bool) {
	target, application, environment, cluster, ok := h.gatewayRouteTargetContext(ctx, project.ID, input)
	if !ok {
		return model.GatewayRoute{}, false
	}
	host := h.normalizeGatewayHost(input.Host, cluster)
	if host == "" {
		host = h.defaultGatewayHost(project, target.Stage, application.Slug, cluster)
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
		certStatus = fallback(strings.TrimSpace(input.CertificateStatus), "pending")
	}
	return model.GatewayRoute{
		ID:                     routeID,
		ProjectID:              project.ID,
		ApplicationID:          application.ID,
		EnvironmentID:          environment.ID,
		DeploymentTargetID:     target.ID,
		Host:                   host,
		Path:                   fallback(strings.TrimSpace(input.Path), "/"),
		ServicePort:            servicePort,
		TLSMode:                tlsMode,
		CertificateStatus:      certStatus,
		CNAMEName:              host,
		CNAMETarget:            h.gatewayCNAMETarget(cluster),
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

func (h *Handlers) gatewayClusterForDomainCheck(ctx *gin.Context) model.RuntimeCluster {
	if routeID := strings.TrimSpace(ctx.Query("routeId")); routeID != "" {
		var route model.GatewayRoute
		if err := h.db.First(&route, "id = ? and project_id = ?", routeID, ctx.Param("projectId")).Error; err == nil {
			if cluster, err := h.runtimeClusterForGatewayRoute(route); err == nil {
				return cluster
			}
		}
	}
	if targetID := strings.TrimSpace(ctx.Query("deploymentTargetId")); targetID != "" {
		var target model.DeploymentTarget
		if err := h.db.First(&target, "id = ? and project_id = ?", targetID, ctx.Param("projectId")).Error; err == nil {
			if cluster, err := h.runtimeClusterForDeploymentTargetValue(target); err == nil {
				return cluster
			}
		}
	}
	return model.RuntimeCluster{}
}

func (h *Handlers) runtimeClusterForGatewayRoute(route model.GatewayRoute) (model.RuntimeCluster, error) {
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		return model.RuntimeCluster{}, err
	}
	return h.runtimeClusterForDeploymentTargetValue(target)
}

func (h *Handlers) runtimeClusterForDeploymentTargetValue(target model.DeploymentTarget) (model.RuntimeCluster, error) {
	var cluster model.RuntimeCluster
	if clusterID := strings.TrimSpace(target.ClusterID); clusterID != "" {
		err := h.db.First(&cluster, "id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Error
		return cluster, err
	}
	err := h.db.Where("scope = ? and is_default = ? and type in ?", "global", true, []string{"kubernetes", "k3s"}).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = h.db.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	}
	return cluster, err
}

func (h *Handlers) defaultGatewayHost(project model.Project, stage, applicationSlug string, cluster model.RuntimeCluster) string {
	rootDomain := h.gatewayRootDomain(cluster)
	if rootDomain == "" {
		return ""
	}
	appSlug := gatewayHostSegment(applicationSlug)
	projectSlug := gatewayHostSegment(project.Slug)
	stageSlug := gatewayHostSegment(normalizeStage(stage))
	if appSlug == "" || projectSlug == "" {
		return ""
	}
	base := strings.Trim(fmt.Sprintf("%s-%s-%s", projectSlug, appSlug, stageSlug), "-")
	for index := 0; index < 100; index++ {
		prefix := base
		if index > 0 {
			prefix = fmt.Sprintf("%s-%d", base, index+1)
		}
		host := fmt.Sprintf("%s.%s", prefix, rootDomain)
		if !h.gatewayHostExists(host, "") {
			return host
		}
	}
	return fmt.Sprintf("%s-%s.%s", base, id.New("gw"), rootDomain)
}

func (h *Handlers) gatewayCNAMETarget(cluster model.RuntimeCluster) string {
	rootDomain := h.gatewayRootDomain(cluster)
	if rootDomain == "" {
		return ""
	}
	return fmt.Sprintf("*.%s", rootDomain)
}

func (h *Handlers) normalizeGatewayHost(value string, cluster model.RuntimeCluster) string {
	host := strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
	if host == "" {
		return ""
	}
	rootDomain := h.gatewayRootDomain(cluster)
	if rootDomain != "" && !strings.Contains(host, ".") {
		prefix := gatewayHostSegment(host)
		if prefix == "" {
			return ""
		}
		return fmt.Sprintf("%s.%s", prefix, rootDomain)
	}
	return host
}

func (h *Handlers) gatewayRootDomain(cluster model.RuntimeCluster) string {
	return normalizeGatewayRootDomain(cluster.GatewayRootDomain, h.legacyGatewayRootDomain())
}

func (h *Handlers) gatewayPublicScheme(cluster model.RuntimeCluster) string {
	return normalizeGatewayPublicScheme(cluster.GatewayPublicScheme)
}

func (h *Handlers) gatewayPublicPort(cluster model.RuntimeCluster) int {
	if h.gatewayPublicScheme(cluster) == "https" {
		return normalizePort(cluster.GatewayPublicPort, 443)
	}
	return normalizePort(cluster.GatewayPublicPort, 80)
}

func (h *Handlers) legacyGatewayRootDomain() string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(h.configValue("gateway.rootDomain"))), ".")
}

func (h *Handlers) gatewayRouteWithAccessURL(route model.GatewayRoute) model.GatewayRoute {
	cluster, err := h.runtimeClusterForGatewayRoute(route)
	if err != nil {
		route.AccessURL = gatewayRouteAccessURL(route, normalizeGatewayPublicScheme(h.configValue("gateway.publicScheme")), 0)
		return route
	}
	route.AccessURL = gatewayRouteAccessURL(route, h.gatewayPublicScheme(cluster), h.gatewayPublicPort(cluster))
	return route
}

func (h *Handlers) gatewayRoutesWithAccessURL(routes []model.GatewayRoute) []model.GatewayRoute {
	result := make([]model.GatewayRoute, len(routes))
	for index, route := range routes {
		result[index] = h.gatewayRouteWithAccessURL(route)
	}
	return result
}

func gatewayRouteAccessURL(route model.GatewayRoute, scheme string, publicPort int) string {
	host := strings.TrimSpace(route.Host)
	if host == "" {
		return ""
	}
	if scheme != "https" {
		scheme = "http"
	}
	pathValue := strings.TrimSpace(route.Path)
	if pathValue == "" {
		pathValue = "/"
	}
	if !strings.HasPrefix(pathValue, "/") {
		pathValue = "/" + pathValue
	}
	if pathValue == "/" {
		pathValue = ""
	}
	if shouldShowGatewayPublicPort(scheme, publicPort) {
		host = net.JoinHostPort(host, strconv.Itoa(publicPort))
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: pathValue}).String()
}

func shouldShowGatewayPublicPort(scheme string, publicPort int) bool {
	if publicPort <= 0 {
		return false
	}
	return !(scheme == "https" && publicPort == 443) && !(scheme == "http" && publicPort == 80)
}

func (h *Handlers) gatewayHostExists(host, routeID string) bool {
	if strings.TrimSpace(host) == "" {
		return false
	}
	var count int64
	query := h.db.Model(&model.GatewayRoute{}).Where("host = ? and id <> ?", host, routeID)
	return query.Count(&count).Error == nil && count > 0
}

var gatewayHostSegmentPattern = regexp.MustCompile(`[^a-z0-9-]+`)

func gatewayHostSegment(value string) string {
	segment := strings.Trim(strings.ToLower(strings.TrimSpace(value)), "-")
	segment = gatewayHostSegmentPattern.ReplaceAllString(segment, "-")
	segment = strings.Join(strings.FieldsFunc(segment, func(char rune) bool { return char == '-' }), "-")
	return strings.Trim(segment, "-")
}

func apiProjectNamespace(projectID string) string {
	return apiIDResourceName("ns", projectID)
}

func apiApplicationResourceName(target model.DeploymentTarget) string {
	return apiIDResourceName("dplt", target.ID)
}

func apiIDResourceName(prefix string, value string) string {
	suffix := apiShortID(value)
	if suffix == "" {
		return gatewayHostSegment(prefix)
	}
	return gatewayHostSegment(prefix + "-" + suffix)
}

func apiShortID(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, "_"); index >= 0 {
		value = value[index+1:]
	}
	value = gatewayHostSegment(value)
	if len(value) > 10 {
		return value[:10]
	}
	return value
}

func (h *Handlers) configValue(key string) string {
	values := h.configs.get([]string{key})
	return values[key]
}

func normalizeTLSMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "http-challenge", "manual-cert":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "http-only"
	}
}

type gatewayRouteInput struct {
	ApplicationID          string `json:"applicationId" binding:"required"`
	EnvironmentID          string `json:"environmentId"`
	DeploymentTargetID     string `json:"deploymentTargetId" binding:"required"`
	Host                   string `json:"host"`
	Path                   string `json:"path"`
	ServicePort            int    `json:"servicePort"`
	TLSMode                string `json:"tlsMode"`
	CertificateStatus      string `json:"certificateStatus"`
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

func gatewayRouteInputEnabled(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

var (
	gatewayHeaderNamePattern = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)
	gatewayHopByHopHeaders   = map[string]struct{}{
		"connection":          {},
		"keep-alive":          {},
		"proxy-authenticate":  {},
		"proxy-authorization": {},
		"te":                  {},
		"trailer":             {},
		"transfer-encoding":   {},
		"upgrade":             {},
	}
	gatewayPrivilegedHeaders = map[string]struct{}{
		"authorization":     {},
		"cookie":            {},
		"host":              {},
		"x-forwarded-for":   {},
		"x-forwarded-host":  {},
		"x-forwarded-port":  {},
		"x-forwarded-proto": {},
		"x-real-ip":         {},
		"set-cookie":        {},
	}
)

func parseGatewayHeaderMap(value string, allowPrivileged bool) (map[string]string, error) {
	items, err := parseGatewayKeyValueMap(value)
	if err != nil {
		return nil, err
	}
	for key := range items {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if !gatewayHeaderNamePattern.MatchString(key) {
			return nil, fmt.Errorf("header %q 不是合法 HTTP header 名称", key)
		}
		if _, exists := gatewayHopByHopHeaders[normalized]; exists {
			return nil, fmt.Errorf("不允许配置逐跳 header %q", key)
		}
		if _, exists := gatewayPrivilegedHeaders[normalized]; exists && !allowPrivileged {
			return nil, fmt.Errorf("header %q 仅平台管理员可配置", key)
		}
	}
	return items, nil
}

func parseGatewayKeyValueMap(value string) (map[string]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return map[string]string{}, nil
	}
	if strings.HasPrefix(value, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			return nil, err
		}
		parsed := make(map[string]string, len(raw))
		for key, item := range raw {
			parsed[strings.TrimSpace(key)] = fmt.Sprint(item)
		}
		return compactGatewayKeyValueMap(parsed), nil
	}
	parsed := map[string]string{}
	for lineNumber, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, item, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("第 %s 行缺少 =", strconv.Itoa(lineNumber+1))
		}
		parsed[strings.TrimSpace(key)] = strings.TrimSpace(item)
	}
	return compactGatewayKeyValueMap(parsed), nil
}

func compactGatewayKeyValueMap(values map[string]string) map[string]string {
	compacted := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		compacted[key] = strings.TrimSpace(value)
	}
	return compacted
}

func looksLikeSecretValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "secret=") ||
		strings.Contains(lower, "token=") ||
		strings.Contains(lower, "password=") ||
		strings.Contains(lower, "authorization:")
}

func normalizeHTTPRoutePathMatchType(value string) string {
	if strings.TrimSpace(value) == "Exact" {
		return "Exact"
	}
	return "PathPrefix"
}

func normalizeBackendWeight(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func validateGatewayRouteFilterJSON(label string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return fmt.Errorf("%s 配置必须是 JSON 对象", label)
	}
	for key, item := range raw {
		if looksLikeSecretValue(key) || looksLikeSecretValue(fmt.Sprint(item)) {
			return fmt.Errorf("%s 配置不应直接包含密钥值", label)
		}
	}
	return nil
}
