package kubernetes

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureNamespaceCreatesNamespace(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())

	err := client.EnsureNamespace(context.Background(), "luna-build", map[string]string{
		ManagedByLabel: ManagedByValue,
		ProjectIDLabel: "prj_build",
	})
	if err != nil {
		t.Fatalf("EnsureNamespace returned error: %v", err)
	}

	namespace, err := client.client.CoreV1().Namespaces().Get(context.Background(), "luna-build", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if namespace.Labels["app.kubernetes.io/managed-by"] != "luna-devops" {
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

	labels := ProjectNamespaceLabels("prj_build")
	labels["existing"] = "true"
	if err := client.EnsureNamespace(ctx, "luna-build", labels); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	labels = ProjectNamespaceLabels("prj_build")
	labels["managed"] = "true"
	if err := client.EnsureNamespace(ctx, "luna-build", labels); err != nil {
		t.Fatalf("update namespace: %v", err)
	}

	namespace, err := client.client.CoreV1().Namespaces().Get(ctx, "luna-build", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if namespace.Labels["existing"] != "true" || namespace.Labels["managed"] != "true" {
		t.Fatalf("labels = %#v", namespace.Labels)
	}
}

func TestEnsureNamespaceRejectsDifferentProjectOwner(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	ctx := context.Background()

	if err := client.EnsureNamespace(ctx, "luna-project", ProjectNamespaceLabels("prj_old")); err != nil {
		t.Fatalf("create namespace: %v", err)
	}
	err := client.EnsureNamespace(ctx, "luna-project", ProjectNamespaceLabels("prj_new"))
	if err == nil || !strings.Contains(err.Error(), ResourceOwnershipConflictCode) {
		t.Fatalf("expected ownership conflict, got %v", err)
	}
}

func TestEnsureNamespaceRejectsUnmanagedExistingNamespace(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "luna-project"}}))

	err := client.EnsureNamespace(context.Background(), "luna-project", ProjectNamespaceLabels("prj_new"))
	if err == nil || !strings.Contains(err.Error(), ResourceOwnershipConflictCode) {
		t.Fatalf("expected ownership conflict, got %v", err)
	}
}

func TestEnsureNamespaceRejectsInvalidName(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())

	if err := client.EnsureNamespace(context.Background(), "Invalid_Name", nil); err == nil {
		t.Fatal("expected invalid namespace name to fail")
	}
}
