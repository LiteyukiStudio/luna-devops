package api

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
)

func TestAccessTokenUnknownRouteIsDenied(t *testing.T) {
	if accessTokenAllows("*", "system:unmapped") {
		t.Fatal("expected unmapped route to be denied even for wildcard legacy token")
	}
}

func TestEnqueueDeployRunPassesStablePayload(t *testing.T) {
	fake := &fakeBuildTaskEnqueuer{}
	h := &Handlers{taskClient: fake}
	release := model.Release{ID: "rel_1", ProjectID: "prj_1", CreatedBy: "usr_1"}

	if !h.enqueueDeployRun(context.Background(), release) {
		t.Fatal("expected enqueueDeployRun to succeed")
	}

	want := tasks.DeployRunPayload{
		ReleaseID: "rel_1",
		ProjectID: "prj_1",
		ActorID:   "usr_1",
	}
	if fake.deployPayload != want {
		t.Fatalf("payload = %#v", fake.deployPayload)
	}
}

func TestEnqueueGatewayApplyPassesStablePayload(t *testing.T) {
	fake := &fakeBuildTaskEnqueuer{}
	h := &Handlers{taskClient: fake}
	route := model.GatewayRoute{ID: "gwr_1", ProjectID: "prj_1", CreatedBy: "usr_1"}

	if !h.enqueueGatewayApply(context.Background(), route) {
		t.Fatal("expected enqueueGatewayApply to succeed")
	}

	want := tasks.GatewayApplyPayload{
		GatewayRouteID: "gwr_1",
		ProjectID:      "prj_1",
		ActorID:        "usr_1",
	}
	if fake.gatewayPayload != want {
		t.Fatalf("payload = %#v", fake.gatewayPayload)
	}
}

func TestRollbackReleaseFromTargetUsesPreviousSuccessfulRelease(t *testing.T) {
	source := model.Release{
		ID:            "rel_current",
		ProjectID:     "prj_1",
		ApplicationID: "app_1",
		EnvironmentID: "env_1",
		ImageRef:      "registry.example.com/acme/api:v3",
		Revision:      3,
	}
	target := model.Release{
		ID:       "rel_previous",
		ImageRef: "registry.example.com/acme/api:v2",
		Revision: 2,
	}

	release := rollbackReleaseFromTarget(source, target, "usr_1", 4)
	if release.ImageRef != target.ImageRef || release.RollbackFromID != target.ID {
		t.Fatalf("release = %#v", release)
	}
	if release.Type != "rollback" || release.Status != "pending" || release.Revision != 4 {
		t.Fatalf("rollback metadata = %#v", release)
	}
}

func TestDeploymentTargetMatchesBuildRunUsesTargetPatterns(t *testing.T) {
	run := model.BuildRun{SourceBranch: "main", SourceTag: "v1.2.3"}
	if !deploymentTargetMatchesBuildRun(model.DeploymentTarget{BranchPattern: "main", TagPattern: "v*"}, run) {
		t.Fatal("expected target patterns to match build run")
	}
	if deploymentTargetMatchesBuildRun(model.DeploymentTarget{BranchPattern: "release-*"}, run) {
		t.Fatal("expected unmatched target branch pattern to skip auto deploy")
	}
}

func TestFlattenKubeconfigEmbedsCertificateFiles(t *testing.T) {
	caFile := writeTempKubeconfigFile(t, "ca.crt", "ca-data")
	certFile := writeTempKubeconfigFile(t, "client.crt", "cert-data")
	keyFile := writeTempKubeconfigFile(t, "client.key", "key-data")
	input := `
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: https://127.0.0.1:6443
    certificate-authority: ` + caFile + `
users:
- name: local
  user:
    client-certificate: ` + certFile + `
    client-key: ` + keyFile + `
contexts:
- name: local
  context:
    cluster: local
    user: local
current-context: local
`

	output, err := flattenKubeconfig(input)
	if err != nil {
		t.Fatalf("flattenKubeconfig returned error: %v", err)
	}
	if strings.Contains(output, caFile) || strings.Contains(output, certFile) || strings.Contains(output, keyFile) {
		t.Fatalf("expected file paths to be removed, got %s", output)
	}
	if !strings.Contains(output, "certificate-authority-data") || !strings.Contains(output, "client-certificate-data") || !strings.Contains(output, "client-key-data") {
		t.Fatalf("expected certificate data to be embedded, got %s", output)
	}
}

func writeTempKubeconfigFile(t *testing.T, name string, content string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

type fakeBuildTaskEnqueuer struct {
	buildPayload             tasks.BuildRunPayload
	deployPayload            tasks.DeployRunPayload
	gatewayPayload           tasks.GatewayApplyPayload
	applicationDeletePayload tasks.ApplicationDeletePayload
	resourceCleanupPayload   tasks.ResourceCleanupPayload
}

func (f *fakeBuildTaskEnqueuer) EnqueueBuildRun(_ context.Context, payload tasks.BuildRunPayload) (*asynq.TaskInfo, error) {
	f.buildPayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueDeployRun(_ context.Context, payload tasks.DeployRunPayload) (*asynq.TaskInfo, error) {
	f.deployPayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueGatewayApply(_ context.Context, payload tasks.GatewayApplyPayload) (*asynq.TaskInfo, error) {
	f.gatewayPayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueApplicationDelete(_ context.Context, payload tasks.ApplicationDeletePayload) (*asynq.TaskInfo, error) {
	f.applicationDeletePayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueResourceCleanup(_ context.Context, payload tasks.ResourceCleanupPayload) (*asynq.TaskInfo, error) {
	f.resourceCleanupPayload = payload
	return &asynq.TaskInfo{}, nil
}
