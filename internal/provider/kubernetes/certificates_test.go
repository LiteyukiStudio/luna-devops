package kubernetes

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestApplyCertificateCreatesCertManagerCertificate(t *testing.T) {
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(), newCertificateDynamicClient())
	spec := CertificateSpec{
		Name:          "api-dev",
		Namespace:     "project-demo",
		ProjectID:     "prj_demo",
		RouteID:       "gwr_api",
		Host:          "api.example.com",
		SecretName:    "api-dev-tls",
		ClusterIssuer: "letsencrypt-http01",
	}

	if err := client.ApplyCertificate(context.Background(), spec); err != nil {
		t.Fatalf("ApplyCertificate returned error: %v", err)
	}

	certificate, err := client.dynamic.Resource(certificateResource).Namespace(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get certificate: %v", err)
	}
	certSpec, ok := certificate.Object["spec"].(map[string]any)
	if !ok {
		t.Fatalf("spec = %#v", certificate.Object["spec"])
	}
	if certSpec["secretName"] != spec.SecretName {
		t.Fatalf("secretName = %#v", certSpec["secretName"])
	}
	dnsNames := certSpec["dnsNames"].([]any)
	if dnsNames[0] != spec.Host {
		t.Fatalf("dnsNames = %#v", dnsNames)
	}
	issuer := certSpec["issuerRef"].(map[string]any)
	if issuer["name"] != spec.ClusterIssuer || issuer["kind"] != "ClusterIssuer" {
		t.Fatalf("issuerRef = %#v", issuer)
	}
}

func TestApplyCertificateSupportsNamespacedIssuer(t *testing.T) {
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(), newCertificateDynamicClient())
	spec := CertificateSpec{
		Name:          "api-dev",
		Namespace:     "project-demo",
		ProjectID:     "prj_demo",
		RouteID:       "gwr_api",
		Host:          "api.example.com",
		SecretName:    "api-dev-tls",
		IssuerKind:    "Issuer",
		ClusterIssuer: "letsencrypt-http01",
	}

	if err := client.ApplyCertificate(context.Background(), spec); err != nil {
		t.Fatalf("ApplyCertificate returned error: %v", err)
	}

	certificate, err := client.dynamic.Resource(certificateResource).Namespace(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get certificate: %v", err)
	}
	certSpec := certificate.Object["spec"].(map[string]any)
	issuer := certSpec["issuerRef"].(map[string]any)
	if issuer["kind"] != "Issuer" {
		t.Fatalf("issuerRef = %#v", issuer)
	}
}

func TestApplyCertificateSupportsMultipleDNSNames(t *testing.T) {
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(), newCertificateDynamicClient())
	spec := CertificateSpec{
		Name:          "wildcard-apps-example-com",
		Namespace:     "certs",
		ProjectID:     "prj_demo",
		Host:          "apps.example.com",
		DNSNames:      []string{"*.apps.example.com"},
		SecretName:    "wildcard-apps-example-com",
		ClusterIssuer: "letsencrypt-dns01",
	}

	if err := client.ApplyCertificate(context.Background(), spec); err != nil {
		t.Fatalf("ApplyCertificate returned error: %v", err)
	}

	certificate, err := client.dynamic.Resource(certificateResource).Namespace(spec.Namespace).Get(context.Background(), spec.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get certificate: %v", err)
	}
	certSpec := certificate.Object["spec"].(map[string]any)
	dnsNames := certSpec["dnsNames"].([]any)
	if len(dnsNames) != 2 || dnsNames[0] != "apps.example.com" || dnsNames[1] != "*.apps.example.com" {
		t.Fatalf("dnsNames = %#v", dnsNames)
	}
}

func TestCertificateSnapshotMapsReadyCondition(t *testing.T) {
	certificate := certificateWithStatus(map[string]any{
		"notAfter": "2026-06-08T00:00:00Z",
		"conditions": []any{map[string]any{
			"type":    "Ready",
			"status":  "True",
			"message": "issued",
		}},
	})

	snapshot := certificateSnapshot(certificate, time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC))
	if snapshot.Phase != CertificateIssued || snapshot.Message != "issued" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestCertificateSnapshotMapsFailureCondition(t *testing.T) {
	certificate := certificateWithStatus(map[string]any{
		"conditions": []any{map[string]any{
			"type":    "Ready",
			"status":  "False",
			"message": "challenge failed",
		}},
	})

	snapshot := certificateSnapshot(certificate, time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC))
	if snapshot.Phase != CertificateFailed || snapshot.Message != "challenge failed" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestCertificateSnapshotMapsExpiredCertificate(t *testing.T) {
	certificate := certificateWithStatus(map[string]any{
		"notAfter": "2026-06-06T00:00:00Z",
		"conditions": []any{map[string]any{
			"type":   "Ready",
			"status": "True",
		}},
	})

	snapshot := certificateSnapshot(certificate, time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC))
	if snapshot.Phase != CertificateExpired {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func certificateWithStatus(status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{"status": status}}
}

func newCertificateDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		certificateResource: "CertificateList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
}
