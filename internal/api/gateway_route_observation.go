package api

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
			result[index] = h.observeGatewayRoute(ctx, result[index])
		}()
	}
	wait.Wait()
	return result
}

func (h *Handlers) observeGatewayRoute(ctx context.Context, route model.GatewayRoute) model.GatewayRoute {
	observedAt := time.Now().UTC()
	route.ObservedAt = &observedAt
	if !route.Enabled {
		route.Status = "disabled"
		route.DNSStatus = "disabled"
		route.CertificateStatus = "disabled"
		return route
	}

	var project model.Project
	if err := h.db.First(&project, "id = ?", route.ProjectID).Error; err != nil {
		return unavailableGatewayRoute(route, "gateway_route.project_reference_unavailable")
	}
	cluster, err := h.runtimeClusterForGatewayRoute(route)
	if err != nil {
		route.Status = observation.StatusNotConfigured
		route.ObservationCode = "gateway_route.runtime_cluster_not_configured"
		route.DNSStatus = observeGatewayRouteDNS(ctx, route)
		route.CertificateStatus = gatewayRouteCertificateFallback(route)
		return route
	}
	route.CertificateIssuerKind = gatewayRouteCertificateIssuerKind(cluster)
	route.CertificateIssuerName = strings.TrimSpace(cluster.GatewayCertIssuerName)

	kubeconfig := strings.TrimSpace(h.secrets.Resolve(cluster.KubeconfigRef))
	if strings.TrimSpace(cluster.KubeconfigRef) == "" || kubeconfig == "" {
		route.Status = observation.StatusNotConfigured
		route.ObservationCode = "gateway_route.kubeconfig_not_configured"
		route.DNSStatus = observeGatewayRouteDNS(ctx, route)
		route.CertificateStatus = gatewayRouteCertificateFallback(route)
		return route
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return unavailableGatewayRoute(route, "gateway_route.invalid_kubeconfig")
	}

	namespace := runtimeProjectNamespace(project)
	resourceName := resourcename.GatewayRoute(route.ID)
	probeCtx, cancel := context.WithTimeout(ctx, gatewayObservationTimeout)
	defer cancel()
	snapshot, err := client.GetHTTPRouteStatus(probeCtx, namespace, resourceName)
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

	route.DNSStatus = observeGatewayRouteDNS(ctx, route)
	observeGatewayRouteCertificate(probeCtx, client, cluster, namespace, resourceName, &route)
	return route
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
