package kubernetes

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubectlManagedCleanupCatalogTracksEveryDeletableBuiltin(t *testing.T) {
	items := kubectlManagedCleanupCatalog(nil)
	available := make(map[schema.GroupVersionResource]bool, len(items))
	for _, item := range items {
		available[item] = true
	}
	for _, expected := range []schema.GroupVersionResource{
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
		{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"},
		{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"},
	} {
		if !available[expected] {
			t.Fatalf("deletable built-in %s is missing from cleanup catalog", expected.String())
		}
	}
	if available[schema.GroupVersionResource{Version: "v1", Resource: "events"}] {
		t.Fatal("read-only events must not be treated as kubectl-owned cleanup targets")
	}
}

func TestCleanupKubectlManagedResourcesDeletesOnlyKubectlOwnedObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	listKinds := map[schema.GroupVersionResource]string{}
	for _, item := range kubectlManagedCleanupCatalog(nil) {
		listKinds[item] = "List"
	}
	listKinds[gvr] = "DeploymentList"
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "kubectl-app",
				"namespace": "project-demo",
				"labels": map[string]any{
					ManagedByLabel:                      ManagedByValue,
					KubectlGatewayManagementSourceLabel: KubectlGatewayManagementSourceValue,
					ProjectIDLabel:                      "prj_demo",
				},
			},
		}},
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "platform-app",
				"namespace": "project-demo",
				"labels": map[string]any{
					ManagedByLabel:                      ManagedByValue,
					KubectlGatewayManagementSourceLabel: PlatformManagementSourceValue,
					ProjectIDLabel:                      "prj_demo",
				},
			},
		}},
	)
	client := NewClientForInterfaces(fake.NewSimpleClientset(), dynamicClient)
	result, err := client.CleanupKubectlManagedResources(t.Context(), KubectlManagedCleanupSpec{
		ProjectID: "prj_demo",
		Namespace: "project-demo",
	})
	if err != nil {
		t.Fatalf("CleanupKubectlManagedResources() error = %v", err)
	}
	if result.Deleted != 1 || result.ByGVR[gvr] != 1 {
		t.Fatalf("cleanup result = %#v", result)
	}
	list, err := dynamicClient.Resource(gvr).Namespace("project-demo").List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list remaining deployments: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].GetName() != "platform-app" {
		t.Fatalf("remaining deployments = %#v", list.Items)
	}
}

func TestCleanupManagedResourcesDoesNotDependOnCurrentProjectBindings(t *testing.T) {
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	listKinds := map[schema.GroupVersionResource]string{}
	for _, item := range kubectlManagedCleanupCatalog(nil) {
		listKinds[item] = "List"
	}
	listKinds[gvr] = "DeploymentList"
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "historical-kubectl-app",
				"namespace": "project-removed-before-cluster-delete",
				"labels": map[string]any{
					ManagedByLabel:                      ManagedByValue,
					KubectlGatewayManagementSourceLabel: KubectlGatewayManagementSourceValue,
					ProjectIDLabel:                      "prj_historical",
				},
			},
		}},
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "unowned-lookalike",
				"namespace": "project-removed-before-cluster-delete",
				"labels": map[string]any{
					KubectlGatewayManagementSourceLabel: KubectlGatewayManagementSourceValue,
					ProjectIDLabel:                      "prj_historical",
				},
			},
		}},
	)
	manager := NewKubectlGatewayManager(NewClientForInterfaces(fake.NewSimpleClientset(), dynamicClient))
	if err := manager.CleanupManagedResources(t.Context(), GatewayAccessSpec{}); err != nil {
		t.Fatalf("CleanupManagedResources() error = %v", err)
	}
	list, err := dynamicClient.Resource(gvr).Namespace("project-removed-before-cluster-delete").List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list remaining deployments: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].GetName() != "unowned-lookalike" {
		t.Fatalf("remaining deployments = %#v", list.Items)
	}
}
