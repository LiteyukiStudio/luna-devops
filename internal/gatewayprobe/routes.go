package gatewayprobe

import (
	"context"
	"strings"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

type RouteDiscoverer interface {
	ListRoutes(ctx context.Context) ([]RouteRef, error)
}

type GatewayAPIRouteDiscoverer struct {
	client gatewayclient.Interface
}

func NewGatewayAPIRouteDiscoverer(config *rest.Config) (*GatewayAPIRouteDiscoverer, error) {
	client, err := gatewayclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &GatewayAPIRouteDiscoverer{client: client}, nil
}

func (d *GatewayAPIRouteDiscoverer) ListRoutes(ctx context.Context) ([]RouteRef, error) {
	items, err := d.client.GatewayV1().HTTPRoutes("").List(ctx, metav1.ListOptions{
		LabelSelector: kubeprovider.ManagedByLabel + "=" + kubeprovider.ManagedByValue,
	})
	if err != nil {
		return nil, err
	}
	routes := make([]RouteRef, 0, len(items.Items))
	for _, item := range items.Items {
		routeID := strings.TrimSpace(item.Labels[kubeprovider.GatewayRouteIDLabel])
		if routeID == "" {
			continue
		}
		hostnames := make([]string, 0, len(item.Spec.Hostnames))
		for _, hostname := range item.Spec.Hostnames {
			if text := strings.TrimSpace(string(hostname)); text != "" {
				hostnames = append(hostnames, text)
			}
		}
		routes = append(routes, RouteRef{
			ID:         routeID,
			Namespace:  item.Namespace,
			Name:       item.Name,
			Hostnames:  hostnames,
			Candidates: routeCandidates(routeID, item.Namespace, item.Name, hostnames),
		})
	}
	return routes, nil
}

func routeCandidates(routeID string, namespace string, name string, hostnames []string) []string {
	values := []string{
		routeID,
		name,
		namespace + "-" + name,
		namespace + "_" + name,
		namespace + "/" + name,
	}
	values = append(values, hostnames...)
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}
