package gatewayapi

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

const GatewayObservationTimeout = gatewayObservationTimeout

var GatewayHostSegmentPattern = gatewayHostSegmentPattern

func GatewayHostSegment(value string) string { return gatewayHostSegment(value) }

func ParseGatewayHeaderMap(value string, allowPrivileged bool) (map[string]string, error) {
	return parseGatewayHeaderMap(value, allowPrivileged)
}

func ParseGatewayKeyValueMap(value string) (map[string]string, error) {
	return parseGatewayKeyValueMap(value)
}

func CompactGatewayKeyValueMap(values map[string]string) map[string]string {
	return compactGatewayKeyValueMap(values)
}

func EncodeGatewayHeaderMap(values map[string]string) string {
	return encodeGatewayHeaderMap(values)
}

func LooksLikeSecretValue(value string) bool { return looksLikeSecretValue(value) }

func NormalizeHTTPRoutePathMatchType(value string) string {
	return normalizeHTTPRoutePathMatchType(value)
}

func NormalizeBackendWeight(value int) int { return normalizeBackendWeight(value) }

func ValidateGatewayRouteFilterJSON(label, value string) error {
	return validateGatewayRouteFilterJSON(label, value)
}

func UnavailableGatewayRoute(route model.GatewayRoute, code string) model.GatewayRoute {
	return unavailableGatewayRoute(route, code)
}

func GatewayRouteStatusFromSummary(summary string) string {
	return gatewayRouteStatusFromSummary(summary)
}

func GatewayRouteConditions(conditions []kubeprovider.RouteConditionSnapshot) []model.RouteCondition {
	return gatewayRouteConditions(conditions)
}

func ObserveGatewayRouteDNS(ctx context.Context, route model.GatewayRoute) string {
	return observeGatewayRouteDNS(ctx, route)
}

func ObserveGatewayRouteCertificate(
	ctx context.Context,
	client *kubeprovider.Client,
	cluster model.RuntimeCluster,
	projectNamespace string,
	resourceName string,
	route *model.GatewayRoute,
) {
	observeGatewayRouteCertificate(ctx, client, cluster, projectNamespace, resourceName, route)
}

func GatewayRouteCertificateFallback(route model.GatewayRoute) string {
	return gatewayRouteCertificateFallback(route)
}

func GatewayRouteCertificateIssuerKind(cluster model.RuntimeCluster) string {
	return gatewayRouteCertificateIssuerKind(cluster)
}

func GatewayRouteAccessURL(route model.GatewayRoute, scheme string, publicPort int) string {
	return gatewayRouteAccessURL(route, scheme, publicPort)
}

func ShouldShowGatewayPublicPort(scheme string, publicPort int) bool {
	return shouldShowGatewayPublicPort(scheme, publicPort)
}

func NormalizeGatewayRouteSectionName(value string, cluster model.RuntimeCluster) (string, error) {
	return normalizeGatewayRouteSectionName(value, cluster)
}

func GatewayAdvancedConfigRequiresProjectAdmin(config GatewayRouteAdvancedConfig) bool {
	return gatewayAdvancedConfigRequiresProjectAdmin(config)
}

func GatewayAdvancedConfigPresent(config GatewayRouteAdvancedConfig) bool {
	return gatewayAdvancedConfigPresent(config)
}

func DeploymentTargetServicePort(target model.DeploymentTarget) int {
	return deploymentTargetServicePort(target)
}

func DeploymentTargetHasServicePort(target model.DeploymentTarget, port int) bool {
	return deploymentTargetHasServicePort(target, port)
}

func GatewayRouteInputEnabled(value *bool) bool { return gatewayRouteInputEnabled(value) }

func NormalizeTLSMode(value string) string { return normalizeTLSMode(value) }

func (h *Handler) GatewayClusterForDomainCheck(ctx *gin.Context) model.RuntimeCluster {
	return h.gatewayClusterForDomainCheck(ctx)
}

func (h *Handler) DefaultGatewayHost(project model.Project, stage, applicationIdentifier string, cluster model.RuntimeCluster, domainSuffix string, ctx context.Context) string {
	return h.defaultGatewayHost(project, stage, applicationIdentifier, cluster, domainSuffix, ctx)
}

func (h *Handler) GatewayCNAMETarget(cluster model.RuntimeCluster, domainSuffix string) string {
	return h.gatewayCNAMETarget(cluster, domainSuffix)
}

func (h *Handler) NormalizeGatewayHost(value string, cluster model.RuntimeCluster, domainSuffix string) string {
	return h.normalizeGatewayHost(value, cluster, domainSuffix)
}

func (h *Handler) GatewayRootDomain(cluster model.RuntimeCluster) string {
	return h.gatewayRootDomain(cluster)
}

func (h *Handler) GatewayDomainSuffix(cluster model.RuntimeCluster, selected string) string {
	return h.gatewayDomainSuffix(cluster, selected)
}

func (h *Handler) GatewayDomainSuffixes(cluster model.RuntimeCluster) []string {
	return h.gatewayDomainSuffixes(cluster)
}

func (h *Handler) GatewayRouteDomainSuffix(ctx *gin.Context, selected string, cluster model.RuntimeCluster) (string, bool) {
	return h.gatewayRouteDomainSuffix(ctx, selected, cluster)
}

func (h *Handler) GatewayPublicScheme(cluster model.RuntimeCluster) string {
	return h.gatewayPublicScheme(cluster)
}

func (h *Handler) GatewayPublicPort(cluster model.RuntimeCluster) int {
	return h.gatewayPublicPort(cluster)
}

func (h *Handler) LegacyGatewayRootDomain() string { return h.legacyGatewayRootDomain() }

func (h *Handler) GatewayRouteWithAccessURL(route model.GatewayRoute, ctx context.Context) model.GatewayRoute {
	return h.gatewayRouteWithAccessURL(route, ctx)
}

func (h *Handler) GatewayRoutesWithAccessURL(routes []model.GatewayRoute, ctx context.Context) []model.GatewayRoute {
	return h.gatewayRoutesWithAccessURL(routes, ctx)
}

func (h *Handler) GatewayHostExists(host, routeID string, ctx context.Context) bool {
	return h.gatewayHostExists(host, routeID, ctx)
}

func (h *Handler) ObserveGatewayRoutes(ctx context.Context, routes []model.GatewayRoute) []model.GatewayRoute {
	return h.observeGatewayRoutes(ctx, routes)
}

func (h *Handler) ObserveGatewayRoute(ctx context.Context, route model.GatewayRoute) model.GatewayRoute {
	return h.observeGatewayRoute(ctx, route)
}

func (h *Handler) GatewayRouteFromInput(ctx *gin.Context, project model.Project, user model.User, creatorID string, input GatewayRouteInput, routeID string) (model.GatewayRoute, bool) {
	return h.gatewayRouteFromInput(ctx, project, user, creatorID, input, routeID)
}

func (h *Handler) EnsureGatewayRouteBackendAvailable(ctx *gin.Context, route model.GatewayRoute) bool {
	return h.ensureGatewayRouteBackendAvailable(ctx, route)
}

func (h *Handler) GatewayRouteAdvancedConfig(ctx *gin.Context, projectID string, user model.User, cluster model.RuntimeCluster, input GatewayRouteInput) (GatewayRouteAdvancedConfig, bool) {
	return h.gatewayRouteAdvancedConfig(ctx, projectID, user, cluster, input)
}

func (h *Handler) GatewayRouteTargetContext(ctx *gin.Context, projectID string, input GatewayRouteInput) (model.DeploymentTarget, model.Application, model.Environment, model.RuntimeCluster, bool) {
	return h.gatewayRouteTargetContext(ctx, projectID, input)
}

func (h *Handler) RuntimeClusterForGatewayRoute(route model.GatewayRoute, ctx context.Context) (model.RuntimeCluster, error) {
	return h.runtimeClusterForGatewayRoute(route, ctx)
}

func (h *Handler) RuntimeClusterForDeploymentTargetValue(target model.DeploymentTarget, ctx context.Context) (model.RuntimeCluster, error) {
	return h.runtimeClusterForDeploymentTargetValue(target, ctx)
}

func (h *Handler) FindGatewayRoute(ctx *gin.Context) (model.GatewayRoute, bool) {
	return h.findGatewayRoute(ctx)
}

func (h *Handler) EnqueueGatewayApply(ctx context.Context, route model.GatewayRoute, actorID string) bool {
	return h.enqueueGatewayApply(ctx, route, actorID)
}

func (h *Handler) ConfigValue(key string) string { return h.configValue(key) }
