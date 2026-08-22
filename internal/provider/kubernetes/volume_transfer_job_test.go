package kubernetes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPrepareVolumeTransferCreatesIdlePodWithoutCredentials(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := directTransferSpec("import")
	reference, err := client.PrepareVolumeTransfer(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	pod, err := clientset.CoreV1().Pods(spec.Namespace).Get(context.Background(), reference.PodName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	container := pod.Spec.Containers[0]
	if got := container.Command; len(got) != 2 || got[0] != volumeTransferBinaryPath || got[1] != "serve" {
		t.Fatalf("command = %#v", got)
	}
	allowedEnvironment := map[string]bool{
		"LUNA_VOLUME_TRANSFER_ID": true, "LUNA_VOLUME_TRANSFER_DIRECTION": true,
		"LUNA_VOLUME_TRANSFER_FORMAT": true, "LUNA_VOLUME_TRANSFER_VOLUME_MODE": true,
		"LUNA_VOLUME_TRANSFER_CONSISTENCY_MODE": true, "LUNA_VOLUME_TRANSFER_CAPACITY_BYTES": true,
		"LUNA_VOLUME_TRANSFER_MAX_BYTES":      true,
		"LUNA_VOLUME_TRANSFER_EXPECTED_BYTES": true, "LUNA_VOLUME_TRANSFER_EXPECTED_SHA256": true,
		"LUNA_VOLUME_TRANSFER_MAX_FILES": true, "LUNA_VOLUME_TRANSFER_EXPORTED_AT": true,
		"LUNA_VOLUME_TRANSFER_DATA_PATH": true, "LUNA_VOLUME_TRANSFER_TRACEPARENT": true,
		"LUNA_VOLUME_TRANSFER_TRACESTATE": true,
	}
	for _, item := range container.Env {
		if !allowedEnvironment[item.Name] {
			t.Fatalf("unexpected Pod environment variable %q", item.Name)
		}
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatal("service account token automount must be disabled")
	}
	if len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].PersistentVolumeClaim == nil {
		t.Fatalf("volumes = %#v", pod.Spec.Volumes)
	}
	policy, err := clientset.NetworkingV1().NetworkPolicies(spec.Namespace).Get(context.Background(), reference.NetworkPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Spec.Egress) != 0 || len(policy.Spec.PolicyTypes) != 1 || policy.Spec.PolicyTypes[0] != networkingv1.PolicyTypeEgress {
		t.Fatalf("policy = %#v", policy.Spec)
	}
}

func TestPrepareVolumeTransferSeparatesFilesystemAndBlock(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	filesystem, err := client.PrepareVolumeTransfer(context.Background(), directTransferSpec("export"))
	if err != nil {
		t.Fatal(err)
	}
	pod, _ := client.client.CoreV1().Pods(filesystem.Namespace).Get(context.Background(), filesystem.PodName, metav1.GetOptions{})
	container := pod.Spec.Containers[0]
	if len(container.VolumeMounts) != 1 || !container.VolumeMounts[0].ReadOnly {
		t.Fatalf("filesystem mounts = %#v", container.VolumeMounts)
	}

	blockSpec := directTransferSpec("import")
	blockSpec.TransferID = "vtx_block"
	blockSpec.VolumeMode = "Block"
	blockSpec.Format = "raw_zst"
	block, err := client.PrepareVolumeTransfer(context.Background(), blockSpec)
	if err != nil {
		t.Fatal(err)
	}
	pod, _ = client.client.CoreV1().Pods(block.Namespace).Get(context.Background(), block.PodName, metav1.GetOptions{})
	container = pod.Spec.Containers[0]
	if len(container.VolumeDevices) != 1 || container.VolumeDevices[0].DevicePath != volumeTransferBlockDevicePath {
		t.Fatalf("block devices = %#v", container.VolumeDevices)
	}
	if container.SecurityContext.RunAsUser == nil || *container.SecurityContext.RunAsUser != 0 || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatalf("block security context = %#v", container.SecurityContext)
	}
}

func TestObserveAndCleanupVolumeTransfer(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := directTransferSpec("export")
	reference, err := client.PrepareVolumeTransfer(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	pod, _ := clientset.CoreV1().Pods(spec.Namespace).Get(context.Background(), reference.PodName, metav1.GetOptions{})
	pod.Status.Phase = corev1.PodRunning
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if _, err := clientset.CoreV1().Pods(spec.Namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	observation, err := client.ObserveVolumeTransfer(context.Background(), spec.Namespace, spec.TransferID)
	if err != nil || observation.State != "ready" || observation.PodName != reference.PodName {
		t.Fatalf("observation = %#v, %v", observation, err)
	}
	if err := client.CleanupVolumeTransfer(context.Background(), spec.Namespace, spec.TransferID); err != nil {
		t.Fatal(err)
	}
	if _, err := clientset.CoreV1().Pods(spec.Namespace).Get(context.Background(), reference.PodName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("pod after cleanup = %v", err)
	}
	if _, err := clientset.NetworkingV1().NetworkPolicies(spec.Namespace).Get(context.Background(), reference.NetworkPolicyName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("policy after cleanup = %v", err)
	}
}

func TestReadyVolumeTransferPodFencesProjectAndVolumeIdentity(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientForInterface(clientset)
	spec := directTransferSpec("export")
	reference, err := client.PrepareVolumeTransfer(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	pod, err := clientset.CoreV1().Pods(spec.Namespace).Get(context.Background(), reference.PodName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pod.Status.Phase = corev1.PodRunning
	pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if _, err = clientset.CoreV1().Pods(spec.Namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	target := VolumeTransferStreamTarget{
		Namespace: spec.Namespace, TransferID: spec.TransferID,
		ProjectID: spec.ProjectID, ProjectVolumeID: spec.ProjectVolumeID,
	}
	if _, err = client.readyVolumeTransferPod(context.Background(), target, spec.Direction); err != nil {
		t.Fatalf("prepared Pod identity was rejected: %v", err)
	}
	target.ProjectVolumeID = "pvol_other"
	if _, err = client.readyVolumeTransferPod(context.Background(), target, spec.Direction); !errors.Is(err, ErrVolumeTransferNotReady) {
		t.Fatalf("mismatched project volume identity error = %v", err)
	}
}

func TestPrepareVolumeTransferIsIdempotentAndFenced(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset())
	spec := directTransferSpec("import")
	first, err := client.PrepareVolumeTransfer(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.PrepareVolumeTransfer(context.Background(), spec)
	if err != nil || second != first {
		t.Fatalf("idempotent prepare = %#v, %v", second, err)
	}
	changed := spec
	changed.ExpectedBytes++
	if _, err := client.PrepareVolumeTransfer(context.Background(), changed); !errors.Is(err, ErrVolumeTransferConflict) {
		t.Fatalf("changed execution error = %v", err)
	}
}

func TestExecCommandPropagatesCurrentStreamTraceContext(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	traceState, err := trace.ParseTraceState("vendor=value")
	if err != nil {
		t.Fatal(err)
	}
	parent := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1, 2, 3}, SpanID: trace.SpanID{4, 5, 6},
		TraceFlags: trace.FlagsSampled, TraceState: traceState, Remote: true})
	ctx, span := provider.Tracer("test").Start(trace.ContextWithRemoteSpanContext(context.Background(), parent), "api-stream")
	defer span.End()
	client := NewClientForInterface(fake.NewSimpleClientset())
	reference, err := client.PrepareVolumeTransfer(ctx, directTransferSpec("export"))
	if err != nil {
		t.Fatal(err)
	}
	pod, _ := client.client.CoreV1().Pods(reference.Namespace).Get(context.Background(), reference.PodName, metav1.GetOptions{})
	if environmentValue(pod.Spec.Containers[0].Env, "LUNA_VOLUME_TRANSFER_TRACEPARENT") != "" {
		t.Fatal("Pod preparation persisted a stale traceparent")
	}
	command := strings.Join(volumeTransferExecCommand(ctx, "export"), " ")
	if !strings.Contains(command, "LUNA_VOLUME_TRANSFER_TRACEPARENT=00-") || !strings.Contains(command, "LUNA_VOLUME_TRANSFER_TRACESTATE=vendor=value") {
		t.Fatalf("exec command = %q", command)
	}
}

func TestParseVolumeTransferControl(t *testing.T) {
	result, err := parseVolumeTransferControl([]byte("LUNA_VOLUME_TRANSFER_RESULT {\"result\":{\"transferredBytes\":12,\"processedFiles\":1,\"sha256\":\"" + strings.Repeat("a", 64) + "\",\"logicalBytes\":0}}\n"))
	if err != nil || result.TransferredBytes != 12 || result.ProcessedFiles != 1 {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if _, err := parseVolumeTransferControl([]byte("untrusted stderr")); err == nil {
		t.Fatal("missing control record was accepted")
	}
}

func TestOpenVolumeTransferExportRecordsFailedClientSpan(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
	})
	client := NewClientForInterface(fake.NewSimpleClientset())
	if _, err := client.OpenVolumeTransferExport(context.Background(), VolumeTransferStreamTarget{
		Namespace: "project-test", TransferID: "vtx_test", ProjectID: "prj_test", ProjectVolumeID: "pvol_test",
	}); err == nil {
		t.Fatal("missing REST config was accepted")
	}
	for _, span := range recorder.Ended() {
		if span.Name() == "volume.transfer_stream.export" {
			if span.SpanKind() != trace.SpanKindClient || span.Status().Code != codes.Error {
				t.Fatalf("span kind=%v status=%v", span.SpanKind(), span.Status())
			}
			return
		}
	}
	t.Fatal("export client span was not recorded")
}

func TestStreamControlSummaryIsRecordedOnAPISpan(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
	})
	ctx, span := provider.Tracer("test").Start(context.Background(), "direct-stream")
	recordVolumeTransferStreamTelemetry(ctx, "export", VolumeTransferStreamResult{
		TransferredBytes: 12, LogicalBytes: 34, ProcessedFiles: 2, SHA256: strings.Repeat("a", 64),
	}, nil)
	span.End()
	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d", len(ended))
	}
	attributes := map[string]any{}
	for _, item := range ended[0].Attributes() {
		attributes[string(item.Key)] = item.Value.AsInterface()
	}
	if attributes["volume.transfer.outcome"] != "succeeded" || attributes["volume.transfer.transferred_bytes"] != int64(12) ||
		attributes["volume.transfer.logical_bytes"] != int64(34) || attributes["volume.transfer.processed_files"] != int64(2) {
		t.Fatalf("span attributes = %#v", attributes)
	}
	if _, exists := attributes["volume.transfer.sha256"]; exists {
		t.Fatal("high-cardinality checksum leaked into telemetry")
	}
}

func directTransferSpec(direction string) VolumeTransferSpec {
	spec := VolumeTransferSpec{TransferID: "vtx_test", ProjectID: "prj_test", ProjectVolumeID: "pvol_test",
		Namespace: "project-test", ClaimName: "claim-test", Direction: direction, Format: "tar_gz", VolumeMode: "Filesystem",
		ConsistencyMode: "unmounted", Image: "example.invalid/luna-worker:test", CapacityBytes: 1 << 30,
		MaxArchiveBytes: 1 << 30,
		ExportedAt:      time.Unix(1, 0).UTC()}
	if direction == "import" {
		spec.ExpectedBytes = 1024
		spec.ExpectedSHA256 = strings.Repeat("a", 64)
	}
	return spec
}

func environmentValue(environment []corev1.EnvVar, name string) string {
	for _, item := range environment {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}
