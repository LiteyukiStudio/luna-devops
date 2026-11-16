package kubernetes

import (
	"context"
	"reflect"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestPodTemplateResourcesSumsRegularContainers(t *testing.T) {
	container := func(cpuRequest, memoryRequest, cpuLimit, memoryLimit string) corev1.Container {
		requirements := corev1.ResourceRequirements{Requests: corev1.ResourceList{}, Limits: corev1.ResourceList{}}
		for name, value := range map[corev1.ResourceName]string{corev1.ResourceCPU: cpuRequest, corev1.ResourceMemory: memoryRequest} {
			if value != "" {
				requirements.Requests[name] = resource.MustParse(value)
			}
		}
		for name, value := range map[corev1.ResourceName]string{corev1.ResourceCPU: cpuLimit, corev1.ResourceMemory: memoryLimit} {
			if value != "" {
				requirements.Limits[name] = resource.MustParse(value)
			}
		}
		return corev1.Container{Resources: requirements}
	}
	policy := resourcepolicy.Policy{CPURequestPercent: 10, MemoryRequestPercent: 25, CPULimitPercent: 100, MemoryLimitPercent: 100}
	for _, test := range []struct {
		name        string
		annotations map[string]string
		containers  []corev1.Container
		initOnly    []corev1.Container
		want        PodTemplateResources
	}{
		{
			name:        "application container with render policy",
			annotations: map[string]string{ResourcePolicyAnnotation: EncodeResourcePolicyAnnotation(policy)},
			containers:  []corev1.Container{container("100m", "256Mi", "1", "1Gi")},
			want:        PodTemplateResources{CPURequestMilli: 100, MemoryRequestBytes: 256 << 20, CPULimitMilli: 1000, MemoryLimitBytes: 1 << 30, Policy: &policy},
		},
		{
			name:       "sidecar requests and limits are included",
			containers: []corev1.Container{container("100m", "256Mi", "1", "1Gi"), container("50m", "64Mi", "200m", "128Mi")},
			want:       PodTemplateResources{CPURequestMilli: 150, MemoryRequestBytes: 320 << 20, CPULimitMilli: 1200, MemoryLimitBytes: 1152 << 20},
		},
		{
			name:       "a container without limits makes the pod unbounded",
			containers: []corev1.Container{container("100m", "256Mi", "1", "1Gi"), container("50m", "", "", "128Mi")},
			want:       PodTemplateResources{CPURequestMilli: 150, MemoryRequestBytes: 256 << 20, CPULimitMilli: 0, MemoryLimitBytes: 1152 << 20},
		},
		{
			name:       "init containers are not running capacity",
			containers: []corev1.Container{container("", "", "", "")},
			initOnly:   []corev1.Container{container("2", "2Gi", "2", "2Gi")},
			want:       PodTemplateResources{},
		},
		{
			name:        "invalid policy annotation is not recorded",
			annotations: map[string]string{ResourcePolicyAnnotation: `{"cpuRequestPercent":200}`},
			containers:  []corev1.Container{container("100m", "", "", "")},
			want:        PodTemplateResources{CPURequestMilli: 100},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			template := corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: test.annotations},
				Spec:       corev1.PodSpec{Containers: test.containers, InitContainers: test.initOnly},
			}
			if got := podTemplateResources(template); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("podTemplateResources() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestApplicationPodTemplateAnnotatesRenderPolicy(t *testing.T) {
	spec := ApplicationResourcesSpec{Name: "app", Image: "example.invalid/app:1", ResourcePolicy: `{"cpuRequestPercent":10}`}
	template := applicationPodTemplate(spec, map[string]string{}, map[string]string{})
	if got := template.Annotations[ResourcePolicyAnnotation]; got != spec.ResourcePolicy {
		t.Fatalf("resource policy annotation = %q, want %q", got, spec.ResourcePolicy)
	}
}

func TestRuntimeMetricsPodSelectorExcludesHookJobPods(t *testing.T) {
	selector, err := labels.Parse(runtimeMetricsPodLabelSelector("dplt_metrics"))
	if err != nil {
		t.Fatalf("parse selector: %v", err)
	}
	workload := labels.Set{ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: "dplt_metrics", ScopeLabel: "project"}
	hook := labels.Set{ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: "dplt_metrics", HookRunIDLabel: "hrun_1"}
	if !selector.Matches(workload) || selector.Matches(hook) {
		t.Fatalf("selector %q: workload=%v hook=%v", selector, selector.Matches(workload), selector.Matches(hook))
	}
}

func TestRuntimeMetricsCollectsSortedPerPodUsage(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app-prod", Namespace: "ns-demo", Generation: 1},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
					}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			UpdatedReplicas:    2,
			ReadyReplicas:      2,
			AvailableReplicas:  2,
		},
	}

	podMetricsObject := func(name string, extraLabels map[string]string, containers ...map[string]any) *unstructured.Unstructured {
		lbls := map[string]any{
			ManagedByLabel:          ManagedByValue,
			DeploymentTargetIDLabel: "dplt_metrics",
			ScopeLabel:              "project",
		}
		for k, v := range extraLabels {
			lbls[k] = v
		}
		rawContainers := make([]any, 0, len(containers))
		for _, item := range containers {
			rawContainers = append(rawContainers, item)
		}
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "metrics.k8s.io/v1beta1",
			"kind":       "PodMetrics",
			"metadata": map[string]any{
				"name":      name,
				"namespace": "ns-demo",
				"labels":    lbls,
			},
			"containers": rawContainers,
		}}
	}

	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "PodMetricsList"},
	)
	for _, item := range []*unstructured.Unstructured{
		podMetricsObject(
			"app-prod-pod-b",
			nil,
			map[string]any{"name": "app", "usage": map[string]any{"cpu": "400m", "memory": "1Gi"}},
			map[string]any{"name": "sidecar", "usage": map[string]any{"cpu": "50m", "memory": "128Mi"}},
		),
		podMetricsObject(
			"app-prod-pod-a",
			nil,
			map[string]any{"name": "app", "usage": map[string]any{"cpu": "150m", "memory": "512Mi"}},
		),
		podMetricsObject(
			"app-prod-hook-job",
			map[string]string{HookRunIDLabel: "hrun_1"},
			map[string]any{"name": "hook", "usage": map[string]any{"cpu": "900m", "memory": "1Gi"}},
		),
	} {
		if err := dynamicClient.Tracker().Create(gvr, item, "ns-demo"); err != nil {
			t.Fatalf("seed PodMetrics %s: %v", item.GetName(), err)
		}
	}

	client := NewClientForInterfaces(kubefake.NewSimpleClientset(deployment), dynamicClient)
	snapshot, err := client.RuntimeMetrics(context.Background(), RuntimeMetricsOptions{
		Namespace:          "ns-demo",
		DeploymentTargetID: "dplt_metrics",
		WorkloadName:       "app-prod",
		WorkloadType:       "Deployment",
	})
	if err != nil {
		t.Fatalf("RuntimeMetrics returned error: %v", err)
	}
	if !snapshot.Available || snapshot.PodCount != 2 || snapshot.ContainerCount != 3 {
		t.Fatalf("unexpected snapshot header: %#v", snapshot)
	}
	wantPods := []RuntimePodMetricsSnapshot{
		{PodName: "app-prod-pod-a", CPUUsageMilli: 150, MemoryUsageBytes: 512 << 20},
		{PodName: "app-prod-pod-b", CPUUsageMilli: 450, MemoryUsageBytes: (1 << 30) + (128 << 20)},
	}
	if !reflect.DeepEqual(snapshot.Pods, wantPods) {
		t.Fatalf("snapshot.Pods = %#v, want %#v", snapshot.Pods, wantPods)
	}
	if snapshot.CPUUsageMilli != 600 || snapshot.MemoryUsageBytes != (512<<20)+(1<<30)+(128<<20) {
		t.Fatalf("aggregate usage = cpu %d mem %d", snapshot.CPUUsageMilli, snapshot.MemoryUsageBytes)
	}
}
