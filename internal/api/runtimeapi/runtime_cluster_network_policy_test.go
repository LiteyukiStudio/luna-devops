package runtimeapi

import (
	"testing"
	"time"

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
	for _, invalid := range []string{"0.0.0.0/0", "::/0", "192.168.31.1", "nope/8", "::ffff:10.0.0.0/104"} {
		if _, err := normalizeNetworkBlockedCIDRs(invalid); err == nil {
			t.Fatalf("%q should be rejected", invalid)
		}
	}
}

func TestObserveWorkloadNetworkPoliciesComparesLiveState(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_cluster_network_policy_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.SystemComponentInstallation{}, &model.DeploymentTarget{}, &model.ServiceBinding{})
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

func TestNormalizeBuildPrivateEgress(t *testing.T) {
	if got, err := normalizeBuildPrivateEgressPorts(""); err != nil || got != "443" {
		t.Fatalf("empty ports = %q, %v", got, err)
	}
	if got, err := normalizeBuildPrivateEgressPorts("5000, 443\n5000"); err != nil || got != "5000,443" {
		t.Fatalf("ports = %q, %v", got, err)
	}
	for _, invalid := range []string{"0", "65536", "http", "1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17"} {
		if _, err := normalizeBuildPrivateEgressPorts(invalid); err == nil {
			t.Fatalf("ports %q should be rejected", invalid)
		}
	}
	if got, err := normalizeBuildPrivateEgressCIDRs("10.20.1.5/16"); err != nil || got != "10.20.0.0/16" {
		t.Fatalf("cidrs = %q, %v", got, err)
	}
	for _, invalid := range []string{"0.0.0.0/0", "169.254.169.254/32", "100.100.100.200", "::ffff:0.0.0.0/96"} {
		if _, err := normalizeBuildPrivateEgressCIDRs(invalid); err == nil {
			t.Fatalf("private egress %q must be rejected", invalid)
		}
	}
	// 包含元数据地址的网段（如 Tailscale 的 CGNAT 段）允许保存，由渲染器排除元数据地址。
	if got, err := normalizeBuildPrivateEgressCIDRs("100.64.0.0/10"); err != nil || got != "100.64.0.0/10" {
		t.Fatalf("CGNAT private egress = %q, %v", got, err)
	}
}

func TestSaveRuntimeClusterRecordKeepsProbeResultUnlessReset(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_cluster_probe_save_test",
		Migrate:      func(db *gorm.DB) error { return db.AutoMigrate(&model.RuntimeCluster{}) },
	})
	probedAt := time.Now().UTC().Truncate(time.Second)
	if err := db.Create(&model.RuntimeCluster{ID: "rc_save", Name: "save", Scope: "global", NetworkPolicyEnforcement: "verified", NetworkPolicyProbedAt: &probedAt}).Error; err != nil {
		t.Fatal(err)
	}
	// API 在请求开始时读到的旧值不能覆盖 Worker 已写入的探测结论。
	stale := model.RuntimeCluster{ID: "rc_save", Name: "renamed", Scope: "global", NetworkPolicyEnforcement: "unknown"}
	if err := saveRuntimeClusterRecord(db, stale, false); err != nil {
		t.Fatal(err)
	}
	var stored model.RuntimeCluster
	db.First(&stored, "id = ?", "rc_save")
	if stored.Name != "renamed" || stored.NetworkPolicyEnforcement != "verified" || stored.NetworkPolicyProbedAt == nil {
		t.Fatalf("normal save overwrote probe result: %#v", stored)
	}
	if err := saveRuntimeClusterRecord(db, stale, true); err != nil {
		t.Fatal(err)
	}
	var reset model.RuntimeCluster
	db.First(&reset, "id = ?", "rc_save")
	if reset.NetworkPolicyEnforcement != "unknown" || reset.NetworkPolicyProbedAt != nil {
		t.Fatalf("kubeconfig replacement must reset probe result: enforcement=%s probedAt=%v", reset.NetworkPolicyEnforcement, reset.NetworkPolicyProbedAt)
	}
}
