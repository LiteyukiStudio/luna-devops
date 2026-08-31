package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DiscoveryScopeResolver lets the platform validate administrator-provided
// extension rules against the authoritative cluster discovery document.
type DiscoveryScopeResolver struct {
	client *Client
}

func NewDiscoveryScopeResolver(client *Client) DiscoveryScopeResolver {
	return DiscoveryScopeResolver{client: client}
}

func (r DiscoveryScopeResolver) IsNamespaced(ctx context.Context, gvr schema.GroupVersionResource) (bool, error) {
	if r.client == nil || r.client.client == nil {
		return false, fmt.Errorf("kubernetes discovery client is unavailable")
	}
	groupVersion := gvr.Version
	if strings.TrimSpace(gvr.Group) != "" {
		groupVersion = gvr.Group + "/" + gvr.Version
	}
	resources, err := r.client.client.Discovery().ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		return false, err
	}
	for _, resource := range resources.APIResources {
		if resource.Name == gvr.Resource {
			return resource.Namespaced, nil
		}
	}
	return false, fmt.Errorf("kubernetes resource %s was not found in discovery", gvr.String())
}
