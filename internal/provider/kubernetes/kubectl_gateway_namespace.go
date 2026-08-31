package kubernetes

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KubectlGatewaySystemNamespaceName = "luna-system"
	kubectlGatewaySystemComponentName = "luna-system"
)

var ErrKubectlGatewaySystemNamespaceConflict = errors.New("kube_gateway.system_namespace_conflict")

func KubectlGatewaySystemNamespaceLabels() map[string]string {
	return map[string]string{
		ManagedByLabel:       ManagedByValue,
		ScopeLabel:           "system",
		SystemResourceLabel:  "true",
		SystemComponentLabel: kubectlGatewaySystemComponentName,
	}
}

// EnsureKubectlGatewaySystemNamespace creates or updates the shared luna-system
// namespace without taking over a same-name namespace that is not already owned
// by Luna DevOps.
func (c *Client) EnsureKubectlGatewaySystemNamespace(ctx context.Context) error {
	if c == nil || c.client == nil {
		return ErrKubectlGatewaySystemNamespaceConflict
	}
	labels := KubectlGatewaySystemNamespaceLabels()
	namespaces := c.client.CoreV1().Namespaces()
	existing, err := namespaces.Get(ctx, KubectlGatewaySystemNamespaceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = namespaces.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   KubectlGatewaySystemNamespaceName,
				Labels: labels,
			},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("Namespace", existing, labels); err != nil {
		return errors.Join(ErrKubectlGatewaySystemNamespaceConflict, err)
	}
	changed := false
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for key, value := range labels {
		if existing.Labels[key] == value {
			continue
		}
		existing.Labels[key] = value
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = namespaces.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}
