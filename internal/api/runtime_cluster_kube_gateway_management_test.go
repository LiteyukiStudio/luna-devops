package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type fakeKubectlGatewayTaskEnqueuer struct {
	fakeBuildTaskEnqueuer
	payloads []tasks.KubectlGatewayPayload
	err      error
}

func (f *fakeKubectlGatewayTaskEnqueuer) EnqueueKubectlGateway(_ context.Context, payload tasks.KubectlGatewayPayload) (*asynq.TaskInfo, error) {
	f.payloads = append(f.payloads, payload)
	if f.err != nil {
		return nil, f.err
	}
	return &asynq.TaskInfo{}, nil
}

func TestPersistRuntimeClusterKubeGatewayDesiredKeepsCommittedStateWhenQueueFails(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "kube_gateway_desired",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{})
		},
	})
	cluster := model.RuntimeCluster{ID: "clu_gateway", Name: "Gateway", Type: "kubernetes", Scope: "global"}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatalf("create runtime cluster: %v", err)
	}
	fake := &fakeKubectlGatewayTaskEnqueuer{err: errors.New("redis unavailable")}
	handlers := &Handlers{db: db, taskClient: fake}

	err := handlers.persistRuntimeClusterKubeGatewayDesired(t.Context(), cluster.ID, true, `[{"apiGroup":"example.io"}]`)
	if !errors.Is(err, errKubeGatewayEnqueue) {
		t.Fatalf("persistRuntimeClusterKubeGatewayDesired() error = %v, want enqueue failure", err)
	}
	var stored model.RuntimeCluster
	if err := db.First(&stored, "id = ?", cluster.ID).Error; err != nil {
		t.Fatalf("reload runtime cluster: %v", err)
	}
	if !stored.KubeGatewayEnabled || !strings.Contains(stored.KubeGatewayExtraResourceRules, "example.io") {
		t.Fatalf("committed desired state = %#v", stored)
	}
	if len(fake.payloads) != 1 || fake.payloads[0].ClusterID != cluster.ID {
		t.Fatalf("queued payloads = %#v", fake.payloads)
	}
}

func TestRuntimeClusterConnectionChangeRequiresGatewayCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name  string
		input runtimeClusterInput
	}{
		{name: "replace kubeconfig", input: runtimeClusterInput{Type: "kubernetes", Kubeconfig: "new-config"}},
		{name: "switch provider type", input: runtimeClusterInput{Type: "docker-compose"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/runtime/clusters/clu_gateway", nil)
			handlers := &Handlers{}
			allowed := handlers.allowRuntimeClusterConnectionChange(ctx, model.RuntimeCluster{
				ID: "clu_gateway", Type: "kubernetes", KubeconfigRef: "sec_old", KubeGatewayEnabled: true,
			}, test.input)
			if allowed {
				t.Fatal("connection change was allowed while kubectl gateway is enabled")
			}
			if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "kube_gateway.connection_change_requires_disable") {
				t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRuntimeClusterAuditMetadataIsAllowListed(t *testing.T) {
	metadata := runtimeClusterSafeAuditMetadata(model.RuntimeCluster{
		ID: "clu_secret", Type: "kubernetes", Scope: "global", Endpoint: "https://private.example.invalid",
		KubeconfigRef: "secret/kubeconfig", GatewayTrustedProxyCIDRs: "10.0.0.0/8",
		GatewayDefaultRequestHeaders: `{"Authorization":"secret"}`, KubeGatewayEnabled: true,
	}, true)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal audit metadata: %v", err)
	}
	for _, forbidden := range []string{"private.example.invalid", "secret/kubeconfig", "10.0.0.0/8", "Authorization"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("audit metadata leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestUpdateRuntimeClusterKubeGatewayMarksResponseNoStoreBeforeAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/runtime/clusters/clu_gateway/kube-gateway", nil)

	(&Handlers{}).UpdateRuntimeClusterKubeGateway(ctx)

	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
