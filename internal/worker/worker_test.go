package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/provider/networkpolicy"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewRunnerDefaultsBuildJobOptions(t *testing.T) {
	runner := NewRunner(nil, Options{})
	if runner.deployRolloutTimeoutSeconds != 600 {
		t.Fatalf("deployRolloutTimeoutSeconds = %d", runner.deployRolloutTimeoutSeconds)
	}
	if runner.certManagerClusterIssuer != "letsencrypt-http01" {
		t.Fatalf("certManagerClusterIssuer = %q", runner.certManagerClusterIssuer)
	}
}

func TestNewRunnerUsesBuildJobOptions(t *testing.T) {
	runner := NewRunner(nil, Options{
		DeployRolloutTimeoutSeconds: 120,
		CertManagerClusterIssuer:    "letsencrypt-staging",
	})
	if runner.deployRolloutTimeoutSeconds != 120 {
		t.Fatalf("deployRolloutTimeoutSeconds = %d", runner.deployRolloutTimeoutSeconds)
	}
	if runner.certManagerClusterIssuer != "letsencrypt-staging" {
		t.Fatalf("certManagerClusterIssuer = %q", runner.certManagerClusterIssuer)
	}
}

func TestPeriodicTaskSpecsIncludeGitRefresh(t *testing.T) {
	specs, err := periodicTaskSpecs()
	if err != nil {
		t.Fatalf("periodicTaskSpecs returned error: %v", err)
	}
	foundGitRefresh := false
	foundRuntimeBilling := false
	for _, spec := range specs {
		if spec.Task.Type() == tasks.TypeGitAccountRefresh {
			foundGitRefresh = spec.Cron == "@every 5m" && spec.Queue == tasks.QueueLight
		}
		if spec.Task.Type() == tasks.TypeBillingRuntime {
			foundRuntimeBilling = spec.Cron == "@every 10m" && spec.Queue == tasks.QueueLight
		}
	}
	if !foundGitRefresh || !foundRuntimeBilling {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestCompletedHourlyWindowsReturnsOnlyCompleteHours(t *testing.T) {
	now := time.Date(2026, 6, 19, 15, 27, 30, 0, time.FixedZone("UTC+8", 8*3600))
	windows := completedHourlyWindows(now, 2)
	if len(windows) != 2 {
		t.Fatalf("windows = %#v", windows)
	}
	if !windows[0].Start.Equal(time.Date(2026, 6, 19, 5, 0, 0, 0, time.UTC)) || !windows[1].End.Equal(time.Date(2026, 6, 19, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("windows = %#v", windows)
	}
}

func TestRuntimeBillingEffectivePeriodProratesWindowStart(t *testing.T) {
	windowStart := time.Date(2026, 6, 19, 6, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	targetCreatedAt := windowStart.Add(10 * time.Minute)
	releaseStart := windowStart.Add(25 * time.Minute)
	start, end, ok := runtimeBillingEffectivePeriod(windowStart, windowEnd, targetCreatedAt, releaseStart)
	if !ok || !start.Equal(releaseStart) || !end.Equal(windowEnd) {
		t.Fatalf("period = %s %s %v", start, end, ok)
	}
}

func TestStorageBillingEffectivePeriodProratesWindowStart(t *testing.T) {
	windowStart := time.Date(2026, 6, 19, 6, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	targetCreatedAt := windowStart.Add(10 * time.Minute)
	start, end, ok := storageBillingEffectivePeriod(windowStart, windowEnd, targetCreatedAt)
	if !ok || !start.Equal(targetCreatedAt) || !end.Equal(windowEnd) {
		t.Fatalf("period = %s %s %v", start, end, ok)
	}
}

func TestExpiredBuildJobUpdatesClearLease(t *testing.T) {
	finishedAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	updates := expiredBuildJobUpdates(finishedAt)
	if updates["status"] != "lost" || updates["message"] != "lease_expired" || updates["lease_token"] != "" || updates["lease_until"] != nil {
		t.Fatalf("updates = %#v", updates)
	}
	gotFinishedAt, ok := updates["finished_at"].(*time.Time)
	if !ok || !gotFinishedAt.Equal(finishedAt) {
		t.Fatalf("finished_at = %#v", updates["finished_at"])
	}
}

func TestTaskEnvelopeFromPayloadReadsEnvelope(t *testing.T) {
	task, err := tasks.NewDeployRunTask(tasks.DeployRunPayload{ReleaseID: "rel_1", ProjectID: "prj_1", ActorID: "usr_1"})
	if err != nil {
		t.Fatalf("NewDeployRunTask returned error: %v", err)
	}
	envelope := taskEnvelopeFromPayload(task.Type(), task.Payload())
	if envelope.TaskType != tasks.TypeDeployRun || envelope.ResourceRef != "rel_1" || envelope.ActorID != "usr_1" {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestTaskEnvelopeFromPayloadFallsBackForLegacyPayload(t *testing.T) {
	envelope := taskEnvelopeFromPayload(tasks.TypeSyncStatus, []byte("{}"))
	if envelope.TaskType != tasks.TypeSyncStatus || envelope.TaskID != tasks.TypeSyncStatus || envelope.DedupeKey != tasks.TypeSyncStatus {
		t.Fatalf("envelope = %#v", envelope)
	}
}

func TestKubernetesNotFoundDetection(t *testing.T) {
	err := apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "blog-dev")
	if !isKubernetesNotFound(err) {
		t.Fatalf("expected kubernetes not found error to be detected")
	}
	if isKubernetesNotFound(errors.New("dial tcp refused")) {
		t.Fatalf("expected network error not to be treated as not found")
	}
}

func TestProjectNamespaceUsesProjectSlug(t *testing.T) {
	got := projectNamespace(model.Project{ID: "prj_abcdef1234567890", Slug: "Demo_App"})
	if got != "ns-abcdef1234" {
		t.Fatalf("namespace = %q", got)
	}
}

func TestProjectNamespaceCapsDNSLabelLength(t *testing.T) {
	got := projectNamespace(model.Project{ID: "prj_" + strings.Repeat("a", 80)})
	if len(got) > 63 {
		t.Fatalf("namespace too long: %q", got)
	}
}

func TestDeploymentNamespaceAlwaysUsesProjectNamespace(t *testing.T) {
	got := deploymentNamespace(model.Project{ID: "prj_abcdef1234567890", Slug: "demo"}, model.Environment{Namespace: " Prod_App "})
	if got != "ns-abcdef1234" {
		t.Fatalf("namespace = %q", got)
	}
}

func TestEnvironmentClusterLookupUsesEnvironmentClusterID(t *testing.T) {
	query, args := environmentClusterLookup(" rcl_env ")
	if query != "id = ? and type in ?" {
		t.Fatalf("query = %q", query)
	}
	if args[0] != "rcl_env" {
		t.Fatalf("cluster id arg = %#v", args[0])
	}
}

func TestRuntimeClusterKubeconfigErrorExplainsLocalFileRefs(t *testing.T) {
	err := runtimeClusterKubeconfigError(errors.New("invalid configuration: unable to read client-cert /Users/sfkm/.minikube/client.crt"))
	if !strings.Contains(err.Error(), "已内联证书的 kubeconfig") {
		t.Fatalf("error = %q", err)
	}
}

func TestApplicationResourceNameUsesDeploymentTargetID(t *testing.T) {
	got := applicationResourceName(model.DeploymentTarget{ID: "dplt_abcdef1234567890"})
	if got != "dplt-abcdef1234" {
		t.Fatalf("resource name = %q", got)
	}
}

func TestApplicationResourceNameFallsBackWhenTargetIDMissing(t *testing.T) {
	got := applicationResourceName(model.DeploymentTarget{})
	if got != "dplt" {
		t.Fatalf("resource name = %q", got)
	}
}

func TestBuildJobSpecUsesRestrictedServiceAccountAndBuildScope(t *testing.T) {
	spec := buildJobSpec(
		"build-job-1",
		"build-job-1-secret",
		model.Environment{ID: "env_dev"},
		model.BuildRun{BuildCPURequest: "750m", BuildMemoryRequest: "768Mi"},
		builder.Task{ProjectID: "prj_demo", ApplicationID: "app_api", DeploymentTargetID: "dplt_api", BuildRunID: "brn_1", JobID: "bjb_1"},
		"moby/buildkit:v0.24.0-rootless",
		"",
		false,
		"buildcache",
		1800,
		3600,
	)

	if spec.Spec.ActiveDeadlineSeconds == nil || *spec.Spec.ActiveDeadlineSeconds != 1800 {
		t.Fatalf("active deadline seconds = %#v", spec.Spec.ActiveDeadlineSeconds)
	}
	pod := spec.Spec.Template
	if pod.Labels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("pod labels = %#v", pod.Labels)
	}
	if pod.Spec.ServiceAccountName != buildJobServiceAccountName {
		t.Fatalf("service account = %q", pod.Spec.ServiceAccountName)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("automount service account token = %#v", pod.Spec.AutomountServiceAccountToken)
	}
	container := pod.Spec.Containers[0]
	if container.Resources.Requests.Cpu().String() != "750m" || container.Resources.Limits.Memory().String() != "768Mi" {
		t.Fatalf("resources = %#v", container.Resources)
	}
	if container.SecurityContext == nil {
		t.Fatal("container security context is nil")
	}
	if container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
		t.Fatalf("container should not be privileged: %#v", container.SecurityContext)
	}
	if container.SecurityContext.AllowPrivilegeEscalation == nil || !*container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("rootless BuildKit requires privilege escalation for newuidmap/newgidmap: %#v", container.SecurityContext)
	}
	if container.SecurityContext.RunAsUser == nil || *container.SecurityContext.RunAsUser != 1000 {
		t.Fatalf("runAsUser = %#v", container.SecurityContext.RunAsUser)
	}
	if container.SecurityContext.RunAsGroup == nil || *container.SecurityContext.RunAsGroup != 1000 {
		t.Fatalf("runAsGroup = %#v", container.SecurityContext.RunAsGroup)
	}
	if container.SecurityContext.SeccompProfile == nil || container.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeUnconfined {
		t.Fatalf("seccomp profile = %#v", container.SecurityContext.SeccompProfile)
	}
	if container.SecurityContext.AppArmorProfile == nil || container.SecurityContext.AppArmorProfile.Type != corev1.AppArmorProfileTypeUnconfined {
		t.Fatalf("appArmor profile = %#v", container.SecurityContext.AppArmorProfile)
	}
	var foundBuildkitFlags bool
	for _, env := range container.Env {
		if env.Name == "BUILDKITD_FLAGS" && strings.Contains(env.Value, "--oci-worker-no-process-sandbox") {
			foundBuildkitFlags = true
		}
	}
	if !foundBuildkitFlags {
		t.Fatalf("BUILDKITD_FLAGS not configured: %#v", container.Env)
	}
}

func TestBuildJobSpecCopiesOnlyProjectedExecutorFiles(t *testing.T) {
	spec := buildJobSpec(
		"build-job-1",
		"build-job-1-secret",
		model.Environment{ID: "env_dev"},
		model.BuildRun{},
		builder.Task{
			ProjectID:          "prj_demo",
			ApplicationID:      "app_api",
			DeploymentTargetID: "dplt_api",
			BuildRunID:         "brn_1",
			JobID:              "bjb_1",
			Build: builder.BuildPayload{Hooks: []builder.HookPayload{{
				ID:     "prebuild",
				Script: "echo prebuild",
			}}},
		},
		"moby/buildkit:v0.24.0-rootless",
		"",
		false,
		"buildcache",
		1800,
		3600,
	)

	container := spec.Spec.Template.Spec.Containers[0]
	command := strings.Join(container.Command, " ")
	if strings.Contains(command, "cp -R /executor/.") {
		t.Fatalf("executor command should not copy projected volume internals: %s", command)
	}
	if !strings.Contains(command, "cp /executor/run.sh /workspace/run.sh") {
		t.Fatalf("executor command should copy run.sh explicitly: %s", command)
	}

	var scriptModes []int32
	for _, volume := range spec.Spec.Template.Spec.Volumes {
		if volume.Name != "executor-files" || volume.Secret == nil {
			continue
		}
		for _, item := range volume.Secret.Items {
			if strings.HasSuffix(item.Path, ".sh") {
				if item.Mode == nil {
					t.Fatalf("script item %s mode is nil", item.Path)
				}
				scriptModes = append(scriptModes, *item.Mode)
			}
		}
	}
	if len(scriptModes) != 2 {
		t.Fatalf("script modes = %#v", scriptModes)
	}
	for _, mode := range scriptModes {
		if mode != 0o555 {
			t.Fatalf("script mode = %#o, want 0555", mode)
		}
	}
}

func TestEnsureBuildJobServiceAccountDisablesTokenAutomount(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta:                   metav1.ObjectMeta{Name: buildJobServiceAccountName, Namespace: "ns-demo"},
		AutomountServiceAccountToken: boolPtr(true),
	})

	if err := ensureBuildJobServiceAccount(context.Background(), client, "ns-demo"); err != nil {
		t.Fatalf("ensureBuildJobServiceAccount returned error: %v", err)
	}

	serviceAccount, err := client.CoreV1().ServiceAccounts("ns-demo").Get(context.Background(), buildJobServiceAccountName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service account: %v", err)
	}
	if serviceAccount.Labels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("labels = %#v", serviceAccount.Labels)
	}
	if serviceAccount.AutomountServiceAccountToken == nil || *serviceAccount.AutomountServiceAccountToken {
		t.Fatalf("automount service account token = %#v", serviceAccount.AutomountServiceAccountToken)
	}
}

func TestWaitForBuildPodWaitsUntilExecutorLogsAreAvailable(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "build-job-pod",
			Namespace: "ns-demo",
			Labels:    map[string]string{"job-name": "build-job"},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "executor",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
			}},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		pod, err := client.CoreV1().Pods("ns-demo").Get(context.Background(), "build-job-pod", metav1.GetOptions{})
		if err != nil {
			return
		}
		pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}}
		_, _ = client.CoreV1().Pods("ns-demo").UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
	}()

	podName, err := waitForBuildPod(ctx, client, "ns-demo", "build-job")
	if err != nil {
		t.Fatalf("waitForBuildPod returned error: %v", err)
	}
	if podName != "build-job-pod" {
		t.Fatalf("podName = %q", podName)
	}
}

func TestWaitForBuildPodReturnsFatalStartupError(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "build-job-pod",
			Namespace: "ns-demo",
			Labels:    map[string]string{"job-name": "build-job"},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "executor",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull failed"}},
			}},
		},
	})

	_, err := waitForBuildPod(context.Background(), client, "ns-demo", "build-job")
	if err == nil || !strings.Contains(err.Error(), "ImagePullBackOff") {
		t.Fatalf("expected ImagePullBackOff error, got %v", err)
	}
}

func TestBuildKubernetesJobFailureMessageIncludesPodTerminationAndEvent(t *testing.T) {
	now := metav1.Now()
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "build-job-pod",
				Namespace: "ns-demo",
				Labels:    map[string]string{"job-name": "build-job"},
			},
			Status: corev1.PodStatus{
				Phase:  corev1.PodFailed,
				Reason: "OOMKilled",
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "executor",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						Reason:   "OOMKilled",
						ExitCode: 137,
					}},
				}},
			},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "build-job-pod.1", Namespace: "ns-demo"},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Namespace: "ns-demo",
				Name:      "build-job-pod",
			},
			Type:          corev1.EventTypeWarning,
			Reason:        "BackOff",
			Message:       "Back-off restarting failed container executor",
			LastTimestamp: now,
		},
	)
	runner := NewRunner(nil, Options{})

	message := runner.buildKubernetesJobFailureMessage(context.Background(), client, "ns-demo", "build-job", "kubernetes build job failed")

	for _, expected := range []string{"kubernetes build job failed", "OOMKilled", "exitCode=137", "Back-off restarting failed container executor"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message %q missing %q", message, expected)
		}
	}
}

func TestGatewayIngressSpecTargetsApplicationService(t *testing.T) {
	spec := gatewayIngressSpec(
		model.GatewayRoute{ID: "gwr_ABC_123", Host: "api.example.com", Path: "api", ServicePort: 8080, TLSMode: "http-challenge"},
		model.Project{ID: "prj_demo"},
		model.Application{Slug: "api"},
		model.Environment{Slug: "dev"},
		"project-demo",
		"dplt-backend",
	)
	if spec.Name != "liteyuki-gateway-gwr-abc-123" || spec.ServiceName != "dplt-backend" || spec.Path != "api" {
		t.Fatalf("spec = %#v", spec)
	}
	if spec.TLSSecretName != "tls-api-example-com" {
		t.Fatalf("tls secret = %q", spec.TLSSecretName)
	}
}

func TestGatewayIngressSpecOmitsTLSForHTTPOnly(t *testing.T) {
	spec := gatewayIngressSpec(
		model.GatewayRoute{ID: "gwr_1", Host: "api.example.com", ServicePort: 3000, TLSMode: "http-only"},
		model.Project{ID: "prj_demo"},
		model.Application{Slug: "api"},
		model.Environment{Slug: "dev"},
		"project-demo",
		"",
	)
	if spec.TLSSecretName != "" || spec.ServicePort != 3000 {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestGatewayCertificateSpecUsesRouteTLSSecret(t *testing.T) {
	spec := gatewayCertificateSpec(
		model.GatewayRoute{ID: "gwr_1", Host: "api.example.com", TLSMode: "http-challenge"},
		model.Project{ID: "prj_demo"},
		"project-demo",
		"letsencrypt-staging",
	)
	if spec.Name != "liteyuki-gateway-gwr-1" || spec.SecretName != "tls-api-example-com" || spec.ClusterIssuer != "letsencrypt-staging" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestGatewayDNSStatusVerifiesCNAME(t *testing.T) {
	runner := NewRunner(nil, Options{})
	runner.dnsResolver = fakeCNameResolver{cname: "gateway.example.com."}

	status := runner.gatewayDNSStatus(context.Background(), model.GatewayRoute{Host: "app.example.com", CNAMETarget: "gateway.example.com"})
	if status != "verified" {
		t.Fatalf("status = %q", status)
	}
}

func TestGatewayDNSStatusFailsOnMismatch(t *testing.T) {
	runner := NewRunner(nil, Options{})
	runner.dnsResolver = fakeCNameResolver{err: fmt.Errorf("not found")}

	status := runner.gatewayDNSStatus(context.Background(), model.GatewayRoute{Host: "app.example.com", CNAMETarget: "gateway.example.com"})
	if status != "failed" {
		t.Fatalf("status = %q", status)
	}
}

func TestParseKeyValueMapSupportsJSONObject(t *testing.T) {
	got, err := parseKeyValueMap(`{"APP_ENV":"prod","REPLICAS":2}`)
	if err != nil {
		t.Fatalf("parseKeyValueMap returned error: %v", err)
	}
	if got["APP_ENV"] != "prod" || got["REPLICAS"] != "2" {
		t.Fatalf("values = %#v", got)
	}
}

type fakeCNameResolver struct {
	cname string
	err   error
}

func (r fakeCNameResolver) LookupCNAME(context.Context, string) (string, error) {
	return r.cname, r.err
}

type fakeNamespaceManager struct{}

func (fakeNamespaceManager) EnsureNamespace(context.Context, string, map[string]string) error {
	return nil
}

func (fakeNamespaceManager) Ping(context.Context) error {
	return nil
}

func (fakeNamespaceManager) EnsureBuildNetworkPolicy(context.Context, kubeprovider.BuildNetworkPolicySpec) error {
	return nil
}

func (fakeNamespaceManager) EnsureBuildPolicy(context.Context, networkpolicy.BuildPolicy) error {
	return nil
}

func (fakeNamespaceManager) ApplyApplicationResources(context.Context, kubeprovider.ApplicationResourcesSpec) error {
	return nil
}

func (fakeNamespaceManager) ApplyApplicationRuntimeConfig(context.Context, kubeprovider.ApplicationResourcesSpec) error {
	return nil
}

func (fakeNamespaceManager) RunHookJob(context.Context, kubeprovider.HookJobSpec) (kubeprovider.HookJobResult, error) {
	return kubeprovider.HookJobResult{}, nil
}

func (fakeNamespaceManager) GetDeploymentSnapshot(context.Context, string, string) (kubeprovider.DeploymentSnapshot, error) {
	return kubeprovider.DeploymentSnapshot{}, nil
}

func (fakeNamespaceManager) ApplyGatewayIngress(context.Context, kubeprovider.GatewayIngressSpec) error {
	return nil
}

func (fakeNamespaceManager) ApplyCertificate(context.Context, kubeprovider.CertificateSpec) error {
	return nil
}

func (fakeNamespaceManager) GetCertificateSnapshot(context.Context, string, string) (kubeprovider.CertificateSnapshot, error) {
	return kubeprovider.CertificateSnapshot{}, nil
}

func (fakeNamespaceManager) ListManagedResources(context.Context, kubeprovider.ResourceListOptions) ([]kubeprovider.ResourceSnapshot, error) {
	return nil, nil
}

func (fakeNamespaceManager) DeleteManagedResource(context.Context, string, string, string) error {
	return nil
}

type recordingNamespaceManager struct {
	fakeNamespaceManager
	deletions []string
	policies  []networkpolicy.BuildPolicy
	err       error
}

func (m *recordingNamespaceManager) DeleteManagedResource(_ context.Context, kind string, namespace string, name string) error {
	m.deletions = append(m.deletions, kind+"/"+namespace+"/"+name)
	return m.err
}

func (m *recordingNamespaceManager) EnsureBuildPolicy(_ context.Context, policy networkpolicy.BuildPolicy) error {
	m.policies = append(m.policies, policy)
	return m.err
}

func TestEnsureProjectNamespaceAppliesBuildEgressPolicy(t *testing.T) {
	manager := &recordingNamespaceManager{}
	runner := NewRunner(nil, Options{})
	runner.kubernetesManagerFactory = func(model.Environment) (kubeprovider.NamespaceManager, error) {
		return manager, nil
	}

	if err := runner.ensureProjectNamespace(context.Background(), "ns-demo", model.Project{ID: "prj_demo"}, model.Environment{}); err != nil {
		t.Fatalf("ensureProjectNamespace returned error: %v", err)
	}
	if len(manager.policies) != 1 {
		t.Fatalf("policies = %#v", manager.policies)
	}
	policy := manager.policies[0]
	if policy.Name != "liteyuki-build-egress" || policy.Namespace != "ns-demo" || policy.PodLabels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("policy = %#v", policy)
	}
	if len(policy.Egress) != 1 || len(policy.Egress[0].To) != 0 || len(policy.Egress[0].Ports) != 0 {
		t.Fatalf("expected permissive egress rule, got %#v", policy.Egress)
	}
}

func TestEnsureProjectNamespaceAppliesRestrictedBuildEgressPolicy(t *testing.T) {
	manager := &recordingNamespaceManager{}
	runner := NewRunner(nil, Options{
		BuildEgressMode:         "restricted",
		BuildPrivateEgressCIDRs: []string{"10.20.0.0/16"},
		BuildPrivateEgressPorts: []int{443, 5000},
		BuildBlockedEgressCIDRs: []string{"169.254.169.254/32", "10.96.0.0/12"},
	})
	runner.kubernetesManagerFactory = func(model.Environment) (kubeprovider.NamespaceManager, error) {
		return manager, nil
	}

	if err := runner.ensureProjectNamespace(context.Background(), "ns-demo", model.Project{ID: "prj_demo"}, model.Environment{}); err != nil {
		t.Fatalf("ensureProjectNamespace returned error: %v", err)
	}
	if len(manager.policies) != 1 {
		t.Fatalf("policies = %#v", manager.policies)
	}
	policy := manager.policies[0]
	if policy.Name != "liteyuki-build-egress" || policy.Namespace != "ns-demo" || policy.PodLabels[kubeprovider.ScopeLabel] != buildJobScope {
		t.Fatalf("policy = %#v", policy)
	}
	if len(policy.Egress) < 4 {
		t.Fatalf("expected dns, public, and private egress rules, got %#v", policy.Egress)
	}
	privateRule := policy.Egress[len(policy.Egress)-1]
	if privateRule.To[0].CIDR != "10.20.0.0/16" || len(privateRule.Ports) != 2 || privateRule.Ports[0].Number != 443 || privateRule.Ports[1].Number != 5000 {
		t.Fatalf("private egress rule = %#v", privateRule)
	}
}

func TestResourceCleanupCanRunAllowsRetryableStates(t *testing.T) {
	for _, status := range []string{"deleting", "delete_failed"} {
		if !resourceCleanupCanRun(status) {
			t.Fatalf("expected status %q to be cleanup runnable", status)
		}
	}
	for _, status := range []string{"", "active", "deleted"} {
		if resourceCleanupCanRun(status) {
			t.Fatalf("expected status %q to be skipped", status)
		}
	}
}

func TestCleanupProjectNamespacesCoversDistinctClusters(t *testing.T) {
	runner := NewRunner(nil, Options{})
	managers := map[string]*recordingNamespaceManager{}
	runner.kubernetesManagerFactory = func(environment model.Environment) (kubeprovider.NamespaceManager, error) {
		key := projectCleanupEnvironmentKey(environment)
		manager := managers[key]
		if manager == nil {
			manager = &recordingNamespaceManager{}
			managers[key] = manager
		}
		return manager, nil
	}

	project := model.Project{ID: "prj_abcdef1234567890", Slug: "demo"}
	targets := []model.DeploymentTarget{
		{ID: "dplt_dev", ClusterID: "rcl_one"},
		{ID: "dplt_prod", ClusterID: "rcl_two"},
		{ID: "dplt_stage", ClusterID: "rcl_one"},
		{ID: "dplt_default"},
	}

	if err := runner.cleanupProjectNamespacesForDeploymentTargets(context.Background(), project, targets); err != nil {
		t.Fatalf("cleanupProjectNamespacesForDeploymentTargets returned error: %v", err)
	}
	for _, key := range []string{"cluster:rcl_one", "cluster:rcl_two", "default"} {
		manager := managers[key]
		if manager == nil {
			t.Fatalf("manager %q was not used", key)
		}
		if len(manager.deletions) != 1 || manager.deletions[0] != "Namespace//ns-abcdef1234" {
			t.Fatalf("manager %q deletions = %#v", key, manager.deletions)
		}
	}
}

func TestDeleteManagedNamespaceIgnoresKubernetesNotFound(t *testing.T) {
	manager := &recordingNamespaceManager{
		err: apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "ns-demo"),
	}

	if err := deleteManagedNamespace(context.Background(), manager, "ns-demo"); err != nil {
		t.Fatalf("deleteManagedNamespace returned error: %v", err)
	}
	if len(manager.deletions) != 1 {
		t.Fatalf("deletions = %#v", manager.deletions)
	}
}

func TestParseKeyValueMapSupportsEnvLines(t *testing.T) {
	got, err := parseKeyValueMap("APP_ENV=prod\n# comment\nLOG_LEVEL=info")
	if err != nil {
		t.Fatalf("parseKeyValueMap returned error: %v", err)
	}
	if got["APP_ENV"] != "prod" || got["LOG_LEVEL"] != "info" {
		t.Fatalf("values = %#v", got)
	}
}

func TestApplicationResourcesSpecAppliesDefaults(t *testing.T) {
	spec, err := applicationResourcesSpec(
		model.Release{ImageRef: "registry.example.com/acme/api:v1"},
		model.Project{ID: "prj_demo", Slug: "demo"},
		model.Application{ID: "app_api", Slug: "api"},
		model.Environment{ID: "env_dev", Slug: "dev", EnvVars: `{"APP_ENV":"dev"}`, ConfigRefs: "LOG_LEVEL=debug", SecretRefs: "TOKEN=secret"},
		model.DeploymentTarget{ID: "dplt_backend"},
		nil,
		"ns-demo",
		120,
	)
	if err != nil {
		t.Fatalf("applicationResourcesSpec returned error: %v", err)
	}
	if spec.Name != "dplt-backend" || spec.Namespace != "ns-demo" || spec.DeploymentTargetID != "dplt_backend" || spec.ServicePort != 8080 || spec.Replicas != 1 || spec.RolloutTimeoutSeconds != 120 {
		t.Fatalf("spec defaults = %#v", spec)
	}
	if spec.ConfigData["APP_ENV"] != "dev" || spec.ConfigData["LOG_LEVEL"] != "debug" || spec.SecretData["TOKEN"] != "secret" {
		t.Fatalf("spec data = config:%#v secret:%#v", spec.ConfigData, spec.SecretData)
	}
}

func TestApplicationResourcesSpecMergesRuntimeConfigFiles(t *testing.T) {
	spec, err := applicationResourcesSpec(
		model.Release{ImageRef: "registry.example.com/acme/api:v1"},
		model.Project{ID: "prj_demo"},
		model.Application{ID: "app_api"},
		model.Environment{ID: "env_dev"},
		model.DeploymentTarget{ID: "dplt_backend", ConfigFiles: `[{"path":"/app/config.yaml","content":"port: 3000"}]`},
		[]model.ProjectRuntimeConfigSet{{ConfigFiles: `[{"path":"/app/config.yaml","content":"port: 8080"},{"path":"/app/base.yaml","content":"enabled: true"}]`}},
		"ns-demo",
		120,
	)
	if err != nil {
		t.Fatalf("applicationResourcesSpec returned error: %v", err)
	}
	if len(spec.ConfigFiles) != 2 {
		t.Fatalf("config files = %#v", spec.ConfigFiles)
	}
	filesByPath := map[string]string{}
	for _, file := range spec.ConfigFiles {
		filesByPath[file.Path] = file.Content
	}
	if filesByPath["/app/config.yaml"] != "port: 3000" || filesByPath["/app/base.yaml"] != "enabled: true" {
		t.Fatalf("config files = %#v", spec.ConfigFiles)
	}
}

func TestReleaseFinishUpdatesIncludesTerminalFields(t *testing.T) {
	finishedAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	updates := releaseFinishUpdates("succeeded", "rollout completed", finishedAt)
	if updates["status"] != "succeeded" || updates["message"] != "rollout completed" {
		t.Fatalf("updates = %#v", updates)
	}
	gotFinishedAt, ok := updates["finished_at"].(*time.Time)
	if !ok || !gotFinishedAt.Equal(finishedAt) {
		t.Fatalf("finished_at = %#v", updates["finished_at"])
	}
}

func TestGitAccountDueForWorkerRefresh(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	soon := now.Add(4 * time.Minute)
	later := now.Add(10 * time.Minute)
	if !gitAccountDueForWorkerRefresh(model.GitAccount{Status: "connected", RefreshTokenRef: "secret", ExpiresAt: &soon}, now) {
		t.Fatal("expected account expiring soon to be due")
	}
	if gitAccountDueForWorkerRefresh(model.GitAccount{Status: "connected", RefreshTokenRef: "secret", ExpiresAt: &later}, now) {
		t.Fatal("expected account outside refresh window to be skipped")
	}
	if gitAccountDueForWorkerRefresh(model.GitAccount{Status: "expired", RefreshTokenRef: "secret", ExpiresAt: &soon}, now) {
		t.Fatal("expected expired account to be skipped")
	}
}

func TestGitAccountDueForWorkerRefreshSkipsAfterSuccessfulRefresh(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	refreshedExpiry := now.Add(1 * time.Hour)
	account := model.GitAccount{Status: "connected", RefreshTokenRef: "secret", ExpiresAt: &refreshedExpiry}
	if gitAccountDueForWorkerRefresh(account, now) {
		t.Fatal("expected refreshed account to be skipped on replay")
	}
}
