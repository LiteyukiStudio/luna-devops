package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type KubectlManagedCleanupSpec struct {
	ProjectID     string                        `json:"projectId"`
	ApplicationID string                        `json:"applicationId,omitempty"`
	Namespace     string                        `json:"namespace,omitempty"`
	AllProjects   bool                          `json:"-"`
	ExtraGVRs     []schema.GroupVersionResource `json:"-"`
}

type KubectlManagedCleanupResult struct {
	Deleted int                                 `json:"deleted"`
	ByGVR   map[schema.GroupVersionResource]int `json:"-"`
}

func kubectlManagedCleanupCatalog(extra []schema.GroupVersionResource) []schema.GroupVersionResource {
	rules := kubecatalog.New().Rules()
	result := make([]schema.GroupVersionResource, 0, len(rules)+len(extra))
	for _, rule := range rules {
		if rule.Local || !rule.Namespaced {
			continue
		}
		if _, deletable := rule.PermissionFor("", "delete"); !deletable {
			if _, deletable = rule.PermissionFor("", "deletecollection"); !deletable {
				continue
			}
		}
		result = append(result, rule.GVR)
	}
	seen := make(map[schema.GroupVersionResource]struct{}, len(result)+len(extra))
	for _, gvr := range result {
		seen[gvr] = struct{}{}
	}
	for _, gvr := range normalizeGatewayManagedGVRs(extra) {
		if _, exists := seen[gvr]; exists {
			continue
		}
		seen[gvr] = struct{}{}
		result = append(result, gvr)
	}
	return result
}

func (c *Client) CleanupKubectlManagedResources(ctx context.Context, spec KubectlManagedCleanupSpec) (KubectlManagedCleanupResult, error) {
	if c == nil || c.dynamic == nil {
		return KubectlManagedCleanupResult{}, fmt.Errorf("kubectl managed cleanup requires a dynamic Kubernetes client")
	}
	spec.ProjectID = strings.TrimSpace(spec.ProjectID)
	spec.ApplicationID = strings.TrimSpace(spec.ApplicationID)
	spec.Namespace = strings.TrimSpace(spec.Namespace)
	if spec.ProjectID == "" && !spec.AllProjects {
		return KubectlManagedCleanupResult{}, fmt.Errorf("project id is required")
	}
	result := KubectlManagedCleanupResult{ByGVR: map[schema.GroupVersionResource]int{}}
	namespaces := []string{spec.Namespace}
	if spec.Namespace == "" {
		namespaces = []string{metav1.NamespaceAll}
	}
	for _, gvr := range kubectlManagedCleanupCatalog(spec.ExtraGVRs) {
		for _, namespace := range namespaces {
			var resource dynamic.ResourceInterface
			if namespace == metav1.NamespaceAll {
				resource = c.dynamic.Resource(gvr)
			} else {
				resource = c.dynamic.Resource(gvr).Namespace(namespace)
			}
			list, err := resource.List(ctx, metav1.ListOptions{LabelSelector: kubectlManagedCleanupSelector(spec)})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return result, err
			}
			for i := range list.Items {
				item := &list.Items[i]
				deleteResource := resource
				if namespace == metav1.NamespaceAll && strings.TrimSpace(item.GetNamespace()) != "" {
					deleteResource = c.dynamic.Resource(gvr).Namespace(item.GetNamespace())
				}
				if err := deleteResource.Delete(ctx, item.GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
					return result, err
				}
				result.Deleted++
				result.ByGVR[gvr]++
			}
		}
	}
	return result, nil
}

func kubectlManagedCleanupSelector(spec KubectlManagedCleanupSpec) string {
	parts := []string{
		ManagedByLabel + "=" + ManagedByValue,
		KubectlGatewayManagementSourceLabel + "=" + KubectlGatewayManagementSourceValue,
	}
	if !spec.AllProjects {
		parts = append(parts, ProjectIDLabel+"="+spec.ProjectID)
	}
	if spec.ApplicationID != "" {
		parts = append(parts, ApplicationIDLabel+"="+spec.ApplicationID)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
