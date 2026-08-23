package kubernetes

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	clientfake "k8s.io/client-go/kubernetes/fake"
)

func TestClusterPressureUsesEffectiveRequestsAndCompleteNodeMetrics(t *testing.T) {
	restartAlways := corev1.ContainerRestartPolicyAlways
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("4"), corev1.ResourceMemory: resource.MustParse("8Gi"),
		}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "demo"},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Containers: []corev1.Container{{Name: "app", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi"),
			}}}},
			InitContainers: []corev1.Container{
				{Name: "sidecar", RestartPolicy: &restartAlways, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("100Mi"),
				}}},
				{Name: "init", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("512Mi"),
				}}},
			},
			Overhead: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
	podLevel := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-level", Namespace: "demo"},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Resources: &corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi"),
			}},
			Containers: []corev1.Container{{Name: "app", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("3"), corev1.ResourceMemory: resource.MustParse("3Gi"),
			}}}},
		},
	}
	terminal := podLevel.DeepCopy()
	terminal.Name = "completed"
	terminal.Status.Phase = corev1.PodSucceeded

	metrics := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "metrics.k8s.io/v1beta1", "kind": "NodeMetrics",
		"metadata": map[string]any{"name": "node-a"},
		"usage":    map[string]any{"cpu": "1500m", "memory": "3Gi"},
	}}
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		nodeMetricsResource: "NodeMetricsList",
	})
	if _, err := dynamicClient.Resource(nodeMetricsResource).Create(context.Background(), metrics, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create fake node metrics: %v", err)
	}
	listedMetrics, listErr := dynamicClient.Resource(nodeMetricsResource).List(context.Background(), metav1.ListOptions{})
	if listErr != nil || len(listedMetrics.Items) != 1 {
		t.Fatalf("list fake node metrics: items=%d err=%v", len(listedMetrics.Items), listErr)
	}
	client := NewClientForInterfaces(clientfake.NewSimpleClientset(node, pod, podLevel, terminal), dynamicClient)

	snapshot, err := client.ClusterPressure(context.Background())
	if err != nil {
		t.Fatalf("ClusterPressure() error = %v", err)
	}
	if snapshot.NodeCount != 1 || snapshot.PodCount != 2 {
		t.Fatalf("counts = nodes %d pods %d", snapshot.NodeCount, snapshot.PodCount)
	}
	if snapshot.CPURequestsMilli != 2700 || snapshot.CPUAllocatableMilli != 4000 {
		t.Fatalf("CPU = requests %d allocatable %d", snapshot.CPURequestsMilli, snapshot.CPUAllocatableMilli)
	}
	wantMemory := parsedValue("1444Mi")
	if snapshot.MemoryRequestsBytes != wantMemory || snapshot.MemoryAllocatableBytes != parsedValue("8Gi") {
		t.Fatalf("memory = requests %d allocatable %d, want requests %d", snapshot.MemoryRequestsBytes, snapshot.MemoryAllocatableBytes, wantMemory)
	}
	if !snapshot.MetricsAvailable || snapshot.CPUUsageMilli != 1500 || snapshot.MemoryUsageBytes != parsedValue("3Gi") {
		t.Fatalf("metrics = %#v", snapshot)
	}
}

func parsedValue(raw string) int64 {
	quantity := resource.MustParse(raw)
	return quantity.Value()
}

func TestClusterPressureTreatsIncompleteMetricsAsUnavailable(t *testing.T) {
	nodes := []runtime.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}}},
	}
	metrics := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "metrics.k8s.io/v1beta1", "kind": "NodeMetrics", "metadata": map[string]any{"name": "node-a"},
		"usage": map[string]any{"cpu": "100m", "memory": "100Mi"},
	}}
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{nodeMetricsResource: "NodeMetricsList"})
	if _, err := dynamicClient.Resource(nodeMetricsResource).Create(context.Background(), metrics, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create fake node metrics: %v", err)
	}
	client := NewClientForInterfaces(clientfake.NewSimpleClientset(nodes...), dynamicClient)
	snapshot, err := client.ClusterPressure(context.Background())
	if err != nil {
		t.Fatalf("ClusterPressure() error = %v", err)
	}
	if snapshot.MetricsAvailable || snapshot.CPUUsageMilli != 0 || snapshot.MemoryUsageBytes != 0 {
		t.Fatalf("incomplete metrics must be unavailable: %#v", snapshot)
	}
}

func TestClusterPressureSpanKeepsParentAndReportsFailureWithoutSensitiveAttributes(t *testing.T) {
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})

	client := NewClientForInterface(clientfake.NewSimpleClientset())
	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, err := client.ClusterPressure(parentCtx)
	parent.End()
	if err == nil {
		t.Fatal("ClusterPressure() expected an error")
	}

	var operation sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "kubernetes.observe_cluster_pressure" {
			operation = span
			break
		}
	}
	if operation == nil {
		t.Fatal("cluster pressure operation span not recorded")
	}
	if operation.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("operation parent = %s, want %s", operation.Parent().SpanID(), parent.SpanContext().SpanID())
	}
	if operation.Status().Code != codes.Error {
		t.Fatalf("operation status = %s, want error", operation.Status().Code)
	}
	if operation.SpanKind() != trace.SpanKindClient {
		t.Fatalf("operation span kind = %s, want client", operation.SpanKind())
	}
	for _, attribute := range operation.Attributes() {
		if strings.Contains(strings.ToLower(attribute.Value.Emit()), "token") || strings.Contains(strings.ToLower(attribute.Value.Emit()), "secret") {
			t.Fatalf("sensitive attribute recorded: %s=%q", attribute.Key, attribute.Value.Emit())
		}
	}
}
