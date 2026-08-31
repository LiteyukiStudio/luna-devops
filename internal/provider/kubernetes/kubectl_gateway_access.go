package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	KubectlGatewayManagementSourceLabel = "luna.devops/management-source"
	KubectlGatewayManagementSourceValue = "kubectl"
	PlatformManagementSourceValue       = "platform"

	KubectlGatewaySpecHashAnnotation = "luna.devops/kubectl-gateway-spec-hash"

	KubectlGatewayServiceAccountName          = "luna-kubectl-gateway"
	KubectlGatewayProjectClusterRoleName      = "luna-kubectl-gateway-project"
	KubectlGatewayDiscoveryClusterRoleName    = "luna-kubectl-gateway-discovery"
	KubectlGatewayDiscoveryClusterRoleBinding = "luna-kubectl-gateway-discovery"
	KubectlGatewayProjectRoleBindingName      = "luna-kubectl-gateway"
)

type GatewayAccessProjectSpec struct {
	ProjectID string `json:"projectId"`
	Namespace string `json:"namespace"`
}

type GatewayAccessSpec struct {
	RuntimeClusterID      string                        `json:"runtimeClusterId"`
	Enabled               bool                          `json:"enabled"`
	Projects              []GatewayAccessProjectSpec    `json:"projects"`
	ExtraProjectRules     []rbacv1.PolicyRule           `json:"extraProjectRules,omitempty"`
	ExtraManagedResources []schema.GroupVersionResource `json:"extraManagedResources,omitempty"`
}

type GatewayAccessComponentStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Ready     bool   `json:"ready"`
	Reason    string `json:"reason,omitempty"`
}

type GatewayAccessObservation struct {
	Status     string                         `json:"status"`
	Ready      bool                           `json:"ready"`
	SpecHash   string                         `json:"specHash"`
	Components []GatewayAccessComponentStatus `json:"components"`
}

type KubectlGatewayManager struct {
	client *Client
}

func NewKubectlGatewayManager(client *Client) *KubectlGatewayManager {
	return &KubectlGatewayManager{client: client}
}

func (m *KubectlGatewayManager) ReconcileGatewayAccess(ctx context.Context, spec GatewayAccessSpec) (GatewayAccessObservation, error) {
	spec = normalizeGatewayAccessSpec(spec)
	if !spec.Enabled {
		if err := m.CleanupGatewayAccess(ctx, spec); err != nil {
			return GatewayAccessObservation{}, err
		}
		return m.ObserveGatewayAccess(ctx, spec)
	}
	if m == nil || m.client == nil || m.client.client == nil {
		return GatewayAccessObservation{}, fmt.Errorf("kubectl gateway client is unavailable")
	}
	if err := m.client.EnsureKubectlGatewaySystemNamespace(ctx); err != nil {
		return GatewayAccessObservation{}, err
	}
	hash := spec.SpecHash()
	if err := m.ensureGatewayServiceAccount(ctx, spec.RuntimeClusterID, hash); err != nil {
		return GatewayAccessObservation{}, err
	}
	if err := m.ensureClusterRole(ctx, KubectlGatewayProjectClusterRoleName, m.projectClusterRoleRules(spec), gatewayAccessLabels(spec.RuntimeClusterID), gatewayAccessAnnotations(hash)); err != nil {
		return GatewayAccessObservation{}, err
	}
	if err := m.ensureClusterRole(ctx, KubectlGatewayDiscoveryClusterRoleName, kubectlGatewayDiscoveryRules(), gatewayAccessLabels(spec.RuntimeClusterID), gatewayAccessAnnotations(hash)); err != nil {
		return GatewayAccessObservation{}, err
	}
	if err := m.ensureDiscoveryClusterRoleBinding(ctx, spec.RuntimeClusterID, hash); err != nil {
		return GatewayAccessObservation{}, err
	}
	if err := m.ensureProjectRoleBindings(ctx, spec, hash); err != nil {
		return GatewayAccessObservation{}, err
	}
	return m.ObserveGatewayAccess(ctx, spec)
}

func (m *KubectlGatewayManager) ObserveGatewayAccess(ctx context.Context, spec GatewayAccessSpec) (GatewayAccessObservation, error) {
	spec = normalizeGatewayAccessSpec(spec)
	if m == nil || m.client == nil || m.client.client == nil {
		return GatewayAccessObservation{}, fmt.Errorf("kubectl gateway client is unavailable")
	}
	hash := spec.SpecHash()
	if !spec.Enabled {
		return m.observeDisabledGatewayAccess(ctx, spec, hash)
	}
	components := []GatewayAccessComponentStatus{
		m.observeServiceAccount(ctx, hash),
		m.observeClusterRole(ctx, KubectlGatewayProjectClusterRoleName, hash),
		m.observeClusterRole(ctx, KubectlGatewayDiscoveryClusterRoleName, hash),
		m.observeClusterRoleBinding(ctx, KubectlGatewayDiscoveryClusterRoleBinding, hash),
	}
	for _, project := range spec.Projects {
		components = append(components, m.observeProjectRoleBinding(ctx, project, hash))
	}
	ready := true
	for _, component := range components {
		if !component.Ready {
			ready = false
			return GatewayAccessObservation{Status: "reconciling", Ready: false, SpecHash: hash, Components: components}, nil
		}
	}
	return GatewayAccessObservation{Status: "ready", Ready: ready, SpecHash: hash, Components: components}, nil
}

func (m *KubectlGatewayManager) observeDisabledGatewayAccess(ctx context.Context, spec GatewayAccessSpec, hash string) (GatewayAccessObservation, error) {
	components := []GatewayAccessComponentStatus{
		m.observeServiceAccount(ctx, ""),
		m.observeClusterRole(ctx, KubectlGatewayProjectClusterRoleName, ""),
		m.observeClusterRole(ctx, KubectlGatewayDiscoveryClusterRoleName, ""),
		m.observeClusterRoleBinding(ctx, KubectlGatewayDiscoveryClusterRoleBinding, ""),
	}
	roleBindings, err := m.client.client.RbacV1().RoleBindings(corev1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: selectorFromLabels(gatewayAccessLabels(spec.RuntimeClusterID)),
	})
	if err != nil {
		return GatewayAccessObservation{}, err
	}
	for i := range roleBindings.Items {
		item := &roleBindings.Items[i]
		if item.Name != KubectlGatewayProjectRoleBindingName {
			continue
		}
		components = append(components, gatewayComponentStatus("RoleBinding", item.Name, item.Namespace, item, nil, ""))
	}
	for _, component := range components {
		if component.Ready {
			return GatewayAccessObservation{Status: "reconciling", Ready: false, SpecHash: hash, Components: components}, nil
		}
		if component.Reason == "unavailable" {
			return GatewayAccessObservation{}, fmt.Errorf("observe disabled kubectl gateway component %s: unavailable", component.Kind)
		}
	}
	return GatewayAccessObservation{Status: "disabled", Ready: true, SpecHash: hash, Components: components}, nil
}

func (m *KubectlGatewayManager) CleanupGatewayAccess(ctx context.Context, spec GatewayAccessSpec) error {
	spec = normalizeGatewayAccessSpec(spec)
	if m == nil || m.client == nil || m.client.client == nil {
		return fmt.Errorf("kubectl gateway client is unavailable")
	}
	if err := m.deleteProjectRoleBindings(ctx, spec.RuntimeClusterID); err != nil {
		return err
	}
	if err := m.deleteClusterRoleBinding(ctx, KubectlGatewayDiscoveryClusterRoleBinding); err != nil {
		return err
	}
	if err := m.deleteClusterRole(ctx, KubectlGatewayDiscoveryClusterRoleName); err != nil {
		return err
	}
	if err := m.deleteClusterRole(ctx, KubectlGatewayProjectClusterRoleName); err != nil {
		return err
	}
	if err := m.deleteServiceAccount(ctx, KubectlGatewaySystemNamespaceName, KubectlGatewayServiceAccountName); err != nil {
		return err
	}
	return nil
}

func (m *KubectlGatewayManager) CleanupManagedResources(ctx context.Context, spec GatewayAccessSpec) error {
	spec = normalizeGatewayAccessSpec(spec)
	if m == nil || m.client == nil || m.client.client == nil {
		return fmt.Errorf("kubectl gateway client is unavailable")
	}
	// Runtime-cluster deletion removes bindings before the drain window so new
	// requests and existing streams lose authorization immediately. Therefore
	// cleanup must not depend on the current binding/project snapshot: remove
	// every object created through this cluster's kubectl gateway by its two
	// immutable ownership labels.
	_, err := m.client.CleanupKubectlManagedResources(ctx, KubectlManagedCleanupSpec{
		AllProjects: true, ExtraGVRs: spec.ExtraManagedResources,
	})
	return err
}

// CleanupBindingManagedResources removes kubectl-owned objects for one
// project or application boundary without changing the cluster-wide gateway
// ServiceAccount or shared RoleBindings.
func (m *KubectlGatewayManager) CleanupBindingManagedResources(ctx context.Context, spec KubectlManagedCleanupSpec) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("kubectl gateway client is unavailable")
	}
	_, err := m.client.CleanupKubectlManagedResources(ctx, spec)
	return err
}

// DeleteManagedProjectNamespace removes a project namespace only through the
// existing ownership-checking provider path. The luna-system namespace is not
// accepted here because it is never a project namespace.
func (m *KubectlGatewayManager) DeleteManagedProjectNamespace(ctx context.Context, namespace string) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("kubectl gateway client is unavailable")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || namespace == KubectlGatewaySystemNamespaceName {
		return fmt.Errorf("managed project namespace is invalid")
	}
	return m.client.DeleteManagedResource(ctx, "Namespace", "", namespace)
}

func (spec GatewayAccessSpec) SpecHash() string {
	normalized := normalizeGatewayAccessSpec(spec)
	payload, _ := json.Marshal(normalized)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func normalizeGatewayAccessSpec(spec GatewayAccessSpec) GatewayAccessSpec {
	spec.RuntimeClusterID = strings.TrimSpace(spec.RuntimeClusterID)
	projects := make([]GatewayAccessProjectSpec, 0, len(spec.Projects))
	seen := make(map[string]struct{}, len(spec.Projects))
	for _, project := range spec.Projects {
		project.ProjectID = strings.TrimSpace(project.ProjectID)
		project.Namespace = strings.TrimSpace(project.Namespace)
		if project.ProjectID == "" || project.Namespace == "" {
			continue
		}
		key := project.ProjectID + "\x00" + project.Namespace
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Namespace == projects[j].Namespace {
			return projects[i].ProjectID < projects[j].ProjectID
		}
		return projects[i].Namespace < projects[j].Namespace
	})
	spec.Projects = projects
	spec.ExtraProjectRules = normalizePolicyRules(spec.ExtraProjectRules)
	spec.ExtraManagedResources = normalizeGatewayManagedGVRs(spec.ExtraManagedResources)
	return spec
}

func normalizeGatewayManagedGVRs(input []schema.GroupVersionResource) []schema.GroupVersionResource {
	seen := make(map[schema.GroupVersionResource]struct{}, len(input))
	result := make([]schema.GroupVersionResource, 0, len(input))
	for _, item := range input {
		item.Group = strings.TrimSpace(item.Group)
		item.Version = strings.TrimSpace(item.Version)
		item.Resource = strings.ToLower(strings.TrimSpace(item.Resource))
		if item.Version == "" || item.Resource == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		if result[i].Version != result[j].Version {
			return result[i].Version < result[j].Version
		}
		return result[i].Resource < result[j].Resource
	})
	return result
}

func normalizePolicyRules(rules []rbacv1.PolicyRule) []rbacv1.PolicyRule {
	if len(rules) == 0 {
		return nil
	}
	normalized := make([]rbacv1.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		rule.APIGroups = normalizeStringSlice(rule.APIGroups)
		rule.Resources = normalizeStringSlice(rule.Resources)
		rule.ResourceNames = normalizeStringSlice(rule.ResourceNames)
		rule.Verbs = normalizeStringSlice(rule.Verbs)
		rule.NonResourceURLs = normalizeStringSlice(rule.NonResourceURLs)
		if len(rule.APIGroups) == 0 && len(rule.Resources) == 0 && len(rule.NonResourceURLs) == 0 {
			continue
		}
		normalized = append(normalized, rule)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return policyRuleKey(normalized[i]) < policyRuleKey(normalized[j])
	})
	return normalized
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func policyRuleKey(rule rbacv1.PolicyRule) string {
	payload, _ := json.Marshal(rule)
	return string(payload)
}

func gatewayAccessLabels(clusterID string) map[string]string {
	labels := SystemComponentLabels(KubectlGatewayServiceAccountName, clusterID)
	labels[ApplicationNameKey] = KubectlGatewayServiceAccountName
	return labels
}

func gatewayProjectBindingLabels(clusterID, projectID string) map[string]string {
	labels := gatewayAccessLabels(clusterID)
	setLabel(labels, ProjectIDLabel, projectID)
	return labels
}

func gatewayAccessAnnotations(hash string) map[string]string {
	return map[string]string{KubectlGatewaySpecHashAnnotation: hash}
}

func kubectlGatewayDiscoveryRules() []rbacv1.PolicyRule {
	return kubecatalog.DiscoveryClusterRoleRules()
}

func kubectlGatewayProjectRules() []rbacv1.PolicyRule {
	return kubecatalog.New().ProjectClusterRoleRules()
}

func (m *KubectlGatewayManager) projectClusterRoleRules(spec GatewayAccessSpec) []rbacv1.PolicyRule {
	rules := append([]rbacv1.PolicyRule{}, kubectlGatewayProjectRules()...)
	rules = append(rules, spec.ExtraProjectRules...)
	return normalizePolicyRules(rules)
}

func (m *KubectlGatewayManager) ensureGatewayServiceAccount(ctx context.Context, clusterID, hash string) error {
	serviceAccounts := m.client.client.CoreV1().ServiceAccounts(KubectlGatewaySystemNamespaceName)
	labels := gatewayAccessLabels(clusterID)
	annotations := gatewayAccessAnnotations(hash)
	existing, err := serviceAccounts.Get(ctx, KubectlGatewayServiceAccountName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		automount := false
		_, err = serviceAccounts.Create(ctx, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:        KubectlGatewayServiceAccountName,
				Namespace:   KubectlGatewaySystemNamespaceName,
				Labels:      labels,
				Annotations: annotations,
			},
			AutomountServiceAccountToken: &automount,
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ServiceAccount", existing, labels); err != nil {
		return err
	}
	desired := existing.DeepCopy()
	mergeStringMap(&desired.Labels, labels)
	mergeStringMap(&desired.Annotations, annotations)
	automount := false
	desired.AutomountServiceAccountToken = &automount
	if reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.Annotations, desired.Annotations) &&
		reflect.DeepEqual(existing.AutomountServiceAccountToken, desired.AutomountServiceAccountToken) {
		return nil
	}
	_, err = serviceAccounts.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (m *KubectlGatewayManager) ensureClusterRole(ctx context.Context, name string, rules []rbacv1.PolicyRule, labels, annotations map[string]string) error {
	clusterRoles := m.client.client.RbacV1().ClusterRoles()
	existing, err := clusterRoles.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = clusterRoles.Create(ctx, &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels, Annotations: annotations},
			Rules:      rules,
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ClusterRole", existing, labels); err != nil {
		return err
	}
	desired := existing.DeepCopy()
	mergeStringMap(&desired.Labels, labels)
	mergeStringMap(&desired.Annotations, annotations)
	desired.Rules = append([]rbacv1.PolicyRule(nil), rules...)
	if reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.Annotations, desired.Annotations) &&
		reflect.DeepEqual(normalizePolicyRules(existing.Rules), normalizePolicyRules(desired.Rules)) {
		return nil
	}
	_, err = clusterRoles.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (m *KubectlGatewayManager) ensureDiscoveryClusterRoleBinding(ctx context.Context, clusterID, hash string) error {
	labels := gatewayAccessLabels(clusterID)
	annotations := gatewayAccessAnnotations(hash)
	subjects := []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      KubectlGatewayServiceAccountName,
		Namespace: KubectlGatewaySystemNamespaceName,
	}}
	return m.ensureClusterRoleBinding(ctx, KubectlGatewayDiscoveryClusterRoleBinding, KubectlGatewayDiscoveryClusterRoleName, labels, annotations, subjects)
}

func (m *KubectlGatewayManager) ensureClusterRoleBinding(ctx context.Context, name, roleName string, labels, annotations map[string]string, subjects []rbacv1.Subject) error {
	clusterRoleBindings := m.client.client.RbacV1().ClusterRoleBindings()
	existing, err := clusterRoleBindings.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = clusterRoleBindings.Create(ctx, &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels, Annotations: annotations},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: roleName},
			Subjects:   append([]rbacv1.Subject(nil), subjects...),
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ClusterRoleBinding", existing, labels); err != nil {
		return err
	}
	desired := existing.DeepCopy()
	mergeStringMap(&desired.Labels, labels)
	mergeStringMap(&desired.Annotations, annotations)
	desired.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: roleName}
	desired.Subjects = append([]rbacv1.Subject(nil), subjects...)
	if reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.Annotations, desired.Annotations) &&
		reflect.DeepEqual(existing.RoleRef, desired.RoleRef) &&
		reflect.DeepEqual(existing.Subjects, desired.Subjects) {
		return nil
	}
	_, err = clusterRoleBindings.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (m *KubectlGatewayManager) ensureProjectRoleBindings(ctx context.Context, spec GatewayAccessSpec, hash string) error {
	for _, project := range spec.Projects {
		if err := m.ensureProjectRoleBinding(ctx, spec.RuntimeClusterID, project, hash); err != nil {
			return err
		}
	}
	desiredNamespaces := make(map[string]struct{}, len(spec.Projects))
	for _, project := range spec.Projects {
		desiredNamespaces[project.Namespace] = struct{}{}
	}
	roleBindings, err := m.client.client.RbacV1().RoleBindings(corev1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: selectorFromLabels(gatewayAccessLabels(spec.RuntimeClusterID)),
	})
	if err != nil {
		return err
	}
	for i := range roleBindings.Items {
		item := &roleBindings.Items[i]
		if item.Name != KubectlGatewayProjectRoleBindingName {
			continue
		}
		if _, keep := desiredNamespaces[item.Namespace]; keep {
			continue
		}
		if err := ensureResourceOwnership("RoleBinding", item, item.Labels); err != nil {
			continue
		}
		if err := m.client.client.RbacV1().RoleBindings(item.Namespace).Delete(ctx, item.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (m *KubectlGatewayManager) ensureProjectRoleBinding(ctx context.Context, clusterID string, project GatewayAccessProjectSpec, hash string) error {
	roleBindings := m.client.client.RbacV1().RoleBindings(project.Namespace)
	labels := gatewayProjectBindingLabels(clusterID, project.ProjectID)
	annotations := gatewayAccessAnnotations(hash)
	existing, err := roleBindings.Get(ctx, KubectlGatewayProjectRoleBindingName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = roleBindings.Create(ctx, &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewayProjectRoleBindingName, Namespace: project.Namespace, Labels: labels, Annotations: annotations},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: KubectlGatewayProjectClusterRoleName},
			Subjects: []rbacv1.Subject{{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      KubectlGatewayServiceAccountName,
				Namespace: KubectlGatewaySystemNamespaceName,
			}},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("RoleBinding", existing, labels); err != nil {
		return err
	}
	desired := existing.DeepCopy()
	mergeStringMap(&desired.Labels, labels)
	mergeStringMap(&desired.Annotations, annotations)
	desired.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: KubectlGatewayProjectClusterRoleName}
	desired.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      KubectlGatewayServiceAccountName,
		Namespace: KubectlGatewaySystemNamespaceName,
	}}
	if reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.Annotations, desired.Annotations) &&
		reflect.DeepEqual(existing.RoleRef, desired.RoleRef) &&
		reflect.DeepEqual(existing.Subjects, desired.Subjects) {
		return nil
	}
	_, err = roleBindings.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (m *KubectlGatewayManager) observeServiceAccount(ctx context.Context, hash string) GatewayAccessComponentStatus {
	item, err := m.client.client.CoreV1().ServiceAccounts(KubectlGatewaySystemNamespaceName).Get(ctx, KubectlGatewayServiceAccountName, metav1.GetOptions{})
	return gatewayComponentStatus("ServiceAccount", KubectlGatewayServiceAccountName, KubectlGatewaySystemNamespaceName, item, err, hash)
}

func (m *KubectlGatewayManager) observeClusterRole(ctx context.Context, name, hash string) GatewayAccessComponentStatus {
	item, err := m.client.client.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
	return gatewayComponentStatus("ClusterRole", name, "", item, err, hash)
}

func (m *KubectlGatewayManager) observeClusterRoleBinding(ctx context.Context, name, hash string) GatewayAccessComponentStatus {
	item, err := m.client.client.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
	return gatewayComponentStatus("ClusterRoleBinding", name, "", item, err, hash)
}

func (m *KubectlGatewayManager) observeProjectRoleBinding(ctx context.Context, project GatewayAccessProjectSpec, hash string) GatewayAccessComponentStatus {
	item, err := m.client.client.RbacV1().RoleBindings(project.Namespace).Get(ctx, KubectlGatewayProjectRoleBindingName, metav1.GetOptions{})
	return gatewayComponentStatus("RoleBinding", KubectlGatewayProjectRoleBindingName, project.Namespace, item, err, hash)
}

func gatewayComponentStatus(kind, name, namespace string, object metav1.Object, err error, hash string) GatewayAccessComponentStatus {
	status := GatewayAccessComponentStatus{Kind: kind, Name: name, Namespace: namespace}
	if apierrors.IsNotFound(err) {
		status.Reason = "missing"
		return status
	}
	if err != nil {
		status.Reason = "unavailable"
		return status
	}
	if hash != "" && object.GetAnnotations()[KubectlGatewaySpecHashAnnotation] != hash {
		status.Reason = "spec_hash_mismatch"
		return status
	}
	status.Ready = true
	return status
}

func (m *KubectlGatewayManager) deleteProjectRoleBindings(ctx context.Context, clusterID string) error {
	roleBindings, err := m.client.client.RbacV1().RoleBindings(corev1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: selectorFromLabels(gatewayAccessLabels(clusterID)),
	})
	if err != nil {
		return err
	}
	for i := range roleBindings.Items {
		item := &roleBindings.Items[i]
		if item.Name != KubectlGatewayProjectRoleBindingName {
			continue
		}
		if err := ensureResourceOwnership("RoleBinding", item, item.Labels); err != nil {
			continue
		}
		if err := m.client.client.RbacV1().RoleBindings(item.Namespace).Delete(ctx, item.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (m *KubectlGatewayManager) deleteClusterRoleBinding(ctx context.Context, name string) error {
	item, err := m.client.client.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ClusterRoleBinding", item, item.Labels); err != nil {
		return nil
	}
	return m.client.client.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{})
}

func (m *KubectlGatewayManager) deleteClusterRole(ctx context.Context, name string) error {
	item, err := m.client.client.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ClusterRole", item, item.Labels); err != nil {
		return nil
	}
	return m.client.client.RbacV1().ClusterRoles().Delete(ctx, name, metav1.DeleteOptions{})
}

func (m *KubectlGatewayManager) deleteServiceAccount(ctx context.Context, namespace, name string) error {
	item, err := m.client.client.CoreV1().ServiceAccounts(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("ServiceAccount", item, item.Labels); err != nil {
		return nil
	}
	return m.client.client.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func selectorFromLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key, value := range labels {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, ",")
}

func mergeStringMap(target *map[string]string, values map[string]string) {
	if *target == nil {
		*target = map[string]string{}
	}
	for key, value := range values {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		(*target)[key] = value
	}
}
