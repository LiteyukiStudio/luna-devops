package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestKubectlGatewayAccessSpecUsesProjectNamespaces(t *testing.T) {
	spec := kubectlGatewayAccessSpec(model.RuntimeCluster{ID: "rcl_demo"}, []model.Project{
		{ID: "prj_demo", KubernetesNamespace: "project-demo"},
	})
	if spec.RuntimeClusterID != "rcl_demo" || len(spec.Projects) != 1 || spec.Projects[0].Namespace != "project-demo" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestKubectlGatewayAccessSpecIncludesStoredExtraRules(t *testing.T) {
	raw, err := json.Marshal([]map[string]any{{
		"apiGroup": "example.io", "apiVersion": "v1", "resource": "widgets",
		"subresources": []string{"status"}, "verbs": []string{"get", "patch"}, "action": "deployment:update",
	}})
	if err != nil {
		t.Fatal(err)
	}
	spec := kubectlGatewayAccessSpec(model.RuntimeCluster{
		ID: "rcl_demo", KubeGatewayEnabled: true, KubeGatewayExtraResourceRules: string(raw),
	}, nil)
	if !spec.Enabled || len(spec.ExtraProjectRules) != 1 {
		t.Fatalf("spec = %#v", spec)
	}
	rule := spec.ExtraProjectRules[0]
	if len(rule.Resources) != 2 || rule.Resources[1] != "widgets/status" {
		t.Fatalf("rule = %#v", rule)
	}
	if len(spec.ExtraManagedResources) != 1 || spec.ExtraManagedResources[0].Resource != "widgets" {
		t.Fatalf("extra managed resources = %#v", spec.ExtraManagedResources)
	}
}

func TestKubectlGatewayAdvisoryLockKeyStable(t *testing.T) {
	if got, want := kubectlGatewayAdvisoryLockKey("rcl_demo"), kubectlGatewayAdvisoryLockKey("rcl_demo"); got != want {
		t.Fatalf("lock key changed: %d vs %d", got, want)
	}
}

type fakeKubectlGatewayManager struct {
	specs []kubeprovider.GatewayAccessSpec
}

func (m *fakeKubectlGatewayManager) ReconcileGatewayAccess(_ context.Context, spec kubeprovider.GatewayAccessSpec) (kubeprovider.GatewayAccessObservation, error) {
	m.specs = append(m.specs, spec)
	return kubeprovider.GatewayAccessObservation{Status: "ready", Ready: true}, nil
}

func (m *fakeKubectlGatewayManager) CleanupGatewayAccess(_ context.Context, spec kubeprovider.GatewayAccessSpec) error {
	m.specs = append(m.specs, spec)
	return nil
}
