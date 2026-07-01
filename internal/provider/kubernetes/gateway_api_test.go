package kubernetes

import (
	"context"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestApplyGatewayAPIResourcesCreatesGatewayAndHTTPRoute(t *testing.T) {
	client := newGatewayAPITestClient()

	if err := client.EnsureGateway(context.Background(), GatewaySpec{
		Name:             "liteyuki-gateway",
		Namespace:        "kube-system",
		GatewayClassName: "traefik",
		ProjectID:        "prj_demo",
	}); err != nil {
		t.Fatalf("EnsureGateway returned error: %v", err)
	}

	if err := client.ApplyHTTPRoute(context.Background(), HTTPRouteSpec{
		Name:                   "liteyuki-gateway-gwr-demo",
		Namespace:              "ns-demo",
		ProjectID:              "prj_demo",
		ApplicationID:          "app_api",
		EnvironmentID:          "env_prod",
		DeploymentTargetID:     "dplt_api",
		RouteID:                "gwr_demo",
		Host:                   "api.example.com",
		Path:                   "/api",
		PathMatchType:          "PathPrefix",
		ParentGatewayName:      "liteyuki-gateway",
		ParentGatewayNamespace: "kube-system",
		ServiceName:            "dplt-api",
		ServicePort:            8080,
		BackendWeight:          20,
		RequestHeaders:         map[string]string{"X-Forwarded-Proto": "https"},
		ResponseHeaders:        map[string]string{"X-Frame-Options": "DENY"},
		URLRewrite:             `{"replacePrefixMatch":"/"}`,
	}); err != nil {
		t.Fatalf("ApplyHTTPRoute returned error: %v", err)
	}

	gateway, err := client.dynamic.Resource(gatewayGVR).Namespace("kube-system").Get(context.Background(), "liteyuki-gateway", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get gateway: %v", err)
	}
	gatewaySpec, _, _ := unstructured.NestedMap(gateway.Object, "spec")
	if gatewaySpec["gatewayClassName"] != "traefik" {
		t.Fatalf("gateway spec = %#v", gatewaySpec)
	}
	listeners := gatewaySpec["listeners"].([]any)
	if len(listeners) != 2 {
		t.Fatalf("listeners = %#v", listeners)
	}
	firstListener := listeners[0].(map[string]any)
	secondListener := listeners[1].(map[string]any)
	if firstListener["name"] != "web" || firstListener["port"] != int64(8080) || secondListener["name"] != "websecure" || secondListener["port"] != int64(8443) {
		t.Fatalf("listeners = %#v", listeners)
	}

	route, err := client.dynamic.Resource(httpRouteGVR).Namespace("ns-demo").Get(context.Background(), "liteyuki-gateway-gwr-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get httproute: %v", err)
	}
	hostnames, _, _ := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
	if len(hostnames) != 1 || hostnames[0] != "api.example.com" {
		t.Fatalf("hostnames = %#v", hostnames)
	}
	parentRefs, _, _ := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	parentRef := parentRefs[0].(map[string]any)
	if parentRef["name"] != "liteyuki-gateway" || parentRef["namespace"] != "kube-system" {
		t.Fatalf("parentRefs = %#v", parentRefs)
	}
	rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
	rule := rules[0].(map[string]any)
	backendRefs := rule["backendRefs"].([]any)
	backend := backendRefs[0].(map[string]any)
	if backend["name"] != "dplt-api" || backend["port"] != int64(8080) || backend["weight"] != int64(20) {
		t.Fatalf("backendRefs = %#v", backendRefs)
	}
	filters := rule["filters"].([]any)
	if len(filters) != 3 {
		t.Fatalf("filters = %#v", filters)
	}
}

func TestDetectGatewayAPISupportReturnsFriendlyError(t *testing.T) {
	dynamicClient := newGatewayAPIDynamicClient()
	dynamicClient.PrependReactor("list", "gatewayclasses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gatewayclasses"}, "")
	})
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(), dynamicClient)

	err := client.DetectGatewayAPISupport(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Gateway API CRDs are not installed") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeleteHTTPRouteIgnoresNotFound(t *testing.T) {
	client := newGatewayAPITestClient()

	if err := client.DeleteHTTPRoute(context.Background(), "ns-demo", "missing"); err != nil {
		t.Fatalf("DeleteHTTPRoute returned error: %v", err)
	}
}

func newGatewayAPITestClient() *Client {
	return NewClientForInterfaces(kubefake.NewSimpleClientset(), newGatewayAPIDynamicClient(gatewayAPIClass()))
}

func newGatewayAPIDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		gatewayClassGVR: "GatewayClassList",
		gatewayGVR:      "GatewayList",
		httpRouteGVR:    "HTTPRouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
}

func gatewayAPIClass() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "GatewayClass",
		"metadata": map[string]any{
			"name": "traefik",
		},
		"spec": map[string]any{
			"controllerName": "traefik.io/gateway-controller",
		},
	}}
}
