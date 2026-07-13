package api

import (
	"context"
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

func TestFlattenKubeconfigRejectsLocalCertificateFiles(t *testing.T) {
	input := `
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: https://127.0.0.1:6443
    certificate-authority: /etc/kubernetes/ca.crt
users:
- name: local
  user:
    client-certificate: /etc/kubernetes/client.crt
    client-key: /etc/kubernetes/client.key
contexts:
- name: local
  context:
    cluster: local
    user: local
current-context: local
`

	if _, err := flattenKubeconfig(input); err == nil || !strings.Contains(err.Error(), "kubeconfig 不安全") {
		t.Fatalf("flattenKubeconfig error = %v, want unsafe kubeconfig error", err)
	}
}

func TestFlattenKubeconfigRejectsExecCredentialPlugin(t *testing.T) {
	input := `
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: https://127.0.0.1:6443
users:
- name: local
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: sh
contexts:
- name: local
  context:
    cluster: local
    user: local
current-context: local
`

	if _, err := flattenKubeconfig(input); err == nil || !strings.Contains(err.Error(), "kubeconfig 不安全") {
		t.Fatalf("flattenKubeconfig error = %v, want unsafe kubeconfig error", err)
	}
}

func TestFlattenKubeconfigAcceptsInlineTokenAndHTTPSServer(t *testing.T) {
	input := `
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: https://kubernetes.example.com:6443
users:
- name: local
  user:
    token: inline-token
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
	if !strings.Contains(output, "https://kubernetes.example.com:6443") || !strings.Contains(output, "inline-token") {
		t.Fatalf("flattenKubeconfig output = %s", output)
	}
}

type fakeBuildTaskEnqueuer struct {
	buildPayload             tasks.BuildRunPayload
	deployPayload            tasks.DeployRunPayload
	gatewayPayload           tasks.GatewayApplyPayload
	systemComponentPayload   tasks.SystemComponentApplyPayload
	notificationPayload      tasks.NotificationDeliverPayload
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

func (f *fakeBuildTaskEnqueuer) EnqueueSystemComponentApply(_ context.Context, payload tasks.SystemComponentApplyPayload) (*asynq.TaskInfo, error) {
	f.systemComponentPayload = payload
	return &asynq.TaskInfo{}, nil
}

func (f *fakeBuildTaskEnqueuer) EnqueueNotificationDeliver(_ context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error) {
	f.notificationPayload = payload
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
