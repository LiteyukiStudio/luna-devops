package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubectlGatewayManagerReconcileCreatesSharedResourcesAndProjectBindings(t *testing.T) {
	clientset := fake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "project-demo", Labels: ProjectNamespaceLabels("prj_demo")},
	})
	manager := NewKubectlGatewayManager(NewClientForInterface(clientset))
	observation, err := manager.ReconcileGatewayAccess(t.Context(), GatewayAccessSpec{
		RuntimeClusterID: "rcl_demo",
		Enabled:          true,
		Projects: []GatewayAccessProjectSpec{{
			ProjectID: "prj_demo",
			Namespace: "project-demo",
		}},
	})
	if err != nil {
		t.Fatalf("ReconcileGatewayAccess() error = %v", err)
	}
	if !observation.Ready || observation.Status != "ready" {
		t.Fatalf("observation = %#v", observation)
	}
	serviceAccount, err := clientset.CoreV1().ServiceAccounts(KubectlGatewaySystemNamespaceName).Get(t.Context(), KubectlGatewayServiceAccountName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service account: %v", err)
	}
	if serviceAccount.Annotations[KubectlGatewaySpecHashAnnotation] == "" {
		t.Fatalf("service account annotations = %#v", serviceAccount.Annotations)
	}
	roleBinding, err := clientset.RbacV1().RoleBindings("project-demo").Get(t.Context(), KubectlGatewayProjectRoleBindingName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get rolebinding: %v", err)
	}
	if roleBinding.RoleRef.Name != KubectlGatewayProjectClusterRoleName || roleBinding.Labels[ProjectIDLabel] != "prj_demo" {
		t.Fatalf("rolebinding = %#v", roleBinding)
	}
}

func TestKubectlGatewayManagerCleanupRemovesOwnedResources(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewaySystemNamespaceName, Labels: KubectlGatewaySystemNamespaceLabels()}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewayServiceAccountName, Namespace: KubectlGatewaySystemNamespaceName, Labels: gatewayAccessLabels("rcl_demo")}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewayProjectClusterRoleName, Labels: gatewayAccessLabels("rcl_demo")}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewayDiscoveryClusterRoleName, Labels: gatewayAccessLabels("rcl_demo")}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewayDiscoveryClusterRoleBinding, Labels: gatewayAccessLabels("rcl_demo")}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: KubectlGatewayProjectRoleBindingName, Namespace: "project-demo", Labels: gatewayProjectBindingLabels("rcl_demo", "prj_demo")}},
	)
	manager := NewKubectlGatewayManager(NewClientForInterface(clientset))
	if err := manager.CleanupGatewayAccess(t.Context(), GatewayAccessSpec{RuntimeClusterID: "rcl_demo"}); err != nil {
		t.Fatalf("CleanupGatewayAccess() error = %v", err)
	}
	if _, err := clientset.CoreV1().ServiceAccounts(KubectlGatewaySystemNamespaceName).Get(t.Context(), KubectlGatewayServiceAccountName, metav1.GetOptions{}); err == nil {
		t.Fatal("service account still exists after cleanup")
	}
	if _, err := clientset.RbacV1().RoleBindings("project-demo").Get(t.Context(), KubectlGatewayProjectRoleBindingName, metav1.GetOptions{}); err == nil {
		t.Fatal("project rolebinding still exists after cleanup")
	}
	observation, err := manager.ObserveGatewayAccess(t.Context(), GatewayAccessSpec{RuntimeClusterID: "rcl_demo", Enabled: false})
	if err != nil {
		t.Fatalf("ObserveGatewayAccess() after cleanup error = %v", err)
	}
	if !observation.Ready || observation.Status != "disabled" {
		t.Fatalf("disabled observation after cleanup = %#v", observation)
	}
}

func TestKubectlGatewayManagerDisabledObservationDetectsOldAndStaleResources(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name: KubectlGatewayServiceAccountName, Namespace: KubectlGatewaySystemNamespaceName,
			Labels: gatewayAccessLabels("rcl_demo"), Annotations: map[string]string{KubectlGatewaySpecHashAnnotation: "old-enabled-hash"},
		}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
			Name: KubectlGatewayProjectRoleBindingName, Namespace: "stale-project",
			Labels: gatewayProjectBindingLabels("rcl_demo", "prj_stale"), Annotations: map[string]string{KubectlGatewaySpecHashAnnotation: "old-enabled-hash"},
		}},
	)
	manager := NewKubectlGatewayManager(NewClientForInterface(clientset))

	observation, err := manager.ObserveGatewayAccess(t.Context(), GatewayAccessSpec{RuntimeClusterID: "rcl_demo", Enabled: false})
	if err != nil {
		t.Fatalf("ObserveGatewayAccess() error = %v", err)
	}
	if observation.Ready || observation.Status != "reconciling" {
		t.Fatalf("disabled observation with residual resources = %#v", observation)
	}
}
