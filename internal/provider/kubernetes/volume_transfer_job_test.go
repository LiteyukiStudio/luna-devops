package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestBuildVolumeTransferJobKeepsTokenInSecretOnly(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	ctx := context.Background()
	resources, err := client.buildVolumeTransferResources(ctx, filesystemTransferJobSpec("import"))
	if err != nil {
		t.Fatalf("buildVolumeTransferResources returned error: %v", err)
	}
	if got := string(resources.secret.Data[volumeTransferTokenKey]); got != strings.Repeat("t", 32) {
		t.Fatalf("secret token = %q", got)
	}
	container := resources.job.Spec.Template.Spec.Containers[0]
	serializedPod := fmt.Sprintf("%#v", resources.job.Spec.Template)
	if strings.Contains(serializedPod, strings.Repeat("t", 32)) {
		t.Fatal("callback token leaked into the Job Pod template")
	}
	if len(container.Command) != 1 || container.Command[0] != volumeTransferBinaryPath || len(container.Args) != 0 {
		t.Fatalf("unexpected command: %#v %#v", container.Command, container.Args)
	}
	if got := environmentValue(container.Env, "LUNA_VOLUME_TRANSFER_TOKEN_FILE"); got != volumeTransferTokenFilePath {
		t.Fatalf("token file env = %q", got)
	}
	if got := resources.secret.Annotations; len(got) != 0 {
		t.Fatalf("secret annotations = %#v", got)
	}
	if got := resources.job.Annotations; len(got) != 0 {
		t.Fatalf("job annotations = %#v", got)
	}
	if resources.job.Spec.Template.Spec.AutomountServiceAccountToken == nil || *resources.job.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("service account token automount must be disabled")
	}
	var tokenSource *corev1.SecretVolumeSource
	for _, volume := range resources.job.Spec.Template.Spec.Volumes {
		if volume.Name == volumeTransferTokenVolumeName {
			tokenSource = volume.Secret
		}
	}
	if tokenSource == nil || tokenSource.SecretName != resources.reference.SecretName || tokenSource.DefaultMode == nil || *tokenSource.DefaultMode != 0o440 {
		t.Fatalf("callback token volume = %#v", tokenSource)
	}
	assertTransferPVCMode(t, resources.job, false, false)
}

func TestBuildVolumeTransferJobSeparatesFilesystemAndBlockModes(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())

	exportResources, err := client.buildVolumeTransferResources(context.Background(), filesystemTransferJobSpec("export"))
	if err != nil {
		t.Fatalf("build filesystem export: %v", err)
	}
	assertTransferPVCMode(t, exportResources.job, true, false)

	blockSpec := filesystemTransferJobSpec("import")
	blockSpec.VolumeMode = "Block"
	blockSpec.Format = "raw_zst"
	blockResources, err := client.buildVolumeTransferResources(context.Background(), blockSpec)
	if err != nil {
		t.Fatalf("build block import: %v", err)
	}
	assertTransferPVCMode(t, blockResources.job, false, true)
	container := blockResources.job.Spec.Template.Spec.Containers[0]
	if len(container.VolumeDevices) != 1 || container.VolumeDevices[0].DevicePath != volumeTransferBlockDevicePath {
		t.Fatalf("block volume devices = %#v", container.VolumeDevices)
	}
	if environmentValue(container.Env, "LUNA_VOLUME_TRANSFER_DATA_PATH") != volumeTransferBlockDevicePath {
		t.Fatal("block data path was not selected")
	}
	if container.SecurityContext == nil || container.SecurityContext.RunAsUser == nil || *container.SecurityContext.RunAsUser != 0 || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("block security context = %#v", container.SecurityContext)
	}
}

func TestBuildVolumeTransferJobSizesScratchForFiveTiBTransfer(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := filesystemTransferJobSpec("import")
	spec.CapacityBytes = 5 * 1024 * 1024 * 1024 * 1024
	spec.ExpectedBytes = spec.CapacityBytes
	spec.ChunkSize = volumetransfer.RequiredChunkSize(spec.ExpectedBytes)
	resources, err := client.buildVolumeTransferResources(context.Background(), spec)
	if err != nil {
		t.Fatalf("buildVolumeTransferResources returned error: %v", err)
	}
	if spec.ChunkSize != 525*1024*1024 {
		t.Fatalf("chunk size = %d, want 525 MiB", spec.ChunkSize)
	}
	container := resources.job.Spec.Template.Spec.Containers[0]
	if got := environmentValue(container.Env, "LUNA_VOLUME_TRANSFER_CHUNK_SIZE"); got != fmt.Sprintf("%d", spec.ChunkSize) {
		t.Fatalf("chunk size env = %q, want %d", got, spec.ChunkSize)
	}
	wantScratchBytes := spec.ChunkSize + volumeTransferScratchOverheadBytes
	requestQuantity := container.Resources.Requests[corev1.ResourceEphemeralStorage]
	if got := requestQuantity.Value(); got != wantScratchBytes {
		t.Fatalf("ephemeral storage request = %d, want %d", got, wantScratchBytes)
	}
	limitQuantity := container.Resources.Limits[corev1.ResourceEphemeralStorage]
	if got := limitQuantity.Value(); got < wantScratchBytes+volumeTransferScratchOverheadBytes {
		t.Fatalf("ephemeral storage limit = %d, want at least %d", got, wantScratchBytes+volumeTransferScratchOverheadBytes)
	}
	for _, item := range resources.job.Spec.Template.Spec.Volumes {
		if item.Name == volumeTransferScratchVolumeName {
			if item.EmptyDir == nil || item.EmptyDir.SizeLimit == nil || item.EmptyDir.SizeLimit.Value() != wantScratchBytes {
				t.Fatalf("scratch volume = %#v, want %d bytes", item.EmptyDir, wantScratchBytes)
			}
			return
		}
	}
	t.Fatal("scratch volume is missing")
}

func TestBuildVolumeTransferJobPropagatesOnlyTraceContext(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	ctx, span := provider.Tracer("test").Start(context.Background(), "parent")
	defer span.End()
	resources, err := NewClientForInterface(fake.NewSimpleClientset()).buildVolumeTransferResources(ctx, filesystemTransferJobSpec("import"))
	if err != nil {
		t.Fatalf("buildVolumeTransferResources returned error: %v", err)
	}
	environment := resources.job.Spec.Template.Spec.Containers[0].Env
	if value := environmentValue(environment, "LUNA_VOLUME_TRANSFER_TRACEPARENT"); value == "" {
		t.Fatal("traceparent was not propagated")
	}
	if value := environmentValue(environment, "OTEL_BAGGAGE"); value != "" {
		t.Fatalf("unexpected baggage environment = %q", value)
	}
	if strings.Contains(fmt.Sprintf("%#v", resources.job.Annotations), "traceparent") {
		t.Fatal("trace context must not be stored in annotations")
	}
}

func TestVolumeTransferNetworkPolicyAllowsOnlyCallbackAndDNS(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := filesystemTransferJobSpec("import")
	spec.CallbackBaseURL = "https://api.internal.example:8443/luna"
	spec.CallbackCIDRs = []string{"10.20.30.40/32"}
	resources, err := client.buildVolumeTransferResources(context.Background(), spec)
	if err != nil {
		t.Fatalf("buildVolumeTransferResources returned error: %v", err)
	}
	egress := resources.policy.Spec.Egress
	if len(egress) != 2 {
		t.Fatalf("egress rules = %#v", egress)
	}
	if len(egress[0].To) != 1 || egress[0].To[0].IPBlock == nil || egress[0].To[0].IPBlock.CIDR != "10.20.30.40/32" {
		t.Fatalf("callback egress = %#v", egress[0])
	}
	if len(egress[0].Ports) != 1 || egress[0].Ports[0].Port == nil || egress[0].Ports[0].Port.IntVal != 8443 {
		t.Fatalf("callback ports = %#v", egress[0].Ports)
	}
	if egress[1].To[0].NamespaceSelector == nil || egress[1].To[0].PodSelector == nil {
		t.Fatalf("DNS egress is not restricted to the DNS workload: %#v", egress[1])
	}
	if len(resources.policy.Spec.Ingress) != 0 || len(resources.policy.Spec.PolicyTypes) != 1 || resources.policy.Spec.PolicyTypes[0] != "Egress" {
		t.Fatalf("network policy scope = %#v", resources.policy.Spec)
	}
}

func TestCreateObserveAndCancelVolumeTransferJob(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := filesystemTransferJobSpec("import")
	reference, err := client.CreateVolumeTransferJob(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateVolumeTransferJob returned error: %v", err)
	}
	if _, err := clientset.CoreV1().Secrets(spec.Namespace).Get(context.Background(), reference.SecretName, metav1.GetOptions{}); err != nil {
		t.Fatalf("callback Secret was not created: %v", err)
	}
	if _, err := clientset.NetworkingV1().NetworkPolicies(spec.Namespace).Get(context.Background(), reference.NetworkPolicyName, metav1.GetOptions{}); err != nil {
		t.Fatalf("NetworkPolicy was not created: %v", err)
	}
	job, err := clientset.BatchV1().Jobs(spec.Namespace).Get(context.Background(), reference.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Job was not created: %v", err)
	}
	now := metav1.NewTime(time.Now().UTC())
	job.Status.StartTime = &now
	job.Status.Active = 1
	if _, err := clientset.BatchV1().Jobs(spec.Namespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update Job status: %v", err)
	}
	observation, err := client.ObserveVolumeTransferJob(context.Background(), spec.Namespace, spec.TransferID)
	if err != nil {
		t.Fatalf("ObserveVolumeTransferJob returned error: %v", err)
	}
	if observation.State != "running" || observation.StartedAt == nil {
		t.Fatalf("observation = %#v", observation)
	}

	if err := client.CancelVolumeTransferJob(context.Background(), spec.Namespace, spec.TransferID); err != nil {
		t.Fatalf("CancelVolumeTransferJob returned error: %v", err)
	}
	if _, err := clientset.CoreV1().Secrets(spec.Namespace).Get(context.Background(), reference.SecretName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("callback Secret lookup after cancellation = %v", err)
	}
	observation, err = client.ObserveVolumeTransferJob(context.Background(), spec.Namespace, spec.TransferID)
	if err != nil || observation.State != "not_found" {
		t.Fatalf("post-cancel observation = %#v, %v", observation, err)
	}
}

func TestCleanupVolumeTransferJobRemovesTerminalResourcesIdempotently(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := filesystemTransferJobSpec("export")
	reference, err := client.CreateVolumeTransferJob(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CleanupVolumeTransferJob(context.Background(), spec.Namespace, spec.TransferID); err != nil {
		t.Fatalf("CleanupVolumeTransferJob returned error: %v", err)
	}
	if _, err := clientset.BatchV1().Jobs(spec.Namespace).Get(context.Background(), reference.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Job lookup after cleanup = %v", err)
	}
	if _, err := clientset.CoreV1().Secrets(spec.Namespace).Get(context.Background(), reference.SecretName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Secret lookup after cleanup = %v", err)
	}
	if _, err := clientset.NetworkingV1().NetworkPolicies(spec.Namespace).Get(context.Background(), reference.NetworkPolicyName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("NetworkPolicy lookup after cleanup = %v", err)
	}
	if err := client.CleanupVolumeTransferJob(context.Background(), spec.Namespace, spec.TransferID); err != nil {
		t.Fatalf("idempotent CleanupVolumeTransferJob returned error: %v", err)
	}
}

func TestCleanupVolumeTransferJobWaitsForAuthoritativeJobDeletion(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := filesystemTransferJobSpec("export")
	reference, err := client.CreateVolumeTransferJob(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an API server accepting foreground deletion while the Job and
	// its dependent Pods are still terminating. Cleanup must not remove callback
	// prerequisites or report success until authoritative reads return NotFound.
	clientset.PrependReactor("delete", "jobs", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := client.CleanupVolumeTransferJob(ctx, spec.Namespace, spec.TransferID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("delayed cleanup error=%v, want context deadline exceeded", err)
	}
	if _, err := clientset.BatchV1().Jobs(spec.Namespace).Get(context.Background(), reference.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("terminating Job was removed by the fake reactor: %v", err)
	}
	if _, err := clientset.CoreV1().Secrets(spec.Namespace).Get(context.Background(), reference.SecretName, metav1.GetOptions{}); err != nil {
		t.Fatalf("callback Secret was removed before Job authority: %v", err)
	}
	if _, err := clientset.NetworkingV1().NetworkPolicies(spec.Namespace).Get(context.Background(), reference.NetworkPolicyName, metav1.GetOptions{}); err != nil {
		t.Fatalf("NetworkPolicy was removed before Job authority: %v", err)
	}
}

func TestVolumeTransferJobObservationUsesStableStatesAndReasons(t *testing.T) {
	cases := []struct {
		name   string
		status batchv1.JobStatus
		state  string
		reason string
	}{
		{name: "pending", state: "pending", reason: "waiting"},
		{name: "running", status: batchv1.JobStatus{Active: 1}, state: "running", reason: "running"},
		{name: "succeeded", status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}, state: "succeeded", reason: "completed"},
		{name: "deadline", status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "DeadlineExceeded"}}}, state: "failed", reason: "deadline_exceeded"},
		{name: "unknown failure", status: batchv1.JobStatus{Failed: 1}, state: "failed", reason: "job_failed"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			observation := volumeTransferJobObservation(&batchv1.Job{Status: item.status})
			if observation.State != item.state || observation.Reason != item.reason {
				t.Fatalf("observation = %#v, want state=%q reason=%q", observation, item.state, item.reason)
			}
		})
	}
}

func TestCreateVolumeTransferJobOnlyReusesMatchingExecution(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := filesystemTransferJobSpec("import")
	first, err := client.CreateVolumeTransferJob(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.CreateVolumeTransferJob(context.Background(), spec)
	if err != nil || second != first {
		t.Fatalf("idempotent create = %#v, %v", second, err)
	}
	changedToken := spec
	changedToken.CallbackToken = []byte(strings.Repeat("x", 32))
	if _, err := client.CreateVolumeTransferJob(context.Background(), changedToken); !errors.Is(err, ErrVolumeTransferJobConflict) {
		t.Fatalf("changed token error = %v", err)
	}
	changedImage := spec
	changedImage.Image = "example.invalid/luna-worker:different"
	if _, err := client.CreateVolumeTransferJob(context.Background(), changedImage); !errors.Is(err, ErrVolumeTransferJobConflict) {
		t.Fatalf("changed image error = %v", err)
	}
}

func TestCreateVolumeTransferJobConcurrentRetriesShareOneExecution(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := filesystemTransferJobSpec("import")

	const callers = 12
	results := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := client.CreateVolumeTransferJob(context.Background(), spec)
			results <- err
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent CreateVolumeTransferJob returned error: %v", err)
		}
	}
	jobs, err := clientset.BatchV1().Jobs(spec.Namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("created Jobs = %d, want 1", len(jobs.Items))
	}
}

func TestVolumeTransferJobRejectsUnsafeCallbackCIDRAndCancelledContext(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := filesystemTransferJobSpec("import")
	spec.CallbackCIDRs = []string{"0.0.0.0/0"}
	if _, err := client.buildVolumeTransferResources(context.Background(), spec); !errors.Is(err, ErrInvalidVolumeTransferJobSpec) {
		t.Fatalf("unsafe callback CIDR error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.CreateVolumeTransferJob(ctx, filesystemTransferJobSpec("import")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled create error = %v", err)
	}

	invalidToken := filesystemTransferJobSpec("import")
	invalidToken.CallbackToken = append([]byte(strings.Repeat("t", 31)), 0xff)
	if _, err := client.buildVolumeTransferResources(context.Background(), invalidToken); !errors.Is(err, ErrInvalidVolumeTransferJobSpec) {
		t.Fatalf("non-ASCII callback token error = %v", err)
	}
}

func filesystemTransferJobSpec(direction string) VolumeTransferJobSpec {
	spec := VolumeTransferJobSpec{
		TransferID:      "vtx_test",
		ProjectID:       "prj_test",
		ProjectVolumeID: "pvol_test",
		Namespace:       "project-test",
		ClaimName:       "luna-pvol-test",
		Direction:       direction,
		Format:          "tar_gz",
		VolumeMode:      "Filesystem",
		CallbackBaseURL: "https://10.20.30.40",
		CallbackToken:   []byte(strings.Repeat("t", 32)),
		Image:           "example.invalid/luna-worker:test",
		CapacityBytes:   1024 * 1024,
		ExpectedBytes:   1024,
		ExpectedSHA256:  strings.Repeat("a", 64),
	}
	if direction == "export" {
		spec.ConsistencyMode = "unmounted"
		spec.ExportedAt = time.Unix(1_700_000_000, 0).UTC()
	}
	return spec
}

func assertTransferPVCMode(t *testing.T, job *batchv1.Job, readOnly, block bool) {
	t.Helper()
	var claim *corev1.PersistentVolumeClaimVolumeSource
	for _, item := range job.Spec.Template.Spec.Volumes {
		if item.Name == volumeTransferVolumeName {
			claim = item.PersistentVolumeClaim
		}
	}
	if claim == nil || claim.ReadOnly != readOnly {
		t.Fatalf("PVC source = %#v, want readOnly=%t", claim, readOnly)
	}
	container := job.Spec.Template.Spec.Containers[0]
	if block && len(container.VolumeDevices) != 1 {
		t.Fatalf("block volume devices = %#v", container.VolumeDevices)
	}
	if !block {
		found := false
		for _, mount := range container.VolumeMounts {
			if mount.Name == volumeTransferVolumeName {
				found = true
				if mount.ReadOnly != readOnly {
					t.Fatalf("PVC mount readOnly = %t, want %t", mount.ReadOnly, readOnly)
				}
			}
		}
		if !found {
			t.Fatal("filesystem PVC mount missing")
		}
	}
}

func environmentValue(values []corev1.EnvVar, name string) string {
	for _, item := range values {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}
