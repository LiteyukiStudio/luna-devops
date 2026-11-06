package kubernetes

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestReleaseGatewayCertificateKeepsSharedSecretUntilLastCertificate(t *testing.T) {
	ctx := context.Background()
	kube := kubefake.NewSimpleClientset(
		issuedSecret("kube-system", "tls-api-example-com", "luna-gateway-gwr-a"),
		issuedSecret("kube-system", "wildcard-example-com", "wildcard-example-com"),
	)
	dynamicClient := newGatewayCertificateDynamicClient(
		managedCertificate("kube-system", "luna-gateway-gwr-a", "tls-api-example-com"),
		managedCertificate("kube-system", "luna-gateway-gwr-b", "tls-api-example-com"),
	)
	client := NewClientForInterfaces(kube, dynamicClient)
	createGatewayWithCertificateRefs(t, client, "tls-api-example-com", "wildcard-example-com")
	release := func(name string) GatewayCertificateReleaseResult {
		t.Helper()
		result, err := client.ReleaseGatewayCertificate(ctx, GatewayCertificateRelease{
			GatewayName:      "luna-gateway",
			GatewayNamespace: "kube-system",
			Namespace:        "kube-system",
			Name:             name,
			SecretName:       "tls-api-example-com",
		})
		if err != nil {
			t.Fatalf("ReleaseGatewayCertificate(%s) error = %v", name, err)
		}
		return result
	}

	first := release("luna-gateway-gwr-a")
	if !first.CertificateDeleted || first.SecretDeleted || first.GatewayRefRemoved {
		t.Fatalf("first release = %#v, want only certificate deleted", first)
	}
	if _, err := kube.CoreV1().Secrets("kube-system").Get(ctx, "tls-api-example-com", metav1.GetOptions{}); err != nil {
		t.Fatalf("shared secret removed while another certificate still uses it: %v", err)
	}
	if refs := gatewayCertificateRefNames(t, client); len(refs) != 2 {
		t.Fatalf("gateway refs after first release = %#v", refs)
	}

	second := release("luna-gateway-gwr-b")
	if !second.CertificateDeleted || !second.SecretDeleted || !second.GatewayRefRemoved || second.SecretName != "tls-api-example-com" {
		t.Fatalf("second release = %#v, want certificate, secret and gateway ref released", second)
	}
	if _, err := kube.CoreV1().Secrets("kube-system").Get(ctx, "tls-api-example-com", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("secret after last release err = %v, want not found", err)
	}
	if refs := gatewayCertificateRefNames(t, client); len(refs) != 1 || refs[0] != "wildcard-example-com" {
		t.Fatalf("gateway refs after last release = %#v, want only wildcard", refs)
	}

	if again := release("luna-gateway-gwr-b"); again.Changed() {
		t.Fatalf("repeated release = %#v, want no change", again)
	}
}

func TestReleaseGatewayCertificateResumesAfterCertificateWasDeleted(t *testing.T) {
	ctx := context.Background()
	kube := kubefake.NewSimpleClientset(issuedSecret("kube-system", "wildcard-example-com", "wildcard-example-com"))
	client := NewClientForInterfaces(kube, newGatewayCertificateDynamicClient())
	createGatewayWithCertificateRefs(t, client, "wildcard-example-com")

	result, err := client.ReleaseGatewayCertificate(ctx, GatewayCertificateRelease{
		GatewayName:      "luna-gateway",
		GatewayNamespace: "kube-system",
		Namespace:        "kube-system",
		Name:             "wildcard-example-com",
		SecretName:       "wildcard-example-com",
	})
	if err != nil {
		t.Fatalf("ReleaseGatewayCertificate() error = %v", err)
	}
	if result.CertificateDeleted || !result.SecretDeleted || !result.GatewayRefRemoved {
		t.Fatalf("result = %#v, want secret and gateway ref released", result)
	}
	gateway, err := client.dynamic.Resource(gatewayGVR).Namespace("kube-system").Get(ctx, "luna-gateway", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get gateway: %v", err)
	}
	listeners, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	if _, hasTLS := listeners[1].(map[string]any)["tls"]; hasTLS {
		t.Fatalf("listener kept empty tls block: %#v", listeners[1])
	}
}

func TestReleaseGatewayCertificateSkipsResourcesNotOwnedByPlatform(t *testing.T) {
	ctx := context.Background()
	userSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "tls-manual", Namespace: "kube-system"}}
	foreign := managedCertificate("kube-system", "luna-gateway-gwr-c", "tls-other")
	foreign.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "someone-else"})
	kube := kubefake.NewSimpleClientset(userSecret)
	client := NewClientForInterfaces(kube, newGatewayCertificateDynamicClient(foreign))
	createGatewayWithCertificateRefs(t, client, "tls-manual")

	result, err := client.ReleaseGatewayCertificate(ctx, GatewayCertificateRelease{
		GatewayName: "luna-gateway", GatewayNamespace: "kube-system", Namespace: "kube-system", Name: "luna-gateway-gwr-c", SecretName: "tls-other",
	})
	if err != nil || result.Changed() {
		t.Fatalf("foreign certificate release = %#v, %v; want untouched", result, err)
	}
	if _, err := client.dynamic.Resource(certificateResource).Namespace("kube-system").Get(ctx, "luna-gateway-gwr-c", metav1.GetOptions{}); err != nil {
		t.Fatalf("foreign certificate was deleted: %v", err)
	}

	result, err = client.ReleaseGatewayCertificate(ctx, GatewayCertificateRelease{
		GatewayName: "luna-gateway", GatewayNamespace: "kube-system", Namespace: "kube-system", Name: "luna-gateway-gwr-d", SecretName: "tls-manual",
	})
	if err != nil {
		t.Fatalf("ReleaseGatewayCertificate() error = %v", err)
	}
	if result.SecretDeleted {
		t.Fatalf("secret not issued by cert-manager was deleted: %#v", result)
	}
	if _, err := kube.CoreV1().Secrets("kube-system").Get(ctx, "tls-manual", metav1.GetOptions{}); err != nil {
		t.Fatalf("user secret removed: %v", err)
	}
}

func TestHasManagedHTTPRoutesIgnoresUnmanagedRoutes(t *testing.T) {
	ctx := context.Background()
	unmanaged := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata":   map[string]any{"name": "manual", "namespace": "ns-demo"},
	}}
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(), newGatewayCertificateDynamicClient(unmanaged))
	if inUse, err := client.HasManagedHTTPRoutes(ctx); err != nil || inUse {
		t.Fatalf("HasManagedHTTPRoutes() = %v, %v; want false", inUse, err)
	}
	managed := unmanaged.DeepCopy()
	managed.SetName("luna-gateway-gwr-a")
	managed.SetLabels(map[string]string{ManagedByLabel: ManagedByValue})
	client = NewClientForInterfaces(kubefake.NewSimpleClientset(), newGatewayCertificateDynamicClient(unmanaged, managed))
	if inUse, err := client.HasManagedHTTPRoutes(ctx); err != nil || !inUse {
		t.Fatalf("HasManagedHTTPRoutes() = %v, %v; want true", inUse, err)
	}
}

func newGatewayCertificateDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		certificateResource: "CertificateList",
		gatewayClassGVR:     "GatewayClassList",
		gatewayGVR:          "GatewayList",
		httpRouteGVR:        "HTTPRouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
}

func managedCertificate(namespace, name, secretName string) *unstructured.Unstructured {
	certificate := certificateObject(CertificateSpec{
		Name:          name,
		Namespace:     namespace,
		ProjectID:     "prj_demo",
		RouteID:       name,
		Host:          "api.example.com",
		SecretName:    secretName,
		ClusterIssuer: "letsencrypt",
	})
	return certificate
}

func issuedSecret(namespace, name, certificateName string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:        name,
		Namespace:   namespace,
		Annotations: map[string]string{certManagerCertificateNameAnnotation: certificateName},
	}}
}

// createGatewayWithCertificateRefs 通过 API 创建 Gateway；fake tracker 会把 Gateway 的复数猜成 gatewaies，不能直接预置对象。
func createGatewayWithCertificateRefs(t *testing.T, client *Client, secretNames ...string) {
	t.Helper()
	refs := make([]any, 0, len(secretNames))
	for _, secretName := range secretNames {
		refs = append(refs, map[string]any{"name": secretName})
	}
	gateway := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata":   map[string]any{"name": "luna-gateway", "namespace": "kube-system"},
		"spec": map[string]any{
			"gatewayClassName": "traefik",
			"listeners": []any{
				map[string]any{"name": "web", "port": int64(8080), "protocol": "HTTP"},
				map[string]any{
					"name": "websecure", "port": int64(8443), "protocol": "HTTPS",
					"tls": map[string]any{"mode": "Terminate", "certificateRefs": refs},
				},
			},
		},
	}}
	if _, err := client.dynamic.Resource(gatewayGVR).Namespace("kube-system").Create(context.Background(), gateway, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create gateway: %v", err)
	}
}

func gatewayCertificateRefNames(t *testing.T, client *Client) []string {
	t.Helper()
	gateway, err := client.dynamic.Resource(gatewayGVR).Namespace("kube-system").Get(context.Background(), "luna-gateway", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get gateway: %v", err)
	}
	listeners, _, _ := unstructured.NestedSlice(gateway.Object, "spec", "listeners")
	names := []string{}
	for _, item := range listeners {
		for _, ref := range nestedSlice(item.(map[string]any), "tls", "certificateRefs") {
			names = append(names, ref.(map[string]any)["name"].(string))
		}
	}
	return names
}
