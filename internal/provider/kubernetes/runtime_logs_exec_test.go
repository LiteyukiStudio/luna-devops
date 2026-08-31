package kubernetes

import (
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	clientexec "k8s.io/client-go/util/exec"
)

func TestRuntimeMetricsExactPodsIsolatesOneKubectlWorkload(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	target := kubectlRuntimePodMetric("web-1", kubectlRuntimeTestLabels("prj_demo", "app_demo"), "125m", "64Mi")
	sibling := kubectlRuntimePodMetric("worker-1", kubectlRuntimeTestLabels("prj_demo", "app_demo"), "900m", "1Gi")
	foreign := kubectlRuntimePodMetric("foreign-1", kubectlRuntimeTestLabels("prj_foreign", "app_demo"), "2", "2Gi")
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "PodMetricsList"})
	for _, item := range []*unstructured.Unstructured{target, sibling, foreign} {
		if _, err := dynamicClient.Resource(gvr).Namespace("project-demo").Create(t.Context(), item, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create metric fixture: %v", err)
		}
	}
	client := NewClientForInterfaces(fake.NewSimpleClientset(), dynamicClient)

	snapshot, err := client.RuntimeMetrics(t.Context(), RuntimeMetricsOptions{
		Namespace:          "project-demo",
		DeploymentTargetID: "kubectl:rcl_demo:prj_demo:app_demo:uid-web",
		ExactPodNames:      []string{"web-1"},
		ProjectID:          "prj_demo",
		ApplicationID:      "app_demo",
		ManagementSource:   KubectlGatewayManagementSourceValue,
		DesiredReplicas:    2,
		ReadyReplicas:      1,
	})
	if err != nil {
		t.Fatalf("RuntimeMetrics() error = %v", err)
	}
	if !snapshot.Available || snapshot.PodCount != 1 || snapshot.ContainerCount != 1 || snapshot.CPUUsageMilli != 125 || snapshot.MemoryUsageBytes != 64*1024*1024 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.DesiredReplicas != 2 || snapshot.ReadyReplicas != 1 {
		t.Fatalf("replica snapshot = %#v", snapshot)
	}
}

func TestRuntimeMetricsExactPodsTreatsEmptySelectionAsKnownZero(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "PodMetricsList"})
	client := NewClientForInterfaces(fake.NewSimpleClientset(), dynamicClient)
	snapshot, err := client.RuntimeMetrics(t.Context(), RuntimeMetricsOptions{
		Namespace:          "project-demo",
		DeploymentTargetID: "kubectl:rcl_demo:prj_demo:app_demo:uid-web",
		ExactPodNames:      []string{},
		ProjectID:          "prj_demo",
		ApplicationID:      "app_demo",
		ManagementSource:   KubectlGatewayManagementSourceValue,
	})
	if err != nil || !snapshot.Available || snapshot.PodCount != 0 {
		t.Fatalf("RuntimeMetrics() = %#v, %v", snapshot, err)
	}
}

func kubectlRuntimePodMetric(name string, labels map[string]string, cpu, memory string) *unstructured.Unstructured {
	unstructuredLabels := make(map[string]any, len(labels))
	for key, value := range labels {
		unstructuredLabels[key] = value
	}
	item := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "metrics.k8s.io/v1beta1",
		"kind":       "PodMetrics",
		"metadata": map[string]any{
			"name": name, "namespace": "project-demo", "labels": unstructuredLabels,
		},
		"containers": []any{map[string]any{
			"name": "app", "usage": map[string]any{"cpu": cpu, "memory": memory},
		}},
	}}
	item.SetCreationTimestamp(metav1.Now())
	return item
}

func TestRuntimeExecOutputIsBoundedAcrossStreams(t *testing.T) {
	output := newRuntimeExecOutput(8)
	if _, err := output.writer(false).Write([]byte("stdout")); err != nil {
		t.Fatal(err)
	}
	if _, err := output.writer(true).Write([]byte("stderr")); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, truncated := output.snapshot()
	if stdout != "stdout" || stderr != "st" || !truncated {
		t.Fatalf("bounded output = stdout %q, stderr %q, truncated %v", stdout, stderr, truncated)
	}
	if len(stdout)+len(stderr) != 8 || strings.Contains(stdout+stderr, "derr") {
		t.Fatalf("combined output exceeded its limit: %q / %q", stdout, stderr)
	}
}

func TestRuntimeExecExitCodePreservesCommandStatus(t *testing.T) {
	code, exited := runtimeExecExitCode(clientexec.CodeExitError{Err: errors.New("command failed"), Code: 42})
	if !exited || code != 42 {
		t.Fatalf("runtime exec exit = (%d, %t), want (42, true)", code, exited)
	}
}

func TestRuntimeExecExitCodeRejectsTransportFailure(t *testing.T) {
	code, exited := runtimeExecExitCode(errors.New("transport unavailable"))
	if exited || code != 0 {
		t.Fatalf("transport failure exit = (%d, %t), want (0, false)", code, exited)
	}
}

func TestSelectPodContainer(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	}

	selected, err := selectPodContainer(pod, "")
	if err != nil {
		t.Fatalf("selectPodContainer returned error: %v", err)
	}
	if selected != "app" {
		t.Fatalf("selectPodContainer default = %q, want app", selected)
	}

	selected, err = selectPodContainer(pod, "sidecar")
	if err != nil {
		t.Fatalf("selectPodContainer sidecar returned error: %v", err)
	}
	if selected != "sidecar" {
		t.Fatalf("selectPodContainer sidecar = %q, want sidecar", selected)
	}

	if _, err := selectPodContainer(pod, "missing"); err == nil {
		t.Fatal("expected missing container to fail")
	}
}
