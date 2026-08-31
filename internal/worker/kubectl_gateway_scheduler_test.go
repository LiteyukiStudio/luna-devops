package worker

import (
	"context"
	"sort"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func TestKubectlGatewayPeriodicTaskSpecUsesDedicatedSweep(t *testing.T) {
	spec, err := kubectlGatewayPeriodicTaskSpec()
	if err != nil {
		t.Fatalf("kubectlGatewayPeriodicTaskSpec() error = %v", err)
	}
	policy := tasks.KubectlGatewaySweepEnqueuePolicy()
	if spec.Cron != "@every 5m" || spec.Task.Type() != tasks.TypeKubectlGatewaySweep || spec.Queue != policy.Queue || spec.MaxRetry != policy.MaxRetry {
		t.Fatalf("spec = %#v, policy = %#v", spec, policy)
	}
	if spec.Unique != policy.Unique {
		t.Fatalf("spec.Unique = %v, want %v", spec.Unique, policy.Unique)
	}
}

type kubectlGatewaySweepEnqueuerStub struct {
	clusterIDs []string
}

func (s *kubectlGatewaySweepEnqueuerStub) EnqueueKubectlGateway(_ context.Context, payload tasks.KubectlGatewayPayload) (*asynq.TaskInfo, error) {
	s.clusterIDs = append(s.clusterIDs, payload.ClusterID)
	return &asynq.TaskInfo{}, nil
}

func TestKubectlGatewaySweepReconcilesEnabledAndDisabledActiveClusters(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "kube_gateway_sweep",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{})
		},
	})
	clusters := []model.RuntimeCluster{
		{ID: "clu_enabled", Name: "Enabled", Type: "kubernetes", Scope: "global", DeleteStatus: "active", KubeGatewayEnabled: true},
		{ID: "clu_disabled", Name: "Disabled", Type: "kubernetes", Scope: "global", DeleteStatus: "active", KubeGatewayEnabled: false},
		{ID: "clu_k3s", Name: "K3s", Type: "k3s", Scope: "global", DeleteStatus: "active", KubeGatewayEnabled: false},
		{ID: "clu_compose", Name: "Compose", Type: "docker-compose", Scope: "global", DeleteStatus: "active"},
		{ID: "clu_deleting", Name: "Deleting", Type: "kubernetes", Scope: "global", DeleteStatus: "deleting", KubeGatewayEnabled: true},
	}
	if err := db.Create(&clusters).Error; err != nil {
		t.Fatalf("create runtime clusters: %v", err)
	}
	stub := &kubectlGatewaySweepEnqueuerStub{}
	if err := enqueueScheduledKubectlGatewaysWith(t.Context(), db, stub); err != nil {
		t.Fatalf("enqueueScheduledKubectlGatewaysWith() error = %v", err)
	}
	sort.Strings(stub.clusterIDs)
	want := []string{"clu_disabled", "clu_enabled", "clu_k3s"}
	if len(stub.clusterIDs) != len(want) {
		t.Fatalf("queued cluster IDs = %#v, want %#v", stub.clusterIDs, want)
	}
	for index := range want {
		if stub.clusterIDs[index] != want[index] {
			t.Fatalf("queued cluster IDs = %#v, want %#v", stub.clusterIDs, want)
		}
	}
}
