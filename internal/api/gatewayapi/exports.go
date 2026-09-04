package gatewayapi

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func ParseGatewayHeaderMap(value string, allowPrivileged bool) (map[string]string, error) {
	return parseGatewayHeaderMap(value, allowPrivileged)
}

func EncodeGatewayHeaderMap(values map[string]string) string {
	return encodeGatewayHeaderMap(values)
}

func GatewayRouteAccessURL(route model.GatewayRoute, scheme string, publicPort int) string {
	return gatewayRouteAccessURL(route, scheme, publicPort)
}

func (h *Handler) NormalizeGatewayHost(value string, cluster model.RuntimeCluster, domainSuffix string) string {
	return h.normalizeGatewayHost(value, cluster, domainSuffix)
}

func (h *Handler) GatewayDomainSuffixes(cluster model.RuntimeCluster) []string {
	return h.gatewayDomainSuffixes(cluster)
}

func (h *Handler) GatewayPublicScheme(cluster model.RuntimeCluster) string {
	return h.gatewayPublicScheme(cluster)
}

func (h *Handler) GatewayPublicPort(cluster model.RuntimeCluster) int {
	return h.gatewayPublicPort(cluster)
}

func (h *Handler) RuntimeClusterForDeploymentTargetValue(target model.DeploymentTarget, ctx context.Context) (model.RuntimeCluster, error) {
	return h.runtimeClusterForDeploymentTargetValue(target, ctx)
}

func (h *Handler) EnqueueGatewayApply(ctx context.Context, route model.GatewayRoute, actorID string) bool {
	return h.enqueueGatewayApply(ctx, route, actorID)
}
