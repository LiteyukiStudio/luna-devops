package kubernetes

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureNamespaceCreatesNamespace(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())

	err := client.EnsureNamespace(context.Background(), "liteyuki-build", map[string]string{
		"app.kubernetes.io/managed-by": "liteyuki-devops",
	})
	if err != nil {
		t.Fatalf("EnsureNamespace returned error: %v", err)
	}

	namespace, err := client.client.CoreV1().Namespaces().Get(context.Background(), "liteyuki-build", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if namespace.Labels["app.kubernetes.io/managed-by"] != "liteyuki-devops" {
		t.Fatalf("labels = %#v", namespace.Labels)
	}
}

func TestPingReadsServerVersion(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
}

func TestEnsureNamespaceIsIdempotentAndMergesLabels(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	ctx := context.Background()

	if err := client.EnsureNamespace(ctx, "liteyuki-build", map[string]string{"existing": "true"}); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	if err := client.EnsureNamespace(ctx, "liteyuki-build", map[string]string{"managed": "true"}); err != nil {
		t.Fatalf("update namespace: %v", err)
	}

	namespace, err := client.client.CoreV1().Namespaces().Get(ctx, "liteyuki-build", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if namespace.Labels["existing"] != "true" || namespace.Labels["managed"] != "true" {
		t.Fatalf("labels = %#v", namespace.Labels)
	}
}

func TestEnsureNamespaceRejectsInvalidName(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())

	if err := client.EnsureNamespace(context.Background(), "Invalid_Name", nil); err == nil {
		t.Fatal("expected invalid namespace name to fail")
	}
}
