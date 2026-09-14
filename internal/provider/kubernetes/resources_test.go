package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestListManagedResourcesExhaustsKubernetesPages(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	var requests []metav1.ListOptions
	clientset.PrependReactor("list", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listAction, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			return true, nil, fmt.Errorf("action type = %T", action)
		}
		options := listAction.GetListOptions()
		requests = append(requests, options)
		switch options.Continue {
		case "":
			return true, &corev1.NamespaceList{
				ListMeta: metav1.ListMeta{Continue: "next"},
				Items: []corev1.Namespace{{ObjectMeta: metav1.ObjectMeta{
					Name: "luna-project-a", Labels: map[string]string{ManagedByLabel: ManagedByValue},
				}}},
			}, nil
		case "next":
			return true, &corev1.NamespaceList{Items: []corev1.Namespace{{ObjectMeta: metav1.ObjectMeta{
				Name: "luna-project-b", Labels: map[string]string{ManagedByLabel: ManagedByValue},
			}}}}, nil
		default:
			return true, nil, fmt.Errorf("unexpected continue token %q", options.Continue)
		}
	})

	items, err := NewClientForInterface(clientset).ListManagedResources(context.Background(), ResourceListOptions{Kind: "namespaces"})
	if err != nil {
		t.Fatalf("ListManagedResources returned error: %v", err)
	}
	if len(items) != 2 || items[0].Name != "luna-project-a" || items[1].Name != "luna-project-b" {
		t.Fatalf("items = %#v", items)
	}
	if len(requests) != 2 || requests[0].Continue != "" || requests[1].Continue != "next" {
		t.Fatalf("list requests = %#v", requests)
	}
	for _, request := range requests {
		if request.Limit <= 0 {
			t.Fatalf("Kubernetes list limit = %d, want a bounded page", request.Limit)
		}
	}
}

func TestListManagedResourceEventsPageUsesKubernetesLimitAndRemainingCount(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "app-pod", Namespace: "ns-demo", Labels: map[string]string{ManagedByLabel: ManagedByValue},
	}}
	clientset := fake.NewSimpleClientset(pod)
	clientset.PrependReactor("list", "events", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listAction, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			t.Fatalf("action type = %T", action)
		}
		if limit := listAction.GetListOptions().Limit; limit != 20 {
			t.Fatalf("Kubernetes event list limit = %d, want 20", limit)
		}
		remaining := int64(3)
		return true, &corev1.EventList{
			ListMeta: metav1.ListMeta{RemainingItemCount: &remaining},
			Items:    []corev1.Event{{ObjectMeta: metav1.ObjectMeta{Name: "event-1", Namespace: "ns-demo"}}},
		}, nil
	})

	page, _, err := NewClientForInterface(clientset).ListManagedResourceEventsPage(context.Background(), "pod", "ns-demo", "app-pod", 20)
	if err != nil {
		t.Fatalf("ListManagedResourceEventsPage returned error: %v", err)
	}
	if len(page.Items) != 1 || page.Remaining != 3 {
		t.Fatalf("page = %#v", page)
	}
}

func TestGetManagedResourceYAMLRedactsSecretData(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-secret",
			Namespace: "ns-demo",
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
				ProjectIDLabel: "prj_demo",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte("super-secret"),
		},
	}))

	content, snapshot, err := client.GetManagedResourceYAML(context.Background(), "secret", "ns-demo", "app-secret")
	if err != nil {
		t.Fatalf("GetManagedResourceYAML returned error: %v", err)
	}
	if snapshot.Kind != "Secret" || snapshot.ProjectID != "prj_demo" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if strings.Contains(content, "super-secret") || strings.Contains(content, "c3VwZXItc2VjcmV0") {
		t.Fatalf("secret data was not redacted: %s", content)
	}
	if !strings.Contains(content, "stringData:") || !strings.Contains(content, "password: <redacted>") {
		t.Fatalf("redacted secret key missing from yaml: %s", content)
	}
}

func TestListManagedWorkloadsExcludesBuildPods(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dplt-api",
				Namespace: "ns-demo",
				Labels: map[string]string{
					ManagedByLabel:          ManagedByValue,
					ProjectIDLabel:          "prj_demo",
					ApplicationIDLabel:      "app_api",
					DeploymentTargetIDLabel: "dplt_api",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dplt-api-pod",
				Namespace: "ns-demo",
				Labels: map[string]string{
					ManagedByLabel:          ManagedByValue,
					ProjectIDLabel:          "prj_demo",
					ApplicationIDLabel:      "app_api",
					DeploymentTargetIDLabel: "dplt_api",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "build-job-pod",
				Namespace: "ns-demo",
				Labels: map[string]string{
					ManagedByLabel:          ManagedByValue,
					ProjectIDLabel:          "prj_demo",
					ApplicationIDLabel:      "app_api",
					DeploymentTargetIDLabel: "dplt_api",
					ScopeLabel:              "build",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodFailed},
		},
	)
	client := NewClientForInterface(clientset)

	items, err := client.ListManagedResources(context.Background(), ResourceListOptions{
		Kind:          "workloads",
		Namespace:     "ns-demo",
		ProjectID:     "prj_demo",
		ApplicationID: "app_api",
	})
	if err != nil {
		t.Fatalf("ListManagedResources returned error: %v", err)
	}
	for _, item := range items {
		if item.Name == "build-job-pod" {
			t.Fatalf("build pod should be excluded from runtime workloads: %#v", items)
		}
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	for _, action := range clientset.Actions() {
		if action.GetVerb() != "list" {
			continue
		}
		listAction, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			t.Fatalf("action type = %T", action)
		}
		if limit := listAction.GetListOptions().Limit; limit <= 0 {
			t.Fatalf("%s list limit = %d, want a bounded page", action.GetResource().Resource, limit)
		}
	}
}

func TestListManagedServicesIncludesHTTPRoutesAndGateways(t *testing.T) {
	client := NewClientForInterfaces(fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dplt-api",
			Namespace: "ns-demo",
			Labels: map[string]string{
				ManagedByLabel:      ManagedByValue,
				ProjectIDLabel:      "prj_demo",
				GatewayRouteIDLabel: "gwr_demo",
			},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
	}), newGatewayAPIDynamicClient(
		gatewayAPIClass(),
		&unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":      "luna-gateway-gwr-demo",
				"namespace": "ns-demo",
				"labels": map[string]any{
					ManagedByLabel:      ManagedByValue,
					ProjectIDLabel:      "prj_demo",
					GatewayRouteIDLabel: "gwr_demo",
				},
			},
			"spec": map[string]any{
				"hostnames": []any{"api.example.com"},
			},
			"status": map[string]any{
				"parents": []any{map[string]any{
					"conditions": []any{map[string]any{"type": "Accepted", "status": "True"}},
				}},
			},
		}},
	))
	if err := client.EnsureGateway(context.Background(), GatewaySpec{
		Name:             "luna-gateway",
		Namespace:        "kube-system",
		GatewayClassName: "traefik",
		ProjectID:        "prj_demo",
	}); err != nil {
		t.Fatalf("EnsureGateway returned error: %v", err)
	}

	items, err := client.ListManagedResources(context.Background(), ResourceListOptions{Kind: "services", ProjectID: "prj_demo"})
	if err != nil {
		t.Fatalf("ListManagedResources returned error: %v", err)
	}
	kinds := map[string]bool{}
	for _, item := range items {
		kinds[item.Kind] = true
	}
	if !kinds["Service"] || !kinds["HTTPRoute"] || !kinds["Gateway"] {
		t.Fatalf("items = %#v", items)
	}
}

func TestRuntimePodExcludesBuildPods(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "build-job-pod",
				Namespace: "ns-demo",
				Labels: map[string]string{
					ManagedByLabel:          ManagedByValue,
					DeploymentTargetIDLabel: "dplt_api",
					ScopeLabel:              "build",
				},
			},
			Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "executor"}}},
			Status: corev1.PodStatus{Phase: corev1.PodFailed},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "runtime-pod",
				Namespace: "ns-demo",
				Labels: map[string]string{
					ManagedByLabel:          ManagedByValue,
					DeploymentTargetIDLabel: "dplt_api",
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				}},
			},
		},
	))

	pod, container, err := client.runtimePod(context.Background(), "ns-demo", "dplt_api", "")
	if err != nil {
		t.Fatalf("runtimePod returned error: %v", err)
	}
	if pod.Name != "runtime-pod" || container != "app" {
		t.Fatalf("selected pod/container = %s/%s", pod.Name, container)
	}
}
