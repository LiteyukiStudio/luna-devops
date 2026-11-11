package worker

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNetworkPolicyReconcileSyncsActiveProjectNamespaces(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "network_policy_reconcile_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{}, &model.Project{}, &model.SystemComponentInstallation{})
		},
	})
	cluster := model.RuntimeCluster{ID: "rc_net", Name: "net", Scope: "global", GatewayNamespace: "traefik", NetworkBlockedCIDRs: "203.0.113.0/24"}
	active := model.Project{ID: "prj_active", Name: "active", Identifier: "active", KubernetesNamespace: "prj-active", DeleteStatus: "active"}
	deleting := model.Project{ID: "prj_deleting", Name: "deleting", Identifier: "deleting", KubernetesNamespace: "prj-deleting", DeleteStatus: "deleting"}
	for _, value := range []any{&cluster, &active, &deleting} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.SystemComponentInstallation{ID: "sci_probe", ComponentID: "gateway-traffic-probe", RuntimeClusterID: cluster.ID,
		ProjectID: active.ID, DeploymentTargetID: "dt_probe", Namespace: "prj-active"}).Error; err != nil {
		t.Fatal(err)
	}
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prj-active", Labels: kubeprovider.ProjectNamespaceLabels(active.ID)}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prj-deleting", Labels: kubeprovider.ProjectNamespaceLabels(deleting.ID)}},
		// 标签伪造的 Namespace 与数据库中的项目 Namespace 不一致，不能被当作该项目渲染。
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "spoofed", Labels: kubeprovider.ProjectNamespaceLabels(active.ID)}},
	)
	manager := kubeprovider.NewClientForInterface(clientset)
	runner := &Runner{db: db, kubernetesManagerFactory: func(model.DeploymentTarget) (kubeprovider.NamespaceManager, error) { return manager, nil }}

	task, err := tasks.NewNetworkPolicyReconcileTask(tasks.NetworkPolicyReconcilePayload{RuntimeClusterID: cluster.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.handleNetworkPolicyReconcile(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	policies, err := clientset.NetworkingV1().NetworkPolicies("prj-active").List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(policies.Items) != 4 {
		t.Fatalf("active namespace policies = %d, want ingress, egress, build egress and trusted system egress", len(policies.Items))
	}
	for _, namespace := range []string{"prj-deleting", "spoofed"} {
		list, _ := clientset.NetworkingV1().NetworkPolicies(namespace).List(t.Context(), metav1.ListOptions{})
		if len(list.Items) != 0 {
			t.Fatalf("namespace %s should not be rendered, got %d policies", namespace, len(list.Items))
		}
	}
	desired, err := runner.desiredPoliciesForTest(t, manager, cluster, active)
	if err != nil {
		t.Fatal(err)
	}
	if state, err := manager.WorkloadNetworkPolicyState(t.Context(), "prj-active", desired); err != nil || state != kubeprovider.WorkloadNetworkPolicySynced {
		t.Fatalf("state = %s, %v", state, err)
	}
}

func (r *Runner) desiredPoliciesForTest(t *testing.T, manager kubeprovider.NamespaceManager, cluster model.RuntimeCluster, project model.Project) ([]networkingv1.NetworkPolicy, error) {
	t.Helper()
	nodes, err := manager.ClusterNodeAddresses(t.Context())
	if err != nil {
		return nil, err
	}
	return runtimecluster.DesiredWorkloadNetworkPolicies(t.Context(), r.db, cluster, project.ID, project.KubernetesNamespace, nodes)
}
