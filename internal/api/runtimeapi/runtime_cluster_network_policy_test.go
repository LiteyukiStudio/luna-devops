package runtimeapi

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNormalizeNetworkBlockedCIDRs(t *testing.T) {
	got, err := normalizeNetworkBlockedCIDRs("203.0.113.9/24\n 2001:db8::1/48 ,203.0.113.0/24;")
	if err != nil {
		t.Fatal(err)
	}
	if got != "203.0.113.0/24\n2001:db8::/48" {
		t.Fatalf("normalized = %q", got)
	}
	for _, invalid := range []string{"0.0.0.0/0", "::/0", "192.168.31.1", "nope/8"} {
		if _, err := normalizeNetworkBlockedCIDRs(invalid); err == nil {
			t.Fatalf("%q should be rejected", invalid)
		}
	}
}

func TestObserveWorkloadNetworkPoliciesComparesLiveState(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_cluster_network_policy_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.SystemComponentInstallation{})
		},
	})
	cluster := model.RuntimeCluster{ID: "rc_net", GatewayNamespace: "traefik"}
	synced := model.Project{ID: "prj_synced", Name: "synced", Identifier: "synced", KubernetesNamespace: "prj-synced", DeleteStatus: "active"}
	drifted := model.Project{ID: "prj_drifted", Name: "drifted", Identifier: "drifted", KubernetesNamespace: "prj-drifted", DeleteStatus: "active"}
	for _, project := range []*model.Project{&synced, &drifted} {
		if err := db.Create(project).Error; err != nil {
			t.Fatal(err)
		}
	}
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: synced.KubernetesNamespace, Labels: kubeprovider.ProjectNamespaceLabels(synced.ID)}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: drifted.KubernetesNamespace, Labels: kubeprovider.ProjectNamespaceLabels(drifted.ID)}},
	)
	client := kubeprovider.NewClientForInterface(clientset)
	desired, err := runtimecluster.DesiredWorkloadNetworkPolicies(t.Context(), db, cluster, synced.ID, synced.KubernetesNamespace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SyncWorkloadNetworkPolicies(t.Context(), synced.KubernetesNamespace, desired); err != nil {
		t.Fatal(err)
	}

	response := observeWorkloadNetworkPolicies(t.Context(), db, cluster, client)
	if response.Status != observation.StatusDegraded || response.ObservationCode != "runtime_cluster.network_policy_drifted" {
		t.Fatalf("status = %s/%s", response.Status, response.ObservationCode)
	}
	if response.NamespaceCount != 2 || response.SyncedCount != 1 || response.DriftedCount != 1 {
		t.Fatalf("counts = %+v", response)
	}
}
