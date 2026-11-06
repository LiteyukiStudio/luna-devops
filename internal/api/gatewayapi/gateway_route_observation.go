package gatewayapi

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	dnsprovider "github.com/LiteyukiStudio/devops/internal/provider/dns"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcename"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const gatewayObservationTimeout = 8 * time.Second

func (h *Handlers) observeGatewayRoutes(ctx context.Context, routes []model.GatewayRoute) []model.GatewayRoute {
	return h.observeGatewayRoutesWith(ctx, routes, h.observeGatewayRoute)
}

// observeGatewayRouteStatuses 只读取 HTTPRoute 状态，不探测 DNS 与证书，供只需要就绪计数的聚合视图使用。
func (h *Handlers) observeGatewayRouteStatuses(ctx context.Context, routes []model.GatewayRoute) []model.GatewayRoute {
	return h.observeGatewayRoutesWith(ctx, routes, func(ctx context.Context, route model.GatewayRoute) model.GatewayRoute {
		probeCtx, cancel := context.WithTimeout(ctx, gatewayObservationTimeout)
		defer cancel()
		route, _, _ = h.observeGatewayRouteStatus(probeCtx, route)
		return route
	})
}

func (h *Handlers) observeGatewayRoutesWith(ctx context.Context, routes []model.GatewayRoute, observe func(context.Context, model.GatewayRoute) model.GatewayRoute) []model.GatewayRoute {
	const concurrency = 6
	result := append([]model.GatewayRoute(nil), routes...)
	guard := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	for index := range result {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case guard <- struct{}{}:
				defer func() { <-guard }()
			case <-ctx.Done():
				result[index] = unavailableGatewayRoute(result[index], "gateway_route.observation_cancelled")
				return
			}
			result[index] = observe(ctx, result[index])
		}()
	}
	wait.Wait()
	return result
}

func (h *Handlers) observeGatewayRoute(ctx context.Context, route model.GatewayRoute) model.GatewayRoute {
	probeCtx, cancel := context.WithTimeout(ctx, gatewayObservationTimeout)
	defer cancel()
	route, probe, observed := h.observeGatewayRouteStatus(probeCtx, route)
	if !observed {
		return route
	}
	route.DNSStatus = observeGatewayRouteDNS(ctx, route)
	if probe == nil {
		route.CertificateStatus = gatewayRouteCertificateFallback(route)
		return route
	}
	observeGatewayRouteCertificate(probeCtx, probe.client, probe.cluster, probe.namespace, probe.resourceName, &route)
	return route
}

// gatewayRouteStatusProbe 是读取 HTTPRoute 状态时建立的集群连接，完整观察复用它继续读取证书。
type gatewayRouteStatusProbe struct {
	client       *kubeprovider.Client
	cluster      model.RuntimeCluster
	namespace    string
	resourceName string
}

// observeGatewayRouteStatus 读取访问入口的 HTTPRoute 状态。observed 为 false 表示入口停用或无法观察，
// DNS 与证书状态已按终态填写；probe 为 nil 表示运行集群未配置，无法继续读取证书。
func (h *Handlers) observeGatewayRouteStatus(ctx context.Context, route model.GatewayRoute) (model.GatewayRoute, *gatewayRouteStatusProbe, bool) {
	observedAt := time.Now().UTC()
	route.ObservedAt = &observedAt
	if !route.Enabled {
		route.Status = "disabled"
		route.DNSStatus = "disabled"
		route.CertificateStatus = "disabled"
		return route, nil, false
	}

	var project model.Project
	if err := h.dbWithContext(ctx).First(&project, "id = ?", route.ProjectID).Error; err != nil {
		return unavailableGatewayRoute(route, "gateway_route.project_reference_unavailable"), nil, false
	}
	cluster, err := h.runtimeClusterForGatewayRoute(route, ctx)
	if err != nil {
		route.Status = observation.StatusNotConfigured
		route.ObservationCode = "gateway_route.runtime_cluster_not_configured"
		return route, nil, true
	}
	route.CertificateIssuerKind = gatewayRouteCertificateIssuerKind(cluster)
	route.CertificateIssuerName = strings.TrimSpace(cluster.GatewayCertIssuerName)

	kubeconfig := strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if strings.TrimSpace(cluster.KubeconfigRef) == "" || kubeconfig == "" {
		route.Status = observation.StatusNotConfigured
		route.ObservationCode = "gateway_route.kubeconfig_not_configured"
		return route, nil, true
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return unavailableGatewayRoute(route, "gateway_route.invalid_kubeconfig"), nil, false
	}

	namespace := h.runtimeProjectNamespace(project)
	resourceName := resourcename.GatewayRoute(route.ID)
	snapshot, err := client.GetHTTPRouteStatus(ctx, namespace, resourceName)
	switch {
	case apierrors.IsNotFound(err):
		route.Status = observation.StatusNotFound
		route.ObservationCode = "gateway_route.http_route_not_found"
	case err != nil:
		route.Status = observation.StatusUnavailable
		route.ObservationCode = "gateway_route.http_route_unavailable"
	default:
		route.Status = gatewayRouteStatusFromSummary(snapshot.Summary)
		route.RouteSummary = snapshot.Summary
		route.Conditions = gatewayRouteConditions(snapshot.Conditions)
	}
	return route, &gatewayRouteStatusProbe{client: client, cluster: cluster, namespace: namespace, resourceName: resourceName}, true
}

func unavailableGatewayRoute(route model.GatewayRoute, code string) model.GatewayRoute {
	observedAt := time.Now().UTC()
	route.Status = observation.StatusUnavailable
	route.DNSStatus = observation.StatusUnavailable
	route.CertificateStatus = gatewayRouteCertificateFallback(route)
	route.ObservationCode = code
	route.ObservedAt = &observedAt
	return route
}

func gatewayRouteStatusFromSummary(summary string) string {
	switch strings.ToLower(strings.TrimSpace(summary)) {
	case "accepted", "ready", "active":
		return observation.StatusReady
	case "pending", "progressing":
		return observation.StatusProgressing
	case "failed", "rejected":
		return observation.StatusDegraded
	default:
		return observation.StatusUnknown
	}
}

func gatewayRouteConditions(conditions []kubeprovider.RouteConditionSnapshot) []model.RouteCondition {
	result := make([]model.RouteCondition, 0, len(conditions))
	for _, condition := range conditions {
		result = append(result, model.RouteCondition{
			Type:               condition.Type,
			Status:             condition.Status,
			Reason:             condition.Reason,
			Message:            condition.Message,
			ObservedGeneration: condition.ObservedGeneration,
		})
	}
	return result
}

func observeGatewayRouteDNS(ctx context.Context, route model.GatewayRoute) string {
	if strings.TrimSpace(route.CNAMEName) == "" || strings.TrimSpace(route.CNAMETarget) == "" {
		return observation.StatusNotConfigured
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := dnsprovider.CheckCNAME(probeCtx, dnsprovider.NewNetResolver(), route.CNAMEName, route.CNAMETarget)
	if err == nil {
		return "verified"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return observation.StatusUnavailable
	}
	return "failed"
}

func observeGatewayRouteCertificate(
	ctx context.Context,
	client *kubeprovider.Client,
	cluster model.RuntimeCluster,
	projectNamespace string,
	resourceName string,
	route *model.GatewayRoute,
) {
	if route == nil {
		return
	}
	if route.TLSMode != "http-challenge" {
		route.CertificateStatus = gatewayRouteCertificateFallback(*route)
		return
	}
	namespace := firstNonEmpty(
		strings.TrimSpace(cluster.GatewayCertificateNamespace),
		strings.TrimSpace(cluster.GatewayNamespace),
		projectNamespace,
	)
	snapshot, err := client.GetCertificateSnapshot(ctx, namespace, resourceName)
	switch {
	case apierrors.IsNotFound(err):
		route.CertificateStatus = observation.StatusNotFound
		route.CertificateMessage = ""
	case err != nil:
		route.CertificateStatus = observation.StatusUnavailable
		route.CertificateMessage = ""
	default:
		route.CertificateStatus = snapshot.Phase
		route.CertificateMessage = snapshot.Message
		route.CertificateNotAfter = snapshot.NotAfter
	}
}

func gatewayRouteCertificateFallback(route model.GatewayRoute) string {
	switch route.TLSMode {
	case "http-only":
		return "disabled"
	case "manual-cert":
		return "manual"
	default:
		return observation.StatusUnavailable
	}
}

func gatewayRouteCertificateIssuerKind(cluster model.RuntimeCluster) string {
	if strings.EqualFold(strings.TrimSpace(cluster.GatewayCertIssuerKind), "Issuer") {
		return "Issuer"
	}
	return "ClusterIssuer"
}
