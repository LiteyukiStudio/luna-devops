package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
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

func TestObserveRuntimeClusterKubeGatewayStatusRequiresAdminAndValidatesInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		role       string
		query      string
		wantStatus int
		wantCode   string
	}{
		{
			name: "ordinary user",
			role: authz.PlatformRoleUser, query: "?clusterId=clu_gateway",
			wantStatus: http.StatusForbidden, wantCode: "config.admin.required",
		},
		{
			name: "administrator invalid identifiers",
			role: authz.PlatformRoleAdmin, query: "?clusterId=invalid",
			wantStatus: http.StatusBadRequest, wantCode: "runtime_cluster.kube_gateway_cluster_ids_invalid",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/kube-gateway-status"+test.query, nil)
			ctx.Set(currentUserContextKey, model.User{ID: "usr_gateway_status", Role: test.role, Language: "en-US"})

			(&Handlers{}).ObserveRuntimeClusterKubeGatewayStatus(ctx)

			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), test.wantCode) {
				t.Fatalf("response = %d %s, want status %d and code %q", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantCode)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

func TestRuntimeClusterKubeGatewayStatusOpenAPIContract(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	operation := document["paths"].(map[string]any)["/api/v1/runtime/clusters/kube-gateway-status"].(map[string]any)["get"].(map[string]any)
	if operation["operationId"] != "observeRuntimeClusterKubeGatewayStatus" {
		t.Fatalf("operationId = %#v", operation["operationId"])
	}
	cli := operation["x-luna-cli"].(map[string]any)
	if !reflect.DeepEqual(cli["requiredScopes"], []any{"cluster:read"}) {
		t.Fatalf("requiredScopes = %#v", cli["requiredScopes"])
	}
	agent := operation["x-luna-agent"].(map[string]any)
	for _, field := range []string{"purpose", "aliases", "avoidWhen", "preconditions", "successEvidence"} {
		if agent[field] == nil {
			t.Fatalf("x-luna-agent.%s is missing", field)
		}
	}
	if requiresApproval, _ := agent["requiresApproval"].(bool); requiresApproval {
		t.Fatal("read-only gateway status observation must not require approval")
	}

	parameters := operation["parameters"].([]any)
	if len(parameters) != 1 {
		t.Fatalf("parameters = %#v", parameters)
	}
	parameter := parameters[0].(map[string]any)
	parameterSchema := parameter["schema"].(map[string]any)
	if parameter["name"] != "clusterId" || parameter["required"] != true || parameterSchema["type"] != "array" || parameterSchema["minItems"] != float64(1) || parameterSchema["maxItems"] != float64(100) {
		t.Fatalf("clusterId parameter = %#v", parameter)
	}

	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	statusSchema := schemas["RuntimeClusterKubeGatewayStatus"].(map[string]any)
	if statusSchema["additionalProperties"] != false || !reflect.DeepEqual(statusSchema["required"], []any{"clusterId", "enabled", "status", "observationCode", "lastCheckedAt"}) {
		t.Fatalf("status schema = %#v", statusSchema)
	}
	properties := statusSchema["properties"].(map[string]any)
	if len(properties) != 5 {
		t.Fatalf("status properties = %#v, want only the five sanitized fields", properties)
	}
	for _, field := range []string{"clusterId", "enabled", "status", "observationCode", "lastCheckedAt"} {
		if properties[field] == nil {
			t.Fatalf("status property %q is missing", field)
		}
	}
	listSchema := schemas["RuntimeClusterKubeGatewayStatusList"].(map[string]any)
	items := listSchema["properties"].(map[string]any)["items"].(map[string]any)
	if items["type"] != "array" || items["maxItems"] != float64(100) {
		t.Fatalf("status list items = %#v", items)
	}
}

func TestRuntimeClusterKubeGatewayRuleOpenAPIContract(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	ruleSchema := schemas["RuntimeClusterKubeGatewayRule"].(map[string]any)

	wantRequired := []any{"apiGroup", "apiVersion", "resource", "verbs", "action"}
	if !reflect.DeepEqual(ruleSchema["required"], wantRequired) {
		t.Fatalf("RuntimeClusterKubeGatewayRule.required = %#v, want %#v", ruleSchema["required"], wantRequired)
	}
	subresources := ruleSchema["properties"].(map[string]any)["subresources"].(map[string]any)
	if subresources["type"] != "array" {
		t.Fatalf("RuntimeClusterKubeGatewayRule.subresources = %#v, want array", subresources)
	}

	responseRuleSchema := schemas["RuntimeClusterKubeGatewayRuleResponse"].(map[string]any)
	responseRuleParts := responseRuleSchema["allOf"].([]any)
	if responseRuleParts[0].(map[string]any)["$ref"] != "#/components/schemas/RuntimeClusterKubeGatewayRule" ||
		!reflect.DeepEqual(responseRuleParts[1].(map[string]any)["required"], []any{"subresources"}) {
		t.Fatalf("RuntimeClusterKubeGatewayRuleResponse = %#v", responseRuleSchema)
	}

	inputRuleRef := schemas["RuntimeClusterKubeGatewayInput"].(map[string]any)["properties"].(map[string]any)["extraResourceRules"].(map[string]any)["items"].(map[string]any)["$ref"]
	responseRuleRef := schemas["RuntimeClusterKubeGateway"].(map[string]any)["properties"].(map[string]any)["extraResourceRules"].(map[string]any)["items"].(map[string]any)["$ref"]
	if inputRuleRef != "#/components/schemas/RuntimeClusterKubeGatewayRule" || responseRuleRef != "#/components/schemas/RuntimeClusterKubeGatewayRuleResponse" {
		t.Fatalf("gateway rule refs = input %q, response %q", inputRuleRef, responseRuleRef)
	}
}
