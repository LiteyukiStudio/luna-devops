package kubernetes

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureKubectlGatewaySystemNamespaceCreatesOwnedNamespace(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	if err := client.EnsureKubectlGatewaySystemNamespace(t.Context()); err != nil {
		t.Fatalf("EnsureKubectlGatewaySystemNamespace() error = %v", err)
	}
	namespace, err := client.client.CoreV1().Namespaces().Get(t.Context(), KubectlGatewaySystemNamespaceName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if namespace.Labels[SystemComponentLabel] != kubectlGatewaySystemComponentName || namespace.Labels[ManagedByLabel] != ManagedByValue {
		t.Fatalf("namespace labels = %#v", namespace.Labels)
	}
}

func TestEnsureKubectlGatewaySystemNamespaceRejectsForeignNamespace(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewaySystemNamespaceName},
	}))
	err := client.EnsureKubectlGatewaySystemNamespace(t.Context())
	if !errors.Is(err, ErrKubectlGatewaySystemNamespaceConflict) {
		t.Fatalf("EnsureKubectlGatewaySystemNamespace() error = %v, want conflict", err)
	}
}
