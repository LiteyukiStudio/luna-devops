package kubepolicy

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGatewayRouteValidatesRequestMirrorBackendOwnership(t *testing.T) {
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata":   map[string]any{"namespace": "project-a", "name": "route"},
		"spec": map[string]any{
			"hostnames":  []any{"app.example.com"},
			"parentRefs": []any{map[string]any{"name": "shared-gateway"}},
			"rules": []any{map[string]any{"filters": []any{map[string]any{
				"type": "RequestMirror", "requestMirror": map[string]any{"backendRef": map[string]any{"name": "foreign"}},
			}}}},
		},
	}}
	policy := PolicyContext{
		Namespace: "project-a", ProjectID: "p1", ApplicationID: "a1", AllowedDomains: []string{"*.example.com"},
		AllowedGatewayParents: map[string]struct{}{GatewayParentKey("project-a", "shared-gateway"): {}},
		Resolver: referenceResolver{"services/foreign": &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
			Namespace: "project-a", Labels: map[string]string{ManagedByLabel: ManagedByValue, ProjectIDLabel: "p1", ApplicationIDLabel: "a2"},
		}}},
	}
	if errors := ValidateGateway(t.Context(), policy, object); len(errors) == 0 {
		t.Fatal("request mirror to another application must be rejected")
	}
}

func TestGatewayRouteRejectsExtensionFilter(t *testing.T) {
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "GRPCRoute",
		"metadata": map[string]any{"namespace": "project-a", "name": "route"},
		"spec": map[string]any{
			"hostnames": []any{"grpc.example.com"}, "parentRefs": []any{map[string]any{"name": "shared-gateway"}},
			"rules": []any{
				map[string]any{
					"filters": []any{
						map[string]any{
							"type":         "ExtensionRef",
							"extensionRef": map[string]any{"group": "example.io", "kind": "Policy", "name": "unsafe"},
						},
					},
				},
			},
		},
	}}
	policy := PolicyContext{Namespace: "project-a", ProjectID: "p1", AllowedDomains: []string{"*.example.com"}, AllowedGatewayParents: map[string]struct{}{GatewayParentKey("project-a", "shared-gateway"): {}}}
	if errors := ValidateGateway(t.Context(), policy, object); len(errors) == 0 {
		t.Fatal("extension filter must be rejected")
	}
}

func TestGatewayRouteAllowsOnlyExactConfiguredCrossNamespaceParent(t *testing.T) {
	for _, kind := range []string{"HTTPRoute", "GRPCRoute"} {
		t.Run(kind, func(t *testing.T) {
			object := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "gateway.networking.k8s.io/v1", "kind": kind,
				"metadata": map[string]any{"namespace": "project-a", "name": "route"},
				"spec": map[string]any{
					"hostnames": []any{"app.example.com"},
					"parentRefs": []any{map[string]any{
						"group": "gateway.networking.k8s.io", "kind": "Gateway", "namespace": "kube-system", "name": "luna-gateway",
					}},
				},
			}}
			policy := PolicyContext{
				Namespace: "project-a", ProjectID: "p1", AllowedDomains: []string{"*.example.com"},
				AllowedGatewayParents: map[string]struct{}{GatewayParentKey("kube-system", "luna-gateway"): {}},
			}
			if errors := ValidateGateway(t.Context(), policy, object); len(errors) != 0 {
				t.Fatalf("configured cross-namespace parent was rejected: %#v", errors)
			}
			policy.AllowedGatewayParents = map[string]struct{}{GatewayParentKey("other-system", "luna-gateway"): {}}
			if errors := ValidateGateway(t.Context(), policy, object); len(errors) == 0 {
				t.Fatal("same-name Gateway in an unconfigured namespace must be rejected")
			}
		})
	}
}
