package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GatewayTrafficProbeSpec describes the ServiceAccount/RBAC footprint for a
// workload that needs Gateway API read access. Only identity fields remain; the
// probe workload itself is deployed through the normal application pipeline.
type GatewayTrafficProbeSpec struct {
	Name             string
	Namespace        string
	RuntimeClusterID string
}

func (c *Client) EnsureGatewayTrafficProbeAccess(ctx context.Context, spec GatewayTrafficProbeSpec) error {
	spec.Name = dnsLabel(firstNonEmpty(spec.Name, "luna-gateway-traffic-probe"))
	spec.Namespace = dnsLabel(firstNonEmpty(spec.Namespace, "luna-system"))
	if strings.TrimSpace(spec.RuntimeClusterID) == "" {
		return fmt.Errorf("gateway traffic probe access requires runtime cluster id")
	}
	labels := SystemComponentLabels("gateway-traffic-probe", spec.RuntimeClusterID)
	if err := c.applyGatewayTrafficProbeServiceAccount(ctx, spec, labels); err != nil {
		return err
	}
	return c.applyGatewayTrafficProbeRBAC(ctx, spec, labels)
}

func (c *Client) applyGatewayTrafficProbeServiceAccount(ctx context.Context, spec GatewayTrafficProbeSpec, labels map[string]string) error {
	account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels}}
	existing, err := c.client.CoreV1().ServiceAccounts(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().ServiceAccounts(spec.Namespace).Create(ctx, account, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = mergeStringMaps(existing.Labels, labels)
	_, err = c.client.CoreV1().ServiceAccounts(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyGatewayTrafficProbeRBAC(ctx context.Context, spec GatewayTrafficProbeSpec, labels map[string]string) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Labels: labels},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"services", "endpoints", "pods"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"gateway.networking.k8s.io"}, Resources: []string{"gateways", "httproutes"}, Verbs: []string{"get", "list", "watch"}},
		},
	}
	existingRole, err := c.client.RbacV1().ClusterRoles().Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := c.client.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		existingRole.Labels = mergeStringMaps(existingRole.Labels, labels)
		existingRole.Rules = role.Rules
		if _, err := c.client.RbacV1().ClusterRoles().Update(ctx, existingRole, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Labels: labels},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: spec.Name, Namespace: spec.Namespace}},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: spec.Name},
	}
	existingBinding, err := c.client.RbacV1().ClusterRoleBindings().Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existingBinding.Labels = mergeStringMaps(existingBinding.Labels, labels)
	existingBinding.Subjects = binding.Subjects
	existingBinding.RoleRef = binding.RoleRef
	_, err = c.client.RbacV1().ClusterRoleBindings().Update(ctx, existingBinding, metav1.UpdateOptions{})
	return err
}

func mergeStringMaps(base map[string]string, override map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range base {
		output[key] = value
	}
	for key, value := range override {
		output[key] = value
	}
	return output
}
