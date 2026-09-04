package gatewayapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errGatewayDomainRuntimeReferenceRequired = errors.New("routeId or deploymentTargetId is required")

func (h *Handlers) CheckGatewayDomain(ctx *gin.Context) {
	if _, _, ok := h.authorizeProject(ctx, authz.ActionGatewayRead); !ok {
		return
	}
	cluster, err := h.gatewayClusterForDomainCheck(ctx)
	if err != nil {
		h.writeGatewayRuntimeClusterError(ctx, err)
		return
	}
	domainSuffix, ok := h.gatewayRouteDomainSuffix(ctx, ctx.Query("domainSuffix"), cluster)
	if !ok {
		return
	}
	host := h.normalizeGatewayHost(strings.TrimSpace(ctx.Query("host")), cluster, domainSuffix)
	if host == "" {
		writeError(ctx, http.StatusBadRequest, "请输入域名")
		return
	}
	routeID := strings.TrimSpace(ctx.Query("routeId"))
	var routes []model.GatewayRoute
	if err := h.dbFor(ctx).Select("id").
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

func (h *Handlers) gatewayClusterForDomainCheck(ctx *gin.Context) (model.RuntimeCluster, error) {
	if routeID := strings.TrimSpace(ctx.Query("routeId")); routeID != "" {
		var route model.GatewayRoute
		if err := h.dbFor(ctx).First(&route, "id = ? and project_id = ?", routeID, ctx.Param("projectId")).Error; err != nil {
			return model.RuntimeCluster{}, fmt.Errorf("load gateway route for domain check: %w", err)
		}
		cluster, err := h.runtimeClusterForGatewayRoute(route, ctx.Request.Context())
		if err != nil {
			return model.RuntimeCluster{}, fmt.Errorf("resolve gateway route runtime cluster: %w", err)
		}
		return cluster, nil
	}
	if targetID := strings.TrimSpace(ctx.Query("deploymentTargetId")); targetID != "" {
		var target model.DeploymentTarget
		if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ?", targetID, ctx.Param("projectId")).Error; err != nil {
			return model.RuntimeCluster{}, fmt.Errorf("load deployment target for domain check: %w", err)
		}
		cluster, err := h.runtimeClusterForDeploymentTargetValue(target, ctx.Request.Context())
		if err != nil {
			return model.RuntimeCluster{}, fmt.Errorf("resolve deployment target runtime cluster: %w", err)
		}
		return cluster, nil
	}
	return model.RuntimeCluster{}, errGatewayDomainRuntimeReferenceRequired
}

func (h *Handlers) defaultGatewayHost(project model.Project, stage, applicationIdentifier string, cluster model.RuntimeCluster, domainSuffix string, ctx context.Context) string {
	rootDomain := h.gatewayDomainSuffix(cluster, domainSuffix)
	if rootDomain == "" {
		return ""
	}
	appIdentifier := gatewayHostSegment(applicationIdentifier)
	projectIdentifier := gatewayHostSegment(project.Identifier)
	stageSlug := gatewayHostSegment(h.normalizeStage(stage))
	if appIdentifier == "" || projectIdentifier == "" {
		return ""
	}
	base := strings.Trim(fmt.Sprintf("%s-%s-%s", projectIdentifier, appIdentifier, stageSlug), "-")
	for index := 0; index < 100; index++ {
		prefix := base
		if index > 0 {
			prefix = fmt.Sprintf("%s-%d", base, index+1)
		}
		host := fmt.Sprintf("%s.%s", prefix, rootDomain)
		if !h.gatewayHostExists(host, "", ctx) {
			return host
		}
	}
	return fmt.Sprintf("%s-%s.%s", base, id.New("gw"), rootDomain)
}

func (h *Handlers) gatewayCNAMETarget(cluster model.RuntimeCluster, domainSuffix string) string {
	rootDomain := h.gatewayDomainSuffix(cluster, domainSuffix)
	if rootDomain == "" {
		return ""
	}
	return fmt.Sprintf("*.%s", rootDomain)
}

func (h *Handlers) normalizeGatewayHost(value string, cluster model.RuntimeCluster, domainSuffix string) string {
	host := strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
	if host == "" {
		return ""
	}
	rootDomain := h.gatewayDomainSuffix(cluster, domainSuffix)
	if rootDomain != "" && !strings.Contains(host, ".") {
		prefix := gatewayHostSegment(host)
		if prefix == "" {
			return ""
		}
		return fmt.Sprintf("%s.%s", prefix, rootDomain)
	}
	return host
}

func (h *Handlers) gatewayDomainSuffix(cluster model.RuntimeCluster, selected string) string {
	selected = runtimecluster.NormalizeGatewayDomainSuffix(selected)
	for _, suffix := range h.gatewayDomainSuffixes(cluster) {
		if selected != "" && suffix == selected {
			return suffix
		}
	}
	suffixes := h.gatewayDomainSuffixes(cluster)
	if len(suffixes) == 0 {
		return ""
	}
	return suffixes[0]
}

func (h *Handlers) gatewayDomainSuffixes(cluster model.RuntimeCluster) []string {
	return runtimecluster.DecodeGatewayDomainSuffixes(cluster.GatewayDomainSuffixesRaw)
}

func (h *Handlers) gatewayRouteDomainSuffix(ctx *gin.Context, selected string, cluster model.RuntimeCluster) (string, bool) {
	selected = runtimecluster.NormalizeGatewayDomainSuffix(selected)
	suffixes := h.gatewayDomainSuffixes(cluster)
	if len(suffixes) == 0 {
		writeError(ctx, http.StatusBadRequest, "运行集群未配置可用域名后缀")
		return "", false
	}
	if selected == "" {
		return suffixes[0], true
	}
	for _, suffix := range suffixes {
		if suffix == selected {
			return suffix, true
		}
	}
	writeError(ctx, http.StatusBadRequest, "域名后缀不属于当前部署配置的运行集群")
	return "", false
}

func (h *Handlers) gatewayPublicScheme(cluster model.RuntimeCluster) string {
	return h.normalizeGatewayPublicScheme(cluster.GatewayPublicScheme)
}

func (h *Handlers) gatewayPublicPort(cluster model.RuntimeCluster) int {
	if h.gatewayPublicScheme(cluster) == "https" {
		return h.normalizePort(cluster.GatewayPublicPort, 443)
	}
	return h.normalizePort(cluster.GatewayPublicPort, 80)
}

func (h *Handlers) gatewayRouteWithAccessURL(route model.GatewayRoute, ctx context.Context) (model.GatewayRoute, error) {
	route.AccessURL = ""
	cluster, err := h.runtimeClusterForGatewayRoute(route, ctx)
	if err != nil {
		return route, fmt.Errorf("resolve gateway route access URL runtime cluster: %w", err)
	}
	route.AccessURL = gatewayRouteAccessURL(route, h.gatewayPublicScheme(cluster), h.gatewayPublicPort(cluster))
	return route, nil
}

func (h *Handlers) gatewayRoutesWithAccessURL(routes []model.GatewayRoute, ctx context.Context) []model.GatewayRoute {
	result := make([]model.GatewayRoute, len(routes))
	for index, route := range routes {
		resolved, err := h.gatewayRouteWithAccessURL(route, ctx)
		if err != nil {
			result[index] = unavailableGatewayRoute(resolved, gatewayRouteRuntimeClusterObservationCode(err))
			continue
		}
		result[index] = resolved
	}
	return result
}

func (h *Handlers) writeGatewayRuntimeClusterError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, errGatewayDomainRuntimeReferenceRequired):
		writeErrorCode(ctx, http.StatusBadRequest, "gateway_route.runtime_cluster_reference_required", "routeId or deploymentTargetId is required")
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeErrorCode(ctx, http.StatusNotFound, "gateway_route.runtime_cluster_missing", "gateway route, deployment target, or runtime cluster reference was not found")
	default:
		writeErrorCode(ctx, http.StatusServiceUnavailable, "gateway_route.runtime_cluster_unavailable", "gateway route runtime cluster lookup is unavailable")
	}
}

func gatewayRouteRuntimeClusterObservationCode(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "gateway_route.observation_cancelled"
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "gateway_route.runtime_cluster_not_configured"
	}
	return "gateway_route.runtime_cluster_unavailable"
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

func (h *Handlers) gatewayHostExists(host, routeID string, ctx context.Context) bool {
	if strings.TrimSpace(host) == "" {
		return false
	}
	var count int64
	query := h.dbWithContext(ctx).Model(&model.GatewayRoute{}).Where("host = ? and id <> ?", host, routeID)
	return query.Count(&count).Error == nil && count > 0
}
