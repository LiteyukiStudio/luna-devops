package kubernetes

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type GatewayTrafficProbeSpec struct {
	Name              string
	Namespace         string
	RuntimeClusterID  string
	Image             string
	APIBaseURL        string
	ReportToken       string
	ControllerType    string
	Mode              string
	GatewayNamespace  string
	TraefikMetricsURL string
}

func (c *Client) ApplyGatewayTrafficProbe(ctx context.Context, spec GatewayTrafficProbeSpec) error {
	spec.Name = dnsLabel(firstNonEmpty(spec.Name, "liteyuki-gateway-traffic-probe"))
	spec.Namespace = dnsLabel(firstNonEmpty(spec.Namespace, "liteyuki-system"))
	if strings.TrimSpace(spec.RuntimeClusterID) == "" || strings.TrimSpace(spec.APIBaseURL) == "" || strings.TrimSpace(spec.ReportToken) == "" {
		return fmt.Errorf("gateway traffic probe requires runtime cluster id, API base URL, and report token")
	}
	if strings.TrimSpace(spec.Image) == "" {
		spec.Image = "liteyukistudio/devops-gateway-traffic-probe:nightly"
	}
	if strings.TrimSpace(spec.Mode) == "" {
		spec.Mode = "traefik-metrics"
	}
	if strings.TrimSpace(spec.ControllerType) == "" {
		spec.ControllerType = "traefik"
	}
	if strings.TrimSpace(spec.GatewayNamespace) == "" {
		spec.GatewayNamespace = "kube-system"
	}
	if strings.TrimSpace(spec.TraefikMetricsURL) == "" {
		spec.TraefikMetricsURL = "http://traefik." + spec.GatewayNamespace + ".svc.cluster.local:9100/metrics"
	}
	labels := SystemComponentLabels("gateway-traffic-probe", spec.RuntimeClusterID)
	if err := c.EnsureNamespace(ctx, spec.Namespace, labels); err != nil {
		return err
	}
	if err := c.applyGatewayTrafficProbeServiceAccount(ctx, spec, labels); err != nil {
		return err
	}
	if err := c.applyGatewayTrafficProbeRBAC(ctx, spec, labels); err != nil {
		return err
	}
	if err := c.applyGatewayTrafficProbeConfigMap(ctx, spec, labels); err != nil {
		return err
	}
	if err := c.applyGatewayTrafficProbeSecret(ctx, spec, labels); err != nil {
		return err
	}
	return c.applyGatewayTrafficProbeDeployment(ctx, spec, labels)
}

func (c *Client) EnsureGatewayTrafficProbeAccess(ctx context.Context, spec GatewayTrafficProbeSpec) error {
	spec.Name = dnsLabel(firstNonEmpty(spec.Name, "liteyuki-gateway-traffic-probe"))
	spec.Namespace = dnsLabel(firstNonEmpty(spec.Namespace, "liteyuki-system"))
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

func (c *Client) applyGatewayTrafficProbeConfigMap(ctx context.Context, spec GatewayTrafficProbeSpec, labels map[string]string) error {
	data := map[string]string{
		"API_BASE_URL":        strings.TrimRight(spec.APIBaseURL, "/"),
		"CONTROLLER_TYPE":     spec.ControllerType,
		"MODE":                spec.Mode,
		"GATEWAY_NAMESPACE":   spec.GatewayNamespace,
		"TRAEFIK_METRICS_URL": spec.TraefikMetricsURL,
		"PROBE_ADDR":          ":9090",
		"SCRAPE_INTERVAL":     "60s",
	}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels}, Data: data}
	existing, err := c.client.CoreV1().ConfigMaps(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = mergeStringMaps(existing.Labels, labels)
	existing.Data = data
	_, err = c.client.CoreV1().ConfigMaps(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyGatewayTrafficProbeSecret(ctx context.Context, spec GatewayTrafficProbeSpec, labels map[string]string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{"REPORT_TOKEN": spec.ReportToken},
	}
	existing, err := c.client.CoreV1().Secrets(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().Secrets(spec.Namespace).Create(ctx, secret, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = mergeStringMaps(existing.Labels, labels)
	existing.StringData = secret.StringData
	_, err = c.client.CoreV1().Secrets(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyGatewayTrafficProbeDeployment(ctx context.Context, spec GatewayTrafficProbeSpec, labels map[string]string) error {
	replicas := int32(1)
	podLabels := mergeStringMaps(labels, map[string]string{"app.kubernetes.io/component": "gateway-traffic-probe"})
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": spec.Name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: mergeStringMaps(podLabels, map[string]string{"app.kubernetes.io/name": spec.Name})},
				Spec: corev1.PodSpec{
					ServiceAccountName:           spec.Name,
					AutomountServiceAccountToken: boolPtr(true),
					Containers: []corev1.Container{{
						Name:            "probe",
						Image:           spec.Image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.Name}}},
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: spec.Name}}},
						},
						Env: []corev1.EnvVar{
							{Name: "COMPONENT_ID", Value: "gateway-traffic-probe"},
							{Name: "RUNTIME_CLUSTER_ID", Value: spec.RuntimeClusterID},
						},
						Ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}},
						ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.FromString("metrics"),
						}}},
					}},
				},
			},
		},
	}
	existing, err := c.client.AppsV1().Deployments(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.AppsV1().Deployments(spec.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Labels = mergeStringMaps(existing.Labels, labels)
	existing.Spec = deployment.Spec
	_, err = c.client.AppsV1().Deployments(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
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
